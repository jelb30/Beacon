resource "kubernetes_manifest" "crd" {
  for_each = toset(var.crd_manifest_paths)

  manifest = yamldecode(file(each.value))
}
