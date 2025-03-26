# Grafana Query Integrator GitHub Action

**Grafana Query Integrator** is an experimental GitHub Action that automates the creation of alerting provisioning resources from data source queries so they can be deployed to Grafana. This action is part of the Sigma Rule Deployment GitHub Actions Suite and is intended to be used in conjunction with the Sigma Rule Converter and Sigma Rule Deployer.

## Inputs

| Name               | Description                                                             | Required | Default |
| ------------------ | ----------------------------------------------------------------------- | -------- | ------- |
| `config_path`      | Path to the Sigma Integrator config file.                               | Yes      | `""`    |
| `grafana_sa_token` | Grafana Service Account token to use for the testing of the alert rules | No       | `""`    |
| `github_token`     | GitHub token to use for the fetching of the repo and commiting          | No       | `""`    |
| `pretty_print`     | Whether the alert rule files produced should be pretty printed          | No       | `false` |
| `output_log_lines` | Whether the query testing log line results should be output             | No       | `false` |
| `all_rules`        | Whether to integrate all rules in the conversion folder                 | No       | `false` |
| `changed_files`    | A space-separated list of file paths to integrate                       | No       | `""`    |
| `deleted_files`    | A space-separated list of file paths to remove                          | No       | `""`    |

## Outputs

| Name                 | Description                                                                                                |
| -------------------- | ---------------------------------------------------------------------------------------------------------- |
| `rules_integrated`   | List of the filenames of alert rule files created, updated or deleted during integration (space-separated) |
| `test_query_results` | The results of testing the queries against the datasource for the past hour                                |

## Usage

This action is intended to be used in a workflow that triggers on the change to query files or configuration.
It is expected that the Sigma Rule Converter actions has been run in the PR.

This is an example of a workflow:

```yaml
name: Integrate Sigma rules

on:
  pull_request:
    branches:
      - main
    paths:
      - "conversions/*"
      - "config.yml"

jobs:
  integrate:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - name: Integrate queries
        uses: grafana/sigma-rule-deployment/actions/integrate@<HASH>
        with:
          config_path: "./config.yml"
```

## Notes

This is a composite action relying on the following external actions:

- [docker/login-action v3 by Docker](https://github.com/docker/login-action)
