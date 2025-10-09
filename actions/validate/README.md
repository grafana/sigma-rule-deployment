# Config Validator GitHub Action

A GitHub Action that validates sigma-rule-deployment configuration files against the [JSON schema](https://github.com/grafana/sigma-rule-deployment/blob/main/config/schema.json). This action ensures proper structure and required fields before processing, and is part of the Sigma Rule Deployment GitHub Actions Suite.

## Overview

The Config Validator action should be used as a validation step before running conversion, integration, and deployment actions. It helps catch configuration errors early in your CI/CD pipeline.

## Inputs

| Input         | Description                                                                        | Required | Default              |
| ------------- | ---------------------------------------------------------------------------------- | -------- | -------------------- |
| `config_file` | Path to the configuration file to validate, relative to the root of the repository | No       | `config.yml`         |
| `schema_file` | Path to the JSON schema file, relative to the root of the repository               | No       | `config/schema.json` |

## Usage

### Basic Example

```yaml
name: Validate Configuration

on:
  push:
    paths:
      - 'config.yml'
  pull_request:
    paths:
      - 'config.yml'

jobs:
  validate-config:
    runs-on: ubuntu-latest
    name: Validate Config File
    steps:
      - uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 #v5.0.0
        with:
          fetch-depth: 0
          persist-credentials: false

      - name: Validate Sigma Rule Deployment configuration file
        uses: grafana/sigma-rule-deployment/actions/validate@<HASH>
        with:
          config_file: config.yml
          schema_file: config/schema.json
```

## How It Works

1. **Setup**: The action checks out the repository and prepares the environment for validation.
2. **File Resolution**: Resolves the paths for both the configuration file and JSON schema file relative to the repository root.
3. **Schema Loading**: Loads the JSON schema file (either the default bundled schema or a custom one from your repository).
4. **Configuration Parsing**: Parses the YAML configuration file and converts it to JSON format for validation.
5. **Validation**: Uses `check-jsonschema` to validate the configuration against the schema, checking for:
   - Required fields and their presence
   - Data types and format compliance
   - Nested object structure validation
   - Enum value validation
   - Custom constraint validation
6. **Error Reporting**: Provides detailed error messages if validation fails, including specific field names and expected formats.
7. **Success**: Exits with success status if the configuration passes all validation checks.

## Important Notes

### Path Resolution

- Both `config_file` and `schema_file` paths are resolved relative to the user's repository root.
- Custom schema files must be located within your repository for security reasons.

### Validation Details

- The action uses [`check-jsonschema`](https://github.com/python-jsonschema/check-jsonschema) for validation.
- The action provides detailed error messages when configuration doesn't match the schema.
- When using the default `schema_file`, the action uses [the bundled schema](https://github.com/grafana/sigma-rule-deployment/blob/main/config/schema.json) from the action repository.

### Best Practices

- Use this action as a validation step in workflows that trigger on configuration file changes.
- Include in pull request validation to catch configuration errors early.
- Consider running before conversion, integration, and deployment actions.

## Notes

This is a composite action relying on the following external actions:

- [actions/checkout v5 by GitHub](https://github.com/actions/checkout)
- [actions/setup-python v6 by GitHub](https://github.com/actions/setup-python)
