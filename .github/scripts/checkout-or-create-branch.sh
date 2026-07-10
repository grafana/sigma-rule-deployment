#!/usr/bin/env bash
# Checks out <branch>, (re)creating it from <base-branch> so it always
# starts from the current base tip - even if <branch> already existed with
# stale content that's now diverged from <base-branch>. Callers overwrite
# the whole working tree after this runs, so history on <branch> is never
# worth preserving, and always resetting keeps the branch mergeable into
# <base-branch> instead of accumulating conflicts over repeated runs.
#
# Branch creation/reset goes through the GitHub REST API (a ref pointing at
# an existing, already-verified commit) rather than a local `git push`, for
# the same reason create-verified-commit.sh exists: the GitHub App token
# used by callers has no key to sign a local push with, and the target repo
# requires verified signatures. Ref creation/updates aren't themselves a
# commit, so they aren't subject to that rule.
#
# Prints the branch's expected head oid to stdout (for passing as
# create-verified-commit.sh's expectedHeadOid). Must be run from inside the
# target repo's working copy, after `git fetch`.
set -euo pipefail

REPO="$1"
BRANCH="$2"
BASE_BRANCH="$3"

BASE_OID=$(git rev-parse "origin/${BASE_BRANCH}")

if git rev-parse --verify "refs/remotes/origin/${BRANCH}" >/dev/null 2>&1; then
  gh api --method PATCH "repos/${REPO}/git/refs/heads/${BRANCH}" -f sha="$BASE_OID" -F force=true >/dev/null
else
  gh api "repos/${REPO}/git/refs" -f ref="refs/heads/${BRANCH}" -f sha="$BASE_OID" >/dev/null
fi

git checkout -B "$BRANCH" "$BASE_OID" >&2
echo "$BASE_OID"
