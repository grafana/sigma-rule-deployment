# Sigma Rule Validation Guide

A comprehensive guide for validating Sigma rules using the [SigmaHQ Sigma Rules Validator](https://github.com/SigmaHQ/sigma-rules-validator) GitHub Action. This validation ensures your Sigma rules conform to the [Sigma specification](https://sigmahq.io/docs/) before conversion and deployment.

## Overview

The [SigmaHQ Sigma Rules Validator](https://github.com/SigmaHQ/sigma-rules-validator) is an official GitHub Action that helps ensure that rules are correctly formatted and will work with Sigma converters and the actions in this repository.

**Key Benefits:**
- Validates rules against the official [Sigma specification JSON schema](https://github.com/SigmaHQ/sigma-specification)
- Catches formatting errors early in your CI/CD pipeline
- Ensures compatibility with Sigma conversion tools

> [!IMPORTANT]
> The [Sigma Rules Validator](https://github.com/SigmaHQ/sigma-rules-validator) action does not currently work with multiple documents in a single YAML and hence we recommend storing such rules in a separate directory from the Sigma rules.

## Quick Start

Add this workflow step to validate all Sigma rules in your repository:

```yaml
steps:
  - name: Validate Sigma rules
    uses: SigmaHQ/sigma-rules-validator@66a5abe3fb6d13ac203ded42045580845c9c5534 #v1
```

## Action Inputs

The action supports the following inputs:

| Input        | Description                                                                                           | Required | Default                                                                                                    |
|--------------|-------------------------------------------------------------------------------------------------------|----------|------------------------------------------------------------------------------------------------------------|
| `paths`      | Path(s) to the Sigma rules in your repository. Can be a single path or multiple paths separated by newlines | No       | `./`                                                                                                       |
| `schemaURL`  | URL to the JSON schema for Sigma validation                                                           | No       | Latest schema from [sigma-specification](https://github.com/SigmaHQ/sigma-specification) repository      |
| `schemaFile` | Path to a local JSON schema file in your repository                                                  | No       | Uses `schemaURL` if not provided                                                                           |

> Note that the schemaURL default value [no longer exists](https://github.com/SigmaHQ/sigma-rules-validator/issues/4), and you must specify schemaURL to make sure you validate correctly until it's changed upstream. See the example below for an updated reference.

## Usage Examples


```yaml
name: Validate Sigma Rules

on:
  push:
    branches: ["*"]
    paths:
      - "staging/rules-development/**.yml"
      - "prod/rules/**.yml"

  pull_request:
    branches: [main]
    paths:
      - "staging/rules-development/**.yml"
      - "prod/rules/**.yml"

  workflow_dispatch:

jobs:
  yamllint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: "yaml-lint"
        uses: ibiqlik/action-yamllint@v3.1.1

  sigma-rules-validator:
    runs-on: ubuntu-latest
    steps:
      - name: Validate Sigma rules
        uses: SigmaHQ/sigma-rules-validator@66a5abe3fb6d13ac203ded42045580845c9c5534 #v1
        with:
          schemaURL: https://raw.githubusercontent.com/SigmaHQ/sigma-specification/refs/heads/main/json-schema/sigma-detection-rule-schema.json
          paths: |-
            ./staging/rules-development
            ./prod/rules
```

### Getting Help

- Review the [official documentation](https://github.com/SigmaHQ/sigma-rules-validator)
- Check the [Sigma specification](https://github.com/SigmaHQ/sigma-specification) for rule format requirements
- See the [SigmaHQ documentation](https://sigmahq.io/docs/) for general Sigma rule guidance
