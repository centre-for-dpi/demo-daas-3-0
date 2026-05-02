variable "kubeconfig_path" {
  description = "Absolute path to a working kubeconfig (output of bootstrap/*)."
  type        = string
}

variable "domain" {
  description = "Public DNS root for ingress hostnames (issuer.<domain>, etc.)."
  type        = string
}

variable "cluster_issuer_email" {
  description = "Contact email for cert-manager ACME registrations."
  type        = string
}

variable "lb_mode" {
  description = "Load-balancer mode: 'metallb' (kind, on-prem k3s) or 'cloud' (EKS, GKE, AKS)."
  type        = string
  default     = "metallb"
  validation {
    condition     = contains(["metallb", "cloud"], var.lb_mode)
    error_message = "lb_mode must be 'metallb' or 'cloud'."
  }
}

variable "metallb_already_installed" {
  description = "Skip the MetalLB install when true (e.g. bootstrap/local-kind already did it)."
  type        = bool
  default     = true
}

variable "vault_replicas" {
  description = "Number of Vault HA replicas (Raft requires odd)."
  type        = number
  default     = 3
}

variable "vault_storage_size" {
  description = "PVC size per Vault replica."
  type        = string
  default     = "10Gi"
}

variable "storage_class" {
  description = "StorageClass for stateful workloads. Empty = cluster default."
  type        = string
  default     = ""
}

# --- Pinned chart versions (single place to bump) ---
variable "chart_versions" {
  description = "Pinned chart versions for every helm_release in this module."
  type        = map(string)
  default = {
    ingress_nginx = "4.11.3"
    cert_manager  = "v1.16.1"
    metallb       = "0.14.8"
    cnpg          = "0.22.1"
    minio         = "5.0.15"
    eso           = "0.10.4"
    vault         = "0.28.1"
    kube_prom     = "65.5.1"
    loki          = "6.16.0"
    promtail      = "6.16.6"
    argocd        = "7.6.10"
  }
}
