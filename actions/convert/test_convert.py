import orjson as json
import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from dynaconf.utils import DynaconfDict

from .convert import convert_rules, is_path, is_safe_path, load_rules


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
def mock_config_with_correlation_rule():
    """Mock configuration object with a correlation rule."""
    return DynaconfDict(
        {
            "defaults": {
                "target": "loki",
                "format": "default",
                "skip-unsupported": "true",
                "file-pattern": "*.yml",
                "encoding": "utf-8",
            },
            "conversions": [
                {
                    "name": "test_conversion_with_correlation_rule",
                    "input": ["rules/correlation.yml"],
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


@pytest.fixture
def temp_workspace_with_correlation_rule(tmp_path):
    """Create a correlation rule file."""
    workspace = tmp_path / "workspace"
    workspace.mkdir()
    rules_dir = workspace / "rules"
    rules_dir.mkdir()
    correlation_rule = rules_dir / "correlation.yml"
    correlation_rule_src = Path("test_correlation.yml")
    with (
        open(correlation_rule, "w", encoding="utf-8") as f,
        open(correlation_rule_src, "r", encoding="utf-8") as src,
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
                "queries": [
                    '{job=~".+"} | logfmt | userIdentity_type=~`(?i)^Root$` and eventType!~`(?i)^AwsServiceEvent$`'
                ],
                "conversion_name": "test_conversion",
                "input_file": "rules/test.yml",
                "rules": [
                    {
                        "title": "AWS Root Credentials",
                        "description": "Detects AWS root account usage",
                        "logsource": {"product": "aws", "service": "cloudtrail"},
                        "detection": {
                            "selection": {"userIdentity.type": "Root"},
                            "filter": {"eventType": "AwsServiceEvent"},
                            "condition": "selection and not filter",
                        },
                        "falsepositives": [
                            "AWS Tasks That Require Root User Credentials"
                        ],
                        "level": "medium",
                    }
                ],
                "output_file": "conversions/test_conversion.json",
            }
        ]
    ).decode("utf-8", "replace")


def test_convert_rules_successful_conversion_with_correlation_rule(
    temp_workspace_with_correlation_rule, mock_config_with_correlation_rule
):
    """Test that convert_rules successfully converts a Sigma correlation rule."""
    convert_rules(
        config=mock_config_with_correlation_rule,
        path_prefix=temp_workspace_with_correlation_rule,
        conversions_output_dir="conversions",
    )

    output_file = (
        temp_workspace_with_correlation_rule
        / "conversions"
        / "test_conversion_with_correlation_rule.json"
    )
    assert output_file.exists()
    assert output_file.read_text() == json.dumps(
        [
            {
                "queries": [
                    'sum by (userIdentity_arn) (count_over_time({job=~".+"} | logfmt | eventSource=~`(?i)^s3\\.amazonaws\\.com$` and eventName=~`(?i)^ListBuckets$` and userIdentity_type!~`(?i)^AssumedRole$` [1h])) >= 100'
                ],
                "conversion_name": "test_conversion_with_correlation_rule",
                "input_file": "rules/correlation.yml",
                "rules": [
                    {
                        "title": "Potential Bucket Enumeration on AWS",
                        "id": "f305fd62-beca-47da-ad95-7690a0620084",
                        "related": [
                            {
                                "id": "4723218f-2048-41f6-bcb0-417f2d784f61",
                                "type": "similar",
                            }
                        ],
                        "status": "test",
                        "description": "Looks for potential enumeration of AWS buckets via ListBuckets.",
                        "references": [
                            "https://github.com/Lifka/hacking-resources/blob/c2ae355d381bd0c9f0b32c4ead049f44e5b1573f/cloud-hacking-cheat-sheets.md",
                            "https://jamesonhacking.blogspot.com/2020/12/pivoting-to-private-aws-s3-buckets.html",
                            "https://securitycafe.ro/2022/12/14/aws-enumeration-part-ii-practical-enumeration/",
                        ],
                        "author": "Christopher Peacock @securepeacock, SCYTHE @scythe_io",
                        "date": "2023-01-06",
                        "modified": "2024-07-10",
                        "tags": ["attack.discovery", "attack.t1580"],
                        "logsource": {"product": "aws", "service": "cloudtrail"},
                        "detection": {
                            "selection": {
                                "eventSource": "s3.amazonaws.com",
                                "eventName": "ListBuckets",
                            },
                            "filter": {"userIdentity.type": "AssumedRole"},
                            "condition": "selection and not filter",
                        },
                        "falsepositives": [
                            "Administrators listing buckets, it may be necessary to filter out users who commonly conduct this activity."
                        ],
                        "level": "low",
                    },
                    {
                        "title": "Multiple AWS bucket enumerations by a single user",
                        "id": "be246094-01d3-4bba-88de-69e582eba0cc",
                        "author": "kelnage",
                        "date": "2024-07-29",
                        "status": "experimental",
                        "correlation": {
                            "type": "event_count",
                            "rules": ["f305fd62-beca-47da-ad95-7690a0620084"],
                            "group-by": ["userIdentity.arn"],
                            "timespan": "1h",
                            "condition": {"gte": 100},
                        },
                        "level": "high",
                    },
                ],
                "output_file": "conversions/test_conversion_with_correlation_rule.json",
            }
        ]
    ).decode("utf-8", "replace")


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

        result = load_rules(f.name)

    # Clean up the temporary file
    os.unlink(f.name)

    assert isinstance(result, list)
    assert len(result) == 1
    assert result[0]["title"] == "Test Rule"
    assert result[0]["description"] == "Test description"
    assert result[0]["status"] == "test"
    assert result[0]["level"] == "low"
    assert "logsource" in result[0]
    assert "detection" in result[0]


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
            load_rules(f.name)

    # Clean up the temporary file
    os.unlink(f.name)

    assert "Error loading rule file" in str(exc_info.value)


def test_load_rule_nonexistent_file():
    """Test loading a non-existent file raises ValueError."""
    with pytest.raises(ValueError) as exc_info:
        load_rules("nonexistent_file.yml")

    assert "Error loading rule file" in str(exc_info.value)


def test_load_rule_empty_file():
    """Test loading an empty file."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yml", delete=False) as f:
        f.write("")
        f.flush()

        result = load_rules(f.name)

    # Clean up the temporary file
    os.unlink(f.name)

    assert result == []
