package main

import (
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
