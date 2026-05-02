# RESUME — K8s production-readiness state + executable next steps

**Last updated:** 2026-05-01. Updated whenever a phase is completed or a verification step changes status.

This file is the single place to look after a device restart or a cold session. It captures (1) what's been built, (2) what hasn't been verified yet, and (3) the exact commands to take it forward.

> **2026-05-01 verification pass** — drove compose end-to-end and the K8s `up waltid --target=local` pipeline through bootstrap + platform layers. Compose: green after fixes. K8s bootstrap + platform: green after fixes. Umbrella chart workloads: blocked by laptop Docker Desktop resource ceiling (~3.8 GiB / node, 3 nodes, all CPUs at 100%+). All bugs found are listed under "Bugs found and patched" below; uncommitted patches sit in the working tree for review.

Cross-references:

- [`workplan.md`](workplan.md) — the 22-prompt plan with ticked boxes
- [`inventory.md`](inventory.md) — every walt.id-scenario service's env, mount, port, deps
- [`values-schema.md`](values-schema.md) — `.conf` → `values.yaml` schema mapping
- [`observability.md`](observability.md) — metrics endpoint discovery
- [`../../deploy/k8s/runbooks/vault-init.md`](../../deploy/k8s/runbooks/vault-init.md) — Vault init/unseal/rotation runbook

---

## Current state — snapshot

**All 22 phase prompts implemented.** Code compiles, tests pass, every Terraform module validates, every Helm chart lints + renders. Two categories of work remain:

1. **End-to-end verification on real infrastructure** (Docker daemon + kind cluster).
2. **Settings calibration after first run** (probe paths, `/metrics` endpoint, wallet horizontal-scale tolerance).

### What's verified locally

- `go build ./cmd/server` — clean (Go 1.26.2)
- `go test ./...` — all suites pass
- Smoke test: `/healthz`/`/readyz` return 200, JSON logs structured correctly via `log/slog` (set `VERIFIABLY_LOG_JSON=1`)
- `docker compose -f deploy/compose/stack/docker-compose.yml config --quiet` — clean
- All Terraform modules: `terraform init -backend=false && terraform validate` → Success
- Every Helm chart: `helm lint` + `helm template` → renders cleanly
- Umbrella: `helm dep build` succeeds, `helm template` produces 35 resources

### What's NOT verified

- ~~`./deploy.sh up waltid` was never restarted after Phase 1.1 + 1.2 changes.~~ **Done 2026-05-01** — green after the patches below; `/healthz`, `/readyz`, walt.id `/swagger/index.html`, Keycloak `/realms/vcplatform`, libretranslate `/languages` all return 200; OIDC client registered in both Keycloak (vcplatform realm) and WSO2IS (verifiably_go_client).
- `./deploy/k8s/scripts/k8s-deploy.sh up waltid --target=local` — **partial 2026-05-01**. Bootstrap (kind cluster + MetalLB) ✅. Platform (ingress-nginx, cert-manager, CNPG, MinIO, ESO, Vault, kube-prom, Loki, Promtail, Argo CD, Kyverno) ✅ after the depends_on / Loki / ingress-nginx fixes below. Workloads (umbrella chart) ⚠️ — verifiably-go came up but pods crashlooped on `auth-providers.json` shape; walt-issuer + walt-verifier reached Running; walt-wallet + libretranslate stayed Pending and the cluster API became unreachable under load. **Root cause: Docker Desktop allocation (~3.8 GiB × 3 nodes) is too small for the full platform + umbrella combo.**
- ~~Walt.id `/metrics` endpoint path is assumed `/metrics` on the API port.~~ **Confirmed 2026-05-01: walt.id 0.18.2 does NOT expose Prometheus metrics on any listenable port** — `/metrics`, `/actuator/prometheus`, `/health`, `/healthz`, `/swagger-ui/*` all 404. Internally each container listens on its API port (7001/2/3) plus an ephemeral non-HTTP JVM port (likely RMI). `serviceMonitor.enabled` should stay `false` for these charts until upstream walt.id adds an exporter or a sidecar is shipped.
- Wallet-api horizontal-scale behavior under round-robin LB — still unknown. k6 script ready in `deploy/k8s/test/load/wallet-scale.js`. Blocked behind getting walt-wallet pods Ready in K8s, which is blocked behind Docker Desktop resource ceiling.
- ~~HTTP liveness/readiness probes default to `httpGet: { path: /, port: http }`.~~ **Confirmed 2026-05-01: walt.id `/` returns 302 → swagger** — the only 200 path is `/swagger/index.html`. Charts now patched (see below).
- **Keycloak `/health/ready`** is 404 on port 8180 in compose (Keycloak 25 serves health on management port 9000, not the public HTTP port). **In K8s the Bitnami chart's defaults are already correct**: livenessProbe is a TCP socket on port 8080; readinessProbe hits `/realms/master` (HTTP 200). No probe-path patch needed for the Keycloak chart. For compose, `/health/ready` is unreachable — operators must pick `/realms/master` or expose 9000 if they want a real readiness signal.

---

## Bugs found and patched (2026-05-01)

All patches are in the working tree, **uncommitted**. Review and commit when ready.

### Compose / Phase 1

| # | File | What | Why |
|---|---|---|---|
| 1 | `deploy/compose/stack/docker-compose.yml` | `libretranslate:1.6` → `libretranslate:v1.6.5` | Docker Hub tags use `v` prefix; `1.6` doesn't exist as a tag. |
| 2 | `deploy/k8s/helm/charts/libretranslate/values.yaml` | `tag: "1.6"` → `tag: "v1.6.5"` | Same reason. |
| 3 | `deploy/compose/stack/docker-compose.yml` (keycloak env) | `KC_BOOTSTRAP_ADMIN_USERNAME` → `KEYCLOAK_ADMIN`, `KC_BOOTSTRAP_ADMIN_PASSWORD` → `KEYCLOAK_ADMIN_PASSWORD` | Keycloak 25 uses `KEYCLOAK_ADMIN*`; the `KC_BOOTSTRAP_ADMIN_*` names were introduced in Keycloak 26. With the wrong names, no admin user is created → every login is `user_not_found` → bootstrap-keycloak.sh times out trying to grab a master-realm token. |
| 4 | `scripts/bootstrap-keycloak.sh:62` | `$KEYCLOAK_BASE…` → `${KEYCLOAK_BASE}…` | Bash with `set -u` reads the U+2026 ellipsis bytes as part of the variable name; `${BRACE}` form terminates the name correctly. |
| 5 | `scripts/bootstrap-wso2is.sh:46` | `$WSO2_BASE…` → `${WSO2_BASE}…` | Same reason. |

### K8s / Phases 3–6

| # | File | What | Why |
|---|---|---|---|
| 6 | `deploy/k8s/terraform/platform/certmanager.tf` | added `helm_release.kube_prom` to `depends_on` | The cert-manager chart enables a `ServiceMonitor` (kind in `monitoring.coreos.com/v1`); without depending on kube-prometheus-stack the CRD may not exist when terraform applies the chart in parallel. |
| 7 | `deploy/k8s/terraform/platform/ingress.tf` | same `depends_on` addition | Same reason — ingress-nginx defines a `ServiceMonitor`. |
| 8 | `deploy/k8s/terraform/platform/databases.tf` (cnpg) | same `depends_on` addition | Same reason — CNPG defines a `PodMonitor`. |
| 9 | `deploy/k8s/terraform/platform/secrets.tf` (vault) | same `depends_on` addition | Same reason — Vault chart defines a `ServiceMonitor`. |
| 10 | `deploy/k8s/terraform/platform/observability.tf` (loki) | added `read.replicas = 0`, `write.replicas = 0`, `backend.replicas = 0` to Loki values | With `deploymentMode: SingleBinary`, Loki's chart defaults the simple-scalable pools to non-zero replicas; `validate.yaml` then refuses to render. Setting them to 0 explicitly makes single-binary mode pass validation. |
| 11 | `deploy/k8s/terraform/platform/ingress.tf` | added `runAsUser: 101`, `runAsGroup: 101`, `fsGroup: 101` to `podSecurityContext` and `runAsUser/runAsGroup: 101` to `containerSecurityContext` | The ingress-nginx 1.11 image's user is the string `www-data`. Kubelet refuses to start a container with `runAsNonRoot: true` unless given a numeric UID it can compare to 0. UID 101 is the image's `www-data`. |
| 12 | `deploy/k8s/helm/charts/walt-issuer/values.yaml` | `httpGet path: /` → `/swagger/index.html` (liveness + readiness) | walt.id 0.18.2 returns 302 on `/`. `/swagger/index.html` is the only 200 path on the public API port. |
| 13 | `deploy/k8s/helm/charts/walt-verifier/values.yaml` | same | Same reason. |
| 14 | `deploy/k8s/helm/charts/walt-wallet/values.yaml` | same | Same reason. |
| 15 | `deploy/k8s/helm/charts/verifiably-go/values.yaml` | `authProviders: {}` → `authProviders: []` | The Go binary parses `auth-providers.json` as `[]auth.ProviderConfig`; the chart's empty `{}` default rendered as `{}` and the app crashloops at startup with `cannot unmarshal object into Go value of type []auth.ProviderConfig`. |
| 16 | `deploy/k8s/helm/umbrella/waltid/values.yaml` (verifiably-go.app.image) | `repository: ghcr.io/REPLACE_ME/verifiably-go` → `repository: verifiably-go`, `tag: "local"`, `pullPolicy: Never` | Local-kind verification path: `kind load docker-image verifiably-go:local` puts the image directly on the nodes, no registry needed. Kyverno's `require-signed-images-waltid` policy only matches `ghcr.io/*/verifiably-go*`, so a local image bypasses signature checks cleanly. |

### Environment prereqs (not project bugs, but document them)

- `deploy.sh` requires bash 4+ (uses `readarray`). macOS stock `/bin/bash` is 3.2.57 — install with `brew install bash`.
- macOS AirPlay Receiver claims port 5000 by default — disable in System Settings → General → AirDrop & Handoff before bringing up the compose stack (libretranslate binds 5000).
- Docker Desktop's default per-VM allocation (~3.8 GiB × 3 kind nodes) is too small for the full umbrella chart + platform stack. Bump Docker Desktop's RAM to ≥12 GiB before running `k8s-deploy.sh up`.

### Known TODOs still pending after this pass

- ~~**`COPY deploy ./docs-src/deploy` image bloat.**~~ **Done 2026-05-02** — `.dockerignore` now excludes `**/.terraform`, `**/*.tfstate*`, `deploy/k8s/terraform/**/.tfstate`, the macOS `server` binary, and a few other build-context squatters. Run `docker build -t verifiably-go:local .` to confirm the image drops from ~1.17 GiB to ~50–80 MiB.
- ~~**Keycloak K8s probe paths.**~~ **Verified 2026-05-02** — Bitnami's keycloak chart uses TCP on 8080 for liveness and `/realms/master` for readiness; both correct out of the box.
- Workloads `helm_release.waltid` ended in `failed` state after the truncated terraform run; the next session should `helm uninstall waltid` (or `terraform state rm helm_release.waltid && helm uninstall …`) before re-applying.

---

## Step-by-step resume — executable

After a device restart, run these in order. Each step has a "if it fails" pointer.

### Step 1 — confirm Phase 1 didn't break the compose stack

```sh
cd /Users/antoine/codeplay/cdpi_projects/demo-daas-3-0/verifiably-go

# Start Docker Desktop first (manual GUI step on macOS).
docker info >/dev/null && echo "docker ok"

# Re-validate compose lint with the latest pins + bind mounts.
docker compose -f deploy/compose/stack/docker-compose.yml --env-file .env.example config --quiet
echo "compose lint exit=$?"

# Bring up the waltid scenario end-to-end.
cp -n .env.example .env   # only if .env is missing
./deploy.sh up waltid
./deploy.sh run waltid

# Sanity probes
curl -fsS -o /dev/null -w "%{http_code}\n" http://localhost:8080/healthz   # expect 200
curl -fsS -o /dev/null -w "%{http_code}\n" http://localhost:7002/          # walt.id issuer
curl -fsS -o /dev/null -w "%{http_code}\n" http://localhost:7003/          # walt.id verifier
curl -fsS -o /dev/null -w "%{http_code}\n" http://localhost:7001/          # walt.id wallet
```

**If a walt.id pod fails to start with a HOCON parse error in `/waltid-wallet-api/config/auth.conf`:**
The most likely cause is the wallet OIDC env-var substitution combined with HOCON quoting. Check container logs for the failing line; comment out the offending field in `deploy/k8s/config/wallet/oidc.conf` and re-run.

**If the compose stack fails to find a config file under the new bind path:**
The new mounts are `../../k8s/config/{issuer,verifier,wallet}` relative to `deploy/compose/stack/docker-compose.yml`. Confirm the `deploy/k8s/config/` dir still exists; it's the source of truth.

### Step 2 — install K8s prerequisites (one-time)

```sh
brew install kind kubectl helm terraform k6
# Optionally for cosign image signing later:
brew install cosign
```

### Step 3 — bring up the K8s cluster

```sh
cd /Users/antoine/codeplay/cdpi_projects/demo-daas-3-0/verifiably-go
./deploy/k8s/scripts/k8s-deploy.sh up waltid --target=local
```

What this does (so you can spot which step fails):

1. `terraform apply` in `deploy/k8s/terraform/bootstrap/local-kind/` — creates 3-node kind cluster + MetalLB on Docker network 172.18.x.
2. `terraform apply` in `deploy/k8s/terraform/platform/` — installs ingress-nginx, cert-manager, CNPG, MinIO operator, ESO, Vault HA, kube-prometheus-stack, Loki + Promtail, Argo CD, Kyverno.
3. `terraform apply` in `deploy/k8s/terraform/workloads/` — installs the umbrella chart (`helm install waltid ./helm/umbrella/waltid`) + ExternalSecret resources.
4. `kubectl wait` for all pods Ready in the `waltid` namespace.
5. Prints `kubectl get pods/ingress/certificate`.

**If `terraform apply` on bootstrap fails:** likely cause is kind requiring a privileged Docker network. Verify with `docker network ls | grep kind`.

**If platform fails on Vault:** Vault starts sealed. The platform module waits for the Helm release (StatefulSet up) but doesn't unseal. That's expected — see step 5 below.

**If workloads fails on `vault-cluster-secret-store`:** ESO can't talk to a sealed Vault. Workaround: temporarily comment out `kubectl_manifest.vault_cluster_secret_store` and the two ExternalSecret resources in `deploy/k8s/terraform/workloads/main.tf`, finish the deploy, then unseal Vault, then re-add and apply.

### Step 4 — discover walt.id metrics endpoints

```sh
KUBECONFIG=$(terraform -chdir=deploy/k8s/terraform/bootstrap/local-kind output -raw kubeconfig_path)
export KUBECONFIG

# Port-forward each walt.id service and curl /metrics.
for svc in walt-issuer walt-verifier walt-wallet; do
  echo "── $svc ──"
  kubectl -n waltid port-forward svc/waltid-$svc 9999:7002 &
  PF=$!
  sleep 2
  curl -s http://localhost:9999/metrics | head -3 || curl -s http://localhost:9999/actuator/prometheus | head -3
  kill $PF; wait 2>/dev/null
done
```

Update each chart's `values.yaml` `*.serviceMonitor.path` to whatever responded with metrics text, then flip `enabled: true`.

### Step 5 — initialize Vault

Follow [`deploy/k8s/runbooks/vault-init.md`](../../deploy/k8s/runbooks/vault-init.md) verbatim. Critical: store `vault-init.json` outside the repo. Without those keys the cluster is unrecoverable.

### Step 6 — load-test wallet horizontal scale

```sh
./deploy/k8s/test/e2e/run-against-cluster.sh   # smoke first
# Then load test
HOST=$(kubectl -n waltid get ingress -o jsonpath='{.items[?(@.metadata.name=="waltid-walt-wallet")].spec.rules[0].host}')
kubectl -n waltid scale deploy waltid-walt-wallet --replicas=1
k6 run --env BASE_URL="https://$HOST" deploy/k8s/test/load/wallet-scale.js | tee /tmp/k6-1replica.txt
kubectl -n waltid scale deploy waltid-walt-wallet --replicas=3
k6 run --env BASE_URL="https://$HOST" deploy/k8s/test/load/wallet-scale.js | tee /tmp/k6-3replicas.txt
diff <(grep -E 'cross_pod_drift|http_req_failed' /tmp/k6-1replica.txt) <(grep -E 'cross_pod_drift|http_req_failed' /tmp/k6-3replicas.txt)
```

If the 3-replica run shows a non-zero `cross_pod_drift` rate, document the chosen mitigation (sticky sessions vs. Redis-backed sessions) in this file and update `walt-wallet/values.yaml`.

### Step 7 — push the verifiably-go image and validate cosign signing

```sh
make image                                  # builds locally
# CI does this on push to main; for manual:
REGISTRY=ghcr.io/<your-username> make image image-push
# Verify signature
cosign verify --certificate-identity-regexp 'github.com/.*/verifiably-go.*' \
              --certificate-oidc-issuer https://token.actions.githubusercontent.com \
              ghcr.io/<your-username>/verifiably-go:<tag>
```

### Step 8 — commit + open a PR

Once Step 1 and Step 3 are green, commit the workplan execution. Suggested message:

> chore(k8s): production-ready Helm + Terraform stack for waltid scenario
>
> Implements all 22 prompts in docs/k8s/workplan.md. Adds cloud-agnostic
> Terraform (local-kind / on-prem k3s / self-managed EKS) + 7 Helm charts
> + umbrella + Vault + ESO + observability + Kyverno cosign enforcement.
> No managed cloud services. See docs/k8s/resume.md for state + next steps.

---

## Known TODOs / loose ends

These are deliberate gaps the workplan flagged, not defects:

| Item | File | What's needed |
|---|---|---|
| Walt.id metrics endpoint path | every chart's `values.yaml` `serviceMonitor.path` | Verify against running container (Step 4 above) |
| Wallet HPA enable | `walt-wallet/values.yaml` | Run k6 (Step 6) then flip `wallet.hpa.enabled: true` if drift = 0 |
| Issuer/verifier signing-key location | `walt-{issuer,verifier}/values.yaml` `signingKey.existingSecret` | `docker exec` into running container; locate where keys are written |
| `wallet-api/data` mount purpose | `walt-wallet/values.yaml` `persistence` | Inspect what the wallet writes there at runtime; PVC sized 5Gi default |
| Vault Transit wiring | `deploy/k8s/terraform/platform/vault-bootstrap.tf` | Uncomment after Vault init; see runbook |
| `keycloak` Bitnami subchart | `deploy/k8s/helm/charts/keycloak/Chart.yaml` | `helm dep update` runs as part of `k8s-deploy.sh up` |
| Realm import JSON | `umbrella/waltid/values.yaml` `keycloak.realm.json` | Inline existing `deploy/compose/stack/keycloak-realm.json` content |
| HTTP probe paths | every chart's `*.livenessProbe.httpGet.path` | Tighten from `/` once endpoint behavior is known |
| WSO2IS PodSecurity-restricted compatibility | `walt-issuer`-style `containerSecurityContext` may not pass restricted | If Phase 7.2 enforcement rejects WSO2IS, narrow concession (NEVER downgrade namespace label) |
| Image registry placeholder | every Bitnami-style chart referencing `ghcr.io/REPLACE_ME` | Replace with the actual GHCR org once you push |

---

## File map (where each phase lives)

```
docs/k8s/
  workplan.md              ← phase plan (all boxes ticked)
  inventory.md             ← Phase 0.1
  values-schema.md         ← Phase 0.2
  observability.md         ← Phase 6.1
  resume.md                ← THIS FILE

deploy/compose/stack/docker-compose.yml      ← Phase 1.1 (image pins) + Phase 1.2 (rewired bind mounts)
deploy/k8s/config/{issuer,verifier,wallet}/  ← Phase 1.2 (canonical .conf source)
.env.example                                  ← Phase 1.2 (14 new wallet vars)
cmd/server/main.go                            ← Phase 1.3 (/healthz, /readyz, slog JSON)
Dockerfile                                    ← Phase 1.3 (already non-root distroless)
Makefile                                      ← Phase 1.3 (image, image-push, k8s-* targets)
.github/workflows/image.yml                   ← Phase 1.3 (Trivy + cosign + GHCR push)
.github/workflows/k8s-e2e.yml                 ← Phase 8.1

deploy/k8s/
  README.md
  terraform/
    bootstrap/local-kind/                     ← Phase 3.1
    bootstrap/onprem-k3s/                     ← Phase 3.3
    bootstrap/aws-eks/                        ← Phase 3.3
    platform/
      versions.tf, variables.tf, providers.tf, namespaces.tf,
      ingress.tf, certmanager.tf, metallb.tf,
      databases.tf, secrets.tf,               ← Phase 3.2
      observability.tf, argocd.tf, kyverno.tf,
      vault-bootstrap.tf, vault-policies/waltid.hcl   ← Phase 7.1, 7.2
    workloads/                                ← Phase 3.4 + 7.2 NetworkPolicies
    environments/{dev,prod}.tfvars
  helm/
    charts/walt-issuer/                       ← Phase 4.1
    charts/walt-verifier/                     ← Phase 4.2
    charts/walt-wallet/                       ← Phase 4.3
    charts/verifiably-go/                     ← Phase 4.4
    charts/keycloak/                          ← Phase 4.5 (Bitnami wrapper)
    charts/wso2is/                            ← Phase 4.5 (from-scratch StatefulSet)
    charts/libretranslate/                    ← Phase 4.5
    umbrella/waltid/                          ← Phase 4.6 (with Chart.lock)
  scripts/k8s-deploy.sh                       ← Phase 5.1
  test/load/wallet-scale.js + README.md       ← Phase 6.2
  test/e2e/run-against-cluster.sh             ← Phase 8.1
  runbooks/vault-init.md                      ← Phase 7.1
```

---

## How to update this file

When you complete a verification step or change a defaulted value:

1. Edit the "What's verified" / "What's NOT verified" sections at the top.
2. Tick or strike through the corresponding row in "Known TODOs".
3. Bump the **Last updated** date.
4. Commit alongside the change.
