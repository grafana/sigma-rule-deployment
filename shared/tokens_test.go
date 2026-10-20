package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGrafanaTokens(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    map[string]string // instance URL to the token it resolves to
		wantErr string
	}{
		{name: "empty", raw: "", want: map[string]string{"https://any.example": ""}},
		{name: "plain token is used for every instance", raw: " glsa_plain\n", want: map[string]string{
			"https://a.example":      "glsa_plain",
			"http://other.example/x": "glsa_plain",
		}},
		{name: "token per instance", raw: `{"https://a.example": "glsa_a", "https://b.example/": " glsa_b "}`, want: map[string]string{
			"https://a.example":  "glsa_a",
			"https://a.example/": "glsa_a",
			"https://b.example":  "glsa_b",
		}},
		{name: "equivalent URLs resolve to the same token", raw: `{"HTTPS://Grafana.Example:443/": "glsa_a", "http://local.example:3000": "glsa_b"}`, want: map[string]string{
			"https://grafana.example":      "glsa_a",
			"https://grafana.example/":     "glsa_a",
			"https://GRAFANA.example:443/": "glsa_a",
			"http://local.example:3000/":   "glsa_b",
		}},
		{name: "instances with a path are different", raw: `{"https://a.example/one": "glsa_1", "https://a.example/two": "glsa_2"}`, want: map[string]string{
			"https://a.example/one/": "glsa_1",
			"https://a.example/two":  "glsa_2",
		}},
		{name: "invalid JSON", raw: `{"https://a.example": "glsa_secret"`, wantErr: "can't be parsed"},
		{name: "a token that is not a string", raw: `{"https://a.example": 5}`, wantErr: "can't be parsed"},
		{name: "empty object", raw: `{}`, wantErr: "empty JSON object"},
		{name: "empty token", raw: `{"https://a.example": " "}`, wantErr: "token for Grafana instance https://a.example is empty"},
		{name: "key that is not a URL", raw: `{"prod": "glsa_a"}`, wantErr: `"prod" is not a Grafana instance URL`},
		{name: "key with an unsupported scheme", raw: `{"ftp://a.example": "glsa_a"}`, wantErr: "is not a Grafana instance URL"},
		{name: "same key twice", raw: `{"https://a.example": "glsa_a", "https://a.example": "glsa_b"}`, wantErr: "more than once"},
		{name: "more after the object", raw: `{"https://a.example": "glsa_a"} {"https://b.example": "glsa_b"}`, wantErr: "more after the object"},
		{name: "same instance twice", raw: `{"https://a.example": "glsa_a", "https://A.example/": "glsa_b"}`, wantErr: "more than once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := ParseGrafanaTokens(tt.raw)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.NotContains(t, err.Error(), "glsa_", "an error must not include a token")
				return
			}
			assert.NoError(t, err)
			for instance, want := range tt.want {
				got, err := tokens.For(instance)
				assert.NoError(t, err, instance)
				assert.Equal(t, want, got, instance)
			}
		})
	}
}

func TestGrafanaTokensForUnknownInstance(t *testing.T) {
	tokens, err := ParseGrafanaTokens(`{"https://a.example": "glsa_a"}`)
	assert.NoError(t, err)

	for _, instance := range []string{"https://b.example", "http://a.example", "https://a.example:8443", "https://a.example/sub"} {
		token, err := tokens.For(instance)
		assert.ErrorContains(t, err, "no Grafana token for instance", instance)
		assert.Empty(t, token, "an instance without a token must not get another one: %s", instance)
	}
	_, err = tokens.For("not a url")
	assert.Error(t, err)
}

func TestGrafanaTokensIsSet(t *testing.T) {
	for raw, want := range map[string]bool{"": false, "  ": false, "glsa_a": true, `{"https://a.example": "glsa_a"}`: true} {
		tokens, err := ParseGrafanaTokens(raw)
		assert.NoError(t, err)
		assert.Equal(t, want, tokens.IsSet(), raw)
	}
	assert.True(t, NewGrafanaToken("glsa_a").IsSet())
	assert.False(t, NewGrafanaToken("").IsSet())
	assert.False(t, GrafanaTokens{}.IsSet())
}

func TestGrafanaTokensMatchTheClientBaseURL(t *testing.T) {
	// A token must be found for an instance by the same URL the client for it is keyed by
	tokens, err := ParseGrafanaTokens(`{"HTTPS://Prod.example:443/": "glsa_prod"}`)
	assert.NoError(t, err)
	for _, instance := range []string{"https://prod.example", "https://PROD.example/", "https://prod.example:443"} {
		client := NewGrafanaClient(instance, "ignored", "test", 0)
		token, err := tokens.For(client.BaseURL())
		assert.NoError(t, err, instance)
		assert.Equal(t, "glsa_prod", token, instance)
	}
}
