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

## Prerequisites

Before the quickstart will succeed on a fresh machine:

| requirement | why it matters |
|---|---|
| **Docker Engine 24+** with Compose v2 (`docker compose`, not `docker-compose`) | Every DPG + IdP + translator runs as a container; the compose file uses v2-only features. |
| **Non-root Docker access** — your user is in the `docker` group | `deploy.sh` invokes `docker` without `sudo`. `sudo usermod -aG docker "$USER" && newgrp docker` once. Verify with `docker ps` (no sudo). |
| **~8 GB RAM free** for `./deploy.sh up all` | The full stack runs ~25 containers, including a JVM-heavy WSO2IS (~1 GB) and MOSIP services. `waltid` / `inji` scenarios are much lighter. |
| **Ports free** on the host: 80, 443, 3001, 3004, 3005, 5432–5437, 7001–7003, 8080, 8090–8099, 8180, 8182, 9443 | Compose publishes each DPG on its canonical port. Check `sudo ss -ltn` before starting; `lsof -i :8080` to find who's holding any conflict. |
| **`envsubst`** (part of `gettext`) in your `$PATH` | `deploy.sh` renders `wso2-deployment.toml` from a template with it. Most Linux distros have it preinstalled; on macOS: `brew install gettext` + `brew link --force gettext`. |
| **Go 1.25+** *(optional)* | Only needed if you want to run `verifiably-go` outside docker via `go run ./cmd/server`. `./deploy.sh run` builds its own container image. |
| **`curl`, `jq`, `python3`** | `deploy.sh` and the WSO2 bootstrap use them for seeding OIDC clients + rendering configs. |

Docker Hub has the two first-party images the stack pulls:

- [`adammwaniki/inji-preauth-poc`](https://hub.docker.com/r/adammwaniki/inji-preauth-poc) — a tiny reverse proxy that patches two Inji Certify pre-auth OID4VCI endpoints so wallets outside the container network work. Source: [`verifiably-go/cmd/inji-preauth-proxy`](verifiably-go/cmd/inji-preauth-proxy).
- [`adammwaniki/verification-adapter`](https://hub.docker.com/r/adammwaniki/verification-adapter) — standalone backend-agnostic verifier showcase on port 8085; referenced by the `inji` / `all` scenarios as a parallel demo (verifiably-go itself doesn't call it).

All other containers pull from their vendors' official Docker Hub repos.

## Quickstart

Clone, set your deployment's public hostname, bring it up:

```bash
git clone https://github.com/centre-for-dpi/demo-daas-3-0.git
cd demo-daas-3-0/verifiably-go

cp .env.example .env     # defaults target localhost
# If you're deploying to something other than localhost, edit
# VERIFIABLY_PUBLIC_HOST in .env now — see the next section.

./deploy.sh up  all      # pull images + start every DPG container
./deploy.sh run all      # build + launch the verifiably-go container
```

First `up` takes 5–15 minutes on a fast connection (MOSIP images are large).
Subsequent runs are seconds. When it's done, point a browser at:

```
http://localhost:8080
```

You should see the role-picker landing page. Click **Issuer**, log in as
`holder` / `holder` via the Keycloak button, and you're in.

### Deploying somewhere other than localhost (EC2, bare-metal demo box, …)

There is **exactly one variable** to change. Edit `.env` *before* running
`deploy.sh up` and set `VERIFIABLY_PUBLIC_HOST` to the hostname the
browser will reach the services on:

```ini
# Laptop (default in .env.example):
VERIFIABLY_PUBLIC_HOST=172.24.0.1

# EC2 / remote host:
VERIFIABLY_PUBLIC_HOST=ec2-1-2-3-4.compute-1.amazonaws.com
```

Everything downstream — `backends.json` browser-facing URLs, Mimoto's
OIDC redirect_uris, Keycloak/WSO2IS issuer URLs, eSignet redirects, the
WSO2 `hostname` in `wso2-deployment.toml` — is derived by substituting
`${VERIFIABLY_PUBLIC_HOST}` inside `deploy.sh`, so nothing else needs
hand-editing.

Then `./deploy.sh up all && ./deploy.sh run all` stands the stack up at
`http://${VERIFIABLY_PUBLIC_HOST}:8080`. Full variable reference + TLS /
proxy notes in
[`verifiably-go/docs/deploy.md`](verifiably-go/docs/deploy.md).

### Scenarios

`deploy.sh` supports three scenarios so you don't have to boot
everything when you only care about one stack. Every scenario includes
**both** Keycloak and WSO2 Identity Server so the sign-in page always
offers both OIDC providers; the scenario only gates which DPG backends
come up.

| Scenario     | DPG services                                   | IdPs (always both)  | RAM  |
|--------------|------------------------------------------------|---------------------|------|
| `all`        | walt.id + Inji Certify + Inji Web + Inji Verify | Keycloak + WSO2IS  | ~8 GB |
| `waltid`     | walt.id Community Stack                        | Keycloak + WSO2IS   | ~2 GB |
| `inji`       | Inji Certify + Inji Web + Inji Verify          | Keycloak + WSO2IS   | ~5 GB |

Usage is the same pattern: `./deploy.sh <up|run|down|status|config> <scenario>`.

### Credentials for demo flows

Pre-seeded for every fresh `./deploy.sh up`:

| provider | username | password | lives in |
|---|---|---|---|
| Keycloak realm `vcplatform` | `holder` | `holder` | `deploy/compose/stack/keycloak-realm.json` |
| Keycloak realm `vcplatform` | `issuer` | `issuer` | same |
| Keycloak realm `vcplatform` | `admin`  | `admin`  | same |
| WSO2IS master (admin console at `https://<host>:9443/console`) | `admin` | `admin` | WSO2IS stock defaults |
| eSignet mock-identity (Inji Web holder flow only) | Individual ID `8267411072` | PIN/OTP `111111` | `deploy/compose/injiweb/` mock-identity seed |

WSO2IS doesn't seed app users automatically — for the WSO2IS login
button you either register a user at `https://<host>:9443/console` or
stick with Keycloak, which does come pre-seeded.

### Stopping

```bash
./deploy.sh down all
```

Persistent docker volumes (eSignet DB, Inji Certify keystore, walt.id
wallet DB) are preserved between runs. To start from a fully clean
slate, remove the project volumes with `docker volume rm waltid_<name>` —
see [verifiably-go/docs/deploy.md](verifiably-go/docs/deploy.md#full-reset)
for the exact list.

## Troubleshooting

Common things that trip up a first deploy:

**`pull access denied for verification-adapter-adapter` / `pull access denied for inji-preauth-proxy`**

You're on an older checkout that still references image tags that only
exist locally on a contributor's laptop. `git pull` to the latest `main`
and re-run `./deploy.sh up <scenario>`. The current `docker-compose.yml`
pulls from `adammwaniki/verification-adapter` and `adammwaniki/inji-preauth-poc`
on Docker Hub, both public.

**Keycloak redirects with "Invalid parameter: redirect_uri"**

The realm JSON accepts localhost, `172.24.0.1`, any `*.amazonaws.com`
host, and any `http://*:8080/*` — enough to cover the laptop + EC2
cases. If you're deploying to a custom domain, add it to
`deploy/compose/stack/keycloak-realm.json` under `clients[0].redirectUris`
and `docker compose up -d --force-recreate keycloak`.

**WSO2 redirects to `localhost:9443/authenticationendpoint/login.do`
(and the browser can't resolve localhost)**

WSO2's `deployment.toml` is templated from `.template` using
`VERIFIABLY_PUBLIC_HOST`. If you edited `.env` *after* the first
`./deploy.sh up`, re-run `./deploy.sh up all` — it'll regenerate the toml
and recreate the wso2is container. Confirm with:

```bash
curl -sk https://<your-host>:9443/oauth2/token/.well-known/openid-configuration \
  | jq .issuer
# should echo your host, NOT "localhost"
```

**`./deploy.sh` calls `docker` and gets "permission denied"**

Your user isn't in the `docker` group. Fix once:
```bash
sudo usermod -aG docker "$USER"
newgrp docker     # activate in current shell, or log out and back in
docker ps         # should work without sudo
```

**Port conflicts**

`sudo ss -ltn` lists listening ports. Common collisions:
- 5432 (Postgres) — another postgres on the host
- 8080 — any other web app
- 8099 — Tomcat default on some Linux distros

Either stop the conflicting service or change the port in `.env` (e.g.
`VERIFIABLY_HOST_PORT=8081`). Ports 80/443 are only needed if you bring
up the Caddy TLS reverse proxy; skip it for localhost dev.

## What this app does

Each of the three core roles has a dedicated flow:

**Issuer** — pick a DPG (capability-aware cards so you only see what
that vendor can do) → pick or build a schema → pick flow mode (auth-code
vs pre-authorized-code for Inji; always pre-auth for walt.id) → enter
one subject or upload a bulk CSV → get back a real OID4VCI offer URI +
QR code, or for Inji a printable PDF with an embedded status-list-ready
QR.

**Holder** — pick a wallet DPG → scan, paste, or select an example
offer → review the pending offer → accept it into the wallet → present
it to a verifier via QR, OID4VP link, or direct upload.

**Verifier** — pick a verifier DPG → either request an OID4VP
presentation from a template (signed request JWT + QR for cross-device)
or upload a VC directly (paste JSON-LD, paste SD-JWT compact, or upload
a QR image). Get back signature verification, DID resolution, revocation
status, and the fields actually disclosed.

All user-facing text is translated on the fly when you switch language
in the top bar — both the static template strings and dynamic text
coming from DPG responses.

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

- **[verifiably-go/testdata/bulk-issuance/README.md](verifiably-go/testdata/bulk-issuance/README.md)**
  — copy-paste recipes for the bulk-issuance feature (CSV fixtures,
  SELECT queries against the seeded `citizens` postgres, and a dockerized
  "ministry registry" scenario to exercise the Secured-API bulk source).

## License

See repository root.
