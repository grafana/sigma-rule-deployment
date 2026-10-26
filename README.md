# Sigma Rule Deployment

Sigma Rule Deployment is a suite of GitHub Actions that turns [Sigma rules](https://sigmahq.io/) into [Grafana Alerting](https://grafana.com/docs/grafana/latest/alerting/) rules. Keep your detections and one configuration file in a GitHub repository, and the Actions convert, test, and deploy them through pull requests: Detection as Code for Grafana.

- **Convert** Sigma rules into queries for Loki, Elasticsearch, Splunk, and many more data sources with Sigma CLI.
- **Test** every query against your data source and review the results on the pull request before anything reaches Grafana.
- **Deploy** the alert rules to one or more Grafana instances when you merge, and clean them up when you remove a rule.
- **Control** each run from pull request comments, such as `sigma convert all`.

## What is Detection as Code?

Detection as Code means managing security detections the way you manage software. Rules live as readable files in Git, every change goes through review, and automation deploys them, so you always know which detections run in production, who changed them, and why.

Sigma Rule Deployment brings this to Grafana. [Sigma](https://sigmahq.io/) gives you a vendor-neutral rule format with [thousands of community rules](https://github.com/SigmaHQ/sigma), and the Actions turn those rules into alert rules for your data source. Refer to [How Sigma Rule Deployment works](docs/concepts/how-it-works.md) for the full picture.

## Get started

Follow [Get started](docs/get-started.md) to set up a rule repository and deploy your first alert rules.

## Documentation

The [documentation](docs/README.md) covers setup, configuration, and day-to-day use, including:

- [How Sigma Rule Deployment works](docs/concepts/how-it-works.md)
- [Set up the workflows](docs/set-up/workflows.md)
- [Configuration reference](docs/configure/configuration-reference.md)
- [Data sources and query models](docs/configure/data-sources.md)
- [FAQ](docs/faq.md)

## Actions

| Action                                                  | What it does                                                                     |
| ------------------------------------------------------- | -------------------------------------------------------------------------------- |
| [Config Validator](actions/validate/README.md)          | Validates the configuration file against the JSON schema.                       |
| [Sigma Rule Converter](actions/convert/README.md)       | Converts Sigma rules into queries with Sigma CLI.                               |
| [Grafana Query Integrator](actions/integrate/README.md) | Builds Grafana alert rules from the converted queries and tests them.           |
| [Sigma Rule Deployer](actions/deploy/README.md)         | Deploys the alert rules to Grafana.                                              |

Most repositories use the reusable workflows that combine these Actions. Refer to [Set up the workflows](docs/set-up/workflows.md).

## Contributing

To develop the Actions themselves, refer to [CONTRIBUTING.md](CONTRIBUTING.md). To report a bug or request a feature, [open an issue](https://github.com/grafana/sigma-rule-deployment/issues).

## License

Sigma Rule Deployment is licensed under the [Apache License 2.0](LICENSE).
