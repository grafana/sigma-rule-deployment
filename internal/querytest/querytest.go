package querytest

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/integrate"
	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
)

// QueryTester handles testing queries against Grafana datasources
type QueryTester struct {
	config    model.Configuration
	testFiles []string
	timeout   time.Duration
}

// NewQueryTester creates a new QueryTester instance
func NewQueryTester(config model.Configuration, testFiles []string, timeout time.Duration) *QueryTester {
	return &QueryTester{
		config:    config,
		testFiles: testFiles,
		timeout:   timeout,
	}
}

// Run executes query testing for all test files
func (qt *QueryTester) Run() error {
	fmt.Println("Testing queries against the datasource")
	queryTestResults := make(map[string][]model.QueryTestResult, len(qt.testFiles))

	for _, inputFile := range qt.testFiles {
		fmt.Printf("Testing queries for file: %s\n", inputFile)
		conversionContent, err := shared.ReadLocalFile(inputFile)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", inputFile, err)
			if !qt.config.Defaults.Integration.ContinueOnError {
				return err
			}
			continue
		}

		var conversionObject model.ConversionOutput
		err = json.Unmarshal([]byte(conversionContent), &conversionObject)
		if err != nil {
			fmt.Printf("Error unmarshalling conversion output for file %s: %v\n", inputFile, err)
			if !qt.config.Defaults.Integration.ContinueOnError {
				return fmt.Errorf("error unmarshalling conversion output: %v", err)
			}
			continue
		}

		// Find matching configuration using ConversionName
		var cfg model.NamedConfigBlock
		for _, c := range qt.config.Configurations {
			if c.Name == conversionObject.ConversionName {
				cfg = c
				break
			}
		}
		if cfg.Name == "" {
			fmt.Printf("Warning: No configuration found for conversion name: %s, skipping file: %s\n", conversionObject.ConversionName, inputFile)
			continue
		}

		// Skip if neither the default nor this configuration has TestQueries enabled
		if !qt.config.Defaults.Integration.TestQueries && !cfg.Integration.TestQueries {
			continue
		}

		// Per-configuration ContinueOnError overrides the global default (additive: either source enables it)
		continueOnError := qt.config.Defaults.Integration.ContinueOnError || cfg.Integration.ContinueOnError

		queries := conversionObject.Queries
		if len(queries) == 0 {
			fmt.Printf("No queries found in conversion object for file %s\n", inputFile)
			continue
		}

		// Convert queries slice to map with refIDs
		queryMap := make(map[string]string, len(queries))
		for index, query := range queries {
			refID := fmt.Sprintf("A%d", index)
			queryMap[refID] = query
		}

		// Test all queries against the datasource
		queryResults, err := qt.TestQueries(
			queryMap, cfg, qt.config.Defaults,
		)
		if err != nil {
			fmt.Printf("Error testing queries for file %s: %v\n", inputFile, err)
			// Return error if continue on query testing errors is not enabled
			if !continueOnError {
				return err
			}
		}

		for _, result := range queryResults {
			if len(result.Stats.Errors) > 0 {
				fmt.Printf("Query testing errors occurred for file %s\n", inputFile)
				fmt.Printf("Datasource: %s\n", result.Datasource)
				for _, error := range result.Stats.Errors {
					fmt.Printf("Error: %s\n", error)
				}
			}
		}

		if len(queryResults) > 0 {
			fmt.Printf("Query testing completed successfully for file %s\n", inputFile)
			if len(queryResults) == 1 {
				result := queryResults[0]
				fmt.Printf("Query returned results: %d\n", result.Stats.Count)
				if result.Stats.ExecutionTime.Unit != "" {
					fmt.Printf("Execution time: %g %s\n", result.Stats.ExecutionTime.Value, result.Stats.ExecutionTime.Unit)
				}
				if result.Stats.BytesProcessed.Unit != "" {
					fmt.Printf("Bytes processed: %g %s\n", result.Stats.BytesProcessed.Value, result.Stats.BytesProcessed.Unit)
				}
			} else {
				fmt.Printf("Queries returned results:\n")
				for i, result := range queryResults {
					fmt.Printf("Query %d: %d\n", i, result.Stats.Count)
					if result.Stats.ExecutionTime.Unit != "" {
						fmt.Printf("  Execution time: %g %s\n", result.Stats.ExecutionTime.Value, result.Stats.ExecutionTime.Unit)
					}
					if result.Stats.BytesProcessed.Unit != "" {
						fmt.Printf("  Bytes processed: %g %s\n", result.Stats.BytesProcessed.Value, result.Stats.BytesProcessed.Unit)
					}
				}
			}
		} else if err == nil {
			fmt.Printf("Query testing completed successfully for file %s\n", inputFile)
		}

		queryTestResults[inputFile] = queryResults
	}

	resultsJSON, err := json.Marshal(queryTestResults)
	if err != nil {
		return fmt.Errorf("error marshalling query results: %v", err)
	}

	// Set a single output with all results
	if err := shared.SetOutput("test_query_results", string(resultsJSON)); err != nil {
		return fmt.Errorf("failed to set test query results output: %w", err)
	}

	return nil
}

// TestQueries tests a map of queries against the datasource
func (qt *QueryTester) TestQueries(queries map[string]string, cfg model.NamedConfigBlock, defaults model.ConfigBlock) ([]model.QueryTestResult, error) {
	queryResults := make([]model.QueryTestResult, 0, len(queries))
	datasource := shared.GetConfigValue(cfg.Integration.DataSource, defaults.Integration.DataSource, "")
	// Determine datasource type using the same logic as createAlertQuery
	datasourceType := shared.GetConfigValue(
		cfg.Integration.DataSourceType,
		defaults.Integration.DataSourceType,
		shared.GetConfigValue(cfg.Conversion.Target, defaults.Conversion.Target, shared.Loki),
	)
	customModel := shared.GetConfigValue(cfg.Integration.QueryModel, defaults.Integration.QueryModel, "")
	from := shared.GetConfigValue(cfg.Integration.From, defaults.Integration.From, "now-1h")
	to := shared.GetConfigValue(cfg.Integration.To, defaults.Integration.To, "now")
	grafanaInstance := shared.GetConfigValue(cfg.Deployment.GrafanaInstance, defaults.Deployment.GrafanaInstance, "")
	timeout := qt.timeout
	if cfg.Deployment.Timeout != "" {
		if parsedTimeout, err := time.ParseDuration(cfg.Deployment.Timeout); err == nil {
			timeout = parsedTimeout
		}
	}
	showSampleValues := defaults.Integration.ShowSampleValues || cfg.Integration.ShowSampleValues
	showLogLines := defaults.Integration.ShowLogLines || cfg.Integration.ShowLogLines

	// Sort refIDs to ensure consistent ordering
	refIDs := make([]string, 0, len(queries))
	for refID := range queries {
		refIDs = append(refIDs, refID)
	}
	sort.Strings(refIDs)

	for _, refID := range refIDs {
		query := queries[refID]
		resp, err := integrate.TestQuery(
			query,
			datasource,
			grafanaInstance,
			os.Getenv("INTEGRATOR_GRAFANA_SA_TOKEN"),
			refID,
			from,
			to,
			customModel,
			timeout,
		)
		if err != nil {
			return []model.QueryTestResult{
				{
					Datasource: datasource,
					Link:       "",
					Stats: model.Stats{
						Fields: make(map[string]string),
						Errors: []string{err.Error()},
					},
				},
			}, fmt.Errorf("error testing query %s: %v", query, err)
		}

		// Generate explore link based on datasource type
		orgID := defaults.Integration.OrgID
		if cfg.Integration.OrgID != 0 {
			orgID = cfg.Integration.OrgID
		}
		exploreLink, err := GenerateExploreLink(
			query, datasource, datasourceType, cfg, defaults,
			grafanaInstance,
			from,
			to,
			orgID,
		)
		if err != nil {
			return nil, fmt.Errorf("error generating explore link: %v", err)
		}
		// Parse the response to extract statistics
		result := model.QueryTestResult{
			Datasource: datasource,
			Link:       exploreLink,
			Stats: model.Stats{
				Fields: make(map[string]string),
				Errors: make([]string, 0),
			},
		}

		// Parse the response to extract statistics
		var responseData model.QueryResponse
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
				if err := ProcessFrame(
					frame,
					&result,
					showSampleValues,
					showLogLines,
				); err != nil {
					return nil, fmt.Errorf("error processing frame: %v", err)
				}
			}
		}

		queryResults = append(queryResults, result)
	}

	return queryResults, nil
}

var (
	bytesProcessedStatKey = "Summary: total bytes processed"
	executionTimeStatKey  = "Summary: exec time"
)

// ProcessFrame processes a single frame from the query response and updates the result stats
func ProcessFrame(frame model.Frame, result *model.QueryTestResult, showSampleValues, showLogLines bool) error {
	// Get metrics from frame metadata (Stats are nested within Schema.Meta)
	for _, stat := range frame.Schema.Meta.Stats {
		switch {
		case strings.Contains(stat.DisplayName, bytesProcessedStatKey):
			result.Stats.BytesProcessed = model.MetricValue{
				Value: stat.Value,
				Unit:  stat.Unit,
			}
		case strings.Contains(stat.DisplayName, executionTimeStatKey):
			result.Stats.ExecutionTime = model.MetricValue{
				Value: stat.Value,
				Unit:  stat.Unit,
			}
		}
	}

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
								if showSampleValues {
									result.Stats.Fields[label] = fmt.Sprintf("%v", value)
								} else {
									result.Stats.Fields[label] = ""
								}
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
						if showLogLines {
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
