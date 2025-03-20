package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"github.com/stretchr/testify/assert"
)

func TestConvertToAlert(t *testing.T) {
	tests := []struct {
		name          string
		queries       []string
		rule          *definitions.ProvisionedAlertRule
		titles        string
		config        ConversionConfig
		wantQueryText string
		wantError     bool
	}{
		{
			name:    "valid new loki query",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule 1",
			rule: &definitions.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			config: ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			wantQueryText: "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantError:     false,
		},
		{
			name:    "valid ES query",
			queries: []string{`from * | where eventSource=="kms.amazonaws.com" and eventName=="CreateGrant"`},
			titles:  "Alert Rule 2",
			rule: &definitions.ProvisionedAlertRule{
				UID: "3bb06d82",
			},
			config: ConversionConfig{
				Name:       "conv",
				Target:     "esql",
				DataSource: "my_es_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			wantQueryText: `from * | where eventSource==\"kms.amazonaws.com\" and eventName==\"CreateGrant\"`,
			wantError:     false,
		},
		{
			name:    "invalid time window",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule 3",
			rule: &definitions.ProvisionedAlertRule{
				ID:  0,
				UID: "5c1c217a",
			},
			config: ConversionConfig{
				TimeWindow: "1y",
			},
			wantError: true,
		},
		{
			name:    "invalid time window",
			queries: []string{"{job=`.+`} | json | test=`true`", "sum(count_over_time({job=`.+`} | json | test=`false`[$__auto]))"},
			titles:  "Alert Rule 4 & Alert Rule 5",
			rule: &definitions.ProvisionedAlertRule{
				UID: "f4c34eae-c7c3-4891-8965-08a01e8286b8",
			},
			config: ConversionConfig{
				TimeWindow: "1y",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewIntegrator()
			err := i.ConvertToAlert(tt.rule, tt.queries, tt.titles, tt.config)
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, string(tt.rule.Data[0].Model), tt.wantQueryText)
				assert.Equal(t, tt.config.RuleGroup, tt.rule.RuleGroup)
				assert.Equal(t, tt.config.DataSource, tt.rule.Data[0].DatasourceUID)
				assert.Equal(t, tt.titles, tt.rule.Title)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		token      string
		added      string
		deleted    string
		modified   string
		expConfig  Configuration
		expAdd     []string
		expDel     []string
		wantError  bool
	}{
		{
			name:       "valid loki config, single added file",
			configPath: "testdata/config.yml",
			token:      "my-test-token",
			added:      "testdata/conv.json",
			deleted:    "",
			modified:   "",
			expConfig: Configuration{
				Folders: FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: ConversionConfig{
					Target:          "loki",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-logs",
				},
				Conversions: []ConversionConfig{
					{
						Name:       "conv",
						RuleGroup:  "Every 5 Minutes",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: IntegrationConfig{
					FolderID: "XXXX",
					OrgID:    1,
				},
			},
			expAdd:    []string{"testdata/conv.json"},
			expDel:    []string{},
			wantError: false,
		},
		{
			name:       "valid es config, multiple files added, changed and removed",
			configPath: "testdata/es-config.yml",
			token:      "my-test-token",
			added:      "testdata/conv1.json",
			deleted:    "testdata/conv2.json testdata/conv4.json",
			modified:   "testdata/conv3.json",
			expConfig: Configuration{
				Folders: FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: ConversionConfig{
					Target:          "esql",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-es-logs",
				},
				Conversions: []ConversionConfig{
					{
						Name:       "conv1",
						RuleGroup:  "Every 5 Minutes",
						TimeWindow: "5m",
					},
					{
						Name:       "conv2",
						RuleGroup:  "Every 10 Minutes",
						TimeWindow: "10m",
					},
					{
						Name:       "conv3",
						RuleGroup:  "Every 30 Minutes",
						TimeWindow: "30m",
					},
					{
						Name:       "conv4",
						RuleGroup:  "Every 20 Minutes",
						TimeWindow: "20m",
					},
				},
				IntegratorConfig: IntegrationConfig{
					FolderID: "XXXX",
					OrgID:    1,
				},
			},
			expAdd:    []string{"testdata/conv1.json", "testdata/conv3.json"},
			expDel:    []string{"testdata/conv2.json", "testdata/conv4.json"},
			wantError: false,
		},
		{
			name:       "missing config file",
			configPath: "testdata/missing_config.yml",
			wantError:  true,
		},
		{
			name:       "no path",
			configPath: "",
			wantError:  true,
		},
		{
			name:       "non-local config file",
			configPath: "../testdata/missing_config.yml",
			wantError:  true,
		},
		{
			name:       "conversion path is not local",
			configPath: "testdata/non-local-conv-config.yml",
			wantError:  true,
		},
		{
			name:       "deployment path is not local",
			configPath: "testdata/non-local-deploy-config.yml",
			wantError:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("INTEGRATOR_CONFIG_PATH", tt.configPath)
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", tt.token)
			os.Setenv("ADDED_FILES", tt.added)
			os.Setenv("DELETED_FILES", tt.deleted)
			os.Setenv("MODIFIED_FILES", tt.modified)

			i := NewIntegrator()
			err := i.LoadConfig()
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expConfig, i.config)
				assert.Equal(t, tt.expAdd, i.addedFiles)
				assert.Equal(t, tt.expDel, i.removedFiles)
			}
		})
	}
	defer os.Unsetenv("INTEGRATOR_CONFIG_PATH")
	defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")
	defer os.Unsetenv("ADDED_FILES")
	defer os.Unsetenv("DELETED_FILES")
	defer os.Unsetenv("MODIFIED_FILES")
}

func TestReadWriteAlertRule(t *testing.T) {
	// A simple test of reading and writing alert rule files
	rule := &definitions.ProvisionedAlertRule{}
	err := readRuleFromFile(rule, "testdata/sample_rule.json")
	assert.NoError(t, err)
	err = writeRuleToFile(rule, "testdata/sample_rule.json")
	assert.NoError(t, err)
}

func TestSummariseSigmaRules(t *testing.T) {
	tests := []struct {
		name      string
		rules     []SigmaRule
		wantID    uuid.UUID
		wantTitle string
		wantError bool
	}{
		{
			name: "valid rule",
			rules: []SigmaRule{
				{ID: "996f8884-9144-40e7-ac63-29090ccde9a0", Title: "Rule 1"},
			},
			wantID:    uuid.MustParse("996f8884-9144-40e7-ac63-29090ccde9a0"),
			wantTitle: "Rule 1",
			wantError: false,
		},
		{
			name:      "no rules",
			rules:     []SigmaRule{},
			wantError: true,
		},
		{
			name: "multiple rules",
			rules: []SigmaRule{
				{ID: "a6b097fd-44d2-413f-b5cd-0916e22e6d5c", Title: "Rule 1"},
				{ID: "37f6f301-ddba-496f-9a84-853886ffff6b", Title: "Rule 2"},
			},
			wantID:    uuid.MustParse("914664fc-9968-4850-af49-8c2e64d19237"),
			wantTitle: "Rule 1 & Rule 2",
			wantError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, title, err := summariseSigmaRules(tt.rules)
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantID, id)
				assert.Equal(t, id.Version(), uuid.Version(0x4))
				assert.Equal(t, tt.wantTitle, title)
			}
		})
	}
}

func TestIntegratorRun(t *testing.T) {
	tests := []struct {
		name           string
		conversionName string
		convOutput     []ConversionOutput
		wantQueries    []string
		wantTitles     []string
		wantError      bool
	}{
		{
			name:           "single rule single query",
			conversionName: "test_conv1",
			convOutput: []ConversionOutput{
				{
					Queries: []string{"{job=`test`} | json"},
					Rules: []SigmaRule{
						{
							ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
							Title: "Test Rule",
						},
					},
				},
			},
			wantQueries: []string{"sum(count_over_time({job=`test`} | json[$__auto]))"},
			wantTitles:  []string{"Test Rule"},
			wantError:   false,
		},
		{
			name:           "multiple rules multiple queries",
			conversionName: "test_conv2",
			convOutput: []ConversionOutput{
				{
					Queries: []string{
						"{job=`test1`} | json",
						"{job=`test2`} | json",
					},
					Rules: []SigmaRule{
						{
							ID:    "a6b097fd-44d2-413f-b5cd-0916e22e6d5c",
							Title: "Test Rule 1",
						},
						{
							ID:    "37f6f301-ddba-496f-9a84-853886ffff6b",
							Title: "Test Rule 2",
						},
					},
				},
			},
			wantQueries: []string{
				"sum(count_over_time({job=`test1`} | json[$__auto]))",
				"sum(count_over_time({job=`test2`} | json[$__auto]))",
			},
			wantTitles: []string{"Test Rule 1 & Test Rule 2"},
			wantError:  false,
		},
		{
			name:           "multiple conversion objects",
			conversionName: "test_conv3",
			convOutput: []ConversionOutput{
				{
					Queries: []string{"{job=`test1`} | json"},
					Rules: []SigmaRule{
						{
							ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
							Title: "Test Rule 1",
						},
					},
				},
				{
					Queries: []string{"{job=`test2`} | json"},
					Rules: []SigmaRule{
						{
							ID:    "37f6f301-ddba-496f-9a84-853886ffff6b",
							Title: "Test Rule 2",
						},
					},
				},
			},
			wantQueries: []string{
				"sum(count_over_time({job=`test1`} | json[$__auto]))",
				"sum(count_over_time({job=`test2`} | json[$__auto]))",
			},
			wantTitles: []string{"Test Rule 1", "Test Rule 2"},
			wantError:  false,
		},
		{
			name:           "no queries",
			conversionName: "test_conv4",
			convOutput: []ConversionOutput{
				{
					Queries: []string{},
					Rules: []SigmaRule{
						{
							ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
							Title: "Test Rule",
						},
					},
				},
			},
			wantError: false, // Should skip but not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_run", tt.name)
			err := os.MkdirAll(testDir, 0755)
			assert.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Create conversion and deployment subdirectories
			convPath := filepath.Join(testDir, "conv")
			deployPath := filepath.Join(testDir, "deploy")
			err = os.MkdirAll(convPath, 0755)
			assert.NoError(t, err)
			err = os.MkdirAll(deployPath, 0755)
			assert.NoError(t, err)

			// Create test configuration
			config := Configuration{
				Folders: FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: ConversionConfig{
					Target:     "loki",
					DataSource: "test-datasource",
				},
				Conversions: []ConversionConfig{
					{
						Name:       tt.conversionName,
						RuleGroup:  "Test Rules",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: IntegrationConfig{
					FolderID: "test-folder",
					OrgID:    1,
				},
			}

			// Create test conversion output file
			convBytes, err := json.Marshal(tt.convOutput)
			assert.NoError(t, err)
			convFile := filepath.Join(convPath, tt.conversionName+".json")
			err = os.WriteFile(convFile, convBytes, 0644)
			assert.NoError(t, err)

			// Set up integrator
			i := &Integrator{
				config:       config,
				addedFiles:   []string{convFile},
				removedFiles: []string{},
			}

			// Run integration
			err = i.Run()
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// For cases with no queries, just verify no files were created
			if len(tt.wantQueries) == 0 {
				files, err := os.ReadDir(deployPath)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(files))
				return
			}

			// Verify output files for each conversion object
			for idx, convObj := range tt.convOutput {
				if len(convObj.Queries) == 0 {
					continue
				}

				convID, _, err := summariseSigmaRules(convObj.Rules)
				assert.NoError(t, err)

				expectedFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_%s_%s.json", tt.conversionName, convID.String()))
				_, err = os.Stat(expectedFile)
				assert.NoError(t, err)

				// Verify file contents
				rule := &definitions.ProvisionedAlertRule{}
				err = readRuleFromFile(rule, expectedFile)
				assert.NoError(t, err)

				// Verify rule properties
				assert.Equal(t, convID.String(), rule.UID)
				assert.Equal(t, tt.wantTitles[idx], rule.Title)
				assert.Equal(t, "Test Rules", rule.RuleGroup)
				assert.Equal(t, "test-datasource", rule.Data[0].DatasourceUID)

				// Verify queries
				for qIdx, query := range convObj.Queries {
					assert.Contains(t, string(rule.Data[qIdx].Model), query)
				}
			}
		})
	}
}
