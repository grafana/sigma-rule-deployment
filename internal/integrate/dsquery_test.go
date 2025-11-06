//nolint:goconst
package integrate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDatasource(t *testing.T) {
	tests := []struct {
		name              string
		dsNameOrUID       string
		mockEndpoint      string
		mockStatusCode    int
		mockResponse      string
		expectedUID       string
		expectedType      string
		expectedName      string
		expectedError     bool
		expectedErrorMsg  string
		expectedCallCount map[string]int
	}{
		{
			name:           "successful lookup by UID",
			dsNameOrUID:    "abc123",
			mockEndpoint:   "/api/datasources/uid/abc123",
			mockStatusCode: 200,
			mockResponse:   `{"id":1,"uid":"abc123","orgId":1,"name":"test-datasource","type":"loki","access":"proxy","url":"http://loki:3100"}`,
			expectedUID:    "abc123",
			expectedType:   shared.Loki,
			expectedName:   "test-datasource",
			expectedError:  false,
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/abc123": 1,
			},
		},
		{
			name:             "datasource not found",
			dsNameOrUID:      "nonexistent-datasource",
			mockEndpoint:     "/api/datasources/uid/nonexistent-datasource",
			mockStatusCode:   404,
			mockResponse:     `{"message": "Data source not found"}`,
			expectedError:    true,
			expectedErrorMsg: "HTTP error getting datasource: 404 Not Found",
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/nonexistent-datasource": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Activate httpmock for this subtest
			httpmock.Activate(t)
			defer httpmock.DeactivateAndReset()

			baseURL := "http://grafana:3000"
			apiKey := "test-api-key"
			timeout := 5 * time.Second

			// Register mock for the endpoint
			httpmock.RegisterResponder("GET", fmt.Sprintf("%s%s", baseURL, tt.mockEndpoint),
				httpmock.NewStringResponder(tt.mockStatusCode, tt.mockResponse))

			// Execute the function under test
			ds, err := GetDatasourceByName(tt.dsNameOrUID, baseURL, apiKey, timeout)

			// Verify results
			if tt.expectedError {
				require.Error(t, err)
				assert.Nil(t, ds)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedUID, ds.UID)
				assert.Equal(t, tt.expectedType, ds.Type)
				assert.Equal(t, tt.expectedName, ds.Name)
			}

			// Verify the request was made
			info := httpmock.GetCallCountInfo()
			for url, count := range tt.expectedCallCount {
				assert.Equal(t, count, info[url], "Request count for %s should be %d", url, count)
			}
			for call := range info {
				assert.Contains(t, tt.expectedCallCount, call, "Unexpected request made: %s", call)
			}
		})
	}
}

func TestTestQuery(t *testing.T) {
	tests := []struct {
		name                string
		dsName              string
		query               string
		from                string
		to                  string
		customModel         string
		mockDatasource      *GrafanaDatasource
		mockQueryStatusCode int
		mockQueryResponse   string
		mockQueryResponder  func(req *http.Request) (*http.Response, error)
		expectedError       bool
		expectedErrorMsg    string
		expectedCallCount   map[string]int
	}{
		{
			name:   "successful Loki query",
			dsName: "test-loki",
			query:  `{job="loki"} |= "error"`,
			from:   "now-1h",
			to:     "now",
			mockDatasource: &GrafanaDatasource{
				ID:     1,
				UID:    "loki123",
				OrgID:  1,
				Name:   "test-loki",
				Type:   shared.Loki,
				Access: "proxy",
				URL:    "http://loki:3100",
			},
			mockQueryStatusCode: 200,
			mockQueryResponse: `{
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
									["error log line", "another error log"]
								]
							}
						}]
					}
				}
			}`,
			expectedError: false,
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/test-loki": 1,
				"POST http://grafana:3000/api/ds/query":                 1,
			},
		},
		{
			name:   "successful Elasticsearch query",
			dsName: "test-elasticsearch",
			query:  `type:log AND (level:(ERROR OR FATAL OR CRITICAL))`,
			from:   "1758615188601",
			to:     "1758618788601",
			mockDatasource: &GrafanaDatasource{
				ID:     71,
				UID:    "dej6qd07cf8cgc",
				OrgID:  1,
				Name:   "test-elasticsearch",
				Type:   shared.Elasticsearch,
				Access: "proxy",
				URL:    "http://elasticsearch:9200",
			},
			mockQueryStatusCode: 200,
			mockQueryResponse: `{
				"results": {
					"A": {
						"status": 200,
						"frames": [{
							"schema": {
								"name": "Count",
								"refId": "A",
								"meta": {
									"type": "timeseries-multi",
									"typeVersion": [0, 0]
								},
								"fields": [
									{
										"name": "Time",
										"type": "time",
										"typeInfo": {"frame": "time.Time"}
									},
									{
										"name": "Value",
										"type": "number",
										"typeInfo": {"frame": "float64", "nullable": true}
									}
								]
							},
							"data": {
								"values": [
									[1758615188000, 1758615190000, 1758615192000, 1758615194000, 1758615196000],
									[2, 0, 0, 1, 0]
								]
							}
						}]
					}
				}
			}`,
			expectedError: false,
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/test-elasticsearch": 1,
				"POST http://grafana:3000/api/ds/query":                          1,
			},
		},
		{
			name:   "unsupported datasource type",
			dsName: "test-prometheus",
			query:  "up",
			from:   "now-1h",
			to:     "now",
			mockDatasource: &GrafanaDatasource{
				ID:     1,
				UID:    "prometheus123",
				OrgID:  1,
				Name:   "test-prometheus",
				Type:   "prometheus", // Unsupported datasource type
				Access: "proxy",
				URL:    "http://prometheus:9090",
			},
			expectedError:    true,
			expectedErrorMsg: "unsupported datasource type: prometheus",
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/test-prometheus": 1,
			},
		},
		{
			name:   "HTTP error during query",
			dsName: "test-loki",
			query:  `{job="loki"} |= "error"`,
			from:   "now-1h",
			to:     "now",
			mockDatasource: &GrafanaDatasource{
				ID:     1,
				UID:    "loki123",
				OrgID:  1,
				Name:   "test-loki",
				Type:   shared.Loki,
				Access: "proxy",
				URL:    "http://loki:3100",
			},
			mockQueryStatusCode: 500,
			mockQueryResponse:   `{"error": "Internal server error"}`,
			expectedError:       true,
			expectedErrorMsg:    "HTTP error 500 when querying datasource",
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/test-loki": 1,
				"POST http://grafana:3000/api/ds/query":                 1,
			},
		},
		{
			name:   "invalid JSON response",
			dsName: "test-loki",
			query:  `{job="loki"} |= "error"`,
			from:   "now-1h",
			to:     "now",
			mockDatasource: &GrafanaDatasource{
				ID:     1,
				UID:    "loki123",
				OrgID:  1,
				Name:   "test-loki",
				Type:   shared.Loki,
				Access: "proxy",
				URL:    "http://loki:3100",
			},
			mockQueryStatusCode: 200,
			mockQueryResponse:   `invalid json response`,
			expectedError:       true,
			expectedErrorMsg:    "invalid JSON response",
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/test-loki": 1,
				"POST http://grafana:3000/api/ds/query":                 1,
			},
		},
		{
			name:        "Elasticsearch with custom model",
			dsName:      "test-elasticsearch",
			query:       "my custom query",
			from:        "now-1h",
			to:          "now",
			customModel: `{"refId":"%s","datasource":{"type":"elasticsearch","uid":"%s"},"customQueryField":"%s","customField":"customValue"}`,
			mockDatasource: &GrafanaDatasource{
				ID:     1,
				UID:    "test-elasticsearch-uid",
				OrgID:  1,
				Name:   "test-elasticsearch",
				Type:   shared.Elasticsearch,
				Access: "proxy",
				URL:    "http://elasticsearch:9200",
			},
			mockQueryResponder: func(req *http.Request) (*http.Response, error) {
				// Read the request body to verify the custom model was used
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}

				// Parse the request body to verify structure
				var requestBody map[string]any
				if err := json.Unmarshal(body, &requestBody); err != nil {
					return nil, err
				}

				// Verify the request has queries array
				queries, ok := requestBody["queries"].([]any)
				if !ok {
					return httpmock.NewStringResponse(400, `{"error": "Request should contain queries array"}`), nil
				}
				if len(queries) != 1 {
					return httpmock.NewStringResponse(400, `{"error": "Should have exactly one query"}`), nil
				}

				// Verify the query structure matches our custom model
				query, ok := queries[0].(map[string]any)
				if !ok {
					return httpmock.NewStringResponse(400, `{"error": "Query should be a map"}`), nil
				}

				// Verify custom fields are present
				if query["refId"] != "A" {
					return httpmock.NewStringResponse(400, `{"error": "refId should be A"}`), nil
				}
				if query["customQueryField"] != "my custom query" {
					return httpmock.NewStringResponse(400, `{"error": "customQueryField should match"}`), nil
				}
				if query["customField"] != "customValue" {
					return httpmock.NewStringResponse(400, `{"error": "customField should be present"}`), nil
				}

				// Verify datasource structure
				datasource, ok := query["datasource"].(map[string]any)
				if !ok {
					return httpmock.NewStringResponse(400, `{"error": "Datasource should be a map"}`), nil
				}
				if datasource["type"] != "elasticsearch" {
					return httpmock.NewStringResponse(400, `{"error": "Datasource type should be elasticsearch"}`), nil
				}
				if datasource["uid"] != "test-elasticsearch-uid" {
					return httpmock.NewStringResponse(400, `{"error": "Datasource UID should match"}`), nil
				}

				// Return a mock response
				response := map[string]any{
					"results": map[string]any{
						"A": map[string]any{
							"frames": []any{
								map[string]any{
									"schema": map[string]any{
										"fields": []any{
											map[string]any{"name": "Time", "type": "time"},
											map[string]any{"name": "Count", "type": "number"},
										},
									},
									"data": map[string]any{
										"values": []any{
											[]any{1625126400000, 1625126460000},
											[]any{10, 15},
										},
									},
								},
							},
						},
					},
				}

				return httpmock.NewJsonResponse(200, response)
			},
			expectedError: false,
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/test-elasticsearch": 1,
				"POST http://grafana:3000/api/ds/query":                          1,
			},
		},
		{
			name:   "datasource by name",
			dsName: "nonexistent-datasource",
			query:  "my custom query",
			from:   "now-1h",
			to:     "now",
			mockDatasource: &GrafanaDatasource{
				ID:     1,
				UID:    "no-such-datasource",
				OrgID:  1,
				Name:   "nonexistent-datasource",
				Type:   shared.Loki,
				Access: "proxy",
			},
			mockQueryStatusCode: 404,
			mockQueryResponse:   `{"message": "Data source not found"}`,
			expectedError:       true,
			expectedErrorMsg:    "404 Not Found, Response: {\"message\": \"Data source not found\"}",
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/nonexistent-datasource": 1,
				"POST http://grafana:3000/api/ds/query":                              1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Activate httpmock for this subtest
			httpmock.Activate(t)
			defer httpmock.DeactivateAndReset()

			baseURL := "http://grafana:3000"
			apiKey := "test-api-key"
			timeout := 5 * time.Second

			// Mock datasource response
			datasourceJSON, err := json.Marshal(tt.mockDatasource)
			require.NoError(t, err)

			httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/uid/%s", baseURL, tt.dsName),
				httpmock.NewStringResponder(200, string(datasourceJSON)))

			// Mock query response if needed
			if tt.mockQueryResponder != nil {
				httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL), tt.mockQueryResponder)
			} else if tt.mockQueryStatusCode > 0 {
				httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
					httpmock.NewStringResponder(tt.mockQueryStatusCode, tt.mockQueryResponse))
			}

			// Execute the function under test
			result, err := TestQuery(tt.query, tt.dsName, baseURL, apiKey, "A", tt.from, tt.to, tt.customModel, timeout)

			// Verify results
			if tt.expectedError {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			// Verify the requests were made
			info := httpmock.GetCallCountInfo()
			for url, expectedCount := range tt.expectedCallCount {
				assert.Equal(t, expectedCount, info[url], "Request count for %s should be %d", url, expectedCount)
			}
			for call := range info {
				assert.Contains(t, tt.expectedCallCount, call, "Unexpected request made: %s", call)
			}
		})
	}
}

func TestElasticsearchQueryStructure(t *testing.T) {
	// Activate httpmock
	httpmock.Activate(t)
	defer httpmock.DeactivateAndReset()

	baseURL := "http://grafana:3000"
	apiKey := "test-api-key"
	dsName := "test-elasticsearch"
	query := `type:log AND (level:(ERROR OR FATAL OR CRITICAL))`
	from := "1758615188601"
	to := "1758618788601"
	timeout := 5 * time.Second

	// Mock datasource response
	mockDatasource := &GrafanaDatasource{
		ID:     71,
		UID:    "dej6qd07cf8cgc",
		OrgID:  1,
		Name:   "test-elasticsearch",
		Type:   shared.Elasticsearch,
		Access: "proxy",
		URL:    "http://elasticsearch:9200",
	}

	datasourceJSON, err := json.Marshal(mockDatasource)
	require.NoError(t, err)

	// Mock query response
	mockQueryResponse := map[string]any{
		"results": map[string]any{
			"A": map[string]any{
				"status": 200,
				"frames": []any{},
			},
		},
	}

	queryResponseJSON, err := json.Marshal(mockQueryResponse)
	require.NoError(t, err)

	// Register mocks
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/uid/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	// Capture the request body to verify the query structure
	var capturedRequestBody []byte
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
		func(req *http.Request) (*http.Response, error) {
			// Read the request body
			body := make([]byte, req.ContentLength)
			_, err := req.Body.Read(body)
			require.NoError(t, err)
			capturedRequestBody = body

			return httpmock.NewStringResponse(200, string(queryResponseJSON)), nil
		})

	// Test successful case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the query structure
	require.NotNil(t, capturedRequestBody)
	var requestBody map[string]any
	err = json.Unmarshal(capturedRequestBody, &requestBody)
	require.NoError(t, err)

	// Verify the request body structure
	queries, ok := requestBody["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)

	queryObj, ok := queries[0].(map[string]any)
	require.True(t, ok)

	// Verify Elasticsearch-specific fields are present
	assert.Equal(t, query, queryObj["query"])
	assert.Equal(t, "@timestamp", queryObj["timeField"])
	assert.Equal(t, float64(71), queryObj["datasourceId"])

	// Verify metrics structure
	metrics, ok := queryObj["metrics"].([]any)
	require.True(t, ok)
	require.Len(t, metrics, 1)

	metric, ok := metrics[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "count", metric["type"])
	assert.Equal(t, "1", metric["id"])

	// Verify bucketAggs structure
	bucketAggs, ok := queryObj["bucketAggs"].([]any)
	require.True(t, ok)
	require.Len(t, bucketAggs, 1)

	bucketAgg, ok := bucketAggs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "date_histogram", bucketAgg["type"])
	assert.Equal(t, "2", bucketAgg["id"])
	assert.Equal(t, "@timestamp", bucketAgg["field"])

	settings, ok := bucketAgg["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "auto", settings["interval"])

	// Verify Loki-specific fields are NOT present (should be omitted)
	_, hasExpr := queryObj["expr"]
	assert.False(t, hasExpr, "Elasticsearch query should not have 'expr' field")

	_, hasQueryType := queryObj["queryType"]
	assert.False(t, hasQueryType, "Elasticsearch query should not have 'queryType' field")

	_, hasMaxLines := queryObj["maxLines"]
	assert.False(t, hasMaxLines, "Elasticsearch query should not have 'maxLines' field")

	_, hasFormat := queryObj["format"]
	assert.False(t, hasFormat, "Elasticsearch query should not have 'format' field")

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/uid/test-elasticsearch"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}

func TestLokiQueryStructure(t *testing.T) {
	// Activate httpmock
	httpmock.Activate(t)
	defer httpmock.DeactivateAndReset()

	baseURL := "http://grafana:3000"
	apiKey := "test-api-key"
	dsName := "test-loki"
	query := `{job="loki"} |= "error"`
	from := "now-1h"
	to := "now"
	timeout := 5 * time.Second

	// Mock datasource response
	mockDatasource := &GrafanaDatasource{
		ID:     1,
		UID:    "loki123",
		OrgID:  1,
		Name:   "test-loki",
		Type:   shared.Loki,
		Access: "proxy",
		URL:    "http://loki:3100",
	}

	datasourceJSON, err := json.Marshal(mockDatasource)
	require.NoError(t, err)

	// Mock query response
	mockQueryResponse := map[string]any{
		"results": map[string]any{
			"A": map[string]any{
				"frames": []any{},
			},
		},
	}

	queryResponseJSON, err := json.Marshal(mockQueryResponse)
	require.NoError(t, err)

	// Register mocks
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/uid/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	// Capture the request body to verify the query structure
	var capturedRequestBody []byte
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
		func(req *http.Request) (*http.Response, error) {
			// Read the request body
			body := make([]byte, req.ContentLength)
			_, err := req.Body.Read(body)
			require.NoError(t, err)
			capturedRequestBody = body

			return httpmock.NewStringResponse(200, string(queryResponseJSON)), nil
		})

	// Test successful case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the query structure
	require.NotNil(t, capturedRequestBody)
	var requestBody map[string]any
	err = json.Unmarshal(capturedRequestBody, &requestBody)
	require.NoError(t, err)

	// Verify the request body structure
	queries, ok := requestBody["queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 1)

	queryObj, ok := queries[0].(map[string]any)
	require.True(t, ok)

	// Verify Loki-specific fields are present
	assert.Equal(t, query, queryObj["expr"])
	assert.Equal(t, "range", queryObj["queryType"])
	assert.Equal(t, float64(100), queryObj["maxLines"])
	assert.Equal(t, "time_series", queryObj["format"])

	// Verify Elasticsearch-specific fields are NOT present (should be omitted)
	_, hasQuery := queryObj["query"]
	assert.False(t, hasQuery, "Loki query should not have 'query' field")

	_, hasTimeField := queryObj["timeField"]
	assert.False(t, hasTimeField, "Loki query should not have 'timeField' field")

	_, hasDatasourceID := queryObj["datasourceId"]
	assert.False(t, hasDatasourceID, "Loki query should not have 'datasourceId' field")

	_, hasMetrics := queryObj["metrics"]
	assert.False(t, hasMetrics, "Loki query should not have 'metrics' field")

	_, hasBucketAggs := queryObj["bucketAggs"]
	assert.False(t, hasBucketAggs, "Loki query should not have 'bucketAggs' field")

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/uid/test-loki"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}
