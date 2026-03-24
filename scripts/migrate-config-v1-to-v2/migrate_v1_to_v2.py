#!/usr/bin/env python3
"""Migrate a v1 sigma-rule-deployment config file to v2 format.

Usage:
    python migrate_v1_to_v2.py <input.yml> [output.yml]

If output.yml is omitted, the result is written to stdout.
"""

import argparse
import sys
from pathlib import Path

import yaml

# Fields that stay in the conversion block in v2
_CONVERSION_FIELDS = {
    "target", "format", "skip_unsupported", "fail_unsupported", "file_pattern",
    "encoding", "pipeline_check", "without_pipeline", "pipelines", "filters",
    "backend_options", "correlation_method", "json_indent", "verbose",
    "data_source_type", "required_rule_fields",
}

# Fields from a v1 conversion item (or conversion_defaults) that move to integration in v2
_INTEGRATION_FIELDS_FROM_CONVERSION = {
    "rule_group", "time_window", "lookback", "data_source", "query_model",
}


def migrate(v1: dict) -> dict:
    if v1.get("version") == 2:
        raise ValueError("config is already v2")

    v2: dict = {"version": 2}

    if "folders" in v1:
        v2["folders"] = v1["folders"]

    # Build defaults block
    defaults: dict = {}

    # Split conversion_defaults: pure conversion fields stay, integration-bound fields move
    if "conversion_defaults" in v1:
        cd = v1["conversion_defaults"]
        conv_defaults = {k: v for k, v in cd.items() if k in _CONVERSION_FIELDS}
        if conv_defaults:
            defaults["conversion"] = conv_defaults
        intg_from_cd = {k: v for k, v in cd.items() if k in _INTEGRATION_FIELDS_FROM_CONVERSION}
    else:
        intg_from_cd = {}

    # Merge: v1 integration block takes precedence over anything lifted from conversion_defaults
    intg_defaults: dict = {}
    intg_defaults.update(intg_from_cd)
    intg_defaults.update(v1.get("integration", {}))
    if intg_defaults:
        defaults["integration"] = intg_defaults

    if "deployment" in v1:
        defaults["deployment"] = v1["deployment"]

    if defaults:
        v2["defaults"] = defaults

    # conversions[] → configurations[], splitting each item into conversion/integration sub-blocks
    configurations = []
    for item in v1.get("conversions", []):
        conv = {"input": item["input"]}
        conv.update({k: v for k, v in item.items() if k in _CONVERSION_FIELDS})

        intg = {k: v for k, v in item.items() if k in _INTEGRATION_FIELDS_FROM_CONVERSION}

        config_item: dict = {"name": item["name"], "conversion": conv}
        if intg:
            config_item["integration"] = intg
        configurations.append(config_item)

    v2["configurations"] = configurations

    return v2


def main() -> None:
    parser = argparse.ArgumentParser(description="Migrate a v1 config file to v2 format")
    parser.add_argument("input", help="Path to v1 config YAML file")
    parser.add_argument("output", nargs="?", help="Output path (default: stdout)")
    args = parser.parse_args()

    input_path = Path(args.input)
    if not input_path.is_file():
        print(f"Error: {input_path} not found", file=sys.stderr)
        sys.exit(1)

    with open(input_path) as f:
        v1 = yaml.safe_load(f)

    try:
        v2 = migrate(v1)
    except ValueError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    output = yaml.dump(v2, default_flow_style=False, sort_keys=False, allow_unicode=True)

    if args.output:
        Path(args.output).write_text(output)
        print(f"Written to {args.output}")
    else:
        sys.stdout.write(output)


if __name__ == "__main__":
    main()
