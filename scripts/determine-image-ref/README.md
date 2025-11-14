# Determine Image Reference Script

Determines the Docker image reference based on the GitHub action reference. Standardizes the logic for determining which Docker image tag to use across all actions.

## Usage

### In GitHub Actions

Used by `actions/convert`, `actions/integrate`, and `actions/deploy`. The script is invoked with the action reference:

```bash
REPO_ROOT="$(cd "$GITHUB_ACTION_PATH/../.." && pwd)"
"$REPO_ROOT/scripts/determine-image-ref/determine-image-ref.sh" "$ACTION_REF"
```

### Local Testing

```bash
./determine-image-ref.sh v1.2.3
# Output: image_ref=v1.2.3

./determine-image-ref.sh latest
# Output: image_ref=main

./determine-image-ref.sh abc123def456
# Output: image_ref=sha-abc123def456
```

## Input

- `ACTION_REF` - GitHub action reference (provided as command-line argument or environment variable)
  - Can be a version tag (e.g., `v1.2.3`)
  - Can be `latest`
  - Can be a commit SHA

## Output

The script outputs `image_ref` to `$GITHUB_OUTPUT` (in GitHub Actions) or stdout (locally).

### Logic

1. **Version Tag**: If `ACTION_REF` matches pattern `^v[0-9]+(\.[0-9]+)*(\.[0-9]+)?$` (e.g., `v1.2.3`)
   - Output: `image_ref=$ACTION_REF`

2. **Latest**: If `ACTION_REF` equals `latest`
   - Output: `image_ref=main` (standardized across all actions)

3. **SHA/Other**: For any other value (typically a commit SHA)
   - Output: `image_ref=sha-$ACTION_REF`

## Standardization

This script standardizes the behavior across all actions:
- **Before**: `integrate` and `deploy` used `latest`, while `convert` used `main`
- **After**: All actions use `main` when `ACTION_REF == "latest"`

## Examples

| ACTION_REF | image_ref Output |
|------------|------------------|
| `v1.2.3` | `v1.2.3` |
| `v0.1.0` | `v0.1.0` |
| `latest` | `main` |
| `abc123` | `sha-abc123` |
| `main` | `sha-main` |

