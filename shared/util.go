//nolint:revive
package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
