//nolint:goconst
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

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
			name:           "successful lookup by name",
			dsNameOrUID:    "test-datasource",
			mockEndpoint:   "/api/datasources/name/test-datasource",
			mockStatusCode: 200,
			mockResponse:   `{"id":1,"uid":"abc123","orgId":1,"name":"test-datasource","type":"loki","access":"proxy","url":"http://loki:3100"}`,
			expectedUID:    "abc123",
			expectedType:   Loki,
			expectedName:   "test-datasource",
			expectedError:  false,
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/name/test-datasource": 1,
			},
		},
		{
			name:           "successful lookup by UID",
			dsNameOrUID:    "abc123",
			mockEndpoint:   "/api/datasources/uid/abc123",
			mockStatusCode: 200,
			mockResponse:   `{"id":1,"uid":"abc123","orgId":1,"name":"test-datasource","type":"loki","access":"proxy","url":"http://loki:3100"}`,
			expectedUID:    "abc123",
			expectedType:   Loki,
			expectedName:   "test-datasource",
			expectedError:  false,
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/uid/abc123": 1,
			},
		},
		{
			name:             "datasource not found",
			dsNameOrUID:      "nonexistent-datasource",
			mockEndpoint:     "/api/datasources/name/nonexistent-datasource",
			mockStatusCode:   404,
			mockResponse:     `{"message": "Data source not found"}`,
			expectedError:    true,
			expectedErrorMsg: "HTTP error getting datasource: 404 Not Found",
			expectedCallCount: map[string]int{
				"GET http://grafana:3000/api/datasources/name/nonexistent-datasource": 1,
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

func TestTestQueryLoki(t *testing.T) {
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
		Type:   Loki,
		Access: "proxy",
		URL:    "http://loki:3100",
	}

	datasourceJSON, err := json.Marshal(mockDatasource)
	require.NoError(t, err)

	// Mock query response
	mockQueryResponse := map[string]any{
		"results": map[string]any{
			"A": map[string]any{
				"frames": []any{
					map[string]any{
						"schema": map[string]any{
							"fields": []any{
								map[string]any{"name": "Time", "type": "time"},
								map[string]any{"name": "Line", "type": "string"},
							},
						},
						"data": map[string]any{
							"values": []any{
								[]any{1625126400000, 1625126460000},
								[]any{"error log line", "another error log"},
							},
						},
					},
				},
			},
		},
	}

	queryResponseJSON, err := json.Marshal(mockQueryResponse)
	require.NoError(t, err)

	// Register mocks
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
		httpmock.NewStringResponder(200, string(queryResponseJSON)))

	// Test successful case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)
	require.NoError(t, err)

	// Verify the result contains expected data
	var response map[string]any
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	results, ok := response["results"].(map[string]any)
	require.True(t, ok)
	a, ok := results["A"].(map[string]any)
	require.True(t, ok)
	frames, ok := a["frames"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, frames)

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-loki"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}

func TestTestQueryElasticsearch(t *testing.T) {
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
		Type:   Elasticsearch,
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
				"frames": []any{
					map[string]any{
						"schema": map[string]any{
							"name":  "Count",
							"refId": "A",
							"meta": map[string]any{
								"type":        "timeseries-multi",
								"typeVersion": []any{0, 0},
							},
							"fields": []any{
								map[string]any{
									"name": "Time",
									"type": "time",
									"typeInfo": map[string]any{
										"frame": "time.Time",
									},
								},
								map[string]any{
									"name": "Value",
									"type": "number",
									"typeInfo": map[string]any{
										"frame":    "float64",
										"nullable": true,
									},
								},
							},
						},
						"data": map[string]any{
							"values": []any{
								[]any{1758615188000, 1758615190000, 1758615192000, 1758615194000, 1758615196000},
								[]any{2, 0, 0, 1, 0},
							},
						},
					},
				},
			},
		},
	}

	queryResponseJSON, err := json.Marshal(mockQueryResponse)
	require.NoError(t, err)

	// Register mocks
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
		httpmock.NewStringResponder(200, string(queryResponseJSON)))

	// Test successful case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)
	require.NoError(t, err)

	// Verify the result contains expected data
	var response map[string]any
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	results, ok := response["results"].(map[string]any)
	require.True(t, ok)
	a, ok := results["A"].(map[string]any)
	require.True(t, ok)

	// Verify status
	status, ok := a["status"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(200), status)

	// Verify frames structure
	frames, ok := a["frames"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, frames)

	frame, ok := frames[0].(map[string]any)
	require.True(t, ok)

	// Verify schema
	schema, ok := frame["schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Count", schema["name"])
	assert.Equal(t, "A", schema["refId"])

	// Verify data structure
	data, ok := frame["data"].(map[string]any)
	require.True(t, ok)
	values, ok := data["values"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 2) // Time and Value arrays

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-elasticsearch"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}

func TestTestQueryUnsupportedDatasourceType(t *testing.T) {
	// Activate httpmock
	httpmock.Activate(t)
	defer httpmock.DeactivateAndReset()

	baseURL := "http://grafana:3000"
	apiKey := "test-api-key"
	dsName := "test-prometheus"
	query := "up"
	from := "now-1h"
	to := "now"
	timeout := 5 * time.Second

	// Mock datasource response with unsupported type
	mockDatasource := &GrafanaDatasource{
		ID:     1,
		UID:    "prometheus123",
		OrgID:  1,
		Name:   "test-prometheus",
		Type:   "prometheus", // Unsupported datasource type
		Access: "proxy",
		URL:    "http://prometheus:9090",
	}

	datasourceJSON, err := json.Marshal(mockDatasource)
	require.NoError(t, err)

	// Register mock for datasource
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	// Test that ExecuteQuery returns an error for unsupported datasource type
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)

	// Verify that an error is returned
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported datasource type: prometheus")

	// Verify that no result is returned
	assert.Nil(t, result)

	// Verify that only the datasource request was made (query request should not be made)
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-prometheus"])
	assert.Equal(t, 0, info["POST http://grafana:3000/api/ds/query"])
}

func TestTestQueryHTTPError(t *testing.T) {
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
		Type:   Loki,
		Access: "proxy",
		URL:    "http://loki:3100",
	}

	datasourceJSON, err := json.Marshal(mockDatasource)
	require.NoError(t, err)

	// Register mocks
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	// Mock query endpoint to return 500 error
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
		httpmock.NewStringResponder(500, `{"error": "Internal server error"}`))

	// Test error case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "HTTP error 500 when querying datasource")

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-loki"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}

func TestTestQueryInvalidJSONResponse(t *testing.T) {
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
		Type:   Loki,
		Access: "proxy",
		URL:    "http://loki:3100",
	}

	datasourceJSON, err := json.Marshal(mockDatasource)
	require.NoError(t, err)

	// Register mocks
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
		httpmock.NewStringResponder(200, string(datasourceJSON)))

	// Mock query endpoint to return invalid JSON
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/ds/query", baseURL),
		httpmock.NewStringResponder(200, `invalid json response`))

	// Test error case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, "", timeout)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid JSON response")

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-loki"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
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
		Type:   Elasticsearch,
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
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
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
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-elasticsearch"])
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
		Type:   Loki,
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
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/datasources/name/%s", baseURL, dsName),
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
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-loki"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}

func TestTestQueryElasticsearchWithCustomModel(t *testing.T) {
	// Activate httpmock
	httpmock.Activate(t)
	defer httpmock.DeactivateAndReset()

	// Mock datasource response
	httpmock.RegisterResponder("GET", "http://grafana:3000/api/datasources/name/test-elasticsearch",
		httpmock.NewJsonResponderOrPanic(200, map[string]any{
			"id":   1,
			"uid":  "test-elasticsearch-uid",
			"type": "elasticsearch",
			"name": "test-elasticsearch",
		}))

	// Mock query response
	httpmock.RegisterResponder("POST", "http://grafana:3000/api/ds/query",
		func(req *http.Request) (*http.Response, error) {
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
			require.True(t, ok, "Request should contain queries array")
			require.Len(t, queries, 1, "Should have exactly one query")

			// Verify the query structure matches our custom model
			query, ok := queries[0].(map[string]any)
			require.True(t, ok, "Query should be a map")

			// Verify custom fields are present
			assert.Equal(t, "A", query["refId"], "refId should be A")
			assert.Equal(t, "my custom query", query["customQueryField"], "customQueryField should match")
			assert.Equal(t, "customValue", query["customField"], "customField should be present")

			// Verify datasource structure
			datasource, ok := query["datasource"].(map[string]any)
			require.True(t, ok, "Datasource should be a map")
			assert.Equal(t, "elasticsearch", datasource["type"], "Datasource type should be elasticsearch")
			assert.Equal(t, "test-elasticsearch-uid", datasource["uid"], "Datasource UID should match")

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
		})

	// Test parameters
	query := "my custom query"
	dsName := "test-elasticsearch"
	baseURL := "http://grafana:3000"
	apiKey := "test-api-key"
	from := "now-1h"
	to := "now"
	customModel := `{"refId":"%s","datasource":{"type":"elasticsearch","uid":"%s"},"customQueryField":"%s","customField":"customValue"}`
	timeout := 30 * time.Second

	// Test successful case
	result, err := TestQuery(query, dsName, baseURL, apiKey, "A", from, to, customModel, timeout)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the result contains expected data
	var response map[string]any
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	// Verify response structure
	results, ok := response["results"].(map[string]any)
	require.True(t, ok, "Response should contain results")

	resultA, ok := results["A"].(map[string]any)
	require.True(t, ok, "Results should contain A")

	frames, ok := resultA["frames"].([]any)
	require.True(t, ok, "Result A should contain frames")
	require.Len(t, frames, 1, "Should have exactly one frame")

	// Verify the requests were made
	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://grafana:3000/api/datasources/name/test-elasticsearch"])
	assert.Equal(t, 1, info["POST http://grafana:3000/api/ds/query"])
}
