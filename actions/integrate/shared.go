package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FoldersConfig struct {
	ConversionPath string `yaml:"conversion_path"`
	DeploymentPath string `yaml:"deployment_path"`
}

type ConversionConfig struct {
	Name            string   `yaml:"name"`
	Target          string   `yaml:"target"`
	Format          string   `yaml:"format"`
	SkipUnsupported string   `yaml:"skip_unsupported"`
	FilePattern     string   `yaml:"file_pattern"`
	DataSource      string   `yaml:"data_source"`
	Pipeline        []string `yaml:"pipelines"`
	RuleGroup       string   `yaml:"rule_group"`
	TimeWindow      string   `yaml:"time_window"`
}

type IntegrationConfig struct {
	FolderID    string `yaml:"folder_id"`
	OrgID       int64  `yaml:"org_id"`
	TestQueries bool   `yaml:"test_queries"`
}

type DeploymentConfig struct {
	GrafanaInstance string `yaml:"grafana_instance"`
	Timeout         string `yaml:"timeout"`
}

type Configuration struct {
	Folders            FoldersConfig      `yaml:"folders"`
	ConversionDefaults ConversionConfig   `yaml:"conversion_defaults"`
	Conversions        []ConversionConfig `yaml:"conversions"`
	IntegratorConfig   IntegrationConfig  `yaml:"integration"`
	DeployerConfig     DeploymentConfig   `yaml:"deployment"`
}

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

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
