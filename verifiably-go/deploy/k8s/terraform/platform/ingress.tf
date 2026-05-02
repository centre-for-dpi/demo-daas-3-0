# ingress-nginx — single ingress controller for the entire cluster.
# Service type LoadBalancer → MetalLB on-prem, NLB on EKS (we don't pin
# the AWS LB controller; users running on EKS install it via bootstrap/aws-eks).
resource "helm_release" "ingress_nginx" {
  name       = "ingress-nginx"
  repository = "https://kubernetes.github.io/ingress-nginx"
  chart      = "ingress-nginx"
  version    = var.chart_versions.ingress_nginx
  namespace  = "ingress-nginx"

  values = [yamlencode({
    controller = {
      replicaCount = 2
      service = {
        type = "LoadBalancer"
        annotations = var.lb_mode == "cloud" ? {
          "service.beta.kubernetes.io/aws-load-balancer-type"            = "external"
          "service.beta.kubernetes.io/aws-load-balancer-scheme"          = "internet-facing"
          "service.beta.kubernetes.io/aws-load-balancer-nlb-target-type" = "ip"
        } : {}
      }
      metrics = {
        enabled = true
        serviceMonitor = {
          enabled   = true
          namespace = "monitoring"
        }
      }
      admissionWebhooks = { patch = { enabled = true } }
      # The ingress-nginx 1.11 image uses non-numeric user "www-data".
      # Kubelet refuses to start a container with runAsNonRoot=true unless
      # the user is given as a numeric UID it can compare to 0 — the image's
      # www-data is UID 101.
      podSecurityContext = {
        runAsNonRoot   = true
        runAsUser      = 101
        runAsGroup     = 101
        fsGroup        = 101
        seccompProfile = { type = "RuntimeDefault" }
      }
      containerSecurityContext = {
        runAsNonRoot             = true
        runAsUser                = 101
        runAsGroup               = 101
        readOnlyRootFilesystem   = false # nginx writes /tmp
        allowPrivilegeEscalation = false
        capabilities = {
          drop = ["ALL"]
          add  = ["NET_BIND_SERVICE"]
        }
      }
    }
  })]

  wait    = true
  timeout = 600

  depends_on = [kubernetes_namespace.platform, helm_release.kube_prom]
}
