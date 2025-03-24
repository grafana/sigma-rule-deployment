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

func (m *MockDatasourceQuery) GetDatasource(dsName, baseURL, apiKey string, timeout time.Duration) (*GrafanaDatasource, error) {
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

func (m *MockDatasourceQuery) ExecuteQuery(query, dsName, baseURL, apiKey, from, to string, timeout time.Duration) ([]byte, error) {
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
