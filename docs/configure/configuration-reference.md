# Configuration reference

This page lists every setting in the Sigma Rule Deployment configuration file, with its type and default. Use it when you need the exact name or behavior of a setting. For how the file is structured and how defaults and overrides combine, refer to [Configuration file](README.md).

The settings under `conversion`, `integration`, and `deployment` are valid both in `defaults` and in each entry of `configurations`. A default listed on this page is the value the Actions use when neither the configuration nor `defaults` sets the setting.

## Top-level settings

| Setting          | Type    | Required | Description                                                         |
| ---------------- | ------- | -------- | ------------------------------------------------------------------- |
| `version`        | integer | Yes      | Configuration format version. Must be `2`.                          |
| `folders`        | object  | Yes      | Folders for the generated files. Refer to [Folders](#folders).      |
| `defaults`       | object  | No       | Settings applied to every configuration unless it overrides them.  |
| `configurations` | list    | Yes      | One or more configurations. Refer to [Configurations](#configurations). |

## Folders

Both folders are paths relative to the root of your repository. The Actions create them if they don't exist.

| Setting           | Type   | Required | Description                                                  |
| ----------------- | ------ | -------- | ------------------------------------------------------------ |
| `conversion_path` | string | Yes      | Folder for the conversion files, for example, `./conversions`. |
| `deployment_path` | string | Yes      | Folder for the alert rule files, for example, `./deployments`. |

## Configurations

Each entry in `configurations` accepts these settings in addition to the `conversion`, `integration`, and `deployment` blocks.

| Setting            | Type             | Required | Description                                                                                                                                       |
| ------------------ | ---------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`             | string           | Yes      | Unique name of the configuration. Letters, digits, `-`, and `_` only.                                                                             |
| `conversion.input` | string or list   | Yes      | Glob patterns that select Sigma rule files, relative to the repository root. Use `**` to match subfolders, for example, `rules/**/*.yml`. Valid only in a configuration, not in `defaults`. |

## Conversion settings

These settings control how Sigma CLI converts the rules. Most map directly onto a [`sigma convert`](https://github.com/SigmaHQ/sigma-cli) option.

| Setting                | Type    | Default        | Description                                                                                                                                                         |
| ---------------------- | ------- | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `target`               | string  | `loki`         | Sigma backend to convert with, for example, `loki` or `elasticsearch`. The Loki backend is included. Install any other backend with the `plugin_packages` workflow input, for example, `pysigma-backend-elasticsearch`. |
| `format`               | string  | `default`      | Output format of the backend.                                                                                                                                       |
| `pipelines`            | list    | `[]`           | Sigma pipelines to apply, as file paths relative to the repository root or as the names of pipelines that an installed backend provides.                            |
| `filters`              | list    | `[]`           | Sigma filters to apply, as file paths relative to the repository root.                                                                                              |
| `backend_options`      | map     | `{}`           | Options passed to the backend. For the Loki backend options, refer to [Loki backend options](data-sources.md#loki-backend-options).                                 |
| `file_pattern`         | string  | `*.yml`        | Only files matching this pattern are converted, from the files that `input` selects.                                                                               |
| `skip_unsupported`     | boolean | `true`         | Skip rules the backend can't convert, instead of failing.                                                                                                           |
| `fail_unsupported`     | boolean | `false`        | Fail when a rule uses a feature the backend doesn't support.                                                                                                        |
| `pipeline_check`       | boolean | `true`         | Check that the pipelines are compatible with the backend.                                                                                                           |
| `without_pipeline`     | boolean | `false`        | Convert without a pipeline, even when the backend requires one.                                                                                                     |
| `correlation_method`   | string  | Backend default | Method the backend uses to generate correlation queries.                                                                                                           |
| `encoding`             | string  | `utf-8`        | Encoding of the backend output: `utf-8`, `utf-16`, `ascii`, or `latin1`.                                                                                            |
| `json_indent`          | integer | `0`            | Indentation of JSON backend output.                                                                                                                                 |
| `verbose`              | boolean | `false`        | Print verbose Sigma CLI output.                                                                                                                                     |
| `required_rule_fields` | list    | `[]`           | Sigma rule fields to copy into the conversion file, so labels and annotations can use them. An empty list copies the whole rule. Refer to [Alert rule labels and annotations](labels-and-annotations.md). |

## Integration settings

These settings control the alert rules the integrate stage builds, and how it tests their queries.

| Setting                | Type    | Default    | Description                                                                                                                                                                                         |
| ---------------------- | ------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `data_source`          | string  | None       | UID of the Grafana data source the alert rules query. Refer to [Find a data source UID](data-sources.md#find-a-data-source-uid).                                                                    |
| `data_source_type`     | string  | `target`   | Data source plugin type ID, or a Sigma backend name that identifies one, used to choose a built-in query model. Refer to [Data sources and query models](data-sources.md).                        |
| `query_model`          | string  | None       | Custom query model, as a `fmt.Sprintf` format string. Overrides the built-in model. Refer to [Use a custom query model](data-sources.md#use-a-custom-query-model).                                 |
| `folder_id`            | string  | None       | UID of the Grafana folder the alert rules go in. Every configuration must resolve a value, here or in `defaults`.                                                                                  |
| `org_id`               | integer | None       | ID of the Grafana organization. Every configuration must resolve a value, here or in `defaults`.                                                                                                   |
| `rule_group`           | string  | `Default`  | Name of the alert rule group in Grafana.                                                                                                                                                            |
| `time_window`          | duration | Refer to the description | How far back each query looks, and how often the rule group is evaluated. Set it explicitly: when it isn't set, queries look back `1m` but the rule group is evaluated every `5m`. |
| `lookback`             | duration | `0s`      | Shifts the query time range back by this amount, to allow for ingestion delay. For example, a `5m` window with a `1m` lookback queries from 6 minutes ago to 1 minute ago.                         |
| `test_queries`         | boolean | `false`    | Run each query against the data source and report the results on the pull request.                                                                                                                  |
| `from`                 | string  | `now-1h`   | Start of the query testing time range, for example, `now-24h`.                                                                                                                                       |
| `to`                   | string  | `now`      | End of the query testing time range.                                                                                                                                                                |
| `show_log_lines`       | boolean | `false`    | Include log lines in the query test results.                                                                                                                                                        |
| `show_sample_values`   | boolean | `false`    | Include sample label values in the query test results.                                                                                                                                              |
| `continue_on_error`    | boolean | `false`    | Report query testing errors instead of failing the integration. The `continue_on_query_testing_errors` workflow input sets the default, and the workflow input defaults to `true`.                |
| `template_labels`      | map     | `{}`       | Labels to add to each alert rule, as Go templates. Refer to [Alert rule labels and annotations](labels-and-annotations.md).                                                                       |
| `template_annotations` | map     | `{}`       | Annotations to add to each alert rule, as Go templates.                                                                                                                                             |
| `template_all_rules`   | boolean | `false`    | Pass every rule in the Sigma file to the templates, instead of only the first.                                                                                                                      |

A duration is a number with a unit: `s`, `m`, `h`, or `d`, for example, `30s`, `5m`, `1h30m`, or `1d12h`. Days come first.

## Deployment settings

These settings control where the deploy stage provisions alert rules.

| Setting            | Type           | Default | Description                                                                                                                                       |
| ------------------ | -------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `grafana_instance` | string or list | None    | URL of the Grafana instance to deploy to, or a list of URLs to deploy to several. Every configuration must resolve at least one instance.        |
| `timeout`          | duration       | `10s`   | Timeout for HTTP requests to Grafana.                                                                                                             |

If configurations use different instances, store a token for each one. Refer to [Use more than one Grafana instance](../set-up/grafana-service-account.md#use-more-than-one-grafana-instance).

## Related pages

- [Configuration file](README.md)
- [`schema-v2.json`](../../config/schema-v2.json), the JSON schema of the configuration file
- [`config-example-v2.yml`](../../config/config-example-v2.yml), an annotated example
