# Benchmarking Beacon

Phase 4 provides a synthetic benchmark/demo harness for Beacon's event-driven scaling path.

## What It Measures

The benchmark measures event-to-Deployment-patch latency for synthetic `StarvationEvent` objects. The timer starts at `spec.observedAt` on the event and ends when Beacon processes the event, patches the target Deployment request, and records the reaction latency in `StarvationEvent.status.reactionLatencyMillis` and `BeaconPolicy.status.lastReactionLatencyMillis`.

Phase 6 adds OpenTelemetry spans and Prometheus metrics around the same reconcile path. The benchmark still uses the Kubernetes status fields as its source of truth; it does not depend on tracing or metrics being scraped.

This is useful for validating the control-loop path:

1. Create a synthetic starvation event.
2. Reconcile the event.
3. Resolve the referenced `BeaconPolicy`.
4. Patch the target Deployment container request.
5. Record status and latency.

## What It Does Not Measure Yet

This phase does not measure eBPF signal generation, kernel-to-controller delivery, Prometheus scrape latency, OpenTelemetry exporter delivery, Terraform deployment time, or full application recovery time after scaling. The signal source is synthetic Kubernetes API input.

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

If the operator is running with Phase 6 observability, stdout spans appear in the operator terminal and custom metrics are available from the manager metrics endpoint when it is enabled.

## How To Interpret Results

`Result: PASS` means Beacon processed the synthetic event, increased the CPU request for a CPU starvation event, and stayed within the configured latency budget.

`Result: PASS_MAX_REACHED` means Beacon processed the event and correctly avoided patching because the configured maximum request was already reached.

`Result: FAIL` means the event was not processed, the Deployment request did not change when it should have, the policy decision was unexpected, or the recorded latency exceeded the budget.

## Why Event-Driven Reconciliation Matters

Periodic scrape and polling paths usually wait for the next collection interval before a controller can react. Beacon's synthetic path models an event-driven design: once a starvation signal exists as a Kubernetes object, reconciliation can run immediately and patch the workload without waiting for a metrics scrape period.

That is the core resume-defensible claim this phase demonstrates: low-latency event-to-patch behavior under synthetic signal input. Once eBPF integration is added, the same benchmark structure can be extended to measure real signal-to-patch latency.

## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpustarvation-1781221647-51421
Event creation time: 2026-06-11T23:47:27Z

CPU request before: 100m
CPU request after: 125m

StarvationEvent latency: 782ms
BeaconPolicy latency: 782ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS
JELB@jelb beacon % make benchmark-series
./hack/run-benchmark-series.sh
deployment.apps/sample-api configured
beaconpolicy.autoscaling.beacon.dev/sample-api-policy unchanged
starvationevent.autoscaling.beacon.dev "benchmark-cpustarvation-1781221647-51421" deleted from default namespace
No resources found
deployment.apps/sample-api condition met
Sample API reset complete.
Deployment: default/sample-api
Container: api
Current CPU request: 100m

## Beacon Benchmark Series

Runs: 5

### Run 1/5
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpustarvation-1781221674-51499
Event creation time: 2026-06-11T23:47:54Z

CPU request before: 100m
CPU request after: 125m

StarvationEvent latency: 400ms
BeaconPolicy latency: 400ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS

### Run 2/5
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpustarvation-1781221674-51533
Event creation time: 2026-06-11T23:47:54Z

CPU request before: 125m
CPU request after: 156m

StarvationEvent latency: 896ms
BeaconPolicy latency: 896ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS

### Run 3/5
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpustarvation-1781221675-51567
Event creation time: 2026-06-11T23:47:55Z

CPU request before: 156m
CPU request after: 195m

StarvationEvent latency: 364ms
BeaconPolicy latency: 364ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS

### Run 4/5
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpustarvation-1781221675-51601
Event creation time: 2026-06-11T23:47:55Z

CPU request before: 195m
CPU request after: 243m

StarvationEvent latency: 839ms
BeaconPolicy latency: 839ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS

### Run 5/5
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

Event: benchmark-cpustarvation-1781221676-51636
Event creation time: 2026-06-11T23:47:56Z

CPU request before: 243m
CPU request after: 303m

StarvationEvent latency: 316ms
BeaconPolicy latency: 316ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS

## Series Summary

Passes: 5
Failures: 0
Min latency: 316ms
Max latency: 896ms
Average latency: 563ms
