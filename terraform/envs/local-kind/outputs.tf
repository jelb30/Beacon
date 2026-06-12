output "namespace" {
  description = "Beacon namespace."
  value       = module.namespace.name
}

output "operator_deployment" {
  description = "Beacon operator Deployment."
  value       = module.operator.deployment_name
}

output "agent_daemonset" {
  description = "Beacon agent DaemonSet."
  value       = module.agent.daemonset_name
}
