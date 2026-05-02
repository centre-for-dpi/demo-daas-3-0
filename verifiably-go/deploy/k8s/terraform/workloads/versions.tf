terraform {
  required_version = ">= 1.6"
  required_providers {
    helm       = { source = "hashicorp/helm", version = "~> 2.13" }
    kubernetes = { source = "hashicorp/kubernetes", version = "~> 2.30" }
    kubectl    = { source = "alekc/kubectl", version = "~> 2.1" }
    random     = { source = "hashicorp/random", version = "~> 3.6" }
  }
}
