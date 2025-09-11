# Config Validator GitHub Action

**Config Validator** is a GitHub Action that validates sigma-rule-deployment configuration files against the JSON schema to ensure proper structure and required fields before processing. This action is part of the Sigma Rule Deployment GitHub Actions Suite and is intended to be used as a validation step before running conversion, integration, and deployment actions.

## Inputs

| Name          | Description                                                                                                                                | Required | Default              |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | -------- | -------------------- |
| `config_file` | Path to the configuration file to validate, relative to the root of the repository.                                                        | No       | `config.yml`         |
| `schema_file` | Path to the JSON schema file, relative to the root of the repository. If not provided, uses the default schema from the action repository. | No       | `config/schema.json` |

## Usage

This action is intended to be used as a validation step in workflows that trigger on configuration file changes or as part of pull request validation.

```yaml
name: Validate Sigma Rule Deployment Configuration

on:
  push:
    paths:
      - 'config.yml'
      - '*.yml'
      - '*.yaml'
  pull_request:
    paths:
      - 'config.yml'
      - '*.yml'
      - '*.yaml'

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

## Notes

- Both `config_file` and `schema_file` paths are resolved relative to the repository root.
- The action uses `check-jsonschema` for validation and will fail with detailed error messages if the configuration doesn't match the schema.
- When using the default `schema_file`, the action uses the bundled schema from the action repository.
- Custom schema files must be located within the user's repository for security reasons.
