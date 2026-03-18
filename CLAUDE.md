# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Does

A GitHub Actions suite that converts [Sigma rules](https://sigmahq.io/) to Grafana Alerting rules and deploys them via the Grafana API. It implements Detection as Code (DaC): security rules live in Git, get converted via sigma-cli, and are automatically provisioned to Grafana on merge.

## Commands

### Go

```bash
go build -ldflags="-s -w" -o /build/sigma-deployer ./cmd/sigma-deployer
go test ./...
```

### Python (Convert action)

```bash
uv sync --directory actions/convert          # Install dependencies
uv run --directory actions/convert pytest -vv .   # Run tests
```

### Make

```bash
make test-convert   # Test convert action with example config
make test           # Run full pytest suite
```

### Linting

- Go: `golangci-lint run` (configured in `.golangci.yml`)
- Python: `ruff` and `mypy` (configured in `pyproject.toml`)

## Architecture

The pipeline runs in four stages, each implemented as a separate GitHub Actions composite or Go subcommand:

```
Config YAML → [Convert] → conversions/*.json → [Integrate] → deployments/alert_rule_*.json → [Deploy] → Grafana API
```

1. **Convert** (`actions/convert/`, Python) — Wraps sigma-cli. Reads the config, globs input Sigma rule files, runs them through the sigma-cli backend (Loki, Elasticsearch, etc.), and writes JSON with queries and rule metadata to `conversions/`.

2. **Integrate** (`internal/integrate/`, Go subcommand `sigma-deployer integrate`) — Reads conversion JSON, generates Grafana alerting rule JSON (groups, time windows, labels, annotations), optionally tests queries against the datasource, and writes to `deployments/`.

3. **Deploy** (`internal/deploy/`, Go subcommand `sigma-deployer deploy`) — Reads alert rule files, diffs against current state, and calls the Grafana Alert Provisioning API (create/update/delete).

4. **Query Test** (`internal/querytest/`) — Called during integrate to validate converted queries against Loki or Elasticsearch before committing to deployment.

### Key directories

| Path | Purpose |
|------|---------|
| `cmd/sigma-deployer/` | Main Go binary entry point; dispatches to integrate/deploy subcommands |
| `internal/integrate/` | Alert rule generation from conversion output |
| `internal/deploy/` | Grafana API client for provisioning |
| `internal/model/` | Shared data structures (config, rules, Grafana API types) |
| `internal/querytest/` | Query validation against datasources |
| `actions/convert/` | Python sigma-cli wrapper action |
| `actions/integrate/`, `actions/deploy/` | Composite GitHub Actions wrappers |
| `shared/` | Shared Go utilities: config loading, HTTP client, helpers |
| `config/` | Example configs and JSON schema |
| `.github/workflows/` | Reusable workflow definitions |
| `integration-test/` | End-to-end integration tests |

### Configuration

The config uses a flat structure with `conversion_defaults` + `conversions[]` list. See `config/config-example.yml` for an annotated example. The JSON schema in `config/schema.json` documents the format.

### Data flow details

- sigma-cli outputs are captured and parsed from stdout by the Python action.
- Conversion results are written as JSON files; each file contains the converted query, sigma rule metadata (ID, title, tags, level, author), and source path.
- The Go integrator reads these JSON files and maps them to Grafana alert rule structures, applying template functions for dynamic labels/annotations.
- Incremental mode (default): only changed files are re-converted/re-deployed.
- Fresh deploy mode: destructively resets all alerts in the target folder (for drift recovery).

## Release Process

1. Update action version references in `.github/workflows/*.yml`
2. Create a signed tag: `git tag --sign vX.X.X`
3. Push the tag and create a GitHub Release
4. Docker image is automatically pushed to GHCR on release
