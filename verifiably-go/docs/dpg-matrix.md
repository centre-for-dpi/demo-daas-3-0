# DPG compatibility matrix

Capability claims in the UI reflect specific documented releases. These
versions are **not a tested-compatible matrix** — each vendor publishes
its own compatibility table. `cross-stack` scenarios (walt.id issues, Inji
verifies, etc.) work on a case-by-case basis; known breakage is called
out below with the workaround the verifiably-go inji-proxy applies.

## Versions

| Component                 | Version  | Source                                                                 |
|---------------------------|----------|------------------------------------------------------------------------|
| walt.id Community Stack   | v0.18.2  | `docker.io/waltid/issuer-api`, `wallet-api`, `verifier-api`            |
| Inji Certify              | v0.14.0  | `docker.io/mosipid/inji-certify:0.14.0`                                 |
| Inji Certify Preauth      | v0.14.0  | Same image, different config for pre-auth flow                         |
| Inji Web (Mimoto + SPA)   | v0.16.0  | `mosipid/inji-web-ui:0.16.0`, `mosipid/mimoto:0.16.0`                  |
| Inji Verify (UI + service)| v0.16.0  | `mosipid/inji-verify-ui:0.16.0`, `mosipid/inji-verify-service:0.16.0`  |
| eSignet + mock-identity   | v1.5.x   | `mosipid/esignet`, `mosipid/mock-identity-system`                      |
| Keycloak                  | 25.x     | `quay.io/keycloak/keycloak`                                            |
| WSO2 Identity Server      | 7.0.0    | `docker.wso2.com/wso2is`                                                |
| LibreTranslate            | latest   | `libretranslate/libretranslate`                                         |

## walt.id Community Stack v0.18.2

**What works end-to-end**

- OID4VCI pre-authorized code flow (issuer → wallet → hold)
- OID4VCI authorization code flow (issuer → eSignet-like auth → wallet)
- Legacy OID4VP (Presentation Exchange 2.0)
- Credential formats: `w3c_vcdm_2` (JWT), `sd_jwt_vc` (IETF), `mso_mdoc`

**Known limitations**

- **OID4VP v1.0** is still landing in the wallet/demo apps through v0.18.2.
  Our verifier adapter uses the PE 2.0 path; switching to v1.0 means
  redoing `RequestPresentation` and `FetchPresentationResult` against
  `/openid4vc/v1/*` endpoints when they're available.
- No documented QR-on-PDF export path. `IssueAsPDF` falls back to `DirectPDF: false`.
- The Kotlin wallet's OID4VCI client strips `credential_definition.@context`
  from credential requests. When using walt.id wallet against Inji Certify,
  our `/inji-proxy/issuance/credential` handler injects a sensible default
  (`https://www.w3.org/ns/credentials/v2`).

## Inji Certify v0.14.0

**What works**

- Issuance via both OID4VCI pre-authorized code (demo/staging) and
  authorization code (production) flows.
- Ed25519 signing; keys managed by MOSIP's keymanagerservice and stored
  in `certify.key_store`.

**Known bugs we work around**

1. **Split-kid VC vs status-list signing.** Inji Certify signs regular VCs
   with one kid fragment and bitstring status-list credentials with a
   different kid, both derived from the same Ed25519 key. Its own
   `.well-known/did.json` publishes only one. Inji Verify's
   `DidWebPublicKeyResolver` matches kid strictly, so verification either
   of the main VC or the status list (it needs both) fails with
   `PublicKeyResolutionFailedException` → the UI surfaces "Internal
   Server Error".

   **Workaround**: certify-nginx's `/.well-known/did.json` is rerouted
   through `verifiably-go:/inji-proxy/.well-known/did.json`. The proxy
   watches every VC it forwards for `proof.verificationMethod` kids and
   publishes all observed kids as synthetic `verificationMethod`
   entries — all pointing at the upstream `publicKeyMultibase`. The
   `/v1/certify/credentials/status-list/` endpoint is also proxied so
   status-list kids get recorded.

2. **Key rotation desyncs status-list signatures.** If the Ed25519 key
   rotates (any compose reset that wipes `waltid_certify-pkcs12`) but
   the status-list credential row survives (`waltid_certify-db`), every
   previously-issued VC fails verification because its status-list can't
   verify against the new key. Symptom: `STATUS_VERIFICATION_ERROR -
   Invalid signature on status list VC`.

   **Workaround**: for local dev, do a full reset (see
   [deploy.md § Full reset](deploy.md#full-reset)) so keys and the
   status-list table regenerate together. In production, key rotation
   would require the issuer to re-sign the status-list VCs.

3. **Credential-issuance endpoint proxies back to us.** `certify-nginx`
   routes `POST /v1/certify/issuance/credential` through
   `host.docker.internal:8080/inji-proxy/issuance/credential`. Without
   our handler registering that route, Mimoto's credential download
   gets a 404 and Inji Web shows "An Error Occurred — unable to
   download the card". Our handler forwards to `inji-certify:8090`,
   optionally patching `credential_definition.@context`.

4. **mDoc (ISO 18013-5) issuance is mock-only** per Inji Certify's own
   README at v0.14.0. Our `backends.json` strips mDoc from the Farmer
   catalog; the UI shows only the two LDP formats and SD-JWT.

5. **Two key aliases in `certify.key_alias`** for `CERTIFY_VC_SIGN_ED25519`
   (one with ref_id `ED25519_SIGN`, one with ref_id NULL). Only the
   former has a private key in `key_store`; the latter acts as a
   key-encryption-key wrapper. No action needed, but worth noting if you
   debug the DB.

## Inji Web Wallet v0.16.0

**What works**

- Guest + Google OIDC login
- Browser-hosted wallet (credentials live inside the SPA / Mimoto DB)
- PDF export with embedded QR for JSON-LD VCs (pixelpass compression)

**Known bugs we work around**

1. **`MIMOTO_URL` hardcodes `${PUBLIC_HOST}:3004`.** Injected at container
   start into `env.config.js`. If the browser loads the SPA on a
   different origin (e.g. `localhost:3004`) every `/v1/mimoto/*` XHR is
   cross-origin and browsers block the responses → UI falls back to "No
   Credentials found" even when Mimoto is healthy.

   **Workaround**: `UIURL` in verifiably-go's backends.json points at
   `http://172.24.0.1:3004` (matching `PUBLIC_HOST` in the shared `.env`),
   so the redirect lands on the SPA's configured origin.

2. **eSignet DB caches stale `redirect_uris`.** `seed-esignet-client.sh`
   returns OK on `duplicate_client_id` but doesn't update — so if a
   previous deploy used a different PUBLIC_HOST, eSignet rejects
   /authorize with `invalid_redirect_uri`. The client's redirects are
   also cached in Redis.

   **Workaround**: `repair_injiweb_client_redirect_uri` in deploy.sh
   appends the current redirect to the DB list and `DEL`s the Redis
   `clientdetails::wallet-demo-client` cache. Idempotent.

3. **SD-JWT credentials don't get a QR on PDF export.** Mimoto's pixelpass
   library is designed for structured JSON-LD VCs (CBOR-serialize
   credentialSubject → zlib → base45); SD-JWT is already a compact JWS
   string so the pipeline doesn't apply. v0.16.0 ships no alternative
   SD-JWT QR path. **Not fixed** — pick LDP Farmer (V2 or plain) if you
   need a QR; SD-JWT credential is storage-only.

## Inji Verify v0.16.0

**What works**

- Direct upload / paste of JSON-LD VCs → `POST /v1/verify/vc-verification`
- SD-JWT VC submission (v0.16.0 added `POST /vc-submission` +
  `GET /vp-result/{transactionId}`)
- OID4VP cross-device flow via the Inji Verify SPA (full flow)

**Known bugs we work around**

1. **Missing `/assets/config.json` crashes the result screen.** The UI
   fetches this file at boot for its per-credential field render-order
   map. Upstream v0.16.0 ships without it, so nginx 404s fall through to
   `index.html`, the UI `JSON.parse`s HTML, gets an empty object, then
   crashes with `Cannot read properties of undefined (reading 'map')`
   after every successful verification.

   **Workaround**: we mount
   `deploy/injiweb-overrides/inji-verify-config.json` into the
   inji-verify-ui container at `/usr/share/nginx/html/assets/config.json`
   with render orders for the Farmer credential (and stubs for the
   other types the UI's switch covers).

2. **INJIVER-1131** (cross-device): Inji Verify v0.16.0 can report
   SUCCESS for a VC whose claims don't match the requested fields in a
   presentation definition. Our `injiverify` adapter re-checks the
   disclosed claims against the requested fields and downgrades the
   verdict if they don't match. This is flagged as `Caveats` on the
   Inji Verify DPG card.

3. **Tested-compatibility matrix**: per MOSIP's own release notes, Inji
   Verify v0.16.0 was tested against **Inji Certify v0.13.1** and **Inji
   Web v0.17.0**. We run v0.14.0 + v0.16.0 — so the two workarounds
   above (did.json kids + assets/config.json) also paper over pairing
   mismatches. Upstream is moving to a cleaner verify-service +
   canonicalization pipeline in their next minor.

## Cross-stack compatibility summary

| Issuer ↓ / Holder → / Verifier → | walt.id wallet      | Inji Web Wallet     | walt.id verifier | Inji Verify                         |
|----------------------------------|---------------------|---------------------|------------------|-------------------------------------|
| walt.id issuer                   | End-to-end          | Not supported (Inji Web is catalog-initiated, not offer-consuming) | End-to-end | Works for W3C VCDM formats after @context alignment |
| Inji Certify pre-auth            | End-to-end          | Not compatible (Mimoto assumes auth-code)                          | Works — adapter re-canonicalizes | Works (with inji-proxy kid fix) |
| Inji Certify auth-code           | Not supported (walt.id wallet has no eSignet login) | End-to-end          | Works              | Works (with inji-proxy kid fix) |

The DPG selection cards in the UI reflect these combinations via their
`Capabilities` arrays — users only see combinations that have been verified
to work.
