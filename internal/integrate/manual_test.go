package integrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/stretchr/testify/assert"
)

// manualTestDirs creates fresh relative conversion and deployment directories for
// a test. Relative paths are required because shared.ReadLocalFile rejects
// absolute paths (so t.TempDir cannot be used here).
func manualTestDirs(t *testing.T, name string) (convPath, deployPath string) {
	t.Helper()
	base := filepath.Join("testdata", "test_manual", name)
	assert.NoError(t, os.RemoveAll(base))
	convPath = filepath.Join(base, "conv")
	deployPath = filepath.Join(base, "deploy")
	assert.NoError(t, os.MkdirAll(convPath, 0o755))
	assert.NoError(t, os.MkdirAll(deployPath, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	return convPath, deployPath
}

// TestBackfillManualFlags checks that human-modified deployment files gain the
// manual annotation while their content is preserved, and that files already
// flagged are left untouched.
func TestBackfillManualFlags(t *testing.T) {
	_, deployDir := manualTestDirs(t, "backfill")

	// A deployment file a human edited but did not flag.
	unflaggedPath := filepath.Join(deployDir, "alert_rule_conv_rule_abc123.json")
	unflagged := &model.ProvisionedAlertRule{
		UID:       "abc123",
		Title:     "Hand edited title",
		RuleGroup: "Test Rules",
		Annotations: map[string]string{
			"Query": "custom query",
		},
	}
	assert.NoError(t, writeRuleToFile(unflagged, unflaggedPath, false))

	// A deployment file that is already flagged manual.
	flaggedPath := filepath.Join(deployDir, "alert_rule_conv_rule_def456.json")
	flagged := &model.ProvisionedAlertRule{
		UID:         "def456",
		Title:       "Already manual",
		Annotations: map[string]string{ManualAnnotation: TRUE},
	}
	assert.NoError(t, writeRuleToFile(flagged, flaggedPath, false))
	flaggedBefore, err := os.ReadFile(flaggedPath)
	assert.NoError(t, err)

	i := &Integrator{
		config:      model.Configuration{Folders: model.FoldersConfig{DeploymentPath: deployDir}},
		manualFiles: []string{unflaggedPath, flaggedPath},
	}
	assert.NoError(t, i.BackfillManualFlags())

	// The unflagged file gains the manual annotation while keeping its content.
	got := &model.ProvisionedAlertRule{}
	assert.NoError(t, readRuleFromFile(got, unflaggedPath))
	assert.Equal(t, TRUE, got.Annotations[ManualAnnotation])
	assert.Equal(t, "Hand edited title", got.Title)
	assert.Equal(t, "custom query", got.Annotations["Query"])

	// The already-flagged file is left byte-for-byte unchanged.
	flaggedAfter, err := os.ReadFile(flaggedPath)
	assert.NoError(t, err)
	assert.Equal(t, flaggedBefore, flaggedAfter)
}

// TestDoConversionsSkipsManualDeployment checks that a deployment file marked
// manual is not overwritten even when its source conversion changes.
func TestDoConversionsSkipsManualDeployment(t *testing.T) {
	convPath, deployPath := manualTestDirs(t, "skip_overwrite")

	config := model.Configuration{
		Folders:            model.FoldersConfig{ConversionPath: convPath, DeploymentPath: deployPath},
		ConversionDefaults: model.ConversionConfig{Target: "loki", DataSource: "test-datasource"},
		Conversions:        []model.ConversionConfig{{Name: "test_conv", RuleGroup: "Test Rules", TimeWindow: "5m"}},
		IntegratorConfig:   model.IntegrationConfig{FolderID: "test-folder", OrgID: 1},
	}

	convOutput := model.ConversionOutput{
		ConversionName: "test_conv",
		Queries:        []string{"{job=`test`} | json"},
		Rules:          []model.SigmaRule{{ID: "996f8884-9144-40e7-ac63-29090ccde9a0", Title: "Test Rule"}},
	}
	convFile := filepath.Join(convPath, "test_conv.json")
	writeConversion := func(o model.ConversionOutput) {
		b, mErr := json.Marshal(o)
		assert.NoError(t, mErr)
		assert.NoError(t, os.WriteFile(convFile, b, 0o600))
	}
	writeConversion(convOutput)

	// First pass generates the deployment file.
	i := &Integrator{config: config, addedFiles: []string{convFile}}
	assert.NoError(t, i.DoConversions())

	deployFiles, err := filepath.Glob(filepath.Join(deployPath, "alert_rule_*.json"))
	assert.NoError(t, err)
	assert.Len(t, deployFiles, 1)
	deployFile := deployFiles[0]

	// Human takes ownership: flag it manual and change the title.
	rule := &model.ProvisionedAlertRule{}
	assert.NoError(t, readRuleFromFile(rule, deployFile))
	if rule.Annotations == nil {
		rule.Annotations = map[string]string{}
	}
	rule.Annotations[ManualAnnotation] = TRUE
	rule.Title = "HAND EDITED"
	assert.NoError(t, writeRuleToFile(rule, deployFile, false))

	// Change the conversion so a real run would overwrite the rule (the UID, and
	// therefore the deployment path, is derived from rule identity, not the query).
	convOutput.Queries = []string{"{job=`changed`} | json"}
	writeConversion(convOutput)

	// Second pass must not overwrite the manual file.
	i = &Integrator{config: config, addedFiles: []string{convFile}}
	assert.NoError(t, i.DoConversions())

	got := &model.ProvisionedAlertRule{}
	assert.NoError(t, readRuleFromFile(got, deployFile))
	assert.Equal(t, "HAND EDITED", got.Title)
	assert.Equal(t, TRUE, got.Annotations[ManualAnnotation])
}

// TestDoCleanupPreservesManual checks that manual deployment files survive both
// the deleted-rule cleanup and the orphaned-file cleanup.
func TestDoCleanupPreservesManual(t *testing.T) {
	newConfig := func(convPath, deployPath string) model.Configuration {
		return model.Configuration{
			Folders:     model.FoldersConfig{ConversionPath: convPath, DeploymentPath: deployPath},
			Conversions: []model.ConversionConfig{{Name: "test_conv"}},
		}
	}

	t.Run("deleted rule keeps manual deployment file", func(t *testing.T) {
		convPath, deployPath := manualTestDirs(t, "cleanup_deleted")

		deployFile := filepath.Join(deployPath, "alert_rule_test_conv_test_123abc.json")
		rule := &model.ProvisionedAlertRule{UID: "123abc", Title: "Manual", Annotations: map[string]string{ManualAnnotation: TRUE}}
		assert.NoError(t, writeRuleToFile(rule, deployFile, false))

		i := &Integrator{config: newConfig(convPath, deployPath), removedFiles: []string{filepath.Join(convPath, "test_conv.json")}}
		assert.NoError(t, i.DoCleanup())

		_, err := os.Stat(deployFile)
		assert.NoError(t, err, "manual deployment file should be preserved")
	})

	t.Run("orphaned manual deployment file is kept", func(t *testing.T) {
		convPath, deployPath := manualTestDirs(t, "cleanup_orphan")

		orphan := filepath.Join(deployPath, "alert_rule_orphan_deploy_456def.json")
		rule := &model.ProvisionedAlertRule{
			UID:   "456def",
			Title: "Orphan Manual",
			Annotations: map[string]string{
				ManualAnnotation: TRUE,
				"ConversionFile": "/path/does/not/exist.json",
			},
		}
		assert.NoError(t, writeRuleToFile(rule, orphan, false))

		i := &Integrator{config: newConfig(convPath, deployPath)}
		assert.NoError(t, i.DoCleanup())

		_, err := os.Stat(orphan)
		assert.NoError(t, err, "orphaned manual deployment file should be preserved")
	})
}

// TestBackfillManualFlagsPreservesUnmodeledFields checks that backfilling the flag
// does not drop JSON fields the ProvisionedAlertRule struct does not model.
func TestBackfillManualFlagsPreservesUnmodeledFields(t *testing.T) {
	_, deployDir := manualTestDirs(t, "backfill_preserve")
	path := filepath.Join(deployDir, "alert_rule_conv_rule_zzz.json")
	// Raw JSON carrying a field the struct does not model, and annotations without the flag.
	raw := `{"uid":"zzz","title":"Keep me","customField":"keepme","annotations":{"Query":"q"}}`
	assert.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	i := &Integrator{
		config:      model.Configuration{Folders: model.FoldersConfig{DeploymentPath: deployDir}},
		manualFiles: []string{path},
	}
	assert.NoError(t, i.BackfillManualFlags())

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	var doc map[string]any
	assert.NoError(t, json.Unmarshal(content, &doc))

	// The unmodeled field and existing content survive; the flag is added.
	assert.Equal(t, "keepme", doc["customField"])
	assert.Equal(t, "Keep me", doc["title"])
	annotations, ok := doc["annotations"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, TRUE, annotations[ManualAnnotation])
	assert.Equal(t, "q", annotations["Query"])
}

// TestBackfillManualFlagsRespectsExplicitFalse checks that a deliberately-set
// manual value is never overwritten by the backfill.
func TestBackfillManualFlagsRespectsExplicitFalse(t *testing.T) {
	_, deployDir := manualTestDirs(t, "backfill_false")
	path := filepath.Join(deployDir, "alert_rule_conv_rule_fff.json")
	assert.NoError(t, os.WriteFile(path, []byte(`{"uid":"fff","annotations":{"manual":"false"}}`), 0o600))
	before, err := os.ReadFile(path)
	assert.NoError(t, err)

	i := &Integrator{
		config:      model.Configuration{Folders: model.FoldersConfig{DeploymentPath: deployDir}},
		manualFiles: []string{path},
	}
	assert.NoError(t, i.BackfillManualFlags())

	after, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, before, after, "explicit manual:false must not be re-flagged")
}

// TestDoConversionsRegeneratesManualFalseDeployment checks the opt-out path: a
// deployment file whose manual annotation is set to "false" is not re-flagged by
// the backfill and is regenerated (not skipped) when its conversion changes.
func TestDoConversionsRegeneratesManualFalseDeployment(t *testing.T) {
	convPath, deployPath := manualTestDirs(t, "false_regen")

	config := model.Configuration{
		Folders:            model.FoldersConfig{ConversionPath: convPath, DeploymentPath: deployPath},
		ConversionDefaults: model.ConversionConfig{Target: "loki", DataSource: "test-datasource"},
		Conversions:        []model.ConversionConfig{{Name: "test_conv", RuleGroup: "Test Rules", TimeWindow: "5m"}},
		IntegratorConfig:   model.IntegrationConfig{FolderID: "test-folder", OrgID: 1},
	}

	convOutput := model.ConversionOutput{
		ConversionName: "test_conv",
		Queries:        []string{"{job=`test`} | json"},
		Rules:          []model.SigmaRule{{ID: "996f8884-9144-40e7-ac63-29090ccde9a0", Title: "Test Rule"}},
	}
	convFile := filepath.Join(convPath, "test_conv.json")
	writeConversion := func(o model.ConversionOutput) {
		b, mErr := json.Marshal(o)
		assert.NoError(t, mErr)
		assert.NoError(t, os.WriteFile(convFile, b, 0o600))
	}
	writeConversion(convOutput)

	// First pass generates the deployment file.
	i := &Integrator{config: config, addedFiles: []string{convFile}}
	assert.NoError(t, i.DoConversions())

	deployFiles, err := filepath.Glob(filepath.Join(deployPath, "alert_rule_*.json"))
	assert.NoError(t, err)
	assert.Len(t, deployFiles, 1)
	deployFile := deployFiles[0]

	// Human hands the file back: set the annotation to "false" and leave a stale title.
	rule := &model.ProvisionedAlertRule{}
	assert.NoError(t, readRuleFromFile(rule, deployFile))
	if rule.Annotations == nil {
		rule.Annotations = map[string]string{}
	}
	rule.Annotations[ManualAnnotation] = "false"
	rule.Title = "STALE"
	assert.NoError(t, writeRuleToFile(rule, deployFile, false))

	// Change the conversion, and present the file as human-modified (as the diff would).
	convOutput.Queries = []string{"{job=`changed`} | json"}
	writeConversion(convOutput)

	i = &Integrator{config: config, addedFiles: []string{convFile}, manualFiles: []string{deployFile}}
	assert.NoError(t, i.BackfillManualFlags())
	assert.NoError(t, i.DoConversions())

	got := &model.ProvisionedAlertRule{}
	assert.NoError(t, readRuleFromFile(got, deployFile))
	// The backfill left the "false" flag alone (did not re-add "true")...
	assert.Equal(t, "false", got.Annotations[ManualAnnotation])
	// ...and the false flag did not block regeneration.
	assert.NotEqual(t, "STALE", got.Title)
}

// TestIsManualMalformedReturnsError checks that unparseable files surface an error
// (so callers can fail closed) rather than being silently treated as non-manual.
func TestIsManualMalformedReturnsError(t *testing.T) {
	_, dir := manualTestDirs(t, "is_manual_malformed")
	path := filepath.Join(dir, "broken.json")
	assert.NoError(t, os.WriteFile(path, []byte(`{"uid":"x", broken`), 0o600))

	_, err := isManual(path)
	assert.Error(t, err)
}

// TestDoCleanupKeepsUnparseableFile checks that cleanup fails closed: a file it
// cannot classify is kept, not deleted.
func TestDoCleanupKeepsUnparseableFile(t *testing.T) {
	convPath, deployPath := manualTestDirs(t, "cleanup_malformed")
	deployFile := filepath.Join(deployPath, "alert_rule_test_conv_test_999zzz.json")
	assert.NoError(t, os.WriteFile(deployFile, []byte(`{ broken json`), 0o600))

	config := model.Configuration{
		Folders:     model.FoldersConfig{ConversionPath: convPath, DeploymentPath: deployPath},
		Conversions: []model.ConversionConfig{{Name: "test_conv"}},
	}
	i := &Integrator{config: config, removedFiles: []string{filepath.Join(convPath, "test_conv.json")}}
	assert.NoError(t, i.DoCleanup())

	_, err := os.Stat(deployFile)
	assert.NoError(t, err, "unparseable file must be kept (fail closed), not deleted")
}

// TestIsManual checks that the shared manual-detection helper recognises the flag
// on both deployment files (annotation) and conversion files (top-level boolean).
func TestIsManual(t *testing.T) {
	_, dir := manualTestDirs(t, "is_manual")

	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		assert.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"manual deployment", write("manual_deploy.json", `{"uid":"x","annotations":{"manual":"true"}}`), true},
		{"false deployment", write("false_deploy.json", `{"uid":"x","annotations":{"manual":"false"}}`), false},
		{"plain deployment", write("plain_deploy.json", `{"uid":"x","annotations":{"Query":"q"}}`), false},
		{"manual conversion (bool)", write("manual_conv.json", `{"conversion_name":"c","manual":true}`), true},
		{"manual conversion (string)", write("manual_conv_str.json", `{"conversion_name":"c","manual":"true"}`), true},
		{"plain conversion", write("plain_conv.json", `{"conversion_name":"c","queries":["q"]}`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := isManual(tc.path)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
