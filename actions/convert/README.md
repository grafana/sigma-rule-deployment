# Sigma Rule Converter GitHub Action

**Sigma Rule Converter** is a GitHub Action that converts Sigma rules to target query languages using [`sigma-cli`](https://github.com/SigmaHQ/sigma-cli). It supports dynamic plugin installation, custom configurations, and output management.

## Inputs

| Name               | Description                                                                                                                           | Required | Default           |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------- | -------- | ----------------- |
| `config_path`      | Path to the Sigma conversion config file. An example config file is available in the config directory at the root of this repository. | Yes      | `./config.yaml`   |
| `plugin_packages`  | Comma-separated list of Sigma CLI plugin packages to install.                                                                         | No       | `""`              |
| `render_traceback` | Whether to render the traceback in the output (`true/false`).                                                                         | No       | `false`           |
| `pretty_print`     | Whether to pretty print the converted files (`true/false`).                                                                           | No       | `false`           |
| `all_rules`        | Whether to convert all rules, regardless of changes (`true/false`).                                                                   | No       | `false`           |
| `changed_files`    | Space-separated list of changed files to process.                                                                                     | No       | `""`              |
| `deleted_files`    | Space-separated list of deleted files to process.                                                                                     | No       | `""`              |
| `conversion_path`  | The path where the conversions will be output to                                                                                      | No       | `"./conversions"` |

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
        uses: ./path-to-your-action
        with:
          config_path: "./config.yaml"
          plugin_packages: "pysigma-backend-loki,pysigma-backend-elasticsearch"
          render_traceback: "false"
          pretty_print: "true"
          all_rules: "false"
          changed_files: "rules/example.yml rules/example2.yml"
          deleted_files: "rules/old.yml rules/old2.yml"
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
5. **Output**: Stores the converted files in the specified `folders.conversion_path` directory.

## Example Plugins

- `pysigma-backend-loki`
- `pysigma-backend-elasticsearch`

## Notes

- Ensure that plugin packages follow the naming convention `pysigma-*` as listed in the [pySigma plugins](https://github.com/SigmaHQ/pySigma-plugin-directory/blob/main/pySigma-plugins-v1.json).
- Use the `render_traceback` input to get detailed error information in case of failures. Essentially this will print the full traceback of the error.
- The `pretty_print` option affects the JSON output formatting by adding newlines and indentation (2 spaces).
- The `all_rules` option forces conversion of all matching rules, regardless of changes. By default, only rules that have changed are converted.
- Input patterns can be glob patterns or specific file paths.
- Pipeline files must be relative to the project root.
- The conversion output includes both queries and rule metadata for deployment.
- The `changed_files` and `deleted_files` parameters should be space-separated lists of file paths relative to the repository root. These are used to determine which rules need to be converted or removed. Wildcards are NOT supported, so you must specify each file. Usually you will use git commands to get the list of changed and deleted files to pass into these parameters.
- The output JSON files contain:
  - `queries`: List of converted queries
  - `conversion_name`: Name of the conversion from the config
  - `input_file`: Path to the original Sigma rule file
  - `rules`: List of rule metadata including ID, title, description, severity, and query
  - `output_file`: Path to the output file relative to the repository root
- For correlation rules to work correctly, all the related rules must be present in the same file using the `---` notation in YAML.
