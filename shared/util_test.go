package shared

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMatchConfigName(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		names     []string
		wantName  string
		wantFound bool
	}{
		{"no names", "aws_rule.json", nil, "", false},
		{"no match", "gcp_rule.json", []string{"aws", "okta"}, "", false},
		{"single match", "aws_rule.json", []string{"aws", "okta"}, "aws", true},
		{"underscore separator required", "awsome_rule.json", []string{"aws"}, "", false},
		{"longest wins, shorter first", "aws_cloudtrail_rule.json", []string{"aws", "aws_cloudtrail"}, "aws_cloudtrail", true},
		{"longest wins, longer first", "aws_cloudtrail_rule.json", []string{"aws_cloudtrail", "aws"}, "aws_cloudtrail", true},
		{"shorter still matches its own files", "aws_rule.json", []string{"aws_cloudtrail", "aws"}, "aws", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := MatchConfigName(tt.filename, tt.names)
			assert.Equal(t, tt.wantName, got)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

func TestGetConfigBool(t *testing.T) {
	tests := []struct {
		name       string
		config     *bool
		defaultVal *bool
		want       bool
	}{
		{"both unset", nil, nil, false},
		{"default only, true", nil, new(true), true},
		{"default only, false", nil, new(false), false},
		{"config true over unset default", new(true), nil, true},
		{"config true over false default", new(true), new(false), true},
		{"config false over true default", new(false), new(true), false},
		{"config true over true default", new(true), new(true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetConfigBool(tt.config, tt.defaultVal))
		})
	}
}

func TestParseDuration(t *testing.T) {
	const day = 24 * time.Hour
	valid := map[string]time.Duration{
		"30s":    30 * time.Second,
		"5m":     5 * time.Minute,
		"1h30m":  90 * time.Minute,
		"1.5h":   90 * time.Minute,
		"500ms":  500 * time.Millisecond,
		"1d":     day,
		"7d":     7 * day,
		"0d":     0,
		"1d12h":  day + 12*time.Hour,
		"2d1h5s": 2*day + time.Hour + 5*time.Second,
	}
	for in, want := range valid {
		t.Run("valid "+in, func(t *testing.T) {
			got, err := ParseDuration(in)
			assert.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	invalid := []string{"", "d", "1", "x", "1x", "1h1d", "-1d", "1d-1h", "1d+1h", "1.5d", "1dd", "1d1d", "99999999999999999999d", "9999999d"}
	for _, in := range invalid {
		t.Run("invalid "+in, func(t *testing.T) {
			_, err := ParseDuration(in)
			assert.Error(t, err)
		})
	}

	assert.Equal(t, day, ParseDurationOrDefault("1d", time.Second), "ParseDurationOrDefault supports days")
	assert.Equal(t, time.Second, ParseDurationOrDefault("bogus", time.Second))
}
