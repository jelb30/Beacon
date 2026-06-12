# Beacon

Beacon is a Kubernetes operator foundation for future workload autoscaling based on starvation signals and reaction latency goals.

Repository: `github.com/jelb30/Beacon`

## Phase Scope

Phase 1 provides a clean Kubebuilder/controller-runtime scaffold with one custom resource: `BeaconPolicy`.

Phase 2 adds a `StarvationEvent` custom resource and ingestion path. Synthetic starvation events can be created in Kubernetes, routed to a `BeaconPolicy`, and reflected in policy status.

Phase 3 adds the vertical scaling patch engine. CPU starvation events increase the target Deployment container's CPU request. Memory starvation events increase memory requests when memory policy bounds are configured.

Phase 4 adds a reproducible synthetic benchmark/demo harness that measures event-to-Deployment-patch reaction latency.

Phase 5 adds `beacon-agent`, a local starvation signal source that creates `StarvationEvent` resources automatically in synthetic mode.

Phase 6 adds OpenTelemetry stdout tracing and custom Prometheus metrics for reconcile latency, processed events, Deployment patches, and scaling errors.

These phases do not implement real eBPF collection, Terraform, or production deployment automation.

## Prerequisites

- Go 1.25 or newer
- Kubebuilder
- `make`
- `kubectl`
- Access to a Kubernetes cluster for `make install` and `make run`

## Local Commands

Generate deepcopy methods:

```sh
make generate
```

Generate CRDs and RBAC:

```sh
make manifests
```

Run tests:

```sh
make test
```

Or run the Go test suite directly:

```sh
go test ./...
```

Build the controller image:

```sh
make docker-build IMG=ghcr.io/jelb30/beacon-controller:latest
```

Install the CRD into the current Kubernetes cluster:

```sh
make install
```

Run the operator locally:

```sh
make run
```

In another terminal, apply the sample workload:

```sh
kubectl apply -f config/samples/sample-api-deployment.yaml
```

Apply the sample policy:

```sh
kubectl apply -f config/samples/autoscaling_v1alpha1_beaconpolicy.yaml
```

Emit a synthetic starvation event:

```sh
./hack/emit-starvation-event.sh
```

Check the patched CPU request:

```sh
kubectl get deployment sample-api -n default -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
```

List StarvationEvent resources:

```sh
kubectl get starvationevents -A
```

List BeaconPolicy resources:

```sh
kubectl get beaconpolicy -A
```

Inspect the sample policy:

```sh
kubectl describe beaconpolicy sample-api-policy -n default
```

Inspect the full policy status:

```sh
kubectl get beaconpolicy sample-api-policy -n default -o yaml
```

## Expected Result

For Phase 1, the `BeaconPolicy` resource exists, the `Ready` condition becomes `True`, and `status.lastDecision` becomes `NoopPhase1ScaffoldReady`.

For Phase 2, the `StarvationEvent` shows `Processed=True`, and the `BeaconPolicy` status records the latest event fields.

For Phase 3, the sample Deployment CPU request changes from `100m` to `125m` after one `CPUStarvation` event. Another event increases it again, for example from `125m` to around `156m`. CPU requests never exceed `maxCPURequest`. `BeaconPolicy` status should show `lastDecision=VerticalScalePatchApplied`, and `StarvationEvent` status should show `processed=true`.

## Phase 4 Benchmark Demo

The Phase 4 harness measures synthetic event-to-patch latency: the time from a `StarvationEvent.spec.observedAt` timestamp to Beacon processing the event and recording the Deployment patch decision. This demonstrates the low-latency event-driven path. Real eBPF signal generation is implemented in a later phase.

Terminal 1:

```sh
make install
make run
```

Terminal 2:

```sh
make demo-reset
make benchmark
make benchmark-series
```

Manual inspection:

```sh
kubectl get deployment sample-api -n default -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
kubectl get starvationevents -A
kubectl get beaconpolicy sample-api-policy -n default -o yaml
```

Example benchmark output:

```text
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpu-starvation-1781216200-12345
Event creation time: 2026-06-11T22:16:40Z

CPU request before: 100m
CPU request after: 125m

StarvationEvent latency: 287ms
BeaconPolicy latency: 287ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS
```

## Phase 5 Agent Demo

The Phase 5 agent emits `StarvationEvent` resources through the Kubernetes API. Synthetic mode is for reliable local macOS/kind development. Linux cgroup/eBPF detector modes are isolated behind build tags and are placeholders until a later phase adds node-level signal detection.

Terminal 1:

```sh
make install
make run
```

Terminal 2:

```sh
make demo-reset
make agent-once
```

Inspect the result:

```sh
kubectl get starvationevents -A
kubectl get beaconpolicy sample-api-policy -n default -o yaml
kubectl get deployment sample-api -n default -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
```

Expected result:

- `beacon-agent` creates a `StarvationEvent`.
- The operator processes the event.
- The sample Deployment CPU request increases from `100m` to `125m`.
- `BeaconPolicy.status.lastDecision` becomes `VerticalScalePatchApplied`.

Continuous synthetic mode:

```sh
make agent-run
```

Real eBPF or cgroup-based detection requires Linux node access and elevated permissions. That detector path is intentionally separated from the local synthetic mode so the operator demo remains portable.

## Phase 6 Observability

Beacon initializes an OpenTelemetry tracer provider at operator startup and emits local stdout spans by default. The traced hot paths include:

- `BeaconPolicyReconcile`
- `StarvationEventReconcile`
- `fetch_starvation_event`
- `fetch_beacon_policy`
- `fetch_target_deployment`
- `calculate_resource_patch`
- `patch_deployment`
- `update_beacon_policy_status`
- `update_starvation_event_status`

Run with tracing:

```sh
make install
make run
```

Tracing can be disabled for noisy local runs:

```sh
BEACON_TRACING_DISABLED=true make run
```

Example stdout span excerpt:

```json
{
  "Name": "StarvationEventReconcile",
  "Attributes": [
    {"Key": "policyName", "Value": {"Type": "STRING", "Value": "sample-api-policy"}},
    {"Key": "signalType", "Value": {"Type": "STRING", "Value": "CPUStarvation"}},
    {"Key": "scale_action", "Value": {"Type": "STRING", "Value": "CPURequestIncreased"}},
    {"Key": "reaction_latency_ms", "Value": {"Type": "INT64", "Value": 524}}
  ]
}
```

Custom metrics are registered on the controller-runtime metrics endpoint:

- `beacon_reconcile_latency_milliseconds`
- `beacon_starvation_events_processed_total`
- `beacon_vertical_scale_patches_total`
- `beacon_vertical_scale_errors_total`

The default `make run` command leaves the metrics listener disabled through `--metrics-bind-address=0`. For local Prometheus-style scraping, run:

```sh
go run ./cmd/main.go --metrics-bind-address=:8080 --metrics-secure=false
curl -s localhost:8080/metrics | grep '^beacon_'
```

These traces and metrics make it easier to identify slow reconcile hot paths, failed target resolution, patch conflicts, and status update delays. See `docs/observability.md` for more detail.
