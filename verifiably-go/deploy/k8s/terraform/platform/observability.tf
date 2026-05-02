# kube-prometheus-stack — Prometheus + Alertmanager + Grafana + node-exporter.
# ServiceMonitor CRD installed here is consumed by every other helm_release
# above that sets serviceMonitor.enabled = true.
resource "helm_release" "kube_prom" {
  name       = "kube-prometheus-stack"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  version    = var.chart_versions.kube_prom
  namespace  = "monitoring"

  values = [yamlencode({
    crds = { enabled = true }
    grafana = {
      enabled       = true
      adminPassword = "admin" # rotated by ESO in workloads module
      sidecar = {
        dashboards = {
          enabled         = true
          searchNamespace = "ALL"
        }
      }
    }
    prometheus = {
      prometheusSpec = {
        retention                               = "15d"
        serviceMonitorSelectorNilUsesHelmValues = false
        podMonitorSelectorNilUsesHelmValues     = false
        ruleSelectorNilUsesHelmValues           = false
        storageSpec = {
          volumeClaimTemplate = {
            spec = {
              storageClassName = var.storage_class
              accessModes      = ["ReadWriteOnce"]
              resources        = { requests = { storage = "20Gi" } }
            }
          }
        }
      }
    }
    alertmanager = {
      alertmanagerSpec = {
        storage = {
          volumeClaimTemplate = {
            spec = {
              storageClassName = var.storage_class
              accessModes      = ["ReadWriteOnce"]
              resources        = { requests = { storage = "5Gi" } }
            }
          }
        }
      }
    }
  })]

  wait    = true
  timeout = 900

  depends_on = [kubernetes_namespace.platform]
}

# Loki + Promtail — log aggregation. Loki uses single-binary mode for
# simplicity; chunks live on a PVC. Production-scale users swap to
# distributed mode with MinIO chunk store.
resource "helm_release" "loki" {
  name       = "loki"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "loki"
  version    = var.chart_versions.loki
  namespace  = "logging"

  values = [yamlencode({
    deploymentMode = "SingleBinary"
    loki = {
      auth_enabled = false
      commonConfig = { replication_factor = 1 }
      schemaConfig = {
        configs = [{
          from         = "2024-01-01"
          store        = "tsdb"
          object_store = "filesystem"
          schema       = "v13"
          index        = { prefix = "loki_index_", period = "24h" }
        }]
      }
      storage = { type = "filesystem" }
    }
    singleBinary = {
      replicas = 1
      persistence = {
        enabled      = true
        size         = "20Gi"
        storageClass = var.storage_class
      }
    }
    # Loki's chart defaults the simple-scalable read/write/backend pools to
    # non-zero replicas. With deploymentMode=SingleBinary we have to zero
    # them explicitly or the chart's validate.yaml refuses to render.
    read    = { replicas = 0 }
    write   = { replicas = 0 }
    backend = { replicas = 0 }
    monitoring = {
      serviceMonitor = { enabled = true, namespace = "monitoring" }
    }
    chunksCache  = { enabled = false }
    resultsCache = { enabled = false }
  })]

  wait    = true
  timeout = 600

  depends_on = [helm_release.kube_prom]
}

resource "helm_release" "promtail" {
  name       = "promtail"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "promtail"
  version    = var.chart_versions.promtail
  namespace  = "logging"

  values = [yamlencode({
    config = {
      clients = [{
        url = "http://loki.logging.svc.cluster.local:3100/loki/api/v1/push"
      }]
    }
    serviceMonitor = { enabled = true, namespace = "monitoring" }
  })]

  wait    = true
  timeout = 600

  depends_on = [helm_release.loki]
}
