#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
DEPLOYMENT="${DEPLOYMENT:-sample-api}"
CONTAINER="${CONTAINER:-api}"
POLICY="${POLICY:-sample-api-policy}"
SIGNAL_TYPE="${SIGNAL_TYPE:-CPUStarvation}"
SEVERITY="${SEVERITY:-High}"
LATENCY_BUDGET_MS="${LATENCY_BUDGET_MS:-4000}"

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

jsonpath() {
  kubectl get "$1" "$2" -n "$NAMESPACE" -o "jsonpath=$3" 2>/dev/null || true
}

container_cpu_request() {
  kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" \
    -o "jsonpath={.spec.template.spec.containers[?(@.name==\"$CONTAINER\")].resources.requests.cpu}" 2>/dev/null || true
}

cpu_to_millicores() {
  awk -v cpu="$1" '
    BEGIN {
      if (cpu == "") {
        print ""
        exit
      }
      if (cpu ~ /m$/) {
        sub(/m$/, "", cpu)
        print int(cpu)
        exit
      }
      if (cpu ~ /^[0-9]+(\.[0-9]+)?$/) {
        printf "%.0f\n", cpu * 1000
        exit
      }
      print "ERR"
      exit
    }
  '
}

require_cmd kubectl
require_cmd date
require_cmd awk

kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" >/dev/null 2>&1 || fail "Deployment $NAMESPACE/$DEPLOYMENT not found"
kubectl get beaconpolicy "$POLICY" -n "$NAMESPACE" >/dev/null 2>&1 || fail "BeaconPolicy $NAMESPACE/$POLICY not found"
kubectl get crd starvationevents.autoscaling.beacon.dev >/dev/null 2>&1 || fail "StarvationEvent CRD not found"

cpu_before="$(container_cpu_request)"
[ -n "$cpu_before" ] || fail "CPU request for container $CONTAINER in Deployment $NAMESPACE/$DEPLOYMENT not found"

epoch="$(date -u +%s)"
observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
signal_slug="$(printf '%s' "$SIGNAL_TYPE" | sed -E 's/([a-z0-9])([A-Z])/\1-\2/g' | tr '[:upper:]' '[:lower:]')"
event_name="benchmark-${signal_slug}-${epoch}-$$"

kubectl apply -f - >/dev/null <<EOF
apiVersion: autoscaling.beacon.dev/v1alpha1
kind: StarvationEvent
metadata:
  name: ${event_name}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: beacon
    beacon.dev/benchmark: "true"
spec:
  policyName: ${POLICY}
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ${DEPLOYMENT}
  containerName: ${CONTAINER}
  signalType: ${SIGNAL_TYPE}
  observedAt: "${observed_at}"
  severity: ${SEVERITY}
  message: "Synthetic benchmark ${SIGNAL_TYPE} event."
EOF

processed=""
attempts=0
while [ "$attempts" -lt 40 ]; do
  processed="$(jsonpath starvationevent "$event_name" '{.status.processed}')"
  if [ "$processed" = "true" ]; then
    break
  fi
  attempts=$((attempts + 1))
  sleep 0.25
done

if [ "$processed" != "true" ]; then
  printf '## Beacon Synthetic Scale-Up Benchmark\n\n'
  printf 'Policy: %s\nDeployment: %s\nContainer: %s\nSignal: %s\nSeverity: %s\n\n' "$POLICY" "$DEPLOYMENT" "$CONTAINER" "$SIGNAL_TYPE" "$SEVERITY"
  printf 'Event: %s\nEvent creation time: %s\nCPU request before: %s\nResult: FAIL\n' "$event_name" "$observed_at" "$cpu_before"
  fail "StarvationEvent $NAMESPACE/$event_name was not processed within 10 seconds"
fi

event_latency="$(jsonpath starvationevent "$event_name" '{.status.reactionLatencyMillis}')"
policy_latency="$(jsonpath beaconpolicy "$POLICY" '{.status.lastReactionLatencyMillis}')"
decision="$(jsonpath beaconpolicy "$POLICY" '{.status.lastDecision}')"
cpu_after="$(container_cpu_request)"

before_milli="$(cpu_to_millicores "$cpu_before")"
after_milli="$(cpu_to_millicores "$cpu_after")"
[ -n "$event_latency" ] || fail "StarvationEvent latency is missing"
[ -n "$policy_latency" ] || fail "BeaconPolicy latency is missing"
[ "$before_milli" != "ERR" ] || fail "unsupported CPU quantity before patch: $cpu_before"
[ "$after_milli" != "ERR" ] || fail "unsupported CPU quantity after patch: $cpu_after"

result="FAIL"
if [ "$decision" = "MaxLimitReached" ]; then
  if [ "$after_milli" -eq "$before_milli" ]; then
    result="PASS_MAX_REACHED"
  fi
elif [ "$decision" = "VerticalScalePatchApplied" ]; then
  if [ "$SIGNAL_TYPE" = "CPUStarvation" ] && [ "$after_milli" -gt "$before_milli" ] && [ "$policy_latency" -le "$LATENCY_BUDGET_MS" ]; then
    result="PASS"
  fi
fi

printf '## Beacon Synthetic Scale-Up Benchmark\n\n'
printf 'Policy: %s\n' "$POLICY"
printf 'Deployment: %s\n' "$DEPLOYMENT"
printf 'Container: %s\n' "$CONTAINER"
printf 'Signal: %s\n' "$SIGNAL_TYPE"
printf 'Severity: %s\n\n' "$SEVERITY"
printf 'Event: %s\n' "$event_name"
printf 'Event creation time: %s\n\n' "$observed_at"
printf 'CPU request before: %s\n' "$cpu_before"
printf 'CPU request after: %s\n\n' "$cpu_after"
printf 'StarvationEvent latency: %sms\n' "$event_latency"
printf 'BeaconPolicy latency: %sms\n' "$policy_latency"
printf 'Latency budget: %sms\n' "$LATENCY_BUDGET_MS"
printf 'Decision: %s\n' "$decision"
printf 'Result: %s\n' "$result"

if [ "$result" = "FAIL" ]; then
  exit 1
fi
