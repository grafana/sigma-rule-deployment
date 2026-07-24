#!/usr/bin/env bash
# Refreshes the integration-test repo's main branch to the SRD PR head SHA (via
# the "integration-test-main-update-<N>" PR), waits for its checks, merges it,
# then comments "sigma convert all" on the integration test PR to kick off the
# comment-triggered integration test.
#
# Extracted from .github/workflows/build-docker.yml so the trusted (same-repo)
# and gated fork (workflow_run) paths run identical logic. Helper scripts are
# resolved relative to THIS file so they always come from the same (trusted)
# checkout as this script.
#
# Required environment variables:
#   TEST_REPO         - owner/name of the integration-test repo
#   TEST_REPO_DIR     - path to the checked-out integration-test repo
#   GITHUB_PR_NUMBER  - the originating SRD pull request number
#   ACTION_SHA        - the SRD PR head SHA (pins actions + docker image tag)
#   RUN_TOKEN         - unique token guaranteeing a diff on re-runs
#   GH_TOKEN          - GitHub App token with access to the integration-test repo
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$TEST_REPO_DIR"
git fetch
git checkout main
git pull

EXPECTED_HEAD_OID=$("$SCRIPT_DIR/checkout-or-create-branch.sh" \
  "$TEST_REPO" \
  "integration-test-main-update-${GITHUB_PR_NUMBER}" \
  main)

sed -i "s/uses: grafana\/sigma-rule-deployment\/actions\/convert@.*/uses: grafana\/sigma-rule-deployment\/actions\/convert@$ACTION_SHA/g" ./.github/workflows/convert-integrate-comment-test.yml
sed -i "s/uses: grafana\/sigma-rule-deployment\/actions\/integrate@.*/uses: grafana\/sigma-rule-deployment\/actions\/integrate@$ACTION_SHA/g" ./.github/workflows/convert-integrate-comment-test.yml

# add the action sha so that the integration test can add the check to this PR
echo "$ACTION_SHA" > action_sha.txt
echo "$GITHUB_PR_NUMBER" > github_pr.txt
# per-run token guarantees a change even when re-running on the PR
echo "$RUN_TOKEN" > run_attempt.txt

# only commit the relevant changed files
git add .github/workflows/convert-integrate-comment-test.yml action_sha.txt github_pr.txt run_attempt.txt

"$SCRIPT_DIR/create-verified-commit.sh" \
  "$TEST_REPO" \
  "integration-test-main-update-${GITHUB_PR_NUMBER}" \
  "$EXPECTED_HEAD_OID" \
  "update main branch to relevant hash"

RESULT=$(gh pr list --json id,title -H "integration-test-main-update-${GITHUB_PR_NUMBER}" | jq -r '. | length')

if [ "$RESULT" -eq 0 ]; then
  gh pr create \
    --title "Integration Test Main Update ${GITHUB_PR_NUMBER}" \
    --body "This PR updates the main branch to the relevant hash for grafana/sigma-rule-deployment#${GITHUB_PR_NUMBER}" \
    --head "integration-test-main-update-${GITHUB_PR_NUMBER}" \
    --base main \
    --label "automated" \
    --draft
fi
sleep 10 # wait for the PR checks to start...

# wait for the required PR checks to complete
if gh pr checks "integration-test-main-update-${GITHUB_PR_NUMBER}" --watch; then
  gh pr ready "integration-test-main-update-${GITHUB_PR_NUMBER}"
  gh pr merge "integration-test-main-update-${GITHUB_PR_NUMBER}" --auto --squash
  sleep 5 # wait for the merge to complete
  gh pr comment "integration-test-pr-${GITHUB_PR_NUMBER}" --body "sigma convert all"
else
  echo "Integration Test Main Update PR failed"
  exit 1
fi
