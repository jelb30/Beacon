locals {
  crd_manifest_paths = [
    abspath("${path.root}/../../../config/crd/bases/autoscaling.beacon.dev_beaconpolicies.yaml"),
    abspath("${path.root}/../../../config/crd/bases/autoscaling.beacon.dev_starvationevents.yaml")
  ]
}

module "namespace" {
  source = "../../modules/beacon-namespace"

  name = var.namespace
}

module "crds" {
  source = "../../modules/beacon-crds"

  crd_manifest_paths = local.crd_manifest_paths
}

module "operator" {
  source = "../../modules/beacon-operator"

  namespace      = module.namespace.name
  operator_image = var.operator_image

  depends_on = [module.crds]
}

module "agent" {
  source = "../../modules/beacon-agent"

  namespace   = module.namespace.name
  agent_image = var.agent_image

  depends_on = [module.crds]
}
