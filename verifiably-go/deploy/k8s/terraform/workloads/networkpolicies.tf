# Phase 7.2 — namespace-scoped default-deny + per-service allow-list.
# Each chart additionally declares its own NetworkPolicy; this layer is
# the safety net.

resource "kubectl_manifest" "default_deny" {
  yaml_body = yamlencode({
    apiVersion = "networking.k8s.io/v1"
    kind       = "NetworkPolicy"
    metadata = {
      name      = "default-deny-all"
      namespace = var.namespace
    }
    spec = {
      podSelector = {}
      policyTypes = ["Ingress", "Egress"]
    }
  })
  depends_on = [kubernetes_namespace.waltid]
}

# Allow DNS — every workload needs it.
resource "kubectl_manifest" "allow_dns" {
  yaml_body = yamlencode({
    apiVersion = "networking.k8s.io/v1"
    kind       = "NetworkPolicy"
    metadata = {
      name      = "allow-dns"
      namespace = var.namespace
    }
    spec = {
      podSelector = {}
      policyTypes = ["Egress"]
      egress = [{
        to = [{
          namespaceSelector = {}
          podSelector       = { matchLabels = { "k8s-app" = "kube-dns" } }
        }]
        ports = [
          { protocol = "UDP", port = 53 },
          { protocol = "TCP", port = 53 },
        ]
      }]
    }
  })
  depends_on = [kubernetes_namespace.waltid]
}
