package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"github.com/spaolacci/murmur3"
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

	allRules     bool
	addedFiles   []string
	removedFiles []string
}

type Stats struct {
	Count  int               `json:"count"`
	Fields map[string]string `json:"fields"`
	Errors []string          `json:"errors"`
}

type QueryTestResult struct {
	Datasource string `json:"datasource"`
	Link       string `json:"link"`
	Stats      Stats  `json:"stats"`
}

// Frame represents a single frame from a Grafana datasource query response
type Frame struct {
	Schema struct {
		Fields []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fields"`
	} `json:"schema"`
	Data struct {
		Values [][]any `json:"values"`
	} `json:"data"`
}

// ResultFrame represents a single result frame in the query response
type ResultFrame struct {
	Frames []Frame `json:"frames"`
}

// QueryResponse represents the structure of a Grafana datasource query response
type QueryResponse struct {
	Results map[string]ResultFrame `json:"results"`
	Errors  []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
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
	fmt.Printf("Loading config from %s\n", configFile)

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
	i.allRules = strings.ToLower(os.Getenv("ALL_RULES")) == "true"

	if !filepath.IsLocal(i.config.Folders.ConversionPath) {
		return fmt.Errorf("conversion path is not local: %s", i.config.Folders.ConversionPath)
	}
	if !filepath.IsLocal(i.config.Folders.DeploymentPath) {
		return fmt.Errorf("deployment path is not local: %s", i.config.Folders.DeploymentPath)
	}

	fmt.Printf("Conversion path: %s\nDeployment path: %s\n", i.config.Folders.ConversionPath, i.config.Folders.DeploymentPath)

	if _, err = os.Stat(i.config.Folders.DeploymentPath); err != nil {
		err = os.MkdirAll(i.config.Folders.DeploymentPath, 0o755)
		if err != nil {
			return fmt.Errorf("error creating deployment directory: %v", err)
		}
	}

	// If from and to are not provided, use the default values
	// to query for the last hour.
	if i.config.IntegratorConfig.From == "" {
		i.config.IntegratorConfig.From = "now-1h"
	}
	if i.config.IntegratorConfig.To == "" {
		i.config.IntegratorConfig.To = "now"
	}

	changedFiles := strings.Split(os.Getenv("CHANGED_FILES"), " ")
	deletedFiles := strings.Split(os.Getenv("DELETED_FILES"), " ")

	newUpdatedFiles := make([]string, 0, len(changedFiles))
	removedFiles := make([]string, 0, len(deletedFiles))

	if i.allRules {
		if err = filepath.Walk(i.config.Folders.ConversionPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("failed to walk directory: %w", err)
			}
			if !info.IsDir() {
				newUpdatedFiles = append(newUpdatedFiles, path)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		for _, path := range changedFiles {
			// Ensure paths appear within specified conversion path
			relpath, err := filepath.Rel(i.config.Folders.ConversionPath, path)
			if err != nil {
				return fmt.Errorf("error checking file path %s: %v", path, err)
			}
			if relpath == filepath.Base(path) {
				newUpdatedFiles = append(newUpdatedFiles, path)
			}
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

	fmt.Printf("Changed files: %d\nRemoved files: %d\n", len(newUpdatedFiles), len(removedFiles))
	i.addedFiles = newUpdatedFiles
	i.removedFiles = removedFiles

	return nil
}

func (i *Integrator) Run() error {
	// Parse the timeout from configuration
	timeoutDuration := 10 * time.Second // Default timeout
	if i.config.DeployerConfig.Timeout != "" {
		parsedTimeout, err := time.ParseDuration(i.config.DeployerConfig.Timeout)
		if err != nil {
			fmt.Printf("Warning: Invalid timeout format in config, using default: %v\n", err)
		} else {
			timeoutDuration = parsedTimeout
		}
	}

	if i.config.IntegratorConfig.TestQueries {
		fmt.Println("Testing queries against the datasource")
	}
	queryTestResults := make(map[string][]QueryTestResult, len(i.addedFiles))

	for _, inputFile := range i.addedFiles {
		fmt.Printf("Integrating file: %s\n", inputFile)
		conversionContent, err := ReadLocalFile(inputFile)
		if err != nil {
			return err
		}

		var conversionObject ConversionOutput
		err = json.Unmarshal([]byte(conversionContent), &conversionObject)
		if err != nil {
			return fmt.Errorf("error unmarshalling conversion output: %v", err)
		}

		// Find matching configuration using ConversionName
		var config ConversionConfig
		for _, conf := range i.config.Conversions {
			if conf.Name == conversionObject.ConversionName {
				config = conf
				break
			}
		}
		if config.Name == "" {
			return fmt.Errorf("no conversion configuration found for conversion name: %s", conversionObject.ConversionName)
		}

		queries := conversionObject.Queries
		if len(queries) == 0 {
			fmt.Printf("no queries found in conversion object")
			continue
		}

		conversionID, titles, err := summariseSigmaRules(conversionObject.Rules)
		if err != nil {
			return fmt.Errorf("error summarising sigma rules: %v", err)
		}

		// Extract rule filename from input file name
		ruleFilename := strings.TrimSuffix(filepath.Base(inputFile), ".json")
		ruleFilename = strings.TrimPrefix(ruleFilename, config.Name+"_")
		ruleUID := getRuleUID(conversionObject.ConversionName, conversionID)
		file := fmt.Sprintf("%s%salert_rule_%s_%s_%s.json", i.config.Folders.DeploymentPath, string(filepath.Separator), config.Name, ruleFilename, ruleUID)
		fmt.Printf("Working on alert rule file: %s\n", file)
		rule := &definitions.ProvisionedAlertRule{UID: ruleUID}

		err = readRuleFromFile(rule, file)
		if err != nil {
			return err
		}
		err = i.ConvertToAlert(rule, queries, titles, config, inputFile)
		if err != nil {
			return err
		}
		err = writeRuleToFile(rule, file, i.prettyPrint)
		if err != nil {
			return err
		}

		if i.config.IntegratorConfig.TestQueries {
			fmt.Println("Testing queries against the datasource")
			// Test all queries against the datasource
			queryResults, err := i.TestQueries(queries, config, i.config.ConversionDefaults, timeoutDuration)
			if err != nil {
				return err
			}

			queryTestResults[inputFile] = queryResults
		}
	}

	if i.config.IntegratorConfig.TestQueries {
		// Marshal all query results into a single JSON object
		resultsJSON, err := json.Marshal(queryTestResults)
		if err != nil {
			return fmt.Errorf("error marshalling query results: %v", err)
		}

		// Set a single output with all results
		if err := SetOutput("test_query_results", string(resultsJSON)); err != nil {
			return fmt.Errorf("failed to set test query results output: %w", err)
		}
	}

	for _, deletedFile := range i.removedFiles {
		fmt.Printf("Deleting alert rule file: %s\n", deletedFile)
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
	if err := SetOutput("rules_integrated", rulesIntegrated); err != nil {
		return fmt.Errorf("failed to set rules integrated output: %w", err)
	}

	return nil
}

func (i *Integrator) ConvertToAlert(rule *definitions.ProvisionedAlertRule, queries []string, titles string, config ConversionConfig, conversionFile string) error {
	datasource := getC(config.DataSource, i.config.ConversionDefaults.DataSource, "nil")
	timewindow := getC(config.TimeWindow, i.config.ConversionDefaults.TimeWindow, "1m")
	duration, err := time.ParseDuration(timewindow)
	if err != nil {
		return fmt.Errorf("error parsing time window: %v", err)
	}
	timerange := definitions.RelativeTimeRange{From: definitions.Duration(duration), To: definitions.Duration(time.Duration(0))}

	queryData := make([]definitions.AlertQuery, 0, len(queries)+2)
	refIDs := make([]string, len(queries))
	for index, query := range queries {
		refIDs[index] = fmt.Sprintf("A%d", index)
		alertQuery, err := createAlertQuery(query, refIDs[index], datasource, timerange, config, i.config.ConversionDefaults)
		if err != nil {
			return err
		}
		queryData = append(queryData, alertQuery)
	}
	reducer := json.RawMessage(
		fmt.Sprintf(`{"refId":"B","hide":false,"type":"reduce","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[],"type":"gt"},"operator":{"type":"and"},"query":{"params":["B"]},"reducer":{"params":[],"type":"last"}}],"reducer":"last","expression":"%s"}`,
			strings.Join(refIDs, "||")))
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
				fmt.Printf("No changes to the relevant alert rule, skipping\n")
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

	// Add annotations for context
	if rule.Annotations == nil {
		rule.Annotations = make(map[string]string)
	}

	rule.Annotations["Query"] = strings.Join(queries, " | ")
	rule.Annotations["TimeWindow"] = timewindow

	// LogSourceUid annotation (data source)
	rule.Annotations["LogSourceUid"] = datasource

	// LogSourceType annotation (target)
	logSourceType := getC(config.Target, i.config.ConversionDefaults.Target, "loki")
	rule.Annotations["LogSourceType"] = logSourceType

	// Path to associated conversion file
	rule.Annotations["ConversionFile"] = conversionFile

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

// processFrame processes a single frame from the query response and updates the result stats
func (i *Integrator) processFrame(frame Frame, result *QueryTestResult) error {
	// Map field names to their indices
	fieldIndices := make(map[string]int)
	for i, field := range frame.Schema.Fields {
		fieldIndices[field.Name] = i
	}

	// Skip if no values
	if len(frame.Data.Values) == 0 {
		return nil
	}

	// Get the number of rows from the first field's values
	numRows := 0
	for _, values := range frame.Data.Values {
		if len(values) > numRows {
			numRows = len(values)
		}
	}

	// Process each row of values
	for rowIndex := 0; rowIndex < numRows; rowIndex++ {
		// Process labels if present
		if labelIndex, ok := fieldIndices["labels"]; ok {
			if labelIndex < len(frame.Data.Values) {
				if rowIndex < len(frame.Data.Values[labelIndex]) {
					if labelValues, ok := frame.Data.Values[labelIndex][rowIndex].(map[string]any); ok {
						for label, value := range labelValues {
							if _, exists := result.Stats.Fields[label]; !exists {
								result.Stats.Fields[label] = fmt.Sprintf("%v", value)
							}
						}
					}
				}
			}
		}

		// Process Line field if present
		if lineIndex, ok := fieldIndices["Line"]; ok {
			if lineIndex < len(frame.Data.Values) {
				if rowIndex < len(frame.Data.Values[lineIndex]) {
					if lineValue, ok := frame.Data.Values[lineIndex][rowIndex].(string); ok {
						result.Stats.Count++
						// Only store the line value if show_log_lines is enabled
						if i.config.IntegratorConfig.ShowLogLines {
							if _, exists := result.Stats.Fields["Line"]; !exists {
								result.Stats.Fields["Line"] = lineValue
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func (i *Integrator) TestQueries(queries []string, config, defaultConf ConversionConfig, timeoutDuration time.Duration) ([]QueryTestResult, error) {
	queryResults := make([]QueryTestResult, 0, len(queries))
	datasource := getC(config.DataSource, defaultConf.DataSource, "")
	for _, query := range queries {
		resp, err := TestQuery(
			query,
			datasource,
			i.config.DeployerConfig.GrafanaInstance,
			os.Getenv("INTEGRATOR_GRAFANA_SA_TOKEN"),
			i.config.IntegratorConfig.From,
			i.config.IntegratorConfig.To,
			timeoutDuration,
		)
		if err != nil {
			return nil, fmt.Errorf("error testing query %s: %v", query, err)
		}

		jsonQuery, err := json.Marshal(query)
		if err != nil {
			return nil, fmt.Errorf("error marshalling query %s: %v", query, err)
		}

		pane := fmt.Sprintf(`{"yyz":{"datasource":"%[1]s","queries":[{"refId":"A","expr":%[2]s,"queryType":"range","datasource":{"type":"loki","uid":"%[1]s"},"editorMode":"code","direction":"backward"}],"range":{"from":"%[3]s","to":"%[4]s"}}}`, datasource, string(jsonQuery), i.config.IntegratorConfig.From, i.config.IntegratorConfig.To)
		// Parse the response to extract statistics
		result := QueryTestResult{
			Datasource: datasource,
			Link:       fmt.Sprintf("%s/explore?schemaVersion=1&panes=%s&orgId=%d", i.config.DeployerConfig.GrafanaInstance, url.QueryEscape(pane), i.config.IntegratorConfig.OrgID),
			Stats: Stats{
				Fields: make(map[string]string),
				Errors: make([]string, 0),
			},
		}

		// Parse the response to extract statistics
		var responseData QueryResponse
		if err := json.Unmarshal(resp, &responseData); err != nil {
			return nil, fmt.Errorf("error unmarshalling query response: %v", err)
		}

		// Process errors
		for _, err := range responseData.Errors {
			if err.Type != "cancelled" && err.Message != "" {
				result.Stats.Errors = append(result.Stats.Errors, err.Message)
			}
		}

		// Process data frames from all results
		for _, resultFrame := range responseData.Results {
			for _, frame := range resultFrame.Frames {
				if err := i.processFrame(frame, &result); err != nil {
					return nil, fmt.Errorf("error processing frame: %v", err)
				}
			}
		}

		queryResults = append(queryResults, result)
	}

	return queryResults, nil
}

func getRuleUID(conversionName string, conversionID uuid.UUID) string {
	hash := int64(murmur3.Sum32([]byte(conversionName + "_" + conversionID.String())))
	return fmt.Sprintf("%x", hash)
}

// createAlertQuery creates an AlertQuery based on the target data source and configuration
func createAlertQuery(query string, refID string, datasource string, timerange definitions.RelativeTimeRange, config ConversionConfig, defaultConf ConversionConfig) (definitions.AlertQuery, error) {
	datasourceType := getC(config.DataSourceType, defaultConf.DataSourceType, getC(config.Target, defaultConf.Target, "loki"))
	customModel := getC(config.QueryModel, defaultConf.QueryModel, "")

	// Modify query based on target data source
	if datasourceType == "loki" {
		// if the query is not a metric query, we need to add a sum aggregation to it
		if !strings.HasPrefix(query, "sum") {
			query = fmt.Sprintf("sum(count_over_time(%s[$__auto]))", query)
		}
	}

	// Must manually escape the query as JSON to include it in a json.RawMessage
	escapedQuery, err := escapeQueryJSON(query)
	if err != nil {
		return definitions.AlertQuery{}, fmt.Errorf("could not escape provided query: %s", query)
	}

	// Create generic alert query
	alertQuery := definitions.AlertQuery{
		RefID:             refID,
		DatasourceUID:     datasource,
		RelativeTimeRange: timerange,
	}

	// Populate the alert query model, first see if the user has provided a custom model
	// else use defaults based on the target data source type
	switch {
	case customModel != "":
		alertQuery.Model = json.RawMessage(fmt.Sprintf(customModel, refID, datasource, escapedQuery))
	case datasourceType == "loki":
		alertQuery.QueryType = "instant"
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"loki","uid":"%s"},"hide":false,"expr":"%s","queryType":"instant","editorMode":"code"}`, refID, datasource, escapedQuery))
	case datasourceType == "elasticsearch":
		// Based on the Elasticsearch data source plugin
		// https://github.com/grafana/grafana/blob/main/public/app/plugins/datasource/elasticsearch/dataquery.gen.ts
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"elasticsearch","uid":"%s"},"query":"%s","alias":"","metrics":[{"type":"count","id":"1"}],"bucketAggs":[{"type":"date_histogram","id":"2","settings":{"interval":"auto"}}],"intervalMs":2000,"maxDataPoints":1354,"timeField":"@timestamp"}`, refID, datasource, escapedQuery))
	default:
		// try a basic query
		fmt.Printf("WARNING: Using generic query model for the data source type %s; if these queries don't work, try configuring a custom query_model\n", datasourceType)
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"%s","uid":"%s"},"query":"%s"}`, refID, datasourceType, datasource, escapedQuery))
	}

	return alertQuery, nil
}
