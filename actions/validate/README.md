# Config Validator GitHub Action

A GitHub Action that validates sigma-rule-deployment configuration files against the [v2 JSON schema](https://github.com/grafana/sigma-rule-deployment/blob/main/config/schema-v2.json). It also detects the deprecated v1 format and tells you to migrate. This action ensures proper structure and required fields before processing, and is part of the Sigma Rule Deployment GitHub Actions Suite.

## Overview

The Config Validator action should be used as a validation step before running conversion, integration, and deployment actions. It helps catch configuration errors early in your CI/CD pipeline. For the workflow setup, refer to [Validate the configuration](../../docs/set-up/workflows.md#validate-the-configuration), and for the configuration file itself, refer to [Configuration file](../../docs/configure/README.md).

## Inputs

| Input            | Description                                                                                                                                  | Required | Default                 |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ----------------------- |
| `config_file`    | Path to the configuration file to validate, relative to the root of the repository                                                          | No       | `config.yml`            |
| `v2_schema_file` | Path to the v2 JSON schema file, relative to the root of the repository. Defaults to the schema bundled with this action                    | No       | `config/schema-v2.json` |
| `v1_schema_file` | Path to the v1 JSON schema file, relative to the root of the repository. Only used to recognise a v1 config. Defaults to the bundled schema | No       | `config/schema.json`    |
| `github_token`   | GitHub token used to post a PR comment with a suggested v2 configuration when a v1 configuration is detected. If omitted, no comment is posted | No       | `""`                    |

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
```

## How It Works

1. **Setup**: The action checks out the repository and prepares the environment for validation.
2. **File Resolution**: Resolves the paths for the configuration file and the JSON schema files relative to the repository root.
3. **v2 Validation**: Uses `check-jsonschema` to validate the configuration against the v2 schema, checking for:
   - Required fields and their presence
   - Data types and format compliance
   - Nested object structure validation
   - Enum value validation
   - Custom constraint validation

   If the configuration is valid, the action exits with success.
4. **v1 Detection**: If v2 validation fails, the configuration is checked against the v1 schema. A valid v1 configuration is deprecated and no longer supported by the Actions, so the action fails with a message asking you to migrate. It generates a suggested v2 configuration using the [migration script](../../scripts/migrate-config-v1-to-v2/README.md) and prints it to the log, or, on a pull request when `github_token` is provided, posts it as a PR comment.
5. **Error Reporting**: If the configuration is valid in neither format, the action prints the v2 validation errors, including specific field names and expected formats, and fails.

## Important Notes

### Path Resolution

- `config_file`, `v2_schema_file` and `v1_schema_file` paths are resolved relative to the user's repository root.
- Custom schema files must be located within your repository for security reasons.

### Validation Details

- The action uses [`check-jsonschema`](https://github.com/python-jsonschema/check-jsonschema) for validation.
- The action provides detailed error messages when configuration doesn't match the schema.
- When using the default schema inputs, the action uses the bundled [v2 schema](https://github.com/grafana/sigma-rule-deployment/blob/main/config/schema-v2.json) and [v1 schema](https://github.com/grafana/sigma-rule-deployment/blob/main/config/schema.json) from the action repository.
- A valid v1 configuration still fails the action. To post the suggested migration as a PR comment, pass `github_token` and grant the job `pull-requests: write`.

### Best Practices

- Use this action as a validation step in workflows that trigger on configuration file changes.
- Include in pull request validation to catch configuration errors early.
- Consider running before conversion, integration, and deployment actions.

## Notes

This is a composite action relying on the following external actions:

- [actions/checkout v5 by GitHub](https://github.com/actions/checkout)
- [actions/setup-python v6 by GitHub](https://github.com/actions/setup-python)
