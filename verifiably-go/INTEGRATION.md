# Verifiably — Backend Integration Guide

This prototype is fully functional against an in-memory mock layer. To connect a real backend, you do **not** touch the UI code. You reimplement the `BACKEND` object inside `vc-prototype.html`.

## Where to look in the code

Open `vc-prototype.html` and search for these three markers, in order:

1. **`const MOCK = {`** — every piece of fake data (DPG catalogs, schemas, subject values, issuer identities, offer URIs, verification outcomes). You can delete this entire object once the backend is live, but it's useful to keep around as a fixture for local development.

2. **`const BACKEND = {`** — the ten async adapter functions. Their **names, argument shapes, and return shapes are stable**. Replace each body with a `fetch(...)` call to your real endpoint. The UI calls these via `await BACKEND.foo(...)` and doesn't care where the data comes from.

3. **`const DEBUG_SHOW_MOCK_MARKERS = false`** — flip to `true` during integration to see small `[mock]` pills on every UI surface still rendering from the mock layer. When you've wired all adapters, none of the pills should appear.

## Adapter → endpoint mapping per DPG

Each `BACKEND.*` function has a `// TODO:` comment naming the concrete endpoint. The table below expands those into copy-ready targets. Base URLs shown as env-var placeholders — substitute your deployed DPG URLs.

| Adapter | walt.id Community Stack v0.18.2 | Inji Certify v0.14.0 | Inji Web Wallet v0.16.0 | Inji Verify v0.16.0 |
|---|---|---|---|---|
| `listIssuerDpgs` | N/A (app-level registry) | N/A | — | — |
| `listHolderDpgs` | — | — | N/A (app-level registry) | — |
| `listVerifierDpgs` | — | — | — | N/A (app-level registry) |
| `listSchemas(issuerDpg)` | `GET {ISSUER_API}/credentialConfigurations` | `GET {CERTIFY_API}/v1/certify/credential-configuration` | — | — |
| `prefillSubjectFields(schema)` | N/A (operator types into form) | MOSIP Identity Plugin: `GET {CERTIFY_API}/v1/certify/issuance/data-provider/{plugin}?key={uin}` | — | — |
| `issueToWallet({issuerDpg, schema, subjectData})` | `POST {ISSUER_API}/openid4vc/jwt/issue` (or `/sdjwt/issue`, `/mdoc/issue` depending on format) → returns offer URI | `POST {CERTIFY_API}/v1/certify/issuance/credential-offer` → returns pre-authorized code + offer URI | — | — |
| `issueAsPdf({issuerDpg, schema, subjectData})` | **Not supported at v0.18.2** — must build on top of the issuer library | Relies on Claim 169 QR Code spec (pixelpass lib) + MOSIP Identity Plugin; `POST {CERTIFY_API}/v1/certify/issuance/credential` with QR-embed flag | — | — |
| `issueBulk({issuerDpg, schema, csvRows})` | `POST {ISSUER_API}/openid4vc/batchIssue` | `POST {CERTIFY_API}/v1/certify/issuance/bulk` (check v0.14.0 docs for exact shape) | — | — |
| `parseOffer(offerUri)` | `POST {WALLET_API}/wallet/{walletId}/exchange/resolveCredentialOffer` | — | Redirect flow — not called from this app | — |
| `claimCredential(offer)` | `POST {WALLET_API}/wallet/{walletId}/exchange/useOfferRequest` | — | Redirect flow | — |
| `requestPresentation({verifierDpg, templateKey})` | `POST {VERIFIER_API}/openid4vc/verify` → returns request URI + state | — | — | Redirect flow |
| `fetchPresentationResult({state})` | `GET {VERIFIER_API}/openid4vc/session/{state}` or register a `statusCallbackUri` on the verify call | — | — | Redirect flow |
| `verifyDirect({verifierDpg, method, credentialData})` | `POST {VERIFIER_API}/openid4vc/verify/credential` | — | — | For W3C VCDM: `POST {VERIFY_API}/v1/verify/vc-verification`. For SD-JWT VC (new in v0.16.0): `POST {VERIFY_API}/v1/verify/vc-submission` then `GET /v1/verify/vp-result/{transactionId}` |

## Authentication

The UI already has a Keycloak / WSO2 provider selection on the auth screen. Once authenticated, whatever session token your IDP returns should be attached to every `BACKEND.*` call. A clean way to do this:

```js
const BACKEND = {
  _fetch(url, opts = {}) {
    return fetch(url, {
      ...opts,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${STATE.authToken}`,
        ...(opts.headers || {})
      }
    }).then(r => r.ok ? r.json() : Promise.reject(r));
  },
  async listSchemas(issuerDpg) {
    const base = ISSUER_BASE_URLS[issuerDpg];
    return this._fetch(`${base}/credentialConfigurations`);
  },
  // ...
};
```

Store the token in `STATE.authToken` from `completeAuth()` (currently stubs auth — swap with a real IDP redirect + token exchange).

## Format note — return shapes

The UI expects specific shapes. If your backend returns something different, adapt it inside the `BACKEND` adapter rather than changing the UI:

- `listSchemas` returns an array of `{ id, name, std, dpgs, desc }`.
- `issueToWallet` returns `{ offerUri, offerId, flow, expiresIn }`.
- `issueAsPdf` returns `{ issuerName, issuerDid, payloadSizeKb, fields }`.
- `issueBulk` returns `{ accepted, rejected, errors: [{ row, reason }] }`.
- `parseOffer` returns `{ title, issuer, type, fields }` where `fields` is a flat key→value map.
- `claimCredential` returns the claimed credential with `status: 'accepted'` added.
- `requestPresentation` returns `{ requestUri, state, template: {...} }`.
- `fetchPresentationResult` and `verifyDirect` return `{ valid, method, format, issuer, subject, requested?, issued, checkedRevocation }`.

If your verifier supports richer result metadata (signature suite, trust chain, revocation list source, etc.), add fields to the return shape — `renderVerifyResult(opts)` gracefully ignores unknown fields, and you can extend it when you need to surface them.

## Cross-version compatibility warning

The four DPG versions used in this prototype (walt.id v0.18.2, Certify v0.14.0, Inji Web v0.16.0, Inji Verify v0.16.0) are **not a tested-compatible matrix** — each vendor publishes its own compatibility table. When deploying:

- Inji Web Wallet v0.16.0 declares compatibility with Inji Certify **v0.13.1** and Inji Verify **v0.17.0**.
- Inji Verify v0.16.0 declares compatibility with Inji Web **v0.14.0** and Inji Wallet **v0.20.0**.
- walt.id Community Stack is versioned as a unit; all three (issuer, wallet, verifier) ship together at v0.18.2.

Pick one vendor's matrix and align. Mixing versions is how interop bugs happen.

## Known issues to guard around

- **INJIVER-1131** (Inji Verify v0.16.0 cross-device flow): presentation is reported valid even when a wrong VC is submitted. Implement credential-type validation in your Relying Party code until fixed upstream.
- **INJICERT mDoc** (v0.14.0): issuance is mock-only per the GitHub README. Don't ship mDoc credentials from Certify in production.
- **walt.id wallet OID4VP v1.0** (through v0.18.2): not yet fully supported in the wallet/demo apps. Older OID4VP (Presentation Exchange) works.

## Swap checklist

Once the backend is live, verify:

- [ ] `DEBUG_SHOW_MOCK_MARKERS = true` shows no `[mock]` pills during a full end-to-end flow
- [ ] DPG capability cards still reflect your deployed versions (edit `MOCK.issuerDpgs` etc., or replace with a `listIssuerDpgs` backend call)
- [ ] `parseOffer` correctly handles both `credential_offer=...` (inline) and `credential_offer_uri=...` (by-reference) forms
- [ ] Auth token refresh is handled for long-lived sessions
- [ ] Errors from `BACKEND` calls surface as user-friendly toasts (currently most failure paths throw silently)
