# Walt.id config files — schema + values.yaml mapping

Source: every file mounted into `issuer-api`, `verifier-api`, and `wallet-api` under `verifiably-go/deploy/compose/stack/`. This file is the canonical contract between the existing `.conf` files and the `values.yaml` schemas that Phase 4.1–4.3 will produce.

Format: per file, the **full schema** (with raw values), a column splitting fields into **static** (bake into chart defaults), **env-specific** (templated from `values.yaml`), and **secret** (Secret + ESO/Vault), plus the `values.yaml` path each field will live at.

🔥 = critical security finding flagged for Phase 7.1.

---

## issuer-api (`waltid/issuer-api:0.18.2`)

Image expects two HOCON files at `/waltid-issuer-api/config/`.

### `issuer-service.conf` (1 line)

```hocon
baseUrl = "http://localhost:7002"
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `baseUrl` | no | **yes** | no | `issuer.baseUrl` — must be the public ingress URL (e.g. `https://issuer.example.com`) |

### `web.conf` (2 lines)

```hocon
webHost = "0.0.0.0"
webPort = "${ISSUER_API_PORT}"
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `webHost` | yes | no | no | always `0.0.0.0` |
| `webPort` | no | yes | no | `issuer.service.port` (default 7002) |

### Open question — signing keys

Neither `.conf` file references issuer signing keys. The image either generates them in-memory at startup or expects an `issuer-config.json` / `issuers/` directory we did not inventory. **Phase 4.1 must verify** by `docker exec` into a running issuer-api and reading `/waltid-issuer-api/`. If keys are persisted to disk, that path becomes a PVC + Secret.

### `values.yaml` skeleton

```yaml
issuer:
  image: waltid/issuer-api
  tag: "0.18.2"
  service:
    port: 7002
  baseUrl: ""              # required; e.g. https://issuer.example.com
  replicas: 2
  hpa:
    enabled: false
    minReplicas: 2
    maxReplicas: 5
    targetCPUUtilizationPercentage: 70
  resources: {requests: {cpu: 100m, memory: 256Mi}, limits: {memory: 512Mi}}
  podDisruptionBudget: {minAvailable: 1}
  signingKey:
    existingSecret: ""     # Secret name, mounted into config dir (TBD by Phase 4.1 audit)
```

---

## verifier-api (`waltid/verifier-api:0.18.2`)

Same shape as issuer-api.

### `verifier-service.conf` (1 line)

```hocon
baseUrl = "http://${SERVICE_HOST}:${VERIFIER_API_PORT}"
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `baseUrl` | no | **yes** | no | `verifier.baseUrl` — public ingress URL |

### `web.conf` (2 lines)

```hocon
webHost = "0.0.0.0"
webPort = "${VERIFIER_API_PORT}"
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `webHost` | yes | no | no | always `0.0.0.0` |
| `webPort` | no | yes | no | `verifier.service.port` (default 7003) |

### `values.yaml` skeleton

```yaml
verifier:
  image: waltid/verifier-api
  tag: "0.18.2"
  service:
    port: 7003
  baseUrl: ""              # required
  replicas: 2
  hpa:
    enabled: true          # verifier is fully stateless
    minReplicas: 2
    maxReplicas: 8
    targetCPUUtilizationPercentage: 70
  resources: {requests: {cpu: 100m, memory: 256Mi}, limits: {memory: 512Mi}}
  podDisruptionBudget: {minAvailable: 1}
  signingKey:
    existingSecret: ""     # TBD per Phase 4.2 audit
```

---

## wallet-api (`waltid/wallet-api:0.18.2`) — the complicated one

Nine HOCON files at `/waltid-wallet-api/config/`. This is where the real schema work happens.

### `_features.conf` (11 lines)

```hocon
enabledFeatures = [
    # external-signature-endpoints,
    # trusted-ca,
    # entra,
    # ktor-authnz,
    dev-mode
]
disabledFeatures = [
    # auth   # legacy auth
]
```

🔥 **`dev-mode` is enabled** — must be disabled or made env-conditional in any non-dev deployment. walt.id's dev-mode short-circuits security checks.

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `enabledFeatures` | no | yes | no | `wallet.features.enabled[]` — defaults to empty for prod, dev profile injects `dev-mode` |
| `disabledFeatures` | no | yes | no | `wallet.features.disabled[]` |

### `auth.conf` (13 lines) — 🔥🔥 CRITICAL SECRETS

```hocon
encryptionKey = "dncygwnvivxzlohc"         // 16-char (128-bit) symmetric key
signKey       = "jyjeylmidlylokzh"         // 16-char (128-bit) symmetric key
tokenKey      = "{...RSA JWK private key inline...}"
audTokenClaim = "http://${SERVICE_HOST}:${WALLET_BACKEND_PORT}"
issTokenClaim = "http://${SERVICE_HOST}:${WALLET_BACKEND_PORT}"
tokenLifetime = "30"                       // days
```

🔥 **All three crypto materials are baked into the file with demo defaults committed to git.** They MUST come from Vault in any non-dev deployment. The 30-day token lifetime is also long; consider lowering.

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `encryptionKey` | no | no | **YES — Vault** | `wallet.auth.encryptionKey.existingSecret` (key `encryption-key`) |
| `signKey` | no | no | **YES — Vault** | `wallet.auth.signKey.existingSecret` (key `sign-key`) |
| `tokenKey` | no | no | **YES — Vault Transit** | `wallet.auth.tokenKey.existingSecret` (key `token-key.jwk`); Phase 7.1 moves this to Vault Transit |
| `audTokenClaim` | no | yes | no | `wallet.auth.audClaim` — public wallet URL |
| `issTokenClaim` | no | yes | no | `wallet.auth.issClaim` — public wallet URL |
| `tokenLifetime` | no | yes | no | `wallet.auth.tokenLifetimeDays` — default 7 in prod |

### `db.conf` (26 lines)

```hocon
dataSource {
    jdbcUrl = "jdbc:postgresql://${POSTGRES_DB_HOST}:${POSTGRES_DB_PORT}/${DB_NAME}"
    driverClassName = "org.postgresql.Driver"
    username = "${DB_USERNAME}"
    password = "${DB_PASSWORD}"
    transactionIsolation = "TRANSACTION_SERIALIZABLE"
    maximumPoolSize = 16
    minimumIdle = 4
    maxLifetime = 60000
    autoCommit = false
    dataSource {
        journalMode = WAL
        fullColumnNames = false
    }
}
recreateDatabaseOnStart = false
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `jdbcUrl` | no | yes | no | derived from `wallet.db.host` + `port` + `database` |
| `driverClassName` | yes | no | no | always Postgres |
| `username` | no | yes | no | `wallet.db.username` |
| `password` | no | no | **YES** | `wallet.db.passwordSecret` (`existingSecret` + key) — populated by CNPG-issued credentials Secret |
| `transactionIsolation` | yes | no | no | always SERIALIZABLE |
| `maximumPoolSize` | no | yes | no | `wallet.db.pool.max` (default 16) |
| `minimumIdle` | no | yes | no | `wallet.db.pool.minIdle` (default 4) |
| `maxLifetime` | no | yes | no | `wallet.db.pool.maxLifetimeMs` (default 60000) |
| `autoCommit` | yes | no | no | always false |
| `dataSource.journalMode` | yes | no | no | WAL — applies only when SQLite is used; harmless for Postgres |
| `dataSource.fullColumnNames` | yes | no | no | always false |
| `recreateDatabaseOnStart` | yes | no | no | always false in K8s; CNPG owns the schema |

### `logins.conf` (6 lines)

```hocon
enabledLoginMethods = ["email", "web3", "oidc", "passkeys"]
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `enabledLoginMethods` | no | yes | no | `wallet.logins.enabled[]` — env can disable `oidc` if Keycloak/WSO2IS is not deployed |

### `oidc.conf` (36 lines) — wallet-side OIDC client config

```hocon
publicBaseUrl = "http://${SERVICE_HOST}:${DEMO_WALLET_FRONTEND_PORT}"
providerName = keycloak
oidcRealm = "http://0.0.0.0:8080/realms/waltid-keycloak-ktor"
oidcJwks = "${oidcRealm}/protocol/openid-connect/certs"
oidcScopes = ["roles"]
authorizeUrl    = "${oidcRealm}/protocol/openid-connect/auth"
accessTokenUrl  = "${oidcRealm}/protocol/openid-connect/token"
logoutUrl       = "${oidcRealm}/protocol/openid-connect/logout"
clientId = "waltid_backend"
clientSecret = "**********"
keycloakUserApi = "http://0.0.0.0:8080/admin/realms/waltid-keycloak-ktor/users"
jwksCache = { cacheSize = 10, cacheExpirationHours = 24, rateLimit: { bucketSize: 10, refillRateMinutes: 1 } }
```

🔥 **As shipped this file points at a non-existent realm and uses a `**********` placeholder secret.** OIDC login is broken out of the box in the demo. For K8s either:

- **Path A (recommended)**: drop `oidc` from `enabledLoginMethods` until the realm + client are real.
- **Path B**: wire to our `keycloak` chart's `vcplatform` realm (Phase 4.5).

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `publicBaseUrl` | no | yes | no | `wallet.oidc.publicBaseUrl` (the wallet frontend URL — currently undefined) |
| `providerName` | no | yes | no | `wallet.oidc.providerName` |
| `oidcRealm` | no | yes | no | `wallet.oidc.realmUrl` — must point at Keycloak `vcplatform` realm |
| `oidcJwks` / `authorizeUrl` / `accessTokenUrl` / `logoutUrl` | yes | derived | no | rendered from `oidcRealm` |
| `oidcScopes` | no | yes | no | `wallet.oidc.scopes[]` |
| `clientId` | no | yes | no | `wallet.oidc.clientId` |
| `clientSecret` | no | no | **YES** | `wallet.oidc.clientSecret.existingSecret` |
| `keycloakUserApi` | no | yes | no | `wallet.oidc.keycloakUserApi` |
| `jwksCache.*` | yes | no | no | static defaults |

### `registration-defaults.conf` (43 lines)

```hocon
defaultKeyConfig: { backend: jwk, keyType: secp256r1 }
defaultDidConfig: { method: jwk }
// (commented-out Hashicorp Vault TSE, OCI, did:web examples)
```

The commented-out Vault TSE block is **directly relevant to Phase 7.1** — uncommenting it (with values from our deployed Vault) is the production-grade key backend.

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `defaultKeyConfig.backend` | no | yes | no | `wallet.registration.keyBackend` — `jwk` (dev) or `tse` (prod, points at our Vault) |
| `defaultKeyConfig.keyType` | no | yes | no | `wallet.registration.keyType` — defaults `secp256r1`; `Ed25519` for Vault TSE |
| `defaultKeyConfig.config.server` | no | yes | no | `wallet.registration.vault.address` (Vault URL inside cluster) |
| `defaultKeyConfig.config.accessKey` | no | no | **YES** | `wallet.registration.vault.tokenSecret` (Secret name) |
| `defaultDidConfig.method` | no | yes | no | `wallet.registration.didMethod` — `jwk` (default) or `web` |
| `defaultDidConfig.config.domain` | no | yes | no | `wallet.registration.didWeb.domain` (only when method=`web`) |
| `defaultDidConfig.config.path` | no | yes | no | `wallet.registration.didWeb.path` |

### `rejectionreason.conf` (5 lines)

```hocon
reasons = ["Unknown sender", "Not relevant to me", "Unsure about accuracy", "Need more details"]
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `reasons[]` | no | yes | no | `wallet.rejectionReasons[]` — operator may localize; defaults baked into chart |

### `trust.conf` (9 lines)

```hocon
issuersRecord:  { baseUrl = "<url>", trustRecordPath = "<path>", governanceRecordPath = "<path>" }
verifiersRecord:{ baseUrl = "<url>", trustRecordPath = "<path>", governanceRecordPath = "<path>" }
```

🔥 As shipped, all four URLs/paths are placeholder strings (`<url>`, `<path>`). The trust registry feature is effectively **disabled by being unconfigured**.

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `issuersRecord.baseUrl` | no | yes | no | `wallet.trust.issuers.baseUrl` (empty by default — disables feature) |
| `issuersRecord.trustRecordPath` | no | yes | no | `wallet.trust.issuers.trustPath` |
| `issuersRecord.governanceRecordPath` | no | yes | no | `wallet.trust.issuers.governancePath` |
| `verifiersRecord.*` | no | yes | no | mirrors the above under `wallet.trust.verifiers.*` |

### `web.conf` (2 lines)

```hocon
webHost = "0.0.0.0"
webPort = ${WALLET_BACKEND_PORT}
```

| Field | Static? | Env? | Secret? | Maps to |
|---|---|---|---|---|
| `webHost` | yes | no | no | always `0.0.0.0` |
| `webPort` | no | yes | no | `wallet.service.port` (default 7001) |

### Wallet `values.yaml` skeleton

```yaml
wallet:
  image: waltid/wallet-api
  tag: "0.18.2"

  service:
    port: 7001
  replicas: 1
  hpa:
    enabled: false        # do not enable until Phase 6.2 verifies horizontal-scale safety
  resources: {requests: {cpu: 200m, memory: 512Mi}, limits: {memory: 1Gi}}
  podDisruptionBudget: {minAvailable: 1}

  features:
    enabled: []           # production: empty. dev profile: ["dev-mode"]
    disabled: []

  auth:
    audClaim: ""          # https://wallet.example.com
    issClaim: ""          # same
    tokenLifetimeDays: 7
    secrets:
      existingSecret: ""  # Secret with keys: encryption-key, sign-key, token-key.jwk

  db:
    host: ""              # CNPG cluster RW service, e.g. waltid-pg-rw.waltid.svc
    port: 5432
    database: "waltid"
    username: "waltid"
    passwordSecret:
      name: ""            # CNPG-issued Secret
      key: "password"
    pool: {max: 16, minIdle: 4, maxLifetimeMs: 60000}

  logins:
    enabled: ["email", "passkeys"]   # default — `oidc` opt-in once OIDC is wired

  oidc:
    enabled: false
    publicBaseUrl: ""
    providerName: "keycloak"
    realmUrl: ""          # https://auth.example.com/realms/vcplatform
    scopes: ["roles"]
    clientId: "waltid_backend"
    clientSecret:
      existingSecret: ""  # Secret name, key `client-secret`
    keycloakUserApi: ""

  registration:
    keyBackend: "jwk"     # "tse" in prod
    keyType: "secp256r1"  # Ed25519 for tse
    didMethod: "jwk"
    vault:                # only used when keyBackend=tse
      address: ""
      tokenSecret:
        name: ""
        key: "token"
    didWeb:               # only used when didMethod=web
      domain: ""
      path: ""

  trust:
    issuers:   {baseUrl: "", trustPath: "", governancePath: ""}
    verifiers: {baseUrl: "", trustPath: "", governancePath: ""}

  rejectionReasons:
    - "Unknown sender"
    - "Not relevant to me"
    - "Unsure about accuracy"
    - "Need more details"

  data:
    persistence:
      enabled: true
      size: 5Gi
      storageClass: ""    # use cluster default
```

---

## Summary — secrets that must be in Vault

| Secret | Source today | Used by | Phase |
|---|---|---|---|
| `wallet.auth.encryptionKey` | hard-coded `dncygwnvivxzlohc` | wallet-api | 4.3 (existingSecret), 7.1 (rotate) |
| `wallet.auth.signKey` | hard-coded `jyjeylmidlylokzh` | wallet-api | 4.3, 7.1 |
| `wallet.auth.tokenKey` (RSA JWK) | hard-coded inline | wallet-api | 4.3, 7.1 (Vault Transit) |
| `wallet.db.password` | env `POSTGRES_PASSWORD=waltid` | wallet-api | 4.3 (CNPG-issued Secret) |
| `wallet.oidc.clientSecret` | placeholder `**********` | wallet-api | 4.3, 4.5 (real client Secret) |
| `wallet.registration.vault.token` | none today | wallet-api when `keyBackend=tse` | 7.1 |
| issuer signing key (location TBD) | unknown | issuer-api | 4.1 audit + 7.1 |
| verifier signing key (location TBD) | unknown | verifier-api | 4.2 audit + 7.1 |

## Open questions still unresolved

1. **Where do issuer-api / verifier-api keep their signing keys?** Not in the `.conf` files we reviewed. Must be answered in Phase 4.1 / 4.2 by reading the running container.
2. **Schema-managed vs. operator-managed wallet DB.** Walt.id's `recreateDatabaseOnStart=false` means the app expects the schema to exist. Verify whether wallet-api auto-migrates on first start (good — handle via Helm `pre-install` Job) or whether a separate migration tool ships in the image.
3. **`wallet-api/data` mount purpose.** Determine whether the directory is logs-only (drop) or holds persistent state (keep PVC).
4. **OIDC realm wiring.** Confirm with stakeholders whether wallet-side OIDC login is in scope for the K8s "single click" or deferred.
5. **Trust registry.** Same — feature currently dormant; chart defaults disable it but operator may want to enable in prod.

## Schema-mapping conventions for Phase 4

- All `*.conf` files become a single `ConfigMap` per service, mounted at the original path.
- HOCON env-var substitutions (`${POSTGRES_DB_HOST}` etc.) are preserved; the K8s `env:` block on the Deployment supplies real values, identical to the compose pattern. This keeps the compose **and** K8s rendering paths sharing one set of source files (Phase 1.2 deliverable).
- Secrets are mounted via `envFrom: secretRef` + `${VAR}` substitution — never as files.
- HPA / PDB / NetworkPolicy / SecurityContext — not in the `.conf` schema; defined per-chart in Phase 4.
