# Alert rule labels and annotations

The integrate stage adds built-in annotations to every alert rule it builds, and you can add your own labels and annotations as templates filled in from the Sigma rule's metadata, for example, its level or log source. Labels drive [notification policies](https://grafana.com/docs/grafana/latest/alerting/fundamentals/notifications/notification-policies/), so templated labels let you route alerts by severity, product, or team.

## Template labels and annotations

Set `integration.template_labels` and `integration.template_annotations` to maps of names to [Go templates](https://pkg.go.dev/text/template). The integrate stage fills in each template from the first Sigma rule in the rule file.

```yaml
defaults:
  integration:
    template_labels:
      Level: "{{.Level}}"
      Product: "{{.Logsource.Product}}"
      Service: "{{.Logsource.Service}}"
    template_annotations:
      Author: "{{.Author}}"
      Description: "{{.Description}}"
```

A configuration that sets either map replaces the default map entirely. Refer to [How defaults and overrides combine](README.md#how-defaults-and-overrides-combine).

### Available fields

Templates refer to Sigma rule fields by their Go names, which start with a capital letter:

| Field                                                                 | Sigma field                       |
| --------------------------------------------------------------------- | --------------------------------- |
| `.Title`, `.ID`, `.Name`, `.Status`, `.Description`, `.License`       | Same name in lowercase            |
| `.Author`, `.Date`, `.Modified`, `.Level`, `.Scope`, `.Taxonomy`      | Same name in lowercase            |
| `.References`, `.Tags`, `.Fields`, `.FalsePositives`                  | Lists of the same name            |
| `.Logsource.Category`, `.Logsource.Product`, `.Logsource.Service`, `.Logsource.Definition` | `logsource` fields |

The conversion file only contains the rule fields listed in `conversion.required_rule_fields`, plus `id` and `title`. If you set `required_rule_fields`, include every field your templates use. Leave it empty to keep the whole rule.

### Template functions

Templates can use these string functions, most of them from Go's `strings` package: `toUpper`, `toLower`, `title`, `trim`, `trimSpace`, `trimPrefix`, `trimSuffix`, `trimLeft`, `trimRight`, `hasPrefix`, `hasSuffix`, `contains`, `containsAny`, `replace`, `replaceAll`, `split`, `splitAfter`, `splitAfterN`, `splitN`, `join`, `fields`, `index`, `lastIndex`, `indexAny`, `lastIndexAny`, `count`, `repeat`, `compare`, and `equalFold`.

For example, the following label is `CLOUDTRAIL` for a rule with `logsource.service: cloudtrail`:

```yaml
template_labels:
  Service: "{{ toUpper .Logsource.Service }}"
```

`index` is `strings.Index`, which replaces Go's built-in `index` function in these templates.

## Use every rule in the file

A rule file can contain more than one rule, for example, a correlation rule and the rules it references. By default, templates only see the first rule. Set `integration.template_all_rules: true` to pass the list of every rule in the file instead.

With `template_all_rules` on, the `highestLevel` function returns the highest `level` among the rules, in the order `informational`, `low`, `medium`, `high`, `critical`:

```yaml
integration:
  template_all_rules: true
  template_labels:
    Level: "{{ highestLevel . }}"
    Titles: "{{ range . }}{{ .Title }}; {{ end }}"
```

`highestLevel` only exists when `template_all_rules` is on. Using it without the setting fails with a template parsing error.

## Built-in annotations

Every alert rule gets the following annotations, so you can trace it back to its source:

| Annotation           | Value                                                    |
| -------------------- | -------------------------------------------------------- |
| `Query`              | The first converted query, before any Loki metric wrapping. |
| `TimeWindow`         | The `time_window` the query uses.                        |
| `Lookback`           | The `lookback` the query uses.                           |
| `LogSourceUid`       | The UID of the data source.                              |
| `LogSourceType`      | The conversion `target`.                                 |
| `SigmaRule`          | Path of the Sigma rule file.                             |
| `ConversionFile`     | Path of the conversion file.                             |
| `DeploymentFile`     | Path of the alert rule file.                             |
| `SigmaConfiguration` | Name of the configuration that produced the alert rule.  |

A template annotation with the same name as a built-in annotation replaces it.

## Related pages

- [Configuration reference](configuration-reference.md)
- [Configure notifications](https://grafana.com/docs/grafana/latest/alerting/configure-notifications/) in the Grafana documentation
