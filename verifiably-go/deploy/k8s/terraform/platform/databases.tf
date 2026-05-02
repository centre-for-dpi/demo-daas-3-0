# CloudNativePG operator — declarative, in-cluster Postgres for walt.id wallet.
# Replaces RDS/CloudSQL while keeping HA + PITR backups achievable via
# Cluster.spec.backup pointing at the in-cluster MinIO operator (set up below).
resource "helm_release" "cnpg" {
  name       = "cnpg"
  repository = "https://cloudnative-pg.github.io/charts"
  chart      = "cloudnative-pg"
  version    = var.chart_versions.cnpg
  namespace  = "cnpg-system"

  values = [yamlencode({
    monitoring = {
      podMonitorEnabled          = true
      grafanaDashboard           = { create = true, namespace = "monitoring" }
      podMonitorAdditionalLabels = { release = "kube-prometheus-stack" }
    }
  })]

  wait    = true
  timeout = 600

  depends_on = [kubernetes_namespace.platform, helm_release.kube_prom]
}

# MinIO operator — S3-compatible object storage in-cluster. Used by:
#   - CNPG backups (PostgreSQL WAL + base archives)
#   - Loki chunk store
#   - inji-* services if/when added
resource "helm_release" "minio_operator" {
  name       = "minio-operator"
  repository = "https://operator.min.io"
  chart      = "operator"
  version    = var.chart_versions.minio
  namespace  = "minio-operator"

  wait    = true
  timeout = 600

  depends_on = [kubernetes_namespace.platform]
}
