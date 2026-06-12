variable "namespace" {
  description = "Namespace where the Beacon agent DaemonSet runs."
  type        = string
}

variable "agent_image" {
  description = "Beacon agent image."
  type        = string
  default     = "ghcr.io/jelb30/beacon-agent:latest"
}

variable "target_namespace" {
  description = "Namespace where StarvationEvent resources are created."
  type        = string
  default     = "default"
}

variable "policy_name" {
  description = "BeaconPolicy name to route detected starvation events to."
  type        = string
  default     = "sample-api-policy"
}

variable "deployment_name" {
  description = "Target Deployment name reported by the agent."
  type        = string
  default     = "sample-api"
}

variable "container_name" {
  description = "Target container name reported by the agent."
  type        = string
  default     = "api"
}

variable "interval" {
  description = "cgroup PSI detection interval."
  type        = string
  default     = "5s"
}

variable "psi_threshold_avg10" {
  description = "PSI avg10 threshold that triggers a StarvationEvent."
  type        = string
  default     = "10.0"
}

variable "labels" {
  description = "Extra labels for agent resources."
  type        = map(string)
  default     = {}
}
