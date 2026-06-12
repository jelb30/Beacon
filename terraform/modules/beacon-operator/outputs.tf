output "deployment_name" {
  description = "Beacon operator Deployment name."
  value       = kubernetes_deployment_v1.manager.metadata[0].name
}

output "service_account_name" {
  description = "Beacon operator ServiceAccount name."
  value       = kubernetes_service_account_v1.manager.metadata[0].name
}

output "metrics_service_name" {
  description = "Beacon operator metrics Service name."
  value       = kubernetes_service_v1.metrics.metadata[0].name
}
