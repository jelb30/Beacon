# Beacon Terraform

This directory is a production-grade Terraform template for deploying Beacon Kubernetes infrastructure.

It is intentionally separate from the Go operator code. The reusable modules live under `terraform/modules`, and environment-specific roots live under `terraform/envs`.

## Layout

```text
terraform/
  modules/
    beacon-namespace/
    beacon-crds/
    beacon-operator/
    beacon-agent/
  envs/
    local-kind/
    aws-dev/
```

## What The Modules Do

- `beacon-namespace`: creates the namespace used by Beacon components.
- `beacon-crds`: installs the `BeaconPolicy` and `StarvationEvent` CRDs from the generated CRD YAML files.
- `beacon-operator`: deploys the Beacon controller manager, metrics service, and RBAC.
- `beacon-agent`: deploys the Linux cgroup PSI agent as a DaemonSet with `/sys/fs/cgroup` mounted read-only from each node.

## Local Kind Deployment

From the repository root:

```bash
cd terraform/envs/local-kind
terraform init
terraform fmt -recursive ../..
terraform validate
terraform apply
```

The local environment uses your kubeconfig, usually `~/.kube/config`.

## AWS Dev Deployment

The `terraform/envs/aws-dev` environment includes an S3 backend example:

```hcl
backend "s3" {
  bucket         = "REPLACE_ME_beacon_tf_state"
  key            = "beacon/aws-dev/terraform.tfstate"
  region         = "us-east-1"
  dynamodb_table = "REPLACE_ME_beacon_tf_lock"
  encrypt        = true
}
```

Before using it, replace the placeholder bucket and DynamoDB table names with real infrastructure owned by your team.

No AWS credentials or secrets are stored in this repo. Use your normal AWS credential chain, such as AWS SSO, environment variables, or an IAM role.

To validate the structure without configuring the backend yet:

```bash
cd terraform/envs/aws-dev
terraform init -backend=false
terraform validate
```

After replacing the backend placeholders:

```bash
terraform init
terraform apply
```

## Why S3 Plus DynamoDB Remote State Matters

Terraform state records what infrastructure exists and which Terraform resource owns it. If every engineer keeps their own local state file, two people can make conflicting changes without knowing it.

An S3 backend stores one shared state file in a central bucket. Everyone plans and applies against the same source of truth.

DynamoDB state locking prevents race conditions. When one engineer runs `terraform apply`, Terraform writes a lock row into DynamoDB. If another engineer tries to apply at the same time, Terraform refuses until the first apply finishes and releases the lock.

That prevents common team problems:

- Two applies changing the same Kubernetes resource at the same time.
- One engineer planning from stale state.
- Accidental overwrites caused by separate local state files.
- Broken deploys caused by half-finished concurrent infrastructure changes.

## Notes And Limitations

This Terraform is a deployment template. It assumes:

- The Kubernetes cluster already exists.
- Your kubeconfig can reach the cluster.
- Beacon images are already built and pushed.
- The AWS S3 bucket and DynamoDB lock table already exist before using the `aws-dev` backend.

Future production hardening can add:

- Dedicated module for creating the S3 state bucket and DynamoDB lock table.
- Helm-style values for all BeaconPolicy demo settings.
- Environment-specific image tags instead of `latest`.
- IRSA or workload identity for cloud-native permissions.
