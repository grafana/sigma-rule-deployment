package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DatasourceQuery is an interface for executing Grafana datasource queries
type DatasourceQuery interface {
	GetDatasource(dsName, baseURL, apiKey string, timeout time.Duration) (*GrafanaDatasource, error)
	ExecuteQuery(query, dsName, baseURL, apiKey, from, to string, timeout time.Duration) ([]byte, error)
}

// HTTPDatasourceQuery is the default implementation of DatasourceQuery
type HTTPDatasourceQuery struct{}

// DefaultDatasourceQuery is the default implementation used throughout the application
var DefaultDatasourceQuery DatasourceQuery = &HTTPDatasourceQuery{}

type GrafanaDatasource struct {
	ID                int             `json:"id,omitempty"`
	UID               string          `json:"uid"`
	OrgID             int             `json:"orgId,omitempty"`
	Name              string          `json:"name,omitempty"`
	Type              string          `json:"type"`
	TypeLogoURL       string          `json:"typeLogoUrl,omitempty"`
	Access            string          `json:"access,omitempty"`
	URL               string          `json:"url,omitempty"`
	Password          string          `json:"password,omitempty"`
	User              string          `json:"user,omitempty"`
	Database          string          `json:"database,omitempty"`
	BasicAuth         bool            `json:"basicAuth,omitempty"`
	BasicAuthUser     string          `json:"basicAuthUser,omitempty"`
	BasicAuthPassword string          `json:"basicAuthPassword,omitempty"`
	WithCredentials   bool            `json:"withCredentials,omitempty"`
	IsDefault         bool            `json:"isDefault,omitempty"`
	JSONData          json.RawMessage `json:"jsonData,omitempty"`
	SecureJSONFields  map[string]bool `json:"secureJsonFields,omitempty"`
	Version           int             `json:"version,omitempty"`
	ReadOnly          bool            `json:"readOnly,omitempty"`
}

type Query struct {
	RefID         string            `json:"refId"`
	Expr          string            `json:"expr"`
	QueryType     string            `json:"queryType"`
	Datasource    GrafanaDatasource `json:"datasource"`
	EditorMode    string            `json:"editorMode,omitempty"`
	MaxLines      int               `json:"maxLines"`
	Format        string            `json:"format"`
	IntervalMs    int               `json:"intervalMs"`
	MaxDataPoints int               `json:"maxDataPoints"`
}

type Body struct {
	Queries []Query `json:"queries"`
	From    string  `json:"from"`
	To      string  `json:"to"`
}

// TestQuery uses the default executor to query a datasource
func TestQuery(
	query, dsName, baseURL, apiKey, from, to string,
	timeout time.Duration,
) ([]byte, error) {
	return DefaultDatasourceQuery.ExecuteQuery(query, dsName, baseURL, apiKey, from, to, timeout)
}

// GetDatasourceByName uses the default executor to get datasource information
func GetDatasourceByName(dsName, baseURL, apiKey string, timeout time.Duration) (*GrafanaDatasource, error) {
	return DefaultDatasourceQuery.GetDatasource(dsName, baseURL, apiKey, timeout)
}

// ExecuteQuery implementation for HTTPDatasourceQuery
func (h *HTTPDatasourceQuery) ExecuteQuery(
	query, dsName, baseURL, apiKey, from, to string,
	timeout time.Duration,
) ([]byte, error) {
	datasource, err := h.GetDatasource(dsName, baseURL, apiKey, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %v", err)
	}

	body := Body{
		Queries: []Query{
			{
				RefID:     "A",
				Expr:      query,
				QueryType: "range",
				Datasource: GrafanaDatasource{
					Type: datasource.Type,
					UID:  datasource.UID,
				},
				MaxLines:      100,
				Format:        "time_series",
				IntervalMs:    2000,
				MaxDataPoints: 100,
			},
		},
		From: from,
		To:   to,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	dsQueryURL, err := url.JoinPath(baseURL, "api/ds/query")
	if err != nil {
		return nil, fmt.Errorf("failed to construct API URL: %v", err)
	}

	req, err := http.NewRequest("POST", dsQueryURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("HTTP error %d when querying datasource: %s, Response: %s",
			resp.StatusCode, resp.Status, string(responseData))
	}

	if len(responseData) == 0 {
		return nil, fmt.Errorf("empty response from datasource")
	}

	var jsonResponse any
	if err := json.Unmarshal(responseData, &jsonResponse); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %v", err)
	}

	return responseData, nil
}

// GetDatasource implementation for HTTPDatasourceQuery
func (h *HTTPDatasourceQuery) GetDatasource(dsName, baseURL, apiKey string, timeout time.Duration) (*GrafanaDatasource, error) {
	dsURL, err := url.JoinPath(baseURL, "api/datasources/name", dsName)
	if err != nil {
		return nil, fmt.Errorf("failed to construct API URL: %v", err)
	}

	req, err := http.NewRequest("GET", dsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("HTTP error getting datasource: %s, Response: %s", resp.Status, string(responseData))
	}

	if len(responseData) == 0 {
		return nil, fmt.Errorf("empty response from datasource")
	}

	var datasource GrafanaDatasource
	err = json.Unmarshal(responseData, &datasource)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	return &datasource, nil
}
