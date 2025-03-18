import json
import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from dynaconf.utils import DynaconfDict

from .convert import convert_rules, is_path, is_safe_path, load_rule


@pytest.fixture
def mock_config():
    """Mock configuration object."""
    return DynaconfDict(
        {
            "defaults": {
                "target": "loki",
                "format": "default",
                "skip-unsupported": "true",
                "file-pattern": "*.yml",
            },
            "conversions": [
                {
                    "name": "test_conversion",
                    "input": ["rules/*.yml"],
                    "target": "loki",
                    "format": "default",
                }
            ],
        }
    )


@pytest.fixture
def temp_workspace(tmp_path):
    """Create a temporary workspace with a rules directory."""
    workspace = tmp_path / "workspace"
    workspace.mkdir()
    rules_dir = workspace / "rules"
    rules_dir.mkdir()
    test_rule = rules_dir / "test.yml"
    test_rule_src = Path("test.yml")
    # Copy the test rule to the rules directory
    with (
        open(test_rule, "w", encoding="utf-8") as f,
        open(test_rule_src, "r", encoding="utf-8") as src,
    ):
        f.write(src.read())
    return workspace


def test_convert_rules_missing_path_prefix():
    """Test that an error is raised when path prefix is not set."""
    with pytest.raises(ValueError, match="Path prefix must be set"):
        convert_rules(config=DynaconfDict(), path_prefix="")


def test_convert_rules_invalid_output_dir(temp_workspace, mock_config):
    """Test that an error is raised when output directory is outside the project root."""
    with pytest.raises(ValueError, match="outside the project root"):
        convert_rules(
            config=mock_config,
            path_prefix=temp_workspace,
            conversions_output_dir="../outside",
        )


def test_convert_rules_missing_conversion_name(mock_config):
    """Test that an error is raised when conversion name is missing."""
    invalid_config = DynaconfDict(
        {"conversions": [{"input": ["rules/*.yml"], "target": "loki"}]}
    )
    with pytest.raises(
        ValueError,
        match=(
            "Conversion name is required and must be a unique identifier"
            " across all conversion objects in the config"
        ),
    ):
        convert_rules(config=invalid_config, path_prefix="/tmp")


def test_convert_rules_absolute_input_path(mock_config):
    """Test that an error is raised when input file pattern is absolute."""
    invalid_config = DynaconfDict(
        {
            "conversions": [
                {"name": "test", "input": ["/absolute/path/*.yml"], "target": "loki"}
            ]
        }
    )
    with pytest.raises(ValueError, match="must be relative"):
        convert_rules(config=invalid_config, path_prefix="/tmp")


@pytest.mark.parametrize(
    "base_dir,target_path,expected",
    [
        ("/tmp", "/tmp/file.txt", True),
        ("/tmp", "/tmp/subdir/file.txt", True),
        ("/tmp", "/etc/file.txt", False),
        ("/tmp", "../outside.txt", False),
    ],
)
def test_is_safe_path(base_dir, target_path, expected):
    """Test that is_safe_path returns the expected result."""
    result = is_safe_path(base_dir, target_path)
    assert result == expected


@pytest.mark.parametrize(
    "path_string,file_pattern,expected",
    [
        ("existing.yml", "*.yml", True),
        ("/absolute/path.yml", "*.yml", True),
        ("relative/path.yml", "*.yml", True),
        ("not_a_path", "*.yml", False),
        ("test.yml", "*.json", False),
    ],
)
def test_is_path(path_string, file_pattern, expected):
    """Test that is_path returns the expected result."""
    with patch("os.path.exists") as mock_exists:
        mock_exists.return_value = path_string == "existing.yml"
        result = is_path(path_string, file_pattern)
        assert result == expected


def test_convert_rules_successful_conversion(temp_workspace, mock_config):
    """Test that convert_rules successfully converts Sigma rules."""
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        conversions_output_dir="conversions",
    )

    output_file = temp_workspace / "conversions" / "test_conversion.json"
    assert output_file.exists()
    assert output_file.read_text() == json.dumps(
        [
            {
                "query": '{job=~".+"} | logfmt | userIdentity_type=~`(?i)^Root$` and eventType!~`(?i)^AwsServiceEvent$`',
                "conversion_name": "test_conversion",
                "rule": {
                    "title": "AWS Root Credentials",
                    "description": "Detects AWS root account usage",
                    "logsource": {"product": "aws", "service": "cloudtrail"},
                    "detection": {
                        "selection": {"userIdentity.type": "Root"},
                        "filter": {"eventType": "AwsServiceEvent"},
                        "condition": "selection and not filter",
                    },
                    "falsepositives": ["AWS Tasks That Require Root User Credentials"],
                    "level": "medium",
                },
            }
        ]
    )


def test_convert_rules_successful_conversion_on_rule(temp_workspace, mock_config):
    """Test that convert_rules successfully converts a Sigma rule."""
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        conversions_output_dir="conversions",
    )

    output_file = temp_workspace / "conversions" / "test_conversion.json"
    assert output_file.exists()
    assert output_file.read_text() == json.dumps(
        [
            {
                "query": '{job=~".+"} | logfmt | userIdentity_type=~`(?i)^Root$` and eventType!~`(?i)^AwsServiceEvent$`',
                "conversion_name": "test_conversion",
                "rule": {
                    "title": "AWS Root Credentials",
                    "description": "Detects AWS root account usage",
                    "logsource": {"product": "aws", "service": "cloudtrail"},
                    "detection": {
                        "selection": {"userIdentity.type": "Root"},
                        "filter": {"eventType": "AwsServiceEvent"},
                        "condition": "selection and not filter",
                    },
                    "falsepositives": ["AWS Tasks That Require Root User Credentials"],
                    "level": "medium",
                },
            }
        ]
    )


@patch("click.testing.CliRunner.invoke")
def test_convert_rules_handles_empty_output(mock_invoke, temp_workspace, mock_config):
    """Test that convert_rules handles empty output."""
    mock_result = MagicMock()
    mock_result.exception = None
    mock_result.exc_info = None
    mock_result.exit_code = 0
    mock_result.stdout = "Parsing Sigma rules\n"
    mock_invoke.return_value = mock_result

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        conversions_output_dir="conversions",
    )

    output_file = temp_workspace / "conversions" / "test_conversion.json"
    assert not output_file.exists()


def test_convert_rules_handles_empty_output_on_rule(temp_workspace, mock_config):
    """Test that convert_rules handles empty output on a rule."""

    # Create a test rule with empty content
    test_rule = temp_workspace / "rules" / "test.yml"
    test_rule.write_text("")

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        conversions_output_dir="conversions",
    )

    output_file = temp_workspace / "conversions" / "test_conversion.json"
    assert not output_file.exists()


def test_load_rule_valid_yaml():
    """Test loading a valid YAML rule file."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yml", delete=False) as f:
        f.write(
            """
title: Test Rule
description: Test description
status: test
level: low
logsource:
    category: test
detection:
    selection:
        field: value
    condition: selection
        """
        )
        f.flush()

        result = load_rule(f.name)

    # Clean up the temporary file
    os.unlink(f.name)

    assert isinstance(result, dict)
    assert result["title"] == "Test Rule"
    assert result["description"] == "Test description"
    assert result["status"] == "test"
    assert result["level"] == "low"
    assert "logsource" in result
    assert "detection" in result


def test_load_rule_invalid_yaml():
    """Test loading an invalid YAML file raises ValueError."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yml", delete=False) as f:
        f.write(
            """
title: Invalid Rule
description: Invalid YAML
    wrong:
      indentation:
    - not valid yaml
        """
        )
        f.flush()

        with pytest.raises(ValueError) as exc_info:
            load_rule(f.name)

    # Clean up the temporary file
    os.unlink(f.name)

    assert "Error loading rule file" in str(exc_info.value)


def test_load_rule_nonexistent_file():
    """Test loading a non-existent file raises ValueError."""
    with pytest.raises(ValueError) as exc_info:
        load_rule("nonexistent_file.yml")

    assert "Error loading rule file" in str(exc_info.value)


def test_load_rule_empty_file():
    """Test loading an empty file."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yml", delete=False) as f:
        f.write("")
        f.flush()

        result = load_rule(f.name)

    # Clean up the temporary file
    os.unlink(f.name)

    assert result is None
