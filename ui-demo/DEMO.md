# Demo Walk-through

This document is a scripted 10-minute walk-through for presenting the
backend-agnostic verifiable credentials platform to government stakeholders.

## Before the demo — bring up the stack

```bash
cd /home/adam/cdpi/n8n-demo/ui-demo
make docker-up          # starts Walt.id + Keycloak + WSO2 + LibreTranslate
docker compose -f docker/waltid/docker-compose.yml up -d \
  certify-postgres inji-verify-postgres inji-certify certify-nginx \
  inji-verify-service inji-verify-ui citizens-postgres vc-adapter
```

Wait ~3 minutes for Inji Certify to boot (check `docker logs inji-certify`
for `Started CertifyServiceApplication`). The other services are much faster.

Verify everything is up:

```bash
./scripts/demo-health.sh
```

## Starting the app — three deployment modes

### Mode A: Kenya (Walt.id stack)

```bash
./scripts/demo-kenya.sh
```

- Issuer: Walt.id Issuer API (jwt_vc_json, vc+sd-jwt, mso_mdoc)
- Wallet: Walt.id Wallet API
- Verifier: Walt.id Verifier API (OID4VP session flow)
- Data Source: Citizens DB (filtered to KE records)

### Mode B: Trinidad & Tobago (Inji stack)

```bash
./scripts/demo-trinidad.sh
```

- Issuer: Inji Certify (ldp_vc, vc+sd-jwt, mso_mdoc via OID4VCI Pre-Auth Code)
- Wallet: Inji Holder (in-process) — a minimal Go OID4VCI client that claims
  Inji-issued credentials server-side and stores them in the shared wallet bag
- Verifier: Inji Verify (direct-verify endpoint)
- Data Source: Citizens DB (filtered to TT records)

### Mode C: Hybrid cross-DPG

```bash
./scripts/demo-hybrid.sh
```

- Issuer: Walt.id (for jwt_vc_json, sd-jwt, mdoc) PLUS in-process URDNA2015
  signer (for ldp_vc) — LDP credentials are signed with Ed25519Signature2020
  and stashed directly in the shared wallet bag
- Wallet: Walt.id Wallet API (merged with shared wallet bag so LDP and JWT
  credentials appear in one list)
- Verifier: **verification-adapter** (docker service `vc-adapter`) — routes
  by DID method to Inji Verify / Walt.id Verifier, does URDNA2015 two-hash
  verification for LDP_VC, forwards SD-JWT raw, and supports true air-gap
  via `docker run --network none`
- Fallback: Walt.id Verifier for JWT/SD-JWT that need OID4VP sessions

## Cross-DPG verification matrix

All three modes run through `./scripts/smoke-e2e.sh`. Hybrid adds the
cross-DPG paths:

| Issuer | Format | Wallet | Verifier path | Result |
|---|---|---|---|---|
| Walt.id | jwt_vc_json | Walt.id | Walt.id OID4VP session (fallback) | SUCCESS |
| Walt.id | vc+sd-jwt | Walt.id | Walt.id OID4VP session (fallback) | SUCCESS* |
| **Walt.id (in-process signer)** | **ldp_vc** | **shared bag** | **adapter → URDNA2015 crypto** | **SUCCESS (CRYPTOGRAPHIC)** |
| Inji Certify | ldp_vc | Inji Holder | Inji Verify direct-verify | SUCCESS |
| Inji Certify | vc+sd-jwt | Inji Holder | Inji Verify direct-verify | SUCCESS |

\* SD-JWT OID4VP session verification hits a known walt.id
`SSIKit2WalletService.usePresentationRequest` serializer bug when the
credential ID is a URL rather than a UUID. Walt.id's own JSON-format SD-JWT
issuance + direct claim works; the failure is only in the fallback OID4VP
presentation step. Tracked as a known limitation.

### True air-gap verification

The verification-adapter supports fully offline cryptographic verification:

```bash
# Produce a credential (in hybrid mode) then:
SAMPLE_CRED_FILE=/tmp/cred.ldp.json \
SAMPLE_ISSUER_DID="did:key:..." \
  ./scripts/demo-airgap-verify.sh
```

Sequence:
1. Adapter container warmed up online: `/sync` caches the issuer DID's public
   key into SQLite.
2. Second adapter container started with `docker run --network none` — no
   network interfaces at all. Cache volume mounted read-write.
3. Credential submitted via `docker exec` (no host port forwarding possible
   under `--network none`).
4. Result: `verificationStatus: SUCCESS, offline: true, verificationLevel:
   TRUSTED_ISSUER` for LDP_VC (URDNA2015 context fetch fails air-gap, falls
   back to structural + cached-issuer validation). SD-JWT with `x5c` in the
   header reaches `CRYPTOGRAPHIC` because the cert is self-contained.

## Smoke test

The full stack can be exercised end-to-end via the API:

```bash
./scripts/smoke-e2e.sh
```

This script signs up, onboards an issuer, issues a credential, claims it into
the wallet, and verifies it. It auto-detects which mode the server is running
in and picks the right credential config + verification path
(OID4VP session for Walt.id, direct-verify for Inji).

## Walk-through script (10 minutes)

### 1. The pitch (1 min)
Open the landing page. Point to the three-service model:

> "This is one platform that any government can deploy with any combination of
> DPGs. The same UI. The same data sources. Kenya might run on Walt.id. Trinidad
> might run Inji Certify for issuance and Walt.id for the wallet. The UI adapts
> automatically based on what each backend supports."

### 2. Sign in as issuer (1 min)
Click "Sign In" → "Issuer Operator" → Keycloak. Use a pre-created account.

### 3. Register an issuer (1 min)
Sidebar → "Register Issuer". Fill in organization details. Click
"Register & Generate DID". Show the generated DID on the result panel.

> "This is live — the DID was just created by the backend. If this were Walt.id
> the DID is did:jwk. If it were Inji, it's did:web:certify-nginx. The adaptor
> translates the flow."

### 4. Build a credential schema (2 min)
Sidebar → "Schema Builder".
- Credential Configuration mode: "Use pre-configured type"
- Format filter: "All Formats"
- Select: "University Degree" (Kenya) OR "FarmerCredential" (Trinidad)
- Show that the fields auto-populate from the backend metadata
- Click "Publish Schema"

### 5. Issue a credential from the citizen database (2 min)
Sidebar → "Single Issuance".
- Schema dropdown: show the schema you just created
- Data Source dropdown: "Citizens Database"
- Search for a citizen: enter a `national_id` from the seeded data
  (example: `KE-NID-81016525` for Jelagat Gitau)
- Show the claims auto-filling from the Postgres table
- Click "Sign & Issue"
- Show the OID4VCI offer URL + real QR code

> "The offer URL is what the holder scans or clicks to receive the credential.
> The data came from the mock government database — in production, this is
> connected to a national ID registry, a Sunbird RC instance, or whatever the
> ministry uses."

### 6. Receive the credential as a holder (1.5 min)
- Open a second browser tab
- Sidebar → "My Wallet" → "Claim Credential"
- Paste the offer URL from step 5
- Click "Accept Credential"
- Show the credential appearing in "My Credentials"

### 7. Verify the credential (1.5 min)
- Sidebar → "Verify Credential"
- Tab: "Create Request"
- Click "Create Verification Request"
- Real QR code appears
- Switch back to holder tab → "Present Credential" → paste the OID4VP link
- Switch back to verifier tab → polling result arrives showing:
  - Green checkmark "Verified"
  - The actual credential claims (name, degree, etc.)
  - Signature + schema checks pass

### 8. Switch DPG mid-demo (1 min)
In a terminal:

```bash
# Stop current process, switch issuer to Inji, restart
pkill -f "./server" || true
ISSUER_DPG=inji WALLET_DPG=waltid VERIFIER_DPG=waltid \
  INJI_CERTIFY_URL=http://localhost:8090 \
  INJI_CERTIFY_PUBLIC_URL=http://certify-nginx:80 \
  ./server -config config/demo-trinidad.json &
```

Refresh the browser. Point to:
- The sidebar "Issuer" section now shows "Inji Certify" as the backend
- The credential configurations dropdown now shows FarmerCredential variants
- Batch Upload sidebar entry hides (Inji doesn't support batch in v1)

> "Same codebase. Same UI. Same user workflow. Different backend. One env var."

## What to avoid clicking

- "Onboarding Queue" — admin-only page, shown empty in v1
- "Dependents" — feature flagged out for v1
- "Credebl" DPG — beta stub, will show banners
- Languages other than EN/ES/FR — those three are polished, others are
  hidden for v1
- Any "Batch Upload" when using Inji — disabled by capability matrix
- Inji Verify creating OID4VP sessions — this backend uses direct-verify,
  so the create-session tab is hidden; use the "Check Holder Presentation"
  tab instead

## Reseeding if something breaks

```bash
# Reseed the citizens database from scratch
docker exec -i citizens-postgres psql -U citizens -d citizens \
  < docker/waltid/citizens-db/init.sql
```

```bash
# Full reset (wipes all Walt.id state)
cd docker/waltid && docker compose down -v && docker compose up -d
# Wait 3 minutes for Inji Certify
```
