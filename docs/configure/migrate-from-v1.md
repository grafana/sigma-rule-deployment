# Migrate from a v1 configuration

The Actions only support version 2 of the configuration file. The v1 format, with top-level `conversion_defaults`, `conversions`, `integration`, and `deployment` keys, no longer works. This page shows how to convert a v1 file.

## Before you begin

You need Python 3 with the `pyyaml` package, and a local copy of this repository for the migration script.

## What changes

The v2 format groups every setting by pipeline stage:

- `conversion_defaults` splits into `defaults.conversion` and `defaults.integration`.
- The top-level `integration` block moves to `defaults.integration`. Where it sets the same setting as `conversion_defaults`, its value wins.
- In each `conversions` entry, the integration settings `rule_group`, `time_window`, `lookback`, `data_source`, `data_source_type`, and `query_model` move into an `integration` block.
- `conversions` is renamed to `configurations`.
- The top-level `deployment` block moves to `defaults.deployment`.
- `folders` stays the same.
- `version: 2` is added.

## Migrate the file

The [migration script](../../scripts/migrate-config-v1-to-v2/) converts a v1 file for you.

1. Preview the result:

   ```sh
   python scripts/migrate-config-v1-to-v2/migrate_v1_to_v2.py <V1_CONFIG_FILE>
   ```

1. Write the result to a new file:

   ```sh
   python scripts/migrate-config-v1-to-v2/migrate_v1_to_v2.py <V1_CONFIG_FILE> <V2_CONFIG_FILE>
   ```

1. Copy any comments from the old file. The script doesn't keep YAML comments.
1. Replace the old file in your rule repository and update any workflows that pointed to it.

If the input is already a v2 file, the script exits with an error and doesn't write anything.

You don't need a local copy of the script if you run the [validation workflow](../set-up/workflows.md#validate-the-configuration). When it finds a v1 file, it fails and prints a suggested v2 file in the log. On a pull request, if you pass the Config Validator a `github_token` and grant the job `pull-requests: write`, it posts the suggestion as a pull request comment instead.

## Result

The [validation workflow](../set-up/workflows.md#validate-the-configuration) passes, and the next convert and integrate run uses the new file.

## Next steps

- [Configuration file](README.md)
- [Configuration reference](configuration-reference.md)
