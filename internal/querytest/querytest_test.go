package querytest

import (
	"encoding/json"
	"fmt"
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
				ConversionDefaults: model.ConversionConfig{
					Target:     "loki",
					DataSource: "test-datasource",
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "test_conv",
						RuleGroup:  "Test Rules",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:                     "test-folder",
					OrgID:                        1,
					TestQueries:                  true,
					From:                         "now-1h",
					To:                           "now",
					ContinueOnQueryTestingErrors: tt.continueOnErrors,
				},
				DeployerConfig: model.DeploymentConfig{
					GrafanaInstance: "https://test.grafana.com",
					Timeout:         "5s",
				},
			}

			// Create conversion output files
			var testFiles []string
			for _, fileName := range tt.testFiles {
				convBytes, err := json.Marshal(tt.convOutput)
				assert.NoError(t, err)
				convFile := filepath.Join(convPath, fileName)
				err = os.WriteFile(convFile, convBytes, 0o600)
				assert.NoError(t, err)
				testFiles = append(testFiles, convFile)
			}

			// Create a temporary output file for capturing outputs
			outputFile, err := os.CreateTemp("", "github-output")
			assert.NoError(t, err)
			defer os.Remove(outputFile.Name())

			// Setup environment for the test
			os.Setenv("GITHUB_OUTPUT", outputFile.Name())
			defer os.Unsetenv("GITHUB_OUTPUT")

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

			// Verify test_query_results output was set if testing was performed
			if tt.expectTestResults && len(tt.convOutput.Queries) > 0 {
				outputBytes, err := os.ReadFile(outputFile.Name())
				assert.NoError(t, err)
				outputContent := string(outputBytes)
				assert.Contains(t, outputContent, "test_query_results=")
			}
		})
	}
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
	}`
	return []byte(mockResponse), nil
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
