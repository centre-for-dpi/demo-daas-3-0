# External Secrets Operator — projects Vault secrets into namespaces as
# native Kubernetes Secrets via ClusterSecretStore. The store itself is
# created by the workloads module (Phase 3.4) once Vault is initialized
# and a Kubernetes-auth role is bound, since that requires runtime data
# Terraform can't compute at plan time.
resource "helm_release" "eso" {
  name       = "external-secrets"
  repository = "https://charts.external-secrets.io"
  chart      = "external-secrets"
  version    = var.chart_versions.eso
  namespace  = "external-secrets"

  values = [yamlencode({
    installCRDs = true
    serviceMonitor = {
      enabled   = true
      namespace = "monitoring"
    }
  })]

  wait    = true
  timeout = 600

  depends_on = [kubernetes_namespace.platform]
}

# HashiCorp Vault — HA mode with Raft storage on PVCs. Self-initializes via
# the bootstrap Job declared in the workloads module; this resource only
# stands up the StatefulSet + Service.
resource "helm_release" "vault" {
  name       = "vault"
  repository = "https://helm.releases.hashicorp.com"
  chart      = "vault"
  version    = var.chart_versions.vault
  namespace  = "vault"

  values = [yamlencode({
    server = {
      ha = {
        enabled  = true
        replicas = var.vault_replicas
        raft = {
          enabled   = true
          setNodeId = true
          config    = <<-HCL
            ui = true
            listener "tcp" {
              tls_disable = 1
              address     = "[::]:8200"
              cluster_address = "[::]:8201"
            }
            storage "raft" {
              path = "/vault/data"
            }
            service_registration "kubernetes" {}
          HCL
        }
      }
      dataStorage = {
        enabled      = true
        size         = var.vault_storage_size
        storageClass = var.storage_class
      }
      readinessProbe = { enabled = true }
      auditStorage   = { enabled = true, size = "5Gi" }
    }
    ui  = { enabled = true }
    csi = { enabled = false } # we use the agent injector pattern in 7.1
    injector = {
      enabled = true
      metrics = { enabled = true }
    }
    serverTelemetry = {
      serviceMonitor = {
        enabled   = true
        namespace = "monitoring"
      }
    }
  })]

  wait    = true
  timeout = 900 # initial Raft elections can be slow on cold starts

  depends_on = [kubernetes_namespace.platform, helm_release.kube_prom]
}
