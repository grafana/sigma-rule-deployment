#! /bin/sh

set -euo pipefail
set +x

echo "Sigma Rule Deployment"

if [ "$#" -lt 1 ];
then
    echo "No arguments provided"
    exit 1
fi

case "$1" in
    "integrate")
        shift
        echo "Integrating Sigma rules"
        integrate "$@"
        ;;
    "deploy")
        shift
        echo "Deploying Sigma rules"
        deploy "$@"
        ;;
    "convert")
        shift
        echo "Converting Sigma rules"
        plugin_packages=${PLUGIN_PACKAGES}
        declare -a valid_plugins=()

        shopt -s nocasematch
        for plugin in $(echo $plugin_packages | tr ',' ' '); do
            if [[ "$plugin" == pysigma-backend-* ]]; then
                valid_plugins+=("$plugin")
            else
                echo "Error: Invalid plugin name: $plugin"
                exit 1
            fi
        done
        shopt -u nocasematch

        if [ ${#valid_plugins[@]} -gt 0 ]; then
            uv add --directory /app/convert "${valid_plugins[@]}"
        fi

        uv run --directory /app/convert main.py --config ${CONFIG_PATH} --render-traceback ${RENDER_TRACEBACK}

        ;;
    *)
        echo "Invalid argument: $1"
        exit 1
        ;;
esac