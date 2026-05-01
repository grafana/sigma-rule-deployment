//nolint:revive
package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func SetOutput(output, value string) error {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return errors.New("only output with a github output file supported. See https://github.blog/changelog/2022-10-11-github-actions-deprecating-save-state-and-set-output-commands/ for further details")
	}
	cleaned := filepath.Clean(outputFile)
	if cleaned != outputFile || strings.HasPrefix(cleaned, "..") {
		return errors.New("GITHUB_OUTPUT path is invalid")
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

// GetConfigValueInt64 returns config if non-zero, otherwise defaultVal.
func GetConfigValueInt64(config, defaultVal int64) int64 {
	if config != 0 {
		return config
	}
	return defaultVal
}

// ParseDurationOrDefault parses s as a duration, returning defaultVal if s is empty or invalid.
func ParseDurationOrDefault(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return defaultVal
}
