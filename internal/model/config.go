package model

// FoldersConfig contains folder path configuration
type FoldersConfig struct {
	ConversionPath string `yaml:"conversion_path"`
	DeploymentPath string `yaml:"deployment_path"`
}

// ConversionConfig contains conversion and some integration configuration
type ConversionConfig struct {
	Name            string   `yaml:"name"`
	Target          string   `yaml:"target"`
	Format          string   `yaml:"format"`
	SkipUnsupported string   `yaml:"skip_unsupported"`
	FilePattern     string   `yaml:"file_pattern"`
	DataSource      string   `yaml:"data_source"`
	Pipeline        []string `yaml:"pipelines"`
	RuleGroup       string   `yaml:"rule_group"`
	TimeWindow      string   `yaml:"time_window"`
	Lookback        string   `yaml:"lookback"`
	// the data source type to use for the query, if unspecified, uses the target
	DataSourceType string `yaml:"data_source_type,omitempty"`
	// Use a sprintf format string to populate a bespoke query model
	// refID, datasource, query
	QueryModel         string   `yaml:"query_model,omitempty"`
	RequiredRuleFields []string `yaml:"required_rule_fields,omitempty"`
}

// IntegrationConfig contains integration and testing configuration
type IntegrationConfig struct {
	FolderID                     string            `yaml:"folder_id"`
	OrgID                        int64             `yaml:"org_id"`
	TestQueries                  bool              `yaml:"test_queries"`
	From                         string            `yaml:"from"`
	To                           string            `yaml:"to"`
	ShowLogLines                 bool              `yaml:"show_log_lines"`
	ShowSampleValues             bool              `yaml:"show_sample_values"`
	ContinueOnQueryTestingErrors bool              `yaml:"continue_on_query_testing_errors"`
	TemplateLabels               map[string]string `yaml:"template_labels"`
	TemplateAnnotations          map[string]string `yaml:"template_annotations"`
	TemplateAllRules             bool              `yaml:"template_all_rules"`
}

// DeploymentConfig contains deployment configuration
type DeploymentConfig struct {
	GrafanaInstance string `yaml:"grafana_instance"`
	Timeout         string `yaml:"timeout"`
}

// Configuration is the original (v1) unified configuration structure
type Configuration struct {
	Folders            FoldersConfig      `yaml:"folders"`
	ConversionDefaults ConversionConfig   `yaml:"conversion_defaults"`
	Conversions        []ConversionConfig `yaml:"conversions"`
	IntegratorConfig   IntegrationConfig  `yaml:"integration"`
	DeployerConfig     DeploymentConfig   `yaml:"deployment"`
}

// ConversionConfigV2 contains only conversion configuration
type ConversionConfigV2 struct {
	// Sigma conversion settings
	Target          string   `yaml:"target"`
	Format          string   `yaml:"format"`
	SkipUnsupported string   `yaml:"skip_unsupported"`
	FilePattern     string   `yaml:"file_pattern"`
	Pipeline        []string `yaml:"pipelines"`
	// Templating settings
	// The fields to extract from the rule and store in the conversion file, so they will be available for the integration stage
	RequiredRuleFields []string `yaml:"required_rule_fields,omitempty"`
}

// IntegrationConfigV2 contains only integration configuration
type IntegrationConfigV2 struct {
	// Grafana instance settings
	FolderID string `yaml:"folder_id"`
	OrgID    int64  `yaml:"org_id"`
	// Data source settings
	// the data source type to use for the query, if unspecified, uses the conversion target
	DataSourceType string `yaml:"data_source_type,omitempty"`
	DataSource     string `yaml:"data_source"` // the UID of the data source
	// Grafana alerting settings
	RuleGroup  string `yaml:"rule_group"`
	TimeWindow string `yaml:"time_window"`
	Lookback   string `yaml:"lookback"`
	// Use a sprintf format string to populate a bespoke query model
	// refID, datasource, query
	QueryModel string `yaml:"query_model,omitempty"`
	// Query testing settings
	TestQueries      bool   `yaml:"test_queries"`
	From             string `yaml:"from"`
	To               string `yaml:"to"`
	ShowLogLines     bool   `yaml:"show_log_lines"`
	ShowSampleValues bool   `yaml:"show_sample_values"`
	ContinueOnError  bool   `yaml:"continue_on_error"`
	// Templating settings
	TemplateLabels      map[string]string `yaml:"template_labels"`
	TemplateAnnotations map[string]string `yaml:"template_annotations"`
	TemplateAllRules    bool              `yaml:"template_all_rules"`
}

type ConfigBlock struct {
	Conversion  ConversionConfigV2  `yaml:"conversion"`
	Integration IntegrationConfigV2 `yaml:"integration"`
	Deployment  DeploymentConfig    `yaml:"deployment"`
}

type NamedConfigBlock struct {
	Name string `yaml:"name"`
	ConfigBlock
}

type ConfigurationV2 struct {
	Version        int                `yaml:"version"`
	Folders        FoldersConfig      `yaml:"folders"`
	Defaults       ConfigBlock        `yaml:"defaults"`
	Configurations []NamedConfigBlock `yaml:"configurations"`
}
