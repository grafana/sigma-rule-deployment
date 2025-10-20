# Grafana Query Integrator GitHub Action

**Grafana Query Integrator** is an experimental GitHub Action that automates the creation of alerting provisioning resources from data source queries so they can be deployed to Grafana. This action is part of the Sigma Rule Deployment GitHub Actions Suite and is intended to be used in conjunction with the Sigma Rule Converter and Sigma Rule Deployer.

## Overview

The Grafana Query Integrator action bridges the gap between converted Sigma rules and Grafana alerting by creating properly formatted alert rule files. It processes the JSON output from the Sigma Rule Converter and generates Grafana-compatible alert rule configurations. This action should be used after the Sigma Rule Converter and before the Sigma Rule Deployer in your CI/CD pipeline.

## Inputs

| Name                               | Description                                                                                          | Required | Default               |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------- | -------- | --------------------- |
| `config_path`                      | Path to the configuration file for the Sigma Rule Integrator                                         | Yes      | `""`                  |
| `grafana_sa_token`                 | Service account token for Grafana for query testing                                                  | No       | `""`                  |
| `pretty_print`                     | Pretty print the JSON output                                                                         | No       | `false`               |
| `output_log_lines`                 | Output log lines to the outputs of the test_query_results                                            | No       | `false`               |
| `all_rules`                        | Whether to integrate all rules                                                                       | No       | `false`               |
| `changed_files_from_base`          | Whether to use the changed files from the base branch                                                | No       | `false`               |
| `actions_username`                 | The username of the actions user                                                                     | No       | `github-actions[bot]` |
| `continue_on_query_testing_errors` | Continue integration process even when query testing fails, but print errors and continue the action | No       | `true`                |

## Outputs

| Name                 | Description                                                                                                |
| -------------------- | ---------------------------------------------------------------------------------------------------------- |
| `rules_integrated`   | List of the filenames of alert rule files created, updated or deleted during integration (space-separated) |
| `test_query_results` | The results of testing the queries against the datasource for the past hour                                |

## Usage

This action is intended to be used in a workflow that triggers on changes to query files or configuration.
It is expected that the Sigma Rule Converter action has been run in the PR.

### Basic Example

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
      - name: Checkout repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          persist-credentials: false

      - name: Integrate queries
        uses: grafana/sigma-rule-deployment/actions/integrate@<HASH>
        with:
          config_path: "./config.yml"
          grafana_sa_token: ${{ secrets.GRAFANA_SA_TOKEN }}
```

## How It Works

1. **Setup**: The action prepares the environment and retrieves configuration paths from the config file.
2. **Configuration Loading**: Loads and validates the integration configuration file, extracting conversion and deployment paths.
3. **File Detection**: Identifies changed conversion files using git diff (either from the last commit or from the base branch).
4. **Query Processing**: For each changed conversion file:
   - Parses the JSON output from the Sigma Rule Converter
   - Extracts queries and rule metadata
   - Validates query syntax and structure
5. **Alert Rule Generation**: Creates Grafana-compatible alert rule files with:
   - Proper alert rule structure and formatting
   - Data source configuration
   - Query expressions
   - Alert conditions and thresholds
   - Rule metadata (title, description, severity, etc.)
6. **Query Testing** (Optional): If `grafana_sa_token` is provided:
   - Tests queries against the configured data source
   - Validates query execution and results
   - Reports any query errors or issues
7. **Output Management**:
   - Saves alert rule files to the deployment directory
   - Removes obsolete alert rule files
8. **Results**: Outputs lists of integrated rules and test results for use in subsequent workflow steps.

## Important Notes

### Configuration Requirements

- The integration config file must specify data source connections and alert rule templates.
- The config file must include `folders.conversion_path` and `folders.deployment_path` settings.
- Data source configurations should include connection details and authentication.
- Alert rule templates define the structure and default values for generated rules.

### Query Testing

- Query testing is optional but recommended for validation.
- Requires a valid Grafana Service Account token with appropriate permissions.
- Tests queries against the past hour of data to validate syntax and execution.
- Results are included in the `test_query_results` output.

### File Management

- The action automatically detects changed conversion files using git diff.
- Only processes files that have been modified since the last commit (or base branch).
- Use `all_rules: true` to process all conversion files regardless of changes.
- Obsolete alert rule files are automatically removed when corresponding conversion files are deleted.

### Best Practices

- Use this action in pull request workflows to validate alert rule generation.
- Include query testing in your integration workflow for early error detection.
- Consider using dedicated Grafana Service Accounts for testing with minimal required permissions.
- Use `continue_on_query_testing_errors: true` to allow the integration to complete even if some queries fail testing.

## Notes

This is a composite action relying on the following external actions:

- [docker/login-action v3 by Docker](https://github.com/docker/login-action)
- [actions/github-script v7 by GitHub](https://github.com/actions/github-script)
