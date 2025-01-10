package main

import (
	"bytes"
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
var requestTimeOut = 5 * time.Second

// Structure to store the deployment config
type deploymentConfig struct {
	endpoint       string
	alertPath      string
	saToken        string
	alertsToAdd    []string
	alertsToRemove []string
	alertsToUpdate []string
}

// Structures to unmarshal the YAML config file
type FoldersConfig struct {
	DeploymentPath string `yaml:"deployment_path"`
}
type DeploymentConfig struct {
	GrafanaInstance string `yaml:"grafana_instance"`
}
type Configuration struct {
	Folders        FoldersConfig    `yaml:"folders"`
	DeployerConfig DeploymentConfig `yaml:"deployment"`
}

type Deployer struct {
	config deploymentConfig
}

// Non exhaustive list of alert fields
type Alert struct {
	Uid   string `json:"uid"`
	Title string `json:"title"`
}

func main() {
	// Load the deployment config
	deployer := NewDeployer()
	if err := deployer.LoadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Deploy alerts
	alertsCreated, alertsUpdated, alertsDeleted, errDeploy := deployer.Deploy()

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
	return &Deployer{}
}

func (d *Deployer) Deploy() ([]string, []string, []string, error) {
	// Lists to store the alerts that were created, updated and deleted at any point during the deployment
	alertsCreated := []string{}
	alertsUpdated := []string{}
	alertsDeleted := []string{}

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
		uid, err := d.deleteAlert(alertUid)
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
		uid, err := d.createAlert(content)
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
		uid, err := d.updateAlert(content)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		alertsUpdated = append(alertsUpdated, uid)
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

func (d *Deployer) LoadConfig() error {
	// Load the sigma rule deployer config file
	configFile := os.Getenv("CONFIG_FILE")
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
		endpoint:  configYAML.DeployerConfig.GrafanaInstance,
		alertPath: filepath.Clean(configYAML.Folders.DeploymentPath),
	}

	// Makes sure the endpoint URL ends with a slash
	if !strings.HasSuffix(d.config.endpoint, "/") {
		d.config.endpoint += "/"
	}

	// Get the rest of the config from the environment variables
	d.config.saToken = os.Getenv("DEPLOYER_GRAFANA_SA_TOKEN")
	if d.config.saToken == "" {
		return fmt.Errorf("Grafana SA token is not set or empty")
	}

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

func (d *Deployer) createAlert(content string) (string, error) {
	// For now, we are only interested in the response message, which provides context in case of errors
	type Response struct {
		Message string `json:"message"`
	}

	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", err
	}

	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules", d.config.endpoint)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(content)))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	client := &http.Client{
		Timeout: requestTimeOut,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	resp := Response{}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return "", err
	}

	if res.StatusCode != 201 {
		log.Printf("Can't create alert. Status: %d, Message: %s", res.StatusCode, resp.Message)
		return "", fmt.Errorf("error creating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) created", alert.Uid, alert.Title)

	return alert.Uid, nil
}

func (d *Deployer) updateAlert(content string) (string, error) {
	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", err
	}

	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules/%s", d.config.endpoint, alert.Uid)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer([]byte(content)))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	client := &http.Client{
		Timeout: requestTimeOut,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != 200 {
		log.Printf("Can't update alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error updating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) updated", alert.Uid, alert.Title)

	return alert.Uid, nil
}

func (d *Deployer) deleteAlert(uid string) (string, error) {
	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules/%s", d.config.endpoint, uid)

	req, err := http.NewRequest("DELETE", url, bytes.NewBuffer([]byte{}))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	client := &http.Client{
		Timeout: requestTimeOut,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != 204 {
		log.Printf("Can't delete alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error delete alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s deleted", uid)

	return uid, nil
}

func parseAlert(content string) (Alert, error) {
	alert := Alert{}
	if err := json.Unmarshal([]byte(content), &alert); err != nil {
		return Alert{}, err
	}
	// Sanity check to ensure we've read an alert file
	if alert.Uid == "" || alert.Title == "" {
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

	fileContent, err := os.ReadFile(filePath)

	return string(fileContent), err
}

func getAlertUidFromFilename(filename string) string {
	matches := regexAlertFilename.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}
