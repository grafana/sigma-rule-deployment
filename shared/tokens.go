package shared

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// GrafanaTokens resolves the service account token to use for each Grafana instance.
//
// It is parsed from a single secret value that is either a plain token, used for every
// instance, or a JSON object mapping each instance URL to its own token:
//
//	{"https://prod.grafana.net": "glsa_prod", "https://staging.grafana.net": "glsa_staging"}
//
// With a JSON object an instance is never given another instance's token: an instance that is
// not in the object has no token.
type GrafanaTokens struct {
	single     string
	byInstance map[string]string
}

// NewGrafanaToken returns tokens that use the same token for every instance.
func NewGrafanaToken(token string) GrafanaTokens {
	return GrafanaTokens{single: token}
}

// ParseGrafanaTokens parses a plain token or a JSON object of instance URLs to tokens.
// An empty value gives tokens that resolve every instance to an empty token.
// Errors never include the value, as it is a secret.
func ParseGrafanaTokens(raw string) (GrafanaTokens, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return GrafanaTokens{single: raw}, nil
	}

	var tokens map[string]string
	if err := json.Unmarshal([]byte(raw), &tokens); err != nil {
		return GrafanaTokens{}, fmt.Errorf("the Grafana tokens look like a JSON object but can't be parsed as one mapping instance URLs to token strings: %w", err)
	}
	if len(tokens) == 0 {
		return GrafanaTokens{}, fmt.Errorf("the Grafana tokens are an empty JSON object")
	}
	byInstance := make(map[string]string, len(tokens))
	for instance, token := range tokens {
		key, err := normalizeInstanceKey(instance)
		if err != nil {
			return GrafanaTokens{}, err
		}
		if strings.TrimSpace(token) == "" {
			return GrafanaTokens{}, fmt.Errorf("the token for Grafana instance %s is empty", key)
		}
		if _, ok := byInstance[key]; ok {
			return GrafanaTokens{}, fmt.Errorf("the Grafana tokens list instance %s more than once", key)
		}
		byInstance[key] = strings.TrimSpace(token)
	}
	return GrafanaTokens{byInstance: byInstance}, nil
}

// IsSet reports whether there is any token.
func (t GrafanaTokens) IsSet() bool {
	return t.single != "" || len(t.byInstance) > 0
}

// For returns the token for a Grafana instance. It fails when the tokens are per instance and
// there is none for this one. Without any tokens it returns an empty token.
func (t GrafanaTokens) For(instance string) (string, error) {
	if t.byInstance == nil {
		return t.single, nil
	}
	key, err := normalizeInstanceKey(instance)
	if err != nil {
		return "", err
	}
	token, ok := t.byInstance[key]
	if !ok {
		return "", fmt.Errorf("there is no Grafana token for instance %s: add it to the Grafana tokens", key)
	}
	return token, nil
}

// normalizeInstanceKey makes equivalent URLs for an instance compare equal: the scheme and host
// are lower case, a default port and any trailing slashes are dropped.
func normalizeInstanceKey(instance string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(instance))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("%q is not a Grafana instance URL: it must start with http:// or https://", instance)
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]" // IPv6
	}
	if port := u.Port(); port != "" && port != map[string]string{"http": "80", "https": "443"}[u.Scheme] {
		host += ":" + port
	}
	return strings.ToLower(u.Scheme) + "://" + host + strings.TrimRight(u.EscapedPath(), "/"), nil
}
