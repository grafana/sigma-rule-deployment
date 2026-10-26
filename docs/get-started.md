# Get started with Sigma Rule Deployment

This guide sets up a GitHub repository that deploys Sigma rules to Grafana Alerting. When you're done, every pull request converts and tests your rules, and every merge deploys them to Grafana as alert rules.

## Before you begin

You need:

- A GitHub account that can create repositories and add Actions secrets.
- A Grafana instance, in [Grafana Cloud](https://grafana.com/docs/grafana-cloud/get-started/) or self-managed, where you can create service accounts and alert rules. A Grafana Cloud free account works.
- A data source in that instance that holds the logs your rules search, for example, Loki.
- Some familiarity with GitHub Actions and YAML.

## Create the repository

Create a GitHub repository for your rules, or use an existing one. It becomes the single source of truth for your detections: you change rules through pull requests, and the workflows deploy what's on the main branch.

## Create a service account token

Create a Grafana service account with the alerting and data source permissions the Actions need. Then store its token as a repository secret named `GRAFANA_SA_TOKEN`.

For the required roles and step-by-step instructions, refer to [Create a Grafana service account token](set-up/grafana-service-account.md).

## Add rules and pipelines

Add your Sigma rules to a `rules/` folder, and any Sigma pipelines to a `pipelines/` folder. Each rule needs a UUID `id` and a `title`. If you don't have rules yet, start with a few from the [SigmaHQ rule repository](https://github.com/SigmaHQ/sigma/tree/master/rules) that match a log source you collect.

For the recommended layout and the rule requirements, refer to [Organize rules and pipelines](set-up/rules-and-pipelines.md).

## Create the configuration file

Create a `config.yml` file in the root of the repository. The following example converts every rule in `rules/okta/` for Loki, tests the queries, and deploys the alert rules to one Grafana instance:

```yaml
version: 2
folders:
  conversion_path: ./conversions
  deployment_path: ./deployments
defaults:
  conversion:
    target: loki
  integration:
    data_source: <DATA_SOURCE_UID>
    folder_id: <FOLDER_UID>
    org_id: 1
    rule_group: Every 5 Minutes
    time_window: 5m
    test_queries: true
  deployment:
    grafana_instance: https://<STACK_NAME>.grafana.net
configurations:
  - name: okta_audit
    conversion:
      input: rules/okta/*.yml
      pipelines:
        - pipelines/okta.yml
```

Replace the placeholders:

- _DATA_SOURCE_UID_ is the UID of the data source to query. Refer to [Find a data source UID](configure/data-sources.md#find-a-data-source-uid).
- _FOLDER_UID_ is the UID of the Grafana folder for the alert rules. It's the last part of the folder's URL in Grafana. Use a folder that only Sigma Rule Deployment writes to.
- _STACK_NAME_ is the name of your Grafana Cloud stack, or use the URL of your own Grafana instance.

For everything the file can do, refer to [Configuration file](configure/README.md).

## Add the workflows

Add the workflows that validate, convert and integrate, and deploy your rules. Copy the templates from the [`config/`](../config/) folder of this repository into `.github/workflows/` in your repository, and pin them to a release.

For each workflow and its inputs, refer to [Set up the workflows](set-up/workflows.md).

Merge the workflows to your main branch before you continue. Comment commands only work once the convert and integrate workflow is on the default branch.

## Open a pull request

Create a branch, add or change a Sigma rule, and open a pull request.

The convert and integrate workflow runs and commits the generated files to your branch in a "Sigma Rules Deployment" commit. The convert and integrate stages also comment on the pull request with links to the generated files and, with query testing on, the query test results.

To convert every rule, not only the ones you changed, comment `sigma convert all` on the pull request. Refer to [Comment commands](use/comment-commands.md).

## Merge and deploy

When you're happy with the results, merge the pull request. The deploy workflow provisions the alert rules to Grafana and comments the outcome on the pull request.

## Result

In Grafana, go to **Alerting** > **Alert rules**. Your alert rules are in the folder you configured, with annotations that link each one back to its Sigma rule.

## Next steps

- Set up [notifications](https://grafana.com/docs/grafana/latest/alerting/configure-notifications/) for the folder or rule groups, or connect them to [Grafana IRM](https://grafana.com/docs/grafana-cloud/alerting-and-irm/irm/) to manage on-call rotations.
- Add [labels from Sigma rule metadata](configure/labels-and-annotations.md) to route alerts by severity or team.
- Learn [how Sigma Rule Deployment works](concepts/how-it-works.md).
