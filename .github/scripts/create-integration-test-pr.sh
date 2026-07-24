#!/usr/bin/env bash
# Creates (or refreshes) the "integration-test-pr-<N>" branch and draft PR in
# the integration-test repo from the Sigma Rule Deployment PR's fixtures.
#
# This orchestration was previously inlined in .github/workflows/build-docker.yml
# It is extracted into a script so the trusted (same-repo) and gated fork
# (workflow_run) paths run the *identical* logic. Helper scripts are resolved
# relative to THIS file so they always come from the same (trusted) checkout as
# this script, never from whatever fixtures the PR provides.
#
# Required environment variables:
#   TEST_REPO         - owner/name of the integration-test repo
#   TEST_REPO_DIR     - path to the checked-out integration-test repo
#   FIXTURES_DIR      - path to the SRD checkout containing integration-test/*
#   GITHUB_PR_NUMBER  - the originating SRD pull request number
#   ACTION_SHA        - the SRD PR head SHA (pins actions + docker image tag)
#   RUN_TOKEN         - unique token guaranteeing a diff on re-runs
#   GH_TOKEN          - GitHub App token with access to the integration-test repo
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$TEST_REPO_DIR"
git fetch

EXPECTED_HEAD_OID=$("$SCRIPT_DIR/checkout-or-create-branch.sh" \
  "$TEST_REPO" \
  "integration-test-pr-${GITHUB_PR_NUMBER}" \
  main)

# remove old test files, if they exist
rm -rf ./rules
rm -rf ./pipelines
rm test.yml
rm config.yml

cp -R "$FIXTURES_DIR/integration-test/"* ./
sed -i "s/uses: grafana\/sigma-rule-deployment\/actions\/convert@.*/uses: grafana\/sigma-rule-deployment\/actions\/convert@$ACTION_SHA/g" ./.github/workflows/convert-integrate-test.yml
sed -i "s/uses: grafana\/sigma-rule-deployment\/actions\/integrate@.*/uses: grafana\/sigma-rule-deployment\/actions\/integrate@$ACTION_SHA/g" ./.github/workflows/convert-integrate-test.yml

# add the action sha so that the integration test can add the check to this PR
echo "$ACTION_SHA" > action_sha.txt
echo "$GITHUB_PR_NUMBER" > github_pr.txt
# per-run token guarantees a change even when re-running on the PR
echo "$RUN_TOKEN" > run_attempt.txt

git add -A

"$SCRIPT_DIR/create-verified-commit.sh" \
  "$TEST_REPO" \
  "integration-test-pr-${GITHUB_PR_NUMBER}" \
  "$EXPECTED_HEAD_OID" \
  "update integration test configuration"

RESULT=$(gh pr list --json id,title -H "integration-test-pr-${GITHUB_PR_NUMBER}" | jq -r '. | length')

if [ "$RESULT" -eq 0 ]; then
  gh pr create \
    --title "Integration Test PR ${GITHUB_PR_NUMBER}" \
    --body "This PR to test Sigma Rule Deployment App from Sigma Rule Deployment Integration Test App for grafana/sigma-rule-deployment#${GITHUB_PR_NUMBER}" \
    --head "integration-test-pr-${GITHUB_PR_NUMBER}" \
    --base main \
    --draft \
    --label "automated"
fi
