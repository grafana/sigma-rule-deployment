#! /bin/bash

function _integrate() {
    echo "Integrating Sigma Rules"
    sigma-deployer integrate "$@"
}

function _deploy() {
    echo "Deploying Sigma Rules"
    sigma-deployer deploy "$@"
}

function _convert() {
    echo "Converting Sigma Rules"
    plugin_packages=${PLUGIN_PACKAGES:-}
    declare -a valid_plugins=()

    shopt -s nocasematch
    for plugin in $(echo "$plugin_packages" | tr ',' ' '); do
        if [[ "$plugin" == pysigma-* ]]; then
            valid_plugins+=("$plugin")
        else
            echo "Error: Invalid plugin name: $plugin"
            exit 1
        fi
    done
    shopt -u nocasematch

    if [ ${#valid_plugins[@]} -gt 0 ]; then
        uv add --directory /app/actions/convert "${valid_plugins[@]}"
    fi

    uv run --directory /app/actions/convert main.py
}

set -euo pipefail
set +x

echo "Sigma Rule Deployment"

if [ "$#" -lt 1 ]; then
    echo "No arguments provided"
    exit 1
fi

case "$1" in
"integrate")
    shift
    _integrate "$@"
    ;;
"deploy")
    shift
    _deploy "$@"
    ;;
"convert")
    shift
    _convert "$@"
    ;;
*)
    echo "Invalid argument: $1"
    exit 1
    ;;
esac

