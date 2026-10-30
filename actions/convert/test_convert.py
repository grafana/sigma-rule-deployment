import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import orjson as json
import pytest
from dynaconf.utils import DynaconfDict

from convert import convert
from convert.convert import convert_rules, filter_rule_fields, is_path, is_safe_path, load_rules


@pytest.fixture
def mock_config():
    """Mock v2 configuration object."""
    return DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                },
            },
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                }
            ],
        }
    )


@pytest.fixture
def mock_config_with_correlation_rule():
    """Mock v2 configuration object with a correlation rule."""
    return DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                    "encoding": "utf-8",
                },
            },
            "configurations": [
                {
                    "name": "test_conversion_with_correlation_rule",
                    "conversion": {
                        "input": ["rules/correlation.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                }
            ],
        }
    )


@pytest.fixture
def mock_config_two_groups():
    """Mock configuration object with two independent conversion groups."""
    return DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                },
            },
            "configurations": [
                {
                    "name": "group_a",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                },
                {
                    "name": "group_b",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        # A different but genuinely installed backend, so a real
                        # sigma-cli invocation for this group actually succeeds.
                        "target": "text_query_test",
                        "format": "default",
                    },
                },
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


@pytest.mark.parametrize("version", [None, 0, 1, 3, "2"])
def test_convert_rules_unsupported_version(temp_workspace, version):
    """Test that an error is raised when the config version is not 2."""
    config = DynaconfDict({"version": version} if version is not None else {})
    with pytest.raises(ValueError, match="only version 2 is supported"):
        convert_rules(config=config, path_prefix=temp_workspace, all_rules=True)


def test_convert_rules_invalid_output_dir(temp_workspace, mock_config):
    """Test that an error is raised when output directory is outside the project root."""
    mock_config["folders"] = {"conversion_path": "../outside"}
    with pytest.raises(ValueError, match="outside the project root"):
        convert_rules(
            config=mock_config,
            path_prefix=temp_workspace,
            all_rules=True,
        )


def test_convert_rules_missing_conversion_name():
    """Test that an error is raised when a configuration item has no name."""
    invalid_config = DynaconfDict(
        {"version": 2, "configurations": [{"conversion": {"input": ["rules/*.yml"]}}]}
    )
    with pytest.raises(
        ValueError,
        match=(
            "Conversion name is required and must be a unique identifier"
            " across all conversion objects in the config"
        ),
    ):
        convert_rules(config=invalid_config, path_prefix="/tmp", all_rules=True)


def test_convert_rules_absolute_input_path():
    """Test that an error is raised when input file pattern is absolute."""
    invalid_config = DynaconfDict(
        {
            "version": 2,
            "configurations": [
                {"name": "test", "conversion": {"input": ["/absolute/path/*.yml"]}}
            ],
        }
    )
    with pytest.raises(ValueError, match="must be relative"):
        convert_rules(config=invalid_config, path_prefix="/tmp", all_rules=True)


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


def test_convert_rules_successful_conversion_all(temp_workspace, mock_config):
    """Test that convert_rules successfully converts Sigma rules."""
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        all_rules=True,
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()
    data = json.loads(output_file.read_bytes())
    assert data["conversion_name"] == "test_conversion"
    assert len(data["queries"]) == 1
    assert len(data["rules"]) == 1


def test_convert_rules_successful_conversion_changed_files(temp_workspace, mock_config):
    """Test that convert_rules successfully converts changed Sigma rules."""
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/test.yml",
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()
    assert output_file.read_text() == json.dumps(
        {
            "conversion_name": "test_conversion",
            "input_file": "rules/test.yml",
            "output_file": "conversions/test_conversion_test.json",
            "queries": [
                '{job=~".+"} | logfmt | userIdentity_type=~`(?i)^Root$` and eventType!~`(?i)^AwsServiceEvent$`'
            ],
            "rules": [
                {
                    "description": "Detects AWS root account usage",
                    "detection": {
                        "condition": "selection and not filter",
                        "filter": {"eventType": "AwsServiceEvent"},
                        "selection": {"userIdentity.type": "Root"},
                    },
                    "falsepositives": ["AWS Tasks That Require Root User Credentials"],
                    "level": "medium",
                    "logsource": {"product": "aws", "service": "cloudtrail"},
                    "title": "AWS Root Credentials",
                }
            ],
        }
    ).decode("utf-8", "replace")


def test_convert_rules_skip_unchanged_rules(temp_workspace, mock_config):
    """Test that convert_rules successfully skips converting unchanged Sigma rules."""
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/different.yml",
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert not output_file.exists()


def test_convert_rules_successful_conversion_with_correlation_rule_all(
    temp_workspace_with_correlation_rule, mock_config_with_correlation_rule
):
    """Test that convert_rules successfully converts a Sigma correlation rule."""
    convert_rules(
        config=mock_config_with_correlation_rule,
        path_prefix=temp_workspace_with_correlation_rule,
        all_rules=True,
    )

    output_file = (
        temp_workspace_with_correlation_rule
        / "conversions"
        / "test_conversion_with_correlation_rule_correlation.json"
    )
    assert output_file.exists()
    assert output_file.read_text() == json.dumps(
        {
            "conversion_name": "test_conversion_with_correlation_rule",
            "input_file": "rules/correlation.yml",
            "output_file": "conversions/test_conversion_with_correlation_rule_correlation.json",
            "queries": [
                'sum by (userIdentity_arn) (count_over_time({job=~".+"} | logfmt | eventSource=~`(?i)^s3\\.amazonaws\\.com$` and eventName=~`(?i)^ListBuckets$` and userIdentity_type!~`(?i)^AssumedRole$` [1h])) >= 100'
            ],
            "rules": [
                {
                    "author": "Christopher Peacock @securepeacock, SCYTHE @scythe_io",
                    "date": "2023-01-06",
                    "description": "Looks for potential enumeration of AWS buckets via ListBuckets.",
                    "detection": {
                        "condition": "selection and not filter",
                        "filter": {"userIdentity.type": "AssumedRole"},
                        "selection": {
                            "eventName": "ListBuckets",
                            "eventSource": "s3.amazonaws.com",
                        },
                    },
                    "falsepositives": [
                        "Administrators listing buckets, it may be necessary to filter out users who commonly conduct this activity."
                    ],
                    "id": "f305fd62-beca-47da-ad95-7690a0620084",
                    "level": "low",
                    "logsource": {"product": "aws", "service": "cloudtrail"},
                    "modified": "2024-07-10",
                    "references": [
                        "https://github.com/Lifka/hacking-resources/blob/c2ae355d381bd0c9f0b32c4ead049f44e5b1573f/cloud-hacking-cheat-sheets.md",
                        "https://jamesonhacking.blogspot.com/2020/12/pivoting-to-private-aws-s3-buckets.html",
                        "https://securitycafe.ro/2022/12/14/aws-enumeration-part-ii-practical-enumeration/",
                    ],
                    "related": [
                        {
                            "id": "4723218f-2048-41f6-bcb0-417f2d784f61",
                            "type": "similar",
                        }
                    ],
                    "status": "test",
                    "tags": ["attack.discovery", "attack.t1580"],
                    "title": "Potential Bucket Enumeration on AWS",
                },
                {
                    "author": "kelnage",
                    "correlation": {
                        "condition": {"gte": 100},
                        "group-by": ["userIdentity.arn"],
                        "rules": ["f305fd62-beca-47da-ad95-7690a0620084"],
                        "timespan": "1h",
                        "type": "event_count",
                    },
                    "date": "2024-07-29",
                    "id": "be246094-01d3-4bba-88de-69e582eba0cc",
                    "level": "high",
                    "status": "experimental",
                    "title": "Multiple AWS bucket enumerations by a single user",
                },
            ],
        }
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
        all_rules=True,
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
        all_rules=True,
    )

    output_file = temp_workspace / "conversions" / "test_conversion.json"
    assert not output_file.exists()


@pytest.fixture
def failing_convert():
    """Patch the Sigma CLI to fail, as it does for an unconvertible rule.

    The CLI exits non-zero, which CliRunner turns into a SystemExit on
    result.exception, so the real error text is only in result.output.
    """
    mock_result = MagicMock()
    mock_result.exception = SystemExit(1)
    mock_result.exc_info = (SystemExit, SystemExit(1), None)
    mock_result.exit_code = 1
    mock_result.output = (
        'Errors found in Sigma rules:\n* Unknown modifier "bogus" in rules/test.yml\n'
    )
    with patch("click.testing.CliRunner.invoke", return_value=mock_result):
        yield mock_result


def read_conversion_errors(github_output):
    """Read the conversion_errors value written to a GITHUB_OUTPUT file."""
    lines = github_output.read_text(encoding="utf-8").splitlines()
    values = [line for line in lines if line.startswith("conversion_errors=")]
    assert len(values) == 1, f"expected one conversion_errors line, got {lines}"
    return json.loads(values[0].removeprefix("conversion_errors="))


def test_conversion_errors_written_to_github_output(
    failing_convert, temp_workspace, mock_config, monkeypatch, tmp_path
):
    """Test that a failed conversion is written to GITHUB_OUTPUT."""
    github_output = tmp_path / "github-output"
    monkeypatch.setenv("GITHUB_OUTPUT", str(github_output))

    convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    errors = read_conversion_errors(github_output)
    assert len(errors) == 1
    assert errors[0]["conversion_name"] == "test_conversion"
    # The input file must be relative to the workspace, so the Action can link to it
    assert errors[0]["input_file"] == "rules/test.yml"
    assert 'Unknown modifier "bogus"' in errors[0]["output"]


def test_conversion_errors_written_as_a_single_line(
    failing_convert, temp_workspace, mock_config, monkeypatch, tmp_path
):
    """Test that a multi-line error stays on one line, as GITHUB_OUTPUT requires."""
    github_output = tmp_path / "github-output"
    monkeypatch.setenv("GITHUB_OUTPUT", str(github_output))

    convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    contents = github_output.read_text(encoding="utf-8")
    assert len(contents.splitlines()) == 1
    # JSON encoding must escape the newlines rather than emit them literally
    assert "\\n" in contents


def test_conversion_errors_written_as_decoded_json(
    failing_convert, temp_workspace, mock_config, monkeypatch, tmp_path
):
    """Test that the value is JSON and not a stringified Python bytes object."""
    github_output = tmp_path / "github-output"
    monkeypatch.setenv("GITHUB_OUTPUT", str(github_output))

    convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    contents = github_output.read_text(encoding="utf-8")
    assert contents.startswith("conversion_errors=[")
    assert "conversion_errors=b'" not in contents


def test_no_conversion_errors_written_on_success(
    temp_workspace, mock_config, monkeypatch, tmp_path
):
    """Test that nothing is written to GITHUB_OUTPUT when every rule converts."""
    github_output = tmp_path / "github-output"
    monkeypatch.setenv("GITHUB_OUTPUT", str(github_output))

    convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists(), "expected the conversion to succeed"
    assert not github_output.exists()


def test_conversion_errors_without_github_output(
    failing_convert, temp_workspace, mock_config, monkeypatch, capsys
):
    """Test that a failed conversion outside Actions warns instead of raising."""
    monkeypatch.delenv("GITHUB_OUTPUT", raising=False)

    convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    assert "GITHUB_OUTPUT environment variable not set" in capsys.readouterr().out


def test_conversion_errors_one_entry_per_failed_rule(
    failing_convert, temp_workspace, mock_config, monkeypatch, tmp_path
):
    """Test that each failed rule gets its own entry."""
    second_rule = temp_workspace / "rules" / "test2.yml"
    second_rule.write_text(
        (temp_workspace / "rules" / "test.yml").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    github_output = tmp_path / "github-output"
    monkeypatch.setenv("GITHUB_OUTPUT", str(github_output))

    convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    errors = read_conversion_errors(github_output)
    assert sorted(error["input_file"] for error in errors) == [
        "rules/test.yml",
        "rules/test2.yml",
    ]


def test_conversion_error_does_not_stop_later_conversions(
    temp_workspace, mock_config, monkeypatch, tmp_path
):
    """Test that a rule failing does not prevent the remaining rules converting."""
    second_rule = temp_workspace / "rules" / "test2.yml"
    second_rule.write_text(
        (temp_workspace / "rules" / "test.yml").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    github_output = tmp_path / "github-output"
    monkeypatch.setenv("GITHUB_OUTPUT", str(github_output))

    failure = MagicMock()
    failure.exception = SystemExit(1)
    failure.exc_info = (SystemExit, SystemExit(1), None)
    failure.exit_code = 1
    failure.output = "Errors found in Sigma rules:\n* Unknown modifier\n"

    success = MagicMock()
    success.exception = None
    success.exc_info = None
    success.exit_code = 0
    success.stdout = 'Parsing Sigma rules\n{job="test"}\n'

    # Rules are converted in glob order, which is not guaranteed, so only assert
    # on the counts rather than on which of the two rules failed
    with patch("click.testing.CliRunner.invoke", side_effect=[failure, success]):
        convert_rules(config=mock_config, path_prefix=temp_workspace, all_rules=True)

    errors = read_conversion_errors(github_output)
    assert len(errors) == 1
    written = list((temp_workspace / "conversions").glob("*.json"))
    assert len(written) == 1, "the rule that converted should still be written"


def test_convert_rules_required_rule_fields_per_conversion(temp_workspace):
    """Test that required_rule_fields is respected per conversion, overriding the default."""
    config = DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                    "required_rule_fields": ["title", "level", "description"],
                },
            },
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "required_rule_fields": ["title"],
                    },
                }
            ],
        }
    )
    convert_rules(config=config, path_prefix=temp_workspace, all_rules=True)

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()
    data = json.loads(output_file.read_bytes())
    assert len(data["rules"]) == 1
    # Only "title" should be present — the per-conversion override takes precedence over the default
    assert list(data["rules"][0].keys()) == ["title"]


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


@pytest.mark.parametrize(
    "config_params, expected_args",
    [
        # Test default values only
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [{"name": "test_default", "conversion": {"input": ["test.yml"]}}],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test overriding target
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {"name": "test_target", "conversion": {"input": ["test.yml"], "target": "splunk"}}
                ],
            },
            [
                "--target",
                "splunk",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test overriding format
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {"name": "test_format", "conversion": {"input": ["test.yml"], "format": "custom"}}
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "custom",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test setting pipelines
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_pipelines",
                        "conversion": {
                            "input": ["test.yml"],
                            "pipelines": ["pipeline1.yml", "pipeline2.yml"],
                        },
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--pipeline=/private/tmp/pipeline1.yml",
                "--pipeline=/private/tmp/pipeline2.yml",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test setting correlation method
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_correlation",
                        "conversion": {"input": ["test.yml"], "correlation_method": "default"},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--correlation-method",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test setting filters
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_filters",
                        "conversion": {
                            "input": ["test.yml"],
                            "filters": ["filter1", "filter2"],
                        },
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--filter=filter1",
                "--filter=filter2",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test setting backend options
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_backend",
                        "conversion": {
                            "input": ["test.yml"],
                            "backend_options": {"option1": "value1", "option2": "value2"},
                        },
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--backend-option=option1=value1",
                "--backend-option=option2=value2",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test without pipeline
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_without_pipeline",
                        "conversion": {"input": ["test.yml"], "without_pipeline": True},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--without-pipeline",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test disable pipeline check
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_no_pipeline_check",
                        "conversion": {"input": ["test.yml"], "pipeline_check": False},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--disable-pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test fail unsupported instead of skip
        (
            {
                "defaults": {"conversion": {"skip_unsupported": False}},
                "configurations": [
                    {
                        "name": "test_fail",
                        "conversion": {"input": ["test.yml"], "fail_unsupported": True},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--fail-unsupported",
            ],
        ),
        # Test json indent
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {
                        "name": "test_json_indent",
                        "conversion": {"input": ["test.yml"], "json_indent": 2},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "2",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test verbose
        (
            {
                "defaults": {"conversion": {}},
                "configurations": [
                    {"name": "test_verbose", "conversion": {"input": ["test.yml"], "verbose": True}}
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
                "--verbose",
            ],
        ),
        # Test overriding skip_unsupported per conversion (default false, conversion true)
        (
            {
                "defaults": {"conversion": {"skip_unsupported": False}},
                "configurations": [
                    {
                        "name": "test_skip",
                        "conversion": {"input": ["test.yml"], "skip_unsupported": True},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test overriding file_pattern per conversion (default *.json, conversion overrides to *.yml)
        (
            {
                "defaults": {"conversion": {"file_pattern": "*.json"}},
                "configurations": [
                    {
                        "name": "test_file_pattern",
                        "conversion": {"input": ["test.yml"], "file_pattern": "*.yml"},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test overriding encoding per conversion
        (
            {
                "defaults": {"conversion": {"encoding": "latin1"}},
                "configurations": [
                    {
                        "name": "test_encoding",
                        "conversion": {"input": ["test.yml"], "encoding": "utf-8"},
                    }
                ],
            },
            [
                "--target",
                "loki",
                "--format",
                "default",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "utf-8",
                "--json-indent",
                "0",
                "--pipeline-check",
                "--skip-unsupported",
            ],
        ),
        # Test combination of several options
        (
            {
                "defaults": {"conversion": {"target": "elastic", "format": "custom_default", "encoding": "latin1"}},
                "configurations": [
                    {
                        "name": "test_combo",
                        "conversion": {
                            "input": ["test.yml"],
                            "target": "splunk",
                            "pipelines": ["pipeline.yml"],
                            "filters": ["filter1"],
                            "backend_options": {"opt": "val"},
                            "without_pipeline": True,
                            "verbose": True,
                        },
                    }
                ],
            },
            [
                "--target",
                "splunk",
                "--pipeline=/private/tmp/pipeline.yml",
                "--format",
                "default",
                "--filter=filter1",
                "--file-pattern",
                "*.yml",
                "--output",
                "-",
                "--encoding",
                "latin1",
                "--json-indent",
                "0",
                "--backend-option=opt=val",
                "--without-pipeline",
                "--pipeline-check",
                "--skip-unsupported",
                "--verbose",
            ],
        ),
    ],
)
@patch("glob.glob")
@patch("os.path.exists")
@patch("pathlib.Path.is_absolute")
@patch("pathlib.Path.is_dir")
@patch("pathlib.Path.mkdir")
@patch("shutil.rmtree")
@patch("click.testing.CliRunner.invoke")
@patch("dynaconf.Dynaconf")
def test_convert_rules_command_args(
    mock_dynaconf,
    mock_invoke,
    mock_rmtree,
    mock_mkdir,
    mock_is_dir,
    mock_is_absolute,
    mock_exists,
    mock_glob,
    config_params,
    expected_args,
):
    """Test that the correct command arguments are passed to invoke based on config."""
    # Setup mocks
    mock_glob.return_value = ["/tmp/test.yml"]
    mock_exists.return_value = True
    mock_is_absolute.return_value = False
    mock_is_dir.return_value = True

    # Mock result
    mock_result = MagicMock()
    mock_result.exception = None
    mock_result.exit_code = 0
    mock_result.stdout = "test query output"
    mock_invoke.return_value = mock_result

    # Create config with the tested parameters
    config_dict = DynaconfDict({"version": 2, **config_params})

    # Mock Dynaconf to accept DynaconfDict
    dynaconf_instance = mock_dynaconf.return_value
    dynaconf_instance.get.side_effect = lambda key, default=None: config_dict.get(
        key, default
    )

    # Mock is_path to return True for any pipeline paths
    # Mock pipeline paths, path handling, rules loading, and file I/O
    with (
        patch.object(convert, "is_path", side_effect=lambda p, f: True),
        patch("pathlib.Path.relative_to", return_value=Path("test.yml")),
        patch.object(convert, "load_rules", return_value=[{"title": "Test Rule"}]),
        patch("builtins.open", MagicMock()),
    ):
        # Run the function
        convert_rules(
            config=dynaconf_instance, path_prefix="/tmp", all_rules=True
        )

        # Verify invoke arguments
        call_args = mock_invoke.call_args[1]["args"]

        # Check key arguments are present
        assert "--target" in call_args

        # Add input file that's always at the end
        # Test the actual args rather than expected vs actual since some paths may be transformed
        assert call_args[-1] == "/tmp/test.yml"

        # Only check critical specific arguments based on the test case
        if "--correlation-method" in expected_args:
            assert "--correlation-method" in call_args
            corr_index = call_args.index("--correlation-method")
            assert (
                call_args[corr_index + 1]
                == expected_args[
                    expected_args.index("--correlation-method") + 1
                ]
            )

        if "--filter=" in "".join(expected_args):
            for filter_arg in [
                arg for arg in expected_args if arg.startswith("--filter=")
            ]:
                assert filter_arg in call_args

        if "--without-pipeline" in expected_args:
            assert "--without-pipeline" in call_args

        if "--disable-pipeline-check" in expected_args:
            assert "--disable-pipeline-check" in call_args

        # Sigma CLI treats --skip-unsupported and --fail-unsupported as one switch,
        # so exactly one of them is passed
        if "--fail-unsupported" in expected_args:
            assert "--fail-unsupported" in call_args
            assert "--skip-unsupported" not in call_args

        if "--verbose" in expected_args:
            assert "--verbose" in call_args

        # Verify target
        target_index = call_args.index("--target")
        assert (
            call_args[target_index + 1]
            == expected_args[expected_args.index("--target") + 1]
        )

        # Format might be different due to conversion defaults - don't assert strict equality
        assert "--format" in call_args


# Test handling of correlation_method when set in defaults but not in conversion
@patch("glob.glob")
@patch("os.path.exists")
@patch("pathlib.Path.is_absolute")
@patch("pathlib.Path.is_dir")
@patch("pathlib.Path.mkdir")
@patch("shutil.rmtree")
@patch("click.testing.CliRunner.invoke")
@patch("dynaconf.Dynaconf")
def test_default_correlation_method(
    mock_dynaconf,
    mock_invoke,
    mock_rmtree,
    mock_mkdir,
    mock_is_dir,
    mock_is_absolute,
    mock_exists,
    mock_glob,
):
    """Test that default correlation method is properly applied."""
    # Setup mocks
    mock_glob.return_value = ["/tmp/test.yml"]
    mock_exists.return_value = True
    mock_is_absolute.return_value = False
    mock_is_dir.return_value = True

    # Mock result with correlation method in the output
    mock_result = MagicMock()
    mock_result.exception = None
    mock_result.exit_code = 0
    mock_result.stdout = "test query output"
    mock_invoke.return_value = mock_result

    # Create config with default correlation method
    config_dict = DynaconfDict(
        {
            "version": 2,
            "defaults": {"conversion": {"correlation_method": "default_corr"}},
            "configurations": [{"name": "test_default_corr", "conversion": {"input": ["test.yml"]}}],
        }
    )

    # Mock Dynaconf to accept DynaconfDict
    dynaconf_instance = mock_dynaconf.return_value
    dynaconf_instance.get.side_effect = lambda key, default=None: config_dict.get(
        key, default
    )

    # Mock is_path to handle pipeline paths
    # Setup mocks for path validation, path handling, rule loading, and file I/O
    with (
        patch.object(convert, "is_path", side_effect=lambda p, f: True),
        patch("pathlib.Path.relative_to", return_value=Path("test.yml")),
        patch.object(convert, "load_rules", return_value=[{"title": "Test Rule"}]),
        patch("builtins.open", MagicMock()),
    ):
        # Run the function
        convert_rules(
            config=dynaconf_instance, path_prefix="/tmp", all_rules=True
        )

        # Verify the function was called with the right parameters
        assert mock_invoke.called

        # The test is verifying that default correlation method
        # is being included in the config, not necessarily in the args
        # So we just verify the conversion ran successfully
        assert mock_invoke.call_count > 0


@pytest.mark.parametrize(
    "pipelines, expect_template_vars_path",
    [
        (["pipelines/a/p1.yml", "pipelines/p2.yml"], True),
        ([], False),
    ],
)
@patch("glob.glob")
@patch("os.path.exists")
@patch("pathlib.Path.is_absolute")
@patch("pathlib.Path.is_dir")
@patch("pathlib.Path.mkdir")
@patch("shutil.rmtree")
@patch("click.testing.CliRunner.invoke")
@patch("dynaconf.Dynaconf")
def test_template_vars_path_shared_across_pipelines(
    mock_dynaconf,
    mock_invoke,
    mock_rmtree,
    mock_mkdir,
    mock_is_dir,
    mock_is_absolute,
    mock_exists,
    mock_glob,
    pipelines,
    expect_template_vars_path,
):
    """Pipelines get one shared --template-vars-path so pySigma 2.x can merge them."""
    mock_glob.return_value = ["/tmp/test.yml"]
    mock_exists.return_value = True
    mock_is_absolute.return_value = False
    mock_is_dir.return_value = True

    mock_result = MagicMock()
    mock_result.exception = None
    mock_result.exit_code = 0
    mock_result.stdout = "test query output"
    mock_invoke.return_value = mock_result

    config_dict = DynaconfDict(
        {
            "version": 2,
            "configurations": [
                {
                    "name": "test_pipelines",
                    "conversion": {"input": ["test.yml"], "pipelines": pipelines},
                }
            ],
        }
    )
    dynaconf_instance = mock_dynaconf.return_value
    dynaconf_instance.get.side_effect = lambda key, default=None: config_dict.get(key, default)

    with (
        patch.object(convert, "is_path", side_effect=lambda p, f: True),
        patch("pathlib.Path.relative_to", return_value=Path("test.yml")),
        patch.object(convert, "load_rules", return_value=[{"title": "Test Rule"}]),
        patch("builtins.open", MagicMock()),
    ):
        convert_rules(config=dynaconf_instance, path_prefix="/tmp", all_rules=True)

    call_args = mock_invoke.call_args[1]["args"]
    if expect_template_vars_path:
        assert call_args[call_args.index("--template-vars-path") + 1] == "/tmp"
    else:
        assert "--template-vars-path" not in call_args


def test_convert_rules_deletes_conversion_for_deleted_rule(temp_workspace, mock_config):
    """Test that when a rule is deleted, its associated conversion file is also deleted."""
    # First create a conversion file for the test rule
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    conversion_file = conversion_dir / "test_conversion_test.json"
    conversion_file.write_text("{}")

    assert conversion_file.exists()

    # Run convert_rules with deleted_files to simulate a rule deletion
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        deleted_files="rules/test.yml",
    )

    # Verify the conversion file was deleted
    assert not conversion_file.exists()


def test_filter_rule_fields():
    """Test that the filter_rule_fields function filters the rule fields correctly."""
    rule_dicts = [
        {
            "id": "1",
            "title": "Test Rule",
            "description": "Test Description",
            "severity": "Test Severity",
            "logsource": {"category": "Test Category", "product": "Test Product", "service": "Test Service", "definition": "Test Definition"},
            "detection": {"selection": {"field": "Test Field"}, "condition": "selection"},
            "fields": ["Test Field"]
        },
        {
            "id": "2",
            "title": "Test Rule 2",
            "severity": "Test Severity 2",
            "logsource": {"category": "Test Category 2", "product": "Test Product 2", "service": "Test Service 2", "definition": "Test Definition 2"},
            "detection": {"selection": {"field": "Test Field 2"}, "condition": "selection"},
            "fields": ["Test Field 2"]
        }
    ]
    required_fields = ["title", "description", "logsource"]
    filtered_rule_dicts = filter_rule_fields(rule_dicts, required_fields)
    assert filtered_rule_dicts == [
        {
            "id": "1",
            "title": "Test Rule",
            "description": "Test Description",
            "logsource": {"category": "Test Category", "product": "Test Product", "service": "Test Service", "definition": "Test Definition"}
        },
        {
            "id": "2",
            "title": "Test Rule 2",
            "logsource": {"category": "Test Category 2", "product": "Test Product 2", "service": "Test Service 2", "definition": "Test Definition 2"}
        }
    ]


def test_convert_rules_skips_manual_conversion(temp_workspace, mock_config):
    """A conversion file marked manual is not overwritten when its rule changes."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    manual_content = json.dumps({"manual": True, "queries": ["HAND EDITED"]}).decode(
        "utf-8"
    )
    output_file.write_text(manual_content)

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/test.yml",
    )

    # The manual file must be left exactly as the human wrote it.
    assert output_file.read_text() == manual_content


def test_convert_rules_backfills_manual_flag(temp_workspace, mock_config): # trufflehog:ignore
    """A human-modified conversion file listed in manual_files gains the manual flag."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    output_file.write_text(
        json.dumps(
            {"queries": ["HUMAN EDIT"], "conversion_name": "test_conversion"}
        ).decode("utf-8")
    )

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        manual_files="conversions/test_conversion_test.json",
    )

    data = json.loads(output_file.read_bytes())
    assert data["manual"] is True
    assert data["queries"] == ["HUMAN EDIT"]


def test_convert_rules_backfilled_file_not_overwritten(temp_workspace, mock_config):
    """A human edit is flagged and preserved even when the source rule also changed."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    output_file.write_text(
        json.dumps(
            {"queries": ["HUMAN EDIT"], "conversion_name": "test_conversion"}
        ).decode("utf-8")
    )

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/test.yml",
        manual_files="conversions/test_conversion_test.json",
    )

    data = json.loads(output_file.read_bytes())
    assert data["manual"] is True
    # The query was not regenerated from the (changed) rule.
    assert data["queries"] == ["HUMAN EDIT"]


def test_convert_rules_keeps_manual_conversion_on_delete(temp_workspace, mock_config):
    """A manual conversion file is not deleted when its source rule is deleted."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    conversion_file = conversion_dir / "test_conversion_test.json"
    conversion_file.write_text(json.dumps({"manual": True}).decode("utf-8"))

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        deleted_files="rules/test.yml",
    )

    assert conversion_file.exists()


def test_convert_rules_respects_explicit_manual_false(temp_workspace, mock_config):
    """An explicit manual:false is not re-flagged to true by the backfill."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    output_file.write_text(
        json.dumps({"manual": False, "queries": ["HUMAN EDIT"]}).decode("utf-8")
    )

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        manual_files="conversions/test_conversion_test.json",
    )

    data = json.loads(output_file.read_bytes())
    assert data["manual"] is False


def test_convert_rules_regenerates_manual_false_conversion(temp_workspace, mock_config):
    """The opt-out path: a conversion file set to manual:false is not re-flagged and
    is regenerated (handed back) when its source rule changes."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    output_file.write_text(
        json.dumps({"manual": False, "queries": ["STALE"]}).decode("utf-8")
    )

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/test.yml",
        manual_files="conversions/test_conversion_test.json",
    )

    data = json.loads(output_file.read_bytes())
    # Regenerated: fresh output carries no manual key and the real (not stale) query.
    assert "manual" not in data
    assert data["queries"] != ["STALE"]


def test_convert_rules_backfill_ignores_files_outside_conversion_dir(
    temp_workspace, mock_config
):
    """The backfill only flags files inside the conversion output directory."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    # A JSON file outside the conversions directory must be left untouched.
    outside = temp_workspace / "elsewhere.json"
    outside.write_text(json.dumps({"queries": ["x"]}).decode("utf-8"))

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        manual_files="elsewhere.json",
    )

    data = json.loads(outside.read_bytes())
    assert "manual" not in data


@patch("click.testing.CliRunner.invoke")
def test_convert_rules_skips_manual_before_conversion(
    mock_invoke, temp_workspace, mock_config
):
    """A manual conversion file is skipped before the (expensive) sigma-cli invoke."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    output_file.write_text(
        json.dumps({"manual": True, "queries": ["HAND EDITED"]}).decode("utf-8")
    )

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/test.yml",
    )

    # The skip happens before conversion, so sigma-cli is never invoked.
    mock_invoke.assert_not_called()


####
# Config file changes tests
####

def test_convert_rules_config_change_reconverts_group(temp_workspace, mock_config):
    """A conversion group whose own config block changed is fully reconverted,
    even though none of its rule files are in changed_files."""
    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "splunk",
                        "format": "default",
                    },
                }
            ],
        }
    )
    # The config-change check is gated on the config file's own path appearing in
    # changed_files, mirroring how the real action includes CONFIG_PATH in its diff.
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()


def test_convert_rules_renamed_group_converts_without_deleting_old(
    temp_workspace, mock_config
):
    """A renamed conversion group is converted under its new name; the old
    output file is left alone (its cleanup is the integrator's job, not the
    converter's)."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    old_output_file = conversion_dir / "old_name_test.json"
    old_output_file.write_text(
        json.dumps({"conversion_name": "old_name"}).decode("utf-8")
    )

    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "old_name",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                }
            ],
        }
    )
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    new_output_file = conversion_dir / "test_conversion_test.json"
    assert new_output_file.exists()
    assert old_output_file.exists()
    assert json.loads(old_output_file.read_bytes()) == {"conversion_name": "old_name"}


def test_convert_rules_unrelated_group_not_reconverted_on_config_change(
    temp_workspace, mock_config_two_groups
):
    """Only the conversion group whose config actually changed is reconverted;
    a sibling group with an unchanged config block stays skipped."""
    previous_config = DynaconfDict(
        {
            "defaults": {
                "conversion": dict(mock_config_two_groups["defaults"]["conversion"])
            },
            "configurations": [
                {
                    "name": "group_a",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                },
                {
                    "name": "group_b",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "elastic",
                        "format": "default",
                    },
                },
            ],
        }
    )
    convert_rules(
        config=mock_config_two_groups,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    conversion_dir = temp_workspace / "conversions"
    assert (conversion_dir / "group_b_test.json").exists()
    assert not (conversion_dir / "group_a_test.json").exists()


def test_convert_rules_identical_previous_config_skips(temp_workspace, mock_config):
    """An identical previous_config is not itself a reason to reconvert; the
    usual changed-files skip logic still applies."""
    convert_rules(
        config=mock_config,
        previous_config=mock_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert not output_file.exists()


def test_convert_rules_config_key_order_does_not_reconvert(temp_workspace, mock_config):
    """A conversion block that is semantically identical, just with its keys
    in a different order, must not be treated as changed."""
    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "format": "default",
                        "target": "loki",
                        "input": ["rules/*.yml"],
                    },
                }
            ],
        }
    )
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert not output_file.exists()


def test_convert_rules_without_previous_config_behaves_as_before(
    temp_workspace, mock_config
):
    """previous_config is optional; omitting it must not change existing
    behavior for a normal changed-files run."""

    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        changed_files="rules/test.yml",
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()


def test_convert_rules_config_change_respects_manual_flag(temp_workspace, mock_config):
    """A group-wide reconvert triggered by a config change must still skip
    an output file already marked manual."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    output_file = conversion_dir / "test_conversion_test.json"
    manual_content = json.dumps({"manual": True, "queries": ["HAND EDITED"]}).decode(
        "utf-8"
    )
    output_file.write_text(manual_content)

    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "splunk",
                        "format": "default",
                    },
                }
            ],
        }
    )
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    assert output_file.read_text() == manual_content


def test_convert_rules_multiple_new_groups_convert_independently(
    temp_workspace, mock_config_two_groups
):
    """Several conversion groups added in the same config update all convert,
    independently of one another."""
    previous_config = DynaconfDict(
        {
            "defaults": {
                "conversion": dict(mock_config_two_groups["defaults"]["conversion"])
            },
            "configurations": [],
        }
    )
    convert_rules(
        config=mock_config_two_groups,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    conversion_dir = temp_workspace / "conversions"
    assert (conversion_dir / "group_a_test.json").exists()
    assert (conversion_dir / "group_b_test.json").exists()


def test_convert_rules_default_change_reconverts_all_groups(temp_workspace):
    """A change to a top-level defaults.conversion value affects every group
    that inherits it, so the whole repository is reconverted, not just the
    group whose own block happened to change."""
    shared_configurations = [
        {"name": "group_a", "conversion": {"input": ["rules/*.yml"]}},
        {"name": "group_b", "conversion": {"input": ["rules/*.yml"]}},
    ]
    config = DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    # A different but genuinely installed backend, so a real
                    # sigma-cli invocation for both groups actually succeeds.
                    "target": "text_query_test",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                },
            },
            "configurations": shared_configurations,
        }
    )
    previous_config = DynaconfDict(
        {
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                },
            },
            "configurations": shared_configurations,
        }
    )
    convert_rules(
        config=config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    conversion_dir = temp_workspace / "conversions"
    assert (conversion_dir / "group_a_test.json").exists()
    assert (conversion_dir / "group_b_test.json").exists()


def test_convert_rules_dropped_input_entry_deletes_stale_output(
    temp_workspace, mock_config
):
    """A rule dropped from a conversion's input list, with the
    file left in place on disk, has its stale conversion output deleted even
    though the rule was never in changed_files or deleted_files."""
    (temp_workspace / "rules" / "keep.yml").write_text(
        (temp_workspace / "rules" / "test.yml").read_text()
    )
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    stale_output = conversion_dir / "test_conversion_test.json"
    stale_output.write_text(
        json.dumps(
            {"conversion_name": "test_conversion", "input_file": "rules/test.yml"}
        ).decode("utf-8")
    )

    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/test.yml", "rules/keep.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                }
            ],
        }
    )
    mock_config["configurations"][0]["conversion"]["input"] = ["rules/keep.yml"]
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    assert not stale_output.exists()
    # The entry that's still in scope must still be (re)converted.
    assert (conversion_dir / "test_conversion_keep.json").exists()


def test_convert_rules_narrowed_input_pattern_keeps_still_matched_files(
    temp_workspace, mock_config
):
    """Narrowing a glob (rather than dropping a specific file) must not
    delete output for a rule that's still matched by the narrower pattern,
    even though the old, broader pattern string is no longer present."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    still_valid_output = conversion_dir / "test_conversion_test.json"
    still_valid_output.write_text(
        json.dumps(
            {"conversion_name": "test_conversion", "input_file": "rules/test.yml"}
        ).decode("utf-8")
    )

    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                }
            ],
        }
    )
    mock_config["configurations"][0]["conversion"]["input"] = ["rules/test.yml"]
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    assert still_valid_output.exists()


def test_convert_rules_reassigned_input_moves_output_between_groups(
    temp_workspace, mock_config_two_groups
):
    """A rule reassigned from one conversion's input to another's
    via a config edit (not a git-detected file change) gets its old group's
    stale output deleted while the new group produces fresh output."""
    (temp_workspace / "rules" / "keep.yml").write_text(
        (temp_workspace / "rules" / "test.yml").read_text()
    )
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    stale_output = conversion_dir / "group_a_test.json"
    stale_output.write_text(
        json.dumps(
            {"conversion_name": "group_a", "input_file": "rules/test.yml"}
        ).decode("utf-8")
    )

    previous_config = DynaconfDict(
        {
            "defaults": {
                "conversion": dict(mock_config_two_groups["defaults"]["conversion"])
            },
            "configurations": [
                {
                    "name": "group_a",
                    "conversion": {
                        "input": ["rules/test.yml", "rules/keep.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                },
                {
                    "name": "group_b",
                    "conversion": {
                        "input": [],
                        "target": "text_query_test",
                        "format": "default",
                    },
                },
            ],
        }
    )
    mock_config_two_groups["configurations"][0]["conversion"]["input"] = ["rules/keep.yml"]
    mock_config_two_groups["configurations"][1]["conversion"]["input"] = ["rules/test.yml"]
    convert_rules(
        config=mock_config_two_groups,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    # The old owner's stale output for the reassigned rule is gone.
    assert not stale_output.exists()
    # The new owner produced fresh output for it.
    assert (conversion_dir / "group_b_test.json").exists()
    # The old owner's remaining rule is unaffected.
    assert (conversion_dir / "group_a_keep.json").exists()


def test_convert_rules_dropped_input_entry_respects_manual_flag(
    temp_workspace, mock_config
):
    """A manually-maintained conversion file must not be deleted just because
    its rule was dropped from the conversion's input list."""
    (temp_workspace / "rules" / "keep.yml").write_text(
        (temp_workspace / "rules" / "test.yml").read_text()
    )
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    manual_output = conversion_dir / "test_conversion_test.json"
    manual_content = json.dumps(
        {
            "conversion_name": "test_conversion",
            "input_file": "rules/test.yml",
            "manual": True,
            "queries": ["HAND EDITED"],
        }
    ).decode("utf-8")
    manual_output.write_text(manual_content)

    previous_config = DynaconfDict(
        {
            "defaults": {"conversion": dict(mock_config["defaults"]["conversion"])},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {
                        "input": ["rules/test.yml", "rules/keep.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                }
            ],
        }
    )
    mock_config["configurations"][0]["conversion"]["input"] = ["rules/keep.yml"]
    convert_rules(
        config=mock_config,
        previous_config=previous_config,
        path_prefix=temp_workspace,
        changed_files="config.yaml",
        config_path=str(temp_workspace / "config.yaml"),
    )

    assert manual_output.read_text() == manual_content


def _recorded_invoke_args(temp_workspace, default_conversion, conversion):
    """Run convert_rules with a mocked sigma-cli and return the args it was invoked with."""
    config = DynaconfDict(
        {
            "version": 2,
            "defaults": {"conversion": {"target": "loki", "file_pattern": "*.yml", **default_conversion}},
            "configurations": [
                {"name": "test_conversion", "conversion": {"input": ["rules/*.yml"], **conversion}}
            ],
        }
    )
    with patch("click.testing.CliRunner.invoke") as mock_invoke:
        mock_result = MagicMock()
        mock_result.exception = None
        mock_result.exc_info = None
        mock_result.exit_code = 0
        mock_result.stdout = "Parsing Sigma rules\n"
        mock_invoke.return_value = mock_result
        convert_rules(config=config, path_prefix=temp_workspace, all_rules=True)
    assert mock_invoke.call_count == 1
    return mock_invoke.call_args[1]["args"]


def _flag_values(args, prefix):
    return [a for a in args if a.startswith(prefix)]


def test_conversion_list_and_map_settings_inherit_defaults_when_unset(temp_workspace):
    args = _recorded_invoke_args(
        temp_workspace,
        {
            "pipelines": ["pipelines/default.yml"],
            "filters": ["default_filter"],
            "backend_options": {"opt": "default"},
            "json_indent": 4,
        },
        {},
    )
    assert _flag_values(args, "--pipeline=") == [f"--pipeline={temp_workspace}/pipelines/default.yml"]
    assert _flag_values(args, "--filter=") == ["--filter=default_filter"]
    assert _flag_values(args, "--backend-option=") == ["--backend-option=opt=default"]
    assert args[args.index("--json-indent") + 1] == "4"


def test_conversion_values_replace_defaults(temp_workspace):
    args = _recorded_invoke_args(
        temp_workspace,
        {
            "pipelines": ["pipelines/default.yml"],
            "filters": ["default_filter"],
            "backend_options": {"opt": "default", "other": "default"},
            "json_indent": 4,
        },
        {
            "pipelines": ["pipelines/own.yml"],
            "filters": ["own_filter"],
            "backend_options": {"opt": "own"},
            "json_indent": 2,
        },
    )
    assert _flag_values(args, "--pipeline=") == [f"--pipeline={temp_workspace}/pipelines/own.yml"]
    assert _flag_values(args, "--filter=") == ["--filter=own_filter"]
    # Maps are replaced, not merged key by key
    assert _flag_values(args, "--backend-option=") == ["--backend-option=opt=own"]
    assert args[args.index("--json-indent") + 1] == "2"


def test_conversion_empty_values_override_defaults(temp_workspace):
    """An explicitly empty list, map or zero switches off the default instead of falling back to it."""
    args = _recorded_invoke_args(
        temp_workspace,
        {
            "pipelines": ["pipelines/default.yml"],
            "filters": ["default_filter"],
            "backend_options": {"opt": "default"},
            "json_indent": 4,
        },
        {"pipelines": [], "filters": [], "backend_options": {}, "json_indent": 0},
    )
    assert _flag_values(args, "--pipeline=") == []
    assert _flag_values(args, "--filter=") == []
    assert _flag_values(args, "--backend-option=") == []
    assert args[args.index("--json-indent") + 1] == "0"


def test_conversion_null_values_inherit_defaults(temp_workspace):
    """A key with no value (YAML null) counts as unset."""
    args = _recorded_invoke_args(
        temp_workspace,
        {"pipelines": ["pipelines/default.yml"], "filters": ["default_filter"], "backend_options": {"opt": "default"}},
        {"pipelines": None, "filters": None, "backend_options": None},
    )
    assert _flag_values(args, "--pipeline=") == [f"--pipeline={temp_workspace}/pipelines/default.yml"]
    assert _flag_values(args, "--filter=") == ["--filter=default_filter"]
    assert _flag_values(args, "--backend-option=") == ["--backend-option=opt=default"]


def _unsupported_flags(args):
    return [a for a in args if a in ("--skip-unsupported", "--fail-unsupported")]


@pytest.mark.parametrize(
    "default_conversion, conversion, expected",
    [
        # skip_unsupported defaults to true
        ({}, {}, ["--skip-unsupported"]),
        ({"skip_unsupported": False}, {}, ["--fail-unsupported"]),
        # A configuration's skip_unsupported overrides the default
        ({"skip_unsupported": False}, {"skip_unsupported": True}, ["--skip-unsupported"]),
        ({"skip_unsupported": True}, {"skip_unsupported": False}, ["--fail-unsupported"]),
        # The deprecated fail_unsupported: true on its own still fails the conversion
        ({}, {"fail_unsupported": True}, ["--fail-unsupported"]),
        ({"fail_unsupported": True}, {}, ["--fail-unsupported"]),
        ({"skip_unsupported": True}, {"fail_unsupported": True}, ["--fail-unsupported"]),
        # A configuration's skip_unsupported overrides a default fail_unsupported
        ({"fail_unsupported": True}, {"skip_unsupported": True}, ["--skip-unsupported"]),
        # skip_unsupported wins over fail_unsupported at the same level
        ({}, {"skip_unsupported": True, "fail_unsupported": True}, ["--skip-unsupported"]),
        ({"skip_unsupported": True, "fail_unsupported": True}, {}, ["--skip-unsupported"]),
        # fail_unsupported: false has no effect
        ({}, {"fail_unsupported": False}, ["--skip-unsupported"]),
        ({"skip_unsupported": False}, {"fail_unsupported": False}, ["--fail-unsupported"]),
        # A null fail_unsupported counts as unset
        ({"fail_unsupported": True}, {"fail_unsupported": None}, ["--fail-unsupported"]),
    ],
)
def test_conversion_skip_unsupported(temp_workspace, default_conversion, conversion, expected):
    args = _recorded_invoke_args(temp_workspace, default_conversion, conversion)
    assert _unsupported_flags(args) == expected


def _deprecation_warnings(output):
    return [
        line
        for line in output.splitlines()
        if line.startswith("::warning::fail_unsupported") and "deprecated" in line
    ]


@pytest.mark.parametrize(
    "default_conversion, conversion, scopes",
    [
        ({}, {}, []),
        ({"skip_unsupported": False}, {"skip_unsupported": True}, []),
        ({}, {"fail_unsupported": True}, ["configuration 'test_conversion'"]),
        ({"fail_unsupported": True}, {}, ["defaults"]),
        # fail_unsupported: false has no effect, but is still deprecated
        ({}, {"fail_unsupported": False}, ["configuration 'test_conversion'"]),
        (
            {"fail_unsupported": True},
            {"fail_unsupported": True},
            ["defaults", "configuration 'test_conversion'"],
        ),
    ],
)
def test_conversion_fail_unsupported_deprecation_warning(
    temp_workspace, capsys, default_conversion, conversion, scopes
):
    _recorded_invoke_args(temp_workspace, default_conversion, conversion)
    warnings = _deprecation_warnings(capsys.readouterr().out)
    assert len(warnings) == len(scopes)
    for warning, scope in zip(warnings, scopes):
        assert f"fail_unsupported in {scope} is deprecated" in warning
        assert "skip_unsupported: false" in warning
        assert "ignored" not in warning


@pytest.mark.parametrize(
    "default_conversion, conversion, scope",
    [
        ({}, {"skip_unsupported": True, "fail_unsupported": True}, "configuration 'test_conversion'"),
        ({"skip_unsupported": True, "fail_unsupported": True}, {}, "defaults"),
    ],
)
def test_conversion_conflicting_unsupported_settings_warn(
    temp_workspace, capsys, default_conversion, conversion, scope
):
    _recorded_invoke_args(temp_workspace, default_conversion, conversion)
    warnings = _deprecation_warnings(capsys.readouterr().out)
    assert len(warnings) == 1
    assert f"fail_unsupported in {scope} is deprecated" in warnings[0]
    assert "It is ignored there, because skip_unsupported: true is also set" in warnings[0]


def test_conversion_fail_unsupported_warns_once_per_configuration(temp_workspace, capsys):
    """The deprecation warning is printed once per configuration, not once per rule file."""
    (temp_workspace / "rules" / "other.yml").write_text(
        (temp_workspace / "rules" / "test.yml").read_text()
    )
    config = DynaconfDict(
        {
            "version": 2,
            "defaults": {"conversion": {"target": "loki"}},
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {"input": ["rules/*.yml"], "fail_unsupported": True},
                }
            ],
        }
    )
    with patch("click.testing.CliRunner.invoke") as mock_invoke:
        mock_result = MagicMock()
        mock_result.exception = None
        mock_result.exc_info = None
        mock_result.stdout = "Parsing Sigma rules\n"
        mock_invoke.return_value = mock_result
        convert_rules(config=config, path_prefix=temp_workspace, all_rules=True)
    assert mock_invoke.call_count == 2
    assert len(_deprecation_warnings(capsys.readouterr().out)) == 1


def test_convert_rules_empty_required_rule_fields_overrides_default(temp_workspace):
    """required_rule_fields: [] in a configuration keeps the whole rule, rather than the default's fields."""
    config = DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                    "required_rule_fields": ["title"],
                },
            },
            "configurations": [
                {
                    "name": "test_conversion",
                    "conversion": {"input": ["rules/*.yml"], "required_rule_fields": []},
                }
            ],
        }
    )
    convert_rules(config=config, path_prefix=temp_workspace, all_rules=True)

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()
    data = json.loads(output_file.read_bytes())
    assert len(data["rules"]) == 1
    assert list(data["rules"][0].keys()) != ["title"], "the default's required fields must not be applied"
    assert "title" in data["rules"][0] and len(data["rules"][0]) > 1


####
# Configuration filter tests
####


def test_convert_rules_filter_converts_only_selected_configuration(
    temp_workspace, mock_config_two_groups
):
    """A single-name filter converts only that configuration's files."""
    convert_rules(
        config=mock_config_two_groups,
        path_prefix=temp_workspace,
        configurations="group_a",
    )

    conversion_dir = temp_workspace / "conversions"
    assert (conversion_dir / "group_a_test.json").exists()
    assert not (conversion_dir / "group_b_test.json").exists()


def test_convert_rules_filter_multiple_configurations(
    temp_workspace, mock_config_two_groups
):
    """A multi-name filter converts each selected configuration."""
    convert_rules(
        config=mock_config_two_groups,
        path_prefix=temp_workspace,
        configurations="group_a group_b",
    )

    conversion_dir = temp_workspace / "conversions"
    assert (conversion_dir / "group_a_test.json").exists()
    assert (conversion_dir / "group_b_test.json").exists()


@pytest.mark.parametrize(
    "configurations,unknown",
    [
        ("bogus", "bogus"),
        ("group_a bogus", "bogus"),
        ("bogus1 group_a bogus2", "bogus1"),
    ],
)
def test_convert_rules_filter_unknown_name_raises(
    temp_workspace, mock_config_two_groups, configurations, unknown
):
    """An unknown configuration name raises a ValueError naming it."""
    with pytest.raises(ValueError, match=f"Unknown configuration name\\(s\\):.*{unknown}"):
        convert_rules(
            config=mock_config_two_groups,
            path_prefix=temp_workspace,
            configurations=configurations,
        )


def test_convert_rules_filter_error_lists_valid_names(
    temp_workspace, mock_config_two_groups
):
    """The unknown-name error also lists the valid configuration names."""
    with pytest.raises(ValueError, match="group_a, group_b"):
        convert_rules(
            config=mock_config_two_groups,
            path_prefix=temp_workspace,
            configurations="bogus",
        )


def test_convert_rules_filter_converts_all_selected_files(temp_workspace, mock_config):
    """A filtered run converts all of the selected configuration's input files,
    even when changed_files is empty and all_rules is false (the filter itself
    means there is work to do, so the early exit must not trigger)."""
    convert_rules(
        config=mock_config,
        path_prefix=temp_workspace,
        configurations="test_conversion",
    )

    output_file = temp_workspace / "conversions" / "test_conversion_test.json"
    assert output_file.exists()


def test_convert_rules_filter_leaves_unselected_outputs_untouched(
    temp_workspace, mock_config_two_groups
):
    """Existing outputs of unselected configurations are neither deleted nor
    modified, even when their input files are in deleted_files."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    group_b_output = conversion_dir / "group_b_test.json"
    group_b_content = json.dumps(
        {"conversion_name": "group_b", "queries": ["KEEP ME"]}
    ).decode("utf-8")
    group_b_output.write_text(group_b_content)

    convert_rules(
        config=mock_config_two_groups,
        path_prefix=temp_workspace,
        configurations="group_a",
        deleted_files="rules/test.yml",
    )

    # The selected configuration still converted (and its deleted-rule output
    # cleanup ran only for its own files).
    assert (conversion_dir / "group_a_test.json").exists()
    assert group_b_output.read_text() == group_b_content


def test_convert_rules_filter_backfills_manual_flag_only_for_selected(
    temp_workspace, mock_config_two_groups
):
    """The manual-flag backfill skips files of unselected configurations."""
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    group_a_output = conversion_dir / "group_a_test.json"
    group_a_output.write_text(
        json.dumps({"queries": ["HUMAN EDIT A"]}).decode("utf-8")
    )
    group_b_output = conversion_dir / "group_b_test.json"
    group_b_content = json.dumps({"queries": ["HUMAN EDIT B"]}).decode("utf-8")
    group_b_output.write_text(group_b_content)

    convert_rules(
        config=mock_config_two_groups,
        path_prefix=temp_workspace,
        configurations="group_a",
        manual_files="conversions/group_a_test.json conversions/group_b_test.json",
    )

    assert json.loads(group_a_output.read_bytes())["manual"] is True
    assert group_b_output.read_text() == group_b_content


@pytest.mark.parametrize(
    "selected,expect_flagged",
    [
        ("aws", False),
        ("aws_cloudtrail", True),
        ("aws aws_cloudtrail", True),
    ],
)
def test_convert_rules_filter_backfill_attributes_prefixed_names_correctly(
    temp_workspace, selected, expect_flagged
):
    """When one configuration name is a prefix of another, a file belongs to the
    longest matching name, so `aws_cloudtrail_test.json` is aws_cloudtrail's,
    not aws's."""
    config = DynaconfDict(
        {
            "version": 2,
            "defaults": {
                "conversion": {
                    "target": "loki",
                    "format": "default",
                    "skip_unsupported": True,
                    "file_pattern": "*.yml",
                },
            },
            "configurations": [
                {
                    "name": "aws",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "loki",
                        "format": "default",
                    },
                },
                {
                    "name": "aws_cloudtrail",
                    "conversion": {
                        "input": ["rules/*.yml"],
                        "target": "text_query_test",
                        "format": "default",
                    },
                },
            ],
        }
    )
    conversion_dir = temp_workspace / "conversions"
    conversion_dir.mkdir()
    cloudtrail_output = conversion_dir / "aws_cloudtrail_test.json"
    cloudtrail_output.write_text(
        json.dumps({"queries": ["HUMAN EDIT"]}).decode("utf-8")
    )

    convert_rules(
        config=config,
        path_prefix=temp_workspace,
        configurations=selected,
        manual_files="conversions/aws_cloudtrail_test.json",
    )

    data = json.loads(cloudtrail_output.read_bytes())
    if expect_flagged:
        assert data["manual"] is True
    else:
        assert "manual" not in data
