# Deployment

`deploy.sh` is the single entrypoint. It wraps `docker compose` around the
shared `ui-demo/docker/stack/docker-compose.yml` (which defines every DPG
container), layers a verifiably-go-specific override
(`deploy/docker-compose.injiweb-fix.yml`), and then runs the verifiably-go
image as a separate container on the same docker network.

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

| Scenario | DPG services                                                                  | IdPs              | Translator |
|----------|-------------------------------------------------------------------------------|-------------------|------------|
| `all`    | walt.id (issuer/wallet/verifier) + Inji Certify + Inji Certify Preauth + Inji Verify + Inji Web + Mimoto + eSignet + mock-identity + certify-nginx | Keycloak + WSO2IS | Yes        |
| `waltid` | walt.id only                                                                  | Keycloak          | Yes        |
| `inji`   | Inji Certify + Inji Verify + Inji Web + Mimoto + eSignet + mock-identity + certify-nginx | WSO2IS            | Yes        |

`backends.json` is rendered per scenario so the UI never offers a DPG
whose backend isn't running. Auth providers are scoped the same way —
`waltid` doesn't carry a WSO2IS stanza, `inji` doesn't carry Keycloak.

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

- `ui-demo/docker/injiweb/seed-esignet-client.sh` — extracts the
  `wallet-demo-client` public key from its p12 keystore, converts to JWK,
  and POSTs to eSignet's client-mgmt API. Idempotent; re-runs return
  `duplicate_client_id` and exit 0.

- `ui-demo/docker/injiweb/seed-mock-identity.sh` — stuffs an identity
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

Set before invoking `deploy.sh` or export in your shell:

| Variable                          | Default                   | Purpose                                                  |
|-----------------------------------|---------------------------|----------------------------------------------------------|
| `VERIFIABLY_ADDR`                 | `:8080`                   | Go server bind address inside the container              |
| `VERIFIABLY_HOST_PORT`            | `8080`                    | Host port mapped to the verifiably-go container          |
| `VERIFIABLY_PUBLIC_URL`           | `http://localhost:8080`   | Shown to the user after `deploy.sh run`                  |
| `VERIFIABLY_IMAGE`                | `verifiably-go:local`     | Image tag for the build                                  |
| `VERIFIABLY_CONTAINER`            | `verifiably-go`           | docker container name                                    |
| `VERIFIABLY_COMPOSE_FILE`         | `../ui-demo/docker/stack/docker-compose.yml` | Primary compose file             |
| `VERIFIABLY_COMPOSE_OVERRIDE`     | `deploy/docker-compose.injiweb-fix.yml`      | Override file                    |
| `VERIFIABLY_INJI_EXTRA_KIDS`      | _(empty)_                 | Pre-seed kids for the inji-proxy did.json handler — comma-separated list to use when restarting without re-issuance |
| `VERIFIABLY_DEBUG_MOCK_MARKERS`   | `0`                       | Show `[mock]` pills on any UI surface still mock-backed   |

The shared compose file also reads its own `.env` next to it
(`ui-demo/docker/stack/.env`) which sets `PUBLIC_HOST=172.24.0.1` and
`ESIGNET_PUBLIC_PORT=3005` / `INJIWEB_UI_PUBLIC_PORT=3004`. Those drive
the URLs injected into the Inji Web SPA's `env.config.js` and the
eSignet client's redirect URIs.

## Migrating to EC2 / non-localhost

Today the server-side URLs hardcoded in `deploy.sh` stanzas assume
`localhost`. Migrating to EC2 (or any public host) touches three files
and one env var:

1. `ui-demo/docker/stack/.env` — set `PUBLIC_HOST=ec2-…`. This
   immediately flows into Mimoto's `MIMOTO_URL`, eSignet's configured
   redirect, and the patched `mimoto-issuers-config.json`.

2. `deploy.sh` — replace the `localhost` occurrences in the backend and
   auth-provider stanzas with `$VERIFIABLY_PUBLIC_HOST`, source a local
   `.env` at the top. This is a straightforward sed job.

3. `repair_injiweb_client_redirect_uri` uses `$PUBLIC_HOST` already —
   needs no change.

4. TLS: browsers reaching WSO2IS on `:9443` with a self-signed cert will
   need cert trust (or swap WSO2 for a dev cert). Keycloak on `:8180`
   runs HTTP so is unaffected.

A `.env`-driven config is on the backlog — see the issue list.

## Full reset

To start from a fully clean slate (wipe all keys, all registered clients,
all issued VCs):

```bash
./deploy.sh down all

docker compose -p waltid --profile injiweb \
  -f ../ui-demo/docker/stack/docker-compose.yml \
  rm -f -v certify-postgres inji-certify inji-certify-preauth \
              certify-nginx injeweb-postgres injiweb-esignet \
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
