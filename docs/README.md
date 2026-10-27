# Sigma Rule Deployment documentation

Sigma Rule Deployment is a suite of GitHub Actions that converts [Sigma rules](https://sigmahq.io/) into queries, turns those queries into [Grafana Alerting](https://grafana.com/docs/grafana/latest/alerting/) rules, and deploys them to Grafana. You keep your rules and one configuration file in a GitHub repository, and pull requests drive the whole lifecycle.

## Get started

Set up a rule repository and deploy your first alert rule.

- [Get started](get-started.md): create a repository, connect it to Grafana, and deploy your first rules.
- [How Sigma Rule Deployment works](concepts/how-it-works.md): the Detection as Code approach, the pipeline stages, and the files each stage produces.

## Set up

Prepare your Grafana instance and your GitHub repository.

- [Create a Grafana service account token](set-up/grafana-service-account.md): the permissions the Actions need, and tokens for more than one Grafana instance.
- [Organize rules and pipelines](set-up/rules-and-pipelines.md): repository layout and the requirements Sigma rule files must meet.
- [Set up the workflows](set-up/workflows.md): the four workflows that validate, convert, integrate, and deploy your rules.

## Configure

Control how rules are converted, which data source they query, and where they're deployed.

- [Configuration file](configure/README.md): the structure of the configuration file and how defaults and overrides combine.
- [Configuration reference](configure/configuration-reference.md): every setting, its type, and its default.
- [Data sources and query models](configure/data-sources.md): supported backends and data sources, and custom query models.
- [Alert rule labels and annotations](configure/labels-and-annotations.md): template labels and annotations from Sigma rule metadata.
- [Migrate from a v1 configuration](configure/migrate-from-v1.md): convert a v1 configuration file to the v2 format.

## Use

Work with the pipeline day to day.

- [Comment commands](use/comment-commands.md): run conversion and integration from pull request comments.
- [Deployments](use/deployments.md): incremental and fresh deployments, and what happens when you remove rules or instances.
- [Preserve manual modifications](use/manual-modifications.md): keep hand edits to generated files.
- [FAQ](faq.md): answers to common questions.

## Action reference

Each Action documents its inputs and outputs next to its code. Use these pages when you build your own workflows instead of the reusable ones.

- [Config Validator](../actions/validate/README.md)
- [Sigma Rule Converter](../actions/convert/README.md)
- [Grafana Query Integrator](../actions/integrate/README.md)
- [Sigma Rule Deployer](../actions/deploy/README.md)
