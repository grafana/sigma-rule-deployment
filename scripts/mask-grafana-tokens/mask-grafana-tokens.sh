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

if ! tokens=$(jq -r 'objects | .[] | strings | gsub("^\\s+|\\s+$"; "") | select(length > 0)' <<<"$TOKENS" 2>/dev/null); then
  exit 0
fi

while IFS= read -r token; do
  if [ -n "$token" ]; then
    echo "::add-mask::${token}"
  fi
done <<<"$tokens"
