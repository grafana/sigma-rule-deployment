# Sigma Rule Deployment GitHub Actions Suite

Automate the conversion, testing, and deployment of Sigma Rules to Grafana with GitHub Actions.

## Available Actions

- [Sigma Rule Converter](./actions/convert/README.md): Converts Sigma rules to target query languages using `sigma-cli`. Supports dynamic plugin installation, custom configurations, and output management, producing a JSON output format that can be used by the integrator.
- [Query Integrator](./actions/integrate/README.md): Given a folder of input query files (as produced by the converter), each file containing a list of queries and relevant metadata, convert each into a Grafana Managed Alerting alert rule, optionally testing the queries against a configured Grafana instance to validate that it works as expected.
- [Rule Deployer](./actions/deploy/README.md): Given a folder of Grafana Managed Alerting alert rules (as produced by the integrator), deploy them to the configured Grafana instance, using Alerting's provisioning API.

## Usage

We include two sample workflow configurations ([convert-integrate-sigma.yml](config/convert-integrate-sigma.yml) and [deploy.yml](config/deploy.yml)) to show how these Actions can be combined together to achieve a full end-to-end Sigma rule to deployed Grafana Alert, and a sample [configuration file](config/sigma-convert.example.yml) for the conversions.

## FAQ

### Q: What backends/data sources do you support?

These Actions can convert rules using **any** Sigma backend and produce valid alert rules for **any** data source, however, to date they have only been thoroughly tested with Loki. In particular, converting log queries into metric queries so they can be used correctly with Grafana Managed Alerting is dependent on the backend supporting that option or by modifying the generated queries.

Relevent conversion backends and data sources that can be used in Grafana include:

- [Grafana Loki](https://github.com/grafana/pySigma-backend-loki) and the [Loki data source](https://grafana.com/docs/loki/latest/)
- [Azure KQL](https://github.com/AttackIQ/pySigma-backend-kusto) and the [Azure Monitor data source](https://grafana.com/docs/grafana/latest/datasources/azure-monitor/)
- [Datadog](https://github.com/DataDog/pysigma-backend-datadog) and the [Datadog data source](https://grafana.com/grafana/plugins/grafana-datadog-datasource/)
- [Elasticsearch](https://github.com/SigmaHQ/pySigma-backend-elasticsearch) and the [Elasticsearch data source](https://grafana.com/docs/grafana/latest/datasources/elasticsearch/)
- [QRadar AQL](https://github.com/IBM/pySigma-backend-QRadar-aql) and the [IBM Security QRadar data source](https://grafana.com/grafana/plugins/ibm-aql-datasource/)
- [Opensearch](https://github.com/SigmaHQ/pySigma-backend-opensearch) and the [Opensearch data source](https://grafana.com/grafana/plugins/grafana-opensearch-datasource/)
- [Splunk](https://github.com/SigmaHQ/pySigma-backend-splunk) and the [Splunk data source](https://grafana.com/grafana/plugins/grafana-splunk-datasource/)
- [SQLite](https://github.com/SigmaHQ/pySigma-backend-sqlite) and the [SQLite data source](https://grafana.com/grafana/plugins/frser-sqlite-datasource/)
- [SurrealQL](https://github.com/SigmaHQ/pySigma-backend-surrealql) and the [SurrealDB data source](https://grafana.com/grafana/plugins/grafana-surrealdb-datasource/)

To ensure the data source plugin can execute your queries, you may need to provide a bespoke `query_model` in the conversion configuration. You do this by specifing a [fmt.Sprintf](https://pkg.go.dev/fmt#pkg-overview) formatted JSON string, which receives the following arguments:
1. the ref ID for the query
2. the UID for the data source
3. the query, escaped as a JSON string

An example query model would be:
```
query_model: '{"refId":"%s","datasource":{"type":"my_data_source_type","uid":"%s"},"query":"%s","customKey":"customValue"}'
```

Other than the `refId` and `datasource` (which are required by Grafana), the keys used for the query model are data source dependent. They can be identified by testing a query against the data source with the [Query inspector](https://grafana.com/docs/grafana/latest/explore/explore-inspector/) open, going to the Query tab, and examining the items used in the `request.data.queries` list.

### Q: Are there any restrictions on the Sigma rule files?

The main restriction are they need to be valid Sigma rules. If you are using [Correlation rules](https://github.com/SigmaHQ/sigma-specification/blob/main/specification/sigma-correlation-rules-specification.md), the rule files must contain **all** the referenced rules within the rule file (using [YAML's multiple document feature](https://gettaurus.org/docs/YAMLTutorial/#YAML-Multi-Documents), i.e., combined with `---`).
