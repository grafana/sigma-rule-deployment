package main

import (
	"os"
	"testing"

	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"github.com/stretchr/testify/assert"
)

func TestConvertToAlert(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		rule          *definitions.ProvisionedAlertRule
		config        ConversionConfig
		wantQueryText string
		wantError     bool
	}{
		{
			name:  "valid new loki query",
			query: "{job=`.+`} | json | test=`true`",
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
			name:  "valid ES query",
			query: `from * | where eventSource=="kms.amazonaws.com" and eventName=="CreateGrant"`,
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
			wantQueryText: `from * | where eventSource=="kms.amazonaws.com" and eventName=="CreateGrant"`,
			wantError:     false,
		},
		{
			name:  "invalid time window",
			query: "{job=`.+`} | json | test=`true`",
			rule: &definitions.ProvisionedAlertRule{
				ID:  0,
				UID: "5c1c217a",
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
			escapedQuery, _ := escapeQueryJSON(tt.wantQueryText)
			err := i.ConvertToAlert(tt.rule, tt.query, tt.config)
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, string(tt.rule.Data[0].Model), escapedQuery)
				assert.Equal(t, tt.config.RuleGroup, tt.rule.RuleGroup)
				assert.Equal(t, tt.config.DataSource, tt.rule.Data[0].DatasourceUID)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		configPath     string
		token          string
		added          string
		deleted        string
		modified       string
		expectedConfig Configuration
		wantError      bool
	}{
		{
			name:       "valid loki config",
			configPath: "testdata/config.yml",
			token:      "my-test-token",
			added:      "testdata/conv.txt",
			deleted:    "",
			modified:   "",
			expectedConfig: Configuration{
				Folders: FoldersConfig{
					ConversionPath: "./conversions",
					DeploymentPath: "./deployments",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("INTEGRATOR_CONFIG_PATH", tt.configPath)
			defer os.Unsetenv("INTEGRATOR_CONFIG_PATH")
			os.Setenv("INTEGRATOR_GRAFANA_SA_TOKEN", tt.token)
			defer os.Unsetenv("INTEGRATOR_GRAFANA_SA_TOKEN")
			os.Setenv("ADDED_FILES", tt.added)
			defer os.Unsetenv("ADDED_FILES")
			os.Setenv("DELETED_FILES", tt.deleted)
			defer os.Unsetenv("DELETED_FILES")
			os.Setenv("MODIFIED_FILES", tt.modified)
			defer os.Unsetenv("MODIFIED_FILES")

			i := NewIntegrator()
			err := i.LoadConfig()
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedConfig, i.config)
			}
		})
	}
}

func TestReadWriteAlertRule(t *testing.T) {
	// A simple test of reading and writing alert rule files
	rule := &definitions.ProvisionedAlertRule{}
	err := readRuleFromFile(rule, "testdata/sample_rule.json")
	assert.NoError(t, err)
	err = writeRuleToFile(rule, "testdata/sample_rule.json")
	assert.NoError(t, err)
}
