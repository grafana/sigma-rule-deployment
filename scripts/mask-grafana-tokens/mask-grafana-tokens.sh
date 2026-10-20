#!/usr/bin/env bash

# Mask Grafana Tokens Script
#
# GitHub masks a secret in the logs as a whole. When the Grafana token input is a JSON object
# of tokens per instance, this also masks each token on its own, so one printed by itself is
# still hidden. A plain token (or anything that is not a JSON object) is left to GitHub.
#
# Usage:
#   mask-grafana-tokens.sh
#
# Environment variables:
#   GRAFANA_SA_TOKEN - the Grafana token input

set -euo pipefail

TOKENS="${GRAFANA_SA_TOKEN:-}"

if ! command -v jq >/dev/null 2>&1; then
  # Only a JSON object needs this, and a plain token is already masked by GitHub
  if [[ "$TOKENS" =~ ^[[:space:]]*\{ ]]; then
    echo "::warning::jq is not installed, so the individual Grafana tokens are not masked in the logs"
  fi
  exit 0
fi

# A multi-line token is masked a line at a time, as GitHub does for a multi-line secret, so that
# no part of a token can start another workflow command.
if ! tokens=$(jq -r 'objects | .[] | strings | split("\n")[] | gsub("^\\s+|\\s+$"; "") | select(length > 0)' <<<"$TOKENS" 2>/dev/null); then
  exit 0 # not a JSON object
fi

while IFS= read -r token; do
  if [ -n "$token" ]; then
    echo "::add-mask::${token}"
  fi
done <<<"$tokens"
