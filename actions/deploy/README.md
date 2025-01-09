# Sigma Rule Deployer GitHub Action

**Sigma Rule Deployer** is a GitHub Action that automates the deployment of Grafana alerts based on Sigma rules. This action is part of the Sigma Rule Deployment GitHub Actions Suite.

## Inputs

## Outputs

## Usage
This action is intended to be used in a workflow that is triggered by a push event to the main branch of the repository.

## Notes
This is a composite action relying on the following external actions:
- [actions/checkout v4 by GitHub](https://github.com/actions/checkout)
- [tj-actions/changed-files v45 by Tonye Jack](https://github.com/tj-actions/changed-files)
- [actions/setup-go v5 by GitHub](https://github.com/actions/setup-go)