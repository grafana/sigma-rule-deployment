package integrate

const (
	testLokiTarget                = "loki"
	testGrafanaCloudLogsDS        = "grafanacloud-logs"
	testEventCountMetricQuery     = `sum by (event_actor) (count_over_time({name="gh-audit-logs"} | json [5m])) > 3`
	testValueCountMetricQuery     = testValueCountMetricQueryBody + ` > 3`
	testValueCountMetricQueryBody = `count without (event_repo) (sum by (event_actor, event_repo) (count_over_time({name="gh-audit-logs"} | json [5m])))`
	testWrappedLogQuery           = `{job=~".+"} | json | test="true"`
	testWrappedLogQueryExpr       = `sum(count_over_time({job=~".+"} | json | test="true"[$__auto]))`
	testGhAuditLogQuery           = "{name=`gh-audit-logs`,action=`repo.download_zip`} | json | event_action=~`(?i)^repo\\.download_zip$`"
	testGhAuditLogQueryExpr       = "sum(count_over_time({name=`gh-audit-logs`,action=`repo.download_zip`} | json | event_action=~`(?i)^repo\\.download_zip$`[$__auto]))"
)
