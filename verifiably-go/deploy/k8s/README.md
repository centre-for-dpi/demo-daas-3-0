# Kubernetes deployment — verifiably-go (walt.id slice)

Single-click Kubernetes deployment for the `waltid` scenario, mirroring `./deploy.sh up waltid && ./deploy.sh run waltid`. Cloud-agnostic: the same Terraform + Helm artifacts deploy to a local kind cluster, an on-prem k3s cluster, or self-managed nodes on AWS, with **zero use of cloud-managed services**.

See [`docs/k8s/workplan.md`](../../docs/k8s/workplan.md) for the full implementation plan.

## Layout

```
deploy/k8s/
  config/                       # walt.id .conf files (Phase 1.2 — done)
    issuer/  verifier/  wallet/

  terraform/
    bootstrap/
      local-kind/                 # kind cluster + MetalLB for laptops (Phase 3.1)
      onprem-k3s/                 # k3s on user-supplied nodes (Phase 3.3)
      aws-eks/                    # self-managed EKS, no managed addons (Phase 3.3)
    platform/                     # operators + cluster services, target-agnostic (Phase 3.2)
    workloads/                    # helm-installs the umbrella chart (Phase 3.4)
    environments/                 # *.tfvars per environment

  helm/
    charts/                       # one chart per service (Phase 4.1–4.5)
      walt-issuer/
      walt-verifier/
      walt-wallet/
      verifiably-go/
      keycloak/
      wso2is/
      libretranslate/
    umbrella/waltid/              # subchart aggregator (Phase 4.6)

  scripts/
    k8s-deploy.sh                 # the single-click entry point (Phase 5.1)
```

## Quick start (when Phase 5 lands)

```sh
# Default target — local kind cluster
./scripts/k8s-deploy.sh up waltid

# On-prem
./scripts/k8s-deploy.sh up waltid --target=onprem

# AWS EKS (self-managed)
./scripts/k8s-deploy.sh up waltid --target=aws

# Rebuild + roll the verifiably-go pod
./scripts/k8s-deploy.sh run waltid

# Tear down workloads (keep cluster)
./scripts/k8s-deploy.sh down waltid

# Tear everything down
./scripts/k8s-deploy.sh reset
```

## Cloud-agnostic guarantees

- All persistent storage uses `PersistentVolumeClaim` against the cluster's default StorageClass (any CSI driver works).
- Postgres for walt.id wallet runs in-cluster via the **CloudNativePG** operator. No RDS / CloudSQL.
- Object storage (used by inji-* services if/when added) runs in-cluster via the **MinIO** operator. No S3 / GCS.
- Secrets live in **HashiCorp Vault** (HA, Raft storage on PVCs) and are projected into namespaces by the **External Secrets Operator**. No KMS / Secrets Manager.
- TLS via **cert-manager** with a self-signed `ClusterIssuer` for on-prem and ACME for public hostnames.
- Ingress via **ingress-nginx** + **MetalLB** (on-prem) or the AWS LB controller in NLB mode (EKS) — all behind the same `Ingress` resource.
- Observability via **kube-prometheus-stack** + **Loki** + **Promtail**, all in-cluster.

## Phase status

See [`docs/k8s/workplan.md`](../../docs/k8s/workplan.md) for the running checklist.
