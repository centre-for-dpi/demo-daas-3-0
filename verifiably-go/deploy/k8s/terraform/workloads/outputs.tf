output "namespace" {
  value = kubernetes_namespace.waltid.metadata[0].name
}

output "release_name" {
  value = helm_release.waltid.name
}

output "app_url" {
  value = "https://app.${var.domain}"
}

output "wallet_url" {
  value = "https://wallet.${var.domain}"
}

output "issuer_url" {
  value = "https://issuer.${var.domain}"
}

output "verifier_url" {
  value = "https://verifier.${var.domain}"
}
