package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:gosec // G101: a fake password in a URL
func TestNormalizeBaseURL(t *testing.T) {
	tests := map[string]string{
		"":                            "",
		"https://a.example":           "https://a.example/",
		"https://a.example/":          "https://a.example/",
		"https://a.example//":         "https://a.example/",
		" HTTPS://A.Example:443 ":     "https://a.example/",
		"http://a.example:80/":        "http://a.example/",
		"http://a.example:3000":       "http://a.example:3000/",
		"https://a.example:80":        "https://a.example:80/",
		"https://a.example/Grafana/":  "https://a.example/Grafana/",
		"https://a.example/grafana":   "https://a.example/grafana/",
		"http://[::1]:3000":           "http://[::1]:3000/",
		"https://user:pass@a.example": "https://user:pass@a.example/",
		"not a url":                   "not a url/",
		"localhost:3000":              "localhost:3000/",
	}
	for in, want := range tests {
		assert.Equal(t, want, NormalizeBaseURL(in), in)
	}
}
