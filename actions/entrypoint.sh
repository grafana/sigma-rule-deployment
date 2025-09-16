#! /bin/bash

function _integrate() {
    echo "Integrating Sigma Rules"
    integrate "$@"
}

function _deploy() {
    echo "Deploying Sigma Rules"
    deploy "$@"
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
        # allow prerelease packages to be installed
        # TODO: remove this once we released a new version of sigma-rule-deployer
        uv add --prerelease=allow --directory /app/convert "${valid_plugins[@]}"
    fi

    # allow prerelease packages to be installed
    # TODO: remove this once we released a new version of sigma-rule-deployer
    uv run --prerelease=allow --directory /app/convert main.py
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
