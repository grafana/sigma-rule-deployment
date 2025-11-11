package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/prometheus/common/model"
)

// ProvisionedAlertRule represents a Grafana alert rule
type ProvisionedAlertRule struct {
	ID int64 `json:"id"`
	// required: false
	// minLength: 1
	// maxLength: 40
	// pattern: ^[a-zA-Z0-9-_]+$
	UID string `json:"uid"`
	// required: true
	OrgID int64 `json:"orgID"`
	// required: true
	// example: project_x
	FolderUID string `json:"folderUID"`
	// required: true
	// minLength: 1
	// maxLength: 190
	// example: eval_group_1
	RuleGroup string `json:"ruleGroup"`
	// required: true
	// minLength: 1
	// maxLength: 190
	// example: Always firing
	Title string `json:"title"`
	// required: true
	// example: A
	Condition string `json:"condition"`
	// required: true
	// example: [{"refId":"A","queryType":"","relativeTimeRange":{"from":0,"to":0},"datasourceUid":"__expr__","model":{"conditions":[{"evaluator":{"params":[0,0],"type":"gt"},"operator":{"type":"and"},"query":{"params":[]},"reducer":{"params":[],"type":"avg"},"type":"query"}],"datasource":{"type":"__expr__","uid":"__expr__"},"expression":"1 == 1","hide":false,"intervalMs":1000,"maxDataPoints":43200,"refId":"A","type":"math"}}]
	Data []AlertQuery `json:"data"`
	// readonly: true
	Updated time.Time `json:"updated,omitempty"`
	// required: true
	NoDataState NoDataState `json:"noDataState"`
	// required: true
	ExecErrState ExecutionErrorState `json:"execErrState"`
	// required: true
	// swagger:strfmt duration
	For model.Duration `json:"for"`
	// required: false
	// swagger:strfmt duration
	KeepFiringFor model.Duration `json:"keep_firing_for"`
	// example: {"runbook_url": "https://supercoolrunbook.com/page/13"}
	Annotations map[string]string `json:"annotations,omitempty"`
	// example: {"team": "sre-team-1"}
	Labels map[string]string `json:"labels,omitempty"`
	// readonly: true
	Provenance Provenance `json:"provenance,omitempty"`
	// example: false
	IsPaused bool `json:"isPaused"`
	// example: {"receiver":"email","group_by":["alertname","grafana_folder","cluster"],"group_wait":"30s","group_interval":"1m","repeat_interval":"4d","mute_time_intervals":["Weekends","Holidays"]}
	NotificationSettings *AlertRuleNotificationSettings `json:"notification_settings"`
	// example: {"metric":"grafana_alerts_ratio", "from":"A"} //nolint:gofumpt
	Record *Record `json:"record"`
	// example: 2
	MissingSeriesEvalsToResolve *int `json:"missingSeriesEvalsToResolve,omitempty"`
}

// Record contains mapping information for Recording Rules.
type Record struct {
	// Metric indicates a metric name to send results to.
	Metric string
	// From contains a query RefID, indicating which expression node is the output of the recording rule.
	From string
	// TargetDatasourceUID is the data source to write the result of the recording rule.
	TargetDatasourceUID string
}

type AlertRuleNotificationSettings struct {
	// Name of the receiver to send notifications to.
	// required: true
	// example: grafana-default-email
	Receiver string `json:"receiver"`

	// Optional settings

	// Override the labels by which incoming alerts are grouped together. For example, multiple alerts coming in for
	// cluster=A and alertname=LatencyHigh would be batched into a single group. To aggregate by all possible labels
	// use the special value '...' as the sole label name.
	// This effectively disables aggregation entirely, passing through all alerts as-is. This is unlikely to be what
	// you want, unless you have a very low alert volume or your upstream notification system performs its own grouping.
	// Must include 'alertname' and 'grafana_folder' if not using '...'.
	// default: ["alertname", "grafana_folder"]
	// example: ["alertname", "grafana_folder", "cluster"]
	GroupBy []string `json:"group_by,omitempty"`

	// Override how long to initially wait to send a notification for a group of alerts. Allows to wait for an
	// inhibiting alert to arrive or collect more initial alerts for the same group. (Usually ~0s to few minutes.)
	// example: 30s
	GroupWait *model.Duration `json:"group_wait,omitempty"`

	// Override how long to wait before sending a notification about new alerts that are added to a group of alerts for
	// which an initial notification has already been sent. (Usually ~5m or more.)
	// example: 5m
	GroupInterval *model.Duration `json:"group_interval,omitempty"`

	// Override how long to wait before sending a notification again if it has already been sent successfully for an
	// alert. (Usually ~3h or more).
	// Note that this parameter is implicitly bound by Alertmanager's `--data.retention` configuration flag.
	// Notifications will be resent after either repeat_interval or the data retention period have passed, whichever
	// occurs first. `repeat_interval` should not be less than `group_interval`.
	// example: 4h
	RepeatInterval *model.Duration `json:"repeat_interval,omitempty"`

	// Override the times when notifications should be muted. These must match the name of a mute time interval defined
	// in the alertmanager configuration mute_time_intervals section. When muted it will not send any notifications, but
	// otherwise acts normally.
	// example: ["maintenance"]
	MuteTimeIntervals []string `json:"mute_time_intervals,omitempty"`
}

// AlertQuery represents a single query associated with an alert definition.
type AlertQuery struct {
	// RefID is the unique identifier of the query, set by the frontend call.
	RefID string `json:"refId"`

	// QueryType is an optional identifier for the type of query.
	// It can be used to distinguish different types of queries.
	QueryType string `json:"queryType"`

	// RelativeTimeRange is the relative Start and End of the query as sent by the frontend.
	RelativeTimeRange RelativeTimeRange `json:"relativeTimeRange"`

	// Grafana data source unique identifier; it should be '__expr__' for a Server Side Expression operation.
	DatasourceUID string `json:"datasourceUid"`

	// JSON is the raw JSON query and includes the above properties as well as custom properties.
	Model json.RawMessage `json:"model"`

	modelProps map[string]any //nolint:unused
}

// RelativeTimeRange represents a time range relative to the current time
type RelativeTimeRange struct {
	From Duration `json:"from"`
	To   Duration `json:"to"`
}

// Duration represents a time duration
type Duration time.Duration

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Seconds())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value) * time.Second)
		return nil
	default:
		return fmt.Errorf("invalid duration %v", v)
	}
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).Seconds(), nil
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var v any
	if err := unmarshal(&v); err != nil {
		return err
	}
	switch value := v.(type) {
	case int:
		*d = Duration(time.Duration(value) * time.Second)
		return nil
	default:
		return fmt.Errorf("invalid duration %v", v)
	}
}

// Provenance represents the provenance of the alert rule
type Provenance string

// NoDataState represents the state when no data is available
type NoDataState string

const (
	Alerting NoDataState = "Alerting"
	NoData   NoDataState = "NoData"
	OK       NoDataState = "OK"
)

type ExecutionErrorState string

const (
	OkErrState       ExecutionErrorState = "OK"
	AlertingErrState ExecutionErrorState = "Alerting"
	ErrorErrState    ExecutionErrorState = "Error"
)
