package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDatasourceQuery implements the DatasourceQuery interface for testing
type MockDatasourceQuery struct {
	mockDatasources map[string]*GrafanaDatasource
	mockResponses   map[string][]byte
	dsQueries       []string
	execQueries     []string
}

func NewMockDatasourceQuery() *MockDatasourceQuery {
	return &MockDatasourceQuery{
		mockDatasources: make(map[string]*GrafanaDatasource),
		mockResponses:   make(map[string][]byte),
		dsQueries:       []string{},
		execQueries:     []string{},
	}
}

func (m *MockDatasourceQuery) AddMockDatasource(name string, ds *GrafanaDatasource) {
	m.mockDatasources[name] = ds
}

func (m *MockDatasourceQuery) AddMockResponse(query string, response []byte) {
	m.mockResponses[query] = response
}

func (m *MockDatasourceQuery) GetDatasource(dsName, _, _ string, _ time.Duration) (*GrafanaDatasource, error) {
	m.dsQueries = append(m.dsQueries, dsName)

	if ds, exists := m.mockDatasources[dsName]; exists {
		return ds, nil
	}

	// Default response if not mocked specifically
	return &GrafanaDatasource{
		UID:  "default-uid",
		Type: "loki",
		Name: dsName,
	}, nil
}

func (m *MockDatasourceQuery) ExecuteQuery(query, _, _, _, _, _ string, _ time.Duration) ([]byte, error) {
	m.execQueries = append(m.execQueries, query)

	if response, exists := m.mockResponses[query]; exists {
		return response, nil
	}

	// Default response if not mocked specifically
	return []byte(`{"results":{"A":{"frames":[{"schema":{"fields":[{"name":"Time","type":"time"},{"name":"Line","type":"string"}]},"data":{"values":[[1625126400000,1625126460000],["default response","default response"]]}}]}}}`), nil
}

func TestGetDatasourceByName(t *testing.T) {
	// Set up test mock
	mockQuery := NewMockDatasourceQuery()

	// Add mock datasource
	mockQuery.AddMockDatasource("test-datasource", &GrafanaDatasource{
		ID:     1,
		UID:    "abc123",
		OrgID:  1,
		Name:   "test-datasource",
		Type:   "loki",
		Access: "proxy",
		URL:    "http://loki:3100",
	})

	// Save and restore the default executor
	originalExecutor := DefaultDatasourceQuery
	DefaultDatasourceQuery = mockQuery
	defer func() {
		DefaultDatasourceQuery = originalExecutor
	}()

	// Test successful case
	ds, err := GetDatasourceByName("test-datasource", "http://grafana:3000", "test-api-key", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "abc123", ds.UID)
	assert.Equal(t, "loki", ds.Type)
	assert.Equal(t, "test-datasource", ds.Name)

	// Verify the query was made
	assert.Contains(t, mockQuery.dsQueries, "test-datasource")
}

func TestTestQuery(t *testing.T) {
	// Set up test mock
	mockQuery := NewMockDatasourceQuery()

	// Add mock datasource
	mockQuery.AddMockDatasource("test-datasource", &GrafanaDatasource{
		ID:     1,
		UID:    "xyz789",
		OrgID:  1,
		Name:   "test-datasource",
		Type:   "loki",
		Access: "proxy",
		URL:    "http://loki:3100",
	})

	// Add mock query responses
	mockQuery.AddMockResponse("{job=\"loki\"} |= \"error\"", []byte(`{
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
	}`))

	// Save and restore the default executor
	originalQuery := DefaultDatasourceQuery
	DefaultDatasourceQuery = mockQuery
	defer func() {
		DefaultDatasourceQuery = originalQuery
	}()

	// Test successful case
	result, err := TestQuery("{job=\"loki\"} |= \"error\"", "test-datasource", "http://grafana:3000", "test-api-key", "now-1h", "now", 5*time.Second)
	require.NoError(t, err)

	// Verify the result contains expected data
	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	results, ok := response["results"].(map[string]interface{})
	require.True(t, ok)
	a, ok := results["A"].(map[string]interface{})
	require.True(t, ok)
	frames, ok := a["frames"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, frames)

	// Verify the query was made
	assert.Contains(t, mockQuery.execQueries, "{job=\"loki\"} |= \"error\"")

	// Test with a different query that uses the default response
	result, err = TestQuery("{job=\"loki\"} |= \"debug\"", "test-datasource", "http://grafana:3000", "test-api-key", "now-1h", "now", 5*time.Second)
	require.NoError(t, err)

	// Verify the query was made
	assert.Contains(t, mockQuery.execQueries, "{job=\"loki\"} |= \"debug\"")

	// Verify we got a result (the default in this case)
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)
	_, ok = response["results"]
	require.True(t, ok)
}

func TestTestQueryElasticsearch(t *testing.T) {
	// Set up test mock
	mockQuery := NewMockDatasourceQuery()

	// Add mock Elasticsearch datasource
	mockQuery.AddMockDatasource("test-elasticsearch", &GrafanaDatasource{
		ID:     71,
		UID:    "dej6qd07cf8cgc",
		OrgID:  1,
		Name:   "test-elasticsearch",
		Type:   "elasticsearch",
		Access: "proxy",
		URL:    "http://elasticsearch:9200",
	})

	// Add mock query response with minified Elasticsearch response
	mockQuery.AddMockResponse("type:log AND (level:(ERROR OR FATAL OR CRITICAL))", []byte(`{
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
								"typeInfo": {
									"frame": "time.Time"
								}
							},
							{
								"name": "Value",
								"type": "number",
								"typeInfo": {
									"frame": "float64",
									"nullable": true
								}
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
	}`))

	// Save and restore the default executor
	originalQuery := DefaultDatasourceQuery
	DefaultDatasourceQuery = mockQuery
	defer func() {
		DefaultDatasourceQuery = originalQuery
	}()

	// Test successful case
	result, err := TestQuery("type:log AND (level:(ERROR OR FATAL OR CRITICAL))", "test-elasticsearch", "http://grafana:3000", "test-api-key", "1758615188601", "1758618788601", 5*time.Second)
	require.NoError(t, err)

	// Verify the result contains expected data
	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	results, ok := response["results"].(map[string]interface{})
	require.True(t, ok)
	a, ok := results["A"].(map[string]interface{})
	require.True(t, ok)

	// Verify status
	status, ok := a["status"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(200), status)

	// Verify frames structure
	frames, ok := a["frames"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, frames)

	frame, ok := frames[0].(map[string]interface{})
	require.True(t, ok)

	// Verify schema
	schema, ok := frame["schema"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Count", schema["name"])
	assert.Equal(t, "A", schema["refId"])

	// Verify data structure
	data, ok := frame["data"].(map[string]interface{})
	require.True(t, ok)
	values, ok := data["values"].([]interface{})
	require.True(t, ok)
	assert.Len(t, values, 2) // Time and Value arrays

	// Verify the query was made
	assert.Contains(t, mockQuery.execQueries, "type:log AND (level:(ERROR OR FATAL OR CRITICAL))")
}

func TestElasticsearchQueryStructure(t *testing.T) {
	// Test that the query structure is correctly built for Elasticsearch

	// Create a mock datasource
	ds := &GrafanaDatasource{
		ID:   71,
		UID:  "dej6qd07cf8cgc",
		Type: "elasticsearch",
	}

	// Test the query structure by examining what would be sent
	queryStr := "type:log AND (level:(ERROR OR FATAL OR CRITICAL))"

	// Build the expected query object for Elasticsearch
	expectedQuery := Query{
		RefID: "A",
		Query: queryStr,
		Datasource: GrafanaDatasource{
			Type: ds.Type,
			UID:  ds.UID,
		},
		Metrics: []Metric{
			{
				Type: "count",
				ID:   "1",
			},
		},
		BucketAggs: []BucketAgg{
			{
				Type: "date_histogram",
				ID:   "2",
				Settings: map[string]any{
					"interval": "auto",
				},
				Field: "@timestamp",
			},
		},
		TimeField:     "@timestamp",
		DatasourceID:  ds.ID,
		IntervalMs:    2000,
		MaxDataPoints: 100,
	}

	expectedBody := Body{
		Queries: []Query{expectedQuery},
		From:    "1758615188601",
		To:      "1758618788601",
	}

	// Marshal to JSON to verify the structure
	jsonData, err := json.MarshalIndent(expectedBody, "", "  ")
	require.NoError(t, err)

	// Verify the JSON contains the expected Elasticsearch-specific fields
	var parsedBody map[string]interface{}
	err = json.Unmarshal(jsonData, &parsedBody)
	require.NoError(t, err)

	queries, ok := parsedBody["queries"].([]interface{})
	require.True(t, ok)
	require.Len(t, queries, 1)

	query, ok := queries[0].(map[string]interface{})
	require.True(t, ok)

	// Verify Elasticsearch-specific fields are present
	assert.Equal(t, queryStr, query["query"])
	// alias is empty string, so it should be omitted in JSON (omitempty)
	_, hasAlias := query["alias"]
	assert.False(t, hasAlias, "Empty alias should be omitted due to omitempty tag")
	assert.Equal(t, "@timestamp", query["timeField"])
	assert.Equal(t, float64(71), query["datasourceId"])

	// Verify metrics structure
	metrics, ok := query["metrics"].([]interface{})
	require.True(t, ok)
	require.Len(t, metrics, 1)

	metric, ok := metrics[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "count", metric["type"])
	assert.Equal(t, "1", metric["id"])

	// Verify bucketAggs structure
	bucketAggs, ok := query["bucketAggs"].([]interface{})
	require.True(t, ok)
	require.Len(t, bucketAggs, 1)

	bucketAgg, ok := bucketAggs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "date_histogram", bucketAgg["type"])
	assert.Equal(t, "2", bucketAgg["id"])
	assert.Equal(t, "@timestamp", bucketAgg["field"])

	settings, ok := bucketAgg["settings"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "auto", settings["interval"])

	// Verify Loki-specific fields are NOT present (should be omitted)
	_, hasExpr := query["expr"]
	assert.False(t, hasExpr, "Elasticsearch query should not have 'expr' field")

	_, hasQueryType := query["queryType"]
	assert.False(t, hasQueryType, "Elasticsearch query should not have 'queryType' field")

	_, hasMaxLines := query["maxLines"]
	assert.False(t, hasMaxLines, "Elasticsearch query should not have 'maxLines' field")

	_, hasFormat := query["format"]
	assert.False(t, hasFormat, "Elasticsearch query should not have 'format' field")
}
