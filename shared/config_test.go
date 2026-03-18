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
	for _, v := range []int{0, 1, 3} {
		path := writeConfigFile(t, "version: "+string(rune('0'+v))+"\n")
		_, err := LoadConfigFromFile(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only version 2 is supported")
	}
}
