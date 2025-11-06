package querytest

import (
	"net/url"
	"testing"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/stretchr/testify/assert"
)

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
			datasourceType: shared.Loki,
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
			datasourceType: shared.Elasticsearch,
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
			datasourceType: shared.Loki,
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
			// Test GenerateExploreLink
			exploreLink, err := GenerateExploreLink(
				tt.query,
				tt.datasource,
				tt.datasourceType,
				model.ConversionConfig{},
				model.ConversionConfig{},
				tt.grafanaURL,
				tt.from,
				tt.to,
				tt.orgID,
			)

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
			for _, notExpected := range tt.wantPanesNotContains {
				assert.NotContains(t, decodedPanes, notExpected, "Decoded panes should not contain: %s", notExpected)
			}
		})
	}
}
