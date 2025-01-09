package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Structure to store the deployment config
type deploymentConfig struct {
	endpoint       string
	alertPath      string
	saToken        string
	alertsToAdd    []string
	alertsToRemove []string
	alertsToUpdate []string
}

// Structure of the deployment YAML config file
type DeploymentYAML struct {
	Endpoint  string `yaml:"endpoint"`
	AlertPath string `yaml:"alert_path"`
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

	// No matter if we had an error, we return the alerts that were still created, updated and deleted before the error
	alertsCreatedStr := strings.Join(alertsCreated, " ")
	alertsUpdatedStr := strings.Join(alertsUpdated, " ")
	alertsDeletedStr := strings.Join(alertsDeleted, " ")

	// Write action outputs
	githubOutput := os.Getenv("GITHUB_OUTPUT")
	if githubOutput != "" {
		f, err := os.OpenFile(githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Can't write output: %v", err)
		}
		defer f.Close()
		output := fmt.Sprintf("alerts_created=%s\nalerts_updated=%s\nalerts_deleted=%s\n",
			alertsCreatedStr, alertsUpdatedStr, alertsDeletedStr)
		if _, err := f.WriteString(output); err != nil {
			log.Printf("Can't write output: %v", err)
		}
	}

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

	// Process alert CREATIONS
	for _, alertFile := range d.config.alertsToAdd {
		content, err := readFile(alertFile)
		if err != nil {
			log.Printf("Cannot read file %s: %v", alertFile, err)
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
			log.Printf("Cannot read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, err := d.updateAlert(content)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		alertsUpdated = append(alertsUpdated, uid)
	}
	// Process alert DELETIONS
	for _, alertFile := range d.config.alertsToRemove {
		content, err := readFile(alertFile)
		if err != nil {
			log.Printf("Cannot read file %s: %v", alertFile, err)
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		uid, err := d.deleteAlert(content)
		if err != nil {
			return alertsCreated, alertsUpdated, alertsDeleted, err
		}
		alertsDeleted = append(alertsDeleted, uid)
	}

	return alertsCreated, alertsUpdated, alertsDeleted, nil
}

func (d *Deployer) LoadConfig() error {
	// Load the deployment config file
	configFile := os.Getenv("DEPLOYER_CONFIG_FILE")
	if configFile == "" {
		return fmt.Errorf("Deployer config file is not set or empty")
	}

	// Read the YAML config file
	configFileContent, err := readFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}
	configYAML := DeploymentYAML{}
	err = yaml.Unmarshal([]byte(configFileContent), &configYAML)
	if err != nil {
		return fmt.Errorf("error unmarshalling config file: %v", err)
	}
	d.config = deploymentConfig{
		endpoint:  configYAML.Endpoint,
		alertPath: filepath.Clean(configYAML.AlertPath),
	}

	// Makes sure the endpoint ends with a slash
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
	// Note: we don't take the renamed files into account as they don't modify the alerts per se

	d.config.alertsToAdd = alertsToAdd
	d.config.alertsToRemove = alertsToDelete
	d.config.alertsToUpdate = alertsToUpdate

	return nil
}

func addToAlertList(alertList []string, file string, prefix string) []string {
	// We first check that the modified files are in the expected folder
	// That is, the folder which contains the alert files
	// Otherwise, we ignore this file as they are unrelated to the deployment
	pattern := prefix + string(filepath.Separator) + "*"
	matched, err := filepath.Match(pattern, file)
	if matched && err == nil {
		alertList = append(alertList, file)
	}
	return alertList
}

func (d *Deployer) createAlert(content string) (string, error) {
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
		Timeout: 5 * time.Second,
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
		log.Printf("Cannot create alert. Status: %d, Message: %s", res.StatusCode, resp.Message)
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
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != 200 {
		log.Printf("Cannot update alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error updating alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) updated", alert.Uid, alert.Title)

	return alert.Uid, nil
}

func (d *Deployer) deleteAlert(content string) (string, error) {
	// Retrieve some alert information
	alert, err := parseAlert(content)
	if err != nil {
		return "", err
	}

	// Prepare the request
	url := fmt.Sprintf("%sapi/v1/provisioning/alert-rules/%s", d.config.endpoint, alert.Uid)

	req, err := http.NewRequest("DELETE", url, bytes.NewBuffer([]byte{}))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.config.saToken))

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != 204 {
		log.Printf("Cannot delete alert. Status: %d", res.StatusCode)
		return "", fmt.Errorf("error delete alert: returned status %s", res.Status)
	}

	log.Printf("Alert %s (%s) deleted", alert.Uid, alert.Title)

	return alert.Uid, nil
}

func parseAlert(content string) (Alert, error) {
	alert := Alert{}
	if err := json.Unmarshal([]byte(content), &alert); err != nil {
		return Alert{}, err
	}
	return alert, nil
}

func readFile(filePath string) (string, error) {
	// Check if the file path is local
	// This is to check we're only reading files from the workspace
	if !filepath.IsLocal(filePath) {
		return "", fmt.Errorf("invalid file path: %s", filePath)
	}

	configFileContent, err := os.ReadFile(filePath)

	return string(configFileContent), err
}
