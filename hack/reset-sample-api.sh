#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
DEPLOYMENT="${DEPLOYMENT:-sample-api}"
CONTAINER="${CONTAINER:-api}"
CACHE_SETTLE_SECONDS="${CACHE_SETTLE_SECONDS:-1}"

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

container_cpu_request() {
  kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" \
    -o "jsonpath={.spec.template.spec.containers[?(@.name==\"$CONTAINER\")].resources.requests.cpu}" 2>/dev/null || true
}

require_cmd kubectl

kubectl apply -f config/samples/sample-api-deployment.yaml
kubectl apply -f config/samples/autoscaling_v1alpha1_beaconpolicy.yaml

kubectl delete starvationevents -n "$NAMESPACE" -l beacon.dev/benchmark=true --ignore-not-found=true
kubectl delete starvationevents -n "$NAMESPACE" -l beacon.dev/synthetic=true --ignore-not-found=true
for event in $(kubectl get starvationevents -n "$NAMESPACE" -o name 2>/dev/null | awk -F/ '$2 ~ /^benchmark-/ { print $2 }'); do
  kubectl delete starvationevent "$event" -n "$NAMESPACE" --ignore-not-found=true
done
kubectl wait deployment/"$DEPLOYMENT" -n "$NAMESPACE" --for=condition=Available --timeout=60s
sleep "$CACHE_SETTLE_SECONDS"

cpu_request="$(container_cpu_request)"
[ -n "$cpu_request" ] || fail "CPU request for container $CONTAINER in Deployment $NAMESPACE/$DEPLOYMENT not found"

printf 'Sample API reset complete.\n'
printf 'Deployment: %s/%s\n' "$NAMESPACE" "$DEPLOYMENT"
printf 'Container: %s\n' "$CONTAINER"
printf 'Current CPU request: %s\n' "$cpu_request"
