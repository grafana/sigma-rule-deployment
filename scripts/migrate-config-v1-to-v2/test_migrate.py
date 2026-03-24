import pytest
import yaml

from migrate_v1_to_v2 import migrate


# Minimal valid v1 config for targeted unit tests
_V1_MINIMAL = {
    "conversions": [
        {"name": "aws_cloudtrail", "input": "rules/cloud/aws/cloudtrail/*"},
    ],
}

# Reflects config/config-example.yml
_V1_FULL = {
    "folders": {
        "conversion_path": "./conversions",
        "deployment_path": "./deployments",
    },
    "conversion_defaults": {
        "target": "loki",
        "format": "default",
        "skip_unsupported": True,
        "file_pattern": "*.yml",
        "data_source": "grafanacloud-logs",
        "required_rule_fields": ["level", "logsource", "author"],
    },
    "conversions": [
        {
            "name": "aws_cloudtrail",
            "input": "rules/cloud/aws/cloudtrail/*",
            "backend_options": {"case_sensitive": True},
            "pipelines": [
                "pipelines/cloud/aws/cloudtrail/filter_permitted_aws_accounts.yml",
                "pipelines/datasources/aws_cloudtrail.yml",
            ],
            "rule_group": "Every 5 Minutes",
            "time_window": "5m",
        },
        {
            "name": "gcp_k8s",
            "input": "rules/cloud/kubernetes/audit/*",
            "pipeline_check": False,
            "encoding": "utf-8",
            "pipelines": ["pipelines/cloud/gcp/audit/gcp_kubernetes_audit_logs.yml"],
            "rule_group": "Every 5 Minutes",
            "time_window": "5m",
        },
        {
            "name": "okta_audit",
            "input": "rules/cloud/okta/*",
            "backend_options": {"case_sensitive": False, "add_line_filters": True},
            "pipelines": ["loki_okta_system_log"],
            "rule_group": "Every 1 Hour",
            "time_window": "1h",
            "data_source": "okta-loki",
            "query_model": '{"refId":"%s","datasource":{"type":"loki","uid":"%s"},"query":"%s"}',
        },
    ],
    "integration": {
        "folder_id": "XXXX",
        "org_id": 1,
        "test_queries": True,
        "from": "now-1h",
        "to": "now",
        "template_labels": {
            "Level": "{{.Level}}",
            "Product": "{{.Logsource.Product}}",
            "Service": "{{.Logsource.Service}}",
        },
        "template_annotations": {"Author": "{{.Author}}"},
        "template_all_rules": False,
    },
    "deployment": {
        "grafana_instance": "https://myinstance.grafana.com",
        "timeout": "10s",
    },
}


def test_version_set_to_2():
    result = migrate(_V1_MINIMAL)
    assert result["version"] == 2


def test_already_v2_raises():
    with pytest.raises(ValueError, match="already v2"):
        migrate({"version": 2, "configurations": []})


def test_folders_preserved():
    result = migrate(_V1_FULL)
    assert result["folders"] == _V1_FULL["folders"]


def test_conversion_defaults_split():
    result = migrate(_V1_FULL)
    conv = result["defaults"]["conversion"]
    # Pure conversion fields stay
    assert conv["target"] == "loki"
    assert conv["skip_unsupported"] is True
    assert conv["required_rule_fields"] == ["level", "logsource", "author"]
    # Integration-bound field moves out
    assert "data_source" not in conv


def test_integration_defaults_merged():
    result = migrate(_V1_FULL)
    intg = result["defaults"]["integration"]
    # Lifted from conversion_defaults
    assert intg["data_source"] == "grafanacloud-logs"
    # From v1 integration block (takes precedence)
    assert intg["folder_id"] == "XXXX"
    assert intg["org_id"] == 1
    assert intg["template_labels"]["Level"] == "{{.Level}}"


def test_deployment_defaults_preserved():
    result = migrate(_V1_FULL)
    assert result["defaults"]["deployment"] == _V1_FULL["deployment"]


def test_configurations_count():
    result = migrate(_V1_FULL)
    assert len(result["configurations"]) == 3


def test_configuration_names():
    result = migrate(_V1_FULL)
    names = [c["name"] for c in result["configurations"]]
    assert names == ["aws_cloudtrail", "gcp_k8s", "okta_audit"]


def test_configuration_conversion_fields():
    result = migrate(_V1_FULL)
    aws = result["configurations"][0]
    assert aws["conversion"]["input"] == "rules/cloud/aws/cloudtrail/*"
    assert aws["conversion"]["backend_options"] == {"case_sensitive": True}
    assert "pipelines" in aws["conversion"]


def test_configuration_integration_fields_split_out():
    result = migrate(_V1_FULL)
    aws = result["configurations"][0]
    # rule_group and time_window move to integration sub-block
    assert "rule_group" not in aws["conversion"]
    assert "time_window" not in aws["conversion"]
    assert aws["integration"]["rule_group"] == "Every 5 Minutes"
    assert aws["integration"]["time_window"] == "5m"


def test_configuration_without_integration_fields():
    result = migrate(_V1_FULL)
    gcp = result["configurations"][1]
    # gcp_k8s has no integration-bound fields at item level in v1 (rule_group/time_window
    # are present in v1 but move to integration)
    assert gcp["integration"]["rule_group"] == "Every 5 Minutes"


def test_okta_query_model_in_integration():
    result = migrate(_V1_FULL)
    okta = result["configurations"][2]
    assert "query_model" in okta["integration"]
    assert "query_model" not in okta["conversion"]


def test_no_defaults_when_absent():
    result = migrate(_V1_MINIMAL)
    assert "defaults" not in result


def test_configurations_key_present():
    result = migrate(_V1_MINIMAL)
    assert "configurations" in result
