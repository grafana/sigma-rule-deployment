package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAlertUidFromFileName(t *testing.T) {
	assert.Equal(t, "abcd123", getAlertUidFromFilename("alert_rule_conversion_abcd123.json"))
	assert.Equal(t, "abcd123", getAlertUidFromFilename("alert_rule_conversion_name_abcd123.json"))
	assert.Equal(t, "uAaCwL1wlmA", getAlertUidFromFilename("alert_rule_conversion_uAaCwL1wlmA.json"))
}

func TestParseAlert(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantAlertUid   string
		wantAlertTitle string
		wantError      bool
	}{
		{
			name:           "valid alert",
			content:        `{"uid":"abcd123","title":"Test alert"}`,
			wantAlertUid:   "abcd123",
			wantAlertTitle: "Test alert",
			wantError:      false,
		},
		{
			name:           "invalid alert title",
			content:        `{"uid":"abcd123""`,
			wantAlertUid:   "",
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "invalid alert uid",
			content:        `{"title":"Test alert"}`,
			wantAlertUid:   "",
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "empty alert",
			content:        `{}`,
			wantAlertUid:   "",
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "extra fields",
			content:        `{"uid":"abcd123","title":"Test alert","extra":"field"}`,
			wantAlertUid:   "abcd123",
			wantAlertTitle: "Test alert",
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert, err := parseAlert(tt.content)
			if tt.wantError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantAlertUid, alert.Uid)
				assert.Equal(t, tt.wantAlertTitle, alert.Title)
			}
		})
	}
}

func TestAddAlertToList(t *testing.T) {
	tests := []struct {
		name          string
		file          string
		prefix        string
		wantAlertList []string
	}{
		{
			name:          "simple alert path",
			file:          "deployments/alert_rule_conversion_abcd123.json",
			prefix:        "deployments",
			wantAlertList: []string{"deployments/alert_rule_conversion_abcd123.json"},
		},
		{
			name:          "alert path with extra folder",
			file:          "deployments/extra/alert_rule_conversion_abcd123.json",
			prefix:        "deployments",
			wantAlertList: []string{},
		},
		{
			name:          "root alert path",
			file:          "alert_rule_conversion_abcd123.json",
			prefix:        "deployments",
			wantAlertList: []string{},
		},
		{
			name:          "non-local file",
			file:          "../alert_rule_conversion_abcd123.json",
			prefix:        "deployments",
			wantAlertList: []string{},
		},
		{
			name:          "non-local file 2",
			file:          "../alert_rule_conversion_abcd123.json",
			prefix:        "",
			wantAlertList: []string{},
		},
		{
			name:          "non-local file 3",
			file:          "../deployments/alert_rule_conversion_abcd123.json",
			prefix:        "deployments",
			wantAlertList: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alertList := addToAlertList([]string{}, tt.file, tt.prefix)
			assert.Equal(t, tt.wantAlertList, alertList)
		})
	}
}

func TestUpdateAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") {
			uid := strings.TrimPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/")
			if uid != "abcd123" {
				t.Errorf("Expected to request '/api/v1/provisioning/alert-rules/abcd123', got: %s", r.URL.Path)
			}
		} else {
			t.Errorf("Expected to request '/api/v1/provisioning/alert-rules/abcd123', got: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
		}
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT method, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer my-test-token" {
			t.Errorf("Invalid Authorization header")
		}
		defer r.Body.Close()
		// Read the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
	}

	uid, err := d.updateAlert(`{"uid":"abcd123","title":"Test alert"}`)
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestCreateAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/provisioning/alert-rules" {
			t.Errorf("Expected to request '/api/v1/provisioning/alert-rules', got: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer my-test-token" {
			t.Errorf("Invalid Authorization header")
		}
		defer r.Body.Close()
		// Read the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(body))
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
	}

	uid, err := d.createAlert(`{"uid":"abcd123","title":"Test alert"}`)
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestDeleteAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") {
			uid := strings.TrimPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/")
			if uid != "abcd123" {
				t.Errorf("Expected to request '/api/v1/provisioning/alert-rules/abcd123', got: %s", r.URL.Path)
			}
		} else {
			t.Errorf("Expected to request '/api/v1/provisioning/alert-rules/abcd123', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer my-test-token" {
			t.Errorf("Invalid Authorization header")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
	}

	uid, err := d.deleteAlert("abcd123")
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestLoadConfig(t *testing.T) {
	os.Setenv("CONFIG_PATH", "test_config.yml")
	defer os.Unsetenv("CONFIG_PATH")
	os.Setenv("DEPLOYER_GRAFANA_SA_TOKEN", "my-test-token")
	defer os.Unsetenv("DEPLOYER_GRAFANA_SA_TOKEN")
	os.Setenv("ADDED_FILES", "deployments/alert_rule_conversion_abcd123.json deployments/alert_rule_conversion_def3456789.json")
	defer os.Unsetenv("ADDED_FILES")
	os.Setenv("COPIED_FILES", "deployments/alert_rule_conversion_ghij123.json deployments/alert_rule_conversion_klmn123.json")
	defer os.Unsetenv("COPIED_FILES")
	os.Setenv("DELETED_FILES", "deployments/alert_rule_conversion_opqr123.json deployments/alert_rule_conversion_stuv123.json")
	defer os.Unsetenv("DELETED_FILES")
	os.Setenv("MODIFIED_FILES", "deployments/alert_rule_conversion_wxyz123.json deployments/alert_rule_conversion_123456789.json")
	defer os.Unsetenv("UPDATED_FILES")

	d := NewDeployer()
	d.LoadConfig()
	assert.Equal(t, "my-test-token", d.config.saToken)
	assert.Equal(t, "https://myinstance.grafana.com/", d.config.endpoint)
	assert.Equal(t, "deployments", d.config.alertPath)
	assert.Equal(t, []string{
		"deployments/alert_rule_conversion_abcd123.json",
		"deployments/alert_rule_conversion_def3456789.json",
		"deployments/alert_rule_conversion_ghij123.json",
		"deployments/alert_rule_conversion_klmn123.json",
	}, d.config.alertsToAdd)
	assert.Equal(t, []string{
		"deployments/alert_rule_conversion_opqr123.json",
		"deployments/alert_rule_conversion_stuv123.json",
	}, d.config.alertsToRemove)
	assert.Equal(t, []string{
		"deployments/alert_rule_conversion_wxyz123.json",
		"deployments/alert_rule_conversion_123456789.json",
	}, d.config.alertsToUpdate)
}
