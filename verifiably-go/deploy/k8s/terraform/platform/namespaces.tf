# Pre-create namespaces so each helm_release can target an existing
# namespace and we control labels (PodSecurity comes in Phase 7.2).
locals {
  namespaces = toset([
    "ingress-nginx",
    "cert-manager",
    "cnpg-system",
    "minio-operator",
    "external-secrets",
    "vault",
    "monitoring",
    "logging",
    "argocd",
  ])
}

resource "kubernetes_namespace" "platform" {
  for_each = local.namespaces
  metadata {
    name = each.value
    labels = {
      "app.kubernetes.io/managed-by" = "terraform"
      "verifiably.io/layer"          = "platform"
    }
  }
}
