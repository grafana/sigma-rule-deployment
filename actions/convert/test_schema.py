"""Checks that the v2 JSON schema accepts the configuration files shipped with the repository,
and that it rejects (or accepts) the cases the Go and Python code depend on."""

import copy
import json
from pathlib import Path
from typing import Any

import pytest
import yaml
from jsonschema import Draft202012Validator

REPO_ROOT = Path(__file__).resolve().parents[2]
SCHEMA = json.loads((REPO_ROOT / "config" / "schema-v2.json").read_text())
VALIDATOR = Draft202012Validator(SCHEMA)

# Configuration files that must stay valid: the documented example, and the files the Go tests load
VALID_CONFIGS = [
    REPO_ROOT / "config" / "config-example-v2.yml",
    REPO_ROOT / "integration-test" / "config.yml",
    REPO_ROOT / "internal" / "deploy" / "test_config.yml",
    *sorted((REPO_ROOT / "internal" / "integrate" / "testdata").glob("*.yml")),
]

BASE: dict[str, Any] = {
    "version": 2,
    "folders": {"conversion_path": "c", "deployment_path": "d"},
    "configurations": [{"name": "a", "conversion": {"input": "rules/*"}}],
}


def with_changes(**top_level: Any) -> dict[str, Any]:
    config = copy.deepcopy(BASE)
    config.update(top_level)
    return config


def with_conversion(**settings: Any) -> dict[str, Any]:
    config = copy.deepcopy(BASE)
    config["configurations"][0]["conversion"].update(settings)
    return config


def test_schema_is_valid() -> None:
    Draft202012Validator.check_schema(SCHEMA)


@pytest.mark.parametrize("path", VALID_CONFIGS, ids=lambda p: str(p.relative_to(REPO_ROOT)))
def test_shipped_configs_are_valid(path: Path) -> None:
    errors = [e.message for e in VALIDATOR.iter_errors(yaml.safe_load(path.read_text()))]
    assert errors == []


def test_minimal_config_is_valid() -> None:
    # folder_id, org_id and grafana_instance are resolved (and enforced) by the Go code, not the schema
    assert VALIDATOR.is_valid(BASE)


@pytest.mark.parametrize(
    "config",
    [
        with_conversion(bogus=1),
        with_conversion(data_source_type="elasticsearch"),  # belongs under integration
        with_changes(defaults={"conversion": {"bogus": 1}}),
        with_changes(defaults={"conversion": {"input": "rules/*"}}),  # input is per configuration
        with_changes(defaults={"conversion": {"skip_unsupported": "true"}}),  # must be a boolean
        with_changes(defaults={"integration": {"data_source_type": 1}}),
    ],
)
def test_unknown_or_mistyped_settings_are_rejected(config: dict[str, Any]) -> None:
    assert not VALIDATOR.is_valid(config)


def test_every_conversion_setting_is_accepted() -> None:
    config = with_conversion(
        target="loki",
        format="default",
        skip_unsupported=True,
        fail_unsupported=False,
        file_pattern="*.yml",
        encoding="utf-8",
        pipeline_check=False,
        without_pipeline=False,
        pipelines=["p.yml"],
        filters=["f.yml"],
        backend_options={"case_sensitive": True},
        correlation_method="temporal",
        json_indent=2,
        verbose=True,
        required_rule_fields=["id"],
    )
    assert VALIDATOR.is_valid(config)


@pytest.mark.parametrize(
    "duration", ["30s", "5m", "1h30m", "1.5h", "500ms", "1d", "7d", "1d12h", "2d1h5s"]
)
def test_durations_accepted_by_go_are_valid(duration: str) -> None:
    config = with_changes(
        defaults={
            "integration": {"time_window": duration, "lookback": duration},
            "deployment": {"timeout": duration},
        }
    )
    assert VALIDATOR.is_valid(config)


@pytest.mark.parametrize("duration", ["", "d", "1", "1x", "1h1d", "-1d", "1.5d", "1d1d"])
def test_durations_rejected_by_go_are_invalid(duration: str) -> None:
    for setting in (
        {"integration": {"time_window": duration}},
        {"integration": {"lookback": duration}},
        {"deployment": {"timeout": duration}},
    ):
        assert not VALIDATOR.is_valid(with_changes(defaults=setting))
