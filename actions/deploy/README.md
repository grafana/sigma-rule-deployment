# Sigma Rule Deployer GitHub Action

**Sigma Rule Deployer** is an experimental GitHub Action that automates the deployment of Grafana alerts based on Sigma rules. This action is part of the Sigma Rule Deployment GitHub Actions Suite and is meant to be used in conjunction with the Sigma Rule Integrator, although it can work independently given the proper alert configuration files.

## Inputs
| Name               | Description                                                                            | Required | Default |
| ------------------ | -------------------------------------------------------------------------------------- | -------- | ------- |
| `config_path`      | Path to the Sigma Deployer config file.                                                | Yes      | `""`    |
| `grafana_sa_token` | Grafana Service Account token to use for the deployment of the alert rules             | Yes      | `""`    |
| `fresh_deploy`     | Whether to perform a fresh deployment or not (see below). Warning: destructive action! | No       | `false` |

Note: The token provided in `grafana_sa_token` must have the following permissions:
- Alerting: Rule Reader
- Alerting: Rule Writer
- Alerting: Access to alert rules provisioning API
- Alerting: Set provisioning status

A fresh deployment (`fresh_deploy`) will delete all existing alert rules in the Grafana Alert folder specified in the config file and then create all the alerts existing in the deployment folder. This is therefore a destructive action and should be used with caution. It is meant to be used when the alerts are to be re-deployed from scratch after a deployment drift. The advised way of using this mode is via a manually triggered workflow. Ensure a dedicated Grafana Alert folder is used for this purpose.

## Outputs
| Name             | Description                                                                |
| ---------------- | -------------------------------------------------------------------------- |
| `alerts_created` | List of the UIDs of the alerts created during deployment (space-separated) |
| `alerts_updated` | List of the UIDs of the alerts updated during deployment (space-separated) |
| `alerts_deleted` | List of the UIDs of the alerts deleted during deployment (space-separated) |
   

## Usage
This action is intended to be used in a workflow that is triggered by a push event to the main branch of the repository.
It is expected that the Sigma Rule Converter and Sigma Rule Integrator actions have been run in the PR that is being merged to the main branch.

This is an example of a workflow:

```yaml
name: Deploy Sigma rules

on:
  push:
    branches:
      - main

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
        - name: Deploy Sigma rules
          id: deploy
          uses: grafana/sigma-rule-deployment/actions/deploy@v1
          with:
            config_file: ./sigma_rule_config.yml
            grafana_sa_token: ${{ secrets.GRAFANA_SA_TOKEN }}
```

## Notes
This is a composite action relying on the following external actions:
- [actions/checkout v4 by GitHub](https://github.com/actions/checkout)
- [grafana/changed-files](https://github.com/grafana/changed-files)
- [actions/setup-go v5 by GitHub](https://github.com/actions/setup-go)
