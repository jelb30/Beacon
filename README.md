# Beacon

Beacon is a Kubernetes operator foundation for future workload autoscaling based on starvation signals and reaction latency goals.

Repository: `github.com/jelb30/Beacon`

## Phase 1 Scope

Phase 1 provides a clean Kubebuilder/controller-runtime scaffold with one custom resource: `BeaconPolicy`.

This phase does not implement eBPF collection, Terraform, Prometheus metrics, OpenTelemetry tracing, or real workload scaling. The controller only reconciles `BeaconPolicy` resources and writes a readiness status proving the operator foundation is installed and running.

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

List BeaconPolicy resources:

```sh
kubectl get beaconpolicy -A
```

Inspect the sample policy:

```sh
kubectl describe beaconpolicy sample-api-policy -n default
```

## Expected Result

The `BeaconPolicy` resource exists, the `Ready` condition becomes `True`, and `status.lastDecision` becomes `NoopPhase1ScaffoldReady`.
