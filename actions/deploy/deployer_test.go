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

func TestGetAlertUidFromFileName(t *testing.T) {
	assert.Equal(t, "abcd123", getAlertUidFromFilename("alert_rule_conversion_test_file_1_abcd123.json"))
	assert.Equal(t, "abcd123", getAlertUidFromFilename("alert_rule_conversion_name_test_file_2_abcd123.json"))
	assert.Equal(t, "uAaCwL1wlmA", getAlertUidFromFilename("alert_rule_conversion_test_file_3_uAaCwL1wlmA.json"))
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
		client:         server.Client(),
		groupsToUpdate: map[string]bool{},
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
	assert.Equal(t, "abcdef123", d.config.folderUid)
	assert.Equal(t, int64(23), d.config.orgId)
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
		folderUid          string
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
			folderUid:          "folder123",
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
			folderUid:          "folder123",
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
			folderUid:          "folder123",
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
			folderUid:          "folder123",
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
			folderUid:          "folder-with_special.chars",
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
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

				if r.Method == http.MethodGet {
					// Return the mocked response for GET
					w.WriteHeader(tc.getStatusCode)
					w.Write([]byte(tc.responseBody))
				} else if r.Method == http.MethodPut {
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
				} else {
					t.Errorf("Unexpected HTTP method: %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()

			// Create a deployer with mocked client and config
			d := Deployer{
				config: deploymentConfig{
					endpoint: server.URL + "/",
					saToken:  "test-token",
				},
				client: server.Client(),
			}

			// Call the function being tested
			err := d.updateAlertGroupInterval(context.Background(), tc.folderUid, tc.group, tc.interval)

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
