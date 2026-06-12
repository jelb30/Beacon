# Beacon

[![Tests](https://github.com/jelb30/Beacon/actions/workflows/test.yml/badge.svg)](https://github.com/jelb30/Beacon/actions/workflows/test.yml)
[![Lint](https://github.com/jelb30/Beacon/actions/workflows/lint.yml/badge.svg)](https://github.com/jelb30/Beacon/actions/workflows/lint.yml)
![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)
![Kubernetes](https://img.shields.io/badge/Kubernetes-Operator-326CE5)

Beacon is a Go Kubernetes Operator that reacts to container resource starvation by patching Deployment CPU and memory requests through an event-driven control path. A Linux `beacon-agent` DaemonSet reads cgroup Pressure Stall Information (PSI), emits `StarvationEvent` custom resources, and lets the operator bypass metrics-server scrape delays for sub-4s synthetic scale-up reaction latency.

## The Problem

Standard Kubernetes autoscaling paths usually depend on sampled metrics. HPA commonly reacts through metrics-server polling, and VPA-style recommendation loops often operate on even slower observation windows. In practical clusters, this means scale-up decisions can lag workload pressure by 15-60 seconds.

That delay is dangerous during sharp traffic spikes. A container can become CPU-starved or memory-constrained before a scrape pipeline has enough fresh data to trigger scaling. In extreme cases, the workload burns through memory headroom and hits OOMKills while the autoscaler is still waiting on its next metrics sample.

Beacon treats starvation as an event, not as a delayed metric trend.

## The Solution

Beacon splits detection from actuation:

- `beacon-agent` runs as a Linux DaemonSet and reads node cgroup PSI files such as `/sys/fs/cgroup/cpu.pressure` and `/sys/fs/cgroup/memory.pressure`.
- When `some` or `full` PSI `avg10` crosses a configured threshold, the agent creates a `StarvationEvent` CR.
- The Go operator watches `StarvationEvent` resources and immediately routes each event to a `BeaconPolicy`.
- The operator calculates the next safe request value and patches the target Deployment container.
- Status, OpenTelemetry spans, and Prometheus metrics record the event-to-patch decision path.

```text
Linux Kernel PSI
  /sys/fs/cgroup/{cpu,memory}.pressure
        |
        v
beacon-agent DaemonSet
        |
        v
StarvationEvent CRD
        |
        v
Beacon Go Operator
        |
        v
Deployment resource request patch
```

Beacon currently implements a production-shaped cgroup-PSI detector. The agent package keeps Linux-specific detector code behind Go build tags so future eBPF attachment logic can be added without breaking macOS/local synthetic workflows.

## Key Features

- **Event-driven vertical scaling:** `StarvationEvent` objects trigger immediate Deployment request patches without waiting for metrics-server scrape intervals.
- **Linux cgroup PSI detection:** `beacon-agent --mode cgroup-psi` reads kernel pressure stall signals from host-mounted cgroup files.
- **Safe request bounds:** `BeaconPolicy` enforces `minCPURequest`, `maxCPURequest`, optional memory bounds, and percentage-based scale steps.
- **Sub-4s synthetic benchmark path:** local benchmark scripts measure event creation time, request before/after, policy latency, event latency, and pass/fail against a 4000ms budget.
- **OpenTelemetry tracing:** reconcile hot paths emit spans for fetch, calculation, patch, and status update stages.
- **Prometheus metrics:** Beacon exports reconcile latency histograms, processed-event counters, patch counters, and scale error counters.
- **Terraform deployment packaging:** reusable Terraform modules deploy CRDs, namespace, operator, and agent; `aws-dev` demonstrates S3 remote state with DynamoDB locking.

## Custom Resources

`BeaconPolicy` defines the scaling envelope for one workload:

```yaml
apiVersion: autoscaling.beacon.dev/v1alpha1
kind: BeaconPolicy
metadata:
  name: sample-api-policy
  namespace: default
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: sample-api
  minCPURequest: "100m"
  maxCPURequest: "1000m"
  minMemoryRequest: "128Mi"
  maxMemoryRequest: "1Gi"
  scaleUpStepPercent: 25
  starvationWindowSeconds: 10
  reactionLatencyBudgetMillis: 4000
```

`StarvationEvent` is the event payload consumed by the operator:

```yaml
apiVersion: autoscaling.beacon.dev/v1alpha1
kind: StarvationEvent
metadata:
  name: sample-api-cpu-starvation
  namespace: default
spec:
  policyName: sample-api-policy
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: sample-api
  containerName: api
  signalType: CPUStarvation
  observedAt: "2026-06-11T12:00:00Z"
  severity: High
```

## Quick Start: Local Synthetic Demo

This workflow runs the operator on your workstation and uses the synthetic detector path. It is the fastest way to verify the end-to-end event-to-patch loop without requiring Linux cgroup PSI.

Prerequisites:

- Go 1.25+
- `make`
- `kubectl`
- A local Kubernetes cluster, such as kind

Terminal 1:

```bash
make install
make run
```

Terminal 2:

```bash
make demo-reset
make agent-once
```

Inspect the result:

```bash
kubectl get starvationevents -A
kubectl get beaconpolicy sample-api-policy -n default -o yaml
kubectl get deployment sample-api -n default -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
```

Expected behavior:

- `beacon-agent` creates a `StarvationEvent`.
- The operator marks the event as processed.
- The target Deployment CPU request changes from `100m` to `125m`.
- `BeaconPolicy.status.lastDecision` becomes `VerticalScalePatchApplied`.

Run the latency harness:

```bash
make demo-reset
make benchmark
make benchmark-series
```

Example output:

```text
## Beacon Synthetic Scale-Up Benchmark

Policy: sample-api-policy
Deployment: sample-api
Container: api
Signal: CPUStarvation
Severity: High

CPU request before: 100m
CPU request after: 125m

StarvationEvent latency: 537ms
BeaconPolicy latency: 537ms
Latency budget: 4000ms
Decision: VerticalScalePatchApplied
Result: PASS
```

## Linux Agent Deployment

Build and push the agent image:

```bash
make docker-build-agent AGENT_IMG=ghcr.io/jelb30/beacon-agent:latest
make docker-push-agent AGENT_IMG=ghcr.io/jelb30/beacon-agent:latest
```

Deploy the cgroup-PSI DaemonSet:

```bash
make install
kubectl apply -f config/agent/daemonset.yaml
kubectl rollout status daemonset/beacon-agent -n beacon-system
```

The DaemonSet mounts host cgroups read-only:

```text
host:      /sys/fs/cgroup
container: /host/sys/fs/cgroup
```

The detector reads:

```text
BEACON_CGROUP_PSI_CPU_PATH=/host/sys/fs/cgroup/cpu.pressure
BEACON_CGROUP_PSI_MEMORY_PATH=/host/sys/fs/cgroup/memory.pressure
```

The default PSI trigger threshold is `avg10 > 10.0`. It can be overridden with `BEACON_CGROUP_PSI_THRESHOLD_AVG10`.

## Production Deployment With Terraform

Beacon includes Terraform modules under `terraform/modules`:

- `beacon-namespace`: creates the Beacon namespace.
- `beacon-crds`: installs `BeaconPolicy` and `StarvationEvent` CRDs.
- `beacon-operator`: deploys the controller manager, metrics service, and RBAC.
- `beacon-agent`: deploys the Linux cgroup-PSI DaemonSet and RBAC.

Local kind environment:

```bash
cd terraform/envs/local-kind
terraform init
terraform validate
terraform apply
```

AWS dev environment:

```bash
cd terraform/envs/aws-dev
terraform init
terraform validate
terraform apply
```

`terraform/envs/aws-dev/backend.tf` shows a team-safe remote state backend:

```hcl
terraform {
  backend "s3" {
    bucket         = "REPLACE_ME_beacon_tf_state"
    key            = "beacon/aws-dev/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "REPLACE_ME_beacon_tf_lock"
    encrypt        = true
  }
}
```

S3 stores the shared Terraform state file. DynamoDB provides state locking so two engineers cannot run conflicting `terraform apply` operations at the same time. This prevents stale plans, concurrent writes, and accidental infrastructure ownership drift in team workflows.

The Terraform directory is a production-grade deployment template. It assumes the Kubernetes cluster, S3 state bucket, and DynamoDB lock table already exist.

## Observability

Beacon initializes OpenTelemetry tracing at operator startup. Local development uses stdout spans by default and can be disabled with:

```bash
BEACON_TRACING_DISABLED=true make run
```

Primary spans:

- `BeaconPolicyReconcile`
- `StarvationEventReconcile`
- `fetch_starvation_event`
- `fetch_beacon_policy`
- `fetch_target_deployment`
- `calculate_resource_patch`
- `patch_deployment`
- `update_beacon_policy_status`
- `update_starvation_event_status`

Important span attributes include:

- `signalType`
- `severity`
- `targetDeployment`
- `containerName`
- `previous_cpu_request`
- `new_cpu_request`
- `previous_memory_request`
- `new_memory_request`
- `scale_action`
- `reaction_latency_ms`
- `decision`

Prometheus metrics are exposed through the controller-runtime metrics endpoint:

- `beacon_reconcile_latency_milliseconds`
- `beacon_starvation_events_processed_total`
- `beacon_vertical_scale_patches_total`
- `beacon_vertical_scale_errors_total`

For local metrics scraping:

```bash
go run ./cmd/main.go --metrics-bind-address=:8080 --metrics-secure=false
curl -s localhost:8080/metrics | grep '^beacon_'
```

Tracing and metrics make the reconcile loop debuggable at the level that matters for autoscaling latency: target lookup, resource calculation, Deployment patch, and status update.

## Development Commands

```bash
make generate
make manifests
go test ./...
make install
make run
make demo-reset
make benchmark
make agent-once
make docker-build
make docker-build-agent
```

## Repository Layout

```text
api/v1alpha1/              BeaconPolicy and StarvationEvent API types
cmd/main.go                operator entrypoint
cmd/beacon-agent/          node agent entrypoint
internal/controller/       reconcile loops and vertical patch engine
internal/agent/            detector interface, synthetic detector, Linux cgroup-PSI detector
internal/observability/    OpenTelemetry and Prometheus instrumentation
config/agent/              Linux DaemonSet deployment for beacon-agent
config/samples/            sample workload and custom resources
hack/                      benchmark and demo scripts
terraform/                 reusable Terraform modules and environment roots
docs/                      deeper design notes
```

## Current Scope

Implemented:

- Go controller-runtime operator
- `BeaconPolicy` and `StarvationEvent` CRDs
- Deployment CPU/memory request patching
- Synthetic and cgroup-PSI starvation sources
- Agent DaemonSet packaging
- OpenTelemetry spans and Prometheus metrics
- Terraform deployment modules with S3/DynamoDB backend example

Not implemented yet:

- eBPF program attachment and kernel event streaming
- automatic workload discovery across all containers
- historical recommendation engine
- full cloud cluster provisioning
