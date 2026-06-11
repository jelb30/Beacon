#!/usr/bin/env bash
set -euo pipefail

namespace="${1:-default}"
policy_name="${2:-sample-api-policy}"
target_deployment="${3:-sample-api}"
container_name="${4:-api}"
signal_type="${5:-CPUStarvation}"
severity="${6:-High}"

timestamp="$(date -u +%Y%m%d%H%M%S)"
observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
signal_slug="$(printf '%s' "$signal_type" | sed -E 's/([a-z0-9])([A-Z])/\1-\2/g' | tr '[:upper:]' '[:lower:]')"
event_name="${target_deployment}-${signal_slug}-${timestamp}-$$"

kubectl apply -f - <<EOF
apiVersion: autoscaling.beacon.dev/v1alpha1
kind: StarvationEvent
metadata:
  name: ${event_name}
  namespace: ${namespace}
  labels:
    app.kubernetes.io/name: beacon
    beacon.dev/benchmark: "false"
    beacon.dev/synthetic: "true"
spec:
  policyName: ${policy_name}
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ${target_deployment}
  containerName: ${container_name}
  signalType: ${signal_type}
  observedAt: "${observed_at}"
  severity: ${severity}
  message: "Synthetic ${signal_type} event emitted for Beacon validation."
EOF

printf 'Created StarvationEvent %s/%s\n' "$namespace" "$event_name"
