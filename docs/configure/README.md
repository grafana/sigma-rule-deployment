# Configuration file

One YAML file configures every stage of Sigma Rule Deployment. It says which Sigma rules to convert and how, which data source and Grafana folder their alert rules use, and which Grafana instances they deploy to. This page explains the structure of the file and how its settings combine.

## Structure

A configuration file has four top-level keys:

- `version` is always `2`.
- `folders` names the folders for the generated conversion and deployment files.
- `defaults` holds settings that apply to every configuration.
- `configurations` is a list of named configurations. Each one selects a set of Sigma rules and can override any default.

Settings in `defaults` and in each configuration are grouped into three blocks, one for each pipeline stage: `conversion`, `integration`, and `deployment`.

The following example converts Okta rules with the Loki backend and deploys them to one Grafana instance:

```yaml
version: 2
folders:
  conversion_path: ./conversions
  deployment_path: ./deployments
defaults:
  conversion:
    target: loki
  integration:
    data_source: <DATA_SOURCE_UID>
    folder_id: <FOLDER_UID>
    org_id: 1
    test_queries: true
  deployment:
    grafana_instance: https://<STACK_NAME>.grafana.net
configurations:
  - name: okta_audit
    conversion:
      input: rules/okta/*
      pipelines:
        - pipelines/okta.yml
    integration:
      rule_group: Every 1 Hour
      time_window: 1h
```

For an annotated example with more settings, refer to [`config-example-v2.yml`](../../config/config-example-v2.yml). For every setting, refer to the [Configuration reference](configuration-reference.md).

## Configurations

Each configuration has a `name` and a `conversion.input` that selects its Sigma rule files. The name must be unique, and can contain only letters, digits, `-`, and `_`. It appears in the names of the generated files and in a `SigmaConfiguration` annotation on each alert rule, and you use it to target the configuration with [comment commands](../use/comment-commands.md).

Use separate configurations when a set of rules needs different settings from the rest, for example, a different pipeline, data source, rule group, or Grafana instance.

## How defaults and overrides combine

A setting in a configuration overrides the matching setting in `defaults`. How the two combine depends on the type of the setting.

| Type of setting  | Examples                                                                                                     | Behavior in a configuration                                                                                                                                                                              |
| ---------------- | ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Text and numbers | `rule_group`, `time_window`, `folder_id`, `org_id`, `data_source`, `from`, `to`, `json_indent`               | Replaces the default. An empty text value counts as not set, so the default still applies. A `json_indent` of `0` is a real value that overrides the default.                                          |
| Booleans         | `test_queries`, `show_log_lines`, `show_sample_values`, `continue_on_error`, `template_all_rules`            | Replaces the default when set, including `false` to turn off a default of `true`. When neither is set, the value is `false`.                                                                            |
| Lists and maps   | `pipelines`, `filters`, `backend_options`, `required_rule_fields`, `template_labels`, `template_annotations` | Replaces the default entirely. Lists and maps aren't merged, so repeat any default entries you want to keep. Set `[]` or `{}` to remove the default. Leave the key out to keep it.                      |

In the following example, `okta_audit` skips query testing and gets only the `Level` and `Team` labels. It doesn't get the default `Product` label. `gcp_k8s` keeps all the defaults.

```yaml
defaults:
  integration:
    test_queries: true
    template_labels:
      Level: "{{.Level}}"
      Product: "{{.Logsource.Product}}"
configurations:
  - name: gcp_k8s
    conversion:
      input: rules/gcp/*
  - name: okta_audit
    conversion:
      input: rules/okta/*
    integration:
      test_queries: false
      template_labels:
        Level: "{{.Level}}"
        Team: identity
```

The `continue_on_query_testing_errors` input of the convert and integrate workflow sets the default for `continue_on_error`, so it takes precedence over `defaults.integration.continue_on_error`. A configuration's own `continue_on_error` still overrides it.

## Validate the file

The configuration file follows the JSON schema in [`schema-v2.json`](../../config/schema-v2.json). The [validation workflow](../set-up/workflows.md#validate-the-configuration) checks the file against it on every change.

Editors that use the YAML language server, for example, Visual Studio Code with the Red Hat YAML extension, can also check the file and autocomplete settings as you type. Add this comment to the first line of the file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/grafana/sigma-rule-deployment/main/config/schema-v2.json
```

## Related pages

- [Configuration reference](configuration-reference.md)
- [Data sources and query models](data-sources.md)
- [Alert rule labels and annotations](labels-and-annotations.md)
- [Migrate from a v1 configuration](migrate-from-v1.md)
