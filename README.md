# Beacon

Beacon is a Kubernetes operator foundation for future workload autoscaling based on starvation signals and reaction latency goals.

Repository: `github.com/jelb30/Beacon`

## Phase Scope

Phase 1 provides a clean Kubebuilder/controller-runtime scaffold with one custom resource: `BeaconPolicy`.

Phase 2 adds a `StarvationEvent` custom resource and ingestion path. Synthetic starvation events can be created in Kubernetes, routed to a `BeaconPolicy`, and reflected in policy status.

These phases do not implement eBPF collection, Terraform, Prometheus metrics, OpenTelemetry tracing, or real workload scaling.

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

Apply the sample policy:

```sh
kubectl apply -f config/samples/autoscaling_v1alpha1_beaconpolicy.yaml
```

Emit a synthetic starvation event in another terminal while `make run` is active:

```sh
./hack/emit-starvation-event.sh
```

You can also apply the static sample event:

```sh
kubectl apply -f config/samples/autoscaling_v1alpha1_starvationevent.yaml
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

## Expected Result

For Phase 1, the `BeaconPolicy` resource exists, the `Ready` condition becomes `True`, and `status.lastDecision` becomes `NoopPhase1ScaffoldReady`.

For Phase 2, the `StarvationEvent` shows `Processed=True`. The `BeaconPolicy` status shows `lastDecision=StarvationEventObserved`, plus `lastEventName`, `lastSignalType`, `lastEventSeverity`, and `lastReactionLatencyMillis`.
