output "daemonset_name" {
  description = "Beacon agent DaemonSet name."
  value       = kubernetes_daemon_set_v1.agent.metadata[0].name
}

output "service_account_name" {
  description = "Beacon agent ServiceAccount name."
  value       = kubernetes_service_account_v1.agent.metadata[0].name
}
