# Inji Web docker stack

Real [MOSIP Inji Web](https://github.com/inji/inji-web) running locally with
a full OIDC/OID4VCI backend:

| Container | Image | Purpose | Port |
|---|---|---|---|
| `injiweb-ui` | `injistack/inji-web:0.16.0` | Browser wallet UI | `3004` |
| `injiweb-mimoto` | `injistack/mimoto:0.21.0` | Spring Boot BFF | `8099` |
| `injiweb-esignet` | `mosipid/esignet-with-plugins:1.5.1` | OIDC + OID4VCI server | `8088` |
| `injiweb-mock-identity` | `mosipid/mock-identity-system:0.10.1` | Fake identity backend | `8082` |
| `injiweb-postgres` | `postgres:15` | `inji_mimoto` + `mosip_esignet` + `mosip_mockidentitysystem` | 5432 |
| `injiweb-redis` | `redis:7-alpine` | esignet cache | 6379 |
| `injiweb-datashare` | `mosipid/data-share-service:1.3.0-beta.2` | Credential delivery | 8097 |
| `injiweb-minio` | `minio/minio:latest` | Object store | 9000/9001 |

## Bring-up (first time)

```sh
cd docker/injiweb

# 1. Pull upstream config files (mimoto + esignet schemas, Spring properties,
#    issuer catalog template).
./fetch-config.sh

# 2. Copy the wallet-demo-client p12 into config/certs/ (already done in
#    this repo — alias wallet-demo-client, password xy4gh6swa2i).
ls config/certs/oidckeystore.p12

# 3. Start the stack.
docker compose --profile injiweb up -d

# 4. Wait for esignet + Mimoto to finish booting.
docker compose --profile injiweb logs -f injiweb-esignet injiweb-mimoto

# 5. Register wallet-demo-client with esignet. This extracts the public
#    key from the p12, converts it to a JWK, and POSTs it to esignet's
#    /v1/esignet/client-mgmt/oidc-client endpoint. Idempotent.
./seed-esignet-client.sh
```

Open <http://localhost:3004/issuers> in a browser. The issuer catalog is
loaded from `config/mimoto-issuers-config.json`. Pick an issuer, sign in
via mock-identity-system (any OTP ending in `111` for a seeded identity),
and Mimoto will drive the OID4VCI exchange against the issuer's credential
endpoint using `wallet-demo-client` for private_key_jwt auth.

## What it actually does

Inji Web is **catalog-initiated**. Holders don't paste credential offer
URLs into it — they pick an issuer from the catalog Mimoto loads out of
`config/mimoto-issuers-config.json`, authenticate via that issuer's
esignet/OIDC, and Mimoto runs the OID4VCI exchange on the holder's
behalf.

That means our Go wallet adaptor (`internal/store/injiweb`) cannot hand
Inji Web the offer URL produced by our issuer workspace. What it does
instead: redirect the holder to `http://localhost:3004/issuers`. They
re-pick the issuer inside Inji Web, complete the flow there, and the
credential lives in Mimoto (not in our shared walletbag).

## What's already set up for you

1. **`config/certs/oidckeystore.p12`** — real `wallet-demo-client`
   PKCS12 keystore (4096-bit RSA, password `xy4gh6swa2i`, alias
   `wallet-demo-client`, valid through 2029-11-14). This is the
   standard MOSIP demo wallet client, not a placeholder.

2. **Local esignet + mock-identity-system** — `injiweb-esignet` runs
   `mosipid/esignet-with-plugins:1.5.1` and talks to
   `injiweb-mock-identity` for fake KYC. `seed-esignet-client.sh`
   registers the p12's public key as an OIDC client so Mimoto can
   complete `private_key_jwt` token exchanges locally. No dependency
   on MOSIP's collab environment.

## What you still have to edit

`config/mimoto-issuers-config.json` is pulled from upstream and points
at MOSIP's collab env by default. To issue from your local walt.id or
inji-certify backends, edit the issuer entries so:

- `wellknown_endpoint` → your issuer's
  `/.well-known/openid-credential-issuer` URL
- `token_endpoint` → `http://localhost:3004/v1/mimoto/get-token/<issuer_id>`
  (Mimoto's own proxy — this is what the wallet UI calls)
- `proxy_token_endpoint` → `http://injiweb-esignet:8088/v1/esignet/oauth/v2/token`
  (where Mimoto POSTs the private_key_jwt)
- `authorization_audience` → same as `proxy_token_endpoint`
- `redirect_uri` → `http://localhost:3004/redirect`
- `client_id` → `wallet-demo-client`
- `client_alias` → `wallet-demo-client`

## Wiring our local issuers into Inji Web

To make Inji Web issue credentials from walt.id's issuer-api or
inji-certify, add entries to `mimoto-issuers-config.json` with:

- `wellknown_endpoint` pointing at the issuer's
  `/.well-known/openid-credential-issuer`
- `token_endpoint` / `proxy_token_endpoint` pointing at the
  corresponding esignet (walt.id doesn't ship an esignet — you'll need
  to stand one up or front the walt.id issuer with MOSIP esignet)
- `redirect_uri` set to `http://localhost:3004/redirect`
- `client_id` + `client_alias` matching the p12 keystore entry

The walt.id and certify-nginx containers are on the `waltid_default`
network; add Inji Web to the same network (or use the
`extra_hosts: - "host.docker.internal:host-gateway"` trick) so Mimoto
can reach them.
