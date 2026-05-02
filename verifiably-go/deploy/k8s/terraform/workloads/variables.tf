variable "kubeconfig_path" {
  description = "Path to the cluster kubeconfig (output of bootstrap/*)."
  type        = string
}

variable "domain" {
  description = "Public DNS root for ingress hostnames."
  type        = string
}

variable "lb_mode" {
  description = "Mirrors platform module. Drives ClusterIssuer choice on Ingress."
  type        = string
  default     = "metallb"
}

variable "umbrella_chart_path" {
  description = "Path to the umbrella waltid chart directory."
  type        = string
  default     = "../../helm/umbrella/waltid"
}

variable "values_file" {
  description = "Optional override values file (e.g. values-prod.yaml)."
  type        = string
  default     = ""
}

variable "namespace" {
  type    = string
  default = "waltid"
}

variable "vault_address" {
  description = "Cluster-internal Vault address from platform module output."
  type        = string
  default     = "http://vault.vault.svc.cluster.local:8200"
}
