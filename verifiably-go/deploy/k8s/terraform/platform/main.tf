# Cloud-agnostic platform layer. Installs cluster services in dependency
# order across the per-concern *.tf files in this module:
#
#   namespaces.tf      — pre-create the namespaces every release lands in
#   ingress.tf         — ingress-nginx
#   certmanager.tf     — cert-manager + ClusterIssuers (selfsigned + ACME)
#   metallb.tf         — MetalLB (skipped on EKS or when bootstrap installed it)
#   databases.tf       — CloudNativePG + MinIO operators
#   secrets.tf         — External Secrets Operator + Vault HA
#   observability.tf   — kube-prometheus-stack + Loki + Promtail
#   argocd.tf          — Argo CD
#
# Provider versions are pinned in versions.tf. Inputs in variables.tf.
# Outputs in outputs.tf.
