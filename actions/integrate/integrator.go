package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"github.com/spaolacci/murmur3"
	"gopkg.in/yaml.v3"
)

type Integrator struct {
	config Configuration

	addedFiles   []string
	removedFiles []string
}

func main() {
	integrator := NewIntegrator()
	if err := integrator.LoadConfig(); err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}
	err := integrator.Run()
	if err != nil {
		fmt.Printf("Error running integrator: %v\n", err)
		os.Exit(1)
	}
}

func NewIntegrator() *Integrator {
	return &Integrator{}
}

func (i *Integrator) LoadConfig() error {
	// Load the deployment config file
	configFile := os.Getenv("INTEGRATOR_CONFIG_PATH")
	if configFile == "" {
		return fmt.Errorf("Integrator config file is not set or empty")
	}

	// Read the YAML config file
	cfg, err := ReadLocalFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}
	config := Configuration{}
	err = yaml.Unmarshal([]byte(cfg), &config)
	if err != nil {
		return fmt.Errorf("error unmarshalling config file: %v", err)
	}
	i.config = config
	if _, err = os.Stat(i.config.Folders.DeploymentPath); err != nil {
		err = os.MkdirAll(i.config.Folders.DeploymentPath, 0700)
		if err != nil {
			return fmt.Errorf("error creating deployment directory: %v", err)
		}
	}

	addedFiles := strings.Split(os.Getenv("ADDED_FILES"), " ")
	deletedFiles := strings.Split(os.Getenv("DELETED_FILES"), " ")
	modifiedFiles := strings.Split(os.Getenv("MODIFIED_FILES"), " ")
	// copiedFiles := strings.Split(os.Getenv("COPIED_FILES"), " ") // TODO

	newUpdatedFiles := make([]string, 0, len(addedFiles)+len(modifiedFiles))
	removedFiles := make([]string, 0, len(deletedFiles))

	for _, path := range addedFiles {
		// Ensure paths appear within specified conversion path
		relpath, err := filepath.Rel(i.config.Folders.ConversionPath, path)
		if err != nil {
			return fmt.Errorf("error checking file path %s: %v", path, err)
		}
		if relpath == filepath.Base(path) {
			newUpdatedFiles = append(newUpdatedFiles, path)
		}
	}
	for _, path := range modifiedFiles {
		relpath, err := filepath.Rel(i.config.Folders.ConversionPath, path)
		if err != nil {
			return fmt.Errorf("error checking file path %s: %v", path, err)
		}
		if relpath == filepath.Base(path) {
			newUpdatedFiles = append(newUpdatedFiles, path)
		}
	}
	for _, path := range deletedFiles {
		relpath, err := filepath.Rel(i.config.Folders.ConversionPath, path)
		if err != nil {
			return fmt.Errorf("error checking file path %s: %v", path, err)
		}
		if relpath == filepath.Base(path) {
			removedFiles = append(removedFiles, path)
		}
	}

	i.addedFiles = newUpdatedFiles
	i.removedFiles = removedFiles

	return nil
}

func (i *Integrator) Run() error {
	for _, inputFile := range i.addedFiles {
		raw_queries, err := ReadLocalFile(inputFile)
		if err != nil {
			return err
		}

		config := ConversionConfig{}
		for _, conf := range i.config.Conversions {
			if conf.Name+".txt" == filepath.Base(inputFile) {
				config = conf
				break
			}
		}
		if config.Name == "" {
			return fmt.Errorf("no conversion configuration found for file: %s", inputFile)
		}

		queries := strings.Split(string(raw_queries), "\n\n") // Separator taken from the Sigma source code
		if len(queries) == 0 {
			return fmt.Errorf("no queries found in file: %s", inputFile)
		}

		for _, query := range queries {
			id := int64(murmur3.Sum32([]byte(query)))
			hash := fmt.Sprintf("%x", id) // FIXME: extract from Sigma rule file
			file := fmt.Sprintf("%s%salert_rule_%s_%s.json", i.config.Folders.DeploymentPath, string(filepath.Separator), config.Name, hash)
			rule := &definitions.ProvisionedAlertRule{ID: id, UID: hash}
			err := readRuleFromFile(rule, file)
			if err != nil {
				return err
			}
			err = i.ConvertToAlert(rule, query, config)
			if err != nil {
				return err
			}
			err = writeRuleToFile(rule, file)
			if err != nil {
				return err
			}
		}
	}

	for _, deletedFile := range i.removedFiles {
		deploymentGlob := fmt.Sprintf("alert_rule_%s_*.json", strings.TrimSuffix(filepath.Base(deletedFile), ".txt"))
		deploymentFiles, err := fs.Glob(os.DirFS(i.config.Folders.DeploymentPath), deploymentGlob)
		if err != nil {
			return fmt.Errorf("error when searching for deployment files for %s: %v", deletedFile, err)
		}
		for _, file := range deploymentFiles {
			err = os.Remove(i.config.Folders.DeploymentPath + string(filepath.Separator) + file)
			if err != nil {
				return fmt.Errorf("error when deleting deployment file %s: %v", file, err)
			}
		}
	}

	rulesIntegrated := strings.Join(i.addedFiles, " ")
	if len(i.addedFiles) > 0 {
		rulesIntegrated += " "
	}
	rulesIntegrated += strings.Join(i.removedFiles, " ")
	SetOutput("rules_integrated", rulesIntegrated)

	return nil
}

func (i *Integrator) ConvertToAlert(rule *definitions.ProvisionedAlertRule, query string, config ConversionConfig) error {
	datasource := getC(config.DataSource, i.config.ConversionDefaults.DataSource, "nil")
	timewindow := getC(config.TimeWindow, i.config.ConversionDefaults.TimeWindow, "1m")
	duration, err := time.ParseDuration(timewindow)
	if err != nil {
		return fmt.Errorf("error parsing time window: %v", err)
	}
	timerange := definitions.RelativeTimeRange{From: definitions.Duration(duration), To: definitions.Duration(time.Duration(0))}

	// alerting rule metadata
	rule.OrgID = i.config.IntegratorConfig.OrgID
	rule.FolderUID = i.config.IntegratorConfig.FolderID
	rule.RuleGroup = getC(config.RuleGroup, i.config.ConversionDefaults.RuleGroup, "Default")
	rule.NoDataState = definitions.OK
	rule.ExecErrState = definitions.OkErrState
	rule.Updated = time.Now()
	rule.Title = fmt.Sprintf("Alert Rule %s", rule.UID)

	// alerting rule query
	// disabled for time being due to dependency conflict between loki and alerting :confused:
	// if getC(config.Target, i.config.ConversionDefaults.Target, "loki") == "loki" {
	// 	queryType, err := logql.QueryType(query)
	// 	if err != nil {
	// 		return fmt.Errorf("error parsing loki query: %v", err)
	// 	}
	// 	if queryType != logql.QueryTypeMetric {
	// 		query = fmt.Sprintf("sum(count_over_time(%s[$__auto]))", query)
	// 	}
	// }
	if getC(config.Target, i.config.ConversionDefaults.Target, "loki") == "loki" {
		query = fmt.Sprintf("sum(count_over_time(%s[$__auto]))", query)
	}
	reducer := json.RawMessage(`{"refId":"B","hide":false,"type":"reduce","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[],"type":"gt"},"operator":{"type":"and"},"query":{"params":["B"]},"reducer":{"params":[],"type":"last"}}],"reducer":"last","expression":"A"}`)
	threshold := json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[1],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`)
	// Must manually escape the query as JSON to include it in a json.RawMessage
	escapedQuery, err := escapeQueryJSON(query)
	if err != nil {
		return fmt.Errorf("could not escape provided query: %s", query)
	}
	rule.Data = []definitions.AlertQuery{
		{
			RefID:             "A",
			QueryType:         "instant",
			DatasourceUID:     datasource,
			RelativeTimeRange: timerange,
			Model:             json.RawMessage(fmt.Sprintf(`{"refId":"A","hide":false,"expr":"%s","queryType":"instant","editorMode":"code"}`, escapedQuery)),
		},
		{
			RefID:             "B",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             reducer,
		},
		{
			RefID:             "C",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             threshold,
		},
	}
	rule.Condition = "C"

	return nil
}

func readRuleFromFile(rule *definitions.ProvisionedAlertRule, inputPath string) error {
	if _, err := os.Stat(inputPath); err == nil {
		ruleJSON, err := ReadLocalFile(inputPath)
		if err != nil {
			return fmt.Errorf("error reading rule file %s: %v", inputPath, err)
		}
		err = json.Unmarshal([]byte(ruleJSON), rule)
		if err != nil {
			return fmt.Errorf("error unmarshalling rule file %s: %v", inputPath, err)
		}
	}
	return nil
}

func writeRuleToFile(rule *definitions.ProvisionedAlertRule, outputFile string) error {
	ruleBytes, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("error marshalling alert rule: %v", err)
	}

	// write to output file
	out, err := os.Create(outputFile) // will truncate existing file content
	if err != nil {
		return fmt.Errorf("error opening alert rule file %s to write to: %v", outputFile, err)
	}
	defer out.Close()
	_, err = out.Write(ruleBytes)
	if err != nil {
		return fmt.Errorf("error writing alert rule file to %s: %v", outputFile, err)
	}

	return nil
}

func escapeQueryJSON(query string) (string, error) {
	escapedQuotedQuery, err := json.Marshal(query)
	if err != nil {
		return "", fmt.Errorf("could not escape provided query: %s", query)
	}
	return string(escapedQuotedQuery[1 : len(escapedQuotedQuery)-1]), nil // strip the leading and trailing quotation marks
}

func getC(config, defaultConf, def string) string {
	if config != "" {
		return config
	}
	if defaultConf != "" {
		return defaultConf
	}
	return def
}
