package deploy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
		client:         shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
		groupsToUpdate: map[string]bool{},
	}

	// Update an alert
	uid, created, err := d.updateAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, false, created)
	assert.Equal(t, "abcd123", uid)

	// Try to update an alert that doesn't exist. This should lead to a creation
	uid, created, err = d.updateAlert(ctx, `{"uid":"xyz123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, true, created)
	assert.Equal(t, "xyz123", uid)
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
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
		client:         shared.NewGrafanaClient(server.URL+"/", "my-test-token", "sigma-rule-deployment/deployer", defaultRequestTimeout),
		groupsToUpdate: map[string]bool{},
	}

	// Create an alert
	uid, updated, err := d.createAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, false, updated)
	assert.Equal(t, "abcd123", uid)

	// Try to create an alert that already exists. This should lead to an update
	uid, updated, err = d.createAlert(ctx, `{"uid":"xyz123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`, true)
	assert.NoError(t, err)
	assert.Equal(t, true, updated)
	assert.Equal(t, "xyz123", uid)

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
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
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
			endpoint:  server.URL + "/",
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
	assert.Equal(t, "https://myinstance.grafana.com/", d.config.endpoint)
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
	expectedIntervals := map[string]int64{
		"group1": 600,   // 10m in seconds
		"group2": 3600,  // 1h in seconds
		"group3": 21600, // 6h (default) in seconds
	}

	assert.Equal(t, expectedIntervals, d.config.groupsIntervals)
}

func TestFakeAlertFilename(t *testing.T) {
	d := Deployer{
		config: deploymentConfig{
			alertPath: "deployments",
		},
		client: shared.NewGrafanaClient("", "", "sigma-rule-deployment/deployer", defaultRequestTimeout),
	}
	assert.Equal(t, "abcd123", getAlertUIDFromFilename(d.fakeAlertFilename("abcd123")))
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
					endpoint: server.URL + "/",
					saToken:  "my-test-token",
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
