# verifiably-go

Go + HTMX port of the Verifiable Credentials prototype. Standard library only
(`net/http` + `html/template`), no frameworks.

## Run

```bash
go run ./cmd/server
```

Serves on `:8080`. Override with `VERIFIABLY_ADDR=:3000`. Turn on the debug
banner with `VERIFIABLY_DEBUG_MOCK_MARKERS=1`.

## Plug in your backend

The whole app talks to **one interface**: `backend.Adapter`. The bundled demo
ships a `MockAdapter` that satisfies it from hardcoded data. To go live:

1. Implement `backend.Adapter` with your own HTTP-calling type.
2. Replace one line in `cmd/server/main.go`.

That's it. No handler changes, no template changes, no schema changes.

### Example skeleton

Anywhere in your own module or a sibling package:

```go
package waltid

import (
    "context"
    "github.com/verifiably/verifiably-go/backend"
    "github.com/verifiably/verifiably-go/vctypes"
)

type Adapter struct {
    apiURL string
    token  string
}

func New(apiURL, token string) *Adapter {
    return &Adapter{apiURL: apiURL, token: token}
}

// 19 methods to implement; compile-time check enforces the shape.
var _ backend.Adapter = (*Adapter)(nil)

func (a *Adapter) ListIssuerDpgs(ctx context.Context) (map[string]vctypes.DPG, error) {
    // GET {apiURL}/dpgs?role=issuer, map into vctypes.DPG values
    // ...
}

// ... 18 more methods
```

Then in `cmd/server/main.go` replace:

```go
adapter := mock.NewAdapter()
```

with:

```go
adapter := waltid.New("https://api.walt.id", os.Getenv("WALTID_TOKEN"))
```

See [INTEGRATION.md](static/INTEGRATION.md) for the full endpoint-mapping table
per DPG (walt.id / Inji Certify / Inji Web Wallet / Inji Verify).

## Package layout

```
vctypes/            — shared domain types (DPG, Schema, Credential, ...). No
                      dependencies. Import this.
backend/            — Adapter interface + request/response shapes. Imports only
                      vctypes and time. Import this to implement an adapter.
internal/mock/      — MockAdapter + all demo data. Internal because a real
                      deployment doesn't need it. Import only from main.go.
internal/handlers/  — HTTP handlers. Depend on backend + vctypes. Never on mock.
cmd/server/main.go  — routes, templates, static files, and the single adapter
                      wiring line.
templates/          — html/template files (pages + fragments + layout).
static/             — CSS, a tiny JS file, and INTEGRATION.md for the footer link.
```

Dependency graph:

```
handlers ──→ backend ──→ vctypes
    ↑         ↑
    │         │
cmd/server ───┴──→ mock ──→ vctypes (+ backend for the adapter interface)
```

The `internal/mock` package is the only place that depends on both `backend`
and `vctypes` — that's deliberate: it implements the interface using demo
data. Swap it and nothing else changes.

## HTMX pattern

Every interactive control uses `hx-get` or `hx-post` to swap an HTML fragment
server-side.

- Full page loads render `{{template "layout" .}}` which dispatches to a
  page-specific `content_X` template via the `ContentTemplate` field in PageData.
- HTMX fragment requests skip the layout; the handler calls
  `renderFragment(name, data)` and returns just the fragment.
- Toasts are triggered via `HX-Trigger: toast:<msg>` response headers;
  `static/js/app.js` listens and shows them.
- Out-of-band swaps (`hx-swap-oob`) update sibling elements — e.g. the Continue
  button outside the DPG grid gets its disabled class updated when a card is
  expanded.

## Flows implemented

All 13 views from the original single-file HTML prototype carry over:

**Issuer** — pick DPG (expandable capability cards with Continue commit) →
pick schema (chip filter, search, inline JSON Schema preview with `@vocab`
context injection) or build custom (live JSON preview, dynamic field rows) →
pick scale (single / bulk CSV) × destination (wallet via OID4VCI / PDF with
embedded QR, auto-disabled when DPG doesn't support it) → issue (form adapts
to source: manual / API / MOSIP Identity Plugin / custom plugin /
presentation-during-issuance) → result (offer URI + QR + copyable link, or
PDF preview modal with A4 layout).

**Holder** — pick wallet DPG → wallet (scan / paste / example URI → pending
offers inbox → accept or reject → held credentials stack) → present (scan
verifier QR or paste OID4VP link → simulated selective-disclosure modal).

**Verifier** — pick verifier DPG → verify (OID4VP request: template dropdown,
generate, QR + link, simulate holder response / direct verify: scan / upload /
paste). Result card shows valid/invalid banner with issuer DID, subject DID,
revocation check status, and fields received for OID4VP.

## DPG version disclosure

Capability claims reflect specific documented releases:
- **walt.id Community Stack v0.18.2**
- **Inji Certify v0.14.0**
- **Inji Web Wallet v0.16.0**
- **Inji Verify v0.16.0**

These four are **not a tested-compatible matrix** — each vendor publishes its
own compatibility table. The landing page surfaces this up front.

## Known limitations (preserved from spec audit)

- **INJIVER-1131** (Inji Verify v0.16.0 cross-device): reports valid even when
  wrong VC is submitted — mitigate with RP-side credential-type validation
- **INJICERT mDoc** (Inji Certify v0.14.0): issuance is mock-only per GitHub
  README — don't ship mDoc from Certify in production
- **walt.id wallet OID4VP v1.0** (through v0.18.2): still in progress; older
  OID4VP (Presentation Exchange) works
- **Per-session locking is not implemented**: concurrent HTMX requests from
  the same user could race on session state. See `internal/handlers/session.go`
  for details. A real deployment should add a per-session `sync.Mutex` or move
  sessions to an external store (Redis, Postgres).
