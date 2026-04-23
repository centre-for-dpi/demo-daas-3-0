# verifiably

A thin, backend-agnostic Go + HTMX UI for issuing, holding, and verifying
W3C Verifiable Credentials against real DPG (Digital Public Goods) stacks.
One interface (`backend.Adapter`) drives every screen; swap implementations
to point at a different vendor without touching the UI.

The app and its deploy tooling live under [`verifiably-go/`](verifiably-go/).
Everything below refers to that subtree — run the commands from there.

Supported DPGs out of the box:

- **walt.id Community Stack** v0.18.2 — issuer / holder / verifier via walt.id's issuer-api, wallet-api, verifier-api
- **Inji Certify** v0.14.0 — issuer, both OID4VCI pre-authorised code and authorization code flows
- **Inji Web Wallet** v0.16.0 — holder via the MOSIP Inji Web SPA + Mimoto BFF
- **Inji Verify** v0.16.0 — verifier via Inji Verify's QR-upload and OID4VP endpoints

Plus OIDC sign-in via **Keycloak** or **WSO2 Identity Server**, and
app-wide translation via **LibreTranslate** (English / French / Spanish).

## Quickstart

Requires Docker. Everything else — Go, walt.id, MOSIP containers, IdPs, the
translator, Postgres, Redis, MinIO — runs in compose.

```bash
cd verifiably-go
cp .env.example .env     # first time only — defaults target localhost
./deploy.sh up  all      # start every service for every DPG
./deploy.sh run all      # build + launch the verifiably-go container

# point a browser at:
http://localhost:8080
```

### Deploying somewhere other than localhost (EC2, bare-metal demo box, …)

There is **exactly one variable** to change. Edit `.env` before running
`deploy.sh` and set `VERIFIABLY_PUBLIC_HOST` to the hostname the browser
will reach the services on:

```bash
# Laptop (default):
VERIFIABLY_PUBLIC_HOST=172.24.0.1

# EC2 / remote host:
VERIFIABLY_PUBLIC_HOST=ec2-1-2-3-4.compute-1.amazonaws.com
```

Everything downstream — `backends.json` browser-facing URLs, Mimoto's
OIDC redirect_uris, Keycloak/WSO2IS issuer URLs, eSignet redirects, the
Caddy virtualhost — is derived by substituting `${VERIFIABLY_PUBLIC_HOST}`
inside `deploy.sh`, so nothing else needs hand-editing.

Then the usual `./deploy.sh up all && ./deploy.sh run all` stands the
stack up at `http://${VERIFIABLY_PUBLIC_HOST}:8080`. Full variable
reference + TLS / proxy notes in [`verifiably-go/docs/deploy.md`](verifiably-go/docs/deploy.md).

### Scenarios

`deploy.sh` supports three scenarios so you don't have to boot everything
when you only care about one stack. Every scenario includes **both**
Keycloak and WSO2 Identity Server so the sign-in page always offers both
OIDC providers; the scenario only gates which DPG backends come up.

| Scenario     | DPG services                                   | IdPs (always both)  | Translator |
|--------------|------------------------------------------------|---------------------|------------|
| `all`        | walt.id + Inji Certify + Inji Web + Inji Verify| Keycloak + WSO2IS   | Yes        |
| `waltid`     | walt.id Community Stack                        | Keycloak + WSO2IS   | Yes        |
| `inji`       | Inji Certify + Inji Web + Inji Verify          | Keycloak + WSO2IS   | Yes        |

Usage is the same pattern: `./deploy.sh <up|run|down|status|config> <scenario>`.

### Credentials for demo flows

- **Keycloak login**: any user in the `vcplatform` realm (seeded by the shared compose's `keycloak-realm.json`)
- **WSO2IS login**: any user you register in WSO2's console at `https://localhost:9443/console` (default admin `admin` / `admin`)
- **eSignet login** (Inji Web holder flow only): individual ID `8267411072`, PIN/OTP `111111`

### Stopping

```bash
./deploy.sh down all
```

Persistent docker volumes (eSignet DB, Inji Certify keystore, walt.id wallet
DB) are preserved between runs. To start from a fully clean slate, remove
the project volumes with `docker volume rm waltid_<name>` — see
[verifiably-go/docs/deploy.md](verifiably-go/docs/deploy.md#full-reset) for
the exact list.

## What this app does

Each of the three core roles has a dedicated flow:

**Issuer** — pick a DPG (capability-aware cards so you only see what that
vendor can do) → pick or build a schema → pick flow mode (auth-code vs
pre-authorized-code for Inji; always pre-auth for walt.id) → enter one
subject or upload a bulk CSV → get back a real OID4VCI offer URI + QR
code, or for Inji a printable PDF with an embedded status-list-ready QR.

**Holder** — pick a wallet DPG → scan, paste, or select an example offer →
review the pending offer → accept it into the wallet → present it to a
verifier via QR, OID4VP link, or direct upload.

**Verifier** — pick a verifier DPG → either request an OID4VP presentation
from a template (signed request JWT + QR for cross-device) or upload a VC
directly (paste JSON-LD, paste SD-JWT compact, or upload a QR image). Get
back signature verification, DID resolution, revocation status, and the
fields actually disclosed.

All user-facing text is translated on the fly when you switch language in
the top bar — both the static template strings and dynamic text coming
from DPG responses.

## Where to look next

- **[verifiably-go/docs/architecture.md](verifiably-go/docs/architecture.md)**
  — package layout, adapter interface, registry routing, HTMX patterns,
  translation middleware, and the inji-proxy that bridges walt.id / Mimoto /
  Inji Verify quirks.

- **[verifiably-go/docs/deploy.md](verifiably-go/docs/deploy.md)** —
  deploy.sh walkthrough per scenario, compose overrides, seed scripts,
  database-repair helpers, environment variables, and migrating from
  localhost to an EC2 instance.

- **[verifiably-go/docs/dpg-matrix.md](verifiably-go/docs/dpg-matrix.md)**
  — per-DPG capability matrix, known upstream bugs we work around (Inji
  Certify kid mismatch, Inji Verify UI render-order config, Inji Web
  PUBLIC_HOST coupling), version-compatibility caveats.

- **[verifiably-go/docs/integration.md](verifiably-go/docs/integration.md)**
  — adapter-to-endpoint mapping per DPG, how to swap `MockAdapter` for a
  real implementation, how authenticated requests flow through the OIDC
  providers.

## License

See repository root.
