# Benchmarking Beacon

Phase 4 provides a synthetic benchmark/demo harness for Beacon's event-driven scaling path.

## What It Measures

The benchmark measures event-to-Deployment-patch latency for synthetic `StarvationEvent` objects. The timer starts at `spec.observedAt` on the event and ends when Beacon processes the event, patches the target Deployment request, and records the reaction latency in `StarvationEvent.status.reactionLatencyMillis` and `BeaconPolicy.status.lastReactionLatencyMillis`.

This is useful for validating the control-loop path:

1. Create a synthetic starvation event.
2. Reconcile the event.
3. Resolve the referenced `BeaconPolicy`.
4. Patch the target Deployment container request.
5. Record status and latency.

## What It Does Not Measure Yet

This phase does not measure eBPF signal generation, kernel-to-controller delivery, Prometheus scrape latency, OpenTelemetry traces, Terraform deployment time, or full application recovery time after scaling. The signal source is synthetic Kubernetes API input.

## How To Run

Start the operator:

```sh
make install
make run
```

In another terminal:

```sh
make demo-reset
make benchmark
```

For repeated runs:

```sh
RUNS=5 make benchmark-series
```

Useful manual checks:

```sh
kubectl get deployment sample-api -n default -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
kubectl get starvationevents -A
kubectl get beaconpolicy sample-api-policy -n default -o yaml
```

## How To Interpret Results

`Result: PASS` means Beacon processed the synthetic event, increased the CPU request for a CPU starvation event, and stayed within the configured latency budget.

`Result: PASS_MAX_REACHED` means Beacon processed the event and correctly avoided patching because the configured maximum request was already reached.

`Result: FAIL` means the event was not processed, the Deployment request did not change when it should have, the policy decision was unexpected, or the recorded latency exceeded the budget.

## Why Event-Driven Reconciliation Matters

Periodic scrape and polling paths usually wait for the next collection interval before a controller can react. Beacon's synthetic path models an event-driven design: once a starvation signal exists as a Kubernetes object, reconciliation can run immediately and patch the workload without waiting for a metrics scrape period.

That is the core resume-defensible claim this phase demonstrates: low-latency event-to-patch behavior under synthetic signal input. Once eBPF integration is added, the same benchmark structure can be extended to measure real signal-to-patch latency.
