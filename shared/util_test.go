package shared

import (
	"testing"

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
