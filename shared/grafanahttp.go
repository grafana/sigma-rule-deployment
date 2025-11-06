package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GrafanaClient provides a reusable HTTP client for Grafana API requests
type GrafanaClient struct {
	baseURL   string
	apiKey    string
	timeout   time.Duration
	userAgent string
	client    *http.Client
}

// NewGrafanaClient creates a new Grafana HTTP client
func NewGrafanaClient(baseURL, apiKey, userAgent string, timeout time.Duration) *GrafanaClient {
	// Ensure baseURL ends with a slash
	if baseURL != "" && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}

	return &GrafanaClient{
		baseURL:   baseURL,
		apiKey:    apiKey,
		timeout:   timeout,
		userAgent: userAgent,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// setHeaders sets common headers for Grafana API requests
func (c *GrafanaClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
}

// newRequest creates a new HTTP request with context and common headers
func (c *GrafanaClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setHeaders(req)
	return req, nil
}

// Do executes an HTTP request and returns the response
func (c *GrafanaClient) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// Get performs a GET request
func (c *GrafanaClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.Do(ctx, http.MethodGet, path, nil)
}

// Post performs a POST request with JSON body
func (c *GrafanaClient) Post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(bodyBytes)
	}
	return c.Do(ctx, http.MethodPost, path, bodyReader)
}

// PostRaw performs a POST request with raw body (string or []byte)
func (c *GrafanaClient) PostRaw(ctx context.Context, path string, body []byte) (*http.Response, error) {
	return c.Do(ctx, http.MethodPost, path, bytes.NewBuffer(body))
}

// Put performs a PUT request with JSON body
func (c *GrafanaClient) Put(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(bodyBytes)
	}
	return c.Do(ctx, http.MethodPut, path, bodyReader)
}

// PutRaw performs a PUT request with raw body (string or []byte)
func (c *GrafanaClient) PutRaw(ctx context.Context, path string, body []byte) (*http.Response, error) {
	return c.Do(ctx, http.MethodPut, path, bytes.NewBuffer(body))
}

// Delete performs a DELETE request
func (c *GrafanaClient) Delete(ctx context.Context, path string) (*http.Response, error) {
	return c.Do(ctx, http.MethodDelete, path, nil)
}

// ReadJSONResponse reads and unmarshals a JSON response from the HTTP response
func ReadJSONResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		return fmt.Errorf("empty response body")
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

// ReadResponseBody reads the response body as bytes
func ReadResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// CheckStatusCode checks if the status code is in the expected range and returns an error if not
func CheckStatusCode(resp *http.Response, expectedCodes ...int) error {
	for _, code := range expectedCodes {
		if resp.StatusCode == code {
			return nil
		}
	}

	body, _ := ReadResponseBody(resp)
	return fmt.Errorf("unexpected status code %d: %s, Response: %s", resp.StatusCode, resp.Status, string(body))
}
