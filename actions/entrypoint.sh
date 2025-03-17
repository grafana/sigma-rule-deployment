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
    *)
        echo "Invalid argument: $1"
        exit 1
        ;;
esac