//nolint:revive
package shared

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(".", "test_config_*.yml")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestLoadConfigFromFile_ValidV2(t *testing.T) {
	path := writeConfigFile(t, "version: 2\n")
	cfg, err := LoadConfigFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.Version)
}

func TestLoadConfigFromFile_MissingVersion(t *testing.T) {
	path := writeConfigFile(t, "folders:\n  conversion_path: conversions\n")
	_, err := LoadConfigFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only version 2 is supported")
}

func TestLoadConfigFromFile_WrongVersion(t *testing.T) {
	for _, v := range []string{"0", "1", "3"} {
		path := writeConfigFile(t, "version: "+v+"\n")
		_, err := LoadConfigFromFile(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only version 2 is supported")
	}
}

func TestLoadConfigFromFile_BooleanSettingsDistinguishUnsetFromFalse(t *testing.T) {
	path := writeConfigFile(t, `version: 2
defaults:
  integration:
    test_queries: true
configurations:
  - name: explicit_false
    integration:
      test_queries: false
      template_all_rules: false
  - name: unset
`)
	cfg, err := LoadConfigFromFile(path)
	require.NoError(t, err)
	require.Len(t, cfg.Configurations, 2)

	require.NotNil(t, cfg.Defaults.Integration.TestQueries)
	assert.True(t, *cfg.Defaults.Integration.TestQueries)

	explicit := cfg.Configurations[0].Integration
	require.NotNil(t, explicit.TestQueries)
	assert.False(t, *explicit.TestQueries)
	require.NotNil(t, explicit.TemplateAllRules)
	assert.False(t, *explicit.TemplateAllRules)

	unset := cfg.Configurations[1].Integration
	assert.Nil(t, unset.TestQueries)
	assert.Nil(t, unset.ShowLogLines)
	assert.Nil(t, unset.ContinueOnError)
}

func TestLoadConfigFromFile_SkipUnsupportedIsBoolean(t *testing.T) {
	path := writeConfigFile(t, "version: 2\ndefaults:\n  conversion:\n    skip_unsupported: true\nconfigurations:\n  - name: a\n    conversion:\n      skip_unsupported: false\n  - name: b\n")
	cfg, err := LoadConfigFromFile(path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Defaults.Conversion.SkipUnsupported)
	assert.True(t, *cfg.Defaults.Conversion.SkipUnsupported)
	require.NotNil(t, cfg.Configurations[0].Conversion.SkipUnsupported)
	assert.False(t, *cfg.Configurations[0].Conversion.SkipUnsupported)
	assert.Nil(t, cfg.Configurations[1].Conversion.SkipUnsupported)

	// A quoted string is not a boolean, matching the schema
	path = writeConfigFile(t, "version: 2\ndefaults:\n  conversion:\n    skip_unsupported: \"true\"\n")
	_, err = LoadConfigFromFile(path)
	assert.Error(t, err)
}

func TestLoadConfigFromFile_EmptyTemplateMapIsNotNil(t *testing.T) {
	// An explicitly empty map must stay distinguishable from an unset one, as it replaces the defaults' map
	path := writeConfigFile(t, "version: 2\nconfigurations:\n  - name: empty\n    integration:\n      template_labels: {}\n  - name: unset\n")
	cfg, err := LoadConfigFromFile(path)
	require.NoError(t, err)
	assert.NotNil(t, cfg.Configurations[0].Integration.TemplateLabels)
	assert.Empty(t, cfg.Configurations[0].Integration.TemplateLabels)
	assert.Nil(t, cfg.Configurations[1].Integration.TemplateLabels)
}
