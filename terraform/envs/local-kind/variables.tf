variable "kubeconfig_path" {
  description = "Path to kubeconfig for the local kind cluster."
  type        = string
  default     = "~/.kube/config"
}

variable "kubeconfig_context" {
  description = "Optional kubeconfig context. Leave null to use the current context."
  type        = string
  default     = null
}

variable "namespace" {
  description = "Namespace for Beacon components."
  type        = string
  default     = "beacon-system"
}

variable "operator_image" {
  description = "Beacon operator image."
  type        = string
  default     = "ghcr.io/jelb30/beacon-controller:latest"
}

variable "agent_image" {
  description = "Beacon agent image."
  type        = string
  default     = "ghcr.io/jelb30/beacon-agent:latest"
}
