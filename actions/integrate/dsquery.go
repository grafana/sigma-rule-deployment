package integrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Datasource struct {
	Type string `json:"type,omitempty"`
	UID  string `json:"uid"`
}

type Query struct {
	RefID         string     `json:"refId"`
	Expr          string     `json:"expr"`
	QueryType     string     `json:"queryType"`
	Datasource    Datasource `json:"datasource"`
	EditorMode    string     `json:"editorMode,omitempty"`
	MaxLines      int        `json:"maxLines"`
	Format        string     `json:"format"`
	IntervalMs    int        `json:"intervalMs"`
	MaxDataPoints int        `json:"maxDataPoints"`
}

type Body struct {
	Queries []Query `json:"queries"`
	From    string  `json:"from"`
	To      string  `json:"to"`
}

// TODO: make it configurable
func TestQuery(query_str, dsUID, url, apiKey string) ([]byte, error) {
	body := Body{
		Queries: []Query{
			{
				RefID:     "A",
				Expr:      query_str,
				QueryType: "range",
				Datasource: Datasource{
					Type: "loki",
					UID:  dsUID,
				},
				MaxLines:      1000,
				Format:        "time_series",
				IntervalMs:    2000,
				MaxDataPoints: 100,
			},
		},
		From: "now-15m",
		To:   "now",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error: %s, Response: %s", resp.Status, string(responseData))
	}

	return responseData, nil
}
