# Identify Commits Script

Identifies relevant commits in a pull request for change detection. Finds base commit, first commit, automation commits, and calculates previous refs used for git diff operations.

## Features

- Identifies base commit (parent of first PR commit)
- Tracks automation commits by author username
- Calculates previous refs for efficient change detection
- Outputs commit SHAs for use in git diff operations

## Usage

### In GitHub Actions

Used by `actions/convert` and `actions/integrate`. The script is invoked with environment variables:

```bash
cd "$REPO_ROOT/scripts/identify-commits"
npm ci
node identify-commits.js
```

### Local Testing

```bash
cd scripts/identify-commits
npm install

export GITHUB_TOKEN="your-token"
export GITHUB_REPOSITORY="owner/repo"
export PULL_REQUEST_NUMBER="123"
export ACTIONS_USERNAME="github-actions[bot]"

node identify-commits.js
```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `PULL_REQUEST_NUMBER` | The pull request number | Yes* | From `context.issue.number` in GitHub Actions |
| `ACTIONS_USERNAME` | Username to identify automation commits | No | `github-actions[bot]` |
| `GITHUB_TOKEN` | GitHub token for API access | Yes | - |
| `GITHUB_REPOSITORY` | Repository in format `owner/repo` | Yes (for local) | Auto in GitHub Actions |

*Required when running locally, automatically available in GitHub Actions context

## Outputs

The script outputs the following values to `$GITHUB_OUTPUT` (GitHub Actions) or stdout (local):

| Output | Description |
|--------|-------------|
| `base-commit` | Base commit SHA (parent of first PR commit) |
| `previous-commit` | Last commit before automation commits, or base if none |
| `first-commit` | First commit in the PR |
| `last-commit` | Last commit authored by automation user (empty if none) |
| `previous-ref` | Either `last-commit` or `previous-commit` (used for git diff) |

## How It Works

1. **Paginates through PR commits** using GitHub API
2. **Identifies base commit**: Parent of the first commit in the PR
3. **Tracks automation commits**: Finds commits authored by `ACTIONS_USERNAME`
4. **Calculates previous ref**: Uses last automation commit if exists, otherwise uses base commit
5. **Outputs results** for use in subsequent git operations

## Example Output

```
Base commit or base ref: abc123def456...
Last commit or base ref: xyz789ghi012...
PR First Commit: def456ghi789...
PR Last Commit by automation: jkl345mno678...
PR Previous Ref: jkl345mno678...
```

## Use Cases

- **Change Detection**: Determine which files changed since last automation run
- **Incremental Processing**: Only process files that changed
- **Commit Tracking**: Track automation commits to avoid reprocessing

## Dependencies

- `@actions/core` - GitHub Actions core utilities
- `@actions/github` - GitHub API client
- Node.js >= 18

