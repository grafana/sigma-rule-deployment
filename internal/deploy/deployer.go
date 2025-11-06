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
	"strings"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
)

// Regex to parse the alert UID from the filename
var regexAlertFilename = regexp.MustCompile(`alert_rule_(?:.*)_([^\.]+)\.json`)

// Timeout for the HTTP requests
var defaultRequestTimeout = 10 * time.Second

// Structure to store the deployment config
type deploymentConfig struct {
	endpoint        string
	alertPath       string
	saToken         string
	freshDeploy     bool
	folderUID       string
	orgID           int64
	alertsToAdd     []string
	alertsToRemove  []string
	alertsToUpdate  []string
	groupsIntervals map[string]int64
	timeout         time.Duration
}

// Structures to unmarshal the YAML config file

type Deployer struct {
	config         deploymentConfig
	client         *shared.GrafanaClient
	groupsToUpdate map[string]bool
}

func NewDeployer() *Deployer {
	return &Deployer{
		groupsToUpdate: map[string]bool{},
	}
}

func (d *Deployer) SetClient() {
	d.client = shared.NewGrafanaClient(
		d.config.endpoint,
		d.config.saToken,
		"sigma-rule-deployment/deployer",
		d.config.timeout,
	)
}

func (d *Deployer) IsFreshDeploy() bool {
	return d.config.freshDeploy
}

func (d *Deployer) Deploy(ctx context.Context) ([]string, []string, []string, error) {
	// Lists to store the alerts that were created, updated and deleted at any point during the deployment
	alertsCreated := make([]string, len(d.config.alertsToAdd))
	alertsUpdated := make([]string, len(d.config.alertsToUpdate))
	alertsDeleted := make([]string, len(d.config.alertsToRemove))

	log.Printf("Preparing to deploy %d alerts, update %d alerts and delete %d alerts",
		len(d.config.alertsToAdd), len(d.config.alertsToUpdate), len(d.config.alertsToRemove))

	// Process alert DELETIONS
	// It is important to do this first for the case where an alert
	// is recreated in a different file (with a different UID), to avoid conflicts on the alert title
	// By deleting the old one first, we can then create the new one without issues
	for _, alertFile := range d.config.alertsToRemove {
		alertUID := getAlertUIDFromFilename(filepath.Base(alertFile))
		if alertUID == "" {
			err := fmt.Errorf("invalid alert filename: %s", alertFile)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, err := d.deleteAlert(ctx, alertUID)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		// UID could be empty if the alert was not found
		// In this case, we don't want to add it to the list of deleted alerts
		if uid != "" {
			alertsDeleted = append(alertsDeleted, uid)
		}
	}
	// Process alert CREATIONS
	for _, alertFile := range d.config.alertsToAdd {
		content, err := shared.ReadLocalFile(alertFile)
		if err != nil {
			log.Printf("Can't read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, updated, err := d.createAlert(ctx, content, true)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		if updated {
			// If the alert was updated, we need to add it to the list of updated alerts
			alertsUpdated = append(alertsUpdated, uid)
		} else {
			// If the alert was created, we need to add it to the list of created alerts
			alertsCreated = append(alertsCreated, uid)
		}
	}
	// Process alert UPDATES
	for _, alertFile := range d.config.alertsToUpdate {
		content, err := shared.ReadLocalFile(alertFile)
		if err != nil {
			log.Printf("Can't read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, created, err := d.updateAlert(ctx, content, true)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		// Sometimes the alert to update doesn't exist anymore (e.g. it was deleted manually)
		// In this case, we re-create it instead of updating it
		// So we take this into account for the reporting
		if created {
			// If the alert was created, we need to add it to the list of created alerts
			alertsCreated = append(alertsCreated, uid)
		} else {
			// If the alert was updated, we need to add it to the list of updated alerts
			alertsUpdated = append(alertsUpdated, uid)
		}
	}

	// Process alert group interval updates
	if len(d.groupsToUpdate) > 0 {
		for group := range d.groupsToUpdate {
			if err := d.updateAlertGroupInterval(ctx, d.config.folderUID, group, d.config.groupsIntervals[group]); err != nil {
				return alertsCreated, alertsUpdated, alertsDeleted, err
			}
		}
	}

	return alertsCreated, alertsUpdated, alertsDeleted, nil
}

func (d *Deployer) WriteOutput(alertsCreated []string, alertsUpdated []string, alertsDeleted []string) error {
	alertsCreatedStr := strings.Join(alertsCreated, " ")
	alertsUpdatedStr := strings.Join(alertsUpdated, " ")
	alertsDeletedStr := strings.Join(alertsDeleted, " ")

	if err := shared.SetOutput("alerts_created", alertsCreatedStr); err != nil {
		return err
	}
	if err := shared.SetOutput("alerts_updated", alertsUpdatedStr); err != nil {
		return err
	}
	if err := shared.SetOutput("alerts_deleted", alertsDeletedStr); err != nil {
		return err
	}
	return nil
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
		endpoint:        configYAML.DeployerConfig.GrafanaInstance,
		alertPath:       filepath.Clean(configYAML.Folders.DeploymentPath),
		orgID:           configYAML.IntegratorConfig.OrgID,
		folderUID:       configYAML.IntegratorConfig.FolderID,
		groupsIntervals: make(map[string]int64),
		timeout:         defaultRequestTimeout,
	}

	// Parse timeout if provided
	if configYAML.DeployerConfig.Timeout != "" {
		parsedTimeout, err := time.ParseDuration(configYAML.DeployerConfig.Timeout)
		if err != nil {
			log.Printf("Warning: Invalid timeout format in config, using default: %v\n", err)
		} else {
			d.config.timeout = parsedTimeout
		}
	}

	// Makes sure the endpoint URL ends with a slash
	if !strings.HasSuffix(d.config.endpoint, "/") {
		d.config.endpoint += "/"
	}

	// Get the rest of the config from the environment variables
	d.config.saToken = os.Getenv("DEPLOYER_GRAFANA_SA_TOKEN")
	if d.config.saToken == "" {
		return fmt.Errorf("the Grafana SA token is not set or empty")
	}

	// Extract the groups intervals from the conversion config
	defaultInterval := "5m"
	if configYAML.ConversionDefaults.TimeWindow != "" {
		defaultInterval = configYAML.ConversionDefaults.TimeWindow
	}
	for _, config := range configYAML.Conversions {
		interval := defaultInterval
		if config.TimeWindow != "" {
			interval = config.TimeWindow
		}
		intervalDuration, err := time.ParseDuration(interval)
		log.Printf("Interval duration from %s: %d", interval, int64(intervalDuration.Seconds()))
		if err != nil || int64(intervalDuration.Seconds()) <= 0 {
			return fmt.Errorf("error parsing time window %s: %v", interval, err)
		}
		if _, ok := d.config.groupsIntervals[config.RuleGroup]; !ok {
			d.config.groupsIntervals[config.RuleGroup] = int64(intervalDuration.Seconds())
			log.Printf("Setting interval for rule group %s to %d", config.RuleGroup, d.config.groupsIntervals[config.RuleGroup])
		} else if d.config.groupsIntervals[config.RuleGroup] != int64(intervalDuration.Seconds()) {
			return fmt.Errorf("time window for rule group %s is different between conversion configs", config.RuleGroup)
		}
	}

	// Retrieve the fresh deploy flag
	freshDeploy := strings.ToLower(os.Getenv("DEPLOYER_FRESH_DEPLOY")) == "true"
	d.config.freshDeploy = freshDeploy

	return nil
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
	// List the current alerts in the Grafana folder so that they can be deleted first
	alertsToRemove, err := d.listAlerts(ctx)
	if err != nil {
		return fmt.Errorf("error listing alerts: %v", err)
	}
	for i, alert := range alertsToRemove {
		// We give a fake alert filename so that we can delete it later
		alertsToRemove[i] = d.fakeAlertFilename(alert)
	}
	d.config.alertsToAdd = alertsToAdd
	d.config.alertsToRemove = alertsToRemove
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

func (d *Deployer) createAlert(ctx context.Context, content string, updateIfExists bool) (string, bool, error) {
	//  Return values:
	// 1. UID of the alert
	// 2. Whether the alert was updated instead of create. If updateIfExists is false, this will always be false.
	// 3. Error if any

	// For now, we are only interested in the response message, which provides context in case of errors
	type Response struct {
		Message string `json:"message"`
	}

	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", false, err
	}
	d.groupsToUpdate[alert.RuleGroup] = true

	// Prepare the request
	res, err := d.client.PostRaw(ctx, "api/v1/provisioning/alert-rules", []byte(content))
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()

	// Check the response
	resp := Response{}
	if err := shared.ReadJSONResponse(res, &resp); err != nil {
		return "", false, err
	}

	switch res.StatusCode {
	case http.StatusCreated:
		// Alert created successfully
		log.Printf("Alert %s (%s) created", alert.UID, alert.Title)
		return alert.UID, false, nil
	case http.StatusConflict:
		// Another alert with the same UID exists
		// If the alert already exists and we don't want to update it, we return an error
		if !updateIfExists {
			log.Printf("Alert %s (%s) conflicts with another alert", alert.UID, alert.Title)
			return "", false, fmt.Errorf("error creating alert: returned status %s", res.Status)
		}
		// Otherwise, we need to check if it's a re-creation (in which case we proceed to update it instead)
		// or an actual conflict
		uid, err := d.tryToUpdateConflictingAlert(ctx, alert, content)
		if err != nil {
			return "", false, err
		}

		return uid, true, nil
	default:
		log.Printf("Can't create alert %s (%s). Status: %d, Message: %s", alert.UID, alert.Title, res.StatusCode, resp.Message)
		return "", false, fmt.Errorf("error creating alert: returned status %s", res.Status)
	}
}

func (d *Deployer) tryToUpdateConflictingAlert(ctx context.Context, alert model.Alert, content string) (string, error) {
	// Retrieve the existing alert it's conflicting with
	existingAlert, err := d.getAlert(ctx, alert.UID)
	if err != nil {
		log.Printf("Can't get alert %s. Error: %v", alert.UID, err)
		return "", fmt.Errorf("error getting alert: %v", err)
	}
	// Check if the conflicting alerts have the same parameters
	// Otherwise, it's an actual conflict
	if !d.checkAlertsMatch(existingAlert, alert) {
		// The alert already exists, but with different parameters
		log.Printf("Alert %s (%s) is conflicting with another alert having the same UID", alert.UID, alert.Title)
		return "", fmt.Errorf("error creating alert: %v", err)
	}
	// The alert already exists, but with the same parameters
	// In this case, we can proceed to update it
	log.Printf("Alert %s (%s) already exists, updating it instead", alert.UID, alert.Title)
	uid, _, err := d.updateAlert(ctx, content, false)
	if err != nil {
		log.Printf("Can't update alert %s: %v", alert.UID, err)
		return "", fmt.Errorf("error updating alert: %v", err)
	}
	return uid, nil
}

func (d *Deployer) updateAlert(ctx context.Context, content string, createIfNotFound bool) (string, bool, error) {
	//  Return values:
	// 1. UID of the alert
	// 2. Whether the alert had to be (re-)created. If createIfNotFound is false, this will always be false.
	// 3. Error if any

	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", false, err
	}
	d.groupsToUpdate[alert.RuleGroup] = true

	// Prepare the request
	path := fmt.Sprintf("api/v1/provisioning/alert-rules/%s", alert.UID)
	res, err := d.client.PutRaw(ctx, path, []byte(content))
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode == http.StatusNotFound && createIfNotFound {
		// If an alert has been manually deleted in Grafana, and the deployer isn't aware of it, then next time it's modified
		// it will try to update it. This will fail with a 404 error, so we need to create it instead
		log.Printf("Alert %s not found for update, (re-)creating it instead", alert.UID)
		uid, _, err := d.createAlert(ctx, content, false)
		if err != nil {
			log.Printf("Can't create alert: %v", err)
			return "", true, err
		}
		return uid, true, nil
	} else if res.StatusCode != http.StatusOK {
		log.Printf("Can't update alert. Status: %d", res.StatusCode)
		return "", false, fmt.Errorf("error updating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) updated", alert.UID, alert.Title)

	return alert.UID, false, nil
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

func (d *Deployer) listAlerts(ctx context.Context) ([]string, error) {
	if d.config.folderUID == "" {
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

	// Get the list of alerts in the folder we're deploying to
	for _, alert := range alertsReturned {
		if alert.FolderUID == d.config.folderUID && alert.OrgID == d.config.orgID {
			alertList = append(alertList, alert.UID)
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

func (d *Deployer) fakeAlertFilename(uid string) string {
	filename := fmt.Sprintf("alert_rule_conversion_%s.json", uid)
	return filepath.Join(d.config.alertPath, filename)
}

func getAlertUIDFromFilename(filename string) string {
	matches := regexAlertFilename.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}
