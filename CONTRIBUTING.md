# Contributing to Sigma Rule Deployment

Thanks for contributing! This document covers local development on **this** repository —
the Action suite itself. If you're looking to _use_ the Actions in your own rule repo,
see the [documentation](docs/README.md) instead.

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
| `docs/`                                                       | User documentation                                                       | Markdown  |
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
[`config/schema-v2.json`](config/schema-v2.json) and
[`config/config-example-v2.yml`](config/config-example-v2.yml) together, since the
[`validate`](actions/validate/README.md) action checks user configs against the schema.

**Documentation.** User documentation lives in [`docs/`](docs/README.md), and each Action's
`README.md` is the reference for its inputs and outputs. If you change inputs, outputs, or
behaviour, update the Action's README and the affected `docs/` pages in the same PR. A
configuration change also updates the
[configuration reference](docs/configure/configuration-reference.md).

## Releasing

We use Git tags to mark released versions, and automation to push the matching Docker image and
attach a Software Bill of Materials (SBOM) to the release.

This repository has [immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)
enabled, so a release's assets are locked once it's published and can't be added, changed, or
deleted. The SBOM must therefore be attached while the release is still a **draft**. Pushing the
release tag does this for you: it creates the draft release and attaches the SBOM for you to
review before publishing.

When `main` is ready to release:

1. Choose the version number using [Semantic Versioning](https://semver.org/). Version numbers
   must match `\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?`, for example `v1.2.0` or `v1.2.0-beta.1`.
2. Check out `main` and create a signed tag named after the version with a `v` prefix, for example
   `git tag --sign --message="Release vX.Y.Z" vX.Y.Z`.
3. Push the tag, for example `git push --tags`. This triggers the
   [SBOM on Release](.github/workflows/sbom-immutable-release.yml) workflow, which exports the
   SPDX SBOM from Socket, creates a **draft** release with generated notes, and attaches the SBOM
   as the `sigma-rule-deployment-vX.Y.Z.spdx.json` asset. The release is marked as a pre-release
   when the tag starts with `v0.` or has any `-` suffix, such as `-alpha`, `-beta`, or `-rc1`.
4. Open the draft release in GitHub. Review the generated release notes, check the pre-release
   flag, and confirm the `.spdx.json` SBOM asset is attached. It must be there now, because
   immutable releases prevent adding assets after publishing.
5. Publish the draft release.
6. Check that the [Build & Integration Test Image](.github/workflows/build-docker.yml) workflow,
   which pushes the tagged image to the GitHub Container Registry (GHCR) on publish, completed
   successfully for the release.
