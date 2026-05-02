variable "cluster_name" {
  description = "k3s cluster name (used in the kubeconfig context)."
  type        = string
  default     = "verifiably-onprem"
}

variable "nodes" {
  description = "User-supplied list of nodes. The first 'server' node bootstraps the control plane; the rest join."
  type = list(object({
    ip   = string
    role = string # "server" or "agent"
  }))
}

variable "ssh_user" {
  description = "SSH user for k3sup."
  type        = string
  default     = "ubuntu"
}

variable "ssh_key_path" {
  description = "Path to the SSH private key for k3sup."
  type        = string
}

variable "k3s_version" {
  description = "Pinned k3s channel or version."
  type        = string
  default     = "v1.30.4+k3s1"
}

variable "kubeconfig_dir" {
  description = "Where to write the rendered kubeconfig."
  type        = string
  default     = "../../.tfstate"
}
