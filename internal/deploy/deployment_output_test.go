package deploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/stretchr/testify/assert"
)

func TestWriteOutputIncludesDeploymentDetails(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	deployer := NewDeployer()
	created := []AlertDeployment{
		{UID: "uid-z", Title: "Zulu rule"},
		{UID: "uid-a", Title: "Alpha rule"},
	}
	deleted := []AlertDeployment{{UID: "uid-d", Title: "Deleted rule"}}

	err := deployer.WriteOutput(created, nil, deleted)
	assert.NoError(t, err)

	outputs := readActionOutputs(t, outputPath)
	assert.Equal(t, "uid-z uid-a", outputs["alerts_created"])
	assert.Equal(t, "", outputs["alerts_updated"])
	assert.Equal(t, "uid-d", outputs["alerts_deleted"])

	var createdDetails []AlertDeployment
	err = json.Unmarshal([]byte(outputs["alerts_created_details"]), &createdDetails)
	assert.NoError(t, err)
	assert.Equal(t, created, createdDetails)
	assert.Equal(t, "[]", outputs["alerts_updated_details"])

	var deletedDetails []AlertDeployment
	err = json.Unmarshal([]byte(outputs["alerts_deleted_details"]), &deletedDetails)
	assert.NoError(t, err)
	assert.Equal(t, deleted, deletedDetails)
}

func TestGetAlertTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)

		switch r.URL.Path {
		case alertingAPIPrefix + "/known":
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"uid":"known","title":"Known rule"}`))
			assert.NoError(t, err)
		case alertingAPIPrefix + "/untitled":
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"uid":"untitled","title":"  "}`))
			assert.NoError(t, err)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	deployer := Deployer{
		client: shared.NewGrafanaClient(
			server.URL+"/",
			"my-test-token",
			"sigma-rule-deployment/deployer",
			defaultRequestTimeout,
		),
	}

	assert.Equal(t, "Known rule", deployer.getAlertTitle(context.Background(), "known"))
	assert.Equal(t, "missing", deployer.getAlertTitle(context.Background(), "missing"))
	assert.Equal(t, "untitled", deployer.getAlertTitle(context.Background(), "untitled"))
}

func readActionOutputs(t *testing.T, outputPath string) map[string]string {
	t.Helper()

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	outputs := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		name, value, found := strings.Cut(line, "=")
		if found {
			outputs[name] = value
		}
	}
	return outputs
}
