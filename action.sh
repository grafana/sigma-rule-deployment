#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

# Ensure sigma-cli is installed and the required plugin is available
SIGMA_CLI_VERSION="1.0.4"
SIGMA_PLUGIN="loki"

# Install sigma-cli if not already installed
pip install --disable-pip-version-check sigma-cli==${SIGMA_CLI_VERSION} 2>&1 >/dev/null || true

# Install the required plugin
sigma plugin install "${SIGMA_PLUGIN}" 2>&1 >/dev/null || true

# Run sigma-cli with the provided arguments and remove the parsing line from the output
sigma "$@" 2>&1 | sed '/Parsing Sigma rules/d'
