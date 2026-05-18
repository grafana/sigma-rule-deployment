package integrate

import (
	"encoding/json"
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
		{
			name:  "log query stream selector",
			query: `{name="gh-audit-logs"} | json`,
			want:  false,
		},
		{
			name:  "event_count correlation",
			query: `sum by (event_actor) (count_over_time({name="gh-audit-logs"} | json [5m])) > 3`,
			want:  true,
		},
		{
			name:  "value_count correlation",
			query: `count without (event_repo) (sum by (event_actor, event_repo) (count_over_time({name="gh-audit-logs"} | json [5m]))) > 3`,
			want:  true,
		},
		{
			name:  "leading whitespace metric query",
			query: `  count without (event_repo) (sum by (event_actor) (count_over_time({name="x"} [5m]))) > 1`,
			want:  true,
		},
		{
			name:  "avg aggregation",
			query: `avg by (host) (rate({job="app"}[5m]))`,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isLokiMetricQuery(tt.query))
		})
	}
}

func TestCreateAlertQuery_LokiValueCountCorrelation(t *testing.T) {
	correlationQuery := `count without (event_repo) (sum by (event_actor, event_repo) (count_over_time({name=` + "`gh-audit-logs`" + `,action=` + "`repo.download_zip`" + `} | json [5m]))) > 3`

	alertQuery, err := createAlertQuery(
		correlationQuery,
		"A0",
		"grafanacloud-logs",
		model.RelativeTimeRange{From: model.Duration(5 * time.Minute), To: 0},
		model.ConversionConfig{Target: "loki", DataSource: "grafanacloud-logs"},
		model.ConversionConfig{Target: "loki", DataSource: "grafanacloud-logs"},
	)
	require.NoError(t, err)

	var modelFields map[string]any
	require.NoError(t, json.Unmarshal(alertQuery.Model, &modelFields))

	expr, ok := modelFields["expr"].(string)
	require.True(t, ok)
	assert.Equal(t, correlationQuery, expr)
	assert.NotContains(t, expr, "sum(count_over_time(count without")
}
