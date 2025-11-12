# Comment Sigma Results Script

Posts formatted comments to pull requests showing Sigma rule conversion/integration results. Extracts rule titles from JSON files and creates user-friendly PR comments with clickable links.

## Features

- Extracts `title` field from JSON files (supports top-level or nested in `rules` array)
- Creates clickable links to changed files in the PR
- Minimizes outdated comments from previous runs
- Automatically generates test results table from `TEST_RESULTS` JSON (when provided)

## Usage

### In GitHub Actions

Used by `actions/convert` and `actions/integrate`. The script is invoked with environment variables:

```bash
cd "${{ github.workspace }}/scripts/comment-sigma-results"
npm ci
node comment.js
```

### Local Testing

```bash
cd scripts/comment-sigma-results
npm install

export GITHUB_TOKEN="your-token"
export GITHUB_REPOSITORY="owner/repo"
export PULL_REQUEST_NUMBER="123"
export CHANGED_FILES="file1.json file2.json"
export DELETED_FILES="old.json"
export COMMENT_TITLE="Sigma Rule Conversions"
export COMMENT_IDENTIFIER="Sigma Rule Conversions"

node comment.js
```

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `PULL_REQUEST_NUMBER` | The pull request number | Yes |
| `CHANGED_FILES` | Space-separated list of changed file paths | Yes |
| `DELETED_FILES` | Space-separated list of deleted file paths | Yes |
| `COMMENT_TITLE` | Title for the comment section | Yes |
| `COMMENT_IDENTIFIER` | String to identify old comments for cleanup | Yes |
| `TEST_RESULTS` | JSON string of test results (object mapping file paths to arrays of QueryTestResult) | No |
| `GITHUB_TOKEN` | GitHub token for API access | Yes |
| `GITHUB_REPOSITORY` | Repository in format `owner/repo` | Yes (for local) |

## Output Format

```markdown
### Sigma Rule Conversions

| Changed | Deleted |
| --- | --- |
| 3 | 1 |

### Changed Files

- [Rule Title](link-to-file)
- [Another Rule](link-to-file)

### Deleted Files

- old_rule.json

### Test Results

| File name | Link | Result count | Errors |
| --- | --- | --- | --- |
| Rule Title | [See in Explore](link) | 42 | 0 |
```

The test results table is automatically included when `TEST_RESULTS` is provided.

## Dependencies

- `@actions/core` - GitHub Actions core utilities
- `@actions/github` - GitHub API client
- Node.js >= 18

