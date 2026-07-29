# Contributing to Sigma Rule Deployment

Thanks for contributing! This document covers local development on **this** repository —
the Action suite itself. If you're looking to _use_ the Actions in your own rule repo,
see [GettingStarted.md](GettingStarted.md) instead.

## Repository layout

This repo ships one Docker image containing two runtimes, plus a set of thin composite
Actions and helper scripts that wrap it.

| Path                                                          | What it is                                                               | Language  |
| ------------------------------------------------------------- | ------------------------------------------------------------------------ | --------- |
| `cmd/sigma-deployer/`                                         | Entrypoint for the unified Go binary (`integrate`, `deploy` subcommands) | Go        |
| `internal/integrate/`                                         | Converts conversion output into Grafana alert rule definitions           | Go        |
| `internal/deploy/`                                            | Provisions alert rules to Grafana                                        | Go        |
| `internal/querytest/`                                         | Query testing + "Link to Explore" generation                             | Go        |
| `internal/model/`                                             | Shared config/alert types                                                | Go        |
| `shared/`                                                     | Config loading, Grafana HTTP client, utilities                           | Go        |
| `actions/convert/`                                            | Sigma → query conversion via sigma-cli                                   | Python    |
| `actions/{convert,integrate,deploy,validate}/action.yml`      | Composite Actions                                                        | YAML      |
| `actions/sigma-validation/`                                   | Documentation only — wraps the upstream SigmaHQ validator                | —         |
| `scripts/comment-sigma-results/`, `scripts/identify-commits/` | PR comment handling and commit identification                            | Node      |
| `scripts/determine-image-ref/`                                | Maps an action ref to a Docker image tag                                 | Bash      |
| `.github/scripts/`                                            | Branch creation and verified-commit helpers used by workflows            | Bash      |
| `config/`                                                     | Config JSON schema, example config, reusable workflow templates          | YAML/JSON |
| `integration-test/`                                           | Fixtures copied into the integration test repo on every PR               | YAML/JSON |
| `Dockerfile`, `entrypoint.sh`                                 | The published image and its subcommand dispatch                          | —         |

## Prerequisites

- **Go** (check `go.mod` for the required version)
- **[uv](https://docs.astral.sh/uv/)** for the Python action. `uv` provisions its own
  interpreter, so you don't need a system Python matching `requires-python`.
- **Node.js** for the two JS scripts. (check `package.json` for minimum version)
- **Docker** — only needed if you want to build or run the image locally.
- **`shellcheck`**, **`hadolint`**, **`actionlint`**, **`golangci-lint`** — optional, but each
  is enforced by a CI workflow, so having them locally saves round-trips.

## Running the tests

CI ([`.github/workflows/unit-test.yml`](.github/workflows/unit-test.yml)) is the source of
truth. To reproduce it locally:

### Go

```bash
go get ./...
go test ./internal/integrate/... ./internal/deploy/... ./internal/querytest/...
golangci-lint run --timeout=5m
```

Lint config lives in [`.golangci.yml`](.golangci.yml). Note that it uses `default: none` with
an explicit enable list, and enables `gofumpt` and `goimports` as formatters — run
`golangci-lint fmt` (or `gofumpt -w .`) before pushing.

Go tests use table-driven `testify` assertions with fixtures under
`internal/*/testdata/`. When you add a fixture, prefer extending an existing `testdata`
config over introducing a parallel one.

### Python (`actions/convert`)

```bash
uv sync -q --directory actions/convert
uv sync -q --directory actions/convert --group dev
uv run --directory actions/convert ruff check .
uv run --directory actions/convert mypy .
uv run --directory actions/convert pytest -v .
```

There's also a `make test` shortcut, which wraps the `pytest` invocation above.

### Node scripts

```bash
cd scripts/comment-sigma-results && npm ci && npm test
cd scripts/identify-commits    && npm ci && npm test
```

## Running the conversion against real rules

Unit tests cover conversion logic, but for anything touching sigma-cli behaviour, backend
options, or pipeline resolution, it's worth converting real rules. The Python action resolves
paths relative to `PATH_PREFIX` (defaulting to `GITHUB_WORKSPACE`, then `.`), so you can point
it at a rule repository:

```bash
GITHUB_WORKSPACE=$(realpath integration-test) \
  uv run --directory actions/convert main.py --config config.yml
```

## Testing changes to the Actions themselves

Composite Action and workflow changes can't be validated by unit tests. Two mechanisms exist:

1. **Automatic integration test.** Every PR triggers
   [`build-docker.yml`](.github/workflows/build-docker.yml), which builds and pushes a
   PR-tagged image to GHCR, then opens a PR against
   `grafana/sigma-rule-deployment-integration-test` with the contents of
   [`integration-test/`](integration-test/) and the action refs rewritten to your commit SHA.
   Check that PR's workflow runs to see your change end to end. Merging or closing your PR
   triggers [`close-integration-test.yml`](.github/workflows/close-integration-test.yml) to
   clean up.
2. **Manual pinning.** Point a scratch repo's workflow at
   `grafana/sigma-rule-deployment/actions/convert@<your-sha>`.
   [`scripts/determine-image-ref`](scripts/determine-image-ref/README.md) decides which image
   tag that resolves to: a version tag → that tag, `latest` → `main`, and a raw SHA →
   `sha-<sha>`.

If your change alters behaviour the integration test should cover, add or update fixtures in
[`integration-test/`](integration-test/) in the same PR — including
`integration-test/manual-fixtures/` if you touch the manual-modification preservation logic.

## Conventions

**Pinned Action SHAs.** Every `uses:` in this repo pins a full commit SHA with the version in
a trailing comment (`uses: actions/checkout@9c091bb... # v7.0.0`). Renovate maintains these;
match the format exactly when adding a new one.

**Least-privilege workflows.** Workflows declare `permissions: {}` at the top level and grant
the minimum per job. Keep it that way.

**Untrusted input.** Pass workflow inputs to `run:` steps via `env:` and reference them as
`"${INPUTS_FOO}"` rather than interpolating `${{ }}` directly into shell — see
[`actions/validate/action.yml`](actions/validate/action.yml) for the pattern.

**Config schema.** User-facing config changes must update
[`config/schema.json`](config/schema.json) and
[`config/config-example.yml`](config/config-example.yml) together, since the
[`validate`](actions/validate/README.md) action checks user configs against the schema.

**Documentation.** Each Action's `README.md` is its reference documentation. If you change
inputs, outputs, or behaviour, update that README in the same PR — and
[GettingStarted.md](GettingStarted.md) too if the change affects setup.

## Releasing

See [Releasing](README.md#releasing) in the README. Note that releases are immutable, so the
SBOM must be attached while the release is still a draft — the tag push automates this.
