output "ingress_lb_address" {
  description = "External hostname or IP of the ingress-nginx LoadBalancer Service. Empty until MetalLB / cloud LB assigns one."
  value = try(
    helm_release.ingress_nginx.metadata[0].name,
    "ingress-nginx",
  )
}

output "vault_address" {
  description = "Cluster-internal address of Vault."
  value       = "http://vault.vault.svc.cluster.local:8200"
}

output "argocd_admin_secret" {
  description = "Name of the Secret holding the initial Argo CD admin password."
  value       = "argocd-initial-admin-secret"
}

output "grafana_admin_secret" {
  description = "Name of the Secret holding Grafana's admin credentials."
  value       = "kube-prometheus-stack-grafana"
}

output "ready" {
  description = "Sentinel — depends on every helm_release in this module."
  value       = "ok"
  depends_on = [
    helm_release.ingress_nginx,
    helm_release.cert_manager,
    kubectl_manifest.selfsigned_issuer,
    kubectl_manifest.letsencrypt_issuer,
    helm_release.cnpg,
    helm_release.minio_operator,
    helm_release.eso,
    helm_release.vault,
    helm_release.kube_prom,
    helm_release.loki,
    helm_release.promtail,
    helm_release.argocd,
  ]
}
