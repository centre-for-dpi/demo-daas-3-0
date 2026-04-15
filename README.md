# vc.infra — CDPI white-label verifiable credentials platform

A Go + HTMX + vanilla CSS white-label app that covers the full verifiable
credential lifecycle (issuer / holder / verifier / auditor / admin) and is
backend-agnostic across three DPGs: **walt.id**, **MOSIP Inji Certify /
Inji Verify / Inji Web**, and **Credebl** (beta). A single brand, one
codebase, per-user DPG routing — every holder, issuer and verifier picks
their own backend without any server-side mode switches.

The project lives in two top-level directories:

- **`ui-demo/`** — the Go white-label app, all Go code, all HTML templates,
  all DPG adaptors, docker stack.
- **`agent-service/`** — the chatbot backend: an n8n workflow with three
  CDPI personas (Senior Technical Architect, Programs & Operations Officer,
  Platform Guide), a RAG corpus over CDPI decks and scope docs, and a Marp
  theme for exporting branded `.pptx` artifacts.

Plus **`references/`** — the RAG source corpus (decks, advisory notes,
scope templates, transcripts). Agent ingestion reads from here.

---

## Table of contents

- [Architecture at a glance](#architecture-at-a-glance)
- [What works today](#what-works-today)
- [How each piece works](#how-each-piece-works)
  - [White-label routing and per-user DPG choice](#white-label-routing-and-per-user-dpg-choice)
  - [Issuer workspace](#issuer-workspace)
  - [Holder workspace and wallet DPGs](#holder-workspace-and-wallet-dpgs)
  - [Verifier workspace](#verifier-workspace)
  - [Trust / admin / audit workspaces](#trust--admin--audit-workspaces)
  - [Agent service and Outputs page](#agent-service-and-outputs-page)
  - [Data sources](#data-sources)
  - [Docker stack](#docker-stack)
  - [Inji Web + Mimoto + esignet + Inji Certify end-to-end](#inji-web--mimoto--esignet--inji-certify-end-to-end)
- [Repository layout](#repository-layout)
- [Running it locally](#running-it-locally)
- [What is pending or fragile](#what-is-pending-or-fragile)
- [Known quirks and gotchas](#known-quirks-and-gotchas)

---

## Architecture at a glance

```
┌──────────────────────────────────────────────────────────────────────┐
│  Browser                                                             │
│    • Landing (exploration) pages + Portal (production) pages         │
│    • Chatbot partial on every page (sessionStorage + BroadcastCh.)   │
│    • HTMX for all intra-portal navigation                            │
└────────────┬─────────────────────────────────────────────────────────┘
             │ HTTP (same origin)
             ▼
┌──────────────────────────────────────────────────────────────────────┐
│  ui-demo/cmd/server  — Go 1.24 + net/http std mux                    │
│                                                                      │
│  Handlers (internal/handler/*.go)                                    │
│    landing • auth • portal • issuer • holder • verifier • trust      │
│    admin   • agent • schemas  • bulk   • credtype • share • oidc     │
│    capabilities • translate • local_issuer • inji_proxy              │
│                                                                      │
│  Per-user DPG registries resolve the right store at request time:    │
│    issuerRegistry[waltid|inji|credebl]                               │
│    walletRegistry[waltid|local|credebl|pdf|inji_web]                 │
│    verifierRegistry[waltid|inji|adapter|credebl]                     │
└────────────┬─────────────────────────────────────────────────────────┘
             │ HTTP (per-DPG adaptors)
             ▼
┌───────────────┬───────────────┬────────────────┬────────────────────┐
│  walt.id      │  Inji Certify │  Inji Verify   │  Credebl agent     │
│  issuer-api   │  (+ Mimoto /  │  +verification │  (beta stub)       │
│  verifier-api │   esignet /   │  adapter for   │                    │
│  wallet-api   │   Inji Web)   │  offline verif)│                    │
└───────────────┴───────────────┴────────────────┴────────────────────┘
             │                             │
             ▼                             ▼
  (Go LDP signer for LDP_VC        (PDFWalletStore for
   when the DPG doesn't              printable self-verifying
   natively expose ldp_vc)           PixelPass QR PDFs)
```

One compose file (`ui-demo/docker/waltid/docker-compose.yml`) defines 25
services on a pinned `172.24.0.0/16` network: walt.id issuer / verifier
/ wallet stack, Inji Certify + certify-nginx + certify-postgres, Inji
Verify UI + service + postgres, Citizens Postgres (mock government
registry), verification-adapter, plus the full Inji Web stack (Inji
Web UI + Mimoto + esignet + mock-identity-system + oidc-ui + data-share
+ Minio + Redis + Postgres) under an opt-in `--profile injiweb`. The
Go `vcplatform` app runs directly on the host and talks to the stack
over mapped ports.

---

## What works today

### Issuer
- Register issuer DID + key via any DPG (walt.id, Inji, Credebl)
- Schema builder (live DPG-backed catalog, not static fixtures)
- Custom schema creation with automatic registration against the DPG's
  credential_config SQL table (Inji) or HOCON file (walt.id)
- Single issuance with dynamic data-source lookup (Postgres ILIKE across
  national_id / names / phone / email / student_id / farm_id)
- Bulk issuance via CSV upload or data-provider plugin
- OID4VCI Pre-Authorized Code flow offers in three delivery formats:
  copy-link, QR code, direct claim to the holder's wallet
- In-process URDNA2015 + Ed25519Signature2020 LDP_VC signer (`ldpsigner`)
  used when the chosen issuer DPG doesn't natively expose an `ldp_vc`
  endpoint — fronted by a real OID4VCI server (`local_issuer.go`)
- Structured error responses that expose known DPG interop gaps
  (walt.id wallet ↔ Inji proof-JWT incompatibility, PDF wallet QR-too-large,
  etc.) with recovery actions instead of silent fallbacks

### Holder
- Wallet-backend picker card at the top of `My Credentials` — switches the
  active `WalletDPG` in place, no full wizard re-run
- Five wallet DPG options: walt.id Wallet, In-Process Holder (Go OID4VCI
  Pre-Auth client), Credebl Wallet (beta), **Print PDF Wallet**
  (PixelPass-encoded self-verifying QR + printable PDF), and **Inji Web**
  (redirects the holder to a locally-running `injistack/inji-web` container)
- Onboarding wizard gates entry — new holders are redirected to
  `/portal/onboarding` before any credential lands in a wallet they
  didn't pick
- Claim credential via offer URL, QR scan, or one-click "Claim to My Wallet"
- Selective disclosure, presentation builder, dependent credential
  management, inbox, timeline, export, credential catalog
- Proactive sharing via a short-lived `/share/v/{id}` URL that serves the
  credential's claims + a Verify button running direct-verify against the
  configured verifier. PixelPass encoder produces an offline-verifiable QR
  alongside the link
- Full OID4VCI **Authorization Code Flow** end-to-end against Inji Certify
  via Inji Web + Mimoto + esignet (see section below)

### Verifier
- OID4VP-style verification session via walt.id verifier-api
- Direct-verify via Inji Verify (POST credential, SUCCESS/INVALID)
- Backend-agnostic verification adapter at `:8085` that runs URDNA2015 +
  Ed25519 verification for LDP_VC, routes SD-JWT by `x5c` header, and
  falls back to walt.id OID4VP session for JWT VCs it can't natively
  verify. Supports genuine air-gap with `--network none` once issuer keys
  are pre-synced via `POST /sync`
- Credential type picker driven from `/api/credential-types` (no more
  bare `VerifiableCredential` default that matched everything in the
  wallet and caused "hardcoded credential" bugs)
- Mismatch detection: if the wallet presents a different credential type
  than the verifier requested, the result renders as warning, not green

### Trust / admin / audit
- Schema registry, issuer directory, trust registry, governance, adaptor
  registry, channel config, offline sync, trust bridge, protocol monitor,
  schema harmonize, agent mode, connectivity, multimodal (all feature-
  flagged admin-only)
- Platform admin: issuer intake, guided schema wizard, sandbox, approval
  queue, deployment, reporting, training, health
- Audit: cross-workspace read-only trails, filterable activity view

### Chatbot / agent service
- Three personas routed by a Haiku 4.5 classifier:
  - **Senior Technical Architect** — advisory notes, technical scopes
  - **Programs & Operations Officer** — pitch decks, adoption proposals,
    blog posts, country proposals
  - **Platform Guide** — triage, explanation, handoff
- Main LLM: Claude Sonnet 4.6
- RAG grounded in `references/` (decks, advisory notes, scope templates,
  conversation transcripts) via Qdrant + Voyage embeddings
- Generated artifacts are saved to the **Outputs** page where they can be
  previewed, edited inline, and downloaded as real CDPI-branded `.pptx`
  (via Marp with a custom theme) or `.md`
- Chatbot is a partial included on every vc.infra page; state persists
  via sessionStorage and syncs with the Outputs page via CustomEvent +
  BroadcastChannel

### Data sources
- Pluggable `DataSource` interface at `internal/datasource/datasource.go`
- Implementations: Postgres (`datasource/postgres`), CSV (`datasource/csv`),
  HTTP API (`datasource/httpapi`), manual passthrough (`datasource/manual`)
- Default deployment ships the Citizens Postgres DB (200 mock records,
  KE + TT, covering birth records, university degrees, and farmer
  registrations) with configurable `SearchFields` for full-text ILIKE
  lookup

### Infrastructure
- Single binary, everything embedded via `embed.FS` — templates, static
  assets, mock data
- Optional `custom/` directory for runtime template and asset overrides
- White-label config at `config/default.json` drives brand, colors,
  typography, and enabled feature flags (workspaces can be toggled off
  entirely — routes aren't even registered if disabled)
- i18n via DeepL API with a language selector in the topbar next to the
  theme toggle
- Pinned docker network `172.24.0.0/16` (gateway `172.24.0.1`) so env
  vars in the compose file keep working across `docker compose down/up`

---

## How each piece works

### White-label routing and per-user DPG choice

Every session stores three independent DPG choices — `user.IssuerDPG`,
`user.WalletDPG`, `user.VerifierDPG`. Handlers resolve the correct store
at request time via `h.issuerFor(user) / h.walletFor(user) /
h.verifierFor(user)` helpers. There is no server-wide `ISSUER_DPG` env
var; every DPG the deployment supports is instantiated at startup and
registered in a map.

```go
// internal/handler/handler.go
func (h *Handler) walletFor(user *model.User) store.WalletStore {
    if user != nil && user.WalletDPG != "" {
        if s, ok := h.walletRegistry[user.WalletDPG]; ok && s != nil {
            return s
        }
    }
    return h.stores.Wallet // server-wide default fallback
}
```

The user's DPG choices are captured in a short onboarding wizard
(`/portal/onboarding`) with role-aware DPG cards and a role-aware confirm
screen that renders `WalletCapabilities` / `VerifierCapabilities` /
`IssuerCapabilities` fields depending on which role is onboarding.

### Issuer workspace

Core handlers in `internal/handler/issuer.go`, `schemas.go`, `bulk.go`,
`credtype.go`, `api.go`.

- **Schema catalog** (`/api/schemas/catalog`) is live — it calls
  `issuer.ListCredentialConfigs(ctx)` on the user's chosen DPG and
  overlays static starter-schema fields where a credentialType match
  is found. Inji users see only the three Farmer* configs their SQL
  seed table actually advertises; walt.id users see all 29 from the
  HOCON file.
- **Custom schema publish** (`/api/schemas/catalog/publish`) registers
  the new type with the DPG first (calling `issuer.RegisterCredentialType`)
  and only saves the local `CredentialSchema` row with the backend-returned
  configId. Blocks `VerifiableCredential` as a configId name (not real on
  any DPG) and surfaces clear errors when the DPG doesn't support runtime
  registration.
- **Single issuance** (`/api/credential/issue`) either calls the DPG's
  native issuance endpoint OR routes LDP_VC issuance through the in-process
  URDNA2015 signer at `internal/store/ldpsigner` wrapped in a real OID4VCI
  Pre-Auth server at `internal/handler/local_issuer.go`, so any OID4VCI
  wallet can claim the resulting offer.
- **Bulk issuance** (`/api/issuer/bulk-csv`, `/api/issuer/bulk-api`)
  supports CSV upload and data-provider-plugin paths.
- **Delivery cards**: Copy Link / QR Code / Claim to My Wallet. The last
  option routes through `POST /api/wallet/claim-offer` against the holder's
  chosen wallet, with structured error recovery when the DPGs don't match
  (e.g. walt.id wallet can't claim from Inji Certify because of a
  proof-JWT `iss` claim incompatibility).

### Holder workspace and wallet DPGs

Core handlers in `internal/handler/holder.go`, `api.go`, `share.go`.

Five wallet backends implementing `store.WalletStore`:

1. **`waltid`** — `internal/store/waltid`. Thin adapter around walt.id's
   wallet-api HTTP endpoints. Best for JWT-format credentials. Known
   limitation: walt.id's wallet-api `CredentialOfferProcessor.kt:59`
   can't parse LDP_VC offers, and its proof JWT includes an `iss` claim
   that Inji Certify rejects (`proof_header_ambiguous_key`). Documented
   as a structured error rather than hidden.

2. **`local`** — `internal/store/localholder`. Pure-Go in-process OID4VCI
   Pre-Authorized Code flow client. Generates an ephemeral ECDSA P-256
   holder keypair and a `did:jwk`, signs proof JWTs with only `kid` (not
   `jwk` — Inji rejects both together), omits `iss` from the payload
   (Inji's draft-13 validator rejects it for anonymous Pre-Auth flows),
   and stores the resulting credential in a process-wide shared
   `walletbag`. The most compatible option for Inji Certify's native
   pre-auth flow.

3. **`credebl`** — `internal/store/credebl`. Beta stub for the
   Sovrin / did:indy ecosystem. Most operations return "unsupported".

4. **`pdf`** — `internal/store/pdfwallet`. A real `WalletStore` whose
   `ClaimCredential` runs the OID4VCI Pre-Auth flow (reusing the
   `localholder` client), PixelPass-encodes the credential
   (`base45(zlib(credJSON))` per RFC 9285), and generates a printable PDF
   containing the human-readable claims plus a self-verifying QR. Best-
   effort QR rendering tries Highest → High → Medium → Low error
   correction levels and returns a structured `QRTooLargeError` when
   every level fails — with the raw + encoded byte counts and suggested
   alternatives. LDP_VC credentials (~1-2 KB) always fit; larger JWT VCs
   may not, and the UI renders the structured error accordingly. PDFs
   are cached in-memory keyed by `(walletToken, credID)` and served via
   `GET /api/wallet/pdf?id=...`.

5. **`inji_web`** — `internal/store/injiweb`. Redirect adapter for the
   MOSIP Inji Web browser wallet. Inji Web is **catalog-initiated** —
   it loads its own issuer list from `mimoto-issuers-config.json` and
   runs OID4VCI against whichever issuer the holder picks. It does not
   accept external `credential_offer` URLs. So the adapter's
   `ClaimCredential` returns a structured `RedirectClaimError` pointing
   at `http://localhost:3004/issuers` (the local Inji Web instance's
   catalog page). The wallet page shows a banner linking to Inji Web
   when `currentDPG == "inji_web"` because credentials live inside
   Mimoto, not in our shared walletbag.

The holder's onboarding wizard forces a wallet DPG choice before any
credential can land in a wallet (`HolderWallet` redirects to
`/portal/onboarding` when `user.WalletDPG == ""`, and `APIWalletClaimOffer`
refuses with a structured error pointing at the wizard). This prevents
the earlier bug where new SSO users had credentials silently land in the
server-default wallet they never picked.

### Verifier workspace

`internal/handler/verifier.go` + `web/templates/verifier/*.html`.

- OID4VP session flow against walt.id verifier-api (`CreateVerificationSession`
  → `GetSessionResult`)
- Direct verify against Inji Verify's `/direct-verify` endpoint for LDP_VC
  and SD-JWT with x5c
- Backend-agnostic **verification adapter** (`internal/store/adapter`)
  calls a separate Go service (`docker/waltid/vc-adapter`) that routes by
  DID method: LDP_VC verified in-process via URDNA2015 + Ed25519, SD-JWT
  with x5c verified via certificate chain, JWT VCs delegated to walt.id
  OID4VP session flow. Designed to run with `--network none` for true
  air-gap demonstrations once issuer keys are pre-synced.
- Proactive holder-side sharing: `POST /api/share/proactive` stores the
  credential in an in-memory map keyed by a random share ID, returns a
  `/share/v/{id}` URL + PixelPass QR payload. The public share view runs
  direct-verify without requiring a wallet-api to have the credential in
  its own SQL store (the earlier bug where walt.id's `usePresentationRequest`
  couldn't present credentials surfaced from the shared walletbag).

### Trust / admin / audit workspaces

All feature-flagged and admin-gated. Handlers in `trust.go`, `admin.go`,
`portal.go`. Sixteen trust screens, eleven admin screens, shared audit
and activity views. Driven by the mock stores for now — the interfaces
(`store.SchemaStore`, `store.NotificationStore`, `store.AuditStore`) are
production-ready, but the mock implementations are what ship.

### Agent service and Outputs page

The chatbot is a same-origin partial on every vc.infra page that POSTs to
`/api/agent/chat` — a Go proxy that forwards to an n8n webhook. The n8n
workflow has nine HTTP Request nodes:

```
Webhook → Config → Router LLM → Parse Router → Embed Query →
Qdrant Search → Build Main Request → Main LLM → Respond
```

- **Router LLM**: Claude Haiku 4.5, temperature 0, one-word output
  identifying which persona should answer
- **Main LLM**: Claude Sonnet 4.6
- **Retrieval**: Voyage-3 embeddings → Qdrant vector search over the
  `references/` corpus (decks, advisory notes, scope docs, transcripts)
- **Output artifacts**: saved to `internal/handler/agent_outputs.go`'s
  in-memory store, rendered on the `/agent-output` page, editable inline,
  downloadable as `.md` or real CDPI-branded `.pptx` (via a custom Marp
  theme in `agent-service/marp/`)

The chatbot partial carries `lastPersona` back to the router on every
turn so persona stickiness works across messages. Chat state persists in
`sessionStorage` and syncs across tabs via `CustomEvent` + `BroadcastChannel`.

### Data sources

```go
// internal/datasource/datasource.go
type DataSource interface {
    Name() string
    DisplayName() string
    Kind() string
    Summary() string
    TotalRecords() int
    Fields() []FieldDescriptor
    SuggestedMappings() map[string]map[string]string
    FetchRecord(ctx context.Context, id string) (Record, error)
    Sample(ctx context.Context, limit int) ([]Record, error)
    Search(ctx context.Context, query string, limit int) ([]Record, error)
}
```

The Postgres implementation adds a `Search` method that runs ILIKE across
a configurable list of `SearchFields`. The default Citizens DB deployment
searches national_id / first_name / last_name / email / phone / student_id
/ farm_id / birth_registration_number. Used by the single-issuance
dropdown to let issuers auto-fill claims from a real backend instead of
typing by hand.

### Docker stack

One compose file: `ui-demo/docker/waltid/docker-compose.yml`. Services:

| Service | Purpose | Port |
|---|---|---|
| `postgres` | walt.id wallet-api database | 5432 |
| `caddy` | reverse proxy for walt.id | 7001-7003 |
| `issuer-api`, `verifier-api`, `wallet-api` | walt.id stack | 7001-7003 |
| `keycloak`, `wso2is`, `libretranslate` | auth + i18n sidecars | 8180, 9443, 5000 |
| `certify-postgres`, `inji-certify`, `certify-nginx` | Inji Certify issuer | 8090, 8091 |
| `inji-verify-postgres`, `inji-verify-service`, `inji-verify-ui` | Inji Verify | 3001, 5434, 8082 |
| `citizens-postgres` | mock government registry (200 KE+TT records) | 5435 |
| `vc-adapter` | backend-agnostic verification adapter | 8085 |
| `injiweb-postgres`, `injiweb-redis`, `injiweb-minio`, `injiweb-datashare` | Inji Web sidecars | — |
| `injiweb-mock-identity` | fake KYC (no real OTP) | 8083 |
| `injiweb-esignet` | OIDC / OID4VCI auth server (`mosipid/esignet-with-plugins:1.5.1`) | 8088 |
| `injiweb-oidc-ui` | esignet React login UI (`mosipid/oidc-ui:1.5.1`) | 3005 |
| `injiweb-mimoto` | Spring Boot BFF that runs OID4VCI client on behalf of the wallet | 8099 |
| `injiweb-ui` | Inji Web React SPA (`injistack/inji-web:0.16.0`) | 3004 |

Network pinned to `172.24.0.0/16` / gateway `172.24.0.1` so env vars
like `MOSIP_ESIGNET_DOMAIN_URL=http://172.24.0.1:3005` stay valid across
down/up cycles. Opt-in via `--profile injiweb`.

### Inji Web + Mimoto + esignet + Inji Certify end-to-end

This is the only **Authorization Code Flow** OID4VCI path in the stack
and it required rewiring several upstream MOSIP defaults. End-to-end
verified today: Inji Web catalog → authorize → mock-identity login →
code redirect → Mimoto token exchange → Inji Certify credential issue →
PDF delivered to the browser.

What was needed beyond upstream defaults:

1. **`inji-proxy` passthrough mode** (`internal/handler/inji_proxy.go`):
   the default mode strips `scope`, `display`, and `proof_types_supported`
   from Inji Certify's OID4VCI metadata to make walt.id's kotlinx parser
   happy. Mimoto's validator REQUIRES those three fields. Flipped the
   default to preserve everything; `?client=waltid` query opts back into
   stripping. Also rewrites `authorization_servers` in the metadata to
   point at the local esignet instance.

2. **Network-wide URL consistency**. Mimoto reads the token endpoint
   from esignet's well-known directly and ignores `proxy_token_endpoint`
   in the Mimoto issuers config. So the URL esignet advertises must
   resolve both from the browser and from inside the Mimoto container.
   `localhost:3005` breaks inside Mimoto (loops back to Mimoto itself).
   Solved with the pinned gateway IP `172.24.0.1:3005` plus
   `MOSIP_ESIGNET_DOMAIN_URL=http://172.24.0.1:3005`.

3. **Scope allowlist + resource mapping**. Esignet's shipped default
   scope is `mosip_identity_vc_ldp` but Inji Certify's Farmer credential
   declares `mock_identity_vc_ldp`. Added two SpEL-literal env overrides:
   `MOSIP_ESIGNET_SUPPORTED_CREDENTIAL_SCOPES={'mock_identity_vc_ldp'}`
   and `MOSIP_ESIGNET_CREDENTIAL_SCOPE_RESOURCE_MAPPING={'mock_identity_vc_ldp':'http://certify-nginx:80/v1/certify/issuance/credential'}`.
   The trailing `:80` matters — Nimbus's audience validator does strict
   string match.

4. **Inji Certify as resource server**. Upstream Inji Certify validates
   JWTs against its OWN JWKS (assumes it's the auth server). Overrode
   four env vars on `inji-certify`:
   ```
   mosip_certify_authorization_url=http://172.24.0.1:3005
   mosip_certify_authn_issuer_uri=http://172.24.0.1:3005/v1/esignet
   mosip_certify_authn_jwk_set_uri=http://172.24.0.1:3005/.well-known/jwks.json
   mosip_certify_oauth_issuer=http://172.24.0.1:3005/v1/esignet
   ```
   The `/v1/esignet` suffix on `issuer_uri` was a footgun — esignet sets
   the JWT `iss` claim to `${domain.url}${server.servlet.path}` even
   though its own well-known advertises `issuer: ${domain.url}` without
   the suffix. Upstream bug that we work around.

5. **Data provider plugin swap**. `PreAuthDataProviderPlugin` (upstream
   default for the Farmer credential) expects holder data to already be
   in certify's own transaction cache, which only gets populated during
   certify's own pre-auth flow. We use esignet's auth-code flow so the
   cache is always empty. Swapped to `MockCSVDataProviderPlugin` which
   reads from `farmer_identity_data.csv` keyed by the access token's
   `sub` claim.

6. **PSUT-keyed CSV row**. esignet derives a deterministic Partner-
   Specific User Token (PSUT) from `individualId + client_id`. For
   `8267411072 + wallet-demo-client` the PSUT is
   `J30I8ZKNdftz_gCBUyIMTcBJVXE2gG6No-XyyCe2Bkc`. Added that as the `id`
   column in a new CSV row with Demo Farmer values.

7. **nginx resolver pattern**. Both `injiweb-ui-nginx.conf` and
   `oidc-ui-nginx.conf` use `resolver 127.0.0.11 valid=10s ipv6=off;`
   plus `set $upstream <name>; proxy_pass http://$upstream;` so nginx
   re-resolves upstream hostnames per request — otherwise restarting
   Mimoto or esignet breaks the proxies until nginx is also restarted.

8. **Writable keystore volumes**. Esignet (and Inji Certify) write
   their PKCS12 master key into the container's writable layer by
   default. On recreation, the DB's `key_alias` table still holds the
   old aliases but the new keystore is empty → `KER-KMA-004 No such
   alias` crash. Added `injiweb-esignet-keystore` named volume at
   `/home/mosip/keystore` and set
   `MOSIP_KERNEL_KEYMANAGER_HSM_CONFIG_PATH=/home/mosip/keystore/esignet_local.p12`.
   Inji Certify's `certify-pkcs12` volume was already there but requires
   manual `TRUNCATE certify.key_alias, certify.key_store CASCADE` on
   recreate.

9. **Real wallet-demo-client p12**. The keystore is
   `docker/injiweb/config/certs/oidckeystore.p12` (password
   `xy4gh6swa2i`, alias `wallet-demo-client`, 4096-bit RSA, valid
   through 2029). A runtime copy at `certs-runtime/` is what the
   Mimoto container mounts writable — so Mimoto's keymanager can
   persist its own master-key aliases alongside the wallet-demo-client
   entry without mutating the pristine original.

10. **Seed scripts**. `docker/injiweb/seed-esignet-client.sh` extracts
    the public key from the p12 (with `-legacy` because the keystore
    uses RC2-40-CBC, disabled by default in OpenSSL 3), converts it to
    a JWK, and POSTs it to esignet's `/v1/esignet/client-mgmt/oidc-client`
    endpoint. `seed-mock-identity.sh` POSTs a fake identity
    (`8267411072 / 111111`) to mock-identity-system. Both are idempotent.

Nothing here is clean, but it all works end-to-end and the compose file
has block comments explaining every override.

---

## Repository layout

```
.
├── README.md                    — you are here
├── references/                  — RAG corpus for the agent service
│   ├── Architecture_By_building_blocks/
│   ├── Decks and conversations/
│   └── scope_docs/
├── agent-service/
│   ├── README.md
│   ├── docker-compose.agents.yml   — Qdrant, Voyage embeddings
│   ├── workflows/chat.template.json — n8n workflow
│   ├── prompts/                    — router + three persona system prompts
│   ├── ingest/                     — corpus ingestion scripts
│   ├── marp/                       — CDPI PPTX theme
│   ├── eval/                       — eval harness
│   └── scripts/
├── ui-demo/
│   ├── go.mod, go.sum
│   ├── Makefile
│   ├── Dockerfile
│   ├── docker-compose.yml          — repo-root compose (vcplatform app +
│   │                                 trimmed walt.id subset, via the
│   │                                 `mock` and `waltid` profiles). The
│   │                                 full 25-service stack lives under
│   │                                 docker/waltid/docker-compose.yml.
│   ├── implementation-plan.md      — full product spec, screen inventory
│   ├── dpg-integration-plan.md
│   ├── DEMO.md
│   ├── cmd/server/main.go          — entrypoint: config → stores → handler
│   ├── config/default.json         — brand, colors, feature flags, SSO
│   ├── internal/
│   │   ├── auth/                   — SSO registry (OIDC discovery)
│   │   ├── config/                 — YAML/JSON config loader
│   │   ├── datasource/             — DataSource interface + impls
│   │   │   ├── postgres/           — ILIKE search, suggested mappings
│   │   │   ├── csv/
│   │   │   ├── httpapi/
│   │   │   └── manual/
│   │   ├── handler/                — 22 handler files, all HTTP routes
│   │   │   ├── handler.go          — Handler struct, registry wiring, mux
│   │   │   ├── landing.go          — exploration pages
│   │   │   ├── auth.go             — /login, /signup, /logout, SSO callback
│   │   │   ├── portal.go           — portal dashboard + shared screens
│   │   │   ├── issuer.go           — issuer workspace (16 screens)
│   │   │   ├── schemas.go          — live schema catalog + custom publish
│   │   │   ├── bulk.go             — CSV + data-provider bulk issuance
│   │   │   ├── credtype.go         — credential type list + register
│   │   │   ├── holder.go           — wallet, claim, present, share, catalog
│   │   │   ├── verifier.go         — verifier workspace (10 screens)
│   │   │   ├── trust.go            — trust & interop workspace (16 screens)
│   │   │   ├── admin.go            — platform admin workspace (11 screens)
│   │   │   ├── onboarding.go       — role-aware onboarding wizard
│   │   │   ├── share.go            — proactive credential sharing
│   │   │   ├── inji_proxy.go       — OID4VCI metadata proxy + credential pass-through
│   │   │   ├── local_issuer.go     — in-process OID4VCI Pre-Auth server
│   │   │   ├── agent.go, agent_outputs.go — chatbot proxy + artifact store
│   │   │   ├── capabilities.go     — GET /api/capabilities
│   │   │   ├── translate.go        — DeepL i18n proxy
│   │   │   └── oidc.go             — OIDC helpers
│   │   ├── middleware/             — config inject, HTMX detect, auth, RBAC
│   │   ├── model/                  — User, WalletCredential, capabilities, etc.
│   │   ├── onboarding/             — OnboardingState + in-memory store
│   │   ├── render/                 — template registry, funcmap, tokens.css
│   │   └── store/
│   │       ├── store.go            — AuthStore, WalletStore, IssuerStore, VerifierStore, etc.
│   │       ├── waltid/             — walt.id wallet-api / issuer-api / verifier-api
│   │       ├── inji/               — Inji Certify issuer + Inji Verify verifier adaptors
│   │       ├── injiweb/            — Inji Web redirect wallet adaptor
│   │       ├── localholder/        — pure-Go OID4VCI Pre-Auth holder
│   │       ├── pdfwallet/          — PixelPass + gofpdf printable wallet
│   │       │   ├── pdfwallet.go
│   │       │   ├── base45.go       — RFC 9285
│   │       │   ├── pixelpass.go    — zlib + base45
│   │       │   ├── render.go       — PDF + QR rendering (best-effort EC levels)
│   │       │   └── pdfwallet_test.go
│   │       ├── credebl/            — beta stub
│   │       ├── adapter/            — backend-agnostic verifier adapter client
│   │       ├── ldpsigner/          — URDNA2015 + Ed25519Signature2020 in-process
│   │       ├── walletbag/          — shared in-memory credential bag
│   │       └── mock/               — mock impls for all stores
│   ├── web/
│   │   ├── embed.go                — //go:embed static templates schemas
│   │   ├── static/                 — vanilla CSS + HTMX + vendored qrcode
│   │   ├── templates/              — every screen (landing, portal, workspaces)
│   │   └── schemas/                — starter JSON schemas
│   └── docker/
│       ├── waltid/
│       │   ├── docker-compose.yml  — the real 25-service compose
│       │   ├── Caddyfile
│       │   ├── issuer-api/, verifier-api/, wallet-api/ — walt.id configs
│       │   ├── inji/certify/       — Inji Certify properties + CSV + init.sql
│       │   ├── inji/certify-nginx/ — nginx.conf that proxies through inji-proxy
│       │   ├── inji/verify/        — Inji Verify config
│       │   ├── citizens-db/init.sql — 200-row mock government registry
│       │   ├── vc-adapter/         — verification adapter backend config
│       │   ├── wso2-*, keycloak-*  — SSO sidecars
│       │   └── test-lifecycle.sh
│       └── injiweb/
│           ├── README.md
│           ├── fetch-config.sh     — pulls upstream Mimoto + esignet configs
│           ├── seed-esignet-client.sh — registers wallet-demo-client JWK
│           ├── seed-mock-identity.sh  — seeds fake identity 8267411072
│           └── config/             — Mimoto, esignet, nginx, data-share configs
└── *.html                       — original HTML mockups (phase1-6 + index)
```

---

## Running it locally

### Core stack (walt.id + Inji Certify + Inji Verify + vc-adapter)

```sh
cd ui-demo/docker/waltid
docker compose up -d
```

Brings up walt.id (3 services), Inji Certify (3 services), Inji Verify (3
services), Citizens Postgres, verification-adapter, Keycloak, WSO2IS,
LibreTranslate. ~15 containers.

Then start the Go app:

```sh
cd ui-demo
go build -o server ./cmd/server
./server -config config/default.json
```

Open http://localhost:8080.

### Full stack including Inji Web (Authorization Code Flow)

```sh
# one-time: pull upstream Mimoto + esignet config files
cd ui-demo/docker/injiweb
./fetch-config.sh

# bring up everything
cd ../waltid
docker compose --profile injiweb up -d

# wait ~90s for Mimoto + esignet to finish booting, then:
cd ../injiweb
./seed-esignet-client.sh     # registers wallet-demo-client in esignet
./seed-mock-identity.sh      # seeds fake identity 8267411072 / 111111
```

Open http://localhost:3004/issuers. Pick Agriculture Department, log in
with `8267411072` / `111111`, and you'll get a Demo Farmer credential
delivered as a PDF via the full OID4VCI Auth Code flow.

### Agent service (chatbot + RAG)

```sh
cd agent-service
docker compose -f docker-compose.agents.yml up -d   # Qdrant + Voyage
# import the n8n workflow, configure the Anthropic + Voyage credentials,
# then run the ingest script to populate Qdrant from ../references/
python scripts/build_and_import.py
```

See `agent-service/README.md` for full details.

---

## What is pending or fragile

### Not implemented yet
- **Authorization Code Flow OID4VCI client in vc.infra itself**. The
  current `localholder` adaptor only speaks Pre-Auth. To claim from
  Inji Certify via esignet without going through the Inji Web UI, we
  need a new wallet adaptor (e.g. `localholder_authcode`) that does the
  browser-redirect + PKCE + private_key_jwt flow Mimoto does. Planned
  as a follow-up since the protocol path is now proven working.
- **Walt.id Authorization Code Flow**. Walt.id's issuer-api natively
  supports OID4VCI Pre-Auth and we use it that way; Auth Code Flow
  against walt.id needs a separate integration path (walt.id is its
  own auth server, unlike Inji Certify which needs esignet).
- **Persistent storage for issued credentials**. `walletbag.Shared` is
  process-wide in-memory. Demo-grade. Production needs a real wallet
  store.
- **Trust registry, audit store, notification store**. Currently mock
  implementations. Interfaces are ready for real backends.
- **Real SSO in every flow**. WSO2IS and Keycloak containers are wired
  but the default config uses mock auth; the SSO code paths in
  `internal/auth/` work but aren't the default holder login.
- **Revocation** — schema and UI exist; DPG plumbing is partial.
- **Real i18n content** — DeepL proxy works, content strings are English-
  only.
- **Windows / Mac Docker Desktop testing**. The pinned subnet
  `172.24.0.0/16` and the `extra_hosts: host.docker.internal:host-gateway`
  pattern have only been exercised on Linux native Docker.

### Known fragile spots
- **Inji Certify keystore resets**. On recreating the `inji-certify`
  container the `certify.key_alias` + `certify.key_store` tables need
  to be truncated manually or the service crashes with `KER-KMA-004`.
  An `esignet-reset.sh` helper script would be worth adding.
- **Mock identity seed script + identity schema drift**. The mock-
  identity-system v0.10.1 requires every OIDC standard userinfo field
  (`givenName`, `middleName`, `nickName`, `preferredLang`, etc.). Future
  image upgrades may add more required fields; the seed script uses a
  hardcoded list. Upgrade playbook: bring up the container, POST once,
  read the validation errors, add missing fields, repeat.
- **`wallet-demo-client` p12** is the MOSIP standard demo keystore.
  Real deployments need to replace it with a client keystore onboarded
  against their own esignet. The seed script is the template.
- **`sub` → CSV lookup** for the Farmer credential uses a hardcoded
  deterministic PSUT. Any change to `client_id` or the salt derives a
  different PSUT and the CSV row won't match. A proper data provider
  plugin would do reverse lookup from PSUT to individualId, but that
  requires cooperation from esignet.
- **Go inji-proxy debug token logging** is gated by `INJI_PROXY_LOG_TOKEN=1`
  env var. Useful for debugging the Mimoto → Certify chain. Off by
  default, don't leave it on in anything resembling production — it
  prints Bearer tokens to stdout.

---

## Known quirks and gotchas

These are documented here so future-you doesn't have to rediscover them:

1. **Mimoto ignores `proxy_token_endpoint`** in `mimoto-issuers-config.json`
   and reads the token URL directly from esignet's
   oauth-authorization-server well-known. Setting `proxy_token_endpoint`
   in the issuer config does nothing at the token exchange step.

2. **esignet's JWT `iss` claim differs from its well-known `issuer` field.**
   The JWT iss is `${domain.url}${server.servlet.path}` (`.../v1/esignet`),
   the well-known advertises just `${domain.url}`. Consumers doing strict
   string validation must use the `/v1/esignet`-suffixed version.

3. **nginx `proxy_pass` with a variable** does NOT strip the matched
   location prefix. `location /v1/esignet { proxy_pass http://$up/v1/esignet; }`
   produces `/v1/esignet/v1/esignet/...` at the upstream. Either drop the
   path suffix from `proxy_pass` and let the URI forward verbatim, or use
   an explicit `rewrite ^ <target> break;`.

4. **Inji Web's Mimoto fetches `mimoto-issuers-config.json` from
   `http://inji-web:3004/...` at runtime** — the UI nginx serves it, not
   a local disk mount on the Mimoto container. The UI container name
   must be aliased as `inji-web` on the docker network, and the JSON
   file must be mounted on the UI container.

5. **esignet's shipped scope list is `mosip_identity_vc_ldp`** (note the
   `s`). Inji Certify's Farmer credential declares `mock_identity_vc_ldp`
   (note the `k`). These two upstream defaults are one letter apart and
   don't match; without the scope override, the Auth Code flow fails
   with "requested scope is not supported".

6. **MOSIP containers' HSM client install** calls `sudo apt install
   softhsm` at startup. Without `user: root` + `download_hsm_client=false`
   in the compose, the container exits with a sudo password prompt.

7. **Docker's embedded DNS caches stale IPs** for nginx when you use
   `proxy_pass http://hostname;` without a `resolver` directive. Every
   nginx that proxies to a container that might restart needs:
   ```nginx
   resolver 127.0.0.11 valid=10s ipv6=off;
   set $upstream <hostname>:<port>;
   proxy_pass http://$upstream;
   ```

8. **Walt.id wallet-api can't claim from Inji Certify** because walt.id's
   proof JWT includes an `iss` claim that Inji's OID4VCI draft-13
   validator rejects with `invalid_proof`. The `localholder` client
   avoids this by omitting `iss` and using only `kid` in the header
   (not `jwk`, which triggers `proof_header_ambiguous_key`).

9. **walt.id's wallet-api `CredentialOfferProcessor.kt:59`** crashes
   with `JsonObject is not a JsonPrimitive` when claiming LDP_VC offers.
   Documented as a known limitation — use the `localholder` client or
   the PDF wallet for LDP_VC.

10. **Inji Certify 0.14.0's `PreAuthDataProviderPlugin`** assumes
    certify is the OAuth AS. For Authorization Code flow via an external
    esignet, swap to `MockCSVDataProviderPlugin` which reads directly
    from the CSV keyed by the access token's `sub` claim.

---

## License and attribution

The `ui-demo/web/templates/` screens, the agent-service personas, and
the `references/` corpus are CDPI work. The upstream MOSIP Inji, MOSIP
esignet, walt.id, and Credebl components keep their original licenses.
The Go glue code, the docker compose wiring, and the fix chain
documented above are part of this repository.
