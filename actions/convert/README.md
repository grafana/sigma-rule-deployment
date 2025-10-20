# Sigma Rule Converter GitHub Action

**Sigma Rule Converter** is a GitHub Action that converts Sigma rules to target query languages using [`sigma-cli`](https://github.com/SigmaHQ/sigma-cli). It supports dynamic plugin installation, custom configurations and output management. This action is part of the Sigma Rule Deployment GitHub Actions Suite and should be used as a conversion step before running integration and deployment actions.

## Overview

The Sigma Rule Converter action transforms Sigma detection rules into target-specific query languages (such as Loki, Elasticsearch, Splunk, etc.) using the Sigma CLI toolchain. It processes YAML-based Sigma rules and generates JSON output files containing converted queries and rule metadata. This action should be used as the first step in your CI/CD pipeline, typically triggered by changes to Sigma rule files in pull requests.

## Inputs

| Name                      | Description                                                                                                                           | Required | Default                 |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------- | ----------------------- |
| `config_path`             | Path to the Sigma conversion config file. An example config file is available in the config directory at the root of this repository. | Yes      | `./config.yaml`         |
| `plugin_packages`         | Comma-separated list of Sigma CLI plugin packages to install.                                                                         | No       | `""`                    |
| `render_traceback`        | Whether to render the traceback in the output (`true/false`).                                                                         | No       | `false`                 |
| `pretty_print`            | Whether to pretty print the converted files (`true/false`).                                                                           | No       | `false`                 |
| `all_rules`               | Whether to convert all rules, regardless of changes (`true/false`).                                                                   | No       | `false`                 |
| `actions_username`        | The username of the github actions bot committer.                                                                                     | No       | `"github-actions[bot]"` |
| `changed_files_from_base` | Calculate changes from PR base.                                                                                                       | No       | `"false"`               |

## Usage

```yaml
name: Sigma Rule Conversion

on:
  push:
    branches:
      - main
  workflow_dispatch:  # Allow manual triggering (optional)

jobs:
  convert:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Run Sigma Rule Converter
        uses: grafana/sigma-rule-deployment/actions/convert@<HASH>
        with:
          config_path: "./config.yaml"
          plugin_packages: "pysigma-backend-loki,pysigma-backend-elasticsearch"
          render_traceback: "false"
          pretty_print: "true"
          all_rules: "false"
```

## How It Works

1. **Setup**: Installs Python and `uv`, the dependency manager.
2. **Plugin Installation**: Dynamically installs Sigma CLI plugins specified in `plugin_packages`. Only packages starting with `pysigma-` are allowed. URLs not supported for now. The packages should be comma separated.
3. **Configuration**: Loads and validates the configuration file, applying defaults where needed.
4. **Conversion**: For each conversion object in the config:
   - Processes input files matching the specified patterns
   - Applies pipelines and filters
   - Converts rules using the specified backend
   - Generates JSON output with queries and rule metadata
5. **Output**: Stores the converted files in the specified `folders.conversion_path` directory as JSON files with the following structure:

   ```json
   {
     "queries": ["query1", "query2"],
     "conversion_name": "conversion_name",
     "input_file": "input_file",
     "rules": [
       {
         "id": "rule_id",
         "title": "rule_title",
         "description": "rule_description",
         "severity": "rule_severity",
         "query": "rule_query"
       }
     ],
     "output_file": "output_file"
   }
   ```

## Example Plugins

- `pysigma-backend-loki`
- `pysigma-backend-elasticsearch`

## Notes

- Ensure that plugin packages follow the naming convention `pysigma-*` as listed in the [pySigma plugins](https://github.com/SigmaHQ/pySigma-plugin-directory/blob/main/pySigma-plugins-v1.json).
- Use the `render_traceback` input to get detailed error information in case of failures. Essentially this will print the full traceback of the error.
- The `pretty_print` option affects the JSON output formatting by adding newlines and indentation (2 spaces).
- The `all_rules` option forces conversion of all matching rules, regardless of changes. By default, only rules that have changed are converted.
- Input patterns can be glob patterns or specific file paths. When working with nested directories, use the correct glob syntax:
  - `rules/**/*.yml` - Matches all `.yml` files in the `rules` directory and all subdirectories
  - `rules/*.yml` - Only matches `.yml` files directly in the `rules` directory (not in subdirectories)
- Pipeline files must be relative to the project root.
- The conversion output includes both queries and rule metadata for deployment.
- The action automatically detects changed and deleted files using git diff to determine which rules need to be converted or removed.
- The output JSON files contain:
  - `queries`: List of converted queries
  - `conversion_name`: Name of the conversion from the config
  - `input_file`: Path to the original Sigma rule file
  - `rules`: List of rule metadata including ID, title, description, severity, and query
  - `output_file`: Path to the output file relative to the repository root
- For correlation rules to work correctly, all the related rules must be present in the same file using the `---` notation (multi document) in YAML.

## External Dependencies

This is a composite action relying on the following external actions:

- [docker/login-action v3 by Docker](https://github.com/docker/login-action)
- [actions/github-script v7 by GitHub](https://github.com/actions/github-script)
