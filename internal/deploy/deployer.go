package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
)

// sanitizeForLog removes characters that could be used for log injection (e.g. newlines).
func sanitizeForLog(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}

// Regex to parse the alert UID from the filename
var regexAlertFilename = regexp.MustCompile(`alert_rule_(?:.*)_([^\.]+)\.json`)

// Timeout for the HTTP requests
var defaultRequestTimeout = 10 * time.Second

// folderOrg identifies a Grafana folder within a specific org.
type folderOrg struct {
	folderUID string
	orgID     int64
}

// alertRemoval is an alert to delete, and the instances to delete it from.
type alertRemoval struct {
	uid     string
	clients []*shared.GrafanaClient
}

// ruleGroup identifies a rule group within a folder on a single Grafana instance.
type ruleGroup struct {
	folderUID string
	name      string
}

// Structure to store the deployment config
type deploymentConfig struct {
	endpoints      []string
	alertPath      string
	tokens         shared.GrafanaTokens // the service account token for each Grafana instance
	freshDeploy    bool
	folderUID      string      // default folder UID, from Defaults.Integration.FolderID
	orgID          int64       // default org ID, from Defaults.Integration.OrgID
	folderOrgPairs []folderOrg // every distinct (folderUID, orgID) pair across Defaults and all per-conversion overrides
	// endpointPairs maps a normalised Grafana base URL to the (folderUID, orgID) pairs of the
	// configurations deployed to it, so fresh-deploy cleanup only touches what each instance manages.
	// It is nil when the config was not loaded from a file.
	endpointPairs   map[string][]folderOrg
	alertsToAdd     []string
	alertsToRemove  []string
	alertsToUpdate  []string
	groupsIntervals map[ruleGroup]int64 // interval per rule group; a group is identified by its folder and name
	timeout         time.Duration
}

// Structures to unmarshal the YAML config file

type Deployer struct {
	config               deploymentConfig
	client               *shared.GrafanaClient                        // current-operation client, set per-operation in Deploy
	clients              []*shared.GrafanaClient                      // all default Grafana clients
	defaultTokenErr      error                                        // why there are no default clients, when a default instance has no token
	perConversionClients map[string][]*shared.GrafanaClient           // per-conversion Grafana clients
	configNames          []string                                     // every configuration name, to resolve which config an alert file belongs to
	previousNames        []string                                     // every configuration name in the previous config, if one was provided
	previousClients      map[string][]*shared.GrafanaClient           // the instances of each configuration in the previous config
	previousDefault      []*shared.GrafanaClient                      // the default instances in the previous config, nil if there was no previous config
	freshRemovals        []alertRemoval                               // alerts found on specific instances to delete in fresh deploy mode
	groupsToUpdate       map[*shared.GrafanaClient]map[ruleGroup]bool // rule groups touched on each Grafana instance, for interval updates
}

// AlertDeployment identifies a deployed alert and its display title.
type AlertDeployment struct {
	UID   string `json:"uid"`
	Title string `json:"title"`
	// Instance is the base URL of the Grafana instance the alert was deployed to, so a link to
	// the alert points at the right instance when configurations deploy to different ones
	Instance string `json:"instance,omitempty"`
}

func NewDeployer() *Deployer {
	return &Deployer{
		groupsToUpdate:       map[*shared.GrafanaClient]map[ruleGroup]bool{},
		perConversionClients: map[string][]*shared.GrafanaClient{},
	}
}

// clientsForAlertFile returns the appropriate GrafanaClients for the given alert file,
// based on the conversion name prefix in the filename. Falls back to d.clients.
// The owning configuration is the longest matching name across all configurations, not just
// those with a grafana_instance override, so "aws_cloudtrail" files never go to "aws" clients.
func (d *Deployer) clientsForAlertFile(filename string) []*shared.GrafanaClient {
	if convName, ok := shared.MatchConfigName(alertFileConfigPart(filename), d.currentConfigNames()); ok {
		if clients, ok := d.perConversionClients[convName]; ok {
			return clients
		}
	}
	return d.clients
}

// alertFileConfigPart returns the part of an alert filename that starts with its configuration name.
func alertFileConfigPart(filename string) string {
	return strings.TrimPrefix(filepath.Base(filename), "alert_rule_")
}

// currentConfigNames returns every configuration name in the current config.
func (d *Deployer) currentConfigNames() []string {
	names := append([]string(nil), d.configNames...)
	for convName := range d.perConversionClients {
		names = append(names, convName)
	}
	return names
}

// clientsForRemovedAlertFile returns every instance the rule of a removed alert file may be on.
// The current config routes it as for any other alert file, but a file is usually removed
// together with a change to the config: its configuration may be gone, or its instances changed.
// So the instances the previous config routed it to are included too. A file that matches no
// configuration in the current config, and has no previous config to say where it was deployed,
// is removed from every instance in the current config (a UID that isn't on an instance is
// ignored there).
func (d *Deployer) clientsForRemovedAlertFile(filename string) []*shared.GrafanaClient {
	part := alertFileConfigPart(filename)
	var clients []*shared.GrafanaClient
	_, inCurrent := shared.MatchConfigName(part, d.currentConfigNames())
	if inCurrent {
		clients = append(clients, d.clientsForAlertFile(filename)...)
	}
	previousName, inPrevious := shared.MatchConfigName(part, d.previousNames)
	if inPrevious {
		clients = append(clients, d.previousClients[previousName]...)
	} else if d.previousClients != nil {
		// Alert files matching no configuration were routed to the default instances
		clients = append(clients, d.previousDefault...)
	}
	if !inCurrent && !inPrevious && d.previousClients == nil {
		clients = d.allClients()
	}
	return uniqueClients(clients)
}

// uniqueClients removes clients for an instance already listed, keeping the first of each.
func uniqueClients(clients []*shared.GrafanaClient) []*shared.GrafanaClient {
	seen := make(map[string]bool, len(clients))
	unique := make([]*shared.GrafanaClient, 0, len(clients))
	for _, c := range clients {
		if !seen[c.BaseURL()] {
			seen[c.BaseURL()] = true
			unique = append(unique, c)
		}
	}
	return unique
}

// markGroupForUpdate records that the alert's rule group was touched on the current client,
// so its interval is only checked on instances where the group can exist.
func (d *Deployer) markGroupForUpdate(alert model.Alert) {
	if d.groupsToUpdate == nil {
		d.groupsToUpdate = map[*shared.GrafanaClient]map[ruleGroup]bool{}
	}
	if d.groupsToUpdate[d.client] == nil {
		d.groupsToUpdate[d.client] = map[ruleGroup]bool{}
	}
	d.groupsToUpdate[d.client][ruleGroup{folderUID: alert.FolderUID, name: alert.RuleGroup}] = true
}

// allClients returns all unique clients across default and per-conversion clients.
func (d *Deployer) allClients() []*shared.GrafanaClient {
	seen := make(map[*shared.GrafanaClient]bool)
	result := make([]*shared.GrafanaClient, 0)
	for _, c := range d.clients {
		if !seen[c] {
			seen[c] = true
			result = append(result, c)
		}
	}
	// Visit per-conversion clients in name order so the result is deterministic
	convNames := make([]string, 0, len(d.perConversionClients))
	for convName := range d.perConversionClients {
		convNames = append(convNames, convName)
	}
	sort.Strings(convNames)
	for _, convName := range convNames {
		for _, c := range d.perConversionClients[convName] {
			if !seen[c] {
				seen[c] = true
				result = append(result, c)
			}
		}
	}
	return result
}

// requireClientsForAlertFile is clientsForAlertFile, but fails instead of returning no clients,
// so an alert is never silently skipped because no Grafana instance resolves for it.
func (d *Deployer) requireClientsForAlertFile(filename string) ([]*shared.GrafanaClient, error) {
	clients := d.clientsForAlertFile(filename)
	if len(clients) == 0 && d.defaultTokenErr != nil {
		return nil, fmt.Errorf("alert file %s matches no configuration, so it would be deployed to the default Grafana instances: %w", filename, d.defaultTokenErr)
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("no Grafana instance is configured for alert file %s: set grafana_instance in defaults.deployment or in the matching configuration", filename)
	}
	return clients, nil
}

// pendingRemovals returns the alerts to delete: those named by removed alert files, routed to
// the instances of their configuration, plus any found on specific instances by a fresh deploy.
func (d *Deployer) pendingRemovals() ([]alertRemoval, error) {
	removals := make([]alertRemoval, 0, len(d.config.alertsToRemove)+len(d.freshRemovals))
	for _, alertFile := range d.config.alertsToRemove {
		alertUID := getAlertUIDFromFilename(filepath.Base(alertFile))
		if alertUID == "" {
			return nil, fmt.Errorf("invalid alert filename: %s", alertFile)
		}
		clients := d.clientsForRemovedAlertFile(alertFile)
		if len(clients) == 0 {
			return nil, fmt.Errorf("no Grafana instance is configured for alert file %s: set grafana_instance in defaults.deployment or in the matching configuration", alertFile)
		}
		removals = append(removals, alertRemoval{uid: alertUID, clients: clients})
	}
	return append(removals, d.freshRemovals...), nil
}

// newClient creates a client for a Grafana instance, using the token for that instance.
func (d *Deployer) newClient(endpoint string, timeout time.Duration) (*shared.GrafanaClient, error) {
	token, err := d.config.tokens.For(endpoint)
	if err != nil {
		return nil, err
	}
	return shared.NewGrafanaClient(endpoint, token, "sigma-rule-deployment/deployer", timeout), nil
}

// SetClient creates the clients for the default instances. A default instance with no token
// gets no client, so it can't be used with another's token. LoadConfig requires a token for every
// instance a configuration uses, so this only leaves out default instances that are all overridden.
// An alert file that matches no configuration is deployed to the default instances, and fails
// then if one of them has no token.
func (d *Deployer) SetClient() {
	d.clients = make([]*shared.GrafanaClient, 0, len(d.config.endpoints))
	d.defaultTokenErr = nil
	for _, ep := range d.config.endpoints {
		client, err := d.newClient(ep, d.config.timeout)
		if err != nil {
			d.defaultTokenErr = err
			d.clients = nil
			break
		}
		d.clients = append(d.clients, client)
	}
	if len(d.clients) > 0 {
		d.client = d.clients[0]
	}
}

func (d *Deployer) IsFreshDeploy() bool {
	return d.config.freshDeploy
}

func (d *Deployer) Deploy(ctx context.Context) ([]AlertDeployment, []AlertDeployment, []AlertDeployment, error) {
	// Lists to store the alerts that were created, updated and deleted at any point during the deployment
	alertsCreated := make([]AlertDeployment, 0, len(d.config.alertsToAdd))
	alertsUpdated := make([]AlertDeployment, 0, len(d.config.alertsToUpdate))
	alertsDeleted := make([]AlertDeployment, 0, len(d.config.alertsToRemove))

	// Resolve the instances for every alert up front, so a missing instance fails the deployment
	// before anything has been changed rather than part way through
	removals, err := d.pendingRemovals()
	if err != nil {
		return alertsCreated, alertsUpdated, alertsDeleted, err
	}
	for _, alertFile := range append(append([]string{}, d.config.alertsToAdd...), d.config.alertsToUpdate...) {
		if _, err := d.requireClientsForAlertFile(alertFile); err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
	}

	log.Printf("Preparing to deploy %d alerts, update %d alerts and delete %d alerts",
		len(d.config.alertsToAdd), len(d.config.alertsToUpdate), len(removals))

	// Process alert DELETIONS
	// It is important to do this first for the case where an alert
	// is recreated in a different file (with a different UID), to avoid conflicts on the alert title
	// By deleting the old one first, we can then create the new one without issues
	for _, removal := range removals {
		for _, client := range removal.clients {
			d.client = client
			alertTitle := d.getAlertTitle(ctx, removal.uid)
			uid, err := d.deleteAlert(ctx, removal.uid)
			if err != nil {
				return alertsCreated, alertsUpdated, alertsDeleted, err
			}
			// UID could be empty if the alert was not found
			// In this case, we don't want to add it to the list of deleted alerts
			if uid != "" {
				alertsDeleted = append(alertsDeleted, AlertDeployment{UID: uid, Title: alertTitle, Instance: client.BaseURL()})
			}
		}
	}

	// Process alert CREATIONS
	for _, alertFile := range d.config.alertsToAdd {
		content, err := shared.ReadLocalFile(alertFile)
		if err != nil {
			log.Printf("Can't read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		for _, client := range d.clientsForAlertFile(alertFile) {
			d.client = client
			deployment, updated, err := d.createAlert(ctx, content, true)
			if err != nil {
				return alertsCreated, alertsUpdated, alertsDeleted, err
			}
			deployment.Instance = client.BaseURL()
			if updated {
				// If the alert was updated, we need to add it to the list of updated alerts
				alertsUpdated = append(alertsUpdated, deployment)
			} else {
				// If the alert was created, we need to add it to the list of created alerts
				alertsCreated = append(alertsCreated, deployment)
			}
		}
	}

	// Process alert UPDATES
	for _, alertFile := range d.config.alertsToUpdate {
		content, err := shared.ReadLocalFile(alertFile)
		if err != nil {
			log.Printf("Can't read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		for _, client := range d.clientsForAlertFile(alertFile) {
			d.client = client
			deployment, created, err := d.updateAlert(ctx, content, true)
			if err != nil {
				return alertsCreated, alertsUpdated, alertsDeleted, err
			}
			deployment.Instance = client.BaseURL()
			// Sometimes the alert to update doesn't exist anymore (e.g. it was deleted manually)
			// In this case, we re-create it instead of updating it
			// So we take this into account for the reporting
			if created {
				// If the alert was created, we need to add it to the list of created alerts
				alertsCreated = append(alertsCreated, deployment)
			} else {
				// If the alert was updated, we need to add it to the list of updated alerts
				alertsUpdated = append(alertsUpdated, deployment)
			}
		}
	}

	// Process alert group interval updates, only on the instances where each group was touched.
	// allClients gives a stable client order; groups are sorted for deterministic requests and logs.
	for _, client := range d.allClients() {
		groups := make([]ruleGroup, 0, len(d.groupsToUpdate[client]))
		for group := range d.groupsToUpdate[client] {
			groups = append(groups, group)
		}
		sort.Slice(groups, func(i, j int) bool {
			if groups[i].folderUID != groups[j].folderUID {
				return groups[i].folderUID < groups[j].folderUID
			}
			return groups[i].name < groups[j].name
		})
		d.client = client
		for _, group := range groups {
			// The folder comes from the alert itself, which may be a per-conversion folder_id override
			if err := d.updateAlertGroupInterval(ctx, group.folderUID, group.name, d.config.groupsIntervals[group]); err != nil {
				return alertsCreated, alertsUpdated, alertsDeleted, err
			}
		}
	}

	return alertsCreated, alertsUpdated, alertsDeleted, nil
}

func (d *Deployer) WriteOutput(alertsCreated []AlertDeployment, alertsUpdated []AlertDeployment, alertsDeleted []AlertDeployment) error {
	alertsCreatedStr := strings.Join(alertDeploymentUIDs(alertsCreated), " ")
	alertsUpdatedStr := strings.Join(alertDeploymentUIDs(alertsUpdated), " ")
	alertsDeletedStr := strings.Join(alertDeploymentUIDs(alertsDeleted), " ")

	if err := shared.SetOutput("alerts_created", alertsCreatedStr); err != nil {
		return err
	}
	if err := shared.SetOutput("alerts_updated", alertsUpdatedStr); err != nil {
		return err
	}
	if err := shared.SetOutput("alerts_deleted", alertsDeletedStr); err != nil {
		return err
	}
	if err := setAlertDeploymentOutput("alerts_created_details", alertsCreated); err != nil {
		return err
	}
	if err := setAlertDeploymentOutput("alerts_updated_details", alertsUpdated); err != nil {
		return err
	}
	if err := setAlertDeploymentOutput("alerts_deleted_details", alertsDeleted); err != nil {
		return err
	}
	return nil
}

func alertDeploymentUIDs(alerts []AlertDeployment) []string {
	uids := make([]string, 0, len(alerts))
	for _, alert := range alerts {
		if alert.UID != "" {
			uids = append(uids, alert.UID)
		}
	}
	return uids
}

func setAlertDeploymentOutput(name string, alerts []AlertDeployment) error {
	if alerts == nil {
		alerts = []AlertDeployment{}
	}
	details, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("error encoding deployment details: %w", err)
	}
	return shared.SetOutput(name, string(details))
}

func (d *Deployer) LoadConfig(_ context.Context) error {
	// Load the sigma rule deployer config file
	configFile := os.Getenv("CONFIG_PATH")
	if configFile == "" {
		return fmt.Errorf("Deployer config file is not set or empty")
	}
	// Read and parse the YAML config file
	configYAML, err := shared.LoadConfigFromFile(configFile)
	if err != nil {
		return err
	}
	d.config = deploymentConfig{
		endpoints:       []string(configYAML.Defaults.Deployment.GrafanaInstance),
		alertPath:       filepath.Clean(configYAML.Folders.DeploymentPath),
		orgID:           configYAML.Defaults.Integration.OrgID,
		folderUID:       configYAML.Defaults.Integration.FolderID,
		groupsIntervals: make(map[ruleGroup]int64),
		timeout:         defaultRequestTimeout,
	}
	d.config.folderOrgPairs = []folderOrg{{folderUID: d.config.folderUID, orgID: d.config.orgID}}

	// Parse timeout if provided
	if configYAML.Defaults.Deployment.Timeout != "" {
		parsedTimeout, err := shared.ParseDuration(configYAML.Defaults.Deployment.Timeout)
		if err != nil {
			log.Printf("Warning: Invalid timeout format in config, using default: %v\n", err)
		} else {
			d.config.timeout = parsedTimeout
		}
	}

	// Get the rest of the config from the environment variables
	rawTokens := os.Getenv("DEPLOYER_GRAFANA_SA_TOKEN")
	if strings.TrimSpace(rawTokens) == "" {
		return fmt.Errorf("the Grafana SA token is not set or empty")
	}
	tokens, err := shared.ParseGrafanaTokens(rawTokens)
	if err != nil {
		return fmt.Errorf("invalid Grafana SA token: %w", err)
	}
	d.config.tokens = tokens

	// Every configuration must resolve to at least one Grafana instance, otherwise its alerts
	// would be silently skipped. Build per-conversion clients for configurations that override
	// the grafana_instance or the timeout.
	d.perConversionClients = make(map[string][]*shared.GrafanaClient)
	d.configNames = make([]string, 0, len(configYAML.Configurations))
	for _, cfg := range configYAML.Configurations {
		d.configNames = append(d.configNames, cfg.Name)
		instances := cfg.Deployment.GrafanaInstance.WithDefault(configYAML.Defaults.Deployment.GrafanaInstance)
		if len(instances) == 0 {
			return fmt.Errorf("configuration %q has no Grafana instance: set grafana_instance in defaults.deployment or in the configuration's deployment block", cfg.Name)
		}
		for _, ep := range instances {
			if _, err := d.config.tokens.For(ep); err != nil {
				return fmt.Errorf("configuration %q: %w", cfg.Name, err)
			}
		}

		timeout := d.config.timeout
		hasTimeout := false
		if cfg.Deployment.Timeout != "" {
			if parsedTimeout, err := shared.ParseDuration(cfg.Deployment.Timeout); err != nil {
				log.Printf("Warning: Invalid timeout format for configuration %s, using default: %v\n", sanitizeForLog(cfg.Name), err) //nolint:gosec // G706: cfg.Name sanitized with sanitizeForLog before logging
			} else {
				timeout = parsedTimeout
				hasTimeout = true
			}
		}
		if len(cfg.Deployment.GrafanaInstance) == 0 && !hasTimeout {
			continue // uses the default clients
		}
		clients := make([]*shared.GrafanaClient, 0, len(instances))
		for _, ep := range instances {
			client, err := d.newClient(ep, timeout)
			if err != nil {
				return fmt.Errorf("configuration %q: %w", cfg.Name, err)
			}
			clients = append(clients, client)
		}
		d.perConversionClients[cfg.Name] = clients
	}
	if len(configYAML.Configurations) == 0 && len(configYAML.Defaults.Deployment.GrafanaInstance) == 0 {
		return fmt.Errorf("no Grafana instance is configured: set grafana_instance in defaults.deployment")
	}
	if len(configYAML.Configurations) == 0 {
		for _, ep := range configYAML.Defaults.Deployment.GrafanaInstance {
			if _, err := d.config.tokens.For(ep); err != nil {
				return err
			}
		}
	}

	// Extract the groups intervals from the conversion config
	defaultInterval := "5m"
	if configYAML.Defaults.Integration.TimeWindow != "" {
		defaultInterval = configYAML.Defaults.Integration.TimeWindow
	}
	seenFolderOrgPairs := map[folderOrg]bool{d.config.folderOrgPairs[0]: true}
	d.config.endpointPairs = make(map[string][]folderOrg)
	for _, config := range configYAML.Configurations {
		interval := defaultInterval
		if config.Integration.TimeWindow != "" {
			interval = config.Integration.TimeWindow
		}
		intervalDuration, err := shared.ParseDuration(interval)
		log.Printf("Interval duration from %s: %d", sanitizeForLog(interval), int64(intervalDuration.Seconds())) //nolint:gosec // G706: interval sanitized with sanitizeForLog before logging
		if err != nil || int64(intervalDuration.Seconds()) <= 0 {
			return fmt.Errorf("error parsing time window %s: %v", interval, err)
		}
		// This fallback chain, and the folderUID/orgID ones below, must match the ones used
		// to build the alert itself at integration time (internal/integrate/integrator.go),
		// otherwise the deploy stage tracks the wrong rule group/folder/org for an alert and
		// silently applies the wrong interval, or misses the alert during fresh-deploy cleanup.
		groupName := shared.GetConfigValue(config.Integration.RuleGroup, configYAML.Defaults.Integration.RuleGroup, "Default")
		folderUID := shared.GetConfigValue(config.Integration.FolderID, configYAML.Defaults.Integration.FolderID, "")
		// Grafana identifies a rule group by its folder and name, so the same group name in
		// different folders is a different group and may have its own interval
		group := ruleGroup{folderUID: folderUID, name: groupName}
		if _, ok := d.config.groupsIntervals[group]; !ok {
			d.config.groupsIntervals[group] = int64(intervalDuration.Seconds())
			log.Printf("Setting interval for rule group %s in folder %s to %d", sanitizeForLog(groupName), sanitizeForLog(folderUID), d.config.groupsIntervals[group]) //nolint:gosec // G706: group name and folder sanitized with sanitizeForLog before logging
		} else if d.config.groupsIntervals[group] != int64(intervalDuration.Seconds()) {
			return fmt.Errorf("time window for rule group %s in folder %s is different between conversion configs", groupName, folderUID)
		}

		orgID := shared.GetConfigValueInt64(config.Integration.OrgID, configYAML.Defaults.Integration.OrgID)
		pair := folderOrg{folderUID: folderUID, orgID: orgID}
		// Record the pair against each instance this configuration deploys to, so fresh-deploy
		// cleanup on an instance only matches the folders and orgs deployed there
		for _, ep := range config.Deployment.GrafanaInstance.WithDefault(configYAML.Defaults.Deployment.GrafanaInstance) {
			key := shared.NormalizeBaseURL(ep)
			if !slices.Contains(d.config.endpointPairs[key], pair) {
				d.config.endpointPairs[key] = append(d.config.endpointPairs[key], pair)
			}
		}
		if !seenFolderOrgPairs[pair] {
			seenFolderOrgPairs[pair] = true
			d.config.folderOrgPairs = append(d.config.folderOrgPairs, pair)
		}
	}

	d.loadPreviousConfig(os.Getenv("PREVIOUS_CONFIG_PATH"))

	// Retrieve the fresh deploy flag
	freshDeploy := strings.ToLower(os.Getenv("DEPLOYER_FRESH_DEPLOY")) == "true"
	d.config.freshDeploy = freshDeploy

	return nil
}

// loadPreviousConfig reads the config as it was before the change being deployed, so removed
// alerts can also be deleted from the instances it deployed them to. The previous config is
// optional: it is absent for the first deployment, and may not be a valid v2 config. Either way
// removals fall back to the current config alone, which is logged rather than failing the deployment.
func (d *Deployer) loadPreviousConfig(path string) {
	if path == "" {
		return
	}
	previous, err := shared.LoadConfigFromFile(path)
	if err != nil {
		log.Printf("Warning: ignoring the previous config, so removed alerts are only deleted from the instances in the current config: %v", err) //nolint:gosec // G706: the error comes from a config file path set by the action
		return
	}
	d.previousNames = make([]string, 0, len(previous.Configurations))
	d.previousClients = make(map[string][]*shared.GrafanaClient, len(previous.Configurations))
	newClients := func(instances model.GrafanaInstances) []*shared.GrafanaClient {
		clients := make([]*shared.GrafanaClient, 0, len(instances))
		for _, ep := range instances {
			// An instance that is no longer in the config may have no token: leave it out
			// rather than use another instance's
			token, err := d.config.tokens.For(ep)
			if err != nil {
				log.Printf("Warning: removed alerts are not deleted from %s: %v", sanitizeForLog(ep), err) //nolint:gosec // G706: ep sanitized with sanitizeForLog before logging
				continue
			}
			clients = append(clients, shared.NewGrafanaClient(ep, token, "sigma-rule-deployment/deployer", d.config.timeout))
		}
		return clients
	}
	d.previousDefault = newClients(previous.Defaults.Deployment.GrafanaInstance)
	for _, cfg := range previous.Configurations {
		d.previousNames = append(d.previousNames, cfg.Name)
		d.previousClients[cfg.Name] = newClients(cfg.Deployment.GrafanaInstance.WithDefault(previous.Defaults.Deployment.GrafanaInstance))
	}
}

func (d *Deployer) ConfigNormalMode() error {
	// For a normal deployment, we look at the changes in the alert folder
	alertsToAdd := []string{}
	alertsToDelete := []string{}
	alertsToUpdate := []string{}

	addedFiles := os.Getenv("ADDED_FILES")
	deletedFiles := os.Getenv("DELETED_FILES")
	modifiedFiles := os.Getenv("MODIFIED_FILES")
	copiedFiles := os.Getenv("COPIED_FILES")

	addedFilesList := strings.Split(addedFiles, " ")
	deletedFilesList := strings.Split(deletedFiles, " ")
	modifiedFilesList := strings.Split(modifiedFiles, " ")
	copiedFilesList := strings.Split(copiedFiles, " ")

	// Add the modified files to the alert lists if they are in the right filder (alertPath)
	for _, filePath := range addedFilesList {
		alertsToAdd = addToAlertList(alertsToAdd, filePath, d.config.alertPath)
	}
	// Copied files are treated as added files
	for _, filePath := range copiedFilesList {
		alertsToAdd = addToAlertList(alertsToAdd, filePath, d.config.alertPath)
	}
	for _, filePath := range deletedFilesList {
		alertsToDelete = addToAlertList(alertsToDelete, filePath, d.config.alertPath)
	}
	for _, filePath := range modifiedFilesList {
		alertsToUpdate = addToAlertList(alertsToUpdate, filePath, d.config.alertPath)
	}
	// Renamed files will be considered a deletion and a creation via the changed-files action configuration.
	// This helps to avoid issues where we have both an alert being deleted and another one created in a single PR,
	// as Git would typically consider this as a rename (which poses isues for our deployment logic)

	d.config.alertsToAdd = alertsToAdd
	d.config.alertsToRemove = alertsToDelete
	d.config.alertsToUpdate = alertsToUpdate

	return nil
}

func (d *Deployer) ConfigFreshDeployment(ctx context.Context) error {
	log.Println("Running in fresh deployment mode.")
	// For a fresh deployment, we'll deploy every alert in the deploment folder, regardless of the changes
	alertsToAdd, err := d.listAlertsInDeploymentFolder()
	if err != nil {
		return fmt.Errorf("error listing alerts in deployment folder: %v", err)
	}
	// List the current alerts on every Grafana instance so they can be deleted first.
	// This must include per-conversion instance overrides, not just the default clients,
	// otherwise alerts on an instance only reachable via a conversion's grafana_instance
	// override are never discovered for cleanup. Each instance is matched against only the
	// (folder, org) pairs deployed to it, and its alerts are deleted from that same instance.
	freshRemovals := []alertRemoval{}
	seenEndpoints := make(map[string]bool)
	for _, client := range d.allClients() {
		// Clients that share an endpoint (e.g. differing only by timeout) list it once
		if seenEndpoints[client.BaseURL()] {
			continue
		}
		seenEndpoints[client.BaseURL()] = true
		if len(d.pairsForClient(client)) == 0 {
			log.Printf("No configuration deploys to %s, skipping cleanup", client.BaseURL())
			continue
		}
		d.client = client
		alerts, err := d.listAlerts(ctx)
		if err != nil {
			return fmt.Errorf("error listing alerts: %v", err)
		}
		for _, uid := range alerts {
			freshRemovals = append(freshRemovals, alertRemoval{uid: uid, clients: []*shared.GrafanaClient{client}})
		}
	}
	d.config.alertsToAdd = alertsToAdd
	d.config.alertsToRemove = []string{}
	d.freshRemovals = freshRemovals
	d.config.alertsToUpdate = []string{}

	return nil
}

func addToAlertList(alertList []string, file string, prefix string) []string {
	// We first check that the modified files are in the expected folder
	// That is, the folder which contains the alert files
	// Otherwise, we ignore this file as they are unrelated to the deployment

	// File pattern to match every file in the alert folder
	pattern := prefix + string(filepath.Separator) + "*"
	matched, err := filepath.Match(pattern, file)
	if matched && err == nil {
		alertList = append(alertList, file)
	}
	return alertList
}

func (d *Deployer) createAlert(ctx context.Context, content string, updateIfExists bool) (AlertDeployment, bool, error) {
	//  Return values:
	// 1. Deployment details of the alert
	// 2. Whether the alert was updated instead of create. If updateIfExists is false, this will always be false.
	// 3. Error if any

	// For now, we are only interested in the response message, which provides context in case of errors
	type Response struct {
		Message string `json:"message"`
	}

	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return AlertDeployment{}, false, err
	}
	deployment := AlertDeployment{UID: alert.UID, Title: alert.Title}
	d.markGroupForUpdate(alert)

	// Prepare the request
	res, err := d.client.PostRaw(ctx, "api/v1/provisioning/alert-rules", []byte(content))
	if err != nil {
		return AlertDeployment{}, false, err
	}
	defer res.Body.Close()

	// Check the response
	resp := Response{}
	if err := shared.ReadJSONResponse(res, &resp); err != nil {
		return AlertDeployment{}, false, err
	}

	switch res.StatusCode {
	case http.StatusCreated:
		// Alert created successfully
		log.Printf("Alert %s (%s) created", alert.UID, alert.Title)
		return deployment, false, nil
	case http.StatusConflict:
		// Another alert with the same UID exists
		// If the alert already exists and we don't want to update it, we return an error
		if !updateIfExists {
			log.Printf("Alert %s (%s) conflicts with another alert", alert.UID, alert.Title)
			return AlertDeployment{}, false, fmt.Errorf("error creating alert: returned status %s", res.Status)
		}
		// Otherwise, we need to check if it's a re-creation (in which case we proceed to update it instead)
		// or an actual conflict
		updatedDeployment, err := d.tryToUpdateConflictingAlert(ctx, alert, content)
		if err != nil {
			return AlertDeployment{}, false, err
		}

		return updatedDeployment, true, nil
	default:
		log.Printf("Can't create alert %s (%s). Status: %d, Message: %s", alert.UID, alert.Title, res.StatusCode, resp.Message)
		return AlertDeployment{}, false, fmt.Errorf("error creating alert: returned status %s", res.Status)
	}
}

func (d *Deployer) tryToUpdateConflictingAlert(ctx context.Context, alert model.Alert, content string) (AlertDeployment, error) {
	// Retrieve the existing alert it's conflicting with
	existingAlert, err := d.getAlert(ctx, alert.UID)
	if err != nil {
		log.Printf("Can't get alert %s. Error: %v", alert.UID, err)
		return AlertDeployment{}, fmt.Errorf("error getting alert: %v", err)
	}
	// Check if the conflicting alerts have the same parameters
	// Otherwise, it's an actual conflict
	if !d.checkAlertsMatch(existingAlert, alert) {
		// The alert already exists, but with different parameters
		log.Printf("Alert %s (%s) is conflicting with another alert having the same UID", alert.UID, alert.Title)
		return AlertDeployment{}, fmt.Errorf("error creating alert: %v", err)
	}
	// The alert already exists, but with the same parameters
	// In this case, we can proceed to update it
	log.Printf("Alert %s (%s) already exists, updating it instead", alert.UID, alert.Title)
	deployment, _, err := d.updateAlert(ctx, content, false)
	if err != nil {
		log.Printf("Can't update alert %s: %v", alert.UID, err)
		return AlertDeployment{}, fmt.Errorf("error updating alert: %v", err)
	}
	return deployment, nil
}

func (d *Deployer) updateAlert(ctx context.Context, content string, createIfNotFound bool) (AlertDeployment, bool, error) {
	//  Return values:
	// 1. Deployment details of the alert
	// 2. Whether the alert had to be (re-)created. If createIfNotFound is false, this will always be false.
	// 3. Error if any

	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return AlertDeployment{}, false, err
	}
	deployment := AlertDeployment{UID: alert.UID, Title: alert.Title}
	d.markGroupForUpdate(alert)

	// Prepare the request
	path := fmt.Sprintf("api/v1/provisioning/alert-rules/%s", alert.UID)
	res, err := d.client.PutRaw(ctx, path, []byte(content))
	if err != nil {
		return AlertDeployment{}, false, err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode == http.StatusNotFound && createIfNotFound {
		// If an alert has been manually deleted in Grafana, and the deployer isn't aware of it, then next time it's modified
		// it will try to update it. This will fail with a 404 error, so we need to create it instead
		log.Printf("Alert %s not found for update, (re-)creating it instead", alert.UID)
		createdDeployment, _, err := d.createAlert(ctx, content, false)
		if err != nil {
			log.Printf("Can't create alert: %v", err)
			return AlertDeployment{}, true, err
		}
		return createdDeployment, true, nil
	} else if res.StatusCode != http.StatusOK {
		log.Printf("Can't update alert. Status: %d", res.StatusCode)
		return AlertDeployment{}, false, fmt.Errorf("error updating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) updated", alert.UID, alert.Title)

	return deployment, false, nil
}

func (d *Deployer) updateAlertGroupInterval(ctx context.Context, folderUID string, group string, interval int64) error {
	log.Printf("Checking alert group interval for %s/%s to %d", folderUID, group, interval)
	path := fmt.Sprintf("api/v1/provisioning/folder/%s/rule-groups/%s", folderUID, group)

	// Get the current alert group content
	res, err := d.client.Get(ctx, path)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Check the response
	if err := shared.CheckStatusCode(res, http.StatusOK); err != nil {
		log.Printf("Can't find alert group. Status: %d", res.StatusCode)
		return fmt.Errorf("error finding alert group %s/%s: %w", folderUID, group, err)
	}
	resp := model.AlertRuleGroup{}
	if err := shared.ReadJSONResponse(res, &resp); err != nil {
		return err
	}

	if resp.Interval != interval {
		log.Printf("Updating alert group interval for %s/%s to %d", folderUID, group, interval)
		resp.Interval = interval

		// Note the implicit race condition - if a rule is added to the group between these two requests,
		// they will be overwritten by this request. There's nothing we can do about this; alerting
		// would need to update their API to allow the interval to be updated independent of the alert rules
		updateRes, err := d.client.Put(ctx, path, resp)
		if err != nil {
			return err
		}
		defer updateRes.Body.Close()

		if err := shared.CheckStatusCode(updateRes, http.StatusOK); err != nil {
			log.Printf("Can't update alert group interval. Status: %d", updateRes.StatusCode)
			return fmt.Errorf("error updating alert group interval %s/%s: %w", folderUID, group, err)
		}
	}

	return nil
}

func (d *Deployer) deleteAlert(ctx context.Context, uid string) (string, error) {
	// Prepare the request
	path := fmt.Sprintf("api/v1/provisioning/alert-rules/%s", uid)
	res, err := d.client.Delete(ctx, path)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode == http.StatusNotFound {
		log.Printf("Alert %s not found for deletion. Ignoring.", uid)
		return "", nil
	} else if res.StatusCode != http.StatusNoContent {
		log.Printf("Can't delete alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error deleting alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s deleted", uid)

	return uid, nil
}

func (d *Deployer) getAlertTitle(ctx context.Context, uid string) string {
	alert, err := d.getAlert(ctx, uid)
	if err != nil || strings.TrimSpace(alert.Title) == "" {
		log.Printf("Can't retrieve title for alert %s. Using UID instead.", sanitizeForLog(uid))
		return uid
	}
	return alert.Title
}

func (d *Deployer) checkAlertsMatch(a, b model.Alert) bool {
	if a.UID != b.UID {
		return false
	}
	if a.FolderUID != b.FolderUID {
		return false
	}
	if a.OrgID != b.OrgID {
		return false
	}

	return true
}

func (d *Deployer) getAlert(ctx context.Context, uid string) (model.Alert, error) {
	// Prepare the request
	path := fmt.Sprintf("api/v1/provisioning/alert-rules/%s", uid)
	res, err := d.client.Get(ctx, path)
	if err != nil {
		return model.Alert{}, err
	}
	defer res.Body.Close()

	// Check the response code
	if err := shared.CheckStatusCode(res, http.StatusOK); err != nil {
		log.Printf("Can't get alert. Status: %d", res.StatusCode)
		return model.Alert{}, fmt.Errorf("error getting alert: %w", err)
	}

	alert := model.Alert{}
	if err := shared.ReadJSONResponse(res, &alert); err != nil {
		return model.Alert{}, err
	}

	return alert, nil
}

// pairsForClient returns the (folder, org) pairs deployed to the client's Grafana instance:
// those of every configuration that deploys there, including per-conversion folder_id/org_id
// overrides. A Deployer not built by LoadConfig falls back to the global pairs.
func (d *Deployer) pairsForClient(client *shared.GrafanaClient) []folderOrg {
	if d.config.endpointPairs != nil {
		return d.config.endpointPairs[client.BaseURL()]
	}
	if len(d.config.folderOrgPairs) > 0 {
		return d.config.folderOrgPairs
	}
	if d.config.folderUID != "" {
		return []folderOrg{{folderUID: d.config.folderUID, orgID: d.config.orgID}}
	}
	return nil
}

func (d *Deployer) listAlerts(ctx context.Context) ([]string, error) {
	pairs := d.pairsForClient(d.client)
	if len(pairs) == 0 {
		return nil, fmt.Errorf("folder UID is not set")
	}

	alertList := []string{}
	// Prepare the request
	res, err := d.client.Get(ctx, "api/v1/provisioning/alert-rules")
	if err != nil {
		return []string{}, err
	}
	defer res.Body.Close()

	// Check the response code
	if err := shared.CheckStatusCode(res, http.StatusOK); err != nil {
		log.Printf("Can't list alerts. Status: %d", res.StatusCode)
		return []string{}, fmt.Errorf("error listing alert: %w", err)
	}

	// Check the response body
	alertsReturned := []model.Alert{}
	if err := shared.ReadJSONResponse(res, &alertsReturned); err != nil {
		return []string{}, err
	}

	// Get the list of alerts in the folders we're deploying to
	for _, alert := range alertsReturned {
		for _, pair := range pairs {
			if alert.FolderUID == pair.folderUID && alert.OrgID == pair.orgID {
				alertList = append(alertList, alert.UID)
				break
			}
		}
	}

	log.Printf("%d alert(s) found in the folder", len(alertList))

	return alertList, nil
}

func parseAlert(content string) (model.Alert, error) {
	alert := model.Alert{}
	if err := json.Unmarshal([]byte(content), &alert); err != nil {
		return model.Alert{}, err
	}
	// Sanity check to ensure we've read an alert file
	if alert.UID == "" || alert.Title == "" || alert.FolderUID == "" {
		return model.Alert{}, fmt.Errorf("invalid alert file")
	}

	return alert, nil
}

func (d *Deployer) listAlertsInDeploymentFolder() ([]string, error) {
	folderContent, err := os.ReadDir(d.config.alertPath)
	if err != nil {
		return []string{}, fmt.Errorf("error reading deployment folder: %v", err)
	}
	alertsToAdd := []string{}
	for _, entry := range folderContent {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(d.config.alertPath, entry.Name())
		log.Printf("Found alert file: %s", filePath)
		alertsToAdd = addToAlertList(alertsToAdd, filePath, d.config.alertPath)
	}

	return alertsToAdd, nil
}

func getAlertUIDFromFilename(filename string) string {
	matches := regexAlertFilename.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}
