package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	contentTypeJSON = "application/json"
	//nolint:gosec
	authToken         = "Bearer my-test-token"
	alertingApiPrefix = "/api/v1/provisioning/alert-rules"
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

	// Test cases:
	// 1. Update an alert that exists: abcd123
	// 2. Update an alert that doesn't exist: xyz123
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validRequest := false

		// Case for actually updating an alert
		if r.Method == http.MethodPut {
			if r.URL.Path == alertingApiPrefix+"/abcd123" || r.URL.Path == alertingApiPrefix+"/xyz123" {
				validRequest = true
			}
		}
		// Case for updating an alert that doesn't exist (anymore) with fallback to creation
		if r.Method == http.MethodPost {
			if r.URL.Path == alertingApiPrefix {
				validRequest = true
			}
		}

		if !validRequest {
			t.Errorf("Unexpected request %s (%s)", r.URL.Path, r.Method)
		}
		if r.Header.Get("Content-Type") != contentTypeJSON {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != authToken {
			t.Errorf("Invalid Authorization header")
		}
		defer r.Body.Close()
		// Read the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		switch r.URL.Path {
		case alertingApiPrefix + "/abcd123":
			// Simulate a successful update
			w.WriteHeader(http.StatusOK)
		case alertingApiPrefix + "/xyz123":
			// Simulate a non-existing alert
			w.WriteHeader(http.StatusNotFound)
		case alertingApiPrefix:
			// Simulate a successful creation
			w.WriteHeader(http.StatusCreated)
		default:

			w.WriteHeader(http.StatusInternalServerError)
		}

		if _, err := w.Write(body); err != nil {
			t.Errorf("failed to write response body: %v", err)
			return
		}
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
		client:         server.Client(),
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

func TestCreateAlert(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != alertingApiPrefix {
			t.Errorf("Expected to request '%s', got: %s", alertingApiPrefix, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != contentTypeJSON {
			t.Errorf("Expected Content-Typet: application/json header, got: %s", r.Header.Get("Content-Type"))
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") != authToken {
			t.Errorf("Invalid Authorization header")
		}
		defer r.Body.Close()
		// Read the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write(body); err != nil {
			t.Errorf("failed to write response body: %v", err)
			return
		}
	}))
	defer server.Close()

	d := Deployer{
		config: deploymentConfig{
			endpoint: server.URL + "/",
			saToken:  "my-test-token",
		},
		client:         server.Client(),
		groupsToUpdate: map[string]bool{},
	}

	uid, err := d.createAlert(ctx, `{"uid":"abcd123","title":"Test alert", "folderUID": "efgh456", "orgID": 23}`)
	assert.NoError(t, err)
	assert.Equal(t, "abcd123", uid)
}

func TestDeleteAlert(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, alertingApiPrefix) {
			uid := strings.TrimPrefix(r.URL.Path, alertingApiPrefix+"/")
			if uid != "abcd123" {
				t.Errorf("Expected to request '%s/abcd123', got: %s", alertingApiPrefix, r.URL.Path)
			}
		} else {
			t.Errorf("Expected to request '%s/abcd123', got: %s", alertingApiPrefix, r.URL.Path)
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
		if r.URL.Path != alertingApiPrefix {
			t.Errorf("Expected to request '%s', got: %s", alertingApiPrefix, r.URL.Path)
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
		client: server.Client(),
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
		err = d.configFreshDeployment(ctx)
	} else {
		err = d.configNormalMode()
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
		client: &http.Client{
			Timeout: defaultRequestTimeout,
		},
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
		client: &http.Client{
			Timeout: defaultRequestTimeout,
		},
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

					var updatedGroup AlertRuleGroup
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
				client: server.Client(),
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
