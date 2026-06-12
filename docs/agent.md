# Beacon Agent

`beacon-agent` is the starvation signal source for Beacon. It turns detector output into `StarvationEvent` custom resources, which the Beacon operator reconciles into Deployment resource request patches.

## Why It Exists

Before Phase 5, synthetic events came from shell scripts. The agent moves that responsibility into a Go command with the same Kubernetes client stack as the operator. This gives Beacon a clear path from signal detection to Kubernetes event ingestion without coupling detector code to controller code.

## Supported Modes

- `synthetic`: Emits deterministic starvation detections for local development. This works on macOS, kind, and normal kubeconfig-based clusters.
- `cgroup-psi`: Linux-only placeholder for a future cgroup pressure-stall detector.
- `ebpf`: Linux-only placeholder for a future eBPF/cgroup detector.

On non-Linux platforms, `cgroup-psi` and `ebpf` return a clear error and do not affect normal builds or tests.

## Local Synthetic Workflow

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

Inspect the event and patch result:

```sh
kubectl get starvationevents -A
kubectl get beaconpolicy sample-api-policy -n default -o yaml
kubectl get deployment sample-api -n default -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
```

For continuous synthetic events:

```sh
make agent-run
```

## What Synthetic Mode Proves

Synthetic mode proves the integration path:

1. The agent creates a `StarvationEvent`.
2. The operator observes the event.
3. The operator routes it to a `BeaconPolicy`.
4. The operator patches the target Deployment request.
5. Policy and event status record the reaction latency and decision.

This is useful for local demos and repeatable validation because it does not require Linux kernel features.

## Linux And eBPF Notes

Linux detector modes are behind Go build tags. They are currently stubs, not real signal detectors. Real eBPF support will require:

- Linux nodes.
- Access to the relevant cgroup and process metadata.
- Elevated permissions for loading and attaching programs.
- A deployment model that runs the agent with the right node-level privileges.

The final resume wording should only say "eBPF-detected starvation" after the Linux detector is implemented, documented, and validated.

## Limitations

- Synthetic mode does not measure kernel-level starvation detection latency.
- The current Linux detector modes do not emit events.
- The local path measures event-to-patch behavior, not full application recovery time.
- The optional Kubernetes Job sample assumes a future image that includes the `beacon-agent` binary.

## Resume Mapping

Phase 5 supports a defensible claim that Beacon has an event-driven node-agent path that creates Kubernetes starvation events and triggers low-latency operator reconciliation. Once Linux eBPF or cgroup detection is implemented, the same path can support a stronger claim about kernel-sourced starvation detection feeding automatic vertical scaling.
