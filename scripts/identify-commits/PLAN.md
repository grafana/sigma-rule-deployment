# Plan: Extract "Identify Relevant Commits" Step to Script

## Current State Analysis

### Usage in Actions

**1. `actions/convert/action.yml` (lines 52-93):**
```yaml
- name: Identify relevant commits
  if: ${{ inputs.changed_files_from_base == 'false' }}
  id: commits
  uses: actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd # v8.0.0
  env:
    ACTIONS_USERNAME: ${{ inputs.actions_username }}
  with:
    script: |
      const iterator = await github.paginate(github.rest.pulls.listCommits.endpoint.merge({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: context.issue.number,
        per_page: 1
      }));
      let previous_commit = "";
      let first_commit = "";
      let last_commit = "";
      let base_commit = "";
      for (const remote_commit of iterator) {
        if (previous_commit === "" && remote_commit.parents.length > 0) {
          previous_commit = remote_commit.parents[0].sha;
          base_commit = remote_commit.parents[0].sha;
          first_commit = remote_commit.sha;
        }
        if (remote_commit.commit.author.name === process.env.ACTIONS_USERNAME) {
          previous_commit = remote_commit.sha;
          last_commit = remote_commit.sha;
        }
      }
      console.log(`Base commit or base ref: ${base_commit}`);
      console.log(`Last commit or base ref: ${previous_commit}`);
      console.log(`PR First Commit: ${first_commit}`);
      console.log(`PR Last Commit by automation: ${last_commit}`);
      console.log(`PR Previous Ref: ${last_commit || previous_commit}`);

      core.setOutput('base-commit', base_commit);
      core.setOutput('previous-commit', previous_commit);
      core.setOutput('first-commit', first_commit);
      core.setOutput('last-commit', last_commit);
      core.setOutput('previous-ref', last_commit || previous_commit);

      return {};
```

**2. `actions/integrate/action.yml` (lines 84-125):**
```yaml
- name: Identify relevant commits
  if: ${{ inputs.changed_files_from_base == 'false' }}
  id: commits
  uses: actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd # v8.0.0
  env:
    ACTIONS_USERNAME: ${{ inputs.actions_username }}
  with:
    script: |
      const iterator = await github.paginate(github.rest.pulls.listCommits.endpoint.merge({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: context.issue.number,
        per_page: 1
      }));
      let previous_commit = "";
      let first_commit = "";
      let last_commit = "";
      let base_commit = "";
      for (const remote_commit of iterator) {
        if (previous_commit === "" && remote_commit.parents.length > 0) {
          previous_commit = remote_commit.parents[0].sha;
          base_commit = remote_commit.parents[0].sha;
          first_commit = remote_commit.sha;
        }
        if (remote_commit.commit.author.name === process.env.ACTIONS_USERNAME) {
          previous_commit = remote_commit.sha;
          last_commit = remote_commit.sha;
        }
      }
      console.log(`Last commit or base ref: ${previous_commit}`);
      console.log(`PR First Commit: ${first_commit}`);
      console.log(`PR Last Commit by automation: ${last_commit}`);
      console.log(`PR Previous Ref: ${last_commit || previous_commit}`);
      console.log(`PR Base Ref: ${base_commit}`);

      core.setOutput('previous-commit', previous_commit);
      core.setOutput('first-commit', first_commit);
      core.setOutput('last-commit', last_commit);
      core.setOutput('previous-ref', last_commit || previous_commit);
      core.setOutput('base-ref', base_commit);

      return {};
```

### Functionality Analysis

The script:
1. **Paginates through PR commits** using GitHub API
2. **Identifies key commits:**
   - `base_commit`: Parent of the first commit in the PR (the base branch commit)
   - `first_commit`: First commit in the PR
   - `previous_commit`: Tracks the last commit before automation commits, or base if none
   - `last_commit`: Last commit authored by the automation user
   - `previous_ref`: Either `last_commit` (if automation commits exist) or `previous_commit`
3. **Outputs** these values to GitHub Actions outputs

### Key Differences Between Versions

| Aspect | Convert Action | Integrate Action |
|--------|---------------|------------------|
| Output Name | `base-commit` | `base-ref` |
| Console Logs | 5 log statements | 5 log statements (slightly different order) |
| Logic | Identical | Identical |

**Naming Inconsistency:**
- Convert uses `base-commit` output
- Integrate uses `base-ref` output
- Both refer to the same value (base commit SHA)

**Usage:**
- Convert: Uses `base-commit` in "Generate Comment Data" step
- Integrate: Uses `base-ref` in "Calculate Change Files" and "Generate Comment Data" steps

### Dependencies

- `@actions/github` - GitHub API client (via `actions/github-script`)
- `@actions/core` - For setting outputs (via `actions/github-script`)
- GitHub context (automatically available in GitHub Actions)

## Proposed Solution

### Script Location
`scripts/identify-commits/identify-commits.js`

### Script Design

**Language:** Node.js/JavaScript
- Uses `@actions/github` and `@actions/core` packages (same as `actions/github-script`)
- Can be run standalone or in GitHub Actions
- Similar approach to `comment-sigma-results` script

**Inputs (Environment Variables):**
- `PULL_REQUEST_NUMBER` - PR number (required, or from `context.issue.number` in GitHub Actions)
- `ACTIONS_USERNAME` - Username to identify automation commits (required, default: "github-actions[bot]")
- `GITHUB_TOKEN` - GitHub token for API access (required)
- `GITHUB_REPOSITORY` - Repository in format `owner/repo` (required for local, auto in GitHub Actions)

**Outputs (to `$GITHUB_OUTPUT` or stdout):**
- `base-commit` - Base commit SHA (parent of first PR commit)
- `base-ref` - Same as `base-commit` (for backward compatibility)
- `previous-commit` - Last commit before automation commits, or base if none
- `first-commit` - First commit in the PR
- `last-commit` - Last commit authored by automation user (empty if none)
- `previous-ref` - Either `last_commit` or `previous_commit`

**Script Logic:**
1. Get PR number from context or env var
2. Initialize GitHub client
3. Paginate through PR commits
4. Process commits to identify:
   - Base commit (first commit's parent)
   - First commit
   - Automation commits (by author name)
   - Previous commit tracking
5. Calculate `previous-ref` (last automation commit or previous commit)
6. Output all values to `$GITHUB_OUTPUT` (GitHub Actions) or stdout (local)

### Implementation Plan

#### Phase 1: Create Script Structure
1. Create `scripts/identify-commits/` directory
2. Create `identify-commits.js` script with:
   - Input validation
   - GitHub API integration
   - Commit analysis logic
   - Output formatting
   - Error handling
3. Create `package.json` with dependencies:
   - `@actions/core`
   - `@actions/github`
4. Add `.gitignore` for `node_modules/`

#### Phase 2: Update Actions
1. **Update `actions/convert/action.yml`:**
   - Replace "Identify relevant commits" step with script invocation
   - Install dependencies if needed
   - Pass required environment variables

2. **Update `actions/integrate/action.yml`:**
   - Replace "Identify relevant commits" step with script invocation
   - Install dependencies if needed
   - Pass required environment variables

#### Phase 3: Standardize Output Names
- **Option A (Recommended):** Output both `base-commit` and `base-ref` for backward compatibility
- **Option B:** Standardize on `base-ref` and update convert action to use it
- **Option C:** Standardize on `base-commit` and update integrate action to use it

**Recommendation:** Option A - output both names to maintain backward compatibility without breaking changes.

#### Phase 4: Testing & Documentation
1. Create README.md documenting:
   - Purpose
   - Usage
   - Inputs/Outputs
   - Examples
2. Create unit tests for:
   - Commit analysis logic
   - Edge cases (no automation commits, single commit, etc.)
3. Test in GitHub Actions workflows

### Script Implementation Details

**Error Handling:**
- Validate PR number is provided
- Validate GitHub token is provided
- Handle API errors gracefully
- Handle edge cases (no commits, no automation commits, etc.)

**Output Format:**
- In GitHub Actions: Write to `$GITHUB_OUTPUT`
- Locally: Write to stdout in format `key=value` (one per line)

**Edge Cases:**
- PR with no commits (shouldn't happen, but handle gracefully)
- PR with no automation commits (`last_commit` will be empty)
- PR with single commit
- PR where first commit has no parent (merge commit edge case)

**Console Logging:**
- Maintain existing console.log statements for debugging
- Can be controlled via environment variable if needed

### Benefits

1. **DRY Principle:** Eliminates code duplication between actions
2. **Maintainability:** Single source of truth for commit identification logic
3. **Testability:** Can be unit tested independently
4. **Consistency:** Ensures both actions use identical logic
5. **Reusability:** Can be used in other workflows
6. **Standardization:** Opportunity to fix naming inconsistency

### Considerations

1. **GitHub API Rate Limits:** Script uses pagination, should be efficient
2. **Context Dependency:** Relies on GitHub Actions context for PR number
3. **Backward Compatibility:** Need to maintain existing output names
4. **Conditional Execution:** Currently only runs when `changed_files_from_base == 'false'` - this condition stays in the action

### Files to Create/Modify

**New Files:**
- `scripts/identify-commits/identify-commits.js`
- `scripts/identify-commits/package.json`
- `scripts/identify-commits/README.md`
- `scripts/identify-commits/.gitignore`

**Modified Files:**
- `actions/convert/action.yml` (replace step 52-93)
- `actions/integrate/action.yml` (replace step 84-125)

### Testing Strategy

1. **Unit Tests:**
   - Mock GitHub API responses
   - Test commit analysis logic with various scenarios:
     - PR with automation commits
     - PR without automation commits
     - PR with single commit
     - PR with multiple commits
   - Test output formatting

2. **Integration Testing:**
   - Test in actual GitHub Actions workflows
   - Verify outputs are correctly consumed by subsequent steps
   - Test with different PR scenarios

3. **Edge Cases:**
   - Empty commit list
   - Commits without parents
   - Different automation usernames
   - PR number not found

### Alternative Approaches Considered

1. **Bash Script:** Rejected - GitHub API calls are complex in bash, better in Node.js
2. **Python:** Rejected - JavaScript is more consistent with existing scripts and GitHub Actions ecosystem
3. **Go:** Rejected - Overkill, and would require different dependencies

### Code Structure

```javascript
// Main functions:
- getInputs() - Get inputs from env vars or GitHub Actions context
- getContext() - Get GitHub context (repo owner/name)
- identifyCommits(octokit, context, prNumber, actionsUsername) - Main logic
- main() - Entry point
```

### Output Standardization

**Current State:**
- Convert action expects: `base-commit`
- Integrate action expects: `base-ref`

**Proposed Solution:**
- Output both `base-commit` and `base-ref` with same value
- This maintains backward compatibility
- Both actions can continue using their preferred name
- Future actions can use either name

