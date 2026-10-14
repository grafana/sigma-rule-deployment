package integrate

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinQueryModels(t *testing.T) {
	const (
		refID = "A0"
		uid   = "ds-uid"
		query = `field="value with \"quotes\""`
	)

	// Target names that should resolve to each plugin type, in addition to the type itself
	aliases := map[string][]string{
		dsOpenSearch: {"opensearch"},
		dsQuickwit:   {"quickwit"},
		dsClickHouse: {"clickhouse"},
		dsParseable:  {"parseable"},
		dsSQLite:     {"sqlite"},
		dsSurrealDB:  {"surrealql"},
		dsDataset:    {"sentinelone-pq"},
		dsSplunk:     {"splunk"},
		dsDatadog:    {"datadog"},
	}

	// Where each plugin keeps the query string in its model
	queryField := map[string]func(map[string]any) any{
		dsOpenSearch:    func(m map[string]any) any { return m["query"] },
		dsQuickwit:      func(m map[string]any) any { return m["query"] },
		dsClickHouse:    func(m map[string]any) any { return m["rawSql"] },
		dsParseable:     func(m map[string]any) any { return m["queryText"] },
		dsSQLite:        func(m map[string]any) any { return m["queryText"] },
		dsSurrealDB:     func(m map[string]any) any { return m["surql"] },
		dsDataset:       func(m map[string]any) any { return m["expression"] },
		dsAzureDataExpl: func(m map[string]any) any { return m["query"] },
		dsSplunk:        func(m map[string]any) any { return m["query"] },
		dsDatadog: func(m map[string]any) any {
			return m["logsSearchQuery"].(map[string]any)["filter_query"]
		},
	}
	require.Len(t, queryField, len(builtinQueryModels), "every built-in model needs a query field check")

	for pluginType := range builtinQueryModels {
		names := append([]string{pluginType}, aliases[pluginType]...)
		for _, name := range names {
			t.Run(name, func(t *testing.T) {
				alertQuery, err := createAlertQuery(query, refID, uid, model.RelativeTimeRange{},
					model.ConversionConfig{DataSourceType: name}, model.ConversionConfig{})
				require.NoError(t, err)

				var got map[string]any
				require.NoError(t, json.Unmarshal(alertQuery.Model, &got), "model must be valid JSON: %s", alertQuery.Model)

				assert.Equal(t, refID, got["refId"])
				assert.Equal(t, map[string]any{"type": pluginType, "uid": uid}, got["datasource"])
				assert.Equal(t, query, queryField[pluginType](got), "the query must survive JSON escaping unchanged")
			})
		}
	}
}

func TestBuiltinQueryModelPrecedence(t *testing.T) {
	// A custom query_model always wins over a built-in one
	alertQuery, err := createAlertQuery("q", "A0", "uid", model.RelativeTimeRange{},
		model.ConversionConfig{DataSourceType: dsSplunk, QueryModel: `{"refId":"%s","datasource":{"uid":"%s"},"custom":"%s"}`},
		model.ConversionConfig{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"refId":"A0","datasource":{"uid":"uid"},"custom":"q"}`, string(alertQuery.Model))
}

func TestBuiltinQueryModelNotAliasedForAmbiguousTargets(t *testing.T) {
	// kusto can run on both Azure Monitor and Azure Data Explorer, so it must not pick one
	_, ok := BuiltinQueryModel("kusto")
	assert.False(t, ok)
}

func TestTestQueryUsesBuiltinModels(t *testing.T) {
	const (
		baseURL = "http://grafana:3000"
		dsUID   = "ds-uid"
		query   = `field="value with \"quotes\""`
	)

	for pluginType := range builtinQueryModels {
		t.Run(pluginType, func(t *testing.T) {
			httpmock.Activate(t)
			defer httpmock.DeactivateAndReset()

			datasourceJSON, err := json.Marshal(&GrafanaDatasource{UID: dsUID, Name: "test-ds", Type: pluginType})
			require.NoError(t, err)
			httpmock.RegisterResponder("GET", baseURL+"/api/datasources/uid/test-ds",
				httpmock.NewStringResponder(200, string(datasourceJSON)))

			var sent map[string]any
			httpmock.RegisterResponder("POST", baseURL+"/api/ds/query", func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				if err := json.Unmarshal(body, &sent); err != nil {
					return nil, err
				}
				return httpmock.NewStringResponse(200, `{"results":{}}`), nil
			})

			_, err = TestQuery(query, "test-ds", baseURL, "test-api-key", "A", "now-1h", "now", "", 5*time.Second)
			require.NoError(t, err)

			queries, ok := sent["queries"].([]any)
			require.True(t, ok, "request should contain a queries array")
			require.Len(t, queries, 1)
			sentQuery := queries[0].(map[string]any)

			// It should be the same model that the integrator puts in the alert rule
			expected, err := createAlertQuery(query, "A", dsUID, model.RelativeTimeRange{},
				model.ConversionConfig{DataSourceType: pluginType}, model.ConversionConfig{})
			require.NoError(t, err)
			var expectedModel map[string]any
			require.NoError(t, json.Unmarshal(expected.Model, &expectedModel))
			assert.Equal(t, expectedModel, sentQuery)
			assert.Equal(t, "now-1h", sent["from"])
			assert.Equal(t, "now", sent["to"])
		})
	}
}
