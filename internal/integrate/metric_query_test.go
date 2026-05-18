package integrate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsLokiMetricQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "log query stream selector", query: `{name="gh-audit-logs"} | json`, want: false},
		{name: "log query with line filters", query: `{job=~".+"} |~ "error" | json | level="error"`, want: false},
		{name: "log query with label filter", query: `{name="okta-logs",eventType="user.session.start"} | json | event_outcome="success"`, want: false},

		{name: "sum by event_count correlation", query: `sum by (event_actor) (count_over_time({name="gh-audit-logs"} | json [5m])) > 3`, want: true},
		{name: "sum without aggregation", query: `sum without (instance) (count_over_time({job="app"} | json [1h]))`, want: true},
		{name: "value_count correlation", query: `count without (event_repo) (sum by (event_actor, event_repo) (count_over_time({name="gh-audit-logs"} | json [5m]))) > 3`, want: true},
		{name: "count with leading whitespace", query: `  count without (event_repo) (sum by (event_actor) (count_over_time({name="x"} [5m]))) > 1`, want: true},
		{name: "avg by host", query: `avg by (host) (rate({job="app"} | json [5m]))`, want: true},
		{name: "min over time", query: `min by (pod) (min_over_time({namespace="prod"} | json | unwrap bytes [5m]))`, want: true},
		{name: "max over time", query: `max by (pod) (max_over_time({namespace="prod"} | json | unwrap bytes [5m]))`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isLokiMetricQuery(tt.query))
		})
	}
}

func TestCreateAlertQuery_LokiWrapping(t *testing.T) {
	t.Parallel()

	lokiConfig := model.ConversionConfig{Target: "loki", DataSource: "grafanacloud-logs"}
	timerange := model.RelativeTimeRange{From: model.Duration(5 * time.Minute), To: 0}

	tests := []struct {
		name      string
		input     string
		wantWrap  bool
		wantExact string
	}{
		{
			name:      "wraps standard log query",
			input:     `{job=~".+"} | json | test="true"`,
			wantWrap:  true,
			wantExact: `sum(count_over_time({job=~".+"} | json | test="true"[$__auto]))`,
		},
		{
			name:     "wraps github audit log query",
			input:    "{name=`gh-audit-logs`,action=`repo.download_zip`} | json | event_action=~`(?i)^repo\\.download_zip$`",
			wantWrap: true,
			wantExact: "sum(count_over_time({name=`gh-audit-logs`,action=`repo.download_zip`} | json | event_action=~`(?i)^repo\\.download_zip$`[$__auto]))",
		},
		{
			name:      "preserves event_count correlation metric query",
			input:     `sum by (event_actor) (count_over_time({name="gh-audit-logs"} | json [5m])) > 3`,
			wantWrap:  false,
			wantExact: `sum by (event_actor) (count_over_time({name="gh-audit-logs"} | json [5m])) > 3`,
		},
		{
			name:      "preserves value_count correlation metric query",
			input:     `count without (event_repo) (sum by (event_actor, event_repo) (count_over_time({name="gh-audit-logs"} | json [5m]))) > 3`,
			wantWrap:  false,
			wantExact: `count without (event_repo) (sum by (event_actor, event_repo) (count_over_time({name="gh-audit-logs"} | json [5m]))) > 3`,
		},
		{
			name:      "preserves avg metric query",
			input:     `avg by (host) (rate({job="app"} | json [5m])) > 0.5`,
			wantWrap:  false,
			wantExact: `avg by (host) (rate({job="app"} | json [5m])) > 0.5`,
		},
		{
			name:      "preserves min metric query",
			input:     `min by (pod) (min_over_time({namespace="prod"} | json [5m]))`,
			wantWrap:  false,
			wantExact: `min by (pod) (min_over_time({namespace="prod"} | json [5m]))`,
		},
		{
			name:      "preserves max metric query",
			input:     `max by (pod) (max_over_time({namespace="prod"} | json [5m]))`,
			wantWrap:  false,
			wantExact: `max by (pod) (max_over_time({namespace="prod"} | json [5m]))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			alertQuery, err := createAlertQuery(tt.input, "A0", "grafanacloud-logs", timerange, lokiConfig, lokiConfig)
			require.NoError(t, err)

			var modelFields map[string]any
			require.NoError(t, json.Unmarshal(alertQuery.Model, &modelFields))

			expr, ok := modelFields["expr"].(string)
			require.True(t, ok)
			assert.Equal(t, tt.wantExact, expr)

			if tt.wantWrap {
				assert.True(t, strings.HasPrefix(expr, "sum(count_over_time("))
				assert.Contains(t, expr, "[$__auto]")
				assert.False(t, isLokiMetricQuery(tt.input))
			} else {
				assert.True(t, isLokiMetricQuery(tt.input))
				assert.NotContains(t, expr, "[$__auto]")
			}
		})
	}
}
