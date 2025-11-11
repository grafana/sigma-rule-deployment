package integrate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/grafana/sigma-rule-deployment/shared"
)

// DatasourceQuery is an interface for executing Grafana datasource queries
type DatasourceQuery interface {
	GetDatasource(dsName, baseURL, apiKey string, timeout time.Duration) (*GrafanaDatasource, error)
	ExecuteQuery(query, dsName, baseURL, apiKey, refID, from, to, customModel string, timeout time.Duration) ([]byte, error)
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

// BucketAgg represents a bucket aggregation for Elasticsearch queries
type BucketAgg struct {
	Type     string         `json:"type"`
	ID       string         `json:"id"`
	Settings map[string]any `json:"settings,omitempty"`
	Field    string         `json:"field,omitempty"`
}

// Metric represents a metric for Elasticsearch queries
type Metric struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Query struct {
	RefID         string            `json:"refId"`
	Expr          string            `json:"expr,omitempty"`  // For Loki
	Query         string            `json:"query,omitempty"` // For Elasticsearch
	QueryType     string            `json:"queryType,omitempty"`
	Datasource    GrafanaDatasource `json:"datasource"`
	EditorMode    string            `json:"editorMode,omitempty"`
	MaxLines      int               `json:"maxLines,omitempty"`
	Format        string            `json:"format,omitempty"`
	IntervalMs    int               `json:"intervalMs,omitempty"`
	MaxDataPoints int               `json:"maxDataPoints,omitempty"`

	// Elasticsearch-specific fields
	Alias        string      `json:"alias,omitempty"`
	Metrics      []Metric    `json:"metrics,omitempty"`
	BucketAggs   []BucketAgg `json:"bucketAggs,omitempty"`
	TimeField    string      `json:"timeField,omitempty"`
	DatasourceID int         `json:"datasourceId,omitempty"`
}

type Body struct {
	Queries []Query `json:"queries"`
	From    string  `json:"from"`
	To      string  `json:"to"`
}

// TestQuery uses the default executor to query a datasource
func TestQuery(
	query, dsName, baseURL, apiKey, refID, from, to, customModel string,
	timeout time.Duration,
) ([]byte, error) {
	return DefaultDatasourceQuery.ExecuteQuery(
		query, dsName, baseURL, apiKey, refID, from, to, customModel, timeout,
	)
}

// GetDatasourceByName uses the default executor to get datasource information
func GetDatasourceByName(
	dsName, baseURL, apiKey string, timeout time.Duration,
) (*GrafanaDatasource, error) {
	return DefaultDatasourceQuery.GetDatasource(dsName, baseURL, apiKey, timeout)
}

// ExecuteQuery implementation for HTTPDatasourceQuery
func (h *HTTPDatasourceQuery) ExecuteQuery(
	query, dsName, baseURL, apiKey, refID, from, to, customModel string,
	timeout time.Duration,
) ([]byte, error) {
	datasource, err := h.GetDatasource(dsName, baseURL, apiKey, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %v", err)
	}

	var queryObj json.RawMessage

	// Configure query based on custom model or datasource type
	switch {
	case customModel != "":
		// Use custom model to build the query object
		escapedQuery, err := shared.EscapeQueryJSON(query)
		if err != nil {
			return nil, fmt.Errorf("failed to escape query: %v", err)
		}

		// Use sprintf to populate the custom model with refID, datasource UID, and escaped query
		queryObj = json.RawMessage(fmt.Sprintf(customModel, refID, datasource.UID, escapedQuery))
	case datasource.Type == shared.Elasticsearch:
		structQuery := Query{
			RefID: refID,
			Query: query,
			Datasource: GrafanaDatasource{
				Type: datasource.Type,
				UID:  datasource.UID,
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
			DatasourceID:  datasource.ID,
			IntervalMs:    2000,
			MaxDataPoints: 100,
		}

		queryBytes, err := json.Marshal(structQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal query struct: %v", err)
		}
		queryObj = json.RawMessage(queryBytes)
	case datasource.Type == shared.Loki:
		structQuery := Query{
			RefID:     refID,
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
		}

		queryBytes, err := json.Marshal(structQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal query struct: %v", err)
		}
		queryObj = json.RawMessage(queryBytes)
	default:
		// No default configuration for other datasource types
		return nil, fmt.Errorf("unsupported datasource type: %s", datasource.Type)
	}

	// Create the request body with the query object
	body := map[string]any{
		"queries": []json.RawMessage{queryObj},
		"from":    from,
		"to":      to,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// Create Grafana client for the request
	client := shared.NewGrafanaClient(baseURL, apiKey, "sigma-rule-deployment/integrator", timeout)

	// Use url.JoinPath to construct the path relative to baseURL
	queryPath, err := url.JoinPath("api/ds/query")
	if err != nil {
		return nil, fmt.Errorf("failed to construct API path: %v", err)
	}

	resp, err := client.PostRaw(context.Background(), queryPath, jsonBody)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	responseData, err := shared.ReadResponseBody(resp)
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
func (h *HTTPDatasourceQuery) GetDatasource(
	dsName, baseURL, apiKey string, timeout time.Duration,
) (*GrafanaDatasource, error) {
	return h.getDatasourceByUID(dsName, baseURL, apiKey, timeout)
}

// getDatasourceByUID uses the default executor to get datasource information
func (h *HTTPDatasourceQuery) getDatasourceByUID(
	uid, baseURL, apiKey string, timeout time.Duration,
) (*GrafanaDatasource, error) {
	// Create Grafana client for the request
	client := shared.NewGrafanaClient(baseURL, apiKey, "sigma-rule-deployment/integrator", timeout)

	// Construct the path
	path, err := url.JoinPath("api/datasources/uid", uid)
	if err != nil {
		return nil, fmt.Errorf("failed to construct API path: %v", err)
	}

	resp, err := client.Get(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		responseData, _ := shared.ReadResponseBody(resp)
		return nil, fmt.Errorf("HTTP error getting datasource: %s, Response: %s", resp.Status, string(responseData))
	}

	var datasource GrafanaDatasource
	if err := shared.ReadJSONResponse(resp, &datasource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	return &datasource, nil
}
