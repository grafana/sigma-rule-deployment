# Set up the workflows

Four GitHub Actions workflows in your rule repository run the Sigma Rule Deployment pipeline. Two validate your files on every pull request, one converts and integrates your rules, and one deploys them after you merge. This page explains each workflow and how to add it.

## Before you begin

You need a rule repository with a configuration file, and a Grafana service account token stored as a GitHub secret. Refer to [Get started](../get-started.md) if you haven't set these up yet.

## Choose a version

Every workflow refers to this repository at a specific version. Replace `<COMMIT_SHA>` in the examples on this page with the full commit SHA of a [release](https://github.com/grafana/sigma-rule-deployment/releases), and keep the release tag in a trailing comment so you can tell which version you run:

```yaml
uses: grafana/sigma-rule-deployment/.github/workflows/deploy.yml@<COMMIT_SHA> # <RELEASE_TAG>
```

A release tag such as `@<RELEASE_TAG>` also works, but a commit SHA can't be moved to different code, so it's the safer choice. Tools such as Renovate and Dependabot can keep the SHA up to date.

## Workflow templates

The [`config/`](../../config/) folder of this repository contains a template for three of the workflows. Copy them into `.github/workflows/` in your rule repository and adjust the paths and the version.

| Template                                                                    | Workflow                      |
| --------------------------------------------------------------------------- | ----------------------------- |
| [`validate-config.yml`](../../config/validate-config.yml)                   | Validate the configuration    |
| [`convert-integrate-sigma.yml`](../../config/convert-integrate-sigma.yml)   | Convert and integrate         |
| [`deploy.yml`](../../config/deploy.yml)                                     | Deploy                        |

## Validate the configuration

This workflow checks your configuration file against the configuration schema whenever it changes, so mistakes surface before conversion runs. It uses the [Config Validator](../../actions/validate/README.md) Action.

```yaml
name: Validate Sigma Rule Deployment configuration

on:
  push:
    paths:
      - "config.yml"
  pull_request:
    paths:
      - "config.yml"

permissions: {}

jobs:
  validate-config:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@<CHECKOUT_COMMIT_SHA> # <CHECKOUT_RELEASE_TAG>
        with:
          persist-credentials: false

      - name: Validate configuration file
        uses: grafana/sigma-rule-deployment/actions/validate@<COMMIT_SHA> # <RELEASE_TAG>
        with:
          config_file: config.yml
```

If the file uses the old v1 format, the Action fails and prints a suggested v2 version. Refer to [Migrate from a v1 configuration](../configure/migrate-from-v1.md).

## Validate Sigma rules

This workflow checks your rule files against the Sigma specification with the official [SigmaHQ Sigma Rules Validator](https://github.com/SigmaHQ/sigma-rules-validator), and lints them with yamllint. It catches malformed rules before the converter sees them.

```yaml
name: Validate Sigma rules

on:
  pull_request:
    paths:
      - "rules/**.yml"

permissions: {}

jobs:
  yamllint:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@<CHECKOUT_COMMIT_SHA> # <CHECKOUT_RELEASE_TAG>
        with:
          persist-credentials: false
      - uses: ibiqlik/action-yamllint@<YAMLLINT_COMMIT_SHA> # <YAMLLINT_RELEASE_TAG>
        with:
          file_or_dir: rules/

  sigma-rules-validator:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@<CHECKOUT_COMMIT_SHA> # <CHECKOUT_RELEASE_TAG>
        with:
          persist-credentials: false
      - uses: SigmaHQ/sigma-rules-validator@<VALIDATOR_COMMIT_SHA> # <VALIDATOR_RELEASE_TAG>
        with:
          schemaURL: https://raw.githubusercontent.com/SigmaHQ/sigma-specification/refs/heads/main/json-schema/sigma-detection-rule-schema.json
          paths: |-
            ./rules
```

Always set `schemaURL`. The validator's default schema URL [no longer exists](https://github.com/SigmaHQ/sigma-rules-validator/issues/4), so validation is unreliable without it.

The validator accepts the following inputs:

| Input        | Description                                                                 | Default            |
| ------------ | --------------------------------------------------------------------------- | ------------------ |
| `paths`      | One or more paths to Sigma rules, separated by newlines.                    | `./`               |
| `schemaURL`  | URL of the JSON schema to validate against.                                 | A URL that no longer exists |
| `schemaFile` | Path to a local JSON schema file. Overrides `schemaURL`.                    | None               |

> [!IMPORTANT]
> The validator doesn't support files with more than one YAML document, which correlation rules need. Leave the folder that holds your correlation rules out of `paths`. Refer to [Rule requirements](rules-and-pipelines.md#rule-requirements).

yamllint reads its settings from a `.yamllint` file in the root of your repository. The following configuration is based on the one the [SigmaHQ rule repository](https://github.com/SigmaHQ/sigma/blob/master/.yamllint) uses. It turns off the line length limit, because Sigma rules often have long lines, and expects 4-space indentation, like the SigmaHQ rules. Your configuration file and workflows probably use 2-space indentation, which is why the workflow only lints the `rules/` folder.

```yaml
# https://yamllint.readthedocs.io/en/latest/configuration.html
extends: default

rules:
    comments:
        require-starting-space: true
        min-spaces-from-content: 1
    comments-indentation: disable
    document-start: {present: false}
    empty-lines: {max: 2, max-start: 2, max-end: 2}
    indentation: {spaces: 4, indent-sequences: whatever, ignore: .github/workflows/}
    line-length: disable
    new-line-at-end-of-file: enable
    trailing-spaces: {}
```

## Convert and integrate

This workflow runs the convert and integrate stages on pull requests. It converts the Sigma rules that changed, writes the alert rules, tests the queries if you enable query testing, commits the generated files to the pull request branch, and posts the results as pull request comments. It also handles [comment commands](../use/comment-commands.md), such as `sigma convert all`.

Call the reusable workflow from your repository:

```yaml
name: Convert and integrate Sigma rules

on:
  pull_request:
    branches:
      - main
    paths:
      - "conversions/*.json"
      - "config.yml"
      - "pipelines/**.yml"
      - "rules*/**.yml"
  issue_comment:
    types: [created]

jobs:
  convert-integrate:
    permissions:
      contents: write # to commit the generated files
      packages: read # to pull the Action Docker image
      pull-requests: write # to comment on the pull request
    uses: grafana/sigma-rule-deployment/.github/workflows/convert-integrate.yml@<COMMIT_SHA> # <RELEASE_TAG>
    with:
      config_path: ./config.yml
      pretty_print: true
    secrets:
      grafana_sa_token: ${{ secrets.GRAFANA_SA_TOKEN }}
```

GitHub runs `issue_comment` workflows from your default branch. Comment commands only work after this workflow is merged to the default branch.

The reusable workflow accepts the following inputs:

| Input                              | Type    | Description                                                                                                                                 | Default               |
| ---------------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- |
| `config_path`                      | string  | Path to the configuration file. Required.                                                                                                   | None                  |
| `plugin_packages`                  | string  | Comma-separated list of extra Sigma CLI plugin packages to install. Only packages that start with `pysigma-` are allowed.                   | `""`                  |
| `render_traceback`                 | boolean | Print full tracebacks when conversion fails.                                                                                                | `false`               |
| `pretty_print`                     | boolean | Indent the generated JSON files.                                                                                                            | `false`               |
| `output_log_lines`                 | boolean | Include log lines in the query test results.                                                                                                | `false`               |
| `all_rules`                        | boolean | Convert and integrate every rule on every pull request run, not only the rules that changed. Comment commands ignore it and pick their own target. | `false`               |
| `actions_username`                 | string  | User name of the automation that commits the generated files. The Actions use it to tell automated commits from human edits.                | `github-actions[bot]` |
| `changed_files_from_base`          | boolean | Detect changed files against the pull request base instead of the last automation commit, so every run converts and integrates everything the pull request changed, not only what changed since the last run. Manual edits are still detected against the last automation commit. | `false`               |
| `continue_on_query_testing_errors` | boolean | Finish integration when query testing fails, and report the errors. Currently not passed on to the integrate Action, so it has no effect, and the Action's default of `true` applies. | `true`                |
| `vault_path`                       | string  | For use within Grafana Labs: read the token from the Grafana Labs internal Vault instead of `grafana_sa_token`.                                    | `""`                  |

It accepts the following secret:

| Secret             | Description                                                                                                                                                 |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `grafana_sa_token` | Token used to test queries, or a JSON object of tokens for each instance. Needs the **Data sources: Reader** role. If you don't pass it, set `test_queries: false`, or query tests fail to authenticate. |

## Deploy

This workflow runs the deploy stage when generated alert rules reach your main branch. It creates, updates, and deletes alert rules in Grafana to match your deployment files, and comments the results on the merged pull request.

```yaml
name: Deploy Sigma rules

on:
  push:
    branches:
      - main
    paths:
      - "deployments/*.json"

jobs:
  deploy:
    permissions:
      contents: read # to read the deployment files
      packages: read # to pull the Action Docker image
      pull-requests: write # to comment on the merged pull request
    uses: grafana/sigma-rule-deployment/.github/workflows/deploy.yml@<COMMIT_SHA> # <RELEASE_TAG>
    with:
      config_path: ./config.yml
    secrets:
      grafana_sa_token: ${{ secrets.GRAFANA_SA_TOKEN }}
```

The reusable workflow accepts the following inputs:

| Input          | Type    | Description                                                                                                                         | Default |
| -------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `config_path`  | string  | Path to the configuration file. Required.                                                                                           | None    |
| `fresh_deploy` | boolean | Delete every alert rule in the configured folders and recreate them from the deployment files. Destructive. Refer to [Deployments](../use/deployments.md#fresh-deployments). | `false` |
| `vault_path`   | string  | For use within Grafana Labs: read the token from the Grafana Labs internal Vault instead of `grafana_sa_token`.                            | `""`    |

It accepts the following secrets:

| Secret                | Description                                                                                                                                                                                       |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `grafana_sa_token`    | Token used to provision alert rules, or a JSON object of tokens for each instance. Needs the alerting roles listed in [Create a Grafana service account token](grafana-service-account.md).       |
| `custom_github_token` | GitHub token for the deploy Action. Currently has no effect: the pull request comments always use the workflow's `GITHUB_TOKEN`. |

## Build your own workflows

The reusable workflows cover most repositories. If you need a different flow, call the Actions directly. Each Action's reference page lists its inputs and outputs:

- [Config Validator](../../actions/validate/README.md)
- [Sigma Rule Converter](../../actions/convert/README.md)
- [Grafana Query Integrator](../../actions/integrate/README.md)
- [Sigma Rule Deployer](../../actions/deploy/README.md)

## Next steps

- [Configuration file](../configure/README.md)
- [Comment commands](../use/comment-commands.md)
- [Deployments](../use/deployments.md)
