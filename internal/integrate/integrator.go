//nolint:goconst
package integrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
	"github.com/spaolacci/murmur3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const TRUE = "true"

// ManualAnnotation is the annotation key that marks a deployment file as
// manually maintained. Files carrying annotations["manual"] == "true" are
// neither overwritten nor deleted by the integrator.
const ManualAnnotation = "manual"

var FuncMap = template.FuncMap{
	// Case conversion
	"toUpper": strings.ToUpper,
	"toLower": strings.ToLower,
	"title":   cases.Title(language.AmericanEnglish).String, // use as strings.Title is deprecated

	// Trimming
	"trim":       strings.Trim,
	"trimSpace":  strings.TrimSpace,
	"trimPrefix": strings.TrimPrefix,
	"trimSuffix": strings.TrimSuffix,
	"trimLeft":   strings.TrimLeft,
	"trimRight":  strings.TrimRight,

	// Prefix/Suffix checking
	"hasPrefix":   strings.HasPrefix,
	"hasSuffix":   strings.HasSuffix,
	"contains":    strings.Contains,
	"containsAny": strings.ContainsAny,

	// Replacement
	"replace":    strings.Replace,
	"replaceAll": strings.ReplaceAll,

	// Splitting and joining
	"split":       strings.Split,
	"splitAfter":  strings.SplitAfter,
	"splitAfterN": strings.SplitAfterN,
	"splitN":      strings.SplitN,
	"join":        strings.Join,
	"fields":      strings.Fields,

	// Searching
	"index":        strings.Index,
	"lastIndex":    strings.LastIndex,
	"indexAny":     strings.IndexAny,
	"lastIndexAny": strings.LastIndexAny,
	"count":        strings.Count,

	// Repeating
	"repeat": strings.Repeat,

	// Comparison
	"compare":   strings.Compare,
	"equalFold": strings.EqualFold,
}

// templateFuncs returns the functions available to label and annotation
// templates. Helpers that take the whole rule set are only registered when
// template_all_rules is enabled, so using them without it fails at parse time
// instead of producing a confusing execution error.
func templateFuncs(templateAllRules bool) template.FuncMap {
	funcs := maps.Clone(FuncMap)
	if templateAllRules {
		funcs["highestLevel"] = highestLevel
	}

	return funcs
}

func highestLevel(rules []model.SigmaRule) string {
	highest := ""
	highestPriority := -1

	for _, rule := range rules {
		level := strings.ToLower(strings.TrimSpace(rule.Level))
		priority := sigmaLevelPriority(level)
		if priority > highestPriority {
			highest = level
			highestPriority = priority
		}
	}

	return highest
}

func sigmaLevelPriority(level string) int {
	switch level {
	case "informational":
		return 0
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return -1
	}
}

type Integrator struct {
	config      model.Configuration
	prettyPrint bool

	allRules     bool
	addedFiles   []string
	removedFiles []string
	testFiles    []string
	// manualFiles are deployment files a human modified since the last automation
	// commit. Any that lack the manual annotation are flagged before integration so
	// the change is preserved on this and every future run.
	manualFiles []string
}

func NewIntegrator() *Integrator {
	return &Integrator{}
}

func (i *Integrator) LoadConfig() error {
	// Load the deployment config file
	configFile := os.Getenv("INTEGRATOR_CONFIG_PATH")
	if configFile == "" {
		return fmt.Errorf("Integrator config file is not set or empty")
	}
	fmt.Printf("Loading config from %s\n", configFile)

	// Read and parse the YAML config file
	config, err := shared.LoadConfigFromFile(configFile)
	if err != nil {
		return err
	}
	i.config = config
	i.prettyPrint = strings.ToLower(os.Getenv("PRETTY_PRINT")) == TRUE
	i.allRules = strings.ToLower(os.Getenv("ALL_RULES")) == TRUE

	i.config.IntegratorConfig.ContinueOnQueryTestingErrors = strings.ToLower(os.Getenv("CONTINUE_ON_QUERY_TESTING_ERRORS")) == TRUE

	if !filepath.IsLocal(i.config.Folders.ConversionPath) {
		return fmt.Errorf("conversion path is not local: %s", i.config.Folders.ConversionPath)
	}
	if !filepath.IsLocal(i.config.Folders.DeploymentPath) {
		return fmt.Errorf("deployment path is not local: %s", i.config.Folders.DeploymentPath)
	}

	fmt.Printf("Conversion path: %s\nDeployment path: %s\n", i.config.Folders.ConversionPath, i.config.Folders.DeploymentPath)

	if _, err = os.Stat(i.config.Folders.DeploymentPath); err != nil {
		err = os.MkdirAll(i.config.Folders.DeploymentPath, 0o755)
		if err != nil {
			return fmt.Errorf("error creating deployment directory: %v", err)
		}
	}

	// If from and to are not provided, use the default values
	// to query for the last hour.
	if i.config.IntegratorConfig.From == "" {
		i.config.IntegratorConfig.From = "now-1h"
	}
	if i.config.IntegratorConfig.To == "" {
		i.config.IntegratorConfig.To = "now"
	}

	changedFiles := strings.Split(os.Getenv("CHANGED_FILES"), " ")
	deletedFiles := strings.Split(os.Getenv("DELETED_FILES"), " ")
	testFiles := strings.Split(os.Getenv("TEST_FILES"), " ")
	// Deployment files a human modified since the last automation commit. These are
	// candidates for backfilling the manual annotation before integration runs.
	manualFiles := strings.Split(os.Getenv("MANUAL_FILES"), " ")

	newUpdatedFiles := []string{}
	filesToBeTested := []string{}
	if i.allRules {
		if err = filepath.Walk(i.config.Folders.ConversionPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("failed to walk directory: %w", err)
			}
			if !info.IsDir() {
				newUpdatedFiles = append(newUpdatedFiles, path)
				// If all files is true, test all files
				if i.config.IntegratorConfig.TestQueries {
					filesToBeTested = append(filesToBeTested, path)
				}
			}

			return nil
		}); err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		if newUpdatedFiles, err = filterFilesInDir(changedFiles, i.config.Folders.ConversionPath); err != nil {
			return err
		}
		if i.config.IntegratorConfig.TestQueries {
			if filesToBeTested, err = filterFilesInDir(testFiles, i.config.Folders.ConversionPath); err != nil {
				return err
			}
		}
	}

	removedFiles, err := filterFilesInDir(deletedFiles, i.config.Folders.ConversionPath)
	if err != nil {
		return err
	}
	humanModifiedFiles, err := filterFilesInDir(manualFiles, i.config.Folders.DeploymentPath)
	if err != nil {
		return err
	}

	fmt.Printf("Changed files: %d\nRemoved files: %d\nTest files: %d\nManual files: %d\n", len(newUpdatedFiles), len(removedFiles), len(filesToBeTested), len(humanModifiedFiles))
	i.addedFiles = newUpdatedFiles
	i.removedFiles = removedFiles
	i.testFiles = filesToBeTested
	i.manualFiles = humanModifiedFiles

	return nil
}

// filterFilesInDir keeps only the paths that sit directly inside dir, matching a
// diff-derived file list to a known output directory. Empty entries (e.g. from
// splitting an unset env var) are skipped.
func filterFilesInDir(paths []string, dir string) ([]string, error) {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		relpath, err := filepath.Rel(dir, path)
		if err != nil {
			return nil, fmt.Errorf("error checking file path %s: %v", path, err)
		}
		if relpath == filepath.Base(path) {
			filtered = append(filtered, path)
		}
	}
	return filtered, nil
}

// cleanupOrphanedFilesInPath removes orphaned files in the specified path
func (i *Integrator) cleanupOrphanedFilesInPath(searchPath string, isOrphaned func(string) (bool, error)) error {
	// Get all JSON files in the path
	var files []string
	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory %s: %w", searchPath, err)
	}

	// Check each file for orphaned status
	for _, file := range files {
		orphaned, err := isOrphaned(file)
		if err != nil {
			fmt.Printf("Warning: Could not check file %s: %v\n", file, err)
			continue
		}

		if orphaned {
			if keepAsManual(file, "orphaned") {
				continue
			}
			fmt.Printf("Removing orphaned file: %s\n", file)
			if err := os.Remove(file); err != nil {
				fmt.Printf("Warning: Could not remove orphaned file %s: %v\n", file, err)
			}
		}
	}

	return nil
}

// isConversionFileOrphaned checks if a conversion file has no matching configuration
func (i *Integrator) isConversionFileOrphaned(file string) (bool, error) {
	content, err := shared.ReadLocalFile(file)
	if err != nil {
		return false, err
	}

	var conversionObject model.ConversionOutput
	if err := json.Unmarshal([]byte(content), &conversionObject); err != nil {
		return false, err
	}

	// Check if this conversion name has a matching configuration
	for _, conf := range i.config.Conversions {
		if conf.Name == conversionObject.ConversionName {
			return false, nil
		}
	}

	return true, nil
}

// isDeploymentFileOrphaned checks if a deployment file references a missing conversion file
func (i *Integrator) isDeploymentFileOrphaned(file string) (bool, error) {
	content, err := shared.ReadLocalFile(file)
	if err != nil {
		return false, err
	}

	var deploymentRule model.ProvisionedAlertRule
	if err := json.Unmarshal([]byte(content), &deploymentRule); err != nil {
		return false, err
	}

	// Check if the referenced conversion file still exists
	if conversionFile := deploymentRule.Annotations["ConversionFile"]; conversionFile != "" {
		if _, err := os.Stat(conversionFile); os.IsNotExist(err) {
			return true, nil
		}
	}

	return false, nil
}

// manualValueSet reports whether a decoded JSON value marks a file as manual.
// It accepts both the boolean `true` used by conversion files and the string
// "true" used by deployment annotations, so the converter (Python) and the
// integrator agree on what counts as manual.
func manualValueSet(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == TRUE
	default:
		return false
	}
}

// isManual reports whether a generated file is marked as manually maintained.
// Deployment files carry the marker as annotations["manual"] == "true"; conversion
// files carry a top-level "manual" boolean. A single helper covers both output
// directories so cleanup never destroys a file a human has taken ownership of.
//
// The file is decoded as a generic JSON document so a type mismatch on an
// unrelated field never masks the flag, and so an unparseable file surfaces as an
// error (callers treat that as "keep the file", never as "safe to delete").
func isManual(file string) (bool, error) {
	content, err := shared.ReadLocalFile(file)
	if err != nil {
		return false, err
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return false, fmt.Errorf("could not parse %s as JSON: %w", file, err)
	}

	// Deployment file: manual annotation.
	if annotations, ok := doc["annotations"].(map[string]any); ok {
		if manualValueSet(annotations[ManualAnnotation]) {
			return true, nil
		}
	}

	// Conversion file: top-level manual flag.
	if manualValueSet(doc[ManualAnnotation]) {
		return true, nil
	}

	return false, nil
}

// keepAsManual reports whether a file slated for deletion must be preserved. It
// fails closed: if the manual flag cannot be determined (unreadable/unparseable
// file), the file is kept rather than deleted. kind labels the file in the log.
func keepAsManual(file, kind string) bool {
	manual, err := isManual(file)
	if err != nil {
		fmt.Printf("Warning: could not check manual flag for %s, keeping it: %v\n", file, err)
		return true
	}
	if manual {
		fmt.Printf("Keeping manually-maintained %s file (not deleting): %s\n", kind, file)
		return true
	}
	return false
}

// BackfillManualFlags adds the manual annotation to any human-modified deployment
// files that do not already carry it. Running before DoConversions guarantees the
// freshly-added flag is honoured (and the file preserved) on this same run.
//
// The file is edited as a generic JSON document rather than round-tripped through
// ProvisionedAlertRule, so fields the struct does not model are preserved. Any file
// that cannot be read, parsed, or written is logged and skipped — one bad file must
// not abort the whole integration.
func (i *Integrator) BackfillManualFlags() error {
	for _, file := range i.manualFiles {
		content, err := shared.ReadLocalFile(file)
		if err != nil {
			fmt.Printf("Warning: could not read %s for manual backfill, leaving unchanged: %v\n", file, err)
			continue
		}

		var doc map[string]any
		if err := json.Unmarshal([]byte(content), &doc); err != nil {
			fmt.Printf("Warning: could not parse %s for manual backfill, leaving unchanged: %v\n", file, err)
			continue
		}

		annotations, _ := doc["annotations"].(map[string]any)
		// A manual key that is already present (any value) reflects a deliberate
		// human choice; do not overwrite it.
		if annotations != nil {
			if _, present := annotations[ManualAnnotation]; present {
				continue
			}
		} else {
			annotations = map[string]any{}
			doc["annotations"] = annotations
		}
		annotations[ManualAnnotation] = TRUE

		out, err := marshalJSON(doc, i.prettyPrint)
		if err != nil {
			fmt.Printf("Warning: could not marshal manual backfill for %s, leaving unchanged: %v\n", file, err)
			continue
		}

		fmt.Printf("Marking manually-modified deployment file as manual: %s\n", file)
		if err := os.WriteFile(file, out, 0o600); err != nil {
			fmt.Printf("Warning: could not write manual backfill for %s, leaving unchanged: %v\n", file, err)
			continue
		}
	}
	return nil
}

func (i *Integrator) Run() error {
	// Preserve any deployment files a human modified by flagging them as manual
	// before we integrate, so their changes are not overwritten on this run.
	if err := i.BackfillManualFlags(); err != nil {
		return err
	}

	// Convert all files that have been updated from the last commit
	if err := i.DoConversions(); err != nil {
		return err
	}

	// Clean up any deleted files
	if err := i.DoCleanup(); err != nil {
		return err
	}

	// Write the output of rules integrated (updated and removed) to the GitHub Action outputs
	return i.SetOutputs()
}

// DoConversions handles the conversion of Sigma rules to Grafana alert rules
func (i *Integrator) DoConversions() error {
	for _, inputFile := range i.addedFiles {
		fmt.Printf("Integrating file: %s\n", inputFile)
		conversionContent, err := shared.ReadLocalFile(inputFile)
		if err != nil {
			return err
		}

		var conversionObject model.ConversionOutput
		err = json.Unmarshal([]byte(conversionContent), &conversionObject)
		if err != nil {
			return fmt.Errorf("error unmarshalling conversion output: %v", err)
		}

		// Find matching configuration using ConversionName
		var config model.ConversionConfig
		for _, conf := range i.config.Conversions {
			if conf.Name == conversionObject.ConversionName {
				config = conf
				break
			}
		}
		if config.Name == "" {
			fmt.Printf("Warning: No configuration found for conversion name: %s, skipping file: %s\n", conversionObject.ConversionName, inputFile)
			continue
		}

		queries := conversionObject.Queries
		if len(queries) == 0 {
			fmt.Printf("no queries found in conversion object")
			continue
		}

		conversionID, titles, err := summariseSigmaRules(conversionObject.Rules)
		if err != nil {
			return fmt.Errorf("error summarising sigma rules: %v", err)
		}

		// Extract rule filename from input file name
		ruleFilename := strings.TrimSuffix(filepath.Base(inputFile), ".json")
		ruleFilename = strings.TrimPrefix(ruleFilename, config.Name+"_")
		ruleUID := getRuleUID(conversionObject.ConversionName, conversionID)
		file := fmt.Sprintf("%s%salert_rule_%s_%s_%s.json", i.config.Folders.DeploymentPath, string(filepath.Separator), config.Name, ruleFilename, ruleUID)
		fmt.Printf("Working on alert rule file: %s\n", file)
		rule := &model.ProvisionedAlertRule{UID: ruleUID}

		err = readRuleFromFile(rule, file)
		if err != nil {
			return err
		}
		if rule.Annotations[ManualAnnotation] == TRUE {
			fmt.Printf("Skipping manually-maintained deployment file (not overwriting): %s\n", file)
			continue
		}
		err = i.ConvertToAlert(rule, queries, titles, config, inputFile, conversionObject)
		if err != nil {
			return err
		}
		err = writeRuleToFile(rule, file, i.prettyPrint)
		if err != nil {
			return err
		}
	}
	return nil
}

// DoCleanup handles the removal of deleted files and cleanup of orphaned files
func (i *Integrator) DoCleanup() error {
	for _, deletedFile := range i.removedFiles {
		fmt.Printf("Deleting alert rule file: %s\n", deletedFile)
		deploymentGlob := fmt.Sprintf("alert_rule_%s_*.json", strings.TrimSuffix(filepath.Base(deletedFile), ".json"))
		deploymentFiles, err := fs.Glob(os.DirFS(i.config.Folders.DeploymentPath), deploymentGlob)
		if err != nil {
			return fmt.Errorf("error when searching for deployment files for %s: %v", deletedFile, err)
		}
		for _, file := range deploymentFiles {
			fullPath := i.config.Folders.DeploymentPath + string(filepath.Separator) + file
			if keepAsManual(fullPath, "deployment") {
				continue
			}
			err = os.Remove(fullPath)
			if err != nil {
				return fmt.Errorf("error when deleting deployment file %s: %v", file, err)
			}
		}
	}

	// Clean up orphaned conversion files
	if err := i.cleanupOrphanedFilesInPath(i.config.Folders.ConversionPath, i.isConversionFileOrphaned); err != nil {
		fmt.Printf("Warning: Error during orphaned conversion file cleanup: %v\n", err)
	}

	// Clean up orphaned deployment files
	if err := i.cleanupOrphanedFilesInPath(i.config.Folders.DeploymentPath, i.isDeploymentFileOrphaned); err != nil {
		fmt.Printf("Warning: Error during orphaned deployment file cleanup: %v\n", err)
	}

	return nil
}

// Config returns the configuration
func (i *Integrator) Config() model.Configuration {
	return i.config
}

// TestFiles returns the list of test files
func (i *Integrator) TestFiles() []string {
	return i.testFiles
}

// SetOutputs writes the output of rules integrated (updated and removed) to the GitHub Action outputs
func (i *Integrator) SetOutputs() error {
	i.addedFiles = append(i.addedFiles, i.removedFiles...)
	rulesIntegrated := strings.Join(i.addedFiles, " ")

	if err := shared.SetOutput("rules_integrated", rulesIntegrated); err != nil {
		return fmt.Errorf("failed to set rules integrated output: %w", err)
	}
	return nil
}

func (i *Integrator) ConvertToAlert(rule *model.ProvisionedAlertRule, queries []string, titles string, config model.ConversionConfig, conversionFile string, conversionObject model.ConversionOutput) error {
	datasource := shared.GetConfigValue(config.DataSource, i.config.ConversionDefaults.DataSource, "nil")
	timewindow := shared.GetConfigValue(config.TimeWindow, i.config.ConversionDefaults.TimeWindow, "1m")
	duration, err := time.ParseDuration(timewindow)
	if err != nil {
		return fmt.Errorf("error parsing time window: %v", err)
	}

	lookback := shared.GetConfigValue(config.Lookback, i.config.ConversionDefaults.Lookback, "0s")
	lookbackDuration, err := time.ParseDuration(lookback)
	if err != nil {
		return fmt.Errorf("error parsing lookback: %v", err)
	}

	// Apply lookback to time range: now-5m to now with 1m lookback becomes now-6m to now-1m
	fromDuration := duration + lookbackDuration
	toDuration := lookbackDuration
	timerange := model.RelativeTimeRange{From: model.Duration(fromDuration), To: model.Duration(toDuration)}

	queryData := make([]model.AlertQuery, 0, len(queries)+2)
	refIDs := make([]string, len(queries))
	for index, query := range queries {
		refIDs[index] = fmt.Sprintf("A%d", index)
		alertQuery, err := createAlertQuery(query, refIDs[index], datasource, timerange, config, i.config.ConversionDefaults)
		if err != nil {
			return err
		}
		queryData = append(queryData, alertQuery)
	}
	// Use Math expression to combine queries: ${A0}+${A1}+...
	// For single query: ${A0}
	// For multiple queries: ${A0}+${A1}+${A2}
	mathExpression := make([]string, len(refIDs))
	for i, refID := range refIDs {
		mathExpression[i] = fmt.Sprintf("${%s}", refID)
	}
	combiner := json.RawMessage(
		fmt.Sprintf(`{"refId":"B","hide":false,"type":"math","datasource":{"uid":"__expr__","type":"__expr__"},"expression":"%s"}`,
			strings.Join(mathExpression, "+")))
	threshold := json.RawMessage(`{"refId":"C","hide":false,"type":"threshold","datasource":{"uid":"__expr__","type":"__expr__"},"conditions":[{"type":"query","evaluator":{"params":[0],"type":"gt"},"operator":{"type":"and"},"query":{"params":["C"]},"reducer":{"params":[],"type":"last"}}],"expression":"B"}`)

	queryData = append(queryData,
		model.AlertQuery{
			RefID:             "B",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             combiner,
		},
		model.AlertQuery{
			RefID:             "C",
			DatasourceUID:     "__expr__",
			RelativeTimeRange: timerange,
			QueryType:         "",
			Model:             threshold,
		},
	)

	if len(queryData) == len(rule.Data) {
		for qIdx, query := range queryData {
			if !bytes.Equal(query.Model, rule.Data[qIdx].Model) {
				break
			}
			if qIdx == len(queryData)-1 {
				// if we get here, all the queries are the same, no need to update the rule
				fmt.Printf("No changes to the relevant alert rule, skipping\n")
				return nil
			}
		}
	}
	rule.Data = queryData

	// alerting rule metadata
	rule.OrgID = i.config.IntegratorConfig.OrgID
	rule.FolderUID = i.config.IntegratorConfig.FolderID
	rule.RuleGroup = shared.GetConfigValue(config.RuleGroup, i.config.ConversionDefaults.RuleGroup, "Default")
	rule.NoDataState = model.OK
	rule.ExecErrState = model.OkErrState
	rule.Title = titles
	rule.Condition = "C"

	// Add annotations for context
	if rule.Annotations == nil {
		rule.Annotations = make(map[string]string)
	}

	rule.Annotations["Query"] = queries[0]
	rule.Annotations["TimeWindow"] = timewindow
	rule.Annotations["Lookback"] = lookback

	// LogSourceUid annotation (data source)
	rule.Annotations["LogSourceUid"] = datasource

	// LogSourceType annotation (target)
	logSourceType := shared.GetConfigValue(config.Target, i.config.ConversionDefaults.Target, shared.Loki)
	rule.Annotations["LogSourceType"] = logSourceType

	// Path to associated conversion file
	rule.Annotations["ConversionFile"] = conversionFile

	funcs := templateFuncs(i.config.IntegratorConfig.TemplateAllRules)

	if i.config.IntegratorConfig.TemplateAnnotations != nil {
		for key, value := range i.config.IntegratorConfig.TemplateAnnotations {
			tmpl, err := template.New("annotation_" + key).Funcs(funcs).Parse(value)
			if err != nil {
				return fmt.Errorf("error parsing template %s: %v", key, err)
			}
			var buf bytes.Buffer
			if i.config.IntegratorConfig.TemplateAllRules {
				err = tmpl.Execute(&buf, conversionObject.Rules)
			} else {
				err = tmpl.Execute(&buf, conversionObject.Rules[0])
			}
			if err != nil {
				return fmt.Errorf("error executing template %s: %v", key, err)
			}
			rule.Annotations[key] = buf.String()
		}
	}

	if rule.Labels == nil {
		rule.Labels = make(map[string]string)
	}

	if i.config.IntegratorConfig.TemplateLabels != nil {
		for key, value := range i.config.IntegratorConfig.TemplateLabels {
			tmpl, err := template.New("label_" + key).Funcs(funcs).Parse(value)
			if err != nil {
				return fmt.Errorf("error parsing template %s: %v", key, err)
			}
			var buf bytes.Buffer
			if i.config.IntegratorConfig.TemplateAllRules {
				err = tmpl.Execute(&buf, conversionObject.Rules)
			} else {
				err = tmpl.Execute(&buf, conversionObject.Rules[0])
			}
			if err != nil {
				return fmt.Errorf("error executing template %s: %v", key, err)
			}
			rule.Labels[key] = buf.String()
		}
	}

	return nil
}

func readRuleFromFile(rule *model.ProvisionedAlertRule, inputPath string) error {
	if _, err := os.Stat(inputPath); err == nil {
		ruleJSON, err := shared.ReadLocalFile(inputPath)
		if err != nil {
			return fmt.Errorf("error reading rule file %s: %v", inputPath, err)
		}
		err = json.Unmarshal([]byte(ruleJSON), rule)
		if err != nil {
			return fmt.Errorf("error unmarshalling rule file %s: %v", inputPath, err)
		}
	}
	return nil
}

// marshalJSON serialises v as JSON, honouring the pretty-print flag used across the
// integrator's file writes so output formatting stays consistent in one place.
func marshalJSON(v any, prettyPrint bool) ([]byte, error) {
	if prettyPrint {
		return json.MarshalIndent(v, "", "  ")
	}
	return json.Marshal(v)
}

func writeRuleToFile(rule *model.ProvisionedAlertRule, outputFile string, prettyPrint bool) error {
	ruleBytes, err := marshalJSON(rule, prettyPrint)
	if err != nil {
		return fmt.Errorf("error marshalling alert rule: %v", err)
	}

	// write to output file
	out, err := os.Create(outputFile) // will truncate existing file content
	if err != nil {
		return fmt.Errorf("error opening alert rule file %s to write to: %v", outputFile, err)
	}
	defer out.Close()
	_, err = out.Write(ruleBytes)
	if err != nil {
		return fmt.Errorf("error writing alert rule file to %s: %v", outputFile, err)
	}

	return nil
}

func summariseSigmaRules(rules []model.SigmaRule) (id uuid.UUID, title string, err error) {
	if len(rules) == 0 {
		return uuid.Nil, "", fmt.Errorf("no rules provided")
	}
	conversionIDBytes := make([]byte, 16)
	titles := make([]string, len(rules))
	for ruleIndex, rule := range rules {
		titles[ruleIndex] = rule.Title
		if ruleID, err := uuid.Parse(rule.ID); err == nil {
			if ruleIndex > 0 {
				// xor the rule IDs together to get a unique conversion ID
				for i, b := range ruleID {
					conversionIDBytes[i] ^= b
				}
			} else {
				conversionIDBytes = ruleID[:]
			}
		} else {
			return uuid.Nil, "", fmt.Errorf("error parsing rule ID %s: %v", rule.ID, err)
		}
	}
	// Ensure the final conversion ID is version 4 and variant 10
	conversionIDBytes[6] = (conversionIDBytes[6] & 0x0f) | 0x40
	conversionIDBytes[8] = (conversionIDBytes[8] & 0x3f) | 0x80
	conversionID, err := uuid.FromBytes(conversionIDBytes)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("error creating conversion ID from bytes %s: %v", conversionIDBytes, err)
	}
	title = strings.Join(titles, " & ")
	if len(title) > 190 {
		title = title[:190]
	}
	return conversionID, title, nil
}

func getRuleUID(conversionName string, conversionID uuid.UUID) string {
	hash := int64(murmur3.Sum32([]byte(conversionName + "_" + conversionID.String())))
	return fmt.Sprintf("%x", hash)
}

var lokiMetricQueryPrefixes = []string{"sum", "count", "avg", "min", "max"}

func isLokiMetricQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	for _, prefix := range lokiMetricQueryPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// createAlertQuery creates an AlertQuery based on the target data source and configuration
func createAlertQuery(query string, refID string, datasource string, timerange model.RelativeTimeRange, config model.ConversionConfig, defaultConf model.ConversionConfig) (model.AlertQuery, error) {
	datasourceType := shared.GetConfigValue(config.DataSourceType, defaultConf.DataSourceType, shared.GetConfigValue(config.Target, defaultConf.Target, shared.Loki))
	customModel := shared.GetConfigValue(config.QueryModel, defaultConf.QueryModel, "")

	if datasourceType == shared.Loki {
		if !isLokiMetricQuery(query) {
			query = fmt.Sprintf("sum(count_over_time(%s[$__auto]))", query)
		}
	}

	// Must manually escape the query as JSON to include it in a json.RawMessage
	escapedQuery, err := shared.EscapeQueryJSON(query)
	if err != nil {
		return model.AlertQuery{}, fmt.Errorf("could not escape provided query: %s", query)
	}

	// Create generic alert query
	alertQuery := model.AlertQuery{
		RefID:             refID,
		DatasourceUID:     datasource,
		RelativeTimeRange: timerange,
	}

	// Populate the alert query model, first see if the user has provided a custom model
	// else use defaults based on the target data source type
	switch {
	case customModel != "":
		alertQuery.Model = json.RawMessage(fmt.Sprintf(customModel, refID, datasource, escapedQuery))
	case datasourceType == shared.Loki:
		alertQuery.QueryType = "instant"
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"loki","uid":"%s"},"hide":false,"expr":"%s","queryType":"instant","editorMode":"code"}`, refID, datasource, escapedQuery))
	case datasourceType == shared.Elasticsearch:
		// Based on the Elasticsearch data source plugin
		// https://github.com/grafana/grafana/blob/main/public/app/plugins/datasource/elasticsearch/dataquery.gen.ts
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"elasticsearch","uid":"%s"},"query":"%s","alias":"","metrics":[{"type":"%s","id":"1"}],"bucketAggs":[{"type":"date_histogram","id":"2","settings":{"interval":"auto"}}],"intervalMs":2000,"maxDataPoints":1354,"timeField":"@timestamp"}`, refID, datasource, escapedQuery, elasticsearchMetricTypeCount))
	default:
		// try a basic query
		fmt.Printf("WARNING: Using generic query model for the data source type %s; if these queries don't work, try configuring a custom query_model\n", datasourceType)
		alertQuery.Model = json.RawMessage(fmt.Sprintf(`{"refId":"%s","datasource":{"type":"%s","uid":"%s"},"query":"%s"}`, refID, datasourceType, datasource, escapedQuery))
	}

	return alertQuery, nil
}
