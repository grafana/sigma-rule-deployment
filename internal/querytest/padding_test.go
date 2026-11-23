package querytest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/integrate"
	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMetricQuery = "sum by (accountId) (count_over_time({job=`test`} | json [$__auto]))"

	// A metric query that matched nothing, padded so that Loki still reports its statistics
	paddedEmptyMetricResponse = `{"results":{"A0":{"frames":[{"schema":{"meta":{"stats":[` +
		`{"displayName":"Summary: total bytes processed","unit":"decbytes","value":1234},` +
		`{"displayName":"Summary: exec time","unit":"s","value":0.5}]},` +
		`"fields":[{"name":"Time","type":"time"},{"name":"Value","type":"number"}]},` +
		`"data":{"values":[[1000,2000,3000],[0,0,0]]}}]}}}`

	// The same, but with matches at two of the three steps, plus the zero padding series
	paddedMatchingMetricResponse = `{"results":{"A0":{"frames":[` +
		`{"schema":{"meta":{"stats":[{"displayName":"Summary: exec time","unit":"s","value":0.5}]},` +
		`"fields":[{"name":"Time","type":"time"},{"name":"Value","type":"number"}]},` +
		`"data":{"values":[[1000,2000,3000],[3,0,5]]}},` +
		`{"schema":{"meta":{"stats":[{"displayName":"Summary: exec time","unit":"s","value":0.5}]},` +
		`"fields":[{"name":"Time","type":"time"},{"name":"Value","type":"number"}]},` +
		`"data":{"values":[[1000,2000,3000],[0,0,0]]}}]}}}`
)

// recordingDatasourceQuery records the queries it is asked to execute, and responds to them all alike
type recordingDatasourceQuery struct {
	response string
	queries  []string
}

func (r *recordingDatasourceQuery) GetDatasource(dsName, _, _ string, _ time.Duration) (*integrate.GrafanaDatasource, error) {
	return &integrate.GrafanaDatasource{UID: dsName, Type: "loki", ID: 1}, nil
}

func (r *recordingDatasourceQuery) ExecuteQuery(query, _, _, _, _, _, _, _ string, _ time.Duration) ([]byte, error) {
	r.queries = append(r.queries, query)
	return []byte(r.response), nil
}

func TestTestQueriesPadsLokiMetricQueries(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		queryModel    string
		query         string
		response      string
		wantExecuted  string
		wantCount     int
		wantStatsKept bool
	}{
		{
			name:          "an empty metric query is padded, keeping its statistics",
			target:        "loki",
			query:         testMetricQuery,
			response:      paddedEmptyMetricResponse,
			wantExecuted:  "(" + testMetricQuery + ") or vector(0)",
			wantCount:     0,
			wantStatsKept: true,
		},
		{
			name:          "the padding is not counted as a result",
			target:        "loki",
			query:         testMetricQuery,
			response:      paddedMatchingMetricResponse,
			wantExecuted:  "(" + testMetricQuery + ") or vector(0)",
			wantCount:     2,
			wantStatsKept: true,
		},
		{
			name:         "a log query is not padded",
			target:       "loki",
			query:        "{job=`test`} | json",
			response:     paddedMatchingMetricResponse,
			wantExecuted: "{job=`test`} | json",
			// Nothing is ignored, as there is no padding
			wantCount:     6,
			wantStatsKept: true,
		},
		{
			name:          "a custom query model is not padded",
			target:        "loki",
			queryModel:    `{"refId":"%[1]s","datasource":{"uid":"%[2]s"},"expr":"%[3]s"}`,
			query:         testMetricQuery,
			response:      paddedMatchingMetricResponse,
			wantExecuted:  testMetricQuery,
			wantCount:     6,
			wantStatsKept: true,
		},
		{
			name:          "a query for another data source type is not padded",
			target:        "elasticsearch",
			query:         "sum by (accountId) (something)",
			response:      paddedMatchingMetricResponse,
			wantExecuted:  "sum by (accountId) (something)",
			wantCount:     6,
			wantStatsKept: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := model.Configuration{
				Defaults: model.ConfigBlock{
					Conversion: model.ConversionConfig{Target: tt.target},
					Integration: model.IntegrationConfig{
						DataSource: "test-datasource",
						QueryModel: tt.queryModel,
						OrgID:      1,
						From:       "now-1h",
						To:         "now",
					},
					Deployment: model.DeploymentConfig{
						GrafanaInstance: model.GrafanaInstances{"https://test.grafana.com"},
					},
				},
			}

			mock := &recordingDatasourceQuery{response: tt.response}
			original := integrate.DefaultDatasourceQuery
			integrate.DefaultDatasourceQuery = mock
			defer func() { integrate.DefaultDatasourceQuery = original }()

			results, err := NewQueryTester(config, nil, 5*time.Second).TestQueries(
				map[string]string{"A0": tt.query},
				model.NamedConfigBlock{Name: "test_conv"},
				config.Defaults, false,
			)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// The query is only executed once, and only the executed query is padded
			assert.Equal(t, []string{tt.wantExecuted}, mock.queries)
			assert.NotContains(t, results[0].Link, "vector")

			assert.Equal(t, tt.wantCount, results[0].Stats.Count)
			if tt.wantStatsKept {
				assert.InDelta(t, 0.5, results[0].Stats.ExecutionTime.Value, 0)
				assert.Equal(t, "s", results[0].Stats.ExecutionTime.Unit)
			}
		})
	}
}

func TestTestQueriesReportsBytesProcessedOfAnEmptyMetricQuery(t *testing.T) {
	config := model.Configuration{
		Defaults: model.ConfigBlock{
			Conversion:  model.ConversionConfig{Target: "loki"},
			Integration: model.IntegrationConfig{DataSource: "test-datasource", OrgID: 1, From: "now-1h", To: "now"},
			Deployment:  model.DeploymentConfig{GrafanaInstance: model.GrafanaInstances{"https://test.grafana.com"}},
		},
	}

	mock := &recordingDatasourceQuery{response: paddedEmptyMetricResponse}
	original := integrate.DefaultDatasourceQuery
	integrate.DefaultDatasourceQuery = mock
	defer func() { integrate.DefaultDatasourceQuery = original }()

	results, err := NewQueryTester(config, nil, 5*time.Second).TestQueries(
		map[string]string{"A0": testMetricQuery},
		model.NamedConfigBlock{Name: "test_conv"},
		config.Defaults, false,
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].Stats.Count)
	assert.InDelta(t, 1234, results[0].Stats.BytesProcessed.Value, 0)
	assert.Equal(t, "decbytes", results[0].Stats.BytesProcessed.Unit)
}

func TestProcessFrameIgnoreZeroValues(t *testing.T) {
	var frame model.Frame
	require.NoError(t, json.Unmarshal([]byte(
		`{"schema":{"fields":[{"name":"Time","type":"time"},{"name":"Value","type":"number"}]},`+
			`"data":{"values":[[1,2,3,4],[0,7,0,9]]}}`), &frame))

	tests := []struct {
		name             string
		ignoreZeroValues bool
		wantCount        int
	}{
		{name: "counts every row by default", ignoreZeroValues: false, wantCount: 4},
		{name: "ignores zero values when asked to", ignoreZeroValues: true, wantCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.QueryTestResult{Stats: model.Stats{Fields: map[string]string{}}}
			require.NoError(t, ProcessFrame(frame, &result, false, false, tt.ignoreZeroValues))
			assert.Equal(t, tt.wantCount, result.Stats.Count)
		})
	}

	t.Run("ignores nothing in a frame without a Value field", func(t *testing.T) {
		var logFrame model.Frame
		require.NoError(t, json.Unmarshal([]byte(
			`{"schema":{"fields":[{"name":"Time","type":"time"},{"name":"body","type":"string"}]},`+
				`"data":{"values":[[0,0],["a","b"]]}}`), &logFrame))
		result := model.QueryTestResult{Stats: model.Stats{Fields: map[string]string{}}}
		require.NoError(t, ProcessFrame(logFrame, &result, false, false, true))
		assert.Equal(t, 2, result.Stats.Count)
	})
}
