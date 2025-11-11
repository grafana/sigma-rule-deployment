//nolint:goconst
package integrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/spaolacci/murmur3"
)

const TRUE = "true"

type Integrator struct {
	config      model.Configuration
	prettyPrint bool

	allRules     bool
	addedFiles   []string
	removedFiles []string
	testFiles    []string
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

	// Read and parse the YAML config file
	config, err := shared.LoadConfigFromFile(configFile)
	if err != nil {
		return err
	}
	i.config = config
	i.prettyPrint = strings.ToLower(os.Getenv("PRETTY_PRINT")) == TRUE
	i.allRules = strings.ToLower(os.Getenv("ALL_RULES")) == TRUE

	i.config.IntegratorConfig.ContinueOnQueryTestingErrors = strings.ToLower(os.Getenv("CONTINUE_ON_QUERY_TESTING_ERRORS")) == TRUE

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
	testFiles := strings.Split(os.Getenv("TEST_FILES"), " ")

	newUpdatedFiles := make([]string, 0, len(changedFiles))
	removedFiles := make([]string, 0, len(deletedFiles))
	filesToBeTested := make([]string, 0, len(testFiles))

	if i.allRules {
		if err = filepath.Walk(i.config.Folders.ConversionPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("failed to walk directory: %w", err)
			}
			if !info.IsDir() {
				newUpdatedFiles = append(newUpdatedFiles, path)
				// If all files is true, test all files
				if i.config.IntegratorConfig.TestQueries {
					filesToBeTested = append(filesToBeTested, path)
				}
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
		if i.config.IntegratorConfig.TestQueries {
			for _, path := range testFiles {
				relpath, err := filepath.Rel(i.config.Folders.ConversionPath, path)
				if err != nil {
					return fmt.Errorf("error checking file path %s: %v", path, err)
				}
				if relpath == filepath.Base(path) {
					filesToBeTested = append(filesToBeTested, path)
				}
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

	fmt.Printf("Changed files: %d\nRemoved files: %d\nTest files: %d\n", len(newUpdatedFiles), len(removedFiles), len(filesToBeTested))
	i.addedFiles = newUpdatedFiles
	i.removedFiles = removedFiles
	i.testFiles = filesToBeTested

	return nil
}

// cleanupOrphanedFilesInPath removes orphaned files in the specified path
func (i *Integrator) cleanupOrphanedFilesInPath(searchPath string, isOrphaned func(string) (bool, error)) error {
	// Get all JSON files in the path
	var files []string
	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory %s: %w", searchPath, err)
	}

	// Check each file for orphaned status
	for _, file := range files {
		orphaned, err := isOrphaned(file)
		if err != nil {
			fmt.Printf("Warning: Could not check file %s: %v\n", file, err)
			continue
		}

		if orphaned {
			fmt.Printf("Removing orphaned file: %s\n", file)
			if err := os.Remove(file); err != nil {
				fmt.Printf("Warning: Could not remove orphaned file %s: %v\n", file, err)
			}
		}
	}

	return nil
}

// isConversionFileOrphaned checks if a conversion file has no matching configuration
func (i *Integrator) isConversionFileOrphaned(file string) (bool, error) {
	content, err := shared.ReadLocalFile(file)
	if err != nil {
		return false, err
	}

	var conversionObject model.ConversionOutput
	if err := json.Unmarshal([]byte(content), &conversionObject); err != nil {
		return false, err
	}

	// Check if this conversion name has a matching configuration
	for _, conf := range i.config.Conversions {
		if conf.Name == conversionObject.ConversionName {
			return false, nil
		}
	}

	return true, nil
}

// isDeploymentFileOrphaned checks if a deployment file references a missing conversion file
func (i *Integrator) isDeploymentFileOrphaned(file string) (bool, error) {
	content, err := shared.ReadLocalFile(file)
	if err != nil {
		return false, err
	}

	var deploymentRule model.ProvisionedAlertRule
	if err := json.Unmarshal([]byte(content), &deploymentRule); err != nil {
		return false, err
	}

	// Check if the referenced conversion file still exists
	if conversionFile := deploymentRule.Annotations["ConversionFile"]; conversionFile != "" {
		if _, err := os.Stat(conversionFile); os.IsNotExist(err) {
			return true, nil
		}
	}

	return false, nil
}

func (i *Integrator) Run() error {
	// Convert all files that have been updated from the last commit
	if err := i.DoConversions(); err != nil {
		return err
	}

	// Clean up any deleted files
	if err := i.DoCleanup(); err != nil {
		return err
	}

	// Write the output of rules integrated (updated and removed) to the GitHub Action outputs
	return i.SetOutputs()
}

// DoConversions handles the conversion of Sigma rules to Grafana alert rules
func (i *Integrator) DoConversions() error {
	for _, inputFile := range i.addedFiles {
		fmt.Printf("Integrating file: %s\n", inputFile)
		conversionContent, err := shared.ReadLocalFile(inputFile)
		if err != nil {
			return err
		}

		var conversionObject model.ConversionOutput
		err = json.Unmarshal([]byte(conversionContent), &conversionObject)
		if err != nil {
			return fmt.Errorf("error unmarshalling conversion output: %v", err)
		}

		// Find matching configuration using ConversionName
		var config model.ConversionConfig
		for _, conf := range i.config.Conversions {
			if conf.Name == conversionObject.ConversionName {
				config = conf
				break
			}
		}
		if config.Name == "" {
			fmt.Printf("Warning: No configuration found for conversion name: %s, skipping file: %s\n", conversionObject.ConversionName, inputFile)
			continue
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
		rule := &model.ProvisionedAlertRule{UID: ruleUID}

		err = readRuleFromFile(rule, file)
		if err != nil {
			return err
		}
		err = i.ConvertToAlert(rule, queries, titles, config, inputFile, conversionObject)
		if err != nil {
			return err
		}
		err = writeRuleToFile(rule, file, i.prettyPrint)
		if err != nil {
			return err
		}
	}
	return nil
}

// DoCleanup handles the removal of deleted files and cleanup of orphaned files
func (i *Integrator) DoCleanup() error {
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

	// Clean up orphaned conversion files
	if err := i.cleanupOrphanedFilesInPath(i.config.Folders.ConversionPath, i.isConversionFileOrphaned); err != nil {
		fmt.Printf("Warning: Error during orphaned conversion file cleanup: %v\n", err)
	}

	// Clean up orphaned deployment files
	if err := i.cleanupOrphanedFilesInPath(i.config.Folders.DeploymentPath, i.isDeploymentFileOrphaned); err != nil {
		fmt.Printf("Warning: Error during orphaned deployment file cleanup: %v\n", err)
	}

	return nil
}

// Config returns the configuration
func (i *Integrator) Config() model.Configuration {
	return i.config
}

// TestFiles returns the list of test files
func (i *Integrator) TestFiles() []string {
	return i.testFiles
}

// SetOutputs writes the output of rules integrated (updated and removed) to the GitHub Action outputs
func (i *Integrator) SetOutputs() error {
	i.addedFiles = append(i.addedFiles, i.removedFiles...)
	rulesIntegrated := strings.Join(i.addedFiles, " ")

	if err := shared.SetOutput("rules_integrated", rulesIntegrated); err != nil {
		return fmt.Errorf("failed to set rules integrated output: %w", err)
	}
	return nil
}

func (i *Integrator) ConvertToAlert(rule *model.ProvisionedAlertRule, queries []string, titles string, config model.ConversionConfig, conversionFile string, conversionObject model.ConversionOutput) error {
	datasource := shared.GetConfigValue(config.DataSource, i.config.ConversionDefaults.DataSource, "nil")
	timewindow := shared.GetConfigValue(config.TimeWindow, i.config.ConversionDefaults.TimeWindow, "1m")
	duration, err := time.ParseDuration(timewindow)
	if err != nil {
		return fmt.Errorf("error parsing time window: %v", err)
	}

	lookback := shared.GetConfigValue(config.Lookback, i.config.ConversionDefaults.Lookback, "0s")
	lookbackDuration, err := time.ParseDuration(lookback)
	if err != nil {
		return fmt.Errorf("error parsing lookback: %v", err)
	}

	// Apply lookback to time range: now-5m to now with 1m lookback becomes now-6m to now-1m
	fromDuration := duration + lookbackDuration
	toDuration := lookbackDuration
	timerange := model.RelativeTimeRange{From: model.Duration(fromDuration), To: model.Duration(toDuration)}

	queryData := make([]model.AlertQuery, 0, len(queries)+2)
	refIDs := make([]string, len(queries))
	for index, query := range queries {
		refIDs[index] = fmt.Sprintf("A%d", index)
		alertQuery, err := createAlertQuery(query, refIDs[index], datasource, timerange, config, i.config.ConversionDefaults)
		if err != nil {
			return err
		}
		queryData = append(queryData, alertQuery)
	}
	// Use Math expression to combine queries: ${A0}+${A1}+...
	// For single query: ${A0}
	// For multiple queries: ${A0}+${A1}+${A2}
	mathExpression := make([]string, len(refIDs))
	for i, refID := range refIDs {
		mathExpression[i] = fmt.Sprintf("${%s}", refID)
	}
	combiner := json.RawMessage(
		fmt.Sprintf(`{"refId":"B","hide":false,"type":"math","datasource":{"uid":"__expr__","type":"__expr__"},"expression":"%s"}`,
			strings.Join(mathExpression, "+")))
	threshold := json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[0],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`)

	queryData = append(queryData,
		model.AlertQuery{
			RefID:             "B",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             combiner,
		},
		model.AlertQuery{
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
	rule.RuleGroup = shared.GetConfigValue(config.RuleGroup, i.config.ConversionDefaults.RuleGroup, "Default")
	rule.NoDataState = model.OK
	rule.ExecErrState = model.OkErrState
	rule.Title = titles
	rule.Condition = "C"

	// Add annotations for context
	if rule.Annotations == nil {
		rule.Annotations = make(map[string]string)
	}

	rule.Annotations["Query"] = queries[0]
	rule.Annotations["TimeWindow"] = timewindow
	rule.Annotations["Lookback"] = lookback

	// LogSourceUid annotation (data source)
	rule.Annotations["LogSourceUid"] = datasource

	// LogSourceType annotation (target)
	logSourceType := shared.GetConfigValue(config.Target, i.config.ConversionDefaults.Target, shared.Loki)
	rule.Annotations["LogSourceType"] = logSourceType

	// Path to associated conversion file
	rule.Annotations["ConversionFile"] = conversionFile

	if i.config.IntegratorConfig.TemplateAnnotations != nil {
		for key, value := range i.config.IntegratorConfig.TemplateAnnotations {
			tmpl, err := template.New("annotation_" + key).Parse(value)
			if err != nil {
				return fmt.Errorf("error parsing template %s: %v", key, err)
			}
			var buf bytes.Buffer
			if i.config.IntegratorConfig.TemplateAllRules {
				err = tmpl.Execute(&buf, conversionObject.Rules)
			} else {
				err = tmpl.Execute(&buf, conversionObject.Rules[0])
			}
			if err != nil {
				return fmt.Errorf("error executing template %s: %v", key, err)
			}
			rule.Annotations[key] = buf.String()
		}
	}

	if rule.Labels == nil {
		rule.Labels = make(map[string]string)
	}

	if i.config.IntegratorConfig.TemplateLabels != nil {
		for key, value := range i.config.IntegratorConfig.TemplateLabels {
			tmpl, err := template.New("label_" + key).Parse(value)
			if err != nil {
				return fmt.Errorf("error parsing template %s: %v", key, err)
			}
			var buf bytes.Buffer
			if i.config.IntegratorConfig.TemplateAllRules {
				err = tmpl.Execute(&buf, conversionObject.Rules)
			} else {
				err = tmpl.Execute(&buf, conversionObject.Rules[0])
			}
			if err != nil {
				return fmt.Errorf("error executing template %s: %v", key, err)
			}
			rule.Labels[key] = buf.String()
		}
	}

	return nil
}

func readRuleFromFile(rule *model.ProvisionedAlertRule, inputPath string) error {
	if _, err := os.Stat(inputPath); err == nil {
		ruleJSON, err := shared.ReadLocalFile(inputPath)
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

func writeRuleToFile(rule *model.ProvisionedAlertRule, outputFile string, prettyPrint bool) error {
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

func summariseSigmaRules(rules []model.SigmaRule) (id uuid.UUID, title string, err error) {
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

func getRuleUID(conversionName string, conversionID uuid.UUID) string {
	hash := int64(murmur3.Sum32([]byte(conversionName + "_" + conversionID.String())))
	return fmt.Sprintf("%x", hash)
}

// createAlertQuery creates an AlertQuery based on the target data source and configuration
func createAlertQuery(query string, refID string, datasource string, timerange model.RelativeTimeRange, config model.ConversionConfig, defaultConf model.ConversionConfig) (model.AlertQuery, error) {
	datasourceType := shared.GetConfigValue(config.DataSourceType, defaultConf.DataSourceType, shared.GetConfigValue(config.Target, defaultConf.Target, shared.Loki))
	customModel := shared.GetConfigValue(config.QueryModel, defaultConf.QueryModel, "")

	// Modify query based on target data source
	if datasourceType == shared.Loki {
		// if the query is not a metric query, we need to add a sum aggregation to it
		if !strings.HasPrefix(query, "sum") {
			query = fmt.Sprintf("sum(count_over_time(%s[$__auto]))", query)
		}
	}

	// Must manually escape the query as JSON to include it in a json.RawMessage
	escapedQuery, err := shared.EscapeQueryJSON(query)
	if err != nil {
		return model.AlertQuery{}, fmt.Errorf("could not escape provided query: %s", query)
	}

	// Create generic alert query
	alertQuery := model.AlertQuery{
		RefID:             refID,
		DatasourceUID:     datasource,
		RelativeTimeRange: timerange,
	}

	// Populate the alert query model, first see if the user has provided a custom model
	// else use defaults based on the target data source type
	switch {
	case customModel != "":
		alertQuery.Model = json.RawMessage(fmt.Sprintf(customModel, refID, datasource, escapedQuery))
	case datasourceType == shared.Loki:
		alertQuery.QueryType = "instant"
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"loki","uid":"%s"},"hide":false,"expr":"%s","queryType":"instant","editorMode":"code"}`, refID, datasource, escapedQuery))
	case datasourceType == shared.Elasticsearch:
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
