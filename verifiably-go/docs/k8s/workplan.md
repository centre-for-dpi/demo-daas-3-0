# K8s production-readiness workplan — walt.id slice

Goal: take the `waltid` scenario from `./deploy.sh up waltid && ./deploy.sh run waltid` to a single-click Kubernetes deployment with security hardening, monitoring, and autoscaling. Cloud-agnostic: the same Terraform + Helm artifacts deploy to a local kind cluster, an on-prem k3s cluster, or self-managed nodes on AWS, with **zero use of cloud-managed services**.

Scope (from `deploy.sh:87-92` and the `waltid` case at `deploy.sh:155-161`):

- walt.id services: `postgres`, `caddy`, `issuer-api`, `verifier-api`, `wallet-api`
- IdP: `keycloak`, `wso2is`
- Translation: `libretranslate`
- Application: `verifiably-go` (the Go app from `cmd/`)

Single-click target: `./k8s-deploy.sh up waltid [--target=local|onprem|aws]` and `./k8s-deploy.sh run waltid`.

---

## Architectural ground rules (apply to every prompt below)

- **Cloud agnostic** = workloads only depend on in-cluster primitives:
  - PVCs (any CSI driver)
  - `Service` types `ClusterIP` / `LoadBalancer` (provided by MetalLB on-prem, AWS LB controller on EKS)
  - `Ingress` via ingress-nginx
  - Secrets via Vault + External Secrets Operator
  - Postgres via the CloudNativePG operator (no RDS)
  - Object storage via the MinIO operator
  - Certs via cert-manager (self-signed CA on-prem, ACME elsewhere)
- **Terraform's job** = drive Kubernetes (`kubernetes`, `helm`, `kubectl` providers) against any kubeconfig. Cluster creation lives in *thin, optional* per-target modules (`bootstrap/local-kind`, `bootstrap/onprem-k3s`, `bootstrap/aws-eks-self-managed`) selected by a single var. The core platform module is identical for all three.
- **Helm's job** = walt.id + companion workloads, one chart per service plus an umbrella, values-driven.
- **Single-click** = `./k8s-deploy.sh up waltid` and `./k8s-deploy.sh run waltid`, mirroring the existing CLI surface.

---

## Phase 0 — Audit (no code yet, two prompts)

### Prompt 0.1 — Walt.id config surface inventory

> In `verifiably-go/deploy/compose/stack/docker-compose.yml`, audit only the `waltid` scenario services: `postgres`, `caddy`, `issuer-api`, `verifier-api`, `wallet-api`, `keycloak`, `wso2is`, `libretranslate`. For each, produce a table of: every env var consumed (with which are secret), every bind-mounted file/dir and what it contains, every port, every dependency on another service, and every piece of persistent state. Also list which env vars are referenced in `.env.example`. Write the result to `docs/k8s/inventory.md`. Do not write any chart code. Under 400 lines.

### Prompt 0.2 — Walt.id config-file deep-read

> Read every config file mounted into `issuer-api`, `verifier-api`, and `wallet-api` (look under `deploy/compose/stack/`). For each file, summarize: schema, which fields are environment-specific (URLs, DB DSN, signing keys, OIDC client secrets), and which are static. The output is the basis for the `values.yaml` schema of each Helm chart. Write to `docs/k8s/values-schema.md`.

---

## Phase 1 — Pre-K8s code refactors (three prompts)

### Prompt 1.1 — Pin all base images

> In `verifiably-go/deploy/compose/stack/docker-compose.yml`, replace every `:latest` tag with a pinned minor version (`postgres:16.4`, `caddy:2.8`, `quay.io/keycloak/keycloak:25.0`, `libretranslate/libretranslate:1.6`, etc.). Do not touch walt.id services — they're already at `0.18.2`. Verify `./deploy.sh up waltid` still works. Single commit.

### Prompt 1.2 — Externalize walt.id config from bind mounts to templated files

> Walt.id services currently consume YAML configs via bind mounts. Move those config files into `verifiably-go/deploy/k8s/config/{issuer,verifier,wallet}/` as standalone files with placeholder env-var references (e.g. `{{ .Values.db.dsn }}`). The compose stack should keep working by rendering them with envsubst during `./deploy.sh up waltid` (mirror the existing `deploy/compose/injiweb/render-config.sh` pattern). This makes the same files reusable as Helm ConfigMap sources.

### Prompt 1.3 — Build & publish the verifiably-go image

> Add a multi-stage `Dockerfile` build target for the Go app suitable for K8s (non-root user, distroless or alpine, `/health` endpoint, structured JSON logs to stdout). Add a `make image` target that builds and tags `verifiably-go:<git-sha>`. Add a CI workflow (`.github/workflows/image.yml`) that on push to `main` builds, runs Trivy, and pushes to a configurable registry (`REGISTRY` env var, default `ghcr.io/${GITHUB_REPOSITORY_OWNER}`).

---

## Phase 2 — Repo scaffolding (one prompt)

### Prompt 2.1 — Create K8s deployment skeleton

> Create `verifiably-go/deploy/k8s/` with this layout, all empty stubs with TODO comments:
>
> ```
> deploy/k8s/
>   terraform/
>     bootstrap/local-kind/   # optional cluster creation
>     bootstrap/onprem-k3s/
>     bootstrap/aws-eks/
>     platform/               # operators + cluster services (always applied)
>     workloads/              # thin wrapper that helm-installs the umbrella chart
>     environments/dev.tfvars
>     environments/prod.tfvars
>   helm/
>     charts/
>       walt-issuer/
>       walt-verifier/
>       walt-wallet/
>       verifiably-go/
>       keycloak/             # wrapper around bitnami/keycloak with our values
>       wso2is/
>       libretranslate/
>     umbrella/waltid/        # depends on the above subcharts
>   scripts/
>     k8s-deploy.sh           # the single-click entry point
>   README.md
> ```
>
> Also add a top-level `Makefile` with `make k8s-up`, `make k8s-down`, `make k8s-status` shelling into `scripts/k8s-deploy.sh`. Do not write any real chart/TF content yet.

---

## Phase 3 — Cloud-agnostic platform (Terraform, four prompts)

### Prompt 3.1 — Local-kind bootstrap module

> Implement `deploy/k8s/terraform/bootstrap/local-kind/`. It should: create a kind cluster with 3 nodes via the `tehcyx/kind` provider (or a `null_resource` calling `kind`), expose ports 80/443 via extraPortMappings, install MetalLB with a Docker-network IP pool, output a kubeconfig path. This is the dev-machine target — it must come up with `terraform apply` and nothing else.

### Prompt 3.2 — Platform module (cloud-agnostic, the heart of it)

> Implement `deploy/k8s/terraform/platform/`. Inputs: `kubeconfig_path`, `cluster_issuer_email`, `domain`. Using the `helm` and `kubernetes` providers, install in this order with explicit `depends_on`:
>
> - ingress-nginx
> - cert-manager (with a `ClusterIssuer` for self-signed CA + one for Let's Encrypt, user picks via var)
> - MetalLB CRDs (skip if running on EKS — gate on var `lb_mode = "metallb" | "cloud"`)
> - CloudNativePG operator
> - MinIO operator
> - External Secrets Operator
> - HashiCorp Vault in HA mode (3 replicas, Raft storage on PVCs)
> - kube-prometheus-stack
> - Loki + Promtail
> - Argo CD
>
> Every chart version pinned. No cloud-specific resources anywhere in this module. Outputs: ingress LB hostname, Vault address, Argo CD admin password secret name.

### Prompt 3.3 — On-prem k3s and AWS EKS bootstrap modules

> Implement `bootstrap/onprem-k3s/` (assumes user provides node IPs via var; shells out to `k3sup` to install) and `bootstrap/aws-eks/` (uses the official `terraform-aws-modules/eks/aws` module but with **only self-managed node groups, no Fargate, no managed addons** — keep it portable). Both must produce the same output shape as `bootstrap/local-kind/`: `kubeconfig_path`, `lb_mode`. The platform module from 3.2 must apply unmodified to the output of all three.

### Prompt 3.4 — Workloads module wrapping the umbrella chart

> Implement `deploy/k8s/terraform/workloads/`. It takes `kubeconfig_path` + a `values.yaml` path, installs the `umbrella/waltid` chart from `deploy/k8s/helm/umbrella/waltid` into namespace `waltid`, with `wait = true` and a 15-min timeout. Also creates the `ExternalSecret` resources that pull walt.id DB creds + signing keys from Vault. No business logic here — just glue.

---

## Phase 4 — Walt.id Helm charts (six prompts, can parallelize)

### Prompt 4.1 — `walt-issuer` chart

> Create `deploy/k8s/helm/charts/walt-issuer/`. Image `waltid/issuer-api:0.18.2`. Mounts ConfigMap built from `deploy/k8s/config/issuer/` (per Prompt 1.2). DB DSN, signing keys come from a `Secret` referenced via `existingSecret` in values. Liveness `/livez`, readiness `/readyz` (verify actual endpoints in Phase 0 audit). Resources, HPA (default off), PodDisruptionBudget, NetworkPolicy (deny-all + allow ingress from ingress-nginx and verifiably-go namespaces), securityContext (non-root, readOnlyRootFilesystem, drop ALL caps), ServiceAccount with no token automount. `values.yaml` schema must match `docs/k8s/values-schema.md` from Prompt 0.2. Add `helm lint` + `helm template` smoke test in CI.

### Prompt 4.2 — `walt-verifier` chart

> Same shape as 4.1 but for `waltid/verifier-api:0.18.2`. The verifier is stateless — HPA defaults `minReplicas: 2`.

### Prompt 4.3 — `walt-wallet` chart

> Same shape as 4.1 but for `waltid/wallet-api:0.18.2`. This is the stateful one: it talks to Postgres and holds key material. Default `replicas: 1` and `HPA: disabled` until Phase 6 verifies horizontal-scale behaviour. Wire wallet DB connection to a `CloudNativePG` `Cluster` resource referenced by name from values.

### Prompt 4.4 — `verifiably-go` chart

> Chart for the Go app. Image from Prompt 1.3. The `backends.json` that `deploy.sh run waltid` generates becomes a templated ConfigMap. The chart must accept `backends` map in values and produce the same JSON shape (read `deploy.sh:820-870` for the format).

### Prompt 4.5 — Wrapper charts for keycloak, wso2is, libretranslate

> For each, write a thin chart that either wraps the upstream community chart (Bitnami Keycloak 25.x) or implements directly if no upstream chart exists (WSO2IS — write from scratch, mirror the compose env vars and volumes). Pin all versions. Walt.id integration depends on Keycloak realm import — handle via a `pre-install` Job that imports `keycloak-realm.json` from the existing compose dir.

### Prompt 4.6 — `umbrella/waltid` chart

> Umbrella chart with subchart dependencies on all six charts from 4.1–4.5 plus a `cnpg-cluster` template for the shared walt.id Postgres. Also includes `Ingress` resources for the public hostnames (`issuer.<domain>`, `verifier.<domain>`, `wallet.<domain>`, `app.<domain>`). One `values.yaml` with sane defaults; one `values-prod.yaml` example with HPA + PDB + multi-replica. `helm install waltid ./umbrella/waltid -n waltid --create-namespace` must work end-to-end.

---

## Phase 5 — Single-click orchestrator (one prompt)

### Prompt 5.1 — Implement `k8s-deploy.sh` mirroring `deploy.sh`

> Implement `deploy/k8s/scripts/k8s-deploy.sh` with the same UX as the existing `verifiably-go/deploy.sh`:
>
> - `./k8s-deploy.sh up waltid [--target=local|onprem|aws]` runs `terraform apply` on the chosen `bootstrap/*` module, then `terraform apply` on `platform/`, then `terraform apply` on `workloads/`. Waits for all pods Ready. Prints URLs + credentials.
> - `./k8s-deploy.sh run waltid` re-builds + pushes the verifiably-go image (calls `make image` from Prompt 1.3) and rolls the deployment via `kubectl rollout restart`.
> - `./k8s-deploy.sh down [waltid]` reverses workloads (keeps platform + cluster).
> - `./k8s-deploy.sh reset` destroys everything including the cluster.
> - `./k8s-deploy.sh status` prints pod / ingress / cert state.
> - `./k8s-deploy.sh logs <service>` tails logs.
>
> Re-use the colour helpers and option parsing style from `deploy.sh` so the two feel sibling. Default target: `local`. Document in `deploy/k8s/README.md`.

---

## Phase 6 — Observability + autoscaling validation (two prompts)

### Prompt 6.1 — Wire walt.id metrics

> Verify which Prometheus endpoints walt.id exposes (test against a running container — likely `/actuator/prometheus` or Micrometer on a sidecar port). Add `ServiceMonitor` resources to each chart from Phase 4. Provide a Grafana dashboard JSON in `deploy/k8s/helm/charts/walt-*/dashboards/` and load it via the `kube-prometheus-stack` sidecar pattern.

### Prompt 6.2 — Load-test wallet-api horizontal scale

> Write a k6 script under `deploy/k8s/test/load/wallet-scale.js` that exercises wallet creation + credential storage. Run it against a 1-replica wallet-api, then a 3-replica deployment, and report whether sessions/keys break under round-robin load balancing. If they do: document needed sticky-session config or a session-store dependency. Report only — do not change chart defaults until results are in.

---

## Phase 7 — Security hardening (two prompts)

### Prompt 7.1 — Vault Transit for signing keys + secret rotation

> Replace the file-mounted signing keys in walt-issuer + walt-wallet with Vault Transit references. Issuer + wallet talk to Vault via the agent injector sidecar pattern. Add a Vault policy file under `deploy/k8s/terraform/platform/vault-policies/` and a Job that bootstraps Transit keys on first install. Document the rotation runbook.

### Prompt 7.2 — Pod Security + NetworkPolicy + image signing

> Apply `PodSecurity: restricted` label to the `waltid` namespace. Audit every chart from Phase 4: any pod that fails restricted gets fixed (likely WSO2IS will need work). Add a default-deny `NetworkPolicy` to the namespace and per-service allow rules. Sign all custom images (verifiably-go) with cosign in CI; add a Kyverno policy that rejects unsigned images in the `waltid` namespace.

---

## Phase 8 — End-to-end verification (one prompt)

### Prompt 8.1 — Port the existing e2e suite to K8s

> The repo has an `e2e/` suite that runs against the compose stack. Port it (or wrap it) so it runs against a fresh `./k8s-deploy.sh up waltid --target=local` cluster. Add a CI job that does the full cycle — bring up kind + platform + workloads, run e2e, tear down — on every PR touching `deploy/k8s/`. Wallclock budget: 20 min.

---

## Execution notes

- Run phases in order. Within a phase, prompts marked "can parallelize" are independent.
- After each prompt, verify the deliverable exists and the smoke test in that prompt passes before moving on.
- Each prompt is self-contained — copy into a fresh agent session with no context.
- Track progress by checking off the boxes below (or via the conversation's task list).

### Checklist

- [x] 0.1 Walt.id config surface inventory → `docs/k8s/inventory.md`
- [x] 0.2 Walt.id config-file deep-read → `docs/k8s/values-schema.md`
- [x] 1.1 Pin all base images
- [x] 1.2 Externalize walt.id config to templated files (+ 4 security fixes)
- [x] 1.3 Build & publish verifiably-go image (`/healthz`, `/readyz`, JSON logs, CI)
- [x] 2.1 Create K8s deployment skeleton
- [x] 3.1 Local-kind bootstrap module
- [x] 3.2 Platform module
- [x] 3.3 On-prem k3s + AWS EKS bootstrap modules
- [x] 3.4 Workloads module
- [x] 4.1 walt-issuer chart
- [x] 4.2 walt-verifier chart
- [x] 4.3 walt-wallet chart
- [x] 4.4 verifiably-go chart
- [x] 4.5 keycloak / wso2is / libretranslate wrapper charts
- [x] 4.6 umbrella/waltid chart
- [x] 5.1 k8s-deploy.sh single-click orchestrator
- [x] 6.1 Wire walt.id metrics (ServiceMonitor + dashboards + endpoint discovery doc)
- [x] 6.2 Load-test wallet-api horizontal scale (k6 script — run pending cluster)
- [x] 7.1 Vault Transit + secret rotation (policy, bootstrap stub, runbook)
- [x] 7.2 Pod Security restricted + NetworkPolicy default-deny + Kyverno cosign
- [x] 8.1 Port e2e suite to K8s (`run-against-cluster.sh` + CI workflow)
