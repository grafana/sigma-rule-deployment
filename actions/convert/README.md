# Sigma Rule Converter GitHub Action

**Sigma Rule Converter** is a GitHub Action that converts Sigma rules to target query languages using `sigma-cli`. It supports dynamic plugin installation, custom configurations, and output management.

## Inputs

| Name               | Description                                                                                                                           | Required | Default         |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------- | -------- | --------------- |
| `config_path`      | Path to the Sigma conversion config file. An example config file is available in the config directory at the root of this repository. | Yes      | `./config.yaml` |
| `output_dir`       | Path to the output directory. This path will always be inside the GITHUB_WORKSPACE directory root.                                    | No       | `./conversions` |
| `plugin_packages`  | Comma-separated list of Sigma CLI plugin packages to install.                                                                         | No       | `""`            |
| `render_traceback` | Whether to render the traceback in the output (`true/false`).                                                                         | No       | `false`         |

## Outputs

| Name          | Description                                 |
| ------------- | ------------------------------------------- |
| `output_path` | The path of the generated output directory. |

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
          output_dir: "./conversions"
          plugin_packages: "pysigma-backend-loki"
          render_traceback: "true"

      - name: Upload output artifacts
        uses: actions/upload-artifact@v3
        with:
          name: sigma-conversion-output
          path: ${{ steps.convert.outputs.output_path }}
```

## How It Works

1. **Setup**: Installs Python and `uv`, the dependency manager.
2. **Plugin Installation**: Dynamically installs Sigma CLI plugins specified in `plugin_packages`. Only packages starting with `pysigma-backend-` are allowed.
3. **Conversion**: Runs the conversion script using `uv` and the provided configuration file.
4. **Output**: Stores the converted files in the specified `output_dir` and provides it as an output for further steps.

## Example Plugins

- `pysigma-backend-loki`
- `pysigma-backend-elasticsearch`

## Notes

- Ensure that plugin packages follow the naming convention `pysigma-backend-*`.
- Use the `render_traceback` input to get detailed error information in case of failures.
