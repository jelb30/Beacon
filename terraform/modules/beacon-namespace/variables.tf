variable "name" {
  description = "Kubernetes namespace for Beacon components."
  type        = string
  default     = "beacon-system"
}

variable "labels" {
  description = "Labels to apply to the namespace."
  type        = map(string)
  default = {
    "app.kubernetes.io/name"       = "beacon"
    "app.kubernetes.io/managed-by" = "terraform"
  }
}
