# All providers read from a single kubeconfig file written by the bootstrap
# module (kind / k3s / EKS). No cluster-specific config baked in here —
# that's the cloud-agnostic guarantee.
provider "kubernetes" {
  config_path = var.kubeconfig_path
}

provider "helm" {
  kubernetes {
    config_path = var.kubeconfig_path
  }
}

provider "kubectl" {
  config_path      = var.kubeconfig_path
  load_config_file = true
}
