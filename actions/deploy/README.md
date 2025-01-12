# Sigma Rule Deployer GitHub Action

**Sigma Rule Deployer** is an experimental GitHub Action that automates the deployment of Grafana alerts based on Sigma rules. This action is part of the Sigma Rule Deployment GitHub Actions Suite and is meant to be used in conjunction with the Sigma Rule Integrator.

## Inputs
| Name               | Description                                                                | Required | Default |
| ------------------ | -------------------------------------------------------------------------- | -------- | ------- |
| `config_path`      | Path to the Sigma Deployer config file.                                    | Yes      | `""`    |
| `grafana_sa_token` | Grafana Service Account token to use for the deployment of the alert rules | Yes      | `""`    |

Note: The token provided in `grafana_sa_token` must have the following permissions:
- Alerting: Rule Reader
- Alerting: Rule Writer
- Alerting: Access to alert rules provisioning API
- Alerting: Set provisioning status

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
- [tj-actions/changed-files v45 by Tonye Jack](https://github.com/tj-actions/changed-files)
- [actions/setup-go v5 by GitHub](https://github.com/actions/setup-go)