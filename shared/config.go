package shared

import (
	"fmt"
	"path/filepath"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"gopkg.in/yaml.v3"
)

// LoadConfigFromFile reads a YAML configuration file and unmarshals it into a Configuration struct.
// The configPath is cleaned using filepath.Clean before reading.
func LoadConfigFromFile(configPath string) (model.Configuration, error) {
	configPath = filepath.Clean(configPath)

	// Read the YAML config file
	configContent, err := ReadLocalFile(configPath)
	if err != nil {
		return model.Configuration{}, fmt.Errorf("error reading config file: %w", err)
	}

	var config model.Configuration
	if err := yaml.Unmarshal([]byte(configContent), &config); err != nil {
		return model.Configuration{}, fmt.Errorf("error unmarshalling config file: %w", err)
	}

	return config, nil
}
