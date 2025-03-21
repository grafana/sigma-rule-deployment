package main

import (
	"context"
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
		wantFolderUid  string
		wantOrdId      int64
		wantAlertTitle string
		wantError      bool
	}{
		{
			name:           "valid alert",
			content:        `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`,
			wantAlertUid:   "abcd123",
			wantFolderUid:  "efgh456",
			wantOrdId:      23,
			wantAlertTitle: "Test alert",
			wantError:      false,
		},
		{
			name:           "invalid alert title",
			content:        `{"uid":"abcd123""`,
			wantAlertUid:   "",
			wantFolderUid:  "",
			wantOrdId:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "invalid alert uid",
			content:        `{"title":"Test alert"}`,
			wantAlertUid:   "",
			wantFolderUid:  "",
			wantOrdId:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "invalid folder uid",
			content:        `{"uid":"abcd123", "title":"Test alert"}`,
			wantAlertUid:   "",
			wantFolderUid:  "",
			wantOrdId:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "empty alert",
			content:        `{}`,
			wantAlertUid:   "",
			wantFolderUid:  "",
			wantOrdId:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "extra fields",
			content:        `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23, "extra":"field"}`,
			wantAlertUid:   "abcd123",
			wantFolderUid:  "efgh456",
			wantOrdId:      23,
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
				assert.Equal(t, tt.wantFolderUid, alert.FolderUid)
				assert.Equal(t, tt.wantOrdId, alert.OrgID)
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
	ctx := context.Background()

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
		client: server.Client(),
	}

	uid, err := d.updateAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`)
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestCreateAlert(t *testing.T) {
	ctx := context.Background()

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
		client: server.Client(),
	}

	uid, err := d.createAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`)
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestDeleteAlert(t *testing.T) {
	ctx := context.Background()

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
		client: server.Client(),
	}

	uid, err := d.deleteAlert(ctx, "abcd123")
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestListAlerts(t *testing.T) {
	ctx := context.Background()

	alertList := `[
		{
			"uid": "abcd123",
			"title": "Test alert",
			"folderUid": "efgh456",
			"orgID": 23
		},
		{
			"uid": "ijkl456",
			"title": "Test alert 2",
			"folderUid": "mnop789",
			"orgID": 23
		},
		{
			"uid": "qwerty123",
			"title": "Test alert 3",
			"folderUid": "efgh456",
			"orgID": 23
		},
		{
			"uid": "test123123",
			"title": "Test alert 4",
			"folderUid": "efgh456",
			"orgID": 1
		},
		{
			"uid": "newalert1",
			"title": "Test alert 5",
			"folderUid": "efgh456",
			"orgID": 23
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/provisioning/alert-rules" {
			t.Errorf("Expected to request '/api/v1/provisioning/alert-rules', got: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer my-test-token" {
			t.Errorf("Invalid Authorization header")
		}
		defer r.Body.Close()
		// Read the request body
		_, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(alertList))
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoint:  server.URL + "/",
			saToken:   "my-test-token",
			folderUid: "efgh456",
			orgId:     23,
		},
		client: server.Client(),
	}

	retrievedAlerts, err := d.listAlerts(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"abcd123", "qwerty123", "newalert1"}, retrievedAlerts)
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

	ctx := context.Background()
	d := NewDeployer()
	d.LoadConfig(ctx)
	assert.Equal(t, "my-test-token", d.config.saToken)
	assert.Equal(t, "https://myinstance.grafana.com/", d.config.endpoint)
	assert.Equal(t, "deployments", d.config.alertPath)
	assert.Equal(t, "abcdef123", d.config.folderUid)
	assert.Equal(t, int64(23), d.config.orgId)
	assert.Equal(t, false, d.config.freshDeploy)
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

func TestFakeAlertFilename(t *testing.T) {
	d := Deployer{
		config: deploymentConfig{
			alertPath: "deployments",
		},
		client: &http.Client{
			Timeout: requestTimeOut,
		},
	}
	assert.Equal(t, "abcd123", getAlertUidFromFilename(d.fakeAlertFilename("abcd123")))
}

func TestListAlertsInDeploymentFolder(t *testing.T) {
	d := Deployer{
		config: deploymentConfig{
			alertPath: "testdata",
			folderUid: "abcdef123",
			orgId:     1,
		},
		client: &http.Client{
			Timeout: requestTimeOut,
		},
	}
	alerts, err := d.listAlertsInDeploymentFolder()
	assert.NoError(t, err)
	assert.Equal(t, []string{"testdata/alert_rule_conversion_u123abc.json", "testdata/alert_rule_conversion_u456def.json", "testdata/alert_rule_conversion_u789ghi.json"}, alerts)
}
