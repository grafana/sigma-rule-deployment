# AGENTS.md

Guidance for AI agents working in this repository. People should start with [`README.md`](README.md) and [`CONTRIBUTING.md`](CONTRIBUTING.md). This file adds what those don't say. This repository is public: do not reference internal systems, private repositories or credentials in code, docs, commits or PRs.

## What this repo is

A suite of GitHub Actions that turns Sigma rules into Grafana alert rules. A rule repository holds the Sigma rules and one `config.yml`. The Actions convert, test and deploy them through pull requests. For the concepts, read [`docs/concepts/how-it-works.md`](docs/concepts/how-it-works.md). For the config format, read [`docs/configure/README.md`](docs/configure/README.md) and the [configuration reference](docs/configure/configuration-reference.md).

## Where the code is

```
Sigma rule + config.yml
  │ convert    actions/convert/                 Python, sigma-cli       → <CONFIG>_<RULE>.json
  ▼
  │ integrate  internal/integrate/ + internal/querytest/   Go          → alert_rule_<CONFIG>_<RULE>_<UID>.json
  ▼                                           (committed back to the PR branch)
  │ deploy     internal/deploy/                 Go, on merge to main    → Grafana provisioning API
```

- One Docker image holds both runtimes. `entrypoint.sh` dispatches `convert`, `integrate` and `deploy`, and `cmd/sigma-deployer/` is the Go entrypoint. For `convert`, `entrypoint.sh` rejects any plugin package whose name doesn't start with `pysigma-`.
- `actions/*/action.yml` are thin composite Actions around the image. `.github/workflows/` and `config/` hold the reusable workflows and the schema. `shared/` holds config loading, the Grafana HTTP client and utilities.
- The layout table in `CONTRIBUTING.md` lists the rest.

## Behaviour to know before changing code

- **Alert files route to a configuration by filename.** Deploy picks the configuration whose `<name>_` is the longest prefix of the file name (`shared/util.go:170-183`, called at `internal/deploy/deployer.go:110`). It takes the alert UID from the last `_segment` before `.json` (`internal/deploy/deployer.go:22`). Renaming a configuration, or naming one as a prefix of another, changes routing and what gets deleted. Configuration names allow only letters, digits, `-` and `_`.
- **Lists and maps in a configuration replace the defaults.** They are not merged (`docs/configure/README.md`, "How defaults and overrides combine").
- **Deletions read the previous config file.** Removing a configuration and its alert files in one change still cleans up the instances the old config named (`docs/use/deployments.md`).
- **`fresh_deploy` deletes every alert rule in the configured Grafana folders**, including ones this tool didn't create (`docs/use/deployments.md`).
- **Files marked `manual` are never overwritten or deleted** by the Actions (`docs/use/manual-modifications.md`).
- **Generated files are owned by the Actions.** They regenerate conversion and alert rule files on every run, so don't hand-edit them as part of a code change.

## Rules

- **Ask before any breaking change to the public contract.** That means Action inputs and outputs, `config/schema-v2.json`, generated file names, and the `sigma <command>` comment syntax. If a break is approved, update the schema, `config/config-example-v2.yml`, the Action's `README.md`, the affected `docs/` pages and any migration notes in the same PR, and say so in the PR body.
- **Never push tags, publish releases, or run a fresh deployment.** Releases are immutable and a pushed tag starts the release workflow (`CONTRIBUTING.md`, "Releasing"). To check a workflow or composite Action change, open the PR and read the automatic integration-test PR it creates (`CONTRIBUTING.md`, "Testing changes to the Actions themselves"). Locally, run only unit tests and linters.
- **Follow the repo conventions in `CONTRIBUTING.md`:** pin every `uses:` to a full commit SHA with the version in a trailing comment, keep top-level `permissions: {}` and grant per job, and pass workflow inputs to `run:` steps through `env:` instead of interpolating `${{ }}` into shell.
- **Update docs in the same PR as the behaviour.** A config change also updates the configuration reference.

## Testing

Commands and tooling are in `CONTRIBUTING.md`, "Running the tests". CI is the source of truth (`.github/workflows/unit-test.yml`).

## Known issues

- The `Makefile` targets assume a sibling checkout of a rule repository, and `test-convert` passes `config/sigma-convert.example.yml`, which is not in `config/`. Use the commands in `CONTRIBUTING.md` instead of `make test` and `make test-convert`.
- CI runs `go test` only for `./internal/integrate/...`, `./internal/deploy/...` and `./internal/querytest/...` (`.github/workflows/unit-test.yml:98,125,152`). The tests in `shared/` run in no workflow, so run `go test ./shared/...` yourself when you change that package.
- The layout table in `CONTRIBUTING.md` omits `scripts/parse-sigma-comment`, `scripts/mask-grafana-tokens` and `scripts/migrate-config-v1-to-v2`.

Each of these is a separate fix, not part of a docs change.
