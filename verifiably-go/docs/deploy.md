# Deployment

`deploy.sh` is the single entrypoint. It wraps `docker compose` around
`deploy/compose/stack/docker-compose.yml` (the vendored MOSIP + walt.id
service definitions), layers a verifiably-go-specific override
(`deploy/docker-compose.injiweb-fix.yml`), and then runs the verifiably-go
image as a separate container on the same docker network.

`deploy/compose/` is self-contained — everything the compose file
needs (Caddyfile, Keycloak realm JSON, WSO2IS certs, Inji Certify data
CSVs, Mimoto bootstrap, oidc-ui nginx, seed scripts) lives there. The
tree preserves the sibling `stack/` + `injiweb/` layout the compose file
and seed scripts expect via relative paths.

## Subcommands

| Command                                    | Does                                                                  |
|--------------------------------------------|------------------------------------------------------------------------|
| `./deploy.sh up <all\|waltid\|inji>`       | Brings up every DPG container the scenario needs + seeds clients     |
| `./deploy.sh run <all\|waltid\|inji>`      | Builds + starts the verifiably-go container (run after `up`)          |
| `./deploy.sh down <all\|waltid\|inji>`     | Stops the verifiably-go container and the scenario's DPG services    |
| `./deploy.sh status`                       | Lists running compose services + verifiably-go container state       |
| `./deploy.sh config <all\|waltid\|inji>`   | Regenerates `config/backends.json` + prints it (no container touched)|

Typical first run: `./deploy.sh up all && ./deploy.sh run all`.

## Scenarios

| Scenario | DPG services                                                                  | IdPs (always both) | Translator |
|----------|-------------------------------------------------------------------------------|--------------------|------------|
| `all`    | walt.id (issuer/wallet/verifier) + Inji Certify + Inji Certify Preauth + Inji Verify + Inji Web + Mimoto + eSignet + mock-identity + certify-nginx + certify-preauth-nginx | Keycloak + WSO2IS | Yes |
| `waltid` | walt.id only                                                                  | Keycloak + WSO2IS  | Yes        |
| `inji`   | Inji Certify + Inji Verify + Inji Web + Mimoto + eSignet + mock-identity + certify-nginx + certify-preauth-nginx | Keycloak + WSO2IS  | Yes        |

`backends.json` is rendered per scenario so the UI never offers a DPG
whose backend isn't running. **Auth providers are not scoped**: every
scenario brings up both Keycloak and WSO2 Identity Server, and the
sign-in page always offers both. The scenario only decides which DPG
cards the user can pick from after auth. The WSO2IS OIDC client is
bootstrapped via `scripts/bootstrap-wso2is.sh` on every `deploy.sh up`,
regardless of scenario.

## Compose override pipeline

`deploy/docker-compose.injiweb-fix.yml` holds our additive compose
overlay. Relative paths inside an override get resolved against the
**primary** compose's directory, not the override's, so paths there use a
`{{VERIFIABLY_GO_DIR}}` placeholder that `deploy.sh` substitutes with the
absolute repo root at render time. The rendered file lands in
`config/docker-compose.injiweb-fix.rendered.yml`.

The override mounts:

- `deploy/injiweb-overrides/mimoto-bootstrap.properties` → `/home/mosip/mimoto-bootstrap.properties`
  Fixes a Mimoto crash loop: upstream points `spring.cloud.config.uri` at
  the Inji Web SPA (not a config server); Spring parses HTML, fails, and
  exits 1. The patched bootstrap disables Spring Cloud Config.

- `deploy/injiweb-overrides/mimoto-issuers-config.json` → both
  `injiweb-ui:/home/mosip/mimoto-issuers-config.json` and
  `certify-nginx:/config/server/mimoto-issuers-config.json`
  Fixes two upstream issues: the `wellknown_endpoint` pointed at a 404
  path, and Mimoto's `IssuerConfigUtil.getIssuerWellknown()` ignores the
  field entirely and instead appends a string to `credential_issuer_host`
  — so both fields need to contain `/v1/certify`.

- `deploy/injiweb-overrides/certify-nginx.conf` → `/etc/nginx/conf.d/default.conf`
  Routes `/.well-known/did.json` and `/v1/certify/credentials/status-list/`
  through the verifiably-go inji-proxy so we can patch kid fragments the
  upstream did.json misses. See
  [architecture.md § Inji-proxy](architecture.md#inji-proxy-didweb-resolver--credential-forwarder)
  for the why.

- `deploy/injiweb-overrides/inji-verify-config.json` → `/usr/share/nginx/html/assets/config.json`
  Inji Verify v0.16.0 ships without this render-order config; its UI
  `JSON.parse`s the fallthrough HTML, gets an empty object, and crashes
  with "Cannot read properties of undefined" on every successful
  verification.

## Seed scripts + repair helpers

After `docker compose up` completes, `cmd_up` runs:

- `deploy/compose/injiweb/seed-esignet-client.sh` — extracts the
  `wallet-demo-client` public key from its p12 keystore, converts to JWK,
  and POSTs to eSignet's client-mgmt API. Idempotent; re-runs return
  `duplicate_client_id` and exit 0.

- `deploy/compose/injiweb/seed-mock-identity.sh` — stuffs an identity
  (individualId `8267411072`, PIN `111111`) into mock-identity so the
  OTP login screen has something to authenticate.

- `scripts/bootstrap-wso2is.sh` — registers verifiably-go's OIDC client
  via DCR; writes `config/wso2is.env` so `backends_for_docker` can pick
  up the client_secret.

- `repair_injiweb_client_redirect_uri` (in deploy.sh) — ensures the
  eSignet client's `redirect_uris` list contains
  `http://${PUBLIC_HOST}:3004/redirect`. Needed because the seed script
  registers the client ONCE and treats duplicates as success; if a
  previous deploy used a different PUBLIC_HOST, eSignet rejects
  /authorize with `invalid_redirect_uri`. Repairs the DB row in place
  and deletes the Redis `clientdetails::wallet-demo-client` cache entry.

- `recover_injiweb` (before compose up) — force-removes any eSignet /
  mock-identity containers that might be stuck in a restart loop from a
  previous run's entrypoint + writable-layer state pollution.

## Environment variables

Every knob lives in a single file — `verifiably-go/.env` — which
`deploy.sh` sources at startup and passes to `docker compose` via
`--env-file`. Copy the template on first use:

```bash
cp verifiably-go/.env.example verifiably-go/.env
```

The template has every variable documented with sensible laptop defaults.
The key ones:

| Variable                       | Default       | Purpose                                                                   |
|--------------------------------|---------------|----------------------------------------------------------------------------|
| `VERIFIABLY_PUBLIC_HOST`       | `172.24.0.1`  | **The one knob to flip for EC2.** Host the browser uses for every service |
| `PUBLIC_HOST`                  | `${VERIFIABLY_PUBLIC_HOST}` | Alias read by the compose file + seed scripts           |
| `VERIFIABLY_HOST_PORT`         | `8080`        | Host port mapped to the verifiably-go container                            |
| `VERIFIABLY_PUBLIC_URL`        | `http://${VERIFIABLY_PUBLIC_HOST}:${VERIFIABLY_HOST_PORT}` | Shown on `deploy.sh run` |
| `VERIFIABLY_IMAGE`             | `verifiably-go:local` | Image tag for the build                                          |
| `VERIFIABLY_CONTAINER`         | `verifiably-go` | docker container name                                                  |
| `WALTID_{ISSUER,WALLET,VERIFIER}_PORT` | `7001/7002/7003` | Host ports for walt.id                                       |
| `CERTIFY_NGINX_PORT`           | `8091`        | Inji Certify nginx (auth-code flow)                                        |
| `CERTIFY_PREAUTH_PORT`         | `8094`        | Inji Certify pre-auth stanza                                               |
| `INJI_VERIFY_{UI,SERVICE}_PORT`| `3001/8082`   | Inji Verify                                                                |
| `INJIWEB_UI_PUBLIC_PORT`       | `3004`        | Inji Web SPA                                                               |
| `ESIGNET_PUBLIC_PORT`          | `3005`        | eSignet oidc-ui                                                            |
| `MIMOTO_PORT`                  | `8099`        | Mimoto BFF                                                                 |
| `KEYCLOAK_{PORT,REALM,CLIENT_ID}` | `8180/vcplatform/vcplatform` | Keycloak OIDC wiring                            |
| `WSO2_{PORT,CLIENT_ID,CLIENT_SECRET}` | `9443/verifiably_go_client/<generated>` | WSO2IS OIDC. `CLIENT_SECRET` populated by `scripts/bootstrap-wso2is.sh` on first up |
| `INJIWEB_P12_PASSWORD`         | `xy4gh6swa2i` | Matches the p12 in `deploy/compose/injiweb/config/certs/`                  |
| `INJI_PROXY_EXTRA_KIDS`        | _(empty)_     | Pre-seed kids for the PRIMARY (auth-code) inji-proxy did.json handler      |
| `INJI_PROXY_PREAUTH_EXTRA_KIDS`| _(empty)_     | Pre-seed kids for the PRE-AUTH inji-proxy did.json handler                 |
| `VERIFIABLY_DEBUG_MOCK_MARKERS`| `0`           | Show `[mock]` pills on surfaces still mock-backed                          |

Command-line override: set `VERIFIABLY_ENV_FILE=/path/to/other.env
./deploy.sh ...` to swap the entire file for one invocation (e.g. keep
`.env` pinned to laptop, ship `.env.ec2` for a staging run).

## Migrating to a remote host (EC2, dev VM, LAN)

One-variable flip:

```bash
# edit verifiably-go/.env
VERIFIABLY_PUBLIC_HOST=ec2-1-2-3-4.compute-1.amazonaws.com
```

That's it. The same `.env` drives every service — backends.json / auth-providers.json stanzas, the compose file's `${PUBLIC_HOST}` references (eSignet redirect, Mimoto's MIMOTO_URL injection, the patched Inji Web issuer catalog), and the eSignet client-redirect-URI repair helper.

Caveats:

- **TLS**: browsers reaching WSO2IS on `:9443` self-signed will need cert trust. Keycloak on `:8180` is HTTP so unaffected. For a public-facing EC2 you'd typically drop a Caddy / ALB in front and re-point `VERIFIABLY_PUBLIC_HOST` plus the port vars at your TLS terminator.
- **Firewall**: open `VERIFIABLY_HOST_PORT`, `KEYCLOAK_PORT`, `WSO2_PORT`, `CERTIFY_NGINX_PORT`, `INJIWEB_UI_PUBLIC_PORT`, `ESIGNET_PUBLIC_PORT`, `INJI_VERIFY_UI_PORT` on the instance — every one is visited from the browser.
- **Re-run `./deploy.sh up <scenario>`** after flipping PUBLIC_HOST so the `repair_injiweb_client_redirect_uri` helper re-registers the eSignet client's redirect URI on the new host. The helper is idempotent and only writes when the list diverges.

## Full reset

To start from a fully clean slate (wipe all keys, all registered clients,
all issued VCs):

```bash
./deploy.sh down all

docker compose -p waltid --profile injiweb \
  -f deploy/compose/stack/docker-compose.yml \
  rm -f -v certify-postgres inji-certify \
              certify-preauth-postgres inji-certify-preauth-backend inji-preauth-proxy \
              certify-nginx certify-preauth-nginx \
              injeweb-postgres injiweb-esignet \
              injiweb-mimoto injiweb-ui injiweb-oidc-ui \
              injiweb-mock-identity injiweb-datashare injiweb-minio \
              injiweb-redis

docker volume rm waltid_certify-db waltid_certify-pkcs12 \
                 waltid_certify-preauth-db waltid_certify-preauth-pkcs12 \
                 waltid_injiweb-db waltid_injiweb-esignet-keystore \
                 waltid_injiweb-mockid-keystore waltid_injiweb-minio

./deploy.sh up all && ./deploy.sh run all
```

This is what you want when Inji Certify's keys drift out of sync with
already-issued status-list credentials (see [dpg-matrix.md § Inji Certify](dpg-matrix.md#inji-certify-v0140)).

## Kubernetes / Helm

Every runtime piece already ships as a container, so porting to Helm is
mostly mechanical:

- The `host.docker.internal:host-gateway` trick the inji-proxy depends on
  becomes a proper Kubernetes `Service` DNS name — certify-nginx's
  upstream stanza points at `verifiably-go.<namespace>.svc` instead.
- Persistent docker volumes become `PersistentVolumeClaim`s.
- Seed scripts become `Job`s or `initContainer`s that gate the main
  services' readiness probes.
- Secrets (the `wallet-demo-client` p12 password, OIDC client secrets)
  move into `Secret` resources.
- The translator cache can stay inside the verifiably-go pod or move to
  a shared PVC if you scale horizontally.
