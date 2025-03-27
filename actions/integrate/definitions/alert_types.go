package definitions

import (
	"encoding/json"
	"time"
)

// ProvisionedAlertRule represents a Grafana alert rule
type ProvisionedAlertRule struct {
	UID          string       `json:"uid"`
	OrgID        int64        `json:"orgId"`
	FolderUID    string       `json:"folderUid"`
	RuleGroup    string       `json:"ruleGroup"`
	Title        string       `json:"title"`
	Condition    string       `json:"condition"`
	Data         []AlertQuery `json:"data"`
	NoDataState  NoDataState  `json:"noDataState"`
	ExecErrState ExecErrState `json:"execErrState"`
	Updated      time.Time    `json:"updated"`
}

// AlertQuery represents a query in an alert rule
type AlertQuery struct {
	RefID             string            `json:"refId"`
	QueryType         string            `json:"queryType"`
	DatasourceUID     string            `json:"datasourceUid"`
	RelativeTimeRange RelativeTimeRange `json:"relativeTimeRange"`
	Model             json.RawMessage   `json:"model"`
}

// RelativeTimeRange represents a time range relative to the current time
type RelativeTimeRange struct {
	From Duration `json:"from"`
	To   Duration `json:"to"`
}

// Duration represents a time duration
type Duration time.Duration

// NoDataState represents the state when no data is available
type NoDataState string

// ExecErrState represents the state when there's an execution error
type ExecErrState string

const (
	// OK represents the OK state
	OK NoDataState = "OK"
	// OkErrState represents the OK error state
	OkErrState ExecErrState = "OK"
)
