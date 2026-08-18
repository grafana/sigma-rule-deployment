package main

import (
	"errors"
	"os"
	"path/filepath"
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
