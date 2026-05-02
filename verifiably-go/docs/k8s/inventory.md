# Walt.id scenario — config surface inventory

Source: `verifiably-go/deploy/compose/stack/docker-compose.yml` and `.env.example`. Scope: services in the `waltid` scenario only — `postgres`, `caddy`, `issuer-api`, `verifier-api`, `wallet-api`, `keycloak`, `wso2is`, `libretranslate`. Inji-* services are **out of scope**; they are listed here only when a walt.id-scenario service has a runtime dependency on them (none currently do).

This file is the input to Phase 0.2 (`values-schema.md`) and Phase 4.1–4.5 chart authoring.

Legend for env-var **secret** column: `Y` = sensitive (DB creds, signing keys, OIDC secrets, admin passwords), `N` = non-sensitive (URLs, ports, hostnames, feature flags). Compose-time defaults are noted in parentheses.

---

## postgres

Image: `postgres:latest` (Phase 1.1: pin to `postgres:16.x`).
Role: shared Postgres for `wallet-api`. Issuer + verifier do **not** use it.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `POSTGRES_DB` | `.env:POSTGRES_DB` (`waltid`) | N | DB name |
| `POSTGRES_USER` | `.env:POSTGRES_USER` (`waltid`) | N | DB user |
| `POSTGRES_PASSWORD` | `.env:POSTGRES_PASSWORD` (`waltid`) | **Y** | weak demo default — must rotate in prod |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `wallet-db:/var/lib/postgresql` | named volume | DB data dir. **Note**: mounted at `/var/lib/postgresql`, not `…/data` — non-default but matches official image's PGDATA expectations as long as no init scripts collide. |

### Ports

| Container | Host | `.env` var | Purpose |
|---|---|---|---|
| 5432 | `${POSTGRES_PORT:-5432}` | `POSTGRES_PORT` | dev access from host |

### Dependencies

None (root of dependency graph).

### Persistent state

`wallet-db` — wallet-api's encrypted credential storage. **Critical**: walt.id wallet keys live here (in encrypted form). Loss = all wallet users locked out.

### Healthcheck

`pg_isready -q -U ${POSTGRES_USER:-waltid}` every 5s.

### K8s mapping

Replace with **CloudNativePG** `Cluster` resource (Phase 4.6). Backups via `Cluster.spec.backup` to MinIO (Phase 3.2 platform module).

---

## caddy

Image: `caddy:2` (Phase 1.1: pin to `caddy:2.8`).
Role: reverse proxy for the three walt.id APIs. **In K8s, replaced entirely by ingress-nginx + cert-manager.**

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `WALLET_BACKEND_PORT` | `.env:WALTID_WALLET_PORT` (`7001`) | N | upstream port for `/wallet-api` proxy |
| `ISSUER_API_PORT` | `.env:WALTID_ISSUER_PORT` (`7002`) | N | upstream port for `/issuer-api` proxy |
| `VERIFIER_API_PORT` | `.env:WALTID_VERIFIER_PORT` (`7003`) | N | upstream port for `/verifier-api` proxy |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `./Caddyfile:/etc/caddy/Caddyfile` | bind | reverse-proxy rules — see file below |

### Caddyfile contents

```
:{$WALLET_BACKEND_PORT:7001}  → reverse_proxy http://wallet-api:{$WALLET_BACKEND_PORT:7001}
:{$ISSUER_API_PORT:7002}      → reverse_proxy http://issuer-api:{$ISSUER_API_PORT:7002}
:{$VERIFIER_API_PORT:7003}    → reverse_proxy http://verifier-api:{$VERIFIER_API_PORT:7003}
```

`auto_https off`, `admin off` — no TLS, no admin API. Demo-only.

### Ports

| Container | Host | Purpose |
|---|---|---|
| 7001 | 7001 | wallet-api proxy |
| 7002 | 7002 | issuer-api proxy |
| 7003 | 7003 | verifier-api proxy |

### Dependencies

None declared (depended-on by issuer-api / verifier-api / wallet-api at the compose level).

### K8s mapping

- Drop the Caddy container.
- Map each rule to an `Ingress` resource on `ingress-nginx` with hostnames `wallet.<domain>`, `issuer.<domain>`, `verifier.<domain>`.
- TLS via `cert-manager` `ClusterIssuer` (self-signed CA on-prem, ACME elsewhere).

---

## issuer-api

Image: `waltid/issuer-api:0.18.2` (already pinned).
Role: walt.id OID4VCI issuer.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `SERVICE_HOST` | hard-coded `localhost` | N | advertised host in well-known metadata. **Demo-only — must become public hostname in K8s.** |
| `ISSUER_API_PORT` | `.env:WALTID_ISSUER_PORT` (`7002`) | N | listen port |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `./issuer-api/config:/waltid-issuer-api/config` | bind | walt.id config dir — see files below |

### Config files (under `deploy/compose/stack/issuer-api/config/`)

| File | Lines | Purpose | Env-specific? |
|---|---|---|---|
| `issuer-service.conf` | 1 | issuer service settings | TBD in Phase 0.2 |
| `web.conf` | 2 | HTTP listener config | TBD in Phase 0.2 |

### Ports

Container `${WALTID_ISSUER_PORT:-7002}` — not published on the host directly (Caddy fronts it).

### Dependencies

`depends_on: [caddy]`. Caddy reverse-proxies it; the dependency is so Caddy is up first.

### Persistent state

None observed in the compose mount (config is read-only and signing keys are inside config files — to be confirmed in Phase 0.2). Likely **stateless**; horizontal-scale-friendly.

### K8s mapping

- `Deployment` (stateless), 2+ replicas in prod.
- `Service` ClusterIP on the same port.
- ConfigMap built from `issuer-api/config/`.
- `Ingress` rule for the public hostname.
- HPA on CPU.
- Signing keys → Secret + Vault Transit (Phase 7.1).

---

## verifier-api

Image: `waltid/verifier-api:0.18.2` (already pinned).
Role: walt.id OID4VP verifier.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `SERVICE_HOST` | hard-coded `localhost` | N | same caveat as issuer-api |
| `VERIFIER_API_PORT` | `.env:WALTID_VERIFIER_PORT` (`7003`) | N | listen port |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `./verifier-api/config:/waltid-verifier-api/config` | bind | walt.id config dir |

### Config files (under `deploy/compose/stack/verifier-api/config/`)

| File | Lines | Purpose | Env-specific? |
|---|---|---|---|
| `verifier-service.conf` | 1 | verifier service settings | TBD in Phase 0.2 |
| `web.conf` | 2 | HTTP listener config | TBD in Phase 0.2 |

### Ports

Container `${WALTID_VERIFIER_PORT:-7003}` — not published directly.

### Dependencies

`depends_on: [caddy]`.

### Persistent state

None observed. Stateless.

### K8s mapping

Same shape as issuer-api. Default `minReplicas: 2` (Phase 4.2).

---

## wallet-api

Image: `waltid/wallet-api:0.18.2` (already pinned).
Role: walt.id wallet — stateful, holds user wallet records and key material.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `SERVICE_HOST` | hard-coded `localhost` | N | advertised host |
| `WALLET_BACKEND_PORT` | `.env:WALTID_WALLET_PORT` (`7001`) | N | listen port |
| `DB_NAME` | `.env:POSTGRES_DB` (`waltid`) | N | wallet DB |
| `DB_USERNAME` | `.env:POSTGRES_USER` (`waltid`) | N | wallet DB user |
| `DB_PASSWORD` | `.env:POSTGRES_PASSWORD` (`waltid`) | **Y** | wallet DB password |
| `POSTGRES_DB_HOST` | hard-coded `postgres` | N | service-name DNS |
| `POSTGRES_DB_PORT` | hard-coded `5432` | N | DB port |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `./wallet-api/config:/waltid-wallet-api/config` | bind | 9 `.conf` files — listed below |
| `./wallet-api/data:/waltid-wallet-api/data` | bind | runtime data dir (**directory does not exist on disk yet** — created on first boot). Holds wallet-side state distinct from Postgres. |

### Config files (under `deploy/compose/stack/wallet-api/config/`)

| File | Lines | Purpose | Env-specific? |
|---|---|---|---|
| `_features.conf` | 11 | walt.id feature toggles | TBD in Phase 0.2 |
| `auth.conf` | 13 | auth provider config (likely OIDC client settings) | **Yes — OIDC client secrets** |
| `db.conf` | 26 | DB connection (DSN, pool sizing) | **Yes — DSN, password** |
| `logins.conf` | 6 | login method allowlist | likely static |
| `oidc.conf` | 36 | OIDC provider trust list | **Yes — issuer URLs** |
| `registration-defaults.conf` | 43 | new-wallet defaults | likely static |
| `rejectionreason.conf` | 5 | rejection reason taxonomy | static |
| `trust.conf` | 9 | trusted issuer/verifier list | **Yes — depends on deployment** |
| `web.conf` | 2 | HTTP listener config | port |

### `extra_hosts`

```
- "localhost:host-gateway"
- "host.docker.internal:host-gateway"
```

These map docker-internal DNS to the host. **Not portable to K8s.** In K8s, replace with proper Service names + an `Ingress` for any host-loopback traffic.

### Ports

Container `${WALTID_WALLET_PORT:-7001}` — not published directly.

### Dependencies

```
depends_on:
  postgres: { condition: service_healthy }
  caddy:    { condition: service_started }
```

### Persistent state

- **Postgres rows** — wallet-api owns several tables in the shared `waltid` DB. (Schema audit in Phase 0.2 / Phase 4.3.)
- **`./wallet-api/data` bind mount** — purpose TBC; flagged as a candidate PVC.

### K8s mapping

- `Deployment` (default 1 replica until horizontal-scale verified — Phase 6.2).
- PVC for the `data` dir (RWO).
- Wallet DB hosted by `CloudNativePG` (Phase 4.6 umbrella references the cluster name).
- HPA disabled by default; revisit after Phase 6.2 load test.
- DB credentials via External Secrets Operator pulling from Vault.

---

## keycloak

Image: `quay.io/keycloak/keycloak:latest` (Phase 1.1: pin to `quay.io/keycloak/keycloak:25.0`).
Role: IdP option #1 for verifiably-go. Used to authenticate end-users into the demo app.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `KC_BOOTSTRAP_ADMIN_USERNAME` | `.env:KEYCLOAK_ADMIN_USER` (`admin`) | N | admin user (admin user identity itself isn't a secret, but...) |
| `KC_BOOTSTRAP_ADMIN_PASSWORD` | `.env:KEYCLOAK_ADMIN_PASSWORD` (`admin`) | **Y** | weak demo default |
| `KC_HTTP_PORT` | `.env:KEYCLOAK_PORT` (`8180`) | N | listen port |

### Command

`start-dev --http-port=${KEYCLOAK_PORT:-8180} --import-realm`

**`start-dev` is dev-only** — uses the embedded H2 database, no clustering, no TLS. Production K8s deployment must use `start` with a real DB and `KC_HOSTNAME` set. Phase 4.5 chart wraps Bitnami's chart which handles this.

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `./keycloak-realm.json:/opt/keycloak/data/import/vcplatform-realm.json` | bind | realm definition imported on `--import-realm` |

`keycloak-realm.json` is 52 lines — a stripped-down realm with the `vcplatform` realm and the `vcplatform` client (matching `KEYCLOAK_REALM` / `KEYCLOAK_CLIENT_ID` in `.env`). Treat as an **environment-agnostic seed** safe to bake into the chart, but redirect URIs may need parameterizing.

### Ports

Container `${KEYCLOAK_PORT:-8180}` published to host.

### Dependencies

None at the compose level. The verifiably-go app reads `KEYCLOAK_REALM` / `KEYCLOAK_CLIENT_ID` and points at it.

### Persistent state

H2 (in-memory) — wiped on every restart in dev mode. **No persistence today.** In K8s, the chart's CNPG cluster reference takes over.

### Other env consumed by verifiably-go (not by Keycloak itself)

| Var | Default | Used by |
|---|---|---|
| `KEYCLOAK_REALM` | `vcplatform` | verifiably-go's auth-providers.json render |
| `KEYCLOAK_CLIENT_ID` | `vcplatform` | same |

### K8s mapping

- Use Bitnami's `keycloak` chart (Phase 4.5) backed by the platform CNPG.
- `KEYCLOAK_ADMIN_PASSWORD` from a Secret synced via ESO.
- Realm import via a `Job` running `kc.sh import` against a PVC the running Keycloak mounts.

---

## wso2is

Image: `wso2/wso2is:7.0.0` (already pinned).
Role: IdP option #2. Production target for the verifiably-go app's enterprise auth path.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `JAVA_OPTS` | hard-coded `-Xms512m -Xmx1024m` | N | JVM heap |
| `SIGNUP_REDIRECT_URL` | `http://${VERIFIABLY_PUBLIC_HOST:-172.24.0.1}:${VERIFIABLY_HOST_PORT:-8080}/auth` | N | injected into JSP success-page via patcher script |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `./wso2-deployment.toml:/home/wso2carbon/wso2is-7.0.0/repository/conf/deployment.toml:ro` | bind | rendered from `wso2-deployment.toml.template` by `deploy.sh` — tells WSO2 IS its public hostname/proxy port |
| `./inji/wso2is/patch-signup-redirect.sh:/home/wso2carbon/patch-signup-redirect.sh:ro` | bind | background script that sed-injects a meta-refresh into the signup-success JSP |

`wso2-deployment.toml.template` and `wso2-deployment-default.toml` (43 lines) are the env-specific config. Public hostname + proxy port placeholders, bootstrapped client creds.

### Custom entrypoint

```bash
/home/wso2carbon/patch-signup-redirect.sh > /tmp/patch-signup-redirect.log 2>&1 &
exec /home/wso2carbon/wso2is-7.0.0/bin/wso2server.sh
```

The patcher polls for the extracted JSP and injects the redirect — relies on writeable filesystem inside the container. Conflicts with `readOnlyRootFilesystem: true` in K8s — mount JSP-extraction dir as `emptyDir`.

### Ports

Container `9443` → host `${WSO2_PORT:-9443}`.

### Dependencies

None at compose level.

### Persistent state

WSO2's H2 DB lives inside the container — wiped on restart. For K8s, point WSO2 at an external DB (its `master-datasources.xml` / `deployment.toml` `[database.identity_db]` block) — likely a CNPG database in our case.

### Other env consumed by verifiably-go

| Var | Default | Notes |
|---|---|---|
| `WSO2_CLIENT_ID` | `verifiably_go_client` | hard-coded client name |
| `WSO2_CLIENT_SECRET` | empty in `.env.example` | populated by `scripts/bootstrap-wso2is.sh` on first run — the bootstrap is **K8s-incompatible** and must be replaced with a `Job` |

### K8s mapping (Phase 4.5)

- No upstream community chart — write from scratch.
- StatefulSet (WSO2 needs stable identity for clustering even in a single-replica deploy).
- ConfigMap for `deployment.toml`.
- `initContainer` runs the JSP patcher; main container is read-only-root.
- `Job` replaces `bootstrap-wso2is.sh` to register the client and write `WSO2_CLIENT_SECRET` to a Secret.
- External DB via CNPG.

---

## libretranslate

Image: `libretranslate/libretranslate:latest` (Phase 1.1: pin to `libretranslate/libretranslate:1.6`).
Role: in-house translation API for verifiably-go's i18n.

### Env vars

| Var | Source default | Secret | Notes |
|---|---|---|---|
| `LT_LOAD_ONLY` | hard-coded `en,es,fr` | N | language allowlist |

### Bind mounts / volumes

| Mount | Type | Purpose |
|---|---|---|
| `lt-data:/home/libretranslate/.local` | named volume | downloaded language model files |

### Ports

Container `5000` → host `${LIBRETRANSLATE_PORT:-5000}`.

### Dependencies

None.

### Persistent state

`lt-data` — language model cache. Cold-start downloads ~hundreds of MB. **Should** persist across pod restarts.

### K8s mapping (Phase 4.5)

- `Deployment` with PVC for `/home/libretranslate/.local`.
- 1 replica is fine; load is light. HPA off.
- Cluster-internal `Service` on port 5000.

---

## verifiably-go (the Go application itself)

Not in `docker-compose.yml` — built and run by `deploy.sh:691-768`. The compose stack is *backend infrastructure* and the Go app is bound to it via `backends.json` (rendered per `deploy.sh:820-870`).

Phase 4.4 wraps it in its own chart. Inventory:

### Env vars

| Var | Default | Secret | Notes |
|---|---|---|---|
| `VERIFIABLY_PUBLIC_HOST` | `172.24.0.1` | N | the **single knob** you change between laptop / EC2 / on-prem. In K8s = the cluster's public hostname. |
| `PUBLIC_HOST` | mirrors `VERIFIABLY_PUBLIC_HOST` | N | alias used by some compose services |
| `VERIFIABLY_HOST_PORT` | `8080` | N | host port |
| `VERIFIABLY_ADDR` | `:8080` | N | listen addr |
| `VERIFIABLY_IMAGE` | `verifiably-go:local` | N | image ref — overridden by Phase 1.3 CI build |
| `VERIFIABLY_CONTAINER` | `verifiably-go` | N | container name; n/a in K8s |
| `VERIFIABLY_PUBLIC_URL` | `http://${VERIFIABLY_PUBLIC_HOST}:${VERIFIABLY_HOST_PORT}` | N | self-URL |
| `VERIFIABLY_DEBUG_MOCK_MARKERS` | `0` | N | UI flag |
| `VERIFIABLY_INJI_EXTRA_KIDS` | empty | N | inji-proxy seed (out of scope for waltid scenario) |

### Mounts

The container mounts the rendered `backends.json` and `auth-providers.json` (generated by `deploy.sh`). In K8s these become ConfigMaps.

### Dependencies

Logical (via `backends.json` URLs): `wallet-api`, `issuer-api`, `verifier-api`, `keycloak`, `wso2is`, `libretranslate`.

---

## Cross-cutting: `.env.example` summary

All env vars referenced above plus:

| Var | Default | Notes |
|---|---|---|
| `INJIWEB_P12_PASSWORD` | `xy4gh6swa2i` | only used by inji-* services — **out of scope for waltid scenario but listed for completeness** |
| `GOOGLE_OAUTH_CLIENT_ID` / `_SECRET` | dummy | inji-mimoto only — out of scope |

Variables consumed in the `waltid` scenario:

```
VERIFIABLY_PUBLIC_HOST     PUBLIC_HOST
VERIFIABLY_HOST_PORT       VERIFIABLY_ADDR        VERIFIABLY_IMAGE
VERIFIABLY_CONTAINER       VERIFIABLY_PUBLIC_URL  VERIFIABLY_DEBUG_MOCK_MARKERS
WALTID_ISSUER_PORT         WALTID_WALLET_PORT     WALTID_VERIFIER_PORT
KEYCLOAK_PORT              KEYCLOAK_REALM         KEYCLOAK_CLIENT_ID
KEYCLOAK_ADMIN_USER        KEYCLOAK_ADMIN_PASSWORD
WSO2_PORT                  WSO2_CLIENT_ID         WSO2_CLIENT_SECRET
LIBRETRANSLATE_PORT
POSTGRES_USER              POSTGRES_PASSWORD      POSTGRES_DB             POSTGRES_PORT
```

---

## Summary table — what each chart needs to wire

| Chart (Phase 4) | ConfigMap from | Secret keys | PVC | Service ports | Ingress hostname |
|---|---|---|---|---|---|
| `walt-issuer` | `issuer-api/config/*.conf` | signing key (Phase 7.1: Vault Transit) | none | 7002 | `issuer.<domain>` |
| `walt-verifier` | `verifier-api/config/*.conf` | signing key (Phase 7.1) | none | 7003 | `verifier.<domain>` |
| `walt-wallet` | `wallet-api/config/*.conf` | DB password, OIDC client secrets, signing key | `data` dir RWO | 7001 | `wallet.<domain>` |
| `verifiably-go` | rendered `backends.json` + `auth-providers.json` | none (consumes public URLs only) | none | 8080 | `app.<domain>` |
| `keycloak` (wrapper) | `keycloak-realm.json` | admin password, DB password | via Bitnami chart | 8180 | `auth.<domain>` |
| `wso2is` | `wso2-deployment.toml`, `patch-signup-redirect.sh` | admin password, DB password, client secret | jsp emptydir + DB | 9443 | `wso2.<domain>` |
| `libretranslate` | none | none | model cache | 5000 | `translate.<domain>` (cluster-internal only) |
| `cnpg-cluster` | none | superuser password | managed by operator | 5432 | n/a |

## Open questions for Phase 0.2

1. Exact schema and env-specific fields in each walt.id `.conf` file (esp. wallet-api's `oidc.conf` and `trust.conf`).
2. Whether issuer/verifier signing material lives **inside** the `.conf` files or in a separate keystore directory we missed.
3. What `wallet-api/data` actually holds at runtime — log files, ephemeral cache, or persistent state.
4. Health endpoint paths for issuer / verifier / wallet (Caddy reverse-proxies them but doesn't probe — need to test against a running container).
5. Whether walt.id services expose a Prometheus/Micrometer endpoint and on which port (Phase 6.1 input).
