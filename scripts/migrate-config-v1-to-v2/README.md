# migrate-config-v1-to-v2

Migrates a v1 `sigma-rule-deployment` configuration file to the v2 format.

## What it does

- Splits `conversion_defaults` into separate `defaults.conversion` and `defaults.integration` sub-blocks
- Moves integration-bound fields (`rule_group`, `time_window`, `lookback`, `data_source`, `query_model`) out of each `conversions[]` item into an `integration` sub-block
- Renames `conversions[]` to `configurations[]`
- Moves the top-level `deployment` block to `defaults.deployment`
- Preserves `folders` unchanged

## Usage

```bash
python migrate_v1_to_v2.py <input.yml> [output.yml]
```

If `output.yml` is omitted the result is written to stdout.

```bash
# Preview migration
python migrate_v1_to_v2.py config.yml

# Write to a new file
python migrate_v1_to_v2.py config.yml config-v2.yml
```

## Running tests

```bash
pip install pytest pyyaml
pytest test_migrate.py -v
```

## Notes

- YAML comments in the original file are not preserved in the output.
- The script exits with a non-zero status and an error message if the input is already v2.
- This script is also invoked automatically by the `actions/validate` composite action when a v1 config is detected, to generate a suggested migration in the PR comment.
