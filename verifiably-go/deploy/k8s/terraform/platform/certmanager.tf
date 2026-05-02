# cert-manager — installs CRDs + controllers, then two ClusterIssuers:
# 'selfsigned' for on-prem / dev (self-signed CA roots), and 'letsencrypt'
# for any reachable public domain. Walt.id chart Ingresses pick one via
# the cert-manager.io/cluster-issuer annotation.
resource "helm_release" "cert_manager" {
  name       = "cert-manager"
  repository = "https://charts.jetstack.io"
  chart      = "cert-manager"
  version    = var.chart_versions.cert_manager
  namespace  = "cert-manager"

  values = [yamlencode({
    installCRDs = true
    prometheus = {
      enabled = true
      servicemonitor = {
        enabled   = true
        namespace = "monitoring"
      }
    }
    extraArgs = ["--enable-certificate-owner-ref=true"]
  })]

  wait    = true
  timeout = 600

  depends_on = [kubernetes_namespace.platform, helm_release.kube_prom]
}

resource "kubectl_manifest" "selfsigned_issuer" {
  yaml_body = yamlencode({
    apiVersion = "cert-manager.io/v1"
    kind       = "ClusterIssuer"
    metadata   = { name = "selfsigned" }
    spec       = { selfSigned = {} }
  })
  depends_on = [helm_release.cert_manager]
}

resource "kubectl_manifest" "letsencrypt_issuer" {
  yaml_body = yamlencode({
    apiVersion = "cert-manager.io/v1"
    kind       = "ClusterIssuer"
    metadata   = { name = "letsencrypt" }
    spec = {
      acme = {
        email               = var.cluster_issuer_email
        server              = "https://acme-v02.api.letsencrypt.org/directory"
        privateKeySecretRef = { name = "letsencrypt-account-key" }
        solvers             = [{ http01 = { ingress = { class = "nginx" } } }]
      }
    }
  })
  depends_on = [helm_release.cert_manager, helm_release.ingress_nginx]
}
