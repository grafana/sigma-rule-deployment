#!/usr/bin/env bash
# Creates a commit on an existing remote branch via the GitHub GraphQL API
# (createCommitOnBranch) instead of `git commit` + `git push`. Commits made
# this way are automatically signed and marked "Verified" by GitHub, which
# is required to satisfy org-wide rulesets that mandate verified signatures
# for bot-authored commits (GitHub Apps have no GPG/SSH key to sign with
# locally).
#
# Must be run from inside the target repo's working copy, after the working
# tree has been mutated to its desired final state and `git add -A` has
# staged it.
set -euo pipefail

REPO="$1"
BRANCH="$2"
EXPECTED_HEAD_OID="$3"
MESSAGE="$4"

# One row per changed file: "<status>\t<path>\t<base64 contents, A/M only>".
# base64 still runs once per added/modified file (unavoidable I/O), but the
# JSON array construction below happens in a single jq pass rather than one
# jq invocation per file.
CHANGES=$(
  while IFS=$'\t' read -r status path; do
    case "$status" in
      A|M) printf '%s\t%s\t%s\n' "$status" "$path" "$(base64 -w0 "$path")" ;;
      D)   printf '%s\t%s\t\n' "$status" "$path" ;;
    esac
  done < <(git diff --cached --name-status --no-renames)
)

if [ -z "$CHANGES" ]; then
  echo "No changes to commit, skipping."
  exit 0
fi

jq -R -s \
  --arg repo "$REPO" \
  --arg branch "$BRANCH" \
  --arg oid "$EXPECTED_HEAD_OID" \
  --arg message "$MESSAGE" \
  '
  (split("\n") | map(select(length > 0) | split("\t"))) as $rows |
  {
    query: "mutation($input: CreateCommitOnBranchInput!) { createCommitOnBranch(input: $input) { commit { oid } } }",
    variables: { input: {
      branch: { repositoryNameWithOwner: $repo, branchName: $branch },
      message: { headline: $message },
      fileChanges: {
        additions: [$rows[] | select(.[0] == "A" or .[0] == "M") | {path: .[1], contents: .[2]}],
        deletions: [$rows[] | select(.[0] == "D") | {path: .[1]}]
      },
      expectedHeadOid: $oid
    }}
  }' <<<"$CHANGES" | gh api graphql --input -
