# Parse Sigma Comment Script

Parses `sigma convert` / `sigma integrate` commands from PR issue comments and resolves targets against the v2 configuration file, producing outputs that drive the convert/integrate workflow.

## Command Grammar

Keywords (`sigma`, `convert`, `integrate`, `all`, flags) are case-insensitive; configuration name matching is case-sensitive.

```
sigma convert all
sigma convert <target> [--no-test] [--debug] [--query-time-window <duration>]
sigma integrate all
sigma integrate <target> [--no-test] [--debug] [--query-time-window <duration>]
```

- `convert` runs conversion then integration; `integrate` runs integration only.
- `<target>` is either a configuration name from the v2 config or a name prefix: `prod` matches every configuration whose name starts with `prod`. Exact matches win over prefix matches.
- `--query-time-window` takes a duration like `30m`, `6h`, `1d`, `1d12h` (digits followed by s/m/h/d units, possibly repeated).

## Behavior

1. If the `ALL_RULES` input is `"true"` (non-comment trigger), outputs `triggered=true, action=convert, all_rules=true`.
2. Untrusted commenters are ignored (`triggered=false`). This check is security-critical. Trusted means: bots (only installed GitHub Apps and the repo's own workflows can comment as a bot), or humans with `OWNER`, `MEMBER` or `COLLABORATOR` association.
3. Comments not starting with `sigma` are ignored (`triggered=false`).
4. On a valid command, the parsed outputs are set and an "eyes" reaction is added to the comment (best effort).
5. On a parse error (unknown action/target/flag, invalid duration, unresolvable target), `triggered=false` and an `error` output are set, a "confused" reaction is added, and a reply comment is posted showing the usage grammar (plus valid configuration names for unresolvable targets). Parse errors never fail the workflow; only real execution errors (e.g. missing config file) do.

## Usage

### In GitHub Actions

```bash
cd "$REPO_ROOT/scripts/parse-sigma-comment"
npm ci
node parse-sigma-comment.js
```

### Local Testing

```bash
cd scripts/parse-sigma-comment
npm install

node parse-sigma-comment.js \
  --comment-body "sigma convert aws_cloudtrail --no-test" \
  --comment-author-association MEMBER \
  --comment-id 123 \
  --issue-number 456 \
  --config-path ../../config/config-example-v2.yml \
  --github-token "your-token"
```

## Environment Variables / CLI Arguments

| Variable | CLI argument | Description | Required | Default |
|----------|--------------|-------------|----------|---------|
| `COMMENT_BODY` | `--comment-body` | The issue comment text | Yes (comment triggers) | From event payload in GitHub Actions |
| `COMMENT_AUTHOR_ASSOCIATION` | `--comment-author-association` | Comment author's association (OWNER, MEMBER, ...) | Yes (comment triggers) | From event payload in GitHub Actions |
| `COMMENT_AUTHOR_TYPE` | `--comment-author-type` | Comment author's type (`Bot` or `User`) | No | From event payload in GitHub Actions |
| `COMMENT_ID` | `--comment-id` | Comment ID, used for reactions | No | From event payload in GitHub Actions |
| `ISSUE_NUMBER` | `--issue-number` | PR/issue number, used for reply comments | No | From GitHub Actions context |
| `ALL_RULES` | `--all-rules` | Workflow `all_rules` input (`"true"`/`"false"`) | No | `false` |
| `CONFIG_PATH` | `--config-path` | Path to the v2 config YAML | No | `config/config.yml` |
| `GITHUB_TOKEN` | `--github-token` | GitHub token for reactions/replies | No | - |

## Outputs

The script outputs the following values to `$GITHUB_OUTPUT` (GitHub Actions) or stdout (local):

| Output | Description |
|--------|-------------|
| `triggered` | `true` when a valid command (or `all_rules` input) should trigger the workflow |
| `action` | `convert` (conversion + integration) or `integrate` (integration only) |
| `all_rules` | `true` when the target is `all`; `configurations` is then empty (no filter) |
| `configurations` | Space-separated configuration names the target resolved to (empty for `all`) |
| `no_test` | `true` when `--no-test` was passed |
| `debug` | `true` when `--debug` was passed |
| `query_time_window` | Value of `--query-time-window` (empty when not passed) |
| `error` | Human-readable parse error message (empty on success) |

## Target Resolution

1. Exact (case-sensitive) match against `configurations[].name` → that single configuration.
2. Otherwise, prefix match: every configuration whose name starts with `<target>` → all matching configurations, in config file order. There is deliberately no separator boundary, so any naming style groups by prefix — at the cost of over-matches such as `dev` also selecting `devops_runners`. Exact matches always win, and the resolved names are logged, so over-selection is visible.
3. No matches → parse error; the reply comment lists the valid configuration names.

The unit of work is always the whole configuration: a target selects configurations, never individual rule files. Since a Sigma rule can appear in several configurations' inputs, rule-level targeting would be ambiguous and is intentionally not supported.

## Dependencies

- `@actions/core` - GitHub Actions core utilities
- `@actions/github` - GitHub API client
- `js-yaml` - v2 config parsing
- Node.js >= 18

## Tests

```bash
npm test
```
