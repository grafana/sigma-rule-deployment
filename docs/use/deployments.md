# Deployments

The deploy stage keeps the alert rules in Grafana in step with the alert rule files in your repository. This page explains the two deployment modes, which Grafana instances each alert rule goes to, and what happens when you remove rules, configurations, or instances.

## Incremental deployments

By default, the [deploy workflow](../set-up/workflows.md#deploy) runs after every push to your main branch that changes the alert rule files. It only processes the files that changed in the push: it creates alert rules for new files, updates them for changed files, and deletes them for removed files.

When the deployment finishes, the workflow comments the created, updated, and deleted alert rules on the pull request that was merged. If an alert rule fails to deploy, the deployment stops at that rule, and the comment reports the error.

## Fresh deployments

A fresh deployment deletes every alert rule in the Grafana folders your configurations use, and then creates alert rules from every file in the deployment folder. Use it to recover from drift, for example, after someone edited or deleted the alert rules in Grafana by hand.

> [!WARNING]
> A fresh deployment deletes every alert rule in the configured folders, including alert rules that Sigma Rule Deployment didn't create. Give Sigma Rule Deployment its own Grafana folders.

Run fresh deployments from a manually triggered workflow, not on every push:

```yaml
name: Fresh deploy Sigma rules

on:
  workflow_dispatch:

jobs:
  deploy:
    permissions:
      contents: read
      id-token: write
      packages: read
      pull-requests: write
    uses: grafana/sigma-rule-deployment/.github/workflows/deploy.yml@<COMMIT_SHA> # <RELEASE_TAG>
    with:
      config_path: ./config.yml
      fresh_deploy: true
    secrets:
      grafana_sa_token: ${{ secrets.GRAFANA_SA_TOKEN }}
```

When configurations deploy to different instances, a fresh deployment cleans each instance separately. It only deletes alert rules in the `folder_id` and `org_id` of the configurations that deploy to that instance. A default instance that every configuration overrides isn't touched.

## Which instances an alert rule deploys to

Each configuration must resolve at least one Grafana instance, from its own `deployment.grafana_instance` or from `defaults.deployment.grafana_instance`. The deployment fails if a configuration resolves none, instead of skipping its alert rules. `grafana_instance` can be a list, to deploy the same alert rules to several instances.

An alert rule file that matches no configuration deploys to the default instances.

## Remove rules, configurations, and instances

When you remove an alert rule file, the deploy stage deletes its alert rule from every instance the configuration deployed it to. It reads those instances from the configuration file as it was before the push. So if you remove a configuration, or change its `grafana_instance`, in the same change as its alert rule files, the old instances are still cleaned up.

If an instance that's only in the old configuration is no longer reachable, for example, because it was shut down, the deploy stage logs a warning and carries on. An unreachable instance that's still in the configuration fails the deployment.

There's no previous configuration to read on a first deployment, a manually triggered run, after a force-push, or when the previous file isn't a valid v2 configuration. In those cases, the deploy stage says so and deletes the alert rule from the instances in the current configuration instead.

When you remove a configuration, the integrate stage removes its conversion and alert rule files from the pull request, so the deploy stage deletes its alert rules after you merge. Removing an instance from `grafana_instance` doesn't remove any files, so it doesn't delete anything from that instance. To clean it up, run a fresh deployment or delete the alert rules in Grafana.

## Trace a deployed alert rule

Every alert rule carries a `SigmaConfiguration` annotation with the name of the configuration that produced it, and annotations with the paths of its Sigma rule, conversion, and alert rule files. Refer to [Built-in annotations](../configure/labels-and-annotations.md#built-in-annotations).

## Related pages

- [Set up the workflows](../set-up/workflows.md)
- [Use more than one Grafana instance](../set-up/grafana-service-account.md#use-more-than-one-grafana-instance)
- [Sigma Rule Deployer](../../actions/deploy/README.md) reference
