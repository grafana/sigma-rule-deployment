#!/usr/bin/env bash

# Determine Image Reference Script
#
# Determines the Docker image reference based on the GitHub action reference.
# Outputs the image_ref to $GITHUB_OUTPUT for use in GitHub Actions.
#
# Usage:
#   determine-image-ref.sh [ACTION_REF]
#
# Environment variables:
#   ACTION_REF - GitHub action reference (defaults to $1 if provided)
#   GITHUB_OUTPUT - GitHub Actions output file (defaults to $GITHUB_OUTPUT)

set -euo pipefail

# Get ACTION_REF from argument or environment variable
ACTION_REF="${1:-${ACTION_REF:-}}"

if [ -z "$ACTION_REF" ]; then
  echo "Error: ACTION_REF must be provided as argument or environment variable" >&2
  exit 1
fi

# Official SemVer regex: https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
# Adapted for Bash ERE: \d becomes [0-9], (?:...) becomes (...), and the v tag prefix is required.
SEMVER_REGEX='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$'
# Preserve every numeric version tag accepted by the original script.
LEGACY_VERSION_REGEX='^v[0-9]+(\.[0-9]+)*(\.[0-9]+)?$'

# Accept the original numeric version tags and v-prefixed Semantic Versions.
if [[ "$ACTION_REF" =~ $LEGACY_VERSION_REGEX || "$ACTION_REF" =~ $SEMVER_REGEX ]]; then
  IMAGE_REF="$ACTION_REF"
  echo "Using tag: $ACTION_REF"
elif [[ "$ACTION_REF" == "latest" ]]; then
  IMAGE_REF="main"
  echo "Using main tag"
else
  IMAGE_REF="sha-$ACTION_REF"
  echo "Using SHA: sha-$ACTION_REF"
fi

# Output to GITHUB_OUTPUT if available, otherwise to stdout
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "image_ref=$IMAGE_REF" >> "$GITHUB_OUTPUT"
else
  echo "image_ref=$IMAGE_REF"
fi

