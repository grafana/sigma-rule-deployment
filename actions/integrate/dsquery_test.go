package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDatasourceByName(t *testing.T) {
	// Mock server for the datasource API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check request
		assert.Equal(t, "/api/datasources/name/test-datasource", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "GET", r.Method)

		// Mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		mockResponse := GrafanaDatasource{
			ID:     1,
			UID:    "abc123",
			OrgID:  1,
			Name:   "test-datasource",
			Type:   "loki",
			Access: "proxy",
			URL:    "http://loki:3100",
		}
		responseJSON, _ := json.Marshal(mockResponse)
		_, _ = w.Write(responseJSON)
	}))
	defer mockServer.Close()

	// Test successful case
	ds, err := GetDatasourceByName("test-datasource", mockServer.URL, "test-api-key", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "abc123", ds.UID)
	assert.Equal(t, "loki", ds.Type)
	assert.Equal(t, "test-datasource", ds.Name)

	// Error cases
	t.Run("HTTP error", func(t *testing.T) {
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Datasource not found"}`))
		}))
		defer errorServer.Close()

		_, err := GetDatasourceByName("nonexistent", errorServer.URL, "test-api-key", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP error getting datasource")
	})

	t.Run("Empty response", func(t *testing.T) {
		emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer emptyServer.Close()

		_, err := GetDatasourceByName("empty", emptyServer.URL, "test-api-key", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty response from datasource")
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{invalid json`))
		}))
		defer invalidServer.Close()

		_, err := GetDatasourceByName("invalid", invalidServer.URL, "test-api-key", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal response body")
	})
}

func TestTestQuery(t *testing.T) {
	// Mock server setup that handles both datasource and query API calls
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Handle datasource lookup
		if r.URL.Path == "/api/datasources/name/test-datasource" {
			mockDS := GrafanaDatasource{
				ID:     1,
				UID:    "xyz789",
				OrgID:  1,
				Name:   "test-datasource",
				Type:   "loki",
				Access: "proxy",
				URL:    "http://loki:3100",
			}
			responseJSON, _ := json.Marshal(mockDS)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(responseJSON)
			return
		}

		// Handle query request
		if r.URL.Path == "/api/ds/query" {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

			// Verify request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			defer r.Body.Close()

			var queryRequest Body
			err = json.Unmarshal(body, &queryRequest)
			require.NoError(t, err)

			assert.Equal(t, 1, len(queryRequest.Queries))
			assert.Equal(t, "{job=\"test\"} |= \"error\"", queryRequest.Queries[0].Expr)
			assert.Equal(t, "xyz789", queryRequest.Queries[0].Datasource.UID)
			assert.Equal(t, "loki", queryRequest.Queries[0].Datasource.Type)

			// Send mock response
			mockResponse := map[string]any{
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
										[]int64{1625126400000, 1625126460000},
										[]string{"log line with error", "another error log"},
									},
								},
							},
						},
					},
				},
			}
			responseJSON, _ := json.Marshal(mockResponse)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(responseJSON)
			return
		}

		// Unexpected path
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not found"}`))
	}))
	defer mockServer.Close()

	// Test successful case
	result, err := TestQuery("{job=\"test\"} |= \"error\"", "test-datasource", mockServer.URL, "test-api-key", 5*time.Second)
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

	// Error case: datasource API error
	t.Run("datasource API error", func(t *testing.T) {
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Datasource not found"}`))
		}))
		defer errorServer.Close()

		_, err := TestQuery("{job=\"test\"} |= \"error\"", "nonexistent", errorServer.URL, "test-api-key", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get datasource")
	})

	// Error case: query API error
	t.Run("query API error", func(t *testing.T) {
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/datasources/name/test-datasource" {
				// Return valid datasource
				mockDS := GrafanaDatasource{
					UID:  "xyz789",
					Type: "loki",
				}
				responseJSON, _ := json.Marshal(mockDS)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(responseJSON)
				return
			}

			if r.URL.Path == "/api/ds/query" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"Invalid query"}`))
				return
			}
		}))
		defer errorServer.Close()

		_, err := TestQuery("{invalid=query}", "test-datasource", errorServer.URL, "test-api-key", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP error")
	})

	// Error case: empty response
	t.Run("empty query response", func(t *testing.T) {
		emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/datasources/name/test-datasource" {
				// Return valid datasource
				mockDS := GrafanaDatasource{
					UID:  "xyz789",
					Type: "loki",
				}
				responseJSON, _ := json.Marshal(mockDS)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(responseJSON)
				return
			}

			if r.URL.Path == "/api/ds/query" {
				w.WriteHeader(http.StatusOK)
				// Empty response
				return
			}
		}))
		defer emptyServer.Close()

		_, err := TestQuery("{job=\"test\"} |= \"warn\"", "test-datasource", emptyServer.URL, "test-api-key", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty response from datasource")
	})
}
