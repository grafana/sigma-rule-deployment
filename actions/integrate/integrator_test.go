package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"github.com/stretchr/testify/assert"
)

func TestConvertToAlert(t *testing.T) {
	tests := []struct {
		name             string
		queries          []string
		rule             *definitions.ProvisionedAlertRule
		titles           string
		convConfig       ConversionConfig
		integratorConfig IntegrationConfig
		convObject       ConversionOutput
		wantQueryText    string
		wantDuration     definitions.Duration
		wantUnchanged    bool
		wantError        bool
		wantLabels       map[string]string
		wantAnnotations  map[string]string
	}{
		{
			name:    "valid new loki query",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule 1",
			rule: &definitions.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			wantQueryText: "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantDuration:  definitions.Duration(300 * time.Second),
			wantError:     false,
		},
		{
			name:    "valid ES query",
			queries: []string{`from * | where eventSource=="kms.amazonaws.com" and eventName=="CreateGrant"`},
			titles:  "Alert Rule 2",
			rule: &definitions.ProvisionedAlertRule{
				UID: "3bb06d82",
			},
			convConfig: ConversionConfig{
				Name:           "conv",
				Target:         "esql",
				DataSource:     "my_es_data_source",
				RuleGroup:      "Every 5 Minutes",
				TimeWindow:     "5m",
				DataSourceType: "elasticsearch",
			},
			wantQueryText: `from * | where eventSource==\"kms.amazonaws.com\" and eventName==\"CreateGrant\"`,
			wantDuration:  definitions.Duration(300 * time.Second),
			wantError:     false,
		},
		{
			name:    "invalid time window",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule 3",
			rule: &definitions.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: ConversionConfig{
				TimeWindow: "1y",
			},
			wantDuration: 0, // invalid time window, expect no value
			wantError:    true,
		},
		{
			name:    "invalid time window",
			queries: []string{"{job=`.+`} | json | test=`true`", "sum(count_over_time({job=`.+`} | json | test=`false`[$__auto]))"},
			titles:  "Alert Rule 4 & Alert Rule 5",
			rule: &definitions.ProvisionedAlertRule{
				UID: "f4c34eae-c7c3-4891-8965-08a01e8286b8",
			},
			convConfig: ConversionConfig{
				TimeWindow: "1y",
			},
			wantDuration: 0, // invalid time window, expect no value
			wantError:    true,
		},
		{
			name:    "skip unchanged queries",
			queries: []string{`{job=".+"} | json | test="true"`},
			titles:  "New Alert Rule Title", // This should be ignored
			rule: &definitions.ProvisionedAlertRule{
				UID:   "5c1c217a",
				Title: "Unchanged Alert Rule",
				Data: []definitions.AlertQuery{
					{
						Model: json.RawMessage(`{"refId":"A0","datasource":{"type":"loki","uid":"nil"},"hide":false,"expr":"sum(count_over_time({job=\".+\"} | json | test=\"true\"[$__auto]))","queryType":"instant","editorMode":"code"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"B","hide":false,"type":"reduce","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[],"type":"gt"},"operator":{"type":"and"},"query":{"params":["B"]},"reducer":{"params":[],"type":"last"}}],"reducer":"last","expression":"A0"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[1],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`),
					},
				},
			},
			wantUnchanged: true,
		},
		{
			name:    "process changed queries",
			queries: []string{`{job=".+"} | json | test="true"`},
			titles:  "New Alert Rule Title", // This should *not* be ignored
			convConfig: ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every Minute",
				TimeWindow: "1m",
			},
			rule: &definitions.ProvisionedAlertRule{
				UID:   "5c1c217a",
				Title: "Unchanged Alert Rule",
				Data: []definitions.AlertQuery{
					{
						// old query, which doesn't match the new query
						Model: json.RawMessage(`{"refId":"A0","datasource":{"type":"loki","uid":"nil"},"hide":false,"expr":"sum(count_over_time({old_job=\".+\"} | logfmt | test=\"old_query\"[$__auto]))","queryType":"instant","editorMode":"code"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"B","hide":false,"type":"reduce","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[],"type":"gt"},"operator":{"type":"and"},"query":{"params":["B"]},"reducer":{"params":[],"type":"last"}}],"reducer":"last","expression":"A0"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[1],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`),
					},
				},
			},
			wantDuration:  definitions.Duration(1 * time.Minute),
			wantUnchanged: false,
		},
		{
			name:    "valid query with a custom query model",
			queries: []string{"DO MY QUERY"},
			titles:  "Alert Rule 7",
			rule: &definitions.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: ConversionConfig{
				Name:       "conv",
				Target:     "custom",
				DataSource: "my_custom_data_source",
				RuleGroup:  "Every Hour",
				TimeWindow: "1h",
				QueryModel: `{"refId":"%s","datasource":{"type":"custom","uid":"%s"},"queryString":"(%s)"}`,
			},
			wantQueryText: "(DO MY QUERY)",
			wantDuration:  definitions.Duration(1 * time.Hour),
			wantError:     false,
		},
		{
			name:    "valid query with a generic query model",
			queries: []string{"DO MY QUERY"},
			titles:  "Alert Rule 8",
			rule: &definitions.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: ConversionConfig{
				Name:       "conv",
				Target:     "generic",
				DataSource: "generic_uid",
				RuleGroup:  "Every 30 Minutes",
				TimeWindow: "30m",
			},
			wantQueryText: `"DO MY QUERY"`,
			wantDuration:  definitions.Duration(30 * time.Minute),
			wantError:     false,
		},
		{
			name:    "valid query with lookback",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule with Lookback",
			rule: &definitions.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
				Lookback:   "2m",
			},
			wantQueryText: "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantDuration:  definitions.Duration(7 * time.Minute), // 5m + 2m lookback = 7m
			wantError:     false,
		},
		{
			name:    "template annotations and labels",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Template Rule",
			rule: &definitions.ProvisionedAlertRule{
				UID: "",
			},
			convObject: ConversionOutput{
				Rules: []SigmaRule{
					{
						Level:     "high",
						Logsource: SigmaLogsource{Product: "okta", Service: "okta"},
						Author:    "John Doe",
					},
				},
			},
			convConfig: ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			integratorConfig: IntegrationConfig{
				TemplateLabels: map[string]string{
					"Level":   "{{.Level}}",
					"Product": "{{.Logsource.Product}}",
					"Service": "{{.Logsource.Service}}",
				},
				TemplateAnnotations: map[string]string{
					"Author": "{{.Author}}",
				},
			},
			wantQueryText: "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantDuration:  definitions.Duration(300 * time.Second),
			wantError:     false,
			wantLabels: map[string]string{
				"Level":   "high",
				"Product": "okta",
				"Service": "okta",
			},
			wantAnnotations: map[string]string{
				"Author":         "John Doe",
				"ConversionFile": "test_conversion_file.json",
				"LogSourceType":  "loki",
				"LogSourceUid":   "my_data_source",
				"Lookback":       "0s",
				"Query":          "{job=`.+`} | json | test=`true`",
				"TimeWindow":     "5m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewIntegrator()
			i.config.IntegratorConfig = tt.integratorConfig
			err := i.ConvertToAlert(tt.rule, tt.queries, tt.titles, tt.convConfig, "test_conversion_file.json", tt.convObject)
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantUnchanged {
					// The rule should not be changed as the generated alert rule was identical
					assert.NotEqual(t, tt.titles, tt.rule.Title)
				} else {
					assert.Contains(t, string(tt.rule.Data[0].Model), tt.wantQueryText)
					assert.Equal(t, tt.wantDuration, tt.rule.Data[0].RelativeTimeRange.From)
					assert.Equal(t, tt.convConfig.RuleGroup, tt.rule.RuleGroup)
					assert.Equal(t, tt.convConfig.DataSource, tt.rule.Data[0].DatasourceUID)
					assert.Equal(t, tt.titles, tt.rule.Title)

					if tt.convConfig.Lookback != "" {
						lookbackDuration, err := time.ParseDuration(tt.convConfig.Lookback)
						assert.NoError(t, err)
						expectedTo := definitions.Duration(lookbackDuration)
						assert.Equal(t, tt.wantDuration, tt.rule.Data[0].RelativeTimeRange.From, "From should match expected duration (time window + lookback)")
						assert.Equal(t, expectedTo, tt.rule.Data[0].RelativeTimeRange.To, "To should be lookback duration")
					} else {
						assert.Equal(t, definitions.Duration(0), tt.rule.Data[0].RelativeTimeRange.To, "To should be 0 when no lookback")
					}
					if tt.wantLabels != nil {
						assert.Equal(t, tt.wantLabels, tt.rule.Labels)
					}
					if tt.wantAnnotations != nil {
						assert.Equal(t, tt.wantAnnotations, tt.rule.Annotations)
					}
				}
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		token      string
		changed    string
		deleted    string
		allRules   bool
		expConfig  Configuration
		expAdd     []string
		expDel     []string
		wantError  bool
	}{
		{
			name:       "valid loki config, single added file",
			configPath: "testdata/config.yml",
			token:      "my-test-token",
			changed:    "testdata/conv.json",
			deleted:    "",
			allRules:   false,
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
					From:     "now-1h",
					To:       "now",
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
			changed:    "testdata/conv1.json testdata/conv3.json",
			deleted:    "testdata/conv2.json testdata/conv4.json",
			allRules:   false,
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
					DataSourceType:  "elasticsearch",
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
					From:     "now-1h",
					To:       "now",
				},
			},
			expAdd:    []string{"testdata/conv1.json", "testdata/conv3.json"},
			expDel:    []string{"testdata/conv2.json", "testdata/conv4.json"},
			wantError: false,
		},
		{
			name:       "load all files when ALL_RULES is true",
			configPath: "testdata/config.yml",
			token:      "my-test-token",
			changed:    "",
			deleted:    "",
			allRules:   true,
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
					From:     "now-1h",
					To:       "now",
				},
			},
			expAdd:    []string{"testdata/config.yml", "testdata/es-config.yml", "testdata/non-local-conv-config.yml", "testdata/non-local-deploy-config.yml", "testdata/sample_rule.json"},
			expDel:    []string{},
			wantError: false,
		},
		{
			name:       "missing config file",
			configPath: "testdata/missing_config.yml",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "no path",
			configPath: "",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "non-local config file",
			configPath: "../testdata/missing_config.yml",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "conversion path is not local",
			configPath: "testdata/non-local-conv-config.yml",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "deployment path is not local",
			configPath: "testdata/non-local-deploy-config.yml",
			allRules:   false,
			wantError:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("INTEGRATOR_CONFIG_PATH", tt.configPath)
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", tt.token)
			os.Setenv("CHANGED_FILES", tt.changed)
			os.Setenv("DELETED_FILES", tt.deleted)
			if tt.allRules {
				os.Setenv("ALL_RULES", "true")
			} else {
				os.Setenv("ALL_RULES", "false")
			}

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
	defer os.Unsetenv("CHANGED_FILES")
	defer os.Unsetenv("DELETED_FILES")
	defer os.Unsetenv("ALL_RULES")
}

func TestReadWriteAlertRule(t *testing.T) {
	// A simple test of reading and writing alert rule files
	rule := &definitions.ProvisionedAlertRule{}
	err := readRuleFromFile(rule, "testdata/sample_rule.json")
	assert.NoError(t, err)
	err = writeRuleToFile(rule, "testdata/sample_rule.json", false)
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
		name                string
		conversionName      string
		convOutput          ConversionOutput
		wantQueries         []string
		wantTitles          string
		removedFiles        []string
		wantError           bool
		wantAnnotations     map[string]string
		wantOrphanedCleanup bool
	}{
		{
			name:           "single rule single query",
			conversionName: "test_conv1",
			convOutput: ConversionOutput{
				ConversionName: "test_conv1",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			wantQueries:  []string{"sum(count_over_time({job=`test`} | json[$__auto]))"},
			wantTitles:   "Test Rule",
			removedFiles: []string{},
			wantError:    false,
		},
		{
			name:           "multiple rules multiple queries",
			conversionName: "test_conv2",
			convOutput: ConversionOutput{
				ConversionName: "test_conv2",
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
			wantQueries: []string{
				"sum(count_over_time({job=`test1`} | json[$__auto]))",
				"sum(count_over_time({job=`test2`} | json[$__auto]))",
			},
			wantTitles:   "Test Rule 1 & Test Rule 2",
			removedFiles: []string{},
			wantError:    false,
		},
		{
			name:           "no queries",
			conversionName: "test_conv4",
			convOutput: ConversionOutput{
				ConversionName: "test_conv4",
				Queries:        []string{},
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			wantQueries:  []string{},
			removedFiles: []string{},
			wantError:    false,
		},
		{
			name:           "remove existing alert rule",
			conversionName: "test_conv5",
			convOutput: ConversionOutput{
				ConversionName: "test_conv5",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			wantQueries:  []string{"sum(count_over_time({job=`test`} | json[$__auto]))"},
			wantTitles:   "Test Rule",
			removedFiles: []string{"testdata/test_conv5.json"},
			wantError:    false,
		},
		{
			name:           "verify annotations are added",
			conversionName: "test_annotations",
			convOutput: ConversionOutput{
				ConversionName: "test_annotations",
				Queries:        []string{"{job=`test`} | json", "{service=`api`} | json"},
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Annotations Rule",
					},
				},
			},
			wantQueries:  []string{"sum(count_over_time({job=`test`} | json[$__auto]))", "sum(count_over_time({service=`api`} | json[$__auto]))"},
			wantTitles:   "Test Annotations Rule",
			removedFiles: []string{},
			wantError:    false,
			wantAnnotations: map[string]string{
				"Query":          "{job=`test`} | json",
				"TimeWindow":     "5m",
				"LogSourceUid":   "test-datasource",
				"LogSourceType":  "loki",
				"Lookback":       "2m",
				"ConversionFile": "test_annotations.json",
			},
		},
		{
			name:           "cleanup orphaned files",
			conversionName: "orphaned_test",
			convOutput: ConversionOutput{
				ConversionName: "orphaned_test",
				Queries:        []string{"{job=`orphaned`} | json"},
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Orphaned Test Rule",
					},
				},
			},
			wantQueries:         []string{},
			wantTitles:          "",
			removedFiles:        []string{},
			wantOrphanedCleanup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_run", tt.name)
			err := os.MkdirAll(testDir, 0o755)
			assert.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Set up the github output file
			oldGithubOutput := os.Getenv("GITHUB_OUTPUT")
			os.Setenv("GITHUB_OUTPUT", filepath.Join(testDir, "github-output"))
			defer os.Setenv("GITHUB_OUTPUT", oldGithubOutput)

			// Create conversion and deployment subdirectories
			convPath := filepath.Join(testDir, "conv")
			deployPath := filepath.Join(testDir, "deploy")
			err = os.MkdirAll(convPath, 0o755)
			assert.NoError(t, err)
			err = os.MkdirAll(deployPath, 0o755)
			assert.NoError(t, err)

			// Create test configuration
			conversions := []ConversionConfig{}

			// For orphaned cleanup test cases, don't include the conversion in config
			if !tt.wantOrphanedCleanup {
				conversions = []ConversionConfig{
					{
						Name:       tt.conversionName,
						RuleGroup:  "Test Rules",
						TimeWindow: "5m",
						Lookback:   "2m",
					},
				}
			}

			config := Configuration{
				Folders: FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: ConversionConfig{
					Target:     "loki",
					DataSource: "test-datasource",
				},
				Conversions: conversions,
				IntegratorConfig: IntegrationConfig{
					FolderID: "test-folder",
					OrgID:    1,
				},
			}

			// Create test conversion output file
			convBytes, err := json.Marshal(tt.convOutput)
			assert.NoError(t, err)
			convFile := filepath.Join(convPath, tt.conversionName+".json")
			err = os.WriteFile(convFile, convBytes, 0o600)
			assert.NoError(t, err)

			// For orphaned cleanup test, create a deployment file that references a missing conversion file
			if tt.wantOrphanedCleanup {
				convID, _, err := summariseSigmaRules(tt.convOutput.Rules)
				assert.NoError(t, err)
				ruleUID := getRuleUID(tt.conversionName, convID)
				deployFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_%s_%s.json", tt.conversionName, ruleUID))

				// Create a deployment file that references a non-existent conversion file
				dummyRule := &definitions.ProvisionedAlertRule{
					UID:       ruleUID,
					Title:     tt.wantTitles,
					RuleGroup: "Test Rules",
					Annotations: map[string]string{
						"ConversionFile": "/path/to/non/existent/conversion.json", // References missing file
					},
				}
				err = writeRuleToFile(dummyRule, deployFile, false)
				assert.NoError(t, err)
			}

			// For the remove test case, create a deployment file that should be removed
			if len(tt.removedFiles) > 0 {
				convID, _, err := summariseSigmaRules(tt.convOutput.Rules)
				assert.NoError(t, err)
				ruleUID := getRuleUID(tt.conversionName, convID)
				deployFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_%s_%s_%s.json", tt.conversionName, tt.conversionName, ruleUID))

				// Create a dummy alert rule file
				dummyRule := &definitions.ProvisionedAlertRule{
					UID:       ruleUID,
					Title:     tt.wantTitles,
					RuleGroup: "Test Rules",
				}
				err = writeRuleToFile(dummyRule, deployFile, false)
				assert.NoError(t, err)
			}

			// Set up integrator
			i := &Integrator{
				config:       config,
				addedFiles:   []string{convFile},
				removedFiles: tt.removedFiles,
			}

			// Run integration
			err = i.Run()
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// For orphaned cleanup test cases, verify files were cleaned up
			if tt.wantOrphanedCleanup {
				// Check that conversion file was cleaned up
				convFile := filepath.Join(convPath, tt.conversionName+".json")
				_, err = os.Stat(convFile)
				assert.True(t, os.IsNotExist(err), "Expected orphaned conversion file to be deleted but it still exists")

				// Check that deployment file was also cleaned up
				convID, _, err := summariseSigmaRules(tt.convOutput.Rules)
				assert.NoError(t, err)
				ruleUID := getRuleUID(tt.conversionName, convID)
				deployFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_%s_%s.json", tt.conversionName, ruleUID))
				_, err = os.Stat(deployFile)
				assert.True(t, os.IsNotExist(err), "Expected orphaned deployment file to be deleted but it still exists")
				return
			}

			// For cases with no queries, just verify no files were created
			if len(tt.wantQueries) == 0 {
				files, err := os.ReadDir(deployPath)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(files))
				return
			}

			// Verify output file
			convID, _, err := summariseSigmaRules(tt.convOutput.Rules)
			assert.NoError(t, err)

			ruleUID := getRuleUID(tt.conversionName, convID)
			expectedFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_%s_%s_%s.json", tt.conversionName, tt.conversionName, ruleUID))

			// For removed files, verify the file was deleted
			if len(tt.removedFiles) > 0 {
				_, err = os.Stat(expectedFile)
				assert.True(t, os.IsNotExist(err), "Expected file to be deleted but it still exists")
				return
			}

			// For added files, verify the file exists and has correct content
			_, err = os.Stat(expectedFile)
			assert.NoError(t, err)

			// Verify file contents
			rule := &definitions.ProvisionedAlertRule{}
			err = readRuleFromFile(rule, expectedFile)
			assert.NoError(t, err)

			// Verify rule properties
			assert.Equal(t, ruleUID, rule.UID)
			assert.Equal(t, tt.wantTitles, rule.Title)
			assert.Equal(t, "Test Rules", rule.RuleGroup)
			assert.Equal(t, "test-datasource", rule.Data[0].DatasourceUID)

			// Verify annotations if this test expects them
			if tt.wantAnnotations != nil {
				assert.NotNil(t, rule.Annotations, "Annotations should be present")
				for key, expectedValue := range tt.wantAnnotations {
					if key == "ConversionFile" {
						// ConversionFile contains the full path, so just check it contains the filename
						assert.Contains(t, rule.Annotations[key], expectedValue, "ConversionFile should contain the conversion file path")
					} else {
						assert.Equal(t, expectedValue, rule.Annotations[key], "Annotation %s should match expected value", key)
					}
				}
			}

			// Verify queries
			for qIdx, query := range tt.convOutput.Queries {
				assert.Contains(t, string(rule.Data[qIdx].Model), query)
			}
		})
	}
}

// testQueryExecutor is a test-specific implementation that allows mocking query results
type testDatasourceQuery struct {
	mockResponses map[string][]byte
	queryLog      []string
	datasourceLog []string
}

func newTestDatasourceQuery() *testDatasourceQuery {
	return &testDatasourceQuery{
		mockResponses: map[string][]byte{},
		queryLog:      []string{},
		datasourceLog: []string{},
	}
}

func (t *testDatasourceQuery) AddMockResponse(query string, response []byte) {
	t.mockResponses[query] = response
}

func (t *testDatasourceQuery) GetDatasource(dsName, _, _ string, _ time.Duration) (*GrafanaDatasource, error) {
	t.datasourceLog = append(t.datasourceLog, dsName)

	// For tests, always return a consistent datasource
	return &GrafanaDatasource{
		UID:  "test-uid",
		Type: "loki",
		Name: dsName,
	}, nil
}

func (t *testDatasourceQuery) ExecuteQuery(query, dsName, _, _, _, _, _, _ string, _ time.Duration) ([]byte, error) {
	t.queryLog = append(t.queryLog, query)
	t.datasourceLog = append(t.datasourceLog, dsName)

	// Return the mock response if it exists
	if resp, ok := t.mockResponses[query]; ok {
		return resp, nil
	}

	// Return a default mock response
	return []byte(`{"results":{"A":{"frames":[{"schema":{"fields":[{"name":"Time","type":"time"},{"name":"Line","type":"string"}]},"data":{"values":[[1625126400000,1625126460000],["mocked log line","another mocked log"]]}}]}}}`), nil
}

func TestIntegratorWithQueryTesting(t *testing.T) {
	tests := []struct {
		name         string
		showLogLines bool
		wantLine     bool
	}{
		{
			name:         "with log lines",
			showLogLines: true,
			wantLine:     true,
		},
		{
			name:         "without log lines",
			showLogLines: false,
			wantLine:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test queries
			testQueries := []string{
				"{job=\"loki\"} |= \"error\"",
				"{job=\"loki\"} |= \"warning\"",
			}

			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_query", tt.name)
			err := os.MkdirAll(testDir, 0o755)
			assert.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Create conversion and deployment subdirectories
			convPath := filepath.Join(testDir, "conv")
			deployPath := filepath.Join(testDir, "deploy")
			err = os.MkdirAll(convPath, 0o755)
			assert.NoError(t, err)
			err = os.MkdirAll(deployPath, 0o755)
			assert.NoError(t, err)

			// Create test conversion output
			convOutput := ConversionOutput{
				Queries:        testQueries,
				ConversionName: "test_loki",
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Loki Rule",
					},
				},
			}

			// Create test configuration with query testing enabled
			config := Configuration{
				Folders: FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: ConversionConfig{
					Target:     "loki",
					DataSource: "test-loki-datasource",
				},
				Conversions: []ConversionConfig{
					{
						Name:       "test_loki",
						RuleGroup:  "Loki Test Rules",
						TimeWindow: "5m",
						DataSource: "test-loki-datasource",
					},
				},
				IntegratorConfig: IntegrationConfig{
					FolderID:     "test-folder",
					OrgID:        1,
					TestQueries:  true,
					From:         "now-1h",
					To:           "now",
					ShowLogLines: tt.showLogLines,
				},
				DeployerConfig: DeploymentConfig{
					GrafanaInstance: "https://test.grafana.com",
					Timeout:         "5s",
				},
			}

			// Create test conversion output file
			convBytes, err := json.Marshal(convOutput)
			assert.NoError(t, err)
			convFile := filepath.Join(convPath, "test_loki_test_file_1.json")
			err = os.WriteFile(convFile, convBytes, 0o600)
			assert.NoError(t, err)

			// Create mock query executor
			mockDatasourceQuery := newTestDatasourceQuery()

			// Add mock responses for our test queries
			mockDatasourceQuery.AddMockResponse("{job=\"loki\"} |= \"error\"", []byte(`{
				"results": {
					"A": {
						"frames": [{
							"schema": {
								"fields": [
									{"name": "Time", "type": "time"},
									{"name": "Line", "type": "string"},
									{"name": "labels", "type": "other"}
								]
							},
							"data": {
								"values": [
									[1625126400000, 1625126460000],
									["error log line", "another error log"],
									[{"job": "loki", "level": "error"}]
								]
							}
						}]
					}
				}
			}`))

			mockDatasourceQuery.AddMockResponse("{job=\"loki\"} |= \"warning\"", []byte(`{
				"results": {
					"A": {
						"frames": [{
							"schema": {
								"fields": [
									{"name": "Time", "type": "time"},
									{"name": "Line", "type": "string"},
									{"name": "labels", "type": "other"}
								]
							},
							"data": {
								"values": [
									[1625126400000, 1625126460000],
									["warning log line", "another warning log"],
									[{"job": "loki", "level": "warning"}]
								]
							}
						}]
					}
				}
			}`))

			// Create a temporary output file for capturing outputs
			outputFile, err := os.CreateTemp("", "github-output")
			assert.NoError(t, err)
			defer os.Remove(outputFile.Name())

			// Setup environment for the test
			os.Setenv("GITHUB_OUTPUT", outputFile.Name())
			defer os.Unsetenv("GITHUB_OUTPUT")

			// Set up integrator
			integrator := &Integrator{
				config:       config,
				addedFiles:   []string{convFile},
				removedFiles: []string{},
			}

			// Save original executor and restore after test
			originalDatasourceQuery := DefaultDatasourceQuery
			DefaultDatasourceQuery = mockDatasourceQuery
			defer func() {
				DefaultDatasourceQuery = originalDatasourceQuery
			}()

			// Set environment variable for API token
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", "test-api-token")
			defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")

			// Run integration
			err = integrator.Run()
			assert.NoError(t, err)

			// Verify alert rule file was created
			convID, _, err := summariseSigmaRules(convOutput.Rules)
			assert.NoError(t, err)
			ruleUID := getRuleUID("test_loki", convID)
			expectedFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_test_loki_test_file_1_%s.json", ruleUID))
			_, err = os.Stat(expectedFile)
			assert.NoError(t, err)

			// Read the output file to get the captured outputs
			outputBytes, err := os.ReadFile(outputFile.Name())
			assert.NoError(t, err)
			outputContent := string(outputBytes)

			// Verify test_query_results was captured
			assert.Contains(t, outputContent, "test_query_results=")

			// Extract the test_query_results value
			lines := strings.Split(outputContent, "\n")
			var testQueryResults string
			for _, line := range lines {
				if strings.HasPrefix(line, "test_query_results=") {
					testQueryResults = strings.TrimPrefix(line, "test_query_results=")
					break
				}
			}
			assert.NotEmpty(t, testQueryResults)

			// Parse and validate the query test results
			var queryResults map[string][]QueryTestResult
			err = json.Unmarshal([]byte(testQueryResults), &queryResults)
			assert.NoError(t, err)
			assert.Equal(t, len(testQueries), len(queryResults[convFile]))

			// Verify both queries were executed
			assert.Equal(t, len(testQueries), len(mockDatasourceQuery.queryLog))

			// Verify each query was submitted correctly
			for _, query := range testQueries {
				assert.Contains(t, mockDatasourceQuery.queryLog, query)
			}

			// Verify datasource was used correctly
			assert.Contains(t, mockDatasourceQuery.datasourceLog, "test-loki-datasource")

			// Verify the query results contain expected data
			for _, results := range queryResults {
				for i, queryTestResult := range results {
					assert.Equal(t, "test-loki-datasource", queryTestResult.Datasource)

					// Verify the stats contain expected data
					stats := queryTestResult.Stats
					assert.Equal(t, 2, stats.Count) // Each mock response has 2 log lines
					assert.NotEmpty(t, stats.Fields)
					assert.Empty(t, stats.Errors)

					// Verify specific fields from our mock responses
					if i == 0 {
						if tt.wantLine {
							assert.Contains(t, stats.Fields, "Line")
							assert.Equal(t, "error log line", stats.Fields["Line"])
						} else {
							assert.NotContains(t, stats.Fields, "Line")
						}
						assert.Contains(t, stats.Fields, "job")
						assert.Equal(t, "loki", stats.Fields["job"])
						assert.Contains(t, stats.Fields, "level")
						assert.Equal(t, "error", stats.Fields["level"])
					} else if i == 1 {
						if tt.wantLine {
							assert.Contains(t, stats.Fields, "Line")
							assert.Equal(t, "warning log line", stats.Fields["Line"])
						} else {
							assert.NotContains(t, stats.Fields, "Line")
						}
						assert.Contains(t, stats.Fields, "job")
						assert.Equal(t, "loki", stats.Fields["job"])
						assert.Contains(t, stats.Fields, "level")
						assert.Equal(t, "warning", stats.Fields["level"])
					}
				}
			}
		})
	}
}

// Enhanced testDatasourceQuery to support error injection for testing continue_on_query_testing_errors
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

// TestIntegrationWithQueryTestingErrors tests the core behavior of continue_on_query_testing_errors
func TestGenerateExploreLink(t *testing.T) {
	tests := []struct {
		name                 string
		query                string
		datasource           string
		datasourceType       string
		from                 string
		to                   string
		orgID                int64
		grafanaURL           string
		wantURLContains      []string
		wantPanesContains    []string
		wantPanesNotContains []string
		wantError            bool
	}{
		{
			name:           "Loki explore link generation",
			query:          `{job="loki"} |= "error"`,
			datasource:     "loki-uid-123",
			datasourceType: Loki,
			from:           "now-1h",
			to:             "now",
			orgID:          1,
			grafanaURL:     "https://test.grafana.com",
			wantURLContains: []string{
				"https://test.grafana.com/explore",
				"schemaVersion=1",
				"orgId=1",
			},
			wantPanesContains: []string{
				`"datasource":"loki-uid-123"`,
				`"type":"loki"`,
				`"expr":"{job=\"loki\"} |= \"error\""`,
				`"queryType":"range"`,
				`"editorMode":"code"`,
				`"direction":"backward"`,
				`"from":"now-1h"`,
				`"to":"now"`,
			},
			wantPanesNotContains: []string{
				`"query":`,
				`"metrics"`,
				`"bucketAggs"`,
				`"timeField"`,
			},
			wantError: false,
		},
		{
			name:           "Elasticsearch explore link generation",
			query:          `type:log AND (level:(ERROR OR FATAL OR CRITICAL))`,
			datasource:     "es-uid-456",
			datasourceType: Elasticsearch,
			from:           "now-2h",
			to:             "now-1h",
			orgID:          2,
			grafanaURL:     "https://prod.grafana.com",
			wantURLContains: []string{
				"https://prod.grafana.com/explore",
				"schemaVersion=1",
				"orgId=2",
			},
			wantPanesContains: []string{
				`"datasource":"es-uid-456"`,
				`"type":"elasticsearch"`,
				`"query":"type:log AND (level:(ERROR OR FATAL OR CRITICAL))"`,
				`"metrics":[{"type":"count","id":"1"}]`,
				`"bucketAggs":[{"type":"date_histogram","id":"2","settings":{"interval":"auto"},"field":"@timestamp"}]`,
				`"timeField":"@timestamp"`,
				`"compact":false`,
				`"from":"now-2h"`,
				`"to":"now-1h"`,
			},
			wantPanesNotContains: []string{
				`"expr":`,
				`"queryType"`,
				`"editorMode"`,
				`"direction"`,
			},
			wantError: false,
		},
		{
			name:           "Generic datasource explore link generation",
			query:          `SELECT * FROM logs WHERE level = 'ERROR'`,
			datasource:     "generic-uid-789",
			datasourceType: "prometheus",
			from:           "now-30m",
			to:             "now",
			orgID:          3,
			grafanaURL:     "https://dev.grafana.com",
			wantURLContains: []string{
				"https://dev.grafana.com/explore",
				"schemaVersion=1",
				"orgId=3",
			},
			wantPanesContains: []string{
				`"datasource":"generic-uid-789"`,
				`"type":"prometheus"`,
				`"query":"SELECT * FROM logs WHERE level = 'ERROR'"`,
				`"from":"now-30m"`,
				`"to":"now"`,
			},
			wantPanesNotContains: []string{
				`"expr":`,
				`"queryType"`,
				`"editorMode"`,
				`"direction"`,
				`"metrics"`,
				`"bucketAggs"`,
				`"timeField"`,
			},
			wantError: false,
		},
		{
			name:           "Empty datasource should work fine",
			query:          `{job="test"}`,
			datasource:     "",
			datasourceType: Loki,
			from:           "now-1h",
			to:             "now",
			orgID:          1,
			grafanaURL:     "https://test.grafana.com",
			wantURLContains: []string{
				"https://test.grafana.com/explore",
				"schemaVersion=1",
				"orgId=1",
			},
			wantPanesContains: []string{
				`"datasource":""`,
				`"type":"loki"`,
				`"expr":"{job=\"test\"}"`,
			},
			wantPanesNotContains: []string{
				`"query":`,
				`"metrics"`,
				`"bucketAggs"`,
				`"timeField"`,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create integrator with test configuration
			integrator := &Integrator{
				config: Configuration{
					IntegratorConfig: IntegrationConfig{
						From:  tt.from,
						To:    tt.to,
						OrgID: tt.orgID,
					},
					DeployerConfig: DeploymentConfig{
						GrafanaInstance: tt.grafanaURL,
					},
				},
			}

			// Test generateExploreLink
			exploreLink, err := integrator.generateExploreLink(tt.query, tt.datasource, tt.datasourceType, ConversionConfig{}, ConversionConfig{})

			if tt.wantError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotEmpty(t, exploreLink)

			// Verify URL components
			for _, expected := range tt.wantURLContains {
				assert.Contains(t, exploreLink, expected, "Explore link should contain: %s", expected)
			}

			// Parse the URL to extract the panes parameter
			parsedURL, err := url.Parse(exploreLink)
			assert.NoError(t, err)

			panesParam := parsedURL.Query().Get("panes")
			assert.NotEmpty(t, panesParam, "panes parameter should be present")

			// URL decode the panes parameter
			decodedPanes, err := url.QueryUnescape(panesParam)
			assert.NoError(t, err)
			assert.NotEmpty(t, decodedPanes, "decoded panes should not be empty")

			// Verify the decoded panes contains expected components
			for _, expected := range tt.wantPanesContains {
				assert.Contains(t, decodedPanes, expected, "Decoded panes should contain: %s", expected)
			}

			// Verify the decoded panes does not contain unexpected components
			for _, unexpected := range tt.wantPanesNotContains {
				assert.NotContains(t, decodedPanes, unexpected, "Decoded panes should not contain: %s", unexpected)
			}

			// Verify the link is properly URL encoded (raw JSON should not be visible in the URL)
			assert.Contains(t, exploreLink, "panes=")
			assert.NotContains(t, exploreLink, `{"yyz":`)
		})
	}
}

func TestIntegratorWithExploreLinkGeneration(t *testing.T) {
	tests := []struct {
		name                 string
		datasourceType       string
		query                string
		datasource           string
		wantURLContains      []string
		wantPanesContains    []string
		wantPanesNotContains []string
	}{
		{
			name:           "Loki datasource generates correct explore link",
			datasourceType: Loki,
			query:          `{job="loki"} |= "error"`,
			datasource:     "test-loki-datasource",
			wantURLContains: []string{
				"https://test.grafana.com/explore",
				"schemaVersion=1",
				"orgId=1",
			},
			wantPanesContains: []string{
				`"datasource":"test-loki-datasource"`,
				`"type":"loki"`,
				`"expr":"{job=\"loki\"} |= \"error\""`,
				`"queryType":"range"`,
			},
			wantPanesNotContains: []string{
				`"query":`,
				`"metrics"`,
				`"bucketAggs"`,
				`"timeField"`,
			},
		},
		{
			name:           "Elasticsearch datasource generates correct explore link",
			datasourceType: Elasticsearch,
			query:          `type:log AND (level:(ERROR OR FATAL OR CRITICAL))`,
			datasource:     "test-elasticsearch-datasource",
			wantURLContains: []string{
				"https://test.grafana.com/explore",
				"schemaVersion=1",
				"orgId=1",
			},
			wantPanesContains: []string{
				`"datasource":"test-elasticsearch-datasource"`,
				`"type":"elasticsearch"`,
				`"query":"type:log AND (level:(ERROR OR FATAL OR CRITICAL))"`,
				`"metrics":[{"type":"count","id":"1"}]`,
				`"bucketAggs":[{"type":"date_histogram"`,
				`"timeField":"@timestamp"`,
			},
			wantPanesNotContains: []string{
				`"expr":`,
				`"queryType"`,
				`"editorMode"`,
				`"direction"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test queries
			testQueries := []string{tt.query}

			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_explore_link", tt.name)
			err := os.MkdirAll(testDir, 0o755)
			assert.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Create conversion and deployment subdirectories
			convPath := filepath.Join(testDir, "conv")
			deployPath := filepath.Join(testDir, "deploy")
			err = os.MkdirAll(convPath, 0o755)
			assert.NoError(t, err)
			err = os.MkdirAll(deployPath, 0o755)
			assert.NoError(t, err)

			// Create test conversion output
			convOutput := ConversionOutput{
				Queries:        testQueries,
				ConversionName: "test_explore_link",
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Explore Link Rule",
					},
				},
			}

			// Create test configuration with query testing enabled
			config := Configuration{
				Folders: FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: ConversionConfig{
					Target:         tt.datasourceType,
					DataSource:     tt.datasource,
					DataSourceType: tt.datasourceType,
				},
				Conversions: []ConversionConfig{
					{
						Name:           "test_explore_link",
						RuleGroup:      "Explore Link Test Rules",
						TimeWindow:     "5m",
						DataSource:     tt.datasource,
						DataSourceType: tt.datasourceType,
					},
				},
				IntegratorConfig: IntegrationConfig{
					FolderID:    "test-folder",
					OrgID:       1,
					TestQueries: true,
					From:        "now-1h",
					To:          "now",
				},
				DeployerConfig: DeploymentConfig{
					GrafanaInstance: "https://test.grafana.com",
					Timeout:         "5s",
				},
			}

			// Create test conversion output file
			convBytes, err := json.Marshal(convOutput)
			assert.NoError(t, err)
			convFile := filepath.Join(convPath, "test_explore_link.json")
			err = os.WriteFile(convFile, convBytes, 0o600)
			assert.NoError(t, err)

			// Create mock query executor
			mockDatasourceQuery := newTestDatasourceQuery()

			// Add mock response for our test query
			mockDatasourceQuery.AddMockResponse(tt.query, []byte(`{
				"results": {
					"A": {
						"frames": [{
							"schema": {
								"fields": [
									{"name": "Time", "type": "time"},
									{"name": "Line", "type": "string"}
								]
							},
							"data": {
								"values": [
									[1625126400000, 1625126460000],
									["test log line", "another test log"]
								]
							}
						}]
					}
				}
			}`))

			// Create a temporary output file for capturing outputs
			outputFile, err := os.CreateTemp("", "github-output")
			assert.NoError(t, err)
			defer os.Remove(outputFile.Name())

			// Setup environment for the test
			os.Setenv("GITHUB_OUTPUT", outputFile.Name())
			defer os.Unsetenv("GITHUB_OUTPUT")

			// Set up integrator
			integrator := &Integrator{
				config:       config,
				addedFiles:   []string{convFile},
				removedFiles: []string{},
			}

			// Save original executor and restore after test
			originalDatasourceQuery := DefaultDatasourceQuery
			DefaultDatasourceQuery = mockDatasourceQuery
			defer func() {
				DefaultDatasourceQuery = originalDatasourceQuery
			}()

			// Set environment variable for API token
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", "test-api-token")
			defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")

			// Run integration
			err = integrator.Run()
			assert.NoError(t, err)

			// Read the output file to get the captured outputs
			outputBytes, err := os.ReadFile(outputFile.Name())
			assert.NoError(t, err)
			outputContent := string(outputBytes)

			// Verify test_query_results was captured
			assert.Contains(t, outputContent, "test_query_results=")

			// Extract the test_query_results value
			lines := strings.Split(outputContent, "\n")
			var testQueryResults string
			for _, line := range lines {
				if strings.HasPrefix(line, "test_query_results=") {
					testQueryResults = strings.TrimPrefix(line, "test_query_results=")
					break
				}
			}
			assert.NotEmpty(t, testQueryResults)

			// Parse and validate the query test results
			var queryResults map[string][]QueryTestResult
			err = json.Unmarshal([]byte(testQueryResults), &queryResults)
			assert.NoError(t, err)
			assert.Equal(t, len(testQueries), len(queryResults[convFile]))

			// Verify the explore link in the query results
			for _, results := range queryResults {
				for _, queryTestResult := range results {
					assert.Equal(t, tt.datasource, queryTestResult.Datasource)
					assert.NotEmpty(t, queryTestResult.Link)

					// Verify URL components
					for _, expected := range tt.wantURLContains {
						assert.Contains(t, queryTestResult.Link, expected, "Explore link should contain: %s", expected)
					}

					// Parse the URL to extract the panes parameter
					parsedURL, err := url.Parse(queryTestResult.Link)
					assert.NoError(t, err)

					panesParam := parsedURL.Query().Get("panes")
					assert.NotEmpty(t, panesParam, "panes parameter should be present")

					// URL decode the panes parameter
					decodedPanes, err := url.QueryUnescape(panesParam)
					assert.NoError(t, err)
					assert.NotEmpty(t, decodedPanes, "decoded panes should not be empty")

					// Verify the decoded panes contains expected components
					for _, expected := range tt.wantPanesContains {
						assert.Contains(t, decodedPanes, expected, "Decoded panes should contain: %s", expected)
					}

					// Verify the decoded panes does not contain unexpected components
					for _, unexpected := range tt.wantPanesNotContains {
						assert.NotContains(t, decodedPanes, unexpected, "Decoded panes should not contain: %s", unexpected)
					}

					// Verify the link is properly URL encoded (raw JSON should not be visible in the URL)
					assert.Contains(t, queryTestResult.Link, "panes=")
					assert.NotContains(t, queryTestResult.Link, `{"yyz":`)
				}
			}
		})
	}
}

func TestIntegrationWithQueryTestingErrors(t *testing.T) {
	tests := []struct {
		name                     string
		continueOnErrors         bool
		queryErrors              map[string]error
		expectIntegrationFailure bool
		expectAlertRuleCreated   bool
	}{
		{
			name:                     "query error with continue disabled should fail integration immediately",
			continueOnErrors:         false,
			queryErrors:              map[string]error{"{job=\"test\"} |= \"error\"": fmt.Errorf("datasource not found")},
			expectIntegrationFailure: true,
			expectAlertRuleCreated:   true, // Alert rule is created before query testing
		},
		{
			name:                     "query error with continue enabled should complete integration successfully",
			continueOnErrors:         true,
			queryErrors:              map[string]error{"{job=\"test\"} |= \"error\"": fmt.Errorf("datasource not found")},
			expectIntegrationFailure: false, // Integration should succeed when continue is enabled
			expectAlertRuleCreated:   true,  // Alert rule should still be created
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_continue_errors", tt.name)
			err := os.MkdirAll(testDir, 0o755)
			assert.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Set up the github output file
			oldGithubOutput := os.Getenv("GITHUB_OUTPUT")
			os.Setenv("GITHUB_OUTPUT", filepath.Join(testDir, "github-output"))
			defer os.Setenv("GITHUB_OUTPUT", oldGithubOutput)

			// Create conversion and deployment subdirectories
			convPath := filepath.Join(testDir, "conv")
			deployPath := filepath.Join(testDir, "deploy")
			err = os.MkdirAll(convPath, 0o755)
			assert.NoError(t, err)
			err = os.MkdirAll(deployPath, 0o755)
			assert.NoError(t, err)

			// Create test conversion output
			convOutput := ConversionOutput{
				ConversionName: "test_continue",
				Queries:        []string{"{job=\"test\"} |= \"error\"", "{job=\"test\"} |= \"info\""},
				Rules: []SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Continue Rule",
					},
				},
			}

			// Create test configuration with query testing enabled
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
						Name:       "test_continue",
						RuleGroup:  "Test Rules",
						TimeWindow: "5m",
						DataSource: "test-datasource",
					},
				},
				IntegratorConfig: IntegrationConfig{
					FolderID:                     "test-folder",
					OrgID:                        1,
					TestQueries:                  true,
					From:                         "now-1h",
					To:                           "now",
					ContinueOnQueryTestingErrors: tt.continueOnErrors,
				},
				DeployerConfig: DeploymentConfig{
					GrafanaInstance: "https://test.grafana.com",
					Timeout:         "5s",
				},
			}

			// Create test conversion output file
			convBytes, err := json.Marshal(convOutput)
			assert.NoError(t, err)
			convFile := filepath.Join(convPath, "test_continue.json")
			err = os.WriteFile(convFile, convBytes, 0o600)
			assert.NoError(t, err)

			// Create mock query executor with error injection
			mockDatasourceQuery := newTestDatasourceQueryWithErrors()

			// Add query errors
			for query, queryErr := range tt.queryErrors {
				mockDatasourceQuery.AddMockError(query, queryErr)
			}

			// Add successful responses for queries that don't have errors
			mockDatasourceQuery.AddMockResponse("{job=\"test\"} |= \"info\"", []byte(`{
				"results": {
					"A": {
						"frames": [{
							"schema": {"fields": [{"name": "Time", "type": "time"}, {"name": "Line", "type": "string"}]},
							"data": {"values": [[1625126400000], ["info log"]]}
						}]
					}
				}
			}`))

			// Save original executor and restore after test
			originalDatasourceQuery := DefaultDatasourceQuery
			DefaultDatasourceQuery = mockDatasourceQuery
			defer func() {
				DefaultDatasourceQuery = originalDatasourceQuery
			}()

			// Set environment variable for API token
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", "test-api-token")
			defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")

			// Set up integrator
			integrator := &Integrator{
				config:       config,
				addedFiles:   []string{convFile},
				removedFiles: []string{},
			}

			// Run integration
			err = integrator.Run()

			// Verify integration failure expectation
			if tt.expectIntegrationFailure {
				assert.Error(t, err, "Expected integration to fail but it succeeded")
			} else {
				assert.NoError(t, err, "Expected integration to succeed but it failed")
			}

			// Verify alert rule creation expectation - this is the key test
			convID, _, err := summariseSigmaRules(convOutput.Rules)
			assert.NoError(t, err)
			ruleUID := getRuleUID("test_continue", convID)
			expectedFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_test_continue_test_continue_%s.json", ruleUID))

			if tt.expectAlertRuleCreated {
				_, err = os.Stat(expectedFile)
				assert.NoError(t, err, "Expected alert rule file to be created but it wasn't")
			} else {
				_, err = os.Stat(expectedFile)
				assert.True(t, os.IsNotExist(err), "Expected alert rule file to not be created but it was")
			}
		})
	}
}
