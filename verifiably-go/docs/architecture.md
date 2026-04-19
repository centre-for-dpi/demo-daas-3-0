# Architecture

A Go + HTMX server that renders HTML for every interaction and treats the
backend layer as a single `backend.Adapter` interface. No SPA, no framework,
standard library `net/http` and `html/template` only.

## Package layout

```
vctypes/            Shared domain types (DPG, Schema, Credential, …).
                    Zero third-party deps. Everything else imports this.

backend/            Adapter interface + request/response shapes + the
                    context helper for holder-DPG routing.
                    Depends only on vctypes and stdlib.

internal/adapters/  Concrete implementations of Adapter:
    registry/       Fan-out adapter: routes to the right per-DPG
                    impl based on the DPG selected in the request.
    waltid/         Real walt.id issuer-api / wallet-api / verifier-api client.
    injicertify/    Real Inji Certify client; supports both pre-auth and
                    auth-code flows.
    injiweb/        Thin stub — Inji Web is browser-hosted; the adapter
                    exposes the redirect URL and capability claims.
    injiverify/     Real Inji Verify client for OID4VP + direct verify.
    libretranslate/ Translator client with on-disk cache.
    factory/        Builds the right adapter from a config/backends.json entry.

internal/auth/      OIDC provider registry (Keycloak, WSO2IS) + discovery +
                    PKCE authorize URL + token exchange.

internal/handlers/  HTTP handlers. Depend on backend + vctypes. Never import
                    adapters directly — they go through the Registry.
                    inji_proxy.go hosts the did:web and credential-forwarding
                    endpoints certify-nginx routes back to us.
                    i18n_postprocess.go wraps render output with a text-node
                    walker that translates anything not explicitly marked.

internal/httpx/     Tiny HTTP client with bearer-token context injection.

cmd/server/         main.go + adapter wiring + auth wiring + i18n wiring +
                    template loading. The only place where adapter types
                    are named explicitly.

templates/          html/template files. layout.html wraps every page;
                    fragments/ hold the HTMX-swappable pieces.

static/             CSS + a small JS file + the jsQR scanner.

e2e/                Headless Chromium tests (puppeteer-core) per DPG flow.

deploy/             compose override + per-service config overrides.
deploy.sh           Single-entrypoint orchestrator.
```

## Dependency graph

```
handlers ──→ backend ──→ vctypes
   ↑          ↑
   │          │
cmd/server ──┴──→ registry ──→ waltid / injicertify / injiweb / injiverify
                      │
                      └──→ libretranslate
```

`internal/handlers` only knows about the Adapter interface. Swapping every
concrete adapter in `cmd/server/adapter.go` doesn't move a byte of handler
code.

## The Adapter interface

One ~30-method interface in `backend/adapter.go` covers:

- List capabilities (`ListIssuerDpgs`, `ListHolderDpgs`, `ListVerifierDpgs`)
- Schema browsing + custom-schema lifecycle
- Prefill (MOSIP Identity Plugin, walt.id demo account, …)
- Issuance (`IssueToWallet`, `IssueAsPDF`, `IssueBulk`)
- Wallet operations (`ParseOffer`, `ClaimCredential`, `ListWalletCredentials`)
- Presentation (`PresentCredential`, `RequestPresentation`, `FetchPresentationResult`)
- Direct verification (`VerifyDirect`)
- Example offers (`ListExampleOffers`, `BootstrapOffers`)

Most methods carry a `...Dpg` field in the request struct. The four that
don't (`ParseOffer`, `ClaimCredential`, `ListWalletCredentials`,
`PresentCredential`'s context call path) use
`backend.WithHolderDpg(ctx, vendor)` — the Registry reads it back via
`backend.HolderDpgFromContext(ctx)` to route the call.

## Registry fan-out

`internal/adapters/registry` is the adapter the handlers actually hold. It
keeps per-role maps (`issuers`, `holders`, `verifiers`) keyed by vendor
display name, and for each request dispatches to the matching concrete
adapter. Unknown DPGs return `backend.ErrUnknownDPG`; handlers show a
toast rather than crashing.

When a scenario has a single holder registered, `currentHolder` falls
through to a shortcut — so scenario=waltid works even if a handler forgets
to wrap the context. `all` forces callers to be explicit.

## HTMX pattern

Every interactive control is `hx-get` or `hx-post` that swaps an HTML
fragment back into the page. Key conventions:

- **Page loads** render `{{template "layout" .}}` which dispatches to the
  page-specific `content_X` template via `PageData.ContentTemplate`.
- **HTMX boost** requests (`HX-Target: main`) skip the layout and render
  just the content template.
- **Fragment responses** use `H.renderFragment(w, r, name, data)` — no
  layout, just the named sub-template.
- **Toasts** are triggered via the `HX-Trigger` response header; a JS
  listener in `static/js/app.js` pops them.
- **Out-of-band swaps** (`hx-swap-oob`) let one response update a distant
  element — for example, the Continue button state outside the DPG grid.

## Session model

One in-memory store (`internal/handlers/session.go`) keyed by a cookie.
Holds the currently-picked DPG per role, wallet contents, last seen error,
expand state for DPG cards, schema-builder draft, and the auth token
returned by the OIDC provider.

Per-session locking is **not** implemented — two concurrent HTMX requests
from the same browser could race on writes. Real deployments should move
sessions to Redis or wrap each request in a per-session mutex.

## Authentication

`internal/auth` loads OIDC providers from `config/auth-providers.json`,
does discovery via `.well-known/openid-configuration`, and handles PKCE
code exchange on the `/auth/callback` route. The resulting access token
lives in the session; adapters pick it up via `httpx.WithToken(ctx, tok)`.

Inside a docker deployment the provider URL has two forms: the
browser-visible one (`http://localhost:8180`, what HX-Redirect sends the
browser to) and the container-visible one (`http://keycloak:8180`, what
the Go server uses for discovery + token exchange). `oidc.Discover`
rewrites endpoints to the internal form for server-side use and flips
them back to public for browser redirects.

## Translation

`internal/adapters/libretranslate` caches translations on disk
(`locales/<lang>.json`) so repeat renders are instant.

Two layers keep every surface translated:

1. **Template-level `{{t "string" $.Lang}}`** — static strings known at
   template-write time are wrapped explicitly. Parse-time binding means
   the helper is a single package-level function that looks up the
   translator from a request-scoped package variable.

2. **Post-render HTML walker** (`internal/handlers/i18n_postprocess.go`)
   — when Lang != "en", the render output is captured to a buffer,
   parsed with `golang.org/x/net/html`, walked node-by-node, and every
   text node (plus `title` / `placeholder` / `alt` / `aria-label`
   attributes) is translated. Skips `<script>`, `<style>`, `<code>`,
   `<pre>`, `<textarea>`, and elements with `translate="no"` or class
   `mono` / `notr` — those hold identifiers, URLs, and brand names that
   must render verbatim.

The safety-net walker means a template author can forget to wrap a string
and translation still works. Brand names that get incorrectly translated
("Keycloak" → "Clé" in French) are fixed by adding a `mono` class to the
span holding them.

## Inji-proxy (did:web resolver + credential forwarder)

`certify-nginx` (MOSIP's nginx in front of inji-certify) proxies
`POST /v1/certify/issuance/credential` and `GET /.well-known/did.json`
back through `host.docker.internal:8080` — our port. Three endpoints
fulfil these:

- `POST /inji-proxy/issuance/credential` — forwards to
  `inji-certify:8090`, patching `credential_definition.@context` if the
  wallet omitted it (walt.id's Kotlin wallet does; Mimoto doesn't).

- `GET /inji-proxy/.well-known/did.json` — fetches upstream did.json,
  then appends synthetic `verificationMethod` entries for every kid we've
  seen in issued VCs. Inji Certify v0.14.0 signs VCs and bitstring
  status-list credentials with different kid fragments derived from the
  same Ed25519 key via different hash paths — its own did.json only
  advertises one. Without this indirection, Inji Verify fails with
  `PublicKeyResolutionFailed`.

- `GET /inji-proxy/credentials/status-list/{id}` — forwards the
  status-list VC and records its `proof.verificationMethod` kid so the
  did.json handler publishes it. The set of observed kids can also be
  pre-seeded via the `INJI_PROXY_EXTRA_KIDS` env var for restarts.

See [dpg-matrix.md](dpg-matrix.md) for the upstream bugs each of these
workarounds target.

## Testing

`e2e/` holds puppeteer-core tests per DPG flow:

- `waltid-test.mjs` — end-to-end issue + hold + present on walt.id
- `inji-test.mjs` / `injiweb-test.mjs` / `injiverify-test.mjs` —
  flow-specific checks per MOSIP component
- `matrix-test.mjs` — every DPG × role combination renders and commits
- `injiweb-credentials-visible.mjs` — regression for the FX5 bug (UI origin
  mismatch caused "No Credentials found")
- `i18n-inner-pages.mjs` — translation middleware covers deep pages
- `bulk-csv-test.mjs` / `scan-upload-test.mjs` / `present-test.mjs` —
  isolated feature checks

Run a single test with `CHROME_PATH=/usr/bin/google-chrome node e2e/<name>.mjs`.
Go unit tests (currently `internal/adapters/registry` routing) run with
`go test ./...`.
