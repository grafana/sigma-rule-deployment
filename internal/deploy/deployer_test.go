package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/stretchr/testify/assert"
)

const (
	contentTypeJSON = "application/json"
	//nolint:gosec
	authToken         = "Bearer my-test-token"
	alertingAPIPrefix = "/api/v1/provisioning/alert-rules"
)

func TestGetAlertUidFromFileName(t *testing.T) {
	assert.Equal(t, "abcd123", getAlertUIDFromFilename("alert_rule_conversion_test_file_1_abcd123.json"))
	assert.Equal(t, "abcd123", getAlertUIDFromFilename("alert_rule_conversion_name_test_file_2_abcd123.json"))
	assert.Equal(t, "uAaCwL1wlmA", getAlertUIDFromFilename("alert_rule_conversion_test_file_3_uAaCwL1wlmA.json"))
}

func TestParseAlert(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantAlertUID   string
		wantFolderUID  string
		wantOrdID      int64
		wantAlertTitle string
		wantError      bool
	}{
		{
			name:           "valid alert",
			content:        `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`,
			wantAlertUID:   "abcd123",
			wantFolderUID:  "efgh456",
			wantOrdID:      23,
			wantAlertTitle: "Test alert",
			wantError:      false,
		},
		{
			name:           "invalid alert title",
			content:        `{"uid":"abcd123""`,
			wantAlertUID:   "",
			wantFolderUID:  "",
			wantOrdID:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "invalid alert uid",
			content:        `{"title":"Test alert"}`,
			wantAlertUID:   "",
			wantFolderUID:  "",
			wantOrdID:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "invalid folder uid",
			content:        `{"uid":"abcd123", "title":"Test alert"}`,
			wantAlertUID:   "",
			wantFolderUID:  "",
			wantOrdID:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "empty alert",
			content:        `{}`,
			wantAlertUID:   "",
			wantFolderUID:  "",
			wantOrdID:      0,
			wantAlertTitle: "",
			wantError:      true,
		},
		{
			name:           "extra fields",
			content:        `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23, "extra":"field"}`,
			wantAlertUID:   "abcd123",
			wantFolderUID:  "efgh456",
			wantOrdID:      23,
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
				assert.Equal(t, tt.wantAlertUID, alert.UID)
				assert.Equal(t, tt.wantAlertTitle, alert.Title)
				assert.Equal(t, tt.wantFolderUID, alert.FolderUID)
				assert.Equal(t, tt.wantOrdID, alert.OrgID)
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
			file:          "deployments/alert_rule_conversion_test_file_1_abcd123.json",
			prefix:        "deployments",
			wantAlertList: []string{"deployments/alert_rule_conversion_test_file_1_abcd123.json"},
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

	server := mockServerUpdate(t, []string{
		`{"uid": "abcd123", "title": "Test alert", "folderUID": "efgh456", "orgID": 23}`,
	})
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoints: []string{server.URL + "/"},
			saToken:   "my-test-token",
		},
		client:         shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
		groupsToUpdate: map[*shared.GrafanaClient]map[ruleGroup]bool{},
	}

	// Update an alert
	deployment, created, err := d.updateAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, false, created)
	assert.Equal(t, AlertDeployment{UID: "abcd123", Title: "Test alert"}, deployment)

	// Try to update an alert that doesn't exist. This should lead to a creation
	deployment, created, err = d.updateAlert(ctx, `{"uid":"xyz123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, true, created)
	assert.Equal(t, AlertDeployment{UID: "xyz123", Title: "Test alert"}, deployment)
}

func mockServerUpdate(t *testing.T, existingAlerts []string) *httptest.Server {
	// Create a map of UIDs to alert objects
	alertsMap := make(map[string]string)
	for _, alert := range existingAlerts {
		newAlert, err := parseAlert(alert)
		assert.NoError(t, err)
		alertsMap[newAlert.UID] = alert
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We mock several scenarios:
		// 1. Normal update of an alert
		// 2. Update of an alert that doesn't exist -> create it

		defer r.Body.Close()
		// Read the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		if r.Header.Get("Content-Type") != contentTypeJSON {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
			return
		}
		if r.Header.Get("Authorization") != authToken {
			t.Errorf("Invalid Authorization header")
			return
		}
		if !strings.HasPrefix(r.URL.Path, alertingAPIPrefix) {
			t.Errorf("Expected URL to start with '%s', got: %s", alertingAPIPrefix, r.URL.Path)
			return
		}

		switch r.Method {
		case http.MethodPut:
			// Alert update
			// Check if alert exists
			uid := strings.TrimPrefix(r.URL.Path, alertingAPIPrefix+"/")
			if _, exists := alertsMap[uid]; !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write(body); err != nil {
				t.Errorf("failed to write response body: %v", err)
				return
			}
		case http.MethodPost:
			// Alert creation
			uid := strings.TrimPrefix(r.URL.Path, alertingAPIPrefix+"/")
			if _, exists := alertsMap[uid]; !exists {
				// Simulate a successful creation
				w.WriteHeader(http.StatusCreated)
				if _, err := w.Write(body); err != nil {
					t.Errorf("failed to write response body: %v", err)
					return
				}
				return
			}
			// Simulate a conflict
			w.WriteHeader(http.StatusConflict)
			errMsg := `{"message":"Alert conflict"}`
			if _, err := w.Write([]byte(errMsg)); err != nil {
				t.Errorf("failed to write response body: %v", err)
				return
			}
		}
	}))

	return server
}

func TestCreateAlert(t *testing.T) {
	ctx := context.Background()

	server := mockServerCreation(t, []string{
		`{"uid":"xyz123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`,
	})

	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoints: []string{server.URL + "/"},
			saToken:   "my-test-token",
		},
		client:         shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
		groupsToUpdate: map[*shared.GrafanaClient]map[ruleGroup]bool{},
	}

	// Create an alert
	deployment, updated, err := d.createAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, false, updated)
	assert.Equal(t, AlertDeployment{UID: "abcd123", Title: "Test alert"}, deployment)

	// Try to create an alert that already exists. This should lead to an update
	deployment, updated, err = d.createAlert(ctx, `{"uid":"xyz123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, true, updated)
	assert.Equal(t, AlertDeployment{UID: "xyz123", Title: "Test alert"}, deployment)

	// Simulate a conflict (same alert UID but different folder)
	_, _, err = d.createAlert(ctx, `{"uid":"xyz123","title":"Test alert", "folderUID": "efgh789", "orgID": 23}`, true)
	assert.NotNil(t, err)

	// Simulate a conflict (same alert UID but different org)
	_, _, err = d.createAlert(ctx, `{"uid":"xyz123","title":"Test alert", "folderUID": "efgh456", "orgID": 45}`, true)
	assert.NotNil(t, err)
}

func mockServerCreation(t *testing.T, existingAlerts []string) *httptest.Server {
	// Create a map of UIDs to alert objects
	alertsMap := make(map[string]string)
	for _, alert := range existingAlerts {
		newAlert, err := parseAlert(alert)
		assert.NoError(t, err)
		alertsMap[newAlert.UID] = alert
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		// Read the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		// Check that the request methods and URLs are what we expect
		if r.Header.Get("Content-Type") != contentTypeJSON {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
			return
		}
		if r.Header.Get("Authorization") != authToken {
			t.Errorf("Invalid Authorization header")
			return
		}
		if !strings.HasPrefix(r.URL.Path, alertingAPIPrefix) {
			t.Errorf("Expected URL to start with '%s', got: %s", alertingAPIPrefix, r.URL.Path)
			return
		}

		// We mock several scenarios:
		// 1. Normal creation of an alert
		// 2. Conflict when creating an alert (same UID but different folder)

		switch r.Method {
		// Creation of an alert
		case http.MethodPost:
			// Parse alert content from request body
			alertContent := string(body)
			alert, err := parseAlert(alertContent)
			if err != nil {
				t.Errorf("failed to parse alert: %v", err)
				return
			}
			// Check if the alert UID is present in the request
			if alert.UID == "" {
				t.Errorf("alert UID is missing in the request")
				return
			}

			// Check if it's an existing alert
			if _, exists := alertsMap[alert.UID]; exists {
				// If the alert already exists, we simulate a conflict
				w.WriteHeader(http.StatusConflict)
				errMsg := `{"message":"Alert conflict"}`
				if _, err := w.Write([]byte(errMsg)); err != nil {
					t.Errorf("failed to write response body: %v", err)
					return
				}
				return
			}
			// Otherwise, we simulate a successful creation
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write(body); err != nil {
				t.Errorf("failed to write response body: %v", err)
				return
			}
			return
		// Retrieve alert info during a conflict
		case http.MethodGet:
			uid := strings.TrimPrefix(r.URL.Path, alertingAPIPrefix+"/")
			// Check if it's an existing alert
			if alert, exists := alertsMap[uid]; exists {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(alert)); err != nil {
					t.Errorf("failed to write response body: %v", err)
					return
				}
				return
			}
			t.Errorf("alert UID '%s' not found in the mock server", uid)
			w.WriteHeader(http.StatusNotFound)
			return
		// Update an existing alert
		case http.MethodPut:
			// Simulate an update
			uid := strings.TrimPrefix(r.URL.Path, alertingAPIPrefix+"/")
			// Check if it's an existing alert
			if _, exists := alertsMap[uid]; exists {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write(body); err != nil {
					t.Errorf("failed to write response body: %v", err)
					return
				}
				return
			}
			t.Errorf("alert UID '%s' not found in the mock server", uid)
			w.WriteHeader(http.StatusNotFound)
			return
		default:
			t.Errorf("Unexpected method: %s", r.Method)
			return
		}
	}))

	return server
}

func TestDeleteAlert(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, alertingAPIPrefix) {
			uid := strings.TrimPrefix(r.URL.Path, alertingAPIPrefix+"/")
			if uid != "abcd123" {
				t.Errorf("Expected to request '%s/abcd123', got: %s", alertingAPIPrefix, r.URL.Path)
			}
		} else {
			t.Errorf("Expected to request '%s/abcd123', got: %s", alertingAPIPrefix, r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") != authToken {
			t.Errorf("Invalid Authorization header")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoints: []string{server.URL + "/"},
			saToken:   "my-test-token",
		},
		client: shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
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
			"folderUID": "efgh456",
			"orgID": 23
		},
		{
			"uid": "ijkl456",
			"title": "Test alert 2",
			"folderUID": "mnop789",
			"orgID": 23
		},
		{
			"uid": "qwerty123",
			"title": "Test alert 3",
			"folderUID": "efgh456",
			"orgID": 23
		},
		{
			"uid": "test123123",
			"title": "Test alert 4",
			"folderUID": "efgh456",
			"orgID": 1
		},
		{
			"uid": "newalert1",
			"title": "Test alert 5",
			"folderUID": "efgh456",
			"orgID": 23
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != alertingAPIPrefix {
			t.Errorf("Expected to request '%s', got: %s", alertingAPIPrefix, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != contentTypeJSON {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
		}
		switch r.Method {
		case http.MethodGet:
			// Validate authorization header
			assert.Equal(t, authToken, r.Header.Get("Authorization"))

			// Return a list of alerts
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(alertList)); err != nil {
				t.Errorf("failed to write alert list: %v", err)
				return
			}
		default:
			t.Errorf("Unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoints: []string{server.URL + "/"},
			saToken:   "my-test-token",
			folderUID: "efgh456",
			orgID:     23,
		},
		client: shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
	}

	retrievedAlerts, err := d.listAlerts(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"abcd123", "qwerty123", "newalert1"}, retrievedAlerts)
}

// TestListAlertsMultipleFolderOrgPairs verifies that alerts are matched against every
// configured (folder, org) pair, not just the default one, so alerts that live in a
// conversion-specific folder_id/org_id are still found during fresh-deploy discovery.
func TestListAlertsMultipleFolderOrgPairs(t *testing.T) {
	ctx := context.Background()

	alertList := `[
		{"uid": "abcd123", "title": "Test alert", "folderUID": "efgh456", "orgID": 23},
		{"uid": "ijkl456", "title": "Test alert 2", "folderUID": "mnop789", "orgID": 23},
		{"uid": "test123123", "title": "Test alert 4", "folderUID": "efgh456", "orgID": 1}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(alertList))
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoints: []string{server.URL + "/"},
			saToken:   "my-test-token",
			folderUID: "efgh456",
			orgID:     23,
			folderOrgPairs: []folderOrg{
				{folderUID: "efgh456", orgID: 23},
				{folderUID: "mnop789", orgID: 23},
			},
		},
		client: shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
	}

	retrievedAlerts, err := d.listAlerts(ctx)
	assert.NoError(t, err)
	// abcd123 (efgh456/23) and ijkl456 (mnop789/23) match a configured pair;
	// test123123 (efgh456/1) does not match any configured pair.
	assert.ElementsMatch(t, []string{"abcd123", "ijkl456"}, retrievedAlerts)
}

func TestLoadConfig(t *testing.T) {
	// Set up environment variables
	os.Setenv("CONFIG_PATH", "test_config.yml")
	defer os.Unsetenv("CONFIG_PATH")
	os.Setenv("DEPLOYER_GRAFANA_SA_TOKEN", "my-test-token")
	defer os.Unsetenv("DEPLOYER_GRAFANA_SA_TOKEN")
	os.Setenv("ADDED_FILES", "deployments/alert_rule_conversion_test_file_1_abcd123.json deployments/alert_rule_conversion_test_file_2_def3456789.json")
	defer os.Unsetenv("ADDED_FILES")
	os.Setenv("COPIED_FILES", "deployments/alert_rule_conversion_test_file_3_ghij123.json deployments/alert_rule_conversion_test_file_4_klmn123.json")
	defer os.Unsetenv("COPIED_FILES")
	os.Setenv("DELETED_FILES", "deployments/alert_rule_conversion_test_file_5_opqr123.json deployments/alert_rule_conversion_test_file_6_stuv123.json")
	defer os.Unsetenv("DELETED_FILES")
	os.Setenv("MODIFIED_FILES", "deployments/alert_rule_conversion_test_file_7_wxyz123.json deployments/alert_rule_conversion_test_file_8_123456789.json")
	defer os.Unsetenv("MODIFIED_FILES")

	ctx := context.Background()
	d := NewDeployer()
	err := d.LoadConfig(ctx)
	assert.NoError(t, err)
	if d.config.freshDeploy {
		err = d.ConfigFreshDeployment(ctx)
	} else {
		err = d.ConfigNormalMode()
	}
	assert.NoError(t, err)

	// Test basic config values
	assert.Equal(t, "my-test-token", d.config.saToken)
	assert.Equal(t, []string{"https://myinstance.grafana.com"}, d.config.endpoints)
	assert.Equal(t, "deployments", d.config.alertPath)
	assert.Equal(t, "abcdef123", d.config.folderUID)
	assert.Equal(t, int64(23), d.config.orgID)
	assert.Equal(t, false, d.config.freshDeploy)

	// Test alert file lists
	assert.Equal(t, []string{
		"deployments/alert_rule_conversion_test_file_1_abcd123.json",
		"deployments/alert_rule_conversion_test_file_2_def3456789.json",
		"deployments/alert_rule_conversion_test_file_3_ghij123.json",
		"deployments/alert_rule_conversion_test_file_4_klmn123.json",
	}, d.config.alertsToAdd)
	assert.Equal(t, []string{
		"deployments/alert_rule_conversion_test_file_5_opqr123.json",
		"deployments/alert_rule_conversion_test_file_6_stuv123.json",
	}, d.config.alertsToRemove)
	assert.Equal(t, []string{
		"deployments/alert_rule_conversion_test_file_7_wxyz123.json",
		"deployments/alert_rule_conversion_test_file_8_123456789.json",
	}, d.config.alertsToUpdate)

	// Test group intervals
	// group4_config overrides folder_id/org_id per-conversion; the others fall back to Defaults.
	expectedIntervals := map[ruleGroup]int64{
		{folderUID: "abcdef123", name: "group1"}: 600,   // 10m in seconds
		{folderUID: "abcdef123", name: "group2"}: 3600,  // 1h in seconds
		{folderUID: "abcdef123", name: "group3"}: 21600, // 6h (default) in seconds
		{folderUID: "other456", name: "group4"}:  300,   // 5m in seconds
	}

	assert.Equal(t, expectedIntervals, d.config.groupsIntervals)
	assert.ElementsMatch(t, []folderOrg{
		{folderUID: "abcdef123", orgID: 23},
		{folderUID: "other456", orgID: 99},
	}, d.config.folderOrgPairs)
}

func TestListAlertsInDeploymentFolder(t *testing.T) {
	d := Deployer{
		config: deploymentConfig{
			alertPath: "testdata",
			folderUID: "abcdef123",
			orgID:     1,
		},
		client: shared.NewGrafanaClient("", "", "sigma-rule-deployment/deployer", defaultRequestTimeout),
	}
	alerts, err := d.listAlertsInDeploymentFolder()
	assert.NoError(t, err)
	assert.Equal(t, []string{"testdata/alert_rule_conversion_test_file_1_u123abc.json", "testdata/alert_rule_conversion_test_file_2_u456def.json", "testdata/alert_rule_conversion_test_file_3_u789ghi.json"}, alerts)
}

func TestDeployToMultipleGrafanaInstances(t *testing.T) {
	ctx := context.Background()

	// Track which alert UIDs each server received.
	server1UIDs := []string{}
	server2UIDs := []string{}

	// makeAlertServer returns a mock server that records created alert UIDs and responds to rule-group requests.
	// The provided uids slice is appended to on each POST (alert creation).
	makeAlertServer := func(receivedUIDs *[]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed to read request body: %v", err)
				return
			}

			switch {
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules") && r.Method == http.MethodPost:
				alert, err := parseAlert(string(body))
				if err != nil {
					t.Errorf("failed to parse alert: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				*receivedUIDs = append(*receivedUIDs, alert.UID)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write(body)

			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/folder/") && r.Method == http.MethodGet:
				// Return a rule group with interval matching what the deployer expects, so no PUT is needed
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"folderUID":"folder1","interval":300,"rules":[],"title":"group1"}`))

			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	server1 := makeAlertServer(&server1UIDs)
	defer server1.Close()
	server2 := makeAlertServer(&server2UIDs)
	defer server2.Close()

	// Write alert files to a relative temp directory (ReadLocalFile requires a local/relative path)
	tmpDir, err := os.MkdirTemp(".", "test-multi-grafana-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	conv1Alert := `{"uid":"conv1uid1","title":"Conv1 Alert","folderUID":"folder1","orgID":1,"ruleGroup":"group1"}`
	conv2Alert := `{"uid":"conv2uid1","title":"Conv2 Alert","folderUID":"folder1","orgID":1,"ruleGroup":"group1"}`

	conv1File := filepath.Join(tmpDir, "alert_rule_conv1_myrule_conv1uid1.json")
	conv2File := filepath.Join(tmpDir, "alert_rule_conv2_myrule_conv2uid1.json")
	assert.NoError(t, os.WriteFile(conv1File, []byte(conv1Alert), 0o600))
	assert.NoError(t, os.WriteFile(conv2File, []byte(conv2Alert), 0o600))

	d := Deployer{
		config: deploymentConfig{
			endpoints:       []string{server1.URL + "/"},
			saToken:         "my-test-token",
			alertPath:       tmpDir,
			folderUID:       "folder1",
			alertsToAdd:     []string{conv1File, conv2File},
			groupsIntervals: map[ruleGroup]int64{{folderUID: "folder1", name: "group1"}: 300},
		},
		clients: []*shared.GrafanaClient{
			shared.NewGrafanaClient(server1.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
		},
		perConversionClients: map[string][]*shared.GrafanaClient{
			"conv1": {shared.NewGrafanaClient(server1.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", 5*time.Second)},
			"conv2": {shared.NewGrafanaClient(server2.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", 30*time.Second)},
		},
		groupsToUpdate: map[*shared.GrafanaClient]map[ruleGroup]bool{},
	}

	created, updated, deleted, err := d.Deploy(ctx)
	assert.NoError(t, err)
	// Each alert records the instance it was deployed to
	assert.Contains(t, created, AlertDeployment{UID: "conv1uid1", Title: "Conv1 Alert", Instance: server1.URL + "/"})
	assert.Contains(t, created, AlertDeployment{UID: "conv2uid1", Title: "Conv2 Alert", Instance: server2.URL + "/"})
	assert.Empty(t, updated)
	assert.Empty(t, deleted)

	// conv1 alert should have gone to server1, conv2 alert to server2
	assert.Contains(t, server1UIDs, "conv1uid1")
	assert.NotContains(t, server1UIDs, "conv2uid1")
	assert.Contains(t, server2UIDs, "conv2uid1")
	assert.NotContains(t, server2UIDs, "conv1uid1")
}

func TestUpdateAlertGroupInterval(t *testing.T) {
	testCases := []struct {
		name               string
		folderUID          string
		group              string
		interval           int64
		currentInterval    int64
		getStatusCode      int
		putStatusCode      int
		expectError        bool
		expectPutRequest   bool
		responseBody       string
		expectedRequestURL string
	}{
		{
			name:               "successful interval update",
			folderUID:          "folder123",
			group:              "group1",
			interval:           600, // 10m
			currentInterval:    300, // 5m
			getStatusCode:      http.StatusOK,
			putStatusCode:      http.StatusOK,
			expectError:        false,
			expectPutRequest:   true,
			responseBody:       `{"folderUID":"folder123","interval":300,"rules":[],"title":"group1"}`,
			expectedRequestURL: "/api/v1/provisioning/folder/folder123/rule-groups/group1",
		},
		{
			name:               "interval already set correctly",
			folderUID:          "folder123",
			group:              "group2",
			interval:           600, // 10m
			currentInterval:    600, // 10m (already correct)
			getStatusCode:      http.StatusOK,
			putStatusCode:      http.StatusOK, // Should not be used
			expectError:        false,
			expectPutRequest:   false, // No PUT should be made
			responseBody:       `{"folderUID":"folder123","interval":600,"rules":[],"title":"group2"}`,
			expectedRequestURL: "/api/v1/provisioning/folder/folder123/rule-groups/group2",
		},
		{
			name:               "get request returns error",
			folderUID:          "folder123",
			group:              "group3",
			interval:           600,
			currentInterval:    300,
			getStatusCode:      http.StatusNotFound,
			putStatusCode:      http.StatusOK, // Should not be used
			expectError:        true,
			expectPutRequest:   false,
			responseBody:       `{"message":"Alert rule group not found"}`,
			expectedRequestURL: "/api/v1/provisioning/folder/folder123/rule-groups/group3",
		},
		{
			name:               "put request returns error",
			folderUID:          "folder123",
			group:              "group4",
			interval:           600,
			currentInterval:    300,
			getStatusCode:      http.StatusOK,
			putStatusCode:      http.StatusBadRequest,
			expectError:        true,
			expectPutRequest:   true,
			responseBody:       `{"folderUID":"folder123","interval":300,"rules":[],"title":"group4"}`,
			expectedRequestURL: "/api/v1/provisioning/folder/folder123/rule-groups/group4",
		},
		{
			name:               "special characters in folder and group",
			folderUID:          "folder-with_special.chars",
			group:              "group-with_special.chars",
			interval:           3600, // 1h
			currentInterval:    600,  // 10m
			getStatusCode:      http.StatusOK,
			putStatusCode:      http.StatusOK,
			expectError:        false,
			expectPutRequest:   true,
			responseBody:       `{"folderUID":"folder-with_special.chars","interval":600,"rules":[],"title":"group-with_special.chars"}`,
			expectedRequestURL: "/api/v1/provisioning/folder/folder-with_special.chars/rule-groups/group-with_special.chars",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			putRequestMade := false

			// Create a test server that validates our requests
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check the URL is what we expect
				assert.Equal(t, tc.expectedRequestURL, r.URL.Path)

				// Validate authorization header
				assert.Equal(t, authToken, r.Header.Get("Authorization"))

				switch r.Method {
				case http.MethodGet:
					// Return the mocked response for GET
					w.WriteHeader(tc.getStatusCode)
					_, err := w.Write([]byte(tc.responseBody))
					assert.NoError(t, err)
				case http.MethodPut:
					// Mark that a PUT request was made
					putRequestMade = true

					// Validate the PUT request contains the updated interval
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)

					var updatedGroup model.AlertRuleGroup
					err = json.Unmarshal(body, &updatedGroup)
					assert.NoError(t, err)

					// Verify interval was updated
					assert.Equal(t, tc.interval, updatedGroup.Interval)

					// Return status code based on test case
					w.WriteHeader(tc.putStatusCode)
				default:
					t.Errorf("Unexpected HTTP method: %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()

			// Create a deployer with mocked client and config
			d := Deployer{
				config: deploymentConfig{
					endpoints: []string{server.URL + "/"},
					saToken:   "my-test-token",
				},
				client: shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
			}

			// Call the function being tested
			err := d.updateAlertGroupInterval(context.Background(), tc.folderUID, tc.group, tc.interval)

			// Verify error expectation
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify if PUT request was made or not
			assert.Equal(t, tc.expectPutRequest, putRequestMade,
				"Expected PUT request to be %v but was %v", tc.expectPutRequest, putRequestMade)
		})
	}
}

// TestLoadConfigPerConversionInstancesAndTimeout verifies that LoadConfig correctly reads
// per-configuration GrafanaInstance and Timeout values and creates per-conversion clients
// that route requests to the right Grafana endpoint, and that conv_c (no override) falls
// back to the default client.
func TestLoadConfigPerConversionInstancesAndTimeout(t *testing.T) {
	ctx := context.Background()

	serverAUIDs := []string{}
	serverBUIDs := []string{}
	defaultUIDs := []string{}

	makeServer := func(receivedUIDs *[]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			body, _ := io.ReadAll(r.Body)
			switch {
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules") && r.Method == http.MethodPost:
				alert, err := parseAlert(string(body))
				if err != nil {
					t.Errorf("failed to parse alert: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				*receivedUIDs = append(*receivedUIDs, alert.UID)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write(body)
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/folder/") && r.Method == http.MethodGet:
				// Return the correct interval for each group so no PUT is triggered.
				intervalByGroup := map[string]int{
					"group_a": 300,
					"group_b": 600,
					"group_c": 900,
				}
				parts := strings.Split(r.URL.Path, "/")
				group := parts[len(parts)-1]
				interval := intervalByGroup[group]
				w.WriteHeader(http.StatusOK)
				body := fmt.Sprintf(`{"folderUID":"default-folder","interval":%d,"rules":[],"title":"%s"}`, interval, group)
				_, _ = w.Write([]byte(body)) //nolint:gosec // G705: group comes from the URL path in a test-only mock handler
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	serverA := makeServer(&serverAUIDs)
	defer serverA.Close()
	serverB := makeServer(&serverBUIDs)
	defer serverB.Close()
	serverDefault := makeServer(&defaultUIDs)
	defer serverDefault.Close()

	// Write a config YAML that points conv_a and conv_b at their own servers, and conv_c at the default.
	configContent := "version: 2\n" +
		"folders:\n  deployment_path: \"./deployments\"\n" +
		"defaults:\n" +
		"  integration:\n    folder_id: default-folder\n    org_id: 1\n" +
		"  deployment:\n    grafana_instance: " + serverDefault.URL + "\n    timeout: 10s\n" +
		"configurations:\n" +
		"  - name: conv_a\n    integration:\n      rule_group: \"group_a\"\n      time_window: \"5m\"\n" +
		"    deployment:\n      grafana_instance: " + serverA.URL + "\n      timeout: 5s\n" +
		"  - name: conv_b\n    integration:\n      rule_group: \"group_b\"\n      time_window: \"10m\"\n" +
		"    deployment:\n      grafana_instance: " + serverB.URL + "\n      timeout: 30s\n" +
		"  - name: conv_c\n    integration:\n      rule_group: \"group_c\"\n      time_window: \"15m\"\n"

	configFile, err := os.CreateTemp(".", "test_per_instance_config_*.yml")
	assert.NoError(t, err)
	t.Cleanup(func() { os.Remove(configFile.Name()) })
	_, err = configFile.WriteString(configContent)
	assert.NoError(t, err)
	assert.NoError(t, configFile.Close())

	// Write alert files for each conversion in a temp dir.
	tmpDir, err := os.MkdirTemp(".", "test-per-instance-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	convAAlert := `{"uid":"conv-a-uid","title":"Conv A Alert","folderUID":"default-folder","orgID":1,"ruleGroup":"group_a"}`
	convBAlert := `{"uid":"conv-b-uid","title":"Conv B Alert","folderUID":"default-folder","orgID":1,"ruleGroup":"group_b"}`
	convCAlert := `{"uid":"conv-c-uid","title":"Conv C Alert","folderUID":"default-folder","orgID":1,"ruleGroup":"group_c"}`

	convAFile := filepath.Join(tmpDir, "alert_rule_conv_a_rule_conv-a-uid.json")
	convBFile := filepath.Join(tmpDir, "alert_rule_conv_b_rule_conv-b-uid.json")
	convCFile := filepath.Join(tmpDir, "alert_rule_conv_c_rule_conv-c-uid.json")
	assert.NoError(t, os.WriteFile(convAFile, []byte(convAAlert), 0o600))
	assert.NoError(t, os.WriteFile(convBFile, []byte(convBAlert), 0o600))
	assert.NoError(t, os.WriteFile(convCFile, []byte(convCAlert), 0o600))

	os.Setenv("CONFIG_PATH", configFile.Name())
	defer os.Unsetenv("CONFIG_PATH")
	os.Setenv("DEPLOYER_GRAFANA_SA_TOKEN", "my-test-token")
	defer os.Unsetenv("DEPLOYER_GRAFANA_SA_TOKEN")

	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(ctx))

	// Verify per-conversion clients were built for conv_a and conv_b but not conv_c.
	assert.Len(t, d.perConversionClients, 2, "expected clients for conv_a and conv_b only")
	assert.Contains(t, d.perConversionClients, "conv_a")
	assert.Contains(t, d.perConversionClients, "conv_b")
	assert.NotContains(t, d.perConversionClients, "conv_c")

	// Verify per-config TimeWindow values were recorded correctly.
	assert.Equal(t, map[ruleGroup]int64{
		{folderUID: "default-folder", name: "group_a"}: 300, // 5m
		{folderUID: "default-folder", name: "group_b"}: 600, // 10m
		{folderUID: "default-folder", name: "group_c"}: 900, // 15m
	}, d.config.groupsIntervals)

	// Deploy all three alert files and verify routing: conv_a → serverA, conv_b → serverB, conv_c → default.
	d.config.alertsToAdd = []string{convAFile, convBFile, convCFile}
	d.SetClient()

	_, _, _, err = d.Deploy(ctx) //nolint:dogsled
	assert.NoError(t, err)

	assert.Contains(t, serverAUIDs, "conv-a-uid", "conv_a alert should route to serverA")
	assert.NotContains(t, serverAUIDs, "conv-b-uid")
	assert.NotContains(t, serverAUIDs, "conv-c-uid")

	assert.Contains(t, serverBUIDs, "conv-b-uid", "conv_b alert should route to serverB")
	assert.NotContains(t, serverBUIDs, "conv-a-uid")
	assert.NotContains(t, serverBUIDs, "conv-c-uid")

	assert.Contains(t, defaultUIDs, "conv-c-uid", "conv_c alert should route to the default server")
	assert.NotContains(t, defaultUIDs, "conv-a-uid")
	assert.NotContains(t, defaultUIDs, "conv-b-uid")
}

// TestDeployWithMultipleInstanceListInConfig verifies that grafana_instance can be specified
// as a YAML list in both the defaults and per-conversion sections. Alerts without a
// per-conversion override are deployed to all default instances; alerts with a
// per-conversion list are deployed to each instance in that list.
func TestDeployWithMultipleInstanceListInConfig(t *testing.T) {
	ctx := context.Background()

	server1UIDs := []string{}
	server2UIDs := []string{}
	overrideUIDs := []string{}

	makeServer := func(receivedUIDs *[]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			body, _ := io.ReadAll(r.Body)
			switch {
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules") && r.Method == http.MethodPost:
				alert, err := parseAlert(string(body))
				if err != nil {
					t.Errorf("failed to parse alert: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				*receivedUIDs = append(*receivedUIDs, alert.UID)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write(body)
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/folder/") && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"folderUID":"folder1","interval":300,"rules":[],"title":"group1"}`))
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	server1 := makeServer(&server1UIDs)
	defer server1.Close()
	server2 := makeServer(&server2UIDs)
	defer server2.Close()
	overrideServer := makeServer(&overrideUIDs)
	defer overrideServer.Close()

	// defaults has two instances (server1 and server2) as a YAML list.
	// conv_multi overrides with its own list [overrideServer, server1].
	// conv_default has no override and falls back to both default instances.
	configContent := "version: 2\n" +
		"folders:\n  deployment_path: \"./deployments\"\n" +
		"defaults:\n" +
		"  integration:\n    folder_id: folder1\n    org_id: 1\n" +
		"  deployment:\n    grafana_instance:\n      - " + server1.URL + "\n      - " + server2.URL + "\n    timeout: 10s\n" +
		"configurations:\n" +
		"  - name: conv_multi\n    integration:\n      rule_group: \"group1\"\n      time_window: \"5m\"\n" +
		"    deployment:\n      grafana_instance:\n        - " + overrideServer.URL + "\n        - " + server1.URL + "\n" +
		"  - name: conv_default\n    integration:\n      rule_group: \"group1\"\n      time_window: \"5m\"\n"

	configFile, err := os.CreateTemp(".", "test_multi_instance_list_*.yml")
	assert.NoError(t, err)
	t.Cleanup(func() { os.Remove(configFile.Name()) })
	_, err = configFile.WriteString(configContent)
	assert.NoError(t, err)
	assert.NoError(t, configFile.Close())

	tmpDir, err := os.MkdirTemp(".", "test-multi-instance-list-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	convMultiAlert := `{"uid":"conv-multi-uid","title":"Conv Multi Alert","folderUID":"folder1","orgID":1,"ruleGroup":"group1"}`
	convDefaultAlert := `{"uid":"conv-default-uid","title":"Conv Default Alert","folderUID":"folder1","orgID":1,"ruleGroup":"group1"}`

	convMultiFile := filepath.Join(tmpDir, "alert_rule_conv_multi_rule_conv-multi-uid.json")
	convDefaultFile := filepath.Join(tmpDir, "alert_rule_conv_default_rule_conv-default-uid.json")
	assert.NoError(t, os.WriteFile(convMultiFile, []byte(convMultiAlert), 0o600))
	assert.NoError(t, os.WriteFile(convDefaultFile, []byte(convDefaultAlert), 0o600))

	os.Setenv("CONFIG_PATH", configFile.Name())
	defer os.Unsetenv("CONFIG_PATH")
	os.Setenv("DEPLOYER_GRAFANA_SA_TOKEN", "my-test-token")
	defer os.Unsetenv("DEPLOYER_GRAFANA_SA_TOKEN")

	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(ctx))

	assert.Len(t, d.perConversionClients, 1, "only conv_multi should have a per-conversion override")
	assert.Len(t, d.perConversionClients["conv_multi"], 2, "conv_multi should have 2 clients")

	d.SetClient()
	assert.Len(t, d.clients, 2, "two default clients from the list")

	d.config.alertsToAdd = []string{convMultiFile, convDefaultFile}

	_, _, _, err = d.Deploy(ctx) //nolint:dogsled
	assert.NoError(t, err)

	// conv_multi alert deployed to overrideServer and server1 (its list), not server2
	assert.Contains(t, overrideUIDs, "conv-multi-uid")
	assert.Contains(t, server1UIDs, "conv-multi-uid")
	assert.NotContains(t, server2UIDs, "conv-multi-uid")

	// conv_default alert deployed to both default instances (server1 and server2), not overrideServer
	assert.Contains(t, server1UIDs, "conv-default-uid")
	assert.Contains(t, server2UIDs, "conv-default-uid")
	assert.NotContains(t, overrideUIDs, "conv-default-uid")
}

// TestDeployUsesPerConversionFolderForGroupIntervalUpdate verifies that when a rule group's
// folder_id is overridden per-conversion, the alert-group-interval update targets that
// conversion's folder rather than always using the global default folder.
func TestDeployUsesPerConversionFolderForGroupIntervalUpdate(t *testing.T) {
	ctx := context.Background()

	var folderPaths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules") && r.Method == http.MethodPost:
			if _, err := parseAlert(string(body)); err != nil {
				t.Errorf("failed to parse alert: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
		case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/folder/") && r.Method == http.MethodGet:
			folderPaths = append(folderPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"folderUID":"x","interval":300,"rules":[],"title":"x"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// conv_default has no folder/org override and falls back to Defaults.
	// conv_override sets its own folder_id/org_id and rule_group.
	configContent := "version: 2\n" +
		"folders:\n  deployment_path: \"./deployments\"\n" +
		"defaults:\n" +
		"  integration:\n    folder_id: default-folder\n    org_id: 1\n" +
		"  deployment:\n    grafana_instance: " + server.URL + "\n" +
		"configurations:\n" +
		"  - name: conv_default\n    integration:\n      rule_group: \"normal_group\"\n      time_window: \"5m\"\n" +
		"  - name: conv_override\n    integration:\n      rule_group: \"special_group\"\n      time_window: \"5m\"\n      folder_id: special-folder\n      org_id: 2\n"

	configFile, err := os.CreateTemp(".", "test_folder_override_*.yml")
	assert.NoError(t, err)
	t.Cleanup(func() { os.Remove(configFile.Name()) })
	_, err = configFile.WriteString(configContent)
	assert.NoError(t, err)
	assert.NoError(t, configFile.Close())

	tmpDir, err := os.MkdirTemp(".", "test-folder-override-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	defaultAlert := `{"uid":"default-uid","title":"Default Alert","folderUID":"default-folder","orgID":1,"ruleGroup":"normal_group"}`
	overrideAlert := `{"uid":"override-uid","title":"Override Alert","folderUID":"special-folder","orgID":2,"ruleGroup":"special_group"}`

	defaultFile := filepath.Join(tmpDir, "alert_rule_conv_default_rule_default-uid.json")
	overrideFile := filepath.Join(tmpDir, "alert_rule_conv_override_rule_override-uid.json")
	assert.NoError(t, os.WriteFile(defaultFile, []byte(defaultAlert), 0o600))
	assert.NoError(t, os.WriteFile(overrideFile, []byte(overrideAlert), 0o600))

	os.Setenv("CONFIG_PATH", configFile.Name())
	defer os.Unsetenv("CONFIG_PATH")
	os.Setenv("DEPLOYER_GRAFANA_SA_TOKEN", "my-test-token")
	defer os.Unsetenv("DEPLOYER_GRAFANA_SA_TOKEN")

	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(ctx))
	d.SetClient()
	d.config.alertsToAdd = []string{defaultFile, overrideFile}

	_, _, _, err = d.Deploy(ctx) //nolint:dogsled
	assert.NoError(t, err)

	assert.Contains(t, folderPaths, "/api/v1/provisioning/folder/default-folder/rule-groups/normal_group")
	assert.Contains(t, folderPaths, "/api/v1/provisioning/folder/special-folder/rule-groups/special_group")
}

func TestDeployGroupIntervalUpdatesOnlyOnTouchedInstances(t *testing.T) {
	// makeServer returns a mock Grafana that records the rule-group paths it is asked about.
	// Rule groups it doesn't know about return 404, like a real instance that never received them.
	makeServer := func(t *testing.T, knownGroups map[string]bool, groupGets *[]string) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			body, _ := io.ReadAll(r.Body)
			switch {
			case strings.HasPrefix(r.URL.Path, alertingAPIPrefix) && r.Method == http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write(body)
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/folder/") && r.Method == http.MethodGet:
				*groupGets = append(*groupGets, r.URL.Path)
				if !knownGroups[r.URL.Path] {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"folderUID":"x","interval":300,"rules":[],"title":"x"}`))
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	const (
		pathXGroup  = "/api/v1/provisioning/folder/folder-x/rule-groups/group_x"
		pathYGroup  = "/api/v1/provisioning/folder/folder-y/rule-groups/group_y"
		pathXShared = "/api/v1/provisioning/folder/folder-x/rule-groups/shared"
		pathYShared = "/api/v1/provisioning/folder/folder-y/rule-groups/shared"
	)

	testCases := []struct {
		name         string
		alerts       map[string]string // file name -> alert JSON
		wantXGets    []string
		wantYGets    []string
		xKnownGroups map[string]bool
		yKnownGroups map[string]bool
	}{
		{
			name: "alert only on the overridden instance does not query the default instance",
			alerts: map[string]string{
				"alert_rule_conv_y_rule_uid-y.json": `{"uid":"uid-y","title":"Y","folderUID":"folder-y","orgID":1,"ruleGroup":"group_y"}`,
			},
			yKnownGroups: map[string]bool{pathYGroup: true},
			wantYGets:    []string{pathYGroup},
		},
		{
			name: "each instance is only queried for its own groups",
			alerts: map[string]string{
				"alert_rule_conv_x_rule_uid-x.json": `{"uid":"uid-x","title":"X","folderUID":"folder-x","orgID":1,"ruleGroup":"group_x"}`,
				"alert_rule_conv_y_rule_uid-y.json": `{"uid":"uid-y","title":"Y","folderUID":"folder-y","orgID":1,"ruleGroup":"group_y"}`,
			},
			xKnownGroups: map[string]bool{pathXGroup: true},
			yKnownGroups: map[string]bool{pathYGroup: true},
			wantXGets:    []string{pathXGroup},
			wantYGets:    []string{pathYGroup},
		},
		{
			name: "same group name on both instances is updated once on each",
			alerts: map[string]string{
				"alert_rule_conv_x_rule_uid-x.json": `{"uid":"uid-x","title":"X","folderUID":"folder-x","orgID":1,"ruleGroup":"shared"}`,
				"alert_rule_conv_y_rule_uid-y.json": `{"uid":"uid-y","title":"Y","folderUID":"folder-y","orgID":1,"ruleGroup":"shared"}`,
			},
			xKnownGroups: map[string]bool{pathXShared: true},
			yKnownGroups: map[string]bool{pathYShared: true},
			wantXGets:    []string{pathXShared},
			wantYGets:    []string{pathYShared},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var xGets, yGets []string
			serverX := makeServer(t, tc.xKnownGroups, &xGets)
			defer serverX.Close()
			serverY := makeServer(t, tc.yKnownGroups, &yGets)
			defer serverY.Close()

			tmpDir, err := os.MkdirTemp(".", "test-group-interval-*")
			assert.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			files := make([]string, 0, len(tc.alerts))
			for name, content := range tc.alerts {
				file := filepath.Join(tmpDir, name)
				assert.NoError(t, os.WriteFile(file, []byte(content), 0o600))
				files = append(files, file)
			}

			newClient := func(url string) *shared.GrafanaClient {
				return shared.NewGrafanaClient(url+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout)
			}
			d := NewDeployer()
			d.config = deploymentConfig{
				alertPath:   tmpDir,
				alertsToAdd: files,
				groupsIntervals: map[ruleGroup]int64{
					{folderUID: "folder-x", name: "group_x"}: 300,
					{folderUID: "folder-y", name: "group_y"}: 300,
					{folderUID: "folder-x", name: "shared"}:  300,
					{folderUID: "folder-y", name: "shared"}:  300,
				},
			}
			d.clients = []*shared.GrafanaClient{newClient(serverX.URL)}
			d.perConversionClients = map[string][]*shared.GrafanaClient{"conv_y": {newClient(serverY.URL)}}

			_, _, _, err = d.Deploy(ctx) //nolint:dogsled
			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.wantXGets, xGets, "rule-group requests to the default instance")
			assert.ElementsMatch(t, tc.wantYGets, yGets, "rule-group requests to the overridden instance")
		})
	}
}

func TestClientsForAlertFileOverlappingConfigNames(t *testing.T) {
	newClient := func(url string) *shared.GrafanaClient {
		return shared.NewGrafanaClient(url, "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout)
	}
	defaultClient := newClient("http://default.example/")
	awsClient := newClient("http://aws.example/")
	cloudtrailClient := newClient("http://cloudtrail.example/")

	t.Run("longer config without override is not routed to a shorter config's instance", func(t *testing.T) {
		d := &Deployer{
			clients:              []*shared.GrafanaClient{defaultClient},
			configNames:          []string{"aws", "aws_cloudtrail"},
			perConversionClients: map[string][]*shared.GrafanaClient{"aws": {awsClient}},
		}
		assert.Equal(t, []*shared.GrafanaClient{defaultClient}, d.clientsForAlertFile("alert_rule_aws_cloudtrail_rule_uid1.json"))
		assert.Equal(t, []*shared.GrafanaClient{awsClient}, d.clientsForAlertFile("alert_rule_aws_other_rule_uid2.json"))
	})

	t.Run("result does not depend on map iteration order when both configs have overrides", func(t *testing.T) {
		d := &Deployer{
			clients:     []*shared.GrafanaClient{defaultClient},
			configNames: []string{"aws", "aws_cloudtrail"},
			perConversionClients: map[string][]*shared.GrafanaClient{
				"aws":            {awsClient},
				"aws_cloudtrail": {cloudtrailClient},
			},
		}
		for range 50 {
			assert.Equal(t, []*shared.GrafanaClient{cloudtrailClient}, d.clientsForAlertFile("alert_rule_aws_cloudtrail_rule_uid1.json"))
			assert.Equal(t, []*shared.GrafanaClient{awsClient}, d.clientsForAlertFile("alert_rule_aws_other_rule_uid2.json"))
		}
	})

	t.Run("unknown config falls back to default clients", func(t *testing.T) {
		d := &Deployer{
			clients:              []*shared.GrafanaClient{defaultClient},
			configNames:          []string{"aws"},
			perConversionClients: map[string][]*shared.GrafanaClient{"aws": {awsClient}},
		}
		assert.Equal(t, []*shared.GrafanaClient{defaultClient}, d.clientsForAlertFile("alert_rule_okta_rule_uid3.json"))
	})
}

// writeTestDeployerConfig writes a config file and an empty deployment folder into the working
// directory (config paths must be local) and points the deployer at them via the environment.
func writeTestDeployerConfig(t *testing.T, content string) {
	t.Helper()
	configFile, err := os.CreateTemp(".", "test_deployer_config_*.yml")
	assert.NoError(t, err)
	t.Cleanup(func() { os.Remove(configFile.Name()) })
	_, err = configFile.WriteString(content)
	assert.NoError(t, err)
	assert.NoError(t, configFile.Close())
	t.Setenv("CONFIG_PATH", configFile.Name())
	t.Setenv("DEPLOYER_GRAFANA_SA_TOKEN", "my-test-token")
}

func TestLoadConfigRequiresResolvableGrafanaInstance(t *testing.T) {
	const noInstance = "version: 2\nfolders:\n  deployment_path: \"./deployments\"\n"
	testCases := []struct {
		name      string
		config    string
		wantError string
	}{
		{
			name: "configuration without an instance and no default",
			config: noInstance + "defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n" +
				"configurations:\n  - name: conv_a\n    integration:\n      rule_group: g\n      time_window: 5m\n",
			wantError: `configuration "conv_a" has no Grafana instance`,
		},
		{
			name:      "no configurations and no default instance",
			config:    noInstance + "defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n",
			wantError: "no Grafana instance is configured",
		},
		{
			name: "only some configurations override a missing default",
			config: noInstance + "defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n" +
				"configurations:\n" +
				"  - name: conv_a\n    deployment:\n      grafana_instance: https://a.example\n" +
				"  - name: conv_b\n",
			wantError: `configuration "conv_b" has no Grafana instance`,
		},
		{
			name: "every configuration overrides the instance, so no default is needed",
			config: noInstance + "defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n" +
				"configurations:\n" +
				"  - name: conv_a\n    deployment:\n      grafana_instance: https://a.example\n" +
				"  - name: conv_b\n    deployment:\n      grafana_instance:\n        - https://b1.example\n        - https://b2.example\n",
		},
		{
			name: "configurations inherit the default instance",
			config: noInstance + "defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n" +
				"  deployment:\n    grafana_instance: https://default.example\n" +
				"configurations:\n  - name: conv_a\n",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writeTestDeployerConfig(t, tc.config)
			err := NewDeployer().LoadConfig(context.Background())
			if tc.wantError == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestLoadConfigPerConfigTimeoutWithoutInstanceOverride(t *testing.T) {
	writeTestDeployerConfig(t, "version: 2\nfolders:\n  deployment_path: \"./deployments\"\n"+
		"defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n"+
		"  deployment:\n    grafana_instance: https://default.example\n    timeout: 10s\n"+
		"configurations:\n"+
		"  - name: slow\n    deployment:\n      timeout: 60s\n"+
		"  - name: bad_timeout\n    deployment:\n      timeout: forever\n"+
		"  - name: plain\n")

	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(context.Background()))
	d.SetClient()

	// slow gets its own client on the default instance; an invalid timeout is ignored with a warning,
	// leaving bad_timeout on the shared default clients like plain
	assert.Len(t, d.perConversionClients, 1)
	if assert.Contains(t, d.perConversionClients, "slow") {
		assert.Len(t, d.perConversionClients["slow"], 1)
		assert.Equal(t, "https://default.example/", d.perConversionClients["slow"][0].BaseURL())
		assert.NotSame(t, d.clients[0], d.perConversionClients["slow"][0])
	}
	assert.Equal(t, d.clients, d.clientsForAlertFile("alert_rule_bad_timeout_rule_uid.json"))
	assert.Equal(t, d.clients, d.clientsForAlertFile("alert_rule_plain_rule_uid.json"))
}

func TestDeployFailsWhenNoInstanceResolvesForAlertFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp(".", "test-no-instance-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	alertFile := filepath.Join(tmpDir, "alert_rule_conv_a_rule_uid1.json")
	assert.NoError(t, os.WriteFile(alertFile, []byte(`{"uid":"uid1","title":"A","folderUID":"f","orgID":1,"ruleGroup":"g"}`), 0o600))

	for name, d := range map[string]*Deployer{
		"alert to add":    {config: deploymentConfig{alertsToAdd: []string{alertFile}}},
		"alert to update": {config: deploymentConfig{alertsToUpdate: []string{alertFile}}},
		"alert to remove": {config: deploymentConfig{alertsToRemove: []string{alertFile}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := d.Deploy(context.Background()) //nolint:dogsled
			assert.ErrorContains(t, err, "no Grafana instance is configured for alert file")
		})
	}
}

// TestFreshDeployCleansEachInstanceWithItsOwnFolderOrgPairs verifies that fresh-deploy cleanup
// only matches an instance's alerts against the (folder, org) pairs deployed to that instance,
// and deletes them from that same instance rather than from the default ones.
func TestFreshDeployCleansEachInstanceWithItsOwnFolderOrgPairs(t *testing.T) {
	var deletedOnX, deletedOnY []string

	makeServer := func(alerts string, deleted *[]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			switch {
			case r.URL.Path == "/api/v1/provisioning/alert-rules" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(alerts))
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusNotFound) // the title lookup falls back to the UID
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") && r.Method == http.MethodDelete:
				*deleted = append(*deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/"))
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	// X is the default instance and hosts folder fx/org 1. Y is conv_y's override and hosts fy/org 2.
	// Each also holds an alert in the other's folder, and one in an unmanaged folder.
	serverX := makeServer(`[
		{"uid":"x-managed","title":"x","folderUID":"fx","orgID":1},
		{"uid":"x-in-ys-folder","title":"x","folderUID":"fy","orgID":2},
		{"uid":"x-unmanaged","title":"x","folderUID":"other","orgID":1}
	]`, &deletedOnX)
	defer serverX.Close()
	serverY := makeServer(`[
		{"uid":"y-managed","title":"y","folderUID":"fy","orgID":2},
		{"uid":"y-in-xs-folder","title":"y","folderUID":"fx","orgID":1}
	]`, &deletedOnY)
	defer serverY.Close()

	tmpDir, err := os.MkdirTemp(".", "test-fresh-deploy-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	writeTestDeployerConfig(t, "version: 2\nfolders:\n  deployment_path: \""+tmpDir+"\"\n"+
		"defaults:\n  integration:\n    folder_id: fx\n    org_id: 1\n"+
		"  deployment:\n    grafana_instance: "+serverX.URL+"\n"+
		"configurations:\n"+
		"  - name: conv_x\n    integration:\n      rule_group: gx\n      time_window: 5m\n"+
		"  - name: conv_y\n    integration:\n      rule_group: gy\n      time_window: 5m\n      folder_id: fy\n      org_id: 2\n"+
		"    deployment:\n      grafana_instance: "+serverY.URL+"\n")
	t.Setenv("DEPLOYER_FRESH_DEPLOY", "true")

	ctx := context.Background()
	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(ctx))
	d.SetClient()
	assert.NoError(t, d.ConfigFreshDeployment(ctx))

	_, _, deleted, err := d.Deploy(ctx)
	assert.NoError(t, err)

	assert.Equal(t, []string{"x-managed"}, deletedOnX)
	assert.Equal(t, []string{"y-managed"}, deletedOnY)
	assert.ElementsMatch(t, []AlertDeployment{
		{UID: "x-managed", Title: "x-managed", Instance: serverX.URL + "/"},
		{UID: "y-managed", Title: "y-managed", Instance: serverY.URL + "/"},
	}, deleted)
}

// TestFreshDeploySkipsInstancesNoConfigurationDeploysTo verifies a default instance that every
// configuration overrides is left alone, rather than being cleaned using other instances' pairs.
func TestFreshDeploySkipsInstancesNoConfigurationDeploysTo(t *testing.T) {
	var defaultRequests int
	serverDefault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defaultRequests++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"uid":"stale","title":"s","folderUID":"fy","orgID":1}]`))
	}))
	defer serverDefault.Close()
	serverY := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/provisioning/alert-rules" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer serverY.Close()

	tmpDir, err := os.MkdirTemp(".", "test-fresh-skip-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	writeTestDeployerConfig(t, "version: 2\nfolders:\n  deployment_path: \""+tmpDir+"\"\n"+
		"defaults:\n  integration:\n    folder_id: fy\n    org_id: 1\n"+
		"  deployment:\n    grafana_instance: "+serverDefault.URL+"\n"+
		"configurations:\n  - name: conv_y\n    deployment:\n      grafana_instance: "+serverY.URL+"\n")
	t.Setenv("DEPLOYER_FRESH_DEPLOY", "true")

	ctx := context.Background()
	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(ctx))
	d.SetClient()
	assert.NoError(t, d.ConfigFreshDeployment(ctx))
	assert.Equal(t, 0, defaultRequests, "the default instance is not deployed to, so it must not be listed")
}

func TestLoadConfigRuleGroupIntervalsAreKeyedByFolder(t *testing.T) {
	const header = "version: 2\nfolders:\n  deployment_path: \"./deployments\"\n" +
		"defaults:\n  integration:\n    folder_id: folder-a\n    org_id: 1\n" +
		"  deployment:\n    grafana_instance: https://myinstance.grafana.com\n"

	t.Run("same group name in different folders may have different intervals", func(t *testing.T) {
		writeTestDeployerConfig(t, header+
			"configurations:\n"+
			"  - name: conv_a\n    integration:\n      rule_group: shared\n      time_window: 5m\n"+
			"  - name: conv_b\n    integration:\n      rule_group: shared\n      time_window: 10m\n      folder_id: folder-b\n")

		d := NewDeployer()
		assert.NoError(t, d.LoadConfig(context.Background()))
		assert.Equal(t, map[ruleGroup]int64{
			{folderUID: "folder-a", name: "shared"}: 300,
			{folderUID: "folder-b", name: "shared"}: 600,
		}, d.config.groupsIntervals)
	})

	t.Run("same group in the same folder must have the same interval", func(t *testing.T) {
		writeTestDeployerConfig(t, header+
			"configurations:\n"+
			"  - name: conv_a\n    integration:\n      rule_group: shared\n      time_window: 5m\n"+
			"  - name: conv_b\n    integration:\n      rule_group: shared\n      time_window: 10m\n")

		d := NewDeployer()
		err := d.LoadConfig(context.Background())
		assert.ErrorContains(t, err, "time window for rule group shared in folder folder-a is different")
	})
}

func TestLoadConfigAcceptsDaysInTimeWindowAndTimeout(t *testing.T) {
	writeTestDeployerConfig(t, "version: 2\nfolders:\n  deployment_path: \"./deployments\"\n"+
		"defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n    time_window: 1d\n"+
		"  deployment:\n    grafana_instance: https://myinstance.grafana.com\n    timeout: 1d\n"+
		"configurations:\n  - name: a\n    integration:\n      rule_group: g\n  - name: b\n    integration:\n      rule_group: h\n      time_window: 1d12h\n")

	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(context.Background()))
	assert.Equal(t, 24*time.Hour, d.config.timeout)
	assert.Equal(t, map[ruleGroup]int64{
		{folderUID: "f", name: "g"}: 86400,
		{folderUID: "f", name: "h"}: 129600,
	}, d.config.groupsIntervals)
}

// TestDeployRecordsTheInstanceOfEachAlert verifies each created, updated and deleted alert
// records the instance it was deployed to, so the deployment comment can link to the right one.
func TestDeployRecordsTheInstanceOfEachAlert(t *testing.T) {
	makeServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			switch {
			case r.URL.Path == "/api/v1/provisioning/alert-rules" && r.Method == http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{}`))
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") && r.Method == http.MethodPut:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusNotFound) // the title lookup falls back to the UID
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/alert-rules/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.HasPrefix(r.URL.Path, "/api/v1/provisioning/folder/") && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK) // the interval of the group already matches
				_, _ = w.Write([]byte(`{"interval":300,"rules":[]}`))
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}
	serverX := makeServer()
	defer serverX.Close()
	serverY := makeServer()
	defer serverY.Close()

	tmpDir, err := os.MkdirTemp(".", "test-instance-per-alert-*")
	assert.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	writeAlert := func(name, uid, title string) string {
		file := filepath.Join(tmpDir, name)
		assert.NoError(t, os.WriteFile(file, []byte(`{"uid":"`+uid+`","title":"`+title+`","folderUID":"f","orgID":1,"ruleGroup":"g"}`), 0o600))
		return file
	}
	// conv_x deploys to the default instance X and conv_y to its override Y
	t.Setenv("ADDED_FILES", writeAlert("alert_rule_conv_x_rule_ux1.json", "ux1", "Created on X"))
	t.Setenv("MODIFIED_FILES", writeAlert("alert_rule_conv_y_rule_uy1.json", "uy1", "Updated on Y"))
	t.Setenv("DELETED_FILES", filepath.Join(tmpDir, "alert_rule_conv_y_rule_uy2.json"))
	t.Setenv("COPIED_FILES", "")

	writeTestDeployerConfig(t, "version: 2\nfolders:\n  deployment_path: \""+tmpDir+"\"\n"+
		"defaults:\n  integration:\n    folder_id: f\n    org_id: 1\n    rule_group: g\n    time_window: 5m\n"+
		"  deployment:\n    grafana_instance: "+serverX.URL+"\n"+
		"configurations:\n  - name: conv_x\n  - name: conv_y\n    deployment:\n      grafana_instance: "+serverY.URL+"\n")

	ctx := context.Background()
	d := NewDeployer()
	assert.NoError(t, d.LoadConfig(ctx))
	d.SetClient()
	assert.NoError(t, d.ConfigNormalMode())
	created, updated, deleted, err := d.Deploy(ctx)
	assert.NoError(t, err)

	assert.Equal(t, []AlertDeployment{{UID: "ux1", Title: "Created on X", Instance: serverX.URL + "/"}}, created)
	assert.Equal(t, []AlertDeployment{{UID: "uy1", Title: "Updated on Y", Instance: serverY.URL + "/"}}, updated)
	assert.Equal(t, []AlertDeployment{{UID: "uy2", Title: "uy2", Instance: serverY.URL + "/"}}, deleted)
}
