#!/usr/bin/env bash
# Checks out <branch>, creating it from <base-branch> if it doesn't exist on
# the remote yet. Branch creation goes through the GitHub REST API (a ref
# pointing at an existing, already-verified commit) rather than a local
# `git push`, for the same reason create-verified-commit.sh exists: the
# GitHub App token used by callers has no key to sign a local push with, and
# the target repo requires verified signatures. Ref creation isn't itself a
# commit, so it isn't subject to that rule.
#
# Prints the branch's expected head oid to stdout (for passing as
# create-verified-commit.sh's expectedHeadOid). Must be run from inside the
# target repo's working copy, after `git fetch`.
set -euo pipefail

REPO="$1"
BRANCH="$2"
BASE_BRANCH="$3"

if git rev-parse --verify "refs/remotes/origin/${BRANCH}" >/dev/null 2>&1; then
  git checkout -B "$BRANCH" "origin/${BRANCH}" >&2
  git rev-parse HEAD
else
  BASE_OID=$(git rev-parse "origin/${BASE_BRANCH}")
  gh api "repos/${REPO}/git/refs" -f ref="refs/heads/${BRANCH}" -f sha="$BASE_OID" >/dev/null
  git checkout -b "$BRANCH" "origin/${BASE_BRANCH}" >&2
  echo "$BASE_OID"
fi
