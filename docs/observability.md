# Beacon Observability

Phase 6 adds local observability for the Beacon operator through OpenTelemetry tracing and Prometheus metrics.

## Overview

Beacon records two kinds of signals:

- OpenTelemetry spans for reconcile hot paths.
- Prometheus metrics for reconcile latency, processed starvation events, Deployment patches, and scaling errors.

The local development path uses a stdout trace exporter so spans are visible directly in the operator terminal. No external collector is required.

## OpenTelemetry Spans

The operator initializes a tracer provider at startup with service name `beacon-operator`.

Tracing is enabled by default:

```sh
make run
```

Disable tracing for quieter local runs:

```sh
BEACON_TRACING_DISABLED=true make run
```

The main spans are:

- `BeaconPolicyReconcile`
- `StarvationEventReconcile`
- `fetch_starvation_event`
- `fetch_beacon_policy`
- `fetch_target_deployment`
- `calculate_resource_patch`
- `patch_deployment`
- `update_beacon_policy_status`
- `update_starvation_event_status`

The `StarvationEventReconcile` span records attributes such as namespace, event name, policy name, signal type, severity, target Deployment, container name, scale action, decision, previous request, new request, and reaction latency.

Errors are recorded on spans when Beacon cannot find the referenced policy, target Deployment, or target container, or when a patch/status update fails.

Example stdout span excerpt:

```json
{
  "Name": "patch_deployment",
  "Attributes": [
    {"Key": "targetDeployment", "Value": {"Type": "STRING", "Value": "sample-api"}},
    {"Key": "containerName", "Value": {"Type": "STRING", "Value": "api"}},
    {"Key": "scale_action", "Value": {"Type": "STRING", "Value": "CPURequestIncreased"}},
    {"Key": "should_patch", "Value": {"Type": "BOOL", "Value": true}}
  ]
}
```

## Prometheus Metrics

Beacon registers custom metrics with the controller-runtime metrics registry:

- `beacon_reconcile_latency_milliseconds`
- `beacon_starvation_events_processed_total`
- `beacon_vertical_scale_patches_total`
- `beacon_vertical_scale_errors_total`

Metric labels are intentionally low-cardinality:

- `controller`
- `signal_type`
- `severity`
- `scale_action`

Event names, policy names, Deployment names, and namespaces are not metric labels.

The Kubebuilder scaffold leaves metrics disabled for local `make run` by default with `--metrics-bind-address=0`. To inspect metrics locally:

```sh
go run ./cmd/main.go --metrics-bind-address=:8080 --metrics-secure=false
curl -s localhost:8080/metrics | grep '^beacon_'
```

## Latency Fields

Beacon records latency in multiple places:

- `StarvationEvent.status.reactionLatencyMillis`: event observed time to event processing decision.
- `BeaconPolicy.status.lastReactionLatencyMillis`: latest event reaction latency routed to that policy.
- `beacon_reconcile_latency_milliseconds`: wall-clock duration of the controller reconcile call.

The status latency answers "how long from signal observation to Beacon decision?" The metric histogram answers "how long did the reconcile call take inside the controller?"

## Debugging Hot Paths

Use spans to identify where reconcile time is spent:

- Slow `fetch_beacon_policy` or `fetch_target_deployment` suggests API server/cache delay.
- Slow `calculate_resource_patch` suggests local controller logic cost.
- Slow or failed `patch_deployment` suggests update conflicts or API write latency.
- Slow `update_*_status` suggests status subresource write latency or conflicts.

Use metrics to trend behavior across repeated synthetic events or longer agent runs.

## Limitations

- Stdout tracing is for local development and demos; it is noisy under sustained event streams.
- Metrics are exposed only when the manager metrics endpoint is enabled.
- The benchmark still uses Kubernetes status fields as its source of truth and does not require tracing or metrics.
- This phase does not add OpenTelemetry propagation across the agent and operator.
- This phase does not add production dashboards or alerts.

## Future Production Path

A production deployment would usually replace stdout tracing with OTLP export to an OpenTelemetry Collector, then forward traces to Jaeger, Tempo, Honeycomb, or another tracing backend. Prometheus metrics can be scraped by a ServiceMonitor or PodMonitor once deployment manifests include the correct endpoint and RBAC.
