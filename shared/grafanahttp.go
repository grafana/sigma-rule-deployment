//nolint:revive
package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// defaultPorts are the ports a URL leaves out, by scheme. Its keys are the schemes of a Grafana URL.
var defaultPorts = map[string]string{"http": "80", "https": "443"}

// isHTTPURL reports whether a parsed URL has an http or https scheme and a host.
func isHTTPURL(u *url.URL) bool {
	_, ok := defaultPorts[u.Scheme]
	return ok && u.Hostname() != ""
}

// NormalizeBaseURL returns the canonical form of a Grafana instance URL, as clients store it, so
// equivalent URLs compare equal: the scheme and host are lower case, a default port is dropped
// and the URL ends with a single slash. A URL that isn't a plain http(s) URL only gets the slash.
func NormalizeBaseURL(baseURL string) string {
	if u, err := url.Parse(strings.TrimSpace(baseURL)); err == nil && isHTTPURL(u) &&
		u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		host := strings.ToLower(u.Hostname())
		if strings.Contains(host, ":") {
			host = "[" + host + "]" // IPv6
		}
		if port := u.Port(); port != "" && port != defaultPorts[u.Scheme] {
			host += ":" + port
		}
		return u.Scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/") + "/"
	}
	if baseURL != "" && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	return baseURL
}

// BaseURL returns the normalised base URL of the Grafana instance this client talks to.
func (c *GrafanaClient) BaseURL() string {
	return c.baseURL
}

// NewGrafanaClient creates a new Grafana HTTP client
func NewGrafanaClient(baseURL, apiKey, userAgent string, timeout time.Duration) *GrafanaClient {
	return &GrafanaClient{
		baseURL:   NormalizeBaseURL(baseURL),
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

	baseParsed, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid client base URL: %w", err)
	}
	if req.URL.Hostname() != baseParsed.Hostname() {
		return nil, fmt.Errorf("request URL host %q does not match client base URL", req.URL.Host)
	}

	resp, err := c.client.Do(req) //nolint:gosec // G704: req.URL.Hostname() validated to match client base URL above
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
