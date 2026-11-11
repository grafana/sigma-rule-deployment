package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportDeploymentErrorWritesSingleLineOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	reportDeploymentError("Error deploying", errors.New("first line\nsecond line"))

	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "deployment_error=Error deploying: first line second line\n"
	if string(contents) != expected {
		t.Fatalf("unexpected output:\nwant: %q\n got: %q", expected, contents)
	}
}

// TestIntegrateExitCodeFollowsPerConfigurationContinueOnQueryTestingErrors runs the integrate
// command in a subprocess, so the os.Exit decision is tested, against a Grafana instance that
// refuses connections so every query fails
func TestIntegrateExitCodeFollowsPerConfigurationContinueOnQueryTestingErrors(t *testing.T) {
	if os.Getenv("SIGMA_DEPLOYER_RUN_MAIN") == "1" {
		os.Args = []string{"sigma-deployer", "integrate"}
		main()
		return
	}

	tests := []struct {
		name          string
		defaultLine   string
		configLine    string
		token         string
		wantExitError bool
	}{
		{name: "configuration false over a true default fails", defaultLine: "continue_on_query_testing_errors: true", configLine: "continue_on_query_testing_errors: false", token: "my-test-token", wantExitError: true},
		{name: "configuration false over an unset default fails", configLine: "continue_on_query_testing_errors: false", token: "my-test-token", wantExitError: true},
		{name: "configuration true over a false default continues", defaultLine: "continue_on_query_testing_errors: false", configLine: "continue_on_query_testing_errors: true", token: "my-test-token"},
		{name: "unset everywhere continues", token: "my-test-token"},
		{name: "an invalid token fails even when continuing", token: "{not json", wantExitError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			config := "version: 2\nfolders:\n  conversion_path: conversions\n  deployment_path: deployments\n" +
				"defaults:\n  integration:\n    folder_id: folder\n    org_id: 1\n    data_source: ds\n    data_source_type: loki\n    test_queries: true\n    " + tt.defaultLine + "\n" +
				"  deployment:\n    grafana_instance: http://127.0.0.1:1\n" +
				"configurations:\n  - name: conv\n    integration:\n      " + tt.configLine + "\n"
			writeFile(t, filepath.Join(dir, "config.yml"), config)
			writeFile(t, filepath.Join(dir, "conversions", "conv_rule.json"), `{"conversion_name": "conv", "queries": ["{job=\"test\"}"]}`)

			cmd := exec.Command(os.Args[0], "-test.run=^TestIntegrateExitCodeFollowsPerConfigurationContinueOnQueryTestingErrors$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"SIGMA_DEPLOYER_RUN_MAIN=1",
				"INTEGRATOR_CONFIG_PATH=config.yml",
				"INTEGRATOR_GRAFANA_SA_TOKEN="+tt.token,
				"TEST_FILES=conversions/conv_rule.json",
				"CHANGED_FILES=",
				"DELETED_FILES=",
				"MANUAL_FILES=",
				"GITHUB_OUTPUT="+filepath.Join(dir, "github-output"),
			)
			out, err := cmd.CombinedOutput()
			var exitErr *exec.ExitError
			if tt.wantExitError && (!errors.As(err, &exitErr) || !strings.Contains(string(out), "Error running query tests")) {
				t.Fatalf("want query testing to fail the integration, got %v\n%s", err, out)
			}
			if !tt.wantExitError && err != nil {
				t.Fatalf("want a zero exit, got %v\n%s", err, out)
			}
		})
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
