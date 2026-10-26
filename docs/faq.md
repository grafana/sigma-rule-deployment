# Frequently asked questions

This page answers common questions about Sigma Rule Deployment and links to the pages with the details.

## Which backends and data sources are supported?

Any Sigma backend and any Grafana data source can work, but only Loki and Elasticsearch are tested against live data sources. Several more data sources have a built-in query model, and the rest need a custom one. Refer to [Data sources and query models](configure/data-sources.md).

## Why do my alert rules need metric queries?

Grafana alert rules evaluate numbers, so they need metric queries, but many Sigma backends produce log queries. Refer to [Metric queries and log queries](configure/data-sources.md#metric-queries-and-log-queries).

## How do I use a custom query model?

Set `integration.query_model` to a `fmt.Sprintf` format string. Refer to [Use a custom query model](configure/data-sources.md#use-a-custom-query-model).

## What goes in the `data_source` setting?

The UID of the data source, not its name. Refer to [Find a data source UID](configure/data-sources.md#find-a-data-source-uid).

## Are there restrictions on Sigma rule files?

Rules must be valid Sigma with an `id` and a `title`, and correlation rules must be in the same file as the rules they reference. Refer to [Rule requirements](set-up/rules-and-pipelines.md#rule-requirements).

## What do the Loki `add_line_filters` and `case_sensitive` options do?

`add_line_filters` speeds up queries without changing their results, and `case_sensitive` makes matches case-sensitive, which can miss logs. Refer to [Loki backend options](configure/data-sources.md#loki-backend-options).

## Can I edit a generated file by hand?

Yes. Mark it as manual so the Actions don't overwrite it. Refer to [Preserve manual modifications](use/manual-modifications.md).

## How do I deploy to more than one Grafana instance?

Set `grafana_instance` per configuration, or to a list, and store a token for each instance. Refer to [Which instances an alert rule deploys to](use/deployments.md#which-instances-an-alert-rule-deploys-to) and [Use more than one Grafana instance](set-up/grafana-service-account.md#use-more-than-one-grafana-instance).

## My configuration file stopped working. What changed?

The v1 configuration format is no longer supported. Refer to [Migrate from a v1 configuration](configure/migrate-from-v1.md).

## Where do I get help?

[Open an issue](https://github.com/grafana/sigma-rule-deployment/issues) for bugs and feature requests.
