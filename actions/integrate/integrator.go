package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"gopkg.in/yaml.v3"
)

type SigmaRule struct {
	Title   string `json:"title"`
	ID      string `json:"id"`
	Related []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"related"`
	Name        string   `json:"name"`
	Taxonomy    string   `json:"taxonomy"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
	License     string   `json:"license"`
	Author      string   `json:"author"`
	References  []string `json:"references"`
	Date        string   `json:"date"`
	Modified    string   `json:"modified"`
	Logsource   struct {
		Category   string `json:"category"`
		Product    string `json:"product"`
		Service    string `json:"service"`
		Definition string `json:"definition"`
	} `json:"logsource"`
	Detection      any      `json:"detection"`
	Correlation    any      `json:"correlation"`
	Fields         []string `json:"fields"`
	FalsePositives []string `json:"falsepositives"`
	Level          string   `json:"level"`
	Tags           []string `json:"tags"`
	Scope          string   `json:"scope"`
	Generate       bool     `json:"generate"`
}

type ConversionOutput struct {
	Queries        []string    `json:"queries"`
	ConversionName string      `json:"conversion_name"`
	InputFile      string      `json:"input_file"`
	Rules          []SigmaRule `json:"rules"`
	OutputFile     string      `json:"output_file"`
}

type Integrator struct {
	config      Configuration
	prettyPrint bool

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
	i.prettyPrint = strings.ToLower(os.Getenv("PRETTY_PRINT")) == "true"

	if !filepath.IsLocal(i.config.Folders.ConversionPath) {
		return fmt.Errorf("conversion path is not local: %s", i.config.Folders.ConversionPath)
	}
	if !filepath.IsLocal(i.config.Folders.DeploymentPath) {
		return fmt.Errorf("deployment path is not local: %s", i.config.Folders.DeploymentPath)
	}

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
		conversionContent, err := ReadLocalFile(inputFile)
		if err != nil {
			return err
		}

		config := ConversionConfig{}
		for _, conf := range i.config.Conversions {
			if conf.Name+".json" == filepath.Base(inputFile) {
				config = conf
				break
			}
		}
		if config.Name == "" {
			return fmt.Errorf("no conversion configuration found for file: %s", inputFile)
		}

		var conversionObjects []ConversionOutput
		err = json.Unmarshal([]byte(conversionContent), &conversionObjects)
		if err != nil {
			return fmt.Errorf("error unmarshalling conversion output: %v", err)
		}

		for conversionIndex, conversionObject := range conversionObjects {
			queries := conversionObject.Queries
			if len(queries) == 0 {
				fmt.Printf("no queries found in conversion object: %d", conversionIndex)
				continue
			}

			conversionID, titles, err := summariseSigmaRules(conversionObject.Rules)
			if err != nil {
				return fmt.Errorf("error summarising sigma rules: %v", err)
			}

			file := fmt.Sprintf("%s%salert_rule_%s_%s.json", i.config.Folders.DeploymentPath, string(filepath.Separator), config.Name, conversionID.String())
			rule := &definitions.ProvisionedAlertRule{UID: conversionID.String()}
			err = readRuleFromFile(rule, file)
			if err != nil {
				return err
			}
			err = i.ConvertToAlert(rule, queries, titles, config)
			if err != nil {
				return err
			}
			err = writeRuleToFile(rule, file, i.prettyPrint)
			if err != nil {
				return err
			}

			resp, err := TestQuery(
				query, config.DataSource, i.config.DeployerConfig.GrafanaInstance)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
		}
	}

	for _, deletedFile := range i.removedFiles {
		deploymentGlob := fmt.Sprintf("alert_rule_%s_*.json", strings.TrimSuffix(filepath.Base(deletedFile), ".json"))
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

func (i *Integrator) ConvertToAlert(rule *definitions.ProvisionedAlertRule, queries []string, titles string, config ConversionConfig) error {
	datasource := getC(config.DataSource, i.config.ConversionDefaults.DataSource, "nil")
	timewindow := getC(config.TimeWindow, i.config.ConversionDefaults.TimeWindow, "1m")
	duration, err := time.ParseDuration(timewindow)
	if err != nil {
		return fmt.Errorf("error parsing time window: %v", err)
	}
	timerange := definitions.RelativeTimeRange{From: definitions.Duration(duration), To: definitions.Duration(time.Duration(0))}

	queryData := make([]definitions.AlertQuery, 0, len(queries)+2)
	refIds := make([]string, len(queries))
	for index, query := range queries {
		if getC(config.Target, i.config.ConversionDefaults.Target, "loki") == "loki" {
			// if the query is not a metric query, we need to add a sum aggregation to it
			if !strings.HasPrefix(query, "sum") {
				query = fmt.Sprintf("sum(count_over_time(%s[$__auto]))", query)
			}
		}
		// Must manually escape the query as JSON to include it in a json.RawMessage
		escapedQuery, err := escapeQueryJSON(query)
		if err != nil {
			return fmt.Errorf("could not escape provided query: %s", query)
		}
		refIds[index] = fmt.Sprintf("A%d", index)
		queryData = append(queryData,
			definitions.AlertQuery{
				RefID:             refIds[index],
				QueryType:         "instant",
				DatasourceUID:     datasource,
				RelativeTimeRange: timerange,
				Model:             json.RawMessage(fmt.Sprintf(`{"refId":"%s","hide":false,"expr":"%s","queryType":"instant","editorMode":"code"}`, refIds[index], escapedQuery)),
			})
	}
	reducer := json.RawMessage(
		fmt.Sprintf(`{"refId":"B","hide":false,"type":"reduce","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[],"type":"gt"},"operator":{"type":"and"},"query":{"params":["B"]},"reducer":{"params":[],"type":"last"}}],"reducer":"last","expression":"%s"}`,
			strings.Join(refIds, "||")))
	threshold := json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[1],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`)

	queryData = append(queryData,
		definitions.AlertQuery{
			RefID:             "B",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             reducer,
		},
		definitions.AlertQuery{
			RefID:             "C",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             threshold,
		},
	)

	if len(queryData) == len(rule.Data) {
		for qIdx, query := range queryData {
			if !bytes.Equal(query.Model, rule.Data[qIdx].Model) {
				break
			}
			if qIdx == len(queryData)-1 {
				// if we get here, all the queries are the same, no need to update the rule
				return nil
			}
		}
	}
	rule.Data = queryData

	// alerting rule metadata
	rule.OrgID = i.config.IntegratorConfig.OrgID
	rule.FolderUID = i.config.IntegratorConfig.FolderID
	rule.RuleGroup = getC(config.RuleGroup, i.config.ConversionDefaults.RuleGroup, "Default")
	rule.NoDataState = definitions.OK
	rule.ExecErrState = definitions.OkErrState
	rule.Updated = time.Now()
	rule.Title = titles
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

func writeRuleToFile(rule *definitions.ProvisionedAlertRule, outputFile string, prettyPrint bool) error {
	var ruleBytes []byte
	var err error
	if prettyPrint {
		ruleBytes, err = json.MarshalIndent(rule, "", "  ")
	} else {
		ruleBytes, err = json.Marshal(rule)
	}
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

func summariseSigmaRules(rules []SigmaRule) (id uuid.UUID, title string, err error) {
	if len(rules) == 0 {
		return uuid.Nil, "", fmt.Errorf("no rules provided")
	}
	conversionIDBytes := make([]byte, 16)
	titles := make([]string, len(rules))
	for ruleIndex, rule := range rules {
		titles[ruleIndex] = rule.Title
		if ruleID, err := uuid.Parse(rule.ID); err == nil {
			if ruleIndex > 0 {
				// xor the rule IDs together to get a unique conversion ID
				for i, b := range ruleID {
					conversionIDBytes[i] ^= b
				}
			} else {
				conversionIDBytes = ruleID[:]
			}
		} else {
			return uuid.Nil, "", fmt.Errorf("error parsing rule ID %s: %v", rule.ID, err)
		}
	}
	// Ensure the final conversion ID is version 4 and variant 10
	conversionIDBytes[6] = (conversionIDBytes[6] & 0x0f) | 0x40
	conversionIDBytes[8] = (conversionIDBytes[8] & 0x3f) | 0x80
	conversionID, err := uuid.FromBytes(conversionIDBytes)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("error creating conversion ID from bytes %s: %v", conversionIDBytes, err)
	}
	title = strings.Join(titles, " & ")
	if len(title) > 190 {
		title = title[:190]
	}
	return conversionID, title, nil
}
