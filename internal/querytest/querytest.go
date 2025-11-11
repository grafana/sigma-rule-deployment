package querytest

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
			if !qt.config.IntegratorConfig.ContinueOnQueryTestingErrors {
				return err
			}
			continue
		}

		var conversionObject model.ConversionOutput
		err = json.Unmarshal([]byte(conversionContent), &conversionObject)
		if err != nil {
			fmt.Printf("Error unmarshalling conversion output for file %s: %v\n", inputFile, err)
			if !qt.config.IntegratorConfig.ContinueOnQueryTestingErrors {
				return fmt.Errorf("error unmarshalling conversion output: %v", err)
			}
			continue
		}

		// Find matching configuration using ConversionName
		var config model.ConversionConfig
		for _, conf := range qt.config.Conversions {
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
			queryMap, config, qt.config.ConversionDefaults,
		)
		if err != nil {
			fmt.Printf("Error testing queries for file %s: %v\n", inputFile, err)
			// Return error if continue on query testing errors is not enabled
			if !qt.config.IntegratorConfig.ContinueOnQueryTestingErrors {
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
				fmt.Printf("Query returned results: %d\n", queryResults[0].Stats.Count)
			} else {
				fmt.Printf("Queries returned results:\n")
				for i, result := range queryResults {
					fmt.Printf("Query %d: %d\n", i, result.Stats.Count)
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
func (qt *QueryTester) TestQueries(queries map[string]string, config, defaultConf model.ConversionConfig) ([]model.QueryTestResult, error) {
	queryResults := make([]model.QueryTestResult, 0, len(queries))
	datasource := shared.GetConfigValue(config.DataSource, defaultConf.DataSource, "")
	// Determine datasource type using the same logic as createAlertQuery
	datasourceType := shared.GetConfigValue(
		config.DataSourceType,
		defaultConf.DataSourceType,
		shared.GetConfigValue(config.Target, defaultConf.Target, shared.Loki),
	)
	customModel := shared.GetConfigValue(config.QueryModel, defaultConf.QueryModel, "")

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
			qt.config.DeployerConfig.GrafanaInstance,
			os.Getenv("INTEGRATOR_GRAFANA_SA_TOKEN"),
			refID,
			qt.config.IntegratorConfig.From,
			qt.config.IntegratorConfig.To,
			customModel,
			qt.timeout,
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
		exploreLink, err := GenerateExploreLink(
			query, datasource, datasourceType, config, defaultConf,
			qt.config.DeployerConfig.GrafanaInstance,
			qt.config.IntegratorConfig.From,
			qt.config.IntegratorConfig.To,
			qt.config.IntegratorConfig.OrgID,
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
					qt.config.IntegratorConfig.ShowSampleValues,
					qt.config.IntegratorConfig.ShowLogLines,
				); err != nil {
					return nil, fmt.Errorf("error processing frame: %v", err)
				}
			}
		}

		queryResults = append(queryResults, result)
	}

	return queryResults, nil
}

// ProcessFrame processes a single frame from the query response and updates the result stats
func ProcessFrame(frame model.Frame, result *model.QueryTestResult, showSampleValues, showLogLines bool) error {
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
