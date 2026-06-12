variable "namespace" {
  description = "Namespace where the Beacon operator runs."
  type        = string
}

variable "name_prefix" {
  description = "Prefix for Beacon operator resources."
  type        = string
  default     = "beacon"
}

variable "operator_image" {
  description = "Beacon operator manager image."
  type        = string
  default     = "ghcr.io/jelb30/beacon-controller:latest"
}

variable "replicas" {
  description = "Number of operator replicas."
  type        = number
  default     = 1
}

variable "labels" {
  description = "Extra labels for operator resources."
  type        = map(string)
  default     = {}
}
