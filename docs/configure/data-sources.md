# Data sources and query models

Sigma Rule Deployment converts rules with any Sigma backend and can build alert rules for any Grafana data source. This page lists the backends and data sources that work together, explains how the integrate stage builds the query for each data source, and shows how to supply your own query model when the built-in one doesn't fit.

## Supported backends and data sources

The following table lists Sigma backends and the Grafana data sources that run their queries. **Native** means the integrate stage builds the query model for you. **Custom model** means you must supply a [custom query model](#use-a-custom-query-model), because the query needs a value specific to your environment.

| Sigma backend                                                                          | Data source                                                                                                    | Query model  |
| -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ------------ |
| [Grafana Loki](https://github.com/grafana/pySigma-backend-loki)                        | [Loki](https://grafana.com/docs/loki/latest/)                                                                  | Native       |
| [Elasticsearch](https://github.com/SigmaHQ/pySigma-backend-elasticsearch)              | [Elasticsearch](https://grafana.com/docs/grafana/latest/datasources/elasticsearch/)                            | Native       |
| [Azure KQL](https://github.com/AttackIQ/pySigma-backend-kusto)                         | [Azure Data Explorer](https://grafana.com/grafana/plugins/grafana-azure-data-explorer-datasource/)             | Native       |
| [Azure KQL](https://github.com/AttackIQ/pySigma-backend-kusto)                         | [Azure Monitor](https://grafana.com/docs/grafana/latest/datasources/azure-monitor/)                            | Custom model |
| [Datadog](https://github.com/SigmaHQ/pySigma-backend-datadog)                          | [Datadog](https://grafana.com/grafana/plugins/grafana-datadog-datasource/)                                     | Native       |
| [OpenSearch](https://github.com/SigmaHQ/pySigma-backend-opensearch)                    | [OpenSearch](https://grafana.com/grafana/plugins/grafana-opensearch-datasource/)                               | Native       |
| [Splunk](https://github.com/SigmaHQ/pySigma-backend-splunk)                            | [Splunk](https://grafana.com/grafana/plugins/grafana-splunk-datasource/)                                       | Native       |
| [SQLite](https://github.com/SigmaHQ/pySigma-backend-sqlite)                            | [SQLite](https://grafana.com/grafana/plugins/frser-sqlite-datasource/)                                         | Native       |
| [SurrealQL](https://github.com/SigmaHQ/pySigma-backend-surrealql)                      | [SurrealDB](https://grafana.com/grafana/plugins/fiskaly-surrealdb-datasource/)                                 | Native       |
| [ClickHouse](https://github.com/clicksiem/pySigma-backend-clickhouse)                  | [ClickHouse](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/)                               | Native       |
| [Quickwit](https://github.com/sifex/pySigma-backend-quickwit/)                         | [Quickwit](https://grafana.com/grafana/plugins/quickwit-quickwit-datasource/)                                  | Native       |
| [Parseable](https://github.com/parseablehq/pySigma-backend-parseable)                  | [Parseable](https://grafana.com/grafana/plugins/parseable-parseable-datasource/)                               | Native       |
| [CrowdStrike LogScale](https://github.com/SigmaHQ/pySigma-backend-crowdstrike)         | [Falcon LogScale](https://grafana.com/grafana/plugins/grafana-falconlogscale-datasource/)                      | Custom model |
| [SentinelOne PowerQuery](https://github.com/7RedViolin/pySigma-backend-sentinelone-pq) | [Dataset](https://grafana.com/grafana/plugins/sentinelone-dataset-datasource/)                                 | Native       |

The Loki backend is included. Install any other backend with the `plugin_packages` input of the [convert and integrate workflow](../set-up/workflows.md#convert-and-integrate).

> [!NOTE]
> Only the Loki and Elasticsearch models are tested against live data sources. The other native models are based on each plugin's source code and documentation. If one doesn't work, [open an issue](https://github.com/grafana/sigma-rule-deployment/issues).

## Choose the query model

The integrate stage picks a built-in query model from `integration.data_source_type`. If you don't set it, it uses the conversion `target` instead. For Loki and Elasticsearch, the backend name, `loki` or `elasticsearch`, is enough. For the other native data sources, set `data_source_type` to the plugin type ID, or use a backend name from the following table as the `target`.

| Data source         | `data_source_type`                       | Also selected by `target` | What the query must return                                                                                                                                      |
| ------------------- | ---------------------------------------- | ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Azure Data Explorer | `grafana-azure-data-explorer-datasource` |                           | A time series: KQL that ends `summarize ... by bin(<time>, <step>) \| order by <time> asc`. The data source must have a default database and cluster configured. |
| Datadog             | `grafana-datadog-datasource`             | `datadog`                 | A [logs search](https://grafana.com/docs/plugins/grafana-datadog-datasource/latest/alerting/) in aggregate mode, returning the count of matching logs.            |
| OpenSearch          | `grafana-opensearch-datasource`          | `opensearch`              | A Lucene query. The model counts matching documents per date histogram bucket, using the data source's time field.                                               |
| Splunk              | `grafana-splunk-datasource`              | `splunk`                  | SPL that returns numeric values with a time field, for example, `... \| timechart span=1m count`.                                                                |
| SQLite              | `frser-sqlite-datasource`                | `sqlite`                  | SQL returning a column named `time`, in Unix seconds, and a numeric column.                                                                                      |
| SurrealDB           | `fiskaly-surrealdb-datasource`           | `surrealql`               | SurrealQL run in raw mode. The plugin doesn't document alerting, so test carefully.                                                                              |
| ClickHouse          | `grafana-clickhouse-datasource`          | `clickhouse`              | SQL returning a time series, using `$__timeFilter(<COLUMN>)`, or a single numeric value.                                                                         |
| Quickwit            | `quickwit-quickwit-datasource`           | `quickwit`                | A Lucene query. The model counts matching documents per date histogram bucket, using the index's timestamp field.                                                |
| Parseable           | `parseable-parseable-datasource`         | `parseable`               | SQL returning a single numeric row, for example, `SELECT COUNT(*) FROM "stream" WHERE ...`.                                                                      |
| Dataset             | `sentinelone-dataset-datasource`         | `sentinelone-pq`          | A PowerQuery returning a timestamp and numeric columns.                                                                                                          |

The Azure KQL backend, `kusto`, isn't in the table because its queries can run on either Azure Monitor or Azure Data Explorer. Set `data_source_type` to choose.

## Metric queries and log queries

Grafana [alert rules](https://grafana.com/docs/grafana/latest/alerting/fundamentals/alert-rules/) need metric queries, which return numbers over time. Many Sigma backends produce log queries, which return matching log lines. How the two fit together depends on the data source.

Some data sources, such as Loki, can [apply metric functions](https://grafana.com/docs/loki/latest/query/metric_queries/) to log queries, and the Loki model turns a log query into a metric query for you. Other data sources, such as Elasticsearch, can't express a metric in their query language, but their response can include metric data, such as counts. The query model for those data sources asks for that metric data. A converted Sigma correlation rule usually produces a metric query already.

If the built-in model for your data source doesn't produce a usable metric, supply a custom query model.

## Use a custom query model

A query model is the JSON object Grafana sends to a data source to run a query. Set `integration.query_model`, in `defaults` or in a configuration, to override the built-in model. The value is a [`fmt.Sprintf`](https://pkg.go.dev/fmt#pkg-overview) format string that receives three arguments, in order:

1. The ref ID of the query.
1. The UID of the data source.
1. The query, escaped as a JSON string.

The following example is a minimal model for a custom data source:

```yaml
query_model: '{"refId":"%s","datasource":{"type":"<DATA_SOURCE_TYPE>","uid":"%s"},"query":"%s","customKey":"customValue"}'
```

The following example counts Elasticsearch documents per time bucket:

```yaml
query_model: '{"refId":"%s","datasource":{"type":"elasticsearch","uid":"%s"},"query":"%s","alias":"","metrics":[{"type":"count","id":"1"}],"bucketAggs":[{"type":"date_histogram","id":"2","settings":{"interval":"auto"}}],"intervalMs":2000,"maxDataPoints":1354,"timeField":"@timestamp"}'
```

To repeat a value, use indexed verbs: `%[1]s` for the ref ID, `%[2]s` for the data source UID, and `%[3]s` for the query. Write a literal `%` as `%%`.

Grafana requires `refId` and `datasource`. The other keys depend on the data source. To find them, run a query against the data source in Grafana with the [query inspector](https://grafana.com/docs/grafana/latest/explore/explore-inspector/) open, go to the **Query** tab, and look at the entries in the `request.data.queries` list.

### Azure Monitor

Azure Monitor needs the resource ID of your Log Analytics workspace. Replace _RESOURCE_URI_ with the full ID, for example, `/subscriptions/<SUBSCRIPTION>/resourceGroups/<RESOURCE_GROUP>/providers/Microsoft.OperationalInsights/workspaces/<WORKSPACE>`. The KQL must aggregate by time, for example, `summarize count() by bin(TimeGenerated, 1m)`.

```yaml
query_model: '{"refId":"%s","datasource":{"type":"grafana-azure-monitor-datasource","uid":"%s"},"queryType":"Azure Log Analytics","azureLogAnalytics":{"query":"%s","resultFormat":"time_series","resources":["<RESOURCE_URI>"],"dashboardTime":false}}'
```

### Falcon LogScale

Falcon LogScale needs the repository or view to query. Replace _REPOSITORY_ with its name. Alert rules don't use the data source's default repository, unlike the query editor.

```yaml
query_model: '{"refId":"%s","datasource":{"type":"grafana-falconlogscale-datasource","uid":"%s"},"queryType":"LQL","formatAs":"metrics","repository":"<REPOSITORY>","lsql":"%s"}'
```

### Datadog metrics

The native Datadog model runs a logs search. If your query is a [Datadog metrics query](https://docs.datadoghq.com/metrics/), for example, `avg:system.cpu.user{*}`, use a raw query model instead:

```yaml
query_model: '{"refId":"%s","datasource":{"type":"grafana-datadog-datasource","uid":"%s"},"queryType":"raw_query","rawQuery":true,"query":"%s"}'
```

## Find a data source UID

The `data_source` setting takes the UID of the data source, not its name. To find it, open **Explore** in Grafana, select the data source, and look in the page URL for `"datasource":"<UID>"`.

## Loki backend options

The Loki backend accepts two boolean options under `conversion.backend_options`. They affect performance and results differently.

`add_line_filters` adds a line filter to each query that doesn't have one, using the longest value the rule searches for. Loki can then discard non-matching lines before it parses them. It's a performance improvement that doesn't change which results a query returns, so you can turn it on everywhere.

`case_sensitive` makes Sigma string matches case-sensitive. The default is `false`. Case-sensitive queries are faster, but they can miss logs. Some sources, such as audit logs, are consistent about case, but others aren't, and a rule might then miss events or return nothing at all. Leave it `false` if the values a rule searches for could have been entered by a person. If you turn it on, test the rule with a known example first.

```yaml
conversion:
  target: loki
  backend_options:
    add_line_filters: true
    case_sensitive: false
```

## Related pages

- [Configuration reference](configuration-reference.md)
- [Set up the workflows](../set-up/workflows.md)
