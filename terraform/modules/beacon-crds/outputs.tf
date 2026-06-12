output "crd_names" {
  description = "Names of the Beacon CRDs managed by this module."
  value = [
    for crd in kubernetes_manifest.crd :
    crd.manifest.metadata.name
  ]
}
