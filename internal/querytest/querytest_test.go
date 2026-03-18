package querytest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/integrate"
	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name              string
		testFiles         []string
		convOutput        model.ConversionOutput
		continueOnErrors  bool
		wantError         bool
		expectTestResults bool
		mockQueryError    bool
	}{
		{
			name:      "successful query testing",
			testFiles: []string{"test_conv.json"},
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			continueOnErrors:  true,
			wantError:         false,
			expectTestResults: true,
			mockQueryError:    false,
		},
		{
			name:      "query error with continue enabled",
			testFiles: []string{"test_conv.json"},
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			continueOnErrors: true,
			wantError:        false,
			mockQueryError:   true,
		},
		{
			name:      "query error with continue disabled",
			testFiles: []string{"test_conv.json"},
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			continueOnErrors: false,
			wantError:        true,
			mockQueryError:   true,
		},
		{
			name:      "no queries to test",
			testFiles: []string{"test_conv_no_queries.json"},
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv",
				Queries:        []string{},
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			continueOnErrors: true,
			wantError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_do_query_testing", tt.name)
			err := os.MkdirAll(testDir, 0o755)
			assert.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Create conversion subdirectory
			convPath := filepath.Join(testDir, "conv")
			err = os.MkdirAll(convPath, 0o755)
			assert.NoError(t, err)

			// Create test configuration
			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
				},
				Defaults: model.ConfigBlock{
					Conversion: model.ConversionConfig{
						Target: "loki",
					},
					Integration: model.IntegrationConfig{
						DataSource:      "test-datasource",
						FolderID:        "test-folder",
						OrgID:           1,
						TestQueries:     true,
						From:            "now-1h",
						To:              "now",
						ContinueOnError: tt.continueOnErrors,
					},
					Deployment: model.DeploymentConfig{
						GrafanaInstance: "https://test.grafana.com",
						Timeout:         "5s",
					},
				},
				Configurations: []model.NamedConfigBlock{
					{
						Name: "test_conv",
						ConfigBlock: model.ConfigBlock{
							Integration: model.IntegrationConfig{
								RuleGroup:  "Test Rules",
								TimeWindow: "5m",
							},
						},
					},
				},
			}

			// Create conversion output files
			testFiles := make([]string, len(tt.testFiles))
			for i, fileName := range tt.testFiles {
				convBytes, err := json.Marshal(tt.convOutput)
				assert.NoError(t, err)
				convFile := filepath.Join(convPath, fileName)
				err = os.WriteFile(convFile, convBytes, 0o600)
				assert.NoError(t, err)
				testFiles[i] = convFile
			}

			// Create a temporary output file for capturing outputs
			outputFile, err := os.CreateTemp("", "github-output")
			assert.NoError(t, err)
			defer os.Remove(outputFile.Name())

			// Setup environment for the test
			os.Setenv("GITHUB_OUTPUT", outputFile.Name())
			defer os.Unsetenv("GITHUB_OUTPUT")

			resultsFile := filepath.Join(testDir, "test-query-results.json")
			os.Setenv("TEST_QUERY_RESULTS_FILE", resultsFile)
			defer os.Unsetenv("TEST_QUERY_RESULTS_FILE")

			// Create mock query executor
			var mockDatasourceQuery integrate.DatasourceQuery
			if tt.mockQueryError {
				mockWithErrors := newTestDatasourceQueryWithErrors()
				mockWithErrors.AddMockError("{job=`test`} | json", fmt.Errorf("query failed"))
				mockDatasourceQuery = mockWithErrors
			} else {
				mockDatasourceQuery = newTestDatasourceQuery()
			}

			// Save original executor and restore after test
			originalDatasourceQuery := integrate.DefaultDatasourceQuery
			integrate.DefaultDatasourceQuery = mockDatasourceQuery
			defer func() {
				integrate.DefaultDatasourceQuery = originalDatasourceQuery
			}()

			// Set environment variable for API token
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", "test-api-token")
			defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")

			// Create query tester and run
			timeoutDuration := 5 * time.Second
			queryTester := NewQueryTester(config, testFiles, timeoutDuration)
			err = queryTester.Run()

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify test_query_results_file output was set if testing was performed
			if tt.expectTestResults && len(tt.convOutput.Queries) > 0 {
				_, err = outputFile.Seek(0, 0)
				assert.NoError(t, err)
				outputBytes, err := io.ReadAll(outputFile)
				assert.NoError(t, err)
				outputContent := string(outputBytes)
				assert.Contains(t, outputContent, "test_query_results_file="+resultsFile)

				resultsBytes, err := os.ReadFile(resultsFile)
				assert.NoError(t, err)
				var results map[string][]model.QueryTestResult
				assert.NoError(t, json.Unmarshal(resultsBytes, &results))
				assert.NotEmpty(t, results)
			}
		})
	}
}

func TestTestQueriesPreservesExploreLinkOnError(t *testing.T) {
	// When the underlying datasource query fails (e.g., Grafana auth failure),
	// the returned QueryTestResult should still carry a usable explore link so
	// the PR comment links to a real Grafana URL instead of `[See in Explore]()`.
	config := model.Configuration{
		Defaults: model.ConfigBlock{
			Conversion: model.ConversionConfig{
				Target: "loki",
			},
			Integration: model.IntegrationConfig{
				DataSource: "test-datasource",
				OrgID:      1,
				From:       "now-1h",
				To:         "now",
			},
			Deployment: model.DeploymentConfig{
				GrafanaInstance: "https://test.grafana.com",
			},
		},
	}

	mock := newTestDatasourceQueryWithErrors()
	mock.AddMockError(`{job="test"}`, fmt.Errorf("401 unauthorized"))

	originalDatasourceQuery := integrate.DefaultDatasourceQuery
	integrate.DefaultDatasourceQuery = mock
	defer func() {
		integrate.DefaultDatasourceQuery = originalDatasourceQuery
	}()

	queryTester := NewQueryTester(config, nil, 5*time.Second)
	results, err := queryTester.TestQueries(
		map[string]string{"A0": `{job="test"}`},
		model.NamedConfigBlock{Name: "test_conv"},
		config.Defaults,
	)

	assert.Error(t, err)
	assert.Len(t, results, 1)
	assert.NotEmpty(t, results[0].Link, "explore link should be populated even on test error")
	assert.Contains(t, results[0].Link, "https://test.grafana.com/explore")
	assert.Len(t, results[0].Stats.Errors, 1)
}

func TestTestQueriesReportsCorrectCountEvenOnChangingDataframeStructure(t *testing.T) {
	// When the underlying datasource query returns a different dataframe structure
	// than the previous query, the QueryTestResult should still report the correct
	// number of results (old dataframe structure with lines vs new dataframe structure with body).
	config := model.Configuration{
		Defaults: model.ConfigBlock{
			Conversion: model.ConversionConfig{
				Target: "loki",
			},
			Integration: model.IntegrationConfig{
				DataSource: "test-datasource",
				OrgID:      1,
				From:       "now-1h",
				To:         "now",
			},
			Deployment: model.DeploymentConfig{
				GrafanaInstance: "https://test.grafana.com",
			},
		},
	}

	mock := newTestDatasourceQueryWithChangingStructure()
	mock.AddMockResponse(`{job="test"}`, `{
		"results": {
			"A": {
				"frames": [
					{
						"schema": {
							"fields": [
								{"name": "Time", "type": "time"},
								{"name": "Line", "type": "string"},
								{"name": "labels", "type": "other"}
							]
						},
						"data": {
							"values": [
								[1000000000, 2000000000],
								["error log line", "warning log line"],
								[
									{"job": "loki", "level": "error"},
									{"job": "loki", "level": "warning"}
								]
							]
						}
					}
				]
			}
		},
		"errors": []
	}`)
	mock.AddMockResponse(`{job="test"} | json`, `{
		"results": {
			"A": {
				"frames": [
					{
						"schema": {
							"fields": [
								{"name": "Time", "type": "time"},
								{"name": "body", "type": "string"},
								{"name": "labels", "type": "other"}
							]
						},
						"data": {
							"values": [
								[1000000000, 2000000000],
								["error log line", "warning log line"],
								[
									{"job": "loki", "level": "error"},
									{"job": "loki", "level": "warning"}
								]
							]
						}
					}
				]
			}
		},
		"errors": []
	}`)

	originalDatasourceQuery := integrate.DefaultDatasourceQuery
	integrate.DefaultDatasourceQuery = mock
	defer func() {
		integrate.DefaultDatasourceQuery = originalDatasourceQuery
	}()

	// Test first query with old dataframe structure
	queryTester := NewQueryTester(config, nil, 5*time.Second)
	results, err := queryTester.TestQueries(
		map[string]string{"A0": `{job="test"}`},
		model.NamedConfigBlock{Name: "test_conv"},
		config.Defaults,
	)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 2, results[0].Stats.Count, "total results should be 2 even with changing dataframe structure")

	// Test second query with new dataframe structure
	results, err = queryTester.TestQueries(
		map[string]string{"A0": `{job="test"} | json`},
		model.NamedConfigBlock{Name: "test_conv"},
		config.Defaults,
	)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 2, results[0].Stats.Count, "total results should be 2 even with changing dataframe structure")
}

func TestTestQueriesThrowsErrorWhenShowLogLinesIsTrueAndNoLogLines(t *testing.T) {
	// When the underlying datasource query returns no log lines (or they are on a different label (e.g. body)), but the
	// QueryTestResult is configured to show log lines, an error should be thrown.
	config := model.Configuration{
		Defaults: model.ConfigBlock{
			Conversion: model.ConversionConfig{
				Target: "loki",
			},
			Integration: model.IntegrationConfig{
				DataSource:   "test-datasource",
				OrgID:        1,
				From:         "now-1h",
				To:           "now",
				ShowLogLines: true,
			},
			Deployment: model.DeploymentConfig{
				GrafanaInstance: "https://test.grafana.com",
			},
		},
	}

	mock := newTestDatasourceQueryWithChangingStructure()
	mock.AddMockResponse(`{job="test"}`, `{
		"results": {
			"A": {
				"frames": [
					{
						"schema": {
							"fields": [
								{"name": "Time", "type": "time"},
								{"name": "somethingnew", "type": "string"},
								{"name": "labels", "type": "other"}
							]
						},
						"data": {
							"values": [
								[1000000000, 2000000000],
								["error log line", "warning log line"],
								[
									{"job": "loki", "level": "error"},
									{"job": "loki", "level": "warning"}
								]
							]
						}
					}
				]
			}
		},
		"errors": []
	}`)

	originalDatasourceQuery := integrate.DefaultDatasourceQuery
	integrate.DefaultDatasourceQuery = mock
	defer func() {
		integrate.DefaultDatasourceQuery = originalDatasourceQuery
	}()

	queryTester := NewQueryTester(config, nil, 5*time.Second)
	results, err := queryTester.TestQueries(
		map[string]string{"A0": `{job="test"}`},
		model.NamedConfigBlock{Name: "test_conv"},
		config.Defaults,
	)

	assert.ErrorContains(t, err, "Line")
	assert.ErrorContains(t, err, "body")
	assert.Empty(t, results)
}

// testDatasourceQuery is a mock implementation for testing
type testDatasourceQuery struct {
	queryLog      []string
	datasourceLog []string
}

func newTestDatasourceQuery() *testDatasourceQuery {
	return &testDatasourceQuery{
		queryLog:      make([]string, 0),
		datasourceLog: make([]string, 0),
	}
}

func (t *testDatasourceQuery) GetDatasource(dsName, _ string, _ string, _ time.Duration) (*integrate.GrafanaDatasource, error) {
	t.datasourceLog = append(t.datasourceLog, dsName)
	return &integrate.GrafanaDatasource{
		UID:  dsName,
		Type: "loki",
		ID:   1,
	}, nil
}

func (t *testDatasourceQuery) ExecuteQuery(query, dsName, _ string, _ string, _ string, _ string, _ string, _ string, _ time.Duration) ([]byte, error) {
	t.queryLog = append(t.queryLog, query)
	t.datasourceLog = append(t.datasourceLog, dsName)

	// Return a mock response with sample data
	mockResponse := `{
		"results": {
			"A": {
				"frames": [
					{
						"schema": {
							"fields": [
								{"name": "Time", "type": "time"},
								{"name": "body", "type": "string"},
								{"name": "labels", "type": "other"}
							]
						},
						"data": {
							"values": [
								[1000000000, 2000000000],
								["error log line", "warning log line"],
								[
									{"job": "loki", "level": "error"},
									{"job": "loki", "level": "warning"}
								]
							]
						}
					}
				]
			}
		},
		"errors": []
	}`
	return []byte(mockResponse), nil
}

// TestRunMultipleConfigurations validates per-configuration query testing settings:
// - one config with TestQueries enabled per-cfg and default time range
// - one config with TestQueries enabled, continue_on_error, and a custom from time that gets a query error
// - one config with TestQueries disabled per-cfg — its file is passed but testing must be skipped
// - one config with TestQueries enabled and ShowLogLines per-cfg
// - one config with TestQueries enabled and ShowSampleValues per-cfg
func TestRunMultipleConfigurations(t *testing.T) {
	testDir := filepath.Join("testdata", "test_multi_config")
	err := os.MkdirAll(testDir, 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll(testDir)

	convPath := filepath.Join(testDir, "conv")
	err = os.MkdirAll(convPath, 0o755)
	assert.NoError(t, err)

	// Write conversion output files
	writeConvFile := func(name string, queries []string) string {
		out := model.ConversionOutput{
			ConversionName: name,
			Queries:        queries,
			Rules:          []model.SigmaRule{{ID: "test-id", Title: "Test Rule"}},
		}
		data, _ := json.Marshal(out)
		path := filepath.Join(convPath, name+".json")
		assert.NoError(t, os.WriteFile(path, data, 0o600))
		return path
	}

	enabledFile := writeConvFile("conv_enabled", []string{"{job=`enabled`} | json"})
	continueFile := writeConvFile("conv_continue", []string{"{job=`continue`} | json"})
	noTestFile := writeConvFile("conv_no_test", []string{"{job=`notest`} | json"})
	showLinesFile := writeConvFile("conv_show_lines", []string{"{job=`showlines`} | json"})
	showValuesFile := writeConvFile("conv_show_values", []string{"{job=`showvalues`} | json"})

	config := model.Configuration{
		Defaults: model.ConfigBlock{
			Conversion: model.ConversionConfig{Target: "loki"},
			Integration: model.IntegrationConfig{
				DataSource: "default-ds",
				From:       "now-1h",
				To:         "now",
				// TestQueries intentionally not set in defaults — enabled per-cfg only
			},
			Deployment: model.DeploymentConfig{
				GrafanaInstance: "https://test.grafana.com",
			},
		},
		Configurations: []model.NamedConfigBlock{
			{
				Name: "conv_enabled",
				ConfigBlock: model.ConfigBlock{
					Integration: model.IntegrationConfig{
						DataSource:  "enabled-ds",
						RuleGroup:   "Enabled Alerts",
						TimeWindow:  "5m",
						TestQueries: true,
					},
					Deployment: model.DeploymentConfig{
						GrafanaInstance: "https://enabled.grafana.com",
					},
				},
			},
			{
				Name: "conv_continue",
				ConfigBlock: model.ConfigBlock{
					Integration: model.IntegrationConfig{
						DataSource:      "continue-ds",
						RuleGroup:       "Continue Alerts",
						TimeWindow:      "10m",
						From:            "now-6h",
						TestQueries:     true,
						ContinueOnError: true,
					},
				},
			},
			{
				Name: "conv_no_test",
				ConfigBlock: model.ConfigBlock{
					Integration: model.IntegrationConfig{
						DataSource:  "no-test-ds",
						RuleGroup:   "No Test Alerts",
						TestQueries: false,
					},
				},
			},
			{
				Name: "conv_show_lines",
				ConfigBlock: model.ConfigBlock{
					Integration: model.IntegrationConfig{
						DataSource:   "show-lines-ds",
						RuleGroup:    "Show Lines Alerts",
						TestQueries:  true,
						ShowLogLines: true,
					},
					Deployment: model.DeploymentConfig{
						Timeout: "30s",
					},
				},
			},
			{
				Name: "conv_show_values",
				ConfigBlock: model.ConfigBlock{
					Integration: model.IntegrationConfig{
						DataSource:       "show-values-ds",
						RuleGroup:        "Show Values Alerts",
						TestQueries:      true,
						ShowSampleValues: true,
					},
				},
			},
		},
	}

	// Set up GitHub output capture
	outputFile, err := os.CreateTemp("", "github-output")
	assert.NoError(t, err)
	defer os.Remove(outputFile.Name())
	os.Setenv("GITHUB_OUTPUT", outputFile.Name())
	defer os.Unsetenv("GITHUB_OUTPUT")

	resultsFile := filepath.Join(testDir, "test-query-results.json")
	os.Setenv("TEST_QUERY_RESULTS_FILE", resultsFile)
	defer os.Unsetenv("TEST_QUERY_RESULTS_FILE")

	os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", "test-api-token")
	defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")

	// Use a tracking mock that records (datasource, from) per call and injects an error for conv_continue
	mock := newTrackingDatasourceQuery()
	mock.AddMockError("{job=`continue`} | json", fmt.Errorf("simulated query error"))

	originalDatasourceQuery := integrate.DefaultDatasourceQuery
	integrate.DefaultDatasourceQuery = mock
	defer func() { integrate.DefaultDatasourceQuery = originalDatasourceQuery }()

	// Pass all files including conv_no_test — Run() must skip it based on TestQueries: false
	queryTester := NewQueryTester(config, []string{enabledFile, continueFile, noTestFile, showLinesFile, showValuesFile}, 5*time.Second)
	err = queryTester.Run()
	// Should succeed because conv_continue has ContinueOnError: true
	assert.NoError(t, err)

	defaultTimeout := 5 * time.Second

	// conv_enabled: per-cfg grafana instance, default timeout, default from
	assert.Contains(t, mock.calls, trackingCall{
		datasource:      "enabled-ds",
		from:            "now-1h",
		grafanaInstance: "https://enabled.grafana.com",
		timeout:         defaultTimeout,
	})

	// conv_continue: default grafana instance, default timeout, per-cfg from
	assert.Contains(t, mock.calls, trackingCall{
		datasource:      "continue-ds",
		from:            "now-6h",
		grafanaInstance: "https://test.grafana.com",
		timeout:         defaultTimeout,
	})

	// conv_show_lines: default grafana instance, per-cfg 30s timeout
	assert.Contains(t, mock.calls, trackingCall{
		datasource:      "show-lines-ds",
		from:            "now-1h",
		grafanaInstance: "https://test.grafana.com",
		timeout:         30 * time.Second,
	})

	// conv_no_test was never queried even though its file was passed
	for _, call := range mock.calls {
		assert.NotEqual(t, "no-test-ds", call.datasource, "conv_no_test should not have been queried")
	}

	// Parse the JSON results from the results file to validate ShowLogLines and ShowSampleValues
	_, err = outputFile.Seek(0, 0)
	assert.NoError(t, err)
	outputBytes, err := io.ReadAll(outputFile)
	assert.NoError(t, err)
	outputContent := string(outputBytes)
	assert.Contains(t, outputContent, "test_query_results_file="+resultsFile)

	resultsJSON, err := os.ReadFile(resultsFile)
	assert.NoError(t, err)

	var allResults map[string][]model.QueryTestResult
	assert.NoError(t, json.Unmarshal(resultsJSON, &allResults))

	// conv_enabled: ShowLogLines and ShowSampleValues are both false (defaults) —
	// label keys present but with empty values, no log line field
	enabledResults := allResults[enabledFile]
	assert.Len(t, enabledResults, 1)
	assert.NotContains(t, enabledResults[0].Stats.Fields, "body", "log line field should not be stored without ShowLogLines")
	for _, v := range enabledResults[0].Stats.Fields {
		assert.Empty(t, v, "label values should be empty without ShowSampleValues")
	}

	// conv_show_lines: ShowLogLines true — the log line field ("body" in the mock response) should be stored
	showLinesResults := allResults[showLinesFile]
	assert.Len(t, showLinesResults, 1)
	assert.Contains(t, showLinesResults[0].Stats.Fields, "body", "log line field should be stored with ShowLogLines enabled")
	assert.NotEmpty(t, showLinesResults[0].Stats.Fields["body"])

	// conv_show_values: ShowSampleValues true — label values should be non-empty
	showValuesResults := allResults[showValuesFile]
	assert.Len(t, showValuesResults, 1)
	assert.NotEmpty(t, showValuesResults[0].Stats.Fields, "label fields should be present with ShowSampleValues enabled")
	for _, v := range showValuesResults[0].Stats.Fields {
		assert.NotEmpty(t, v, "label values should be populated with ShowSampleValues enabled")
	}
}

// trackingCall records per-query invocation details for assertion
type trackingCall struct {
	datasource      string
	from            string
	grafanaInstance string
	timeout         time.Duration
}

// trackingDatasourceQuery records each ExecuteQuery call for assertion
type trackingDatasourceQuery struct {
	*testDatasourceQueryWithErrors
	calls []trackingCall
}

func newTrackingDatasourceQuery() *trackingDatasourceQuery {
	return &trackingDatasourceQuery{
		testDatasourceQueryWithErrors: newTestDatasourceQueryWithErrors(),
		calls:                         make([]trackingCall, 0),
	}
}

func (t *trackingDatasourceQuery) ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel string, timeout time.Duration) ([]byte, error) {
	t.calls = append(t.calls, trackingCall{datasource: dsName, from: from, grafanaInstance: baseURL, timeout: timeout})
	return t.testDatasourceQueryWithErrors.ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel, timeout)
}

// testDatasourceQueryWithErrors supports error injection for testing continue_on_query_testing_errors
type testDatasourceQueryWithErrors struct {
	*testDatasourceQuery
	mockErrors map[string]error
}

func newTestDatasourceQueryWithErrors() *testDatasourceQueryWithErrors {
	return &testDatasourceQueryWithErrors{
		testDatasourceQuery: newTestDatasourceQuery(),
		mockErrors:          make(map[string]error),
	}
}

func (t *testDatasourceQueryWithErrors) AddMockError(query string, err error) {
	t.mockErrors[query] = err
}

func (t *testDatasourceQueryWithErrors) ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel string, timeout time.Duration) ([]byte, error) {
	// Check if we should return an error for this query
	if err, exists := t.mockErrors[query]; exists {
		return nil, err
	}

	// Otherwise use the parent implementation
	return t.testDatasourceQuery.ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel, timeout)
}

// testDatasourceQueryWithChangingStructure supports simulating changing dataframe structures for testing
type testDatasourceQueryWithChangingStructure struct {
	*testDatasourceQuery
	mockResponses map[string]string
}

func newTestDatasourceQueryWithChangingStructure() *testDatasourceQueryWithChangingStructure {
	return &testDatasourceQueryWithChangingStructure{
		testDatasourceQuery: newTestDatasourceQuery(),
		mockResponses:       make(map[string]string),
	}
}

func (t *testDatasourceQueryWithChangingStructure) AddMockResponse(query string, response string) {
	t.mockResponses[query] = response
}

func (t *testDatasourceQueryWithChangingStructure) ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel string, timeout time.Duration) ([]byte, error) {
	// Check if we have a mock response for this query
	if response, exists := t.mockResponses[query]; exists {
		return []byte(response), nil
	}

	// Otherwise use the parent implementation
	return t.testDatasourceQuery.ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel, timeout)
}
