# Beacon

Beacon is a Kubernetes operator foundation for future workload autoscaling based on starvation signals and reaction latency goals.

Repository: `github.com/jelb30/Beacon`

## Phase Scope

Phase 1 provides a clean Kubebuilder/controller-runtime scaffold with one custom resource: `BeaconPolicy`.

Phase 2 adds a `StarvationEvent` custom resource and ingestion path. Synthetic starvation events can be created in Kubernetes, routed to a `BeaconPolicy`, and reflected in policy status.

Phase 3 adds the vertical scaling patch engine. CPU starvation events increase the target Deployment container's CPU request. Memory starvation events increase memory requests when memory policy bounds are configured.

Phase 4 adds a reproducible synthetic benchmark/demo harness that measures event-to-Deployment-patch reaction latency.

These phases do not implement eBPF collection, Terraform, Prometheus metrics, OpenTelemetry tracing, or real production benchmarking.

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
