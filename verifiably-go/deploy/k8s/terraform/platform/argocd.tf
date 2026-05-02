# Argo CD — GitOps controller. Workloads module wires Application(s)
# pointing at this repo's helm/umbrella/waltid chart so the umbrella stays
# in sync with main without re-running terraform.
resource "helm_release" "argocd" {
  name       = "argocd"
  repository = "https://argoproj.github.io/argo-helm"
  chart      = "argo-cd"
  version    = var.chart_versions.argocd
  namespace  = "argocd"

  values = [yamlencode({
    global = {
      domain = "argocd.${var.domain}"
    }
    server = {
      ingress = {
        enabled          = true
        ingressClassName = "nginx"
        hostname         = "argocd.${var.domain}"
        tls              = true
        annotations = {
          "cert-manager.io/cluster-issuer"               = var.lb_mode == "cloud" ? "letsencrypt" : "selfsigned"
          "nginx.ingress.kubernetes.io/ssl-passthrough"  = "true"
          "nginx.ingress.kubernetes.io/backend-protocol" = "HTTPS"
        }
      }
      metrics = {
        enabled        = true
        serviceMonitor = { enabled = true, namespace = "monitoring" }
      }
    }
    controller = {
      metrics = {
        enabled        = true
        serviceMonitor = { enabled = true, namespace = "monitoring" }
      }
    }
    repoServer = {
      metrics = {
        enabled        = true
        serviceMonitor = { enabled = true, namespace = "monitoring" }
      }
    }
    notifications = { enabled = false }
    dex           = { enabled = false } # use Keycloak via OIDC in 7.x
  })]

  wait    = true
  timeout = 900

  depends_on = [helm_release.ingress_nginx, helm_release.cert_manager, helm_release.kube_prom]
}
