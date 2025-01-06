#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

# Execute the command and remove the parsing line from the output
sigma "$@" 2>&1 | sed '/Parsing Sigma rules/d'
