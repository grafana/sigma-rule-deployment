package integrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/stretchr/testify/assert"
)

func TestConvertToAlert(t *testing.T) {
	tests := []struct {
		name                   string
		queries                []string
		rule                   *model.ProvisionedAlertRule
		titles                 string
		convConfig             model.ConversionConfig
		integratorConfig       model.IntegrationConfig
		convObject             model.ConversionOutput
		wantQueryText          string
		wantDuration           model.Duration
		wantUnchanged          bool
		wantError              bool
		wantLabels             map[string]string
		wantAnnotations        map[string]string
		wantCombinerExpression string
	}{
		{
			name:    "valid new loki query",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule 1",
			rule: &model.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			wantQueryText: "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantDuration:  model.Duration(300 * time.Second),
			wantError:     false,
		},
		{
			name:    "valid ES query",
			queries: []string{`from * | where eventSource=="kms.amazonaws.com" and eventName=="CreateGrant"`},
			titles:  "Alert Rule 2",
			rule: &model.ProvisionedAlertRule{
				UID: "3bb06d82",
			},
			convConfig: model.ConversionConfig{
				Name:           "conv",
				Target:         "esql",
				DataSource:     "my_es_data_source",
				RuleGroup:      "Every 5 Minutes",
				TimeWindow:     "5m",
				DataSourceType: "elasticsearch",
			},
			wantQueryText: `from * | where eventSource==\"kms.amazonaws.com\" and eventName==\"CreateGrant\"`,
			wantDuration:  model.Duration(300 * time.Second),
			wantError:     false,
		},
		{
			name:    "invalid time window",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule 3",
			rule: &model.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: model.ConversionConfig{
				TimeWindow: "1y",
			},
			wantDuration: 0, // invalid time window, expect no value
			wantError:    true,
		},
		{
			name:    "invalid time window",
			queries: []string{"{job=`.+`} | json | test=`true`", "sum(count_over_time({job=`.+`} | json | test=`false`[$__auto]))"},
			titles:  "Alert Rule 4 & Alert Rule 5",
			rule: &model.ProvisionedAlertRule{
				UID: "f4c34eae-c7c3-4891-8965-08a01e8286b8",
			},
			convConfig: model.ConversionConfig{
				TimeWindow: "1y",
			},
			wantDuration: 0, // invalid time window, expect no value
			wantError:    true,
		},
		{
			name:    "multiple queries use math combiner",
			queries: []string{"{job=`.+`} | json | test=`true`", "{job=`.+`} | json | test=`false`"},
			titles:  "Multiple Query Test",
			rule: &model.ProvisionedAlertRule{
				UID: "multi-test",
			},
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "test_ds",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			wantQueryText:          "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantDuration:           model.Duration(5 * time.Minute),
			wantCombinerExpression: `"expression":"${A0}+${A1}"`,
		},
		{
			name:    "skip unchanged queries",
			queries: []string{`{job=".+"} | json | test="true"`},
			titles:  "New Alert Rule Title", // This should be ignored
			rule: &model.ProvisionedAlertRule{
				UID:   "5c1c217a",
				Title: "Unchanged Alert Rule",
				Data: []model.AlertQuery{
					{
						Model: json.RawMessage(`{"refId":"A0","datasource":{"type":"loki","uid":"nil"},"hide":false,"expr":"sum(count_over_time({job=\".+\"} | json | test=\"true\"[$__auto]))","queryType":"instant","editorMode":"code"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"B","hide":false,"type":"math","datasource":{"uid":"__expr__","type":"__expr__"},"expression":"${A0}"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[0],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`),
					},
				},
			},
			wantUnchanged: true,
		},
		{
			name:    "process changed queries",
			queries: []string{`{job=".+"} | json | test="true"`},
			titles:  "New Alert Rule Title", // This should *not* be ignored
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every Minute",
				TimeWindow: "1m",
			},
			rule: &model.ProvisionedAlertRule{
				UID:   "5c1c217a",
				Title: "Unchanged Alert Rule",
				Data: []model.AlertQuery{
					{
						// old query, which doesn't match the new query
						Model: json.RawMessage(`{"refId":"A0","datasource":{"type":"loki","uid":"nil"},"hide":false,"expr":"sum(count_over_time({old_job=\".+\"} | logfmt | test=\"old_query\"[$__auto]))","queryType":"instant","editorMode":"code"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"B","hide":false,"type":"reduce","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[],"type":"gt"},"operator":{"type":"and"},"query":{"params":["B"]},"reducer":{"params":[],"type":"last"}}],"reducer":"last","expression":"A0"}`),
					},
					{
						Model: json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[0],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`),
					},
				},
			},
			wantDuration:  model.Duration(1 * time.Minute),
			wantUnchanged: false,
		},
		{
			name:    "valid query with a custom query model",
			queries: []string{"DO MY QUERY"},
			titles:  "Alert Rule 7",
			rule: &model.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "custom",
				DataSource: "my_custom_data_source",
				RuleGroup:  "Every Hour",
				TimeWindow: "1h",
				QueryModel: `{"refId":"%s","datasource":{"type":"custom","uid":"%s"},"queryString":"(%s)"}`,
			},
			wantQueryText: "(DO MY QUERY)",
			wantDuration:  model.Duration(1 * time.Hour),
			wantError:     false,
		},
		{
			name:    "valid query with a generic query model",
			queries: []string{"DO MY QUERY"},
			titles:  "Alert Rule 8",
			rule: &model.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "generic",
				DataSource: "generic_uid",
				RuleGroup:  "Every 30 Minutes",
				TimeWindow: "30m",
			},
			wantQueryText: `"DO MY QUERY"`,
			wantDuration:  model.Duration(30 * time.Minute),
			wantError:     false,
		},
		{
			name:    "valid query with lookback",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Alert Rule with Lookback",
			rule: &model.ProvisionedAlertRule{
				UID: "5c1c217a",
			},
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
				Lookback:   "2m",
			},
			wantQueryText: "sum(count_over_time({job=`.+`} | json | test=`true`[$__auto]))",
			wantDuration:  model.Duration(7 * time.Minute), // 5m + 2m lookback = 7m
			wantError:     false,
		},
		{
			name:    "template annotations and labels",
			queries: []string{"{job=`.+`} | json | test=`true`"},
			titles:  "Template Rule",
			rule: &model.ProvisionedAlertRule{
				UID: "",
			},
			convObject: model.ConversionOutput{
				Rules: []model.SigmaRule{
					{
						Level:     "high",
						Logsource: model.SigmaLogsource{Product: "okta", Service: "okta"},
						Author:    "John Doe",
					},
				},
			},
			convConfig: model.ConversionConfig{
				Name:       "conv",
				Target:     "loki",
				DataSource: "my_data_source",
				RuleGroup:  "Every 5 Minutes",
				TimeWindow: "5m",
			},
			integratorConfig: model.IntegrationConfig{
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
			wantDuration:  model.Duration(300 * time.Second),
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

					if tt.wantCombinerExpression != "" {
						combinerModel := string(tt.rule.Data[2].Model)
						assert.Contains(t, combinerModel, tt.wantCombinerExpression)
					}

					if tt.convConfig.Lookback != "" {
						lookbackDuration, err := time.ParseDuration(tt.convConfig.Lookback)
						assert.NoError(t, err)
						expectedTo := model.Duration(lookbackDuration)
						assert.Equal(t, tt.wantDuration, tt.rule.Data[0].RelativeTimeRange.From, "From should match expected duration (time window + lookback)")
						assert.Equal(t, expectedTo, tt.rule.Data[0].RelativeTimeRange.To, "To should be lookback duration")
					} else {
						assert.Equal(t, model.Duration(0), tt.rule.Data[0].RelativeTimeRange.To, "To should be 0 when no lookback")
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
		testFiles  string
		allRules   bool
		expConfig  model.Configuration
		expAdd     []string
		expDel     []string
		expTest    []string
		wantError  bool
	}{
		{
			name:       "valid loki config, single added file",
			configPath: "testdata/config.yml",
			token:      "my-test-token",
			changed:    "testdata/conv.json",
			deleted:    "",
			testFiles:  "testdata/conv.json testdata/conv2.json",
			allRules:   false,
			expConfig: model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: model.ConversionConfig{
					Target:          "loki",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-logs",
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "conv",
						RuleGroup:  "Every 5 Minutes",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "XXXX",
					OrgID:       1,
					From:        "now-1h",
					To:          "now",
					TestQueries: true,
				},
			},
			expAdd:    []string{"testdata/conv.json"},
			expDel:    []string{},
			expTest:   []string{"testdata/conv.json", "testdata/conv2.json"},
			wantError: false,
		},
		{
			name:       "valid loki config, single added file, no test queries",
			configPath: "testdata/no-test-config.yml",
			token:      "my-test-token",
			changed:    "testdata/conv.json",
			deleted:    "",
			testFiles:  "testdata/conv.json testdata/conv2.json",
			allRules:   false,
			expConfig: model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: model.ConversionConfig{
					Target:          "loki",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-logs",
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "conv",
						RuleGroup:  "Every 5 Minutes",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "XXXX",
					OrgID:       1,
					From:        "now-1h",
					To:          "now",
					TestQueries: false,
				},
			},
			expAdd:    []string{"testdata/conv.json"},
			expDel:    []string{},
			expTest:   []string{},
			wantError: false,
		},
		{
			name:       "valid es config, multiple files added, changed and removed",
			configPath: "testdata/es-config.yml",
			token:      "my-test-token",
			changed:    "testdata/conv1.json testdata/conv3.json",
			deleted:    "testdata/conv2.json testdata/conv4.json",
			testFiles:  "testdata/conv1.json testdata/conv3.json",
			allRules:   false,
			expConfig: model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: model.ConversionConfig{
					Target:          "esql",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-es-logs",
					DataSourceType:  "elasticsearch",
				},
				Conversions: []model.ConversionConfig{
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
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "XXXX",
					OrgID:       1,
					From:        "now-1h",
					To:          "now",
					TestQueries: true,
				},
			},
			expAdd:    []string{"testdata/conv1.json", "testdata/conv3.json"},
			expDel:    []string{"testdata/conv2.json", "testdata/conv4.json"},
			expTest:   []string{"testdata/conv1.json", "testdata/conv3.json"},
			wantError: false,
		},
		{
			name:       "valid es config, multiple files added, changed and removed, subset of test files",
			configPath: "testdata/es-config.yml",
			token:      "my-test-token",
			changed:    "testdata/conv1.json testdata/conv3.json",
			deleted:    "testdata/conv2.json testdata/conv4.json",
			testFiles:  "testdata/conv1.json",
			allRules:   false,
			expConfig: model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: model.ConversionConfig{
					Target:          "esql",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-es-logs",
					DataSourceType:  "elasticsearch",
				},
				Conversions: []model.ConversionConfig{
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
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "XXXX",
					OrgID:       1,
					From:        "now-1h",
					To:          "now",
					TestQueries: true,
				},
			},
			expAdd:    []string{"testdata/conv1.json", "testdata/conv3.json"},
			expDel:    []string{"testdata/conv2.json", "testdata/conv4.json"},
			expTest:   []string{"testdata/conv1.json"},
			wantError: false,
		},
		{
			name:       "load all files when ALL_RULES is true",
			configPath: "testdata/config.yml",
			token:      "my-test-token",
			changed:    "",
			deleted:    "",
			testFiles:  "",
			allRules:   true,
			expConfig: model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: model.ConversionConfig{
					Target:          "loki",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-logs",
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "conv",
						RuleGroup:  "Every 5 Minutes",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "XXXX",
					OrgID:       1,
					From:        "now-1h",
					To:          "now",
					TestQueries: true,
				},
			},
			expAdd:    []string{"testdata/config.yml", "testdata/es-config.yml", "testdata/no-test-config.yml", "testdata/non-local-conv-config.yml", "testdata/non-local-deploy-config.yml", "testdata/sample_rule.json"},
			expDel:    []string{},
			expTest:   []string{"testdata/config.yml", "testdata/es-config.yml", "testdata/no-test-config.yml", "testdata/non-local-conv-config.yml", "testdata/non-local-deploy-config.yml", "testdata/sample_rule.json"},
			wantError: false,
		},
		{
			name:       "load all files when ALL_RULES is true, no test queries",
			configPath: "testdata/no-test-config.yml",
			token:      "my-test-token",
			changed:    "",
			deleted:    "",
			testFiles:  "",
			allRules:   true,
			expConfig: model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: "./testdata",
					DeploymentPath: "./testdata",
				},
				ConversionDefaults: model.ConversionConfig{
					Target:          "loki",
					Format:          "default",
					SkipUnsupported: "true",
					FilePattern:     "*.yml",
					DataSource:      "grafanacloud-logs",
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "conv",
						RuleGroup:  "Every 5 Minutes",
						TimeWindow: "5m",
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "XXXX",
					OrgID:       1,
					From:        "now-1h",
					To:          "now",
					TestQueries: false,
				},
			},
			expAdd:    []string{"testdata/config.yml", "testdata/es-config.yml", "testdata/no-test-config.yml", "testdata/non-local-conv-config.yml", "testdata/non-local-deploy-config.yml", "testdata/sample_rule.json"},
			expDel:    []string{},
			expTest:   []string{},
			wantError: false,
		},

		{
			name:       "missing config file",
			configPath: "testdata/missing_config.yml",
			testFiles:  "",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "no path",
			configPath: "",
			testFiles:  "",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "non-local config file",
			configPath: "../testdata/missing_config.yml",
			testFiles:  "",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "conversion path is not local",
			configPath: "testdata/non-local-conv-config.yml",
			testFiles:  "",
			allRules:   false,
			wantError:  true,
		},
		{
			name:       "deployment path is not local",
			configPath: "testdata/non-local-deploy-config.yml",
			testFiles:  "",
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
			os.Setenv("TEST_FILES", tt.testFiles)
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
				assert.Equal(t, tt.expTest, i.testFiles)
			}
		})
	}
	defer os.Unsetenv("INTEGRATOR_CONFIG_PATH")
	defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")
	defer os.Unsetenv("CHANGED_FILES")
	defer os.Unsetenv("DELETED_FILES")
	defer os.Unsetenv("TEST_FILES")
	defer os.Unsetenv("ALL_RULES")
}

func TestDoConversions(t *testing.T) {
	tests := []struct {
		name           string
		addedFiles     []string
		convOutput     model.ConversionOutput
		wantError      bool
		wantFileExists bool
	}{
		{
			name:       "single conversion success",
			addedFiles: []string{"test_conv.json"},
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
			wantError:      false,
			wantFileExists: true,
		},
		{
			name:       "no queries conversion",
			addedFiles: []string{"test_conv_no_queries.json"},
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
			wantError:      false,
			wantFileExists: false,
		},
		{
			name:       "no matching config",
			addedFiles: []string{"test_unknown.json"},
			convOutput: model.ConversionOutput{
				ConversionName: "unknown_conversion",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Rule",
					},
				},
			},
			wantError:      false,
			wantFileExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_do_conversions", tt.name)
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

			// Create test configuration
			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
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
					FolderID: "test-folder",
					OrgID:    1,
				},
			}

			// Create conversion output files
			var convFiles []string
			for _, fileName := range tt.addedFiles {
				convBytes, err := json.Marshal(tt.convOutput)
				assert.NoError(t, err)
				convFile := filepath.Join(convPath, fileName)
				err = os.WriteFile(convFile, convBytes, 0o600)
				assert.NoError(t, err)
				convFiles = append(convFiles, convFile)
			}

			// Set up integrator
			i := &Integrator{
				config:     config,
				addedFiles: convFiles,
			}

			// Run DoConversions
			err = i.DoConversions()
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify alert rule file creation
			if tt.wantFileExists {
				convID, _, err := summariseSigmaRules(tt.convOutput.Rules)
				assert.NoError(t, err)
				ruleUID := getRuleUID(tt.convOutput.ConversionName, convID)

				// Check for deployment files
				files, err := os.ReadDir(deployPath)
				assert.NoError(t, err)

				// Should have at least one file if wantFileExists is true
				assert.Greater(t, len(files), 0)

				// Check that the expected file pattern exists
				expectedPattern := fmt.Sprintf("alert_rule_%s_", tt.convOutput.ConversionName)
				found := false
				for _, file := range files {
					if strings.Contains(file.Name(), expectedPattern) && strings.Contains(file.Name(), ruleUID) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected alert rule file not found")
			} else {
				// Verify no files were created
				files, err := os.ReadDir(deployPath)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(files))
			}
		})
	}
}

func TestDoCleanup(t *testing.T) {
	tests := []struct {
		name                     string
		removedFiles             []string
		createOrphanedConversion bool
		createOrphanedDeployment bool
		wantError                bool
	}{
		{
			name:         "cleanup removed files",
			removedFiles: []string{"test_conv.json"},
			wantError:    false,
		},
		{
			name:                     "cleanup orphaned conversion files",
			removedFiles:             []string{},
			createOrphanedConversion: true,
			wantError:                false,
		},
		{
			name:                     "cleanup orphaned deployment files",
			removedFiles:             []string{},
			createOrphanedDeployment: true,
			wantError:                false,
		},
		{
			name:         "no files to cleanup",
			removedFiles: []string{},
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_do_cleanup", tt.name)
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

			// Create test configuration
			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "test_conv",
						RuleGroup:  "Test Rules",
						TimeWindow: "5m",
					},
				},
			}

			// Create files to be removed
			var removedFilePaths []string
			for _, fileName := range tt.removedFiles {
				// Create dummy deployment file that should be removed
				deployFile := filepath.Join(deployPath, fmt.Sprintf("alert_rule_%s_test_123abc.json", strings.TrimSuffix(fileName, ".json")))
				dummyRule := &model.ProvisionedAlertRule{
					UID:       "123abc",
					Title:     "Test Rule",
					RuleGroup: "Test Rules",
				}
				err = writeRuleToFile(dummyRule, deployFile, false)
				assert.NoError(t, err)
				removedFilePaths = append(removedFilePaths, filepath.Join(convPath, fileName))
			}

			// Create orphaned conversion file
			if tt.createOrphanedConversion {
				orphanedConv := model.ConversionOutput{
					ConversionName: "orphaned_conversion",
					Queries:        []string{"{job=`test`} | json"},
					Rules: []model.SigmaRule{
						{
							ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
							Title: "Orphaned Rule",
						},
					},
				}
				convBytes, err := json.Marshal(orphanedConv)
				assert.NoError(t, err)
				orphanedFile := filepath.Join(convPath, "orphaned.json")
				err = os.WriteFile(orphanedFile, convBytes, 0o600)
				assert.NoError(t, err)
			}

			// Create orphaned deployment file
			if tt.createOrphanedDeployment {
				orphanedDeployFile := filepath.Join(deployPath, "alert_rule_orphaned_deploy_456def.json")
				dummyRule := &model.ProvisionedAlertRule{
					UID:       "456def",
					Title:     "Orphaned Deploy Rule",
					RuleGroup: "Test Rules",
					Annotations: map[string]string{
						"ConversionFile": "/path/to/nonexistent/conversion.json",
					},
				}
				err = writeRuleToFile(dummyRule, orphanedDeployFile, false)
				assert.NoError(t, err)
			}

			// Set up integrator
			i := &Integrator{
				config:       config,
				removedFiles: removedFilePaths,
			}

			// Run DoCleanup
			err = i.DoCleanup()
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify files were cleaned up appropriately
			if len(tt.removedFiles) > 0 {
				files, err := os.ReadDir(deployPath)
				assert.NoError(t, err)
				// Should have fewer files after cleanup (or none if all were removed)
				for _, file := range files {
					for _, removedFile := range tt.removedFiles {
						baseRemoved := strings.TrimSuffix(removedFile, ".json")
						assert.NotContains(t, file.Name(), fmt.Sprintf("alert_rule_%s_", baseRemoved))
					}
				}
			}

			if tt.createOrphanedConversion {
				// Verify orphaned conversion file was removed
				_, err = os.Stat(filepath.Join(convPath, "orphaned.json"))
				assert.True(t, os.IsNotExist(err), "Orphaned conversion file should be deleted")
			}

			if tt.createOrphanedDeployment {
				// Verify orphaned deployment file was removed
				_, err = os.Stat(filepath.Join(deployPath, "alert_rule_orphaned_deploy_456def.json"))
				assert.True(t, os.IsNotExist(err), "Orphaned deployment file should be deleted")
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name                      string
		addedFiles                []string
		testFiles                 []string
		convOutput                model.ConversionOutput
		expectConversionFiles     int
		expectQueryTestingResults bool
	}{
		{
			name:       "conversion and testing same files",
			addedFiles: []string{"test_conv.json"},
			testFiles:  []string{"test_conv.json"},
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
			expectConversionFiles:     1,
			expectQueryTestingResults: true,
		},
		{
			name:       "conversion and testing different files",
			addedFiles: []string{"test_conv1.json"},
			testFiles:  []string{"test_conv1.json", "test_conv2.json"},
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
			expectConversionFiles:     1,
			expectQueryTestingResults: true,
		},
		{
			name:       "only conversion no testing",
			addedFiles: []string{"test_conv.json"},
			testFiles:  []string{},
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
			expectConversionFiles:     1,
			expectQueryTestingResults: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory
			testDir := filepath.Join("testdata", "test_new_architecture", tt.name)
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

			// Create test configuration
			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
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
					FolderID:    "test-folder",
					OrgID:       1,
					TestQueries: true,
					From:        "now-1h",
					To:          "now",
				},
				DeployerConfig: model.DeploymentConfig{
					GrafanaInstance: "https://test.grafana.com",
					Timeout:         "5s",
				},
			}

			// Create conversion output files for both conversion and testing
			var addedFilePaths []string
			var testFilePaths []string

			allFiles := make(map[string]bool)
			for _, fileName := range tt.addedFiles {
				allFiles[fileName] = true
			}
			for _, fileName := range tt.testFiles {
				allFiles[fileName] = true
			}

			for fileName := range allFiles {
				convBytes, err := json.Marshal(tt.convOutput)
				assert.NoError(t, err)
				convFile := filepath.Join(convPath, fileName)
				err = os.WriteFile(convFile, convBytes, 0o600)
				assert.NoError(t, err)

				for _, addedFileName := range tt.addedFiles {
					if fileName == addedFileName {
						addedFilePaths = append(addedFilePaths, convFile)
					}
				}
				for _, testFileName := range tt.testFiles {
					if fileName == testFileName {
						testFilePaths = append(testFilePaths, convFile)
					}
				}
			}

			// Create a temporary output file for capturing outputs
			outputFile, err := os.CreateTemp("", "github-output")
			assert.NoError(t, err)
			defer os.Remove(outputFile.Name())

			// Setup environment for the test
			os.Setenv("GITHUB_OUTPUT", outputFile.Name())
			defer os.Unsetenv("GITHUB_OUTPUT")

			// Set up integrator
			i := &Integrator{
				config:       config,
				addedFiles:   addedFilePaths,
				testFiles:    testFilePaths,
				removedFiles: []string{},
			}

			// Create mock query executor
			mockDatasourceQuery := newTestDatasourceQuery()

			// Save original executor and restore after test
			originalDatasourceQuery := DefaultDatasourceQuery
			DefaultDatasourceQuery = mockDatasourceQuery
			defer func() {
				DefaultDatasourceQuery = originalDatasourceQuery
			}()

			// Set environment variable for API token
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", "test-api-token")
			defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")

			// Call Run
			err = i.Run()
			assert.NoError(t, err)

			// Verify conversion files were created
			files, err := os.ReadDir(deployPath)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectConversionFiles, len(files))

			// Verify rules_integrated output was set
			outputBytes, err := os.ReadFile(outputFile.Name())
			assert.NoError(t, err)
			outputContent := string(outputBytes)
			assert.Contains(t, outputContent, "rules_integrated=")
		})
	}
}

func TestReadWriteAlertRule(t *testing.T) {
	// A simple test of reading and writing alert rule files
	rule := &model.ProvisionedAlertRule{}
	err := readRuleFromFile(rule, "testdata/sample_rule.json")
	assert.NoError(t, err)
	err = writeRuleToFile(rule, "testdata/sample_rule.json", false)
	assert.NoError(t, err)
}

func TestSummariseSigmaRules(t *testing.T) {
	tests := []struct {
		name      string
		rules     []model.SigmaRule
		wantID    uuid.UUID
		wantTitle string
		wantError bool
	}{
		{
			name: "valid rule",
			rules: []model.SigmaRule{
				{ID: "996f8884-9144-40e7-ac63-29090ccde9a0", Title: "Rule 1"},
			},
			wantID:    uuid.MustParse("996f8884-9144-40e7-ac63-29090ccde9a0"),
			wantTitle: "Rule 1",
			wantError: false,
		},
		{
			name:      "no rules",
			rules:     []model.SigmaRule{},
			wantError: true,
		},
		{
			name: "multiple rules",
			rules: []model.SigmaRule{
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

// No query testing in this test
func TestIntegratorRun(t *testing.T) {
	tests := []struct {
		name                string
		conversionName      string
		convOutput          model.ConversionOutput
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
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv1",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []model.SigmaRule{
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
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv2",
				Queries: []string{
					"{job=`test1`} | json",
					"{job=`test2`} | json",
				},
				Rules: []model.SigmaRule{
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
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv4",
				Queries:        []string{},
				Rules: []model.SigmaRule{
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
			convOutput: model.ConversionOutput{
				ConversionName: "test_conv5",
				Queries:        []string{"{job=`test`} | json"},
				Rules: []model.SigmaRule{
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
			convOutput: model.ConversionOutput{
				ConversionName: "test_annotations",
				Queries:        []string{"{job=`test`} | json", "{service=`api`} | json"},
				Rules: []model.SigmaRule{
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
			convOutput: model.ConversionOutput{
				ConversionName: "orphaned_test",
				Queries:        []string{"{job=`orphaned`} | json"},
				Rules: []model.SigmaRule{
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
			conversions := []model.ConversionConfig{}

			// For orphaned cleanup test cases, don't include the conversion in config
			if !tt.wantOrphanedCleanup {
				conversions = []model.ConversionConfig{
					{
						Name:       tt.conversionName,
						RuleGroup:  "Test Rules",
						TimeWindow: "5m",
						Lookback:   "2m",
					},
				}
			}

			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: model.ConversionConfig{
					Target:     "loki",
					DataSource: "test-datasource",
				},
				Conversions: conversions,
				IntegratorConfig: model.IntegrationConfig{
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
				dummyRule := &model.ProvisionedAlertRule{
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
				dummyRule := &model.ProvisionedAlertRule{
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
				testFiles:    []string{}, // No query testing in this test
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
			rule := &model.ProvisionedAlertRule{}
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
		name             string
		showLogLines     bool
		showSampleValues bool
		wantLine         bool
		wantValues       bool
	}{
		{
			name:             "with log lines and sample values",
			showLogLines:     true,
			showSampleValues: true,
			wantLine:         true,
			wantValues:       true,
		},
		{
			name:             "with log lines and without sample values",
			showLogLines:     true,
			showSampleValues: false,
			wantLine:         true,
			wantValues:       false,
		},
		{
			name:             "without log lines but with sample values",
			showLogLines:     false,
			showSampleValues: true,
			wantLine:         false,
			wantValues:       true,
		},
		{
			name:             "without log lines or sample values",
			showLogLines:     false,
			showSampleValues: false,
			wantLine:         false,
			wantValues:       false,
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
			convOutput := model.ConversionOutput{
				Queries:        testQueries,
				ConversionName: "test_loki",
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Loki Rule",
					},
				},
			}

			// Create test configuration with query testing enabled
			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: model.ConversionConfig{
					Target:     "loki",
					DataSource: "test-loki-datasource",
				},
				Conversions: []model.ConversionConfig{
					{
						Name:       "test_loki",
						RuleGroup:  "Loki Test Rules",
						TimeWindow: "5m",
						DataSource: "test-loki-datasource",
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:         "test-folder",
					OrgID:            1,
					TestQueries:      true,
					From:             "now-1h",
					To:               "now",
					ShowLogLines:     tt.showLogLines,
					ShowSampleValues: tt.showSampleValues,
				},
				DeployerConfig: model.DeploymentConfig{
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
				testFiles:    []string{convFile},
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
			datasourceType: shared.Loki,
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
			datasourceType: shared.Elasticsearch,
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
			convOutput := model.ConversionOutput{
				Queries:        testQueries,
				ConversionName: "test_explore_link",
				Rules: []model.SigmaRule{
					{
						ID:    "996f8884-9144-40e7-ac63-29090ccde9a0",
						Title: "Test Explore Link Rule",
					},
				},
			}

			// Create test configuration with query testing enabled
			config := model.Configuration{
				Folders: model.FoldersConfig{
					ConversionPath: convPath,
					DeploymentPath: deployPath,
				},
				ConversionDefaults: model.ConversionConfig{
					Target:         tt.datasourceType,
					DataSource:     tt.datasource,
					DataSourceType: tt.datasourceType,
				},
				Conversions: []model.ConversionConfig{
					{
						Name:           "test_explore_link",
						RuleGroup:      "Explore Link Test Rules",
						TimeWindow:     "5m",
						DataSource:     tt.datasource,
						DataSourceType: tt.datasourceType,
					},
				},
				IntegratorConfig: model.IntegrationConfig{
					FolderID:    "test-folder",
					OrgID:       1,
					TestQueries: true,
					From:        "now-1h",
					To:          "now",
				},
				DeployerConfig: model.DeploymentConfig{
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
				testFiles:    []string{convFile},
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
		})
	}
}
