package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"path/filepath"

	"gopkg.in/yaml.v3"
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
	folderUid       string
	orgId           int64
	alertsToAdd     []string
	alertsToRemove  []string
	alertsToUpdate  []string
	groupsIntervals map[string]int64
	timeout         time.Duration
}

// Structures to unmarshal the YAML config file
type FoldersConfig struct {
	DeploymentPath string `yaml:"deployment_path"`
}
type DeploymentConfig struct {
	GrafanaInstance string `yaml:"grafana_instance"`
	Timeout         string `yaml:"timeout"`
}
type ConversionConfig struct {
	RuleGroup  string `yaml:"rule_group"`
	TimeWindow string `yaml:"time_window"`
}
type Configuration struct {
	Folders          FoldersConfig      `yaml:"folders"`
	DefaultConfig    ConversionConfig   `yaml:"conversion_defaults"`
	ConversionConfig []ConversionConfig `yaml:"conversion"`
	DeployerConfig   DeploymentConfig   `yaml:"deployment"`
	IntegratorConfig IntegrationConfig  `yaml:"integration"`
}
type IntegrationConfig struct {
	FolderID     string `yaml:"folder_id"`
	OrgID        int64  `yaml:"org_id"`
	TestQueries  bool   `yaml:"test_queries"`
	From         string `yaml:"from"`
	To           string `yaml:"to"`
	ShowLogLines bool   `yaml:"show_log_lines"`
}

type Deployer struct {
	config         deploymentConfig
	client         *http.Client
	groupsToUpdate map[string]bool
}

// Non exhaustive list of alert fields
type Alert struct {
	Uid       string `json:"uid"`
	Title     string `json:"title"`
	FolderUid string `json:"folderUID"`
	RuleGroup string `json:"ruleGroup"`
	OrgID     int64  `json:"orgID"`
}

type AlertRuleGroup struct {
	FolderUID string `json:"folderUID"`
	Interval  int64  `json:"interval"`
	Rules     any    `json:"rules"`
	Title     string `json:"title"`
}

func main() {
	ctx := context.Background()

	// Load the deployment config
	deployer := NewDeployer()

	if err := deployer.LoadConfig(ctx); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	deployer.client = &http.Client{
		Timeout: deployer.config.timeout,
	}

	var err error
	if deployer.config.freshDeploy {
		err = deployer.configFreshDeployment(ctx)
	} else {
		err = deployer.configNormalMode()
	}
	if err != nil {
		fmt.Printf("Error configuring deployment: %v\n", err)
		os.Exit(1)
	}

	// Deploy alerts
	alertsCreated, alertsUpdated, alertsDeleted, errDeploy := deployer.Deploy(ctx)

	// Write action outputs
	if err := deployer.writeOutput(alertsCreated, alertsUpdated, alertsDeleted); err != nil {
		fmt.Printf("Error writing output: %v\n", err)
		os.Exit(1)
	}

	// We only check the deployment error AFTER writing the output so that
	// we still report the alerts that were created, updated and deleted before the error
	if errDeploy != nil {
		fmt.Printf("Error deploying: %v\n", errDeploy)
		os.Exit(1)
	}
}

func NewDeployer() *Deployer {
	return &Deployer{
		groupsToUpdate: map[string]bool{},
	}
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
		alertUid := getAlertUidFromFilename(filepath.Base(alertFile))
		if alertUid == "" {
			err := fmt.Errorf("invalid alert filename: %s", alertFile)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, err := d.deleteAlert(ctx, alertUid)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		alertsDeleted = append(alertsDeleted, uid)
	}
	// Process alert CREATIONS
	for _, alertFile := range d.config.alertsToAdd {
		content, err := readFile(alertFile)
		if err != nil {
			log.Printf("Can't read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, err := d.createAlert(ctx, content)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		alertsCreated = append(alertsCreated, uid)
	}
	// Process alert UPDATES
	for _, alertFile := range d.config.alertsToUpdate {
		content, err := readFile(alertFile)
		if err != nil {
			log.Printf("Can't read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, err := d.updateAlert(ctx, content)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		alertsUpdated = append(alertsUpdated, uid)
	}

	// Process alert group interval updates
	if len(d.groupsToUpdate) > 0 {
		for group := range d.groupsToUpdate {
			if err := d.updateAlertGroupInterval(ctx, d.config.folderUid, group, d.config.groupsIntervals[group]); err != nil {
				return alertsCreated, alertsUpdated, alertsDeleted, err
			}
		}
	}

	return alertsCreated, alertsUpdated, alertsDeleted, nil
}

func (d *Deployer) writeOutput(alertsCreated []string, alertsUpdated []string, alertsDeleted []string) error {
	alertsCreatedStr := strings.Join(alertsCreated, " ")
	alertsUpdatedStr := strings.Join(alertsUpdated, " ")
	alertsDeletedStr := strings.Join(alertsDeleted, " ")

	githubOutput := os.Getenv("GITHUB_OUTPUT")
	if githubOutput == "" {
		return fmt.Errorf("GITHUB_OUTPUT is not set or empty")
	}
	f, err := os.OpenFile(githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	output := fmt.Sprintf("alerts_created=%s\nalerts_updated=%s\nalerts_deleted=%s\n",
		alertsCreatedStr, alertsUpdatedStr, alertsDeletedStr)
	if _, err := f.WriteString(output); err != nil {
		return err
	}

	return nil
}

func (d *Deployer) LoadConfig(ctx context.Context) error {
	// Load the sigma rule deployer config file
	configFile := os.Getenv("CONFIG_PATH")
	if configFile == "" {
		return fmt.Errorf("Deployer config file is not set or empty")
	}
	configFile = filepath.Clean(configFile)

	// Read the YAML config file
	configFileContent, err := readFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}
	configYAML := Configuration{}
	err = yaml.Unmarshal([]byte(configFileContent), &configYAML)
	if err != nil {
		return fmt.Errorf("error unmarshalling config file: %v", err)
	}
	d.config = deploymentConfig{
		endpoint:        configYAML.DeployerConfig.GrafanaInstance,
		alertPath:       filepath.Clean(configYAML.Folders.DeploymentPath),
		orgId:           configYAML.IntegratorConfig.OrgID,
		folderUid:       configYAML.IntegratorConfig.FolderID,
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
	if configYAML.DefaultConfig.TimeWindow != "" {
		defaultInterval = configYAML.DefaultConfig.TimeWindow
	}
	for _, config := range configYAML.ConversionConfig {
		interval := defaultInterval
		if config.TimeWindow != "" {
			interval = config.TimeWindow
		}
		intervalDuration, err := time.ParseDuration(interval)
		log.Printf("Interval duration from %s: %s", interval, intervalDuration)
		if err != nil || int64(intervalDuration.Seconds()) == 0 {
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

func (d *Deployer) configNormalMode() error {
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

func (d *Deployer) configFreshDeployment(ctx context.Context) error {
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

func (d *Deployer) createAlert(ctx context.Context, content string) (string, error) {
	// For now, we are only interested in the response message, which provides context in case of errors
	type Response struct {
		Message string `json:"message"`
	}

	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", err
	}
	d.groupsToUpdate[alert.RuleGroup] = true

	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules", d.config.endpoint)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer([]byte(content)))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	res, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	resp := Response{}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return "", err
	}

	if res.StatusCode != http.StatusCreated {
		log.Printf("Can't create alert. Status: %d, Message: %s", res.StatusCode, resp.Message)
		return "", fmt.Errorf("error creating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) created", alert.Uid, alert.Title)

	return alert.Uid, nil
}

func (d *Deployer) updateAlert(ctx context.Context, content string) (string, error) {
	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", err
	}
	d.groupsToUpdate[alert.RuleGroup] = true

	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules/%s", d.config.endpoint, alert.Uid)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer([]byte(content)))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	res, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != http.StatusOK {
		log.Printf("Can't update alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error updating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) updated", alert.Uid, alert.Title)

	return alert.Uid, nil
}

func (d *Deployer) updateAlertGroupInterval(ctx context.Context, folderUid string, group string, interval int64) error {
	log.Printf("Checking alert group interval for %s/%s to %d", folderUid, group, interval)
	url := fmt.Sprintf("%sapi/v1/provisioning/folder/%s/rule-groups/%s", d.config.endpoint, folderUid, group)

	// Get the current alert group content
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	res, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != http.StatusOK {
		log.Printf("Can't find alert group. Status: %d", res.StatusCode)
		return fmt.Errorf("error finding alert group %s/%s: returned status %s", folderUid, group, res.Status)
	}
	resp := AlertRuleGroup{}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return err
	}

	if resp.Interval != interval {
		log.Printf("Updating alert group interval for %s/%s to %d", folderUid, group, interval)
		resp.Interval = interval
		content, err := json.Marshal(resp)
		if err != nil {
			log.Printf("Can't update alert group interval. Error: %s", err.Error())
			return fmt.Errorf("error updating alert group interval %s/%s: returned error %s", folderUid, group, err.Error())
		}

		// Note the implicit race condition - if a rule is added to the group between these two requests,
		// they will be overwritten by this request. There's nothing we can do about this; alerting
		// would need to update their API to allow the interval to be updated independent of the alert rules
		req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer([]byte(content)))
		if err != nil {
			return err
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

		res, err := d.client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			log.Printf("Can't update alert group interval. Status: %d", res.StatusCode)
			return fmt.Errorf("error updating alert group interval %s/%s: returned status %s", folderUid, group, res.Status)
		}
	}

	return nil
}

func (d *Deployer) deleteAlert(ctx context.Context, uid string) (string, error) {
	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules/%s", d.config.endpoint, uid)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, bytes.NewBuffer([]byte{}))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	res, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != http.StatusNoContent {
		log.Printf("Can't delete alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error deleting alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s deleted", uid)

	return uid, nil
}

func (d *Deployer) listAlerts(ctx context.Context) ([]string, error) {
	if d.config.folderUid == "" {
		return nil, fmt.Errorf("folder UID is not set")
	}

	alertList := []string{}
	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules", d.config.endpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer([]byte{}))
	if err != nil {
		return []string{}, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	res, err := d.client.Do(req)
	if err != nil {
		return []string{}, err
	}
	defer res.Body.Close()

	// Check the response code
	if res.StatusCode != http.StatusOK {
		log.Printf("Can't list alerts. Status: %d", res.StatusCode)
		return []string{}, fmt.Errorf("error listing alert: returned status %s", res.Status)
	}

	// Check the response body
	alertsReturned := []Alert{}
	if err := json.NewDecoder(res.Body).Decode(&alertsReturned); err != nil {
		return []string{}, err
	}

	// Get the list of alerts in the folder we're deploying to
	for _, alert := range alertsReturned {
		if alert.FolderUid == d.config.folderUid && alert.OrgID == d.config.orgId {
			alertList = append(alertList, alert.Uid)
		}
	}

	log.Printf("%d alert(s) found in the folder", len(alertList))

	return alertList, nil
}

func parseAlert(content string) (Alert, error) {
	alert := Alert{}
	if err := json.Unmarshal([]byte(content), &alert); err != nil {
		return Alert{}, err
	}
	// Sanity check to ensure we've read an alert file
	if alert.Uid == "" || alert.Title == "" || alert.FolderUid == "" {
		return Alert{}, fmt.Errorf("invalid alert file")
	}

	return alert, nil
}

func readFile(filePath string) (string, error) {
	// Check if the file path is local
	// This is to check we're only reading files from the workspace
	if !filepath.IsLocal(filePath) {
		return "", fmt.Errorf("invalid file path: %s", filePath)
	}

	// TODO: When Go 1.24 releases, use Os.Root
	// https://tip.golang.org/doc/go1.24#directory-limited-filesystem-access
	// https://pkg.go.dev/os@master#Root
	fileContent, err := os.ReadFile(filePath)

	return string(fileContent), err
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

func getAlertUidFromFilename(filename string) string {
	matches := regexAlertFilename.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}
