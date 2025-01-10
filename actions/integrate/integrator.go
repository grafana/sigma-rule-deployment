package integrate

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grafana/sigma-rule-deployment/actions/integrate/definitions"
	"github.com/spaolacci/murmur3"
	"gopkg.in/yaml.v3"
)

type Integrator struct {
	config Configuration

	addedFiles   []string
	updatedFiles []string
	removedFiles []string
}

func main() {

}

func NewIntegrator(cfg Configuration) *Integrator {
	return &Integrator{config: cfg}
}

func (i *Integrator) LoadConfig() error {
	// Load the deployment config file
	configFile := os.Getenv("INTEGRATOR_CONFIG_FILE")
	if configFile == "" {
		return fmt.Errorf("Integrator config file is not set or empty")
	}

	// Read the YAML config file
	cfg, err := ReadLocalFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}
	config := Configuration{}
	err = yaml.Unmarshal([]byte(cfg), &config)
	if err != nil {
		return fmt.Errorf("error unmarshalling config file: %v", err)
	}

	addedFiles := strings.Split(os.Getenv("ADDED_FILES"), " ")
	deletedFiles := strings.Split(os.Getenv("DELETED_FILES"), " ")
	modifiedFiles := strings.Split(os.Getenv("MODIFIED_FILES"), " ")
	// copiedFiles := strings.Split(os.Getenv("COPIED_FILES"), " ") // TODO

	newFiles := make([]string, 0, len(addedFiles))
	removedFiles := make([]string, 0, len(deletedFiles))
	updatedFiles := make([]string, 0, len(modifiedFiles))

	for _, path := range addedFiles {
		newFiles = append(newFiles, i.config.Folders.ConversionPath+string(filepath.Separator)+path)
	}
	for _, path := range deletedFiles {
		removedFiles = append(removedFiles, i.config.Folders.ConversionPath+string(filepath.Separator)+path)
	}
	for _, path := range modifiedFiles {
		updatedFiles = append(updatedFiles, i.config.Folders.ConversionPath+string(filepath.Separator)+path)
	}

	i.addedFiles = newFiles
	i.updatedFiles = updatedFiles
	i.removedFiles = removedFiles

	return nil
}

func (i *Integrator) Run() error {
	for _, inputFile := range i.addedFiles {
		raw_queries, err := ReadLocalFile(inputFile)
		if err != nil {
			return err
		}

		config := ConversionConfig{}
		for _, conf := range i.config.Conversions {
			if conf.Name+".yml" == filepath.Base(inputFile) {
				config = conf
				break
			}
		}
		if config.Name == "" {
			return fmt.Errorf("no conversion configuration found for file: %s", inputFile)
		}

		queries := strings.Split(string(raw_queries), "\n\n") // Separator taken from the Sigma source code
		if len(queries) == 0 {
			return fmt.Errorf("no queries found in file: %s", inputFile)
		}

		for _, query := range queries {
			hash := int64(murmur3.Sum32([]byte(query)))
			uid := base64.StdEncoding.EncodeToString(big.NewInt(hash).Bytes())
			outputFile := fmt.Sprintf("%s%salert_rule_%s.yml", i.config.Folders.DeploymentPath, string(filepath.Separator), uid)

			rule := &definitions.ProvisionedAlertRule{}
			if _, err := os.Stat(outputFile); err == nil {
				ruleYAML, err := ReadLocalFile(outputFile)
				if err != nil {
					return fmt.Errorf("error reading rule file %s: %v", outputFile, err)
				}
				err = yaml.Unmarshal([]byte(ruleYAML), rule)
				if err != nil {
					return fmt.Errorf("error unmarshalling rule file %s: %v", outputFile, err)
				}
			}

			datasource := getC(config.DataSource, i.config.ConversionDefaults.DataSource, "nil")
			timewindow := getC(config.TimeWindow, i.config.ConversionDefaults.TimeWindow, "1m")
			timerange, err := time.ParseDuration(timewindow)
			if err != nil {
				return fmt.Errorf("error parsing time window: %v", err)
			}

			rule.ID = hash
			rule.UID = uid
			rule.Data = []definitions.AlertQuery{
				{
					RefID:             "A",
					DatasourceUID:     datasource,
					RelativeTimeRange: definitions.RelativeTimeRange{From: definitions.Duration(timerange)}},
			}
		}
	}

	return nil
}

func getC(config, defaultConf, def string) string {
	if config != "" {
		return config
	}
	if defaultConf != "" {
		return defaultConf
	}
	return def
}
