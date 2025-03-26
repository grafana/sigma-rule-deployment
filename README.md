# Sigma Rule Deployment GitHub Actions Suite

Automate the conversion, testing, and deployment of Sigma Rules to Grafana with GitHub Actions.

## Available Actions

- [Sigma Rule Converter](./actions/convert/README.md): Converts Sigma rules to target query languages using `sigma-cli`. Supports dynamic plugin installation, custom configurations, and output management, producing a JSON output format that can be used by the integrator.
- [Query Integrator](./actions/integrate/README.md): Given a folder of input query files (as produced by the converter), each file containing a list of queries and relevant metadata, convert each into a Grafana Managed Alerting alert rule, optionally testing the queries against a configured Grafana instance to validate that it works as expected.
- [Rule Deployer](./actions/deploy/README.md): Given a folder of Grafana Managed Alerting alert rules (as produced by the integrator), deploy them to the configured Grafana instance, using Alerting's provisioning API.

## FAQ

### Q: What backends/data sources do you support?

The Actions can load **any** Sigma backend and produce valid alert rules for **any** data source, however, so far we have only thoroughly tested this functionality with Loki. In particular, converting log queries into metric queries so they can be used correctly with Grafana Managed Alerting is dependent on the backend supporting that option or by modifying the generated queries.

### Q: Are there any restrictions on the Sigma rule files?

The only restrictions are they need to be valid Sigma rules, and if you are using Correlation rules, each Correlation rule must contain all the referenced rules within the rule file (using [YAML's multiple document feature](https://gettaurus.org/docs/YAMLTutorial/#YAML-Multi-Documents), i.e., combined with `---`).
