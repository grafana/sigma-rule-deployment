package model

// SigmaLogsource represents a Sigma log source
type SigmaLogsource struct {
	Category   string `json:"category"`
	Product    string `json:"product"`
	Service    string `json:"service"`
	Definition string `json:"definition"`
}

// SigmaRule represents a Sigma rule
type SigmaRule struct {
	Title   string `json:"title"`
	ID      string `json:"id"`
	Related []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"related"`
	Name           string         `json:"name"`
	Taxonomy       string         `json:"taxonomy"`
	Status         string         `json:"status"`
	Description    string         `json:"description"`
	License        string         `json:"license"`
	Author         string         `json:"author"`
	References     []string       `json:"references"`
	Date           string         `json:"date"`
	Modified       string         `json:"modified"`
	Logsource      SigmaLogsource `json:"logsource"`
	Detection      any            `json:"detection"`
	Correlation    any            `json:"correlation"`
	Fields         []string       `json:"fields"`
	FalsePositives []string       `json:"falsepositives"`
	Level          string         `json:"level"`
	Tags           []string       `json:"tags"`
	Scope          string         `json:"scope"`
	Generate       bool           `json:"generate"`
}

// ConversionOutput represents the output from a conversion process
type ConversionOutput struct {
	Queries        []string    `json:"queries"`
	ConversionName string      `json:"conversion_name"`
	InputFile      string      `json:"input_file"`
	Rules          []SigmaRule `json:"rules"`
	OutputFile     string      `json:"output_file"`
}

// Stats represents statistics from query testing
type Stats struct {
	Count  int               `json:"count"`
	Fields map[string]string `json:"fields"`
	Errors []string          `json:"errors"`
}

// QueryTestResult represents the result of testing a query
type QueryTestResult struct {
	Datasource string `json:"datasource"`
	Link       string `json:"link"`
	Stats      Stats  `json:"stats"`
}

// Frame represents a single frame from a Grafana datasource query response
type Frame struct {
	Schema struct {
		Fields []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fields"`
	} `json:"schema"`
	Data struct {
		Values [][]any `json:"values"`
	} `json:"data"`
}

// ResultFrame represents a single result frame in the query response
type ResultFrame struct {
	Frames []Frame `json:"frames"`
}

// QueryResponse represents the structure of a Grafana datasource query response
type QueryResponse struct {
	Results map[string]ResultFrame `json:"results"`
	Errors  []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
}

// Alert represents a basic alert structure (used by deployer)
type Alert struct {
	UID       string `json:"uid"`
	Title     string `json:"title"`
	FolderUID string `json:"folderUID"`
	RuleGroup string `json:"ruleGroup"`
	OrgID     int64  `json:"orgID"`
}

// AlertRuleGroup represents an alert rule group
type AlertRuleGroup struct {
	FolderUID string `json:"folderUID"`
	Interval  int64  `json:"interval"`
	Rules     any    `json:"rules"`
	Title     string `json:"title"`
}
