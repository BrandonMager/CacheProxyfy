#!/usr/bin/env bash
set -euo pipefail

if [ -z "${DOCKER_USERNAME:-}" ]; then
  read -rp "Docker Hub username: " DOCKER_USERNAME
fi
export DOCKER_USERNAME

echo "Deploying CacheProxyfy to Kubernetes with image registry: ${DOCKER_USERNAME}"

for file in "$(dirname "$0")"/*.yaml; do
  envsubst '${DOCKER_USERNAME}' < "$file" | kubectl apply -f -
done

echo "Done. Watch pods: kubectl get pods -n cacheproxyfy -w"
