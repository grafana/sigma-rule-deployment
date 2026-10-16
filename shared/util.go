//nolint:revive
package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Loki          = "loki"
	Elasticsearch = "elasticsearch"
)

func GetInputOrDefault(name string, value string) string {
	envName := "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, " ", "_"))

	env := os.Getenv(envName)
	if env == "" {
		return value
	}

	return env
}

func validateOutputFilePath(path string) error {
	cleaned := filepath.Clean(path)
	if cleaned != path || strings.HasPrefix(cleaned, "..") {
		return errors.New("output file path is invalid")
	}
	return nil
}

func SetOutput(output, value string) error {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return errors.New("only output with a github output file supported. See https://github.blog/changelog/2022-10-11-github-actions-deprecating-save-state-and-set-output-commands/ for further details")
	}
	if err := validateOutputFilePath(outputFile); err != nil {
		return fmt.Errorf("GITHUB_OUTPUT path is invalid: %w", err)
	}

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G703: outputFile validated to reject path traversal above
	if err != nil {
		return fmt.Errorf("unable to open output file, due %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%v=%v\n", output, value)
	if err != nil {
		return fmt.Errorf("unable to write to output file, due %w", err)
	}

	return nil
}

// SetFileOutput writes content to path and records path (not content) as the named
// GitHub Actions output, avoiding E2BIG failures when a later step reads it into an env var.
func SetFileOutput(output, path, content string) error {
	if err := validateOutputFilePath(path); err != nil {
		return fmt.Errorf("%s output file path is invalid: %w", output, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // G703: path validated to reject path traversal above
		return fmt.Errorf("unable to write %s output file: %w", output, err)
	}
	return SetOutput(output, path)
}

func ReadLocalFile(path string) (string, error) {
	// Ensure path is local to avoid path traversal
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("invalid file path: %s", path)
	}

	contents, err := os.ReadFile(path)

	return string(contents), err
}

func EscapeQueryJSON(query string) (string, error) {
	escapedQuotedQuery, err := json.Marshal(query)
	if err != nil {
		return "", fmt.Errorf("could not escape provided query: %s", query)
	}
	return string(escapedQuotedQuery[1 : len(escapedQuotedQuery)-1]), nil // strip the leading and trailing quotation marks
}

// GetConfigValue returns the first non-empty value from config, defaultConf, or def (in that order)
func GetConfigValue(config, defaultConf, def string) string {
	if config != "" {
		return config
	}
	if defaultConf != "" {
		return defaultConf
	}
	return def
}

// GetConfigBool returns config if it is set, otherwise defaultConf if it is set, otherwise false.
// A set false in config overrides a true defaultConf.
func GetConfigBool(config, defaultConf *bool) bool {
	if config != nil {
		return *config
	}
	if defaultConf != nil {
		return *defaultConf
	}
	return false
}

// GetConfigValueInt64 returns config if non-zero, otherwise defaultVal.
func GetConfigValueInt64(config, defaultVal int64) int64 {
	if config != 0 {
		return config
	}
	return defaultVal
}

// ParseDuration parses a duration as time.ParseDuration does, and additionally accepts a leading whole
// number of days ("1d", "2d12h"), which time.ParseDuration doesn't support. A day is 24 hours.
// Days must come first and be unsigned, so "1h1d" and "-1d" are rejected.
func ParseDuration(s string) (time.Duration, error) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 || end >= len(s) || s[end] != 'd' {
		return time.ParseDuration(s)
	}
	days, err := strconv.ParseInt(s[:end], 10, 64)
	if err != nil || days > int64(math.MaxInt64/(24*time.Hour)) {
		return 0, fmt.Errorf("invalid duration %q: too many days", s)
	}
	total := time.Duration(days) * 24 * time.Hour
	rest := s[end+1:]
	if rest == "" {
		return total, nil
	}
	remainder, err := time.ParseDuration(rest)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	// time.ParseDuration accepts a sign, which would be meaningless after the days
	if rest[0] == '-' || rest[0] == '+' {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	if total > math.MaxInt64-remainder {
		return 0, fmt.Errorf("invalid duration %q: overflows", s)
	}
	return total + remainder, nil
}

// ParseDurationOrDefault parses s as a duration (see ParseDuration), returning defaultVal if s is empty or invalid.
func ParseDurationOrDefault(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	if d, err := ParseDuration(s); err == nil {
		return d
	}
	return defaultVal
}

// MatchConfigName returns the configuration name that owns filename, which must start with
// "{name}_". When several names match (e.g. "aws" and "aws_cloudtrail" for
// "aws_cloudtrail_rule.json") the longest wins, so the result doesn't depend on ordering.
// Pass every configuration name, not just those with overrides, so a longer name without
// overrides isn't mistaken for a shorter one that has them.
func MatchConfigName(filename string, names []string) (string, bool) {
	best, found := "", false
	for _, name := range names {
		if strings.HasPrefix(filename, name+"_") && (!found || len(name) > len(best)) {
			best, found = name, true
		}
	}
	return best, found
}
