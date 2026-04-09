# Plan: Replace Mock Data with Real DPG Backends

## Context

The vcplatform app currently uses in-memory mock stores. The goal is to replace these with real DPG backend calls so the same UI connects to live credential infrastructure. The architecture must:

1. **Start with direct HTTP calls** to DPG REST APIs (Go std lib `net/http`)
2. **Abstract the transport** so the same store interfaces can later be fulfilled by n8n webhooks or OpenFn workflows — without changing handlers or templates
3. **Be DPG-agnostic** — the first integration could be Inji, Credebl, Walt.id, or Quark.id
4. **Run locally** via Docker Compose for development

## What's Wrong with the Current Store Interfaces

The current `store.Store` interfaces are too thin for real backends:

```go
// Current — read-only, no write operations, no filtering, no pagination
type CredentialStore interface {
    ListCredentials(ctx context.Context) ([]model.Credential, error)
    GetCredential(ctx context.Context, id string) (*model.Credential, error)
}
```

Real DPG integration needs:
- **Write operations**: `IssueCredential`, `IssueBatch`, `RevokeCredential`, `CreateSchema`, `PublishSchema`
- **Verification**: `VerifyPresentation` (accepts a VP, returns a proof result with 6-point check)
- **Wallet operations**: `ListHeldCredentials(holderID)`, `AcceptCredential`, `ShareCredential`
- **Auth**: Real SSO via OIDC (eSignet, Keycloak, WSO2) — not cookie-based mock
- **Filtering/pagination**: `ListCredentials(ctx, filter)` with status, issuer, holder filters
- **Richer models**: Credential needs claims, proof data, DID references; Schema needs field definitions, JSON-LD context

## Architecture

### Three Layers

```
┌─────────────────────────────────────────────┐
│  Handlers (handler/*.go)                     │
│  - Call store interfaces                     │
│  - Never know about transport or DPG         │
├─────────────────────────────────────────────┤
│  Store Interfaces (store/store.go)           │
│  - Typed Go interfaces                       │
│  - Define the contract                       │
├─────────────────────────────────────────────┤
│  Adaptor Implementations                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐    │
│  │  mock/    │ │  inji/   │ │ credebl/ │    │
│  │ In-memory │ │ HTTP→API │ │ HTTP→API │    │
│  └──────────┘ └──────────┘ └──────────┘    │
│       ↑             ↑            ↑          │
│    (current)    (phase 1)   (phase 2)       │
├─────────────────────────────────────────────┤
│  Transport (internal/transport/)             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐    │
│  │  direct   │ │  n8n     │ │  openfn  │    │
│  │ net/http  │ │ webhook  │ │ webhook  │    │
│  └──────────┘ └──────────┘ └──────────┘    │
│    (default)    (future)     (future)       │
└─────────────────────────────────────────────┘
```

### Transport Abstraction

The transport layer is a simple interface that DPG adaptors use internally:

```go
// internal/transport/transport.go
package transport

type Client interface {
    // Do sends a request and returns the response body.
    // The implementation decides whether to call a URL directly,
    // POST to an n8n webhook, or trigger an OpenFn job.
    Do(ctx context.Context, method, path string, body any) ([]byte, error)
}
```

Implementations:
- `transport.NewHTTPClient(baseURL, authToken)` — direct HTTP calls (default)
- `transport.NewWebhookClient(webhookURL, secret)` — POST to n8n/OpenFn webhook with method+path+body as payload, receive response
- `transport.NewOpenFnClient(projectURL, token)` — trigger OpenFn workflow

Each DPG adaptor accepts a `transport.Client` at construction:

```go
// internal/store/inji/inji.go
func NewStores(client transport.Client) *store.Stores {
    return &store.Stores{
        Credentials: &credentialStore{client: client},
        Schemas:     &schemaStore{client: client},
        // ...
    }
}
```

Swapping transport is a config change in `main.go`:

```go
// Direct HTTP (default)
client := transport.NewHTTPClient(cfg.Backend.URL, cfg.Backend.Token)

// Or via n8n
client := transport.NewWebhookClient(cfg.Backend.WebhookURL, cfg.Backend.Secret)

// Same adaptor either way
stores := inji.NewStores(client)
```

## Expanded Store Interfaces

```go
package store

// --- Auth ---
type AuthStore interface {
    // AuthorizeURL returns the SSO authorization URL for redirect.
    AuthorizeURL(ctx context.Context, provider, redirectURI string) (string, error)
    // ExchangeToken exchanges an auth code for a session/user.
    ExchangeToken(ctx context.Context, code, redirectURI string) (*model.User, *model.Session, error)
    // ValidateSession checks if a session token is still valid.
    ValidateSession(ctx context.Context, token string) (*model.User, error)
}

// --- Schemas ---
type SchemaStore interface {
    ListSchemas(ctx context.Context, filter model.SchemaFilter) ([]model.Schema, int, error)
    GetSchema(ctx context.Context, id string) (*model.SchemaDetail, error)
    CreateSchema(ctx context.Context, req model.CreateSchemaRequest) (*model.Schema, error)
    UpdateSchema(ctx context.Context, id string, req model.UpdateSchemaRequest) (*model.Schema, error)
    PublishSchema(ctx context.Context, id string) error
}

// --- Credentials (Issuer) ---
type IssuerStore interface {
    IssueCredential(ctx context.Context, req model.IssueRequest) (*model.Credential, error)
    IssueBatch(ctx context.Context, req model.BatchIssueRequest) (*model.BatchResult, error)
    ListIssuedCredentials(ctx context.Context, filter model.CredentialFilter) ([]model.Credential, int, error)
    GetIssuedCredential(ctx context.Context, id string) (*model.CredentialDetail, error)
    RevokeCredential(ctx context.Context, id string, reason string) error
    SuspendCredential(ctx context.Context, id string) error
    GetDispatchStatus(ctx context.Context, batchID string) (*model.DispatchStatus, error)
}

// --- Credentials (Holder/Wallet) ---
type WalletStore interface {
    ListHeldCredentials(ctx context.Context, holderID string, filter model.CredentialFilter) ([]model.HeldCredential, error)
    GetHeldCredential(ctx context.Context, holderID, credID string) (*model.HeldCredentialDetail, error)
    AcceptCredential(ctx context.Context, holderID, offerID string) (*model.HeldCredential, error)
    DeleteCredential(ctx context.Context, holderID, credID string) error
    ListPresentationRequests(ctx context.Context, holderID string) ([]model.PresentationRequest, error)
    BuildPresentation(ctx context.Context, req model.BuildPresentationRequest) (*model.Presentation, error)
}

// --- Verification ---
type VerifierStore interface {
    CreateVerificationRequest(ctx context.Context, req model.VerificationRequestDef) (*model.VerificationRequest, error)
    VerifyPresentation(ctx context.Context, vp []byte) (*model.VerificationResult, error)
    ListVerifications(ctx context.Context, filter model.VerificationFilter) ([]model.VerificationResult, int, error)
    GetVerification(ctx context.Context, id string) (*model.VerificationDetail, error)
}

// --- Trust Infrastructure ---
type TrustStore interface {
    ListTrustedIssuers(ctx context.Context, filter model.IssuerFilter) ([]model.TrustedIssuer, error)
    RegisterIssuer(ctx context.Context, req model.RegisterIssuerRequest) (*model.TrustedIssuer, error)
    ApproveIssuer(ctx context.Context, id string) error
    RejectIssuer(ctx context.Context, id string) error
    ListTrustedVerifiers(ctx context.Context) ([]model.TrustedVerifier, error)
}

// --- DID & Keys ---
type DIDStore interface {
    ListDIDs(ctx context.Context) ([]model.DID, error)
    CreateDID(ctx context.Context, req model.CreateDIDRequest) (*model.DID, error)
    RotateKey(ctx context.Context, didID string) (*model.DID, error)
    ResolveDID(ctx context.Context, did string) (*model.DIDDocument, error)
}

// --- Notifications & Audit (can remain local) ---
type NotificationStore interface {
    ListNotifications(ctx context.Context, userID string, filter model.NotifFilter) ([]model.Notification, error)
    MarkRead(ctx context.Context, userID, notifID string) error
}

type AuditStore interface {
    ListEntries(ctx context.Context, filter model.AuditFilter) ([]model.AuditEntry, int, error)
    GetActivitySummary(ctx context.Context, period string) (*model.ActivitySummary, error)
}

// Stores aggregates all store interfaces.
type Stores struct {
    Auth          AuthStore
    Schemas       SchemaStore
    Issuer        IssuerStore
    Wallet        WalletStore
    Verifier      VerifierStore
    Trust         TrustStore
    DIDs          DIDStore
    Notifications NotificationStore
    Audit         AuditStore
}
```

## Expanded Models

The current models are too flat. Real DPGs return richer data:

```go
// Credential with full claims and proof
type CredentialDetail struct {
    Credential
    Claims      map[string]any  `json:"claims"`       // credentialSubject fields
    Proof       *ProofData      `json:"proof"`         // cryptographic proof
    IssuerDID   string          `json:"issuer_did"`
    HolderDID   string          `json:"holder_did"`
    Format      string          `json:"format"`        // W3C-VCDM, SD-JWT, mDL, AnonCreds
    RawJSON     json.RawMessage `json:"raw_json"`      // original credential JSON
}

// 6-point verification result
type VerificationResult struct {
    ID          string          `json:"id"`
    Overall     string          `json:"overall"`       // pass, fail, pending
    Checks      []CheckResult   `json:"checks"`        // 6 checks
    Claims      map[string]any  `json:"claims"`        // disclosed claims
    HolderID    string          `json:"holder_id"`
    CredType    string          `json:"cred_type"`
    IssuerName  string          `json:"issuer_name"`
    Timestamp   time.Time       `json:"timestamp"`
    LatencyMs   int             `json:"latency_ms"`
}

type CheckResult struct {
    Name    string `json:"name"`     // Signature, ProofChain, CertPath, Revocation, Expiry, Schema
    Status  string `json:"status"`   // pass, fail, pending
    Summary string `json:"summary"`
    Detail  string `json:"detail"`   // technical detail
}

// Schema with field definitions
type SchemaDetail struct {
    Schema
    Fields      []SchemaField   `json:"fields"`
    Context     string          `json:"context"`       // JSON-LD @context URL
    RawJSON     json.RawMessage `json:"raw_json"`
}

type SchemaField struct {
    Name     string `json:"name"`      // credentialSubject.fieldName
    Type     string `json:"type"`      // string, number, date, boolean, uri
    Required bool   `json:"required"`
}
```

## Backend Configuration

Add to `config.json`:

```json
{
  "backend": {
    "type": "mock",
    "url": "",
    "token": "",
    "transport": "direct"
  }
}
```

Options:
- `type: "mock"` — current in-memory mock (default, no backend needed)
- `type: "inji"` — Inji Certify + Verify + eSignet
- `type: "credebl"` — Credebl platform
- `type: "waltid"` — Walt.id identity services
- `type: "quarkid"` — Quark.id
- `transport: "direct"` — Go HTTP client calls DPG API directly
- `transport: "n8n"` — Go posts to n8n webhook, n8n calls DPG
- `transport: "openfn"` — Go triggers OpenFn job

## Docker Compose for Local Dev

```yaml
# docker-compose.yml
services:
  vcplatform:
    build: .
    ports: ["8080:8080"]
    environment:
      - VCPLATFORM_CONFIG=/app/config/docker.json
    depends_on:
      - inji-certify   # or credebl, or waltid

  # Option A: Inji stack
  inji-certify:
    image: mosipid/inji-certify:latest
    ports: ["8081:8081"]
  inji-verify:
    image: mosipid/inji-verify:latest
    ports: ["8082:8082"]
  esignet:
    image: mosipid/esignet:latest
    ports: ["8083:8083"]

  # Option B: Credebl
  credebl:
    image: credebl/platform:latest
    ports: ["8081:3000"]

  # Option C: Walt.id
  waltid-issuer:
    image: waltid/issuer-api:latest
    ports: ["8081:7002"]
  waltid-verifier:
    image: waltid/verifier-api:latest
    ports: ["8082:7003"]
  waltid-wallet:
    image: waltid/wallet-api:latest
    ports: ["8083:7001"]
```

## Directory Structure (new files)

```
internal/
  transport/
    transport.go            # Client interface
    http.go                 # Direct HTTP implementation
    webhook.go              # n8n/OpenFn webhook implementation
  store/
    store.go                # Expanded interfaces (replaces current)
    mock/
      mock.go               # Updated mock (implements new interfaces)
    inji/
      inji.go               # Inji adaptor (first real DPG)
      auth.go               # eSignet OIDC flows
      issuer.go             # Inji Certify API calls
      verifier.go           # Inji Verify API calls
      schemas.go            # Schema management
      dids.go               # DID/key management
    credebl/                # (future)
    waltid/                 # (future)
  model/
    credential.go           # Expanded models
    verification.go         # VerificationResult, CheckResult
    schema.go               # SchemaDetail, SchemaField
    request.go              # IssueRequest, BatchIssueRequest, etc.
    did.go                  # DID, DIDDocument, CreateDIDRequest
    filter.go               # Filter types for list operations
config/
  docker.json               # Config pointing to local Docker DPG
docker-compose.yml          # Local dev stack
Dockerfile                  # Multi-stage build for the Go app
```

## Build Phases

### Phase A: Foundation (transport + expanded interfaces + models)
- Create `internal/transport/transport.go` with `Client` interface
- Create `internal/transport/http.go` — direct HTTP client using `net/http`
- Create `internal/transport/webhook.go` — webhook client (sends method+path+body, receives response)
- Expand `store/store.go` with full interfaces (Auth, Issuer, Wallet, Verifier, Trust, DID)
- Expand models: CredentialDetail, VerificationResult, SchemaDetail, filter types, request types
- Add `backend` section to config
- Update `mock/mock.go` to implement the expanded interfaces (returns same data, just through new signatures)
- Update `main.go` to select store implementation based on `config.backend.type`
- **Verify:** App still works identically with `type: "mock"`. No visible change.

### Phase B: First DPG adaptor (Inji or whichever is first available)
- Create `store/inji/` (or the chosen DPG) implementing all store interfaces
- Map DPG API endpoints to store methods:
  - `IssuerStore.IssueCredential` → `POST {certify}/v1/credentials/issue`
  - `IssuerStore.ListIssuedCredentials` → `GET {certify}/v1/credentials`
  - `VerifierStore.VerifyPresentation` → `POST {verify}/v1/verify`
  - `SchemaStore.CreateSchema` → `POST {certify}/v1/schemas`
  - `AuthStore.AuthorizeURL` → build eSignet OIDC authorize URL
  - etc.
- Handle auth token management (the Go app authenticates with the DPG, not the end user)
- Handle response mapping (DPG JSON → Go model structs)
- **Verify:** With Docker Compose running the DPG, the app shows real data.

### Phase C: Auth integration (real SSO)
- Replace cookie-based mock auth with OIDC flow:
  - `GET /login` → redirect to SSO authorize URL
  - `GET /auth/callback` → exchange code for token, create session
  - Session stores real user identity from the SSO provider
- Support eSignet, Keycloak, WSO2 via OIDC discovery (`.well-known/openid-configuration`)
- The `AuthStore` interface handles the abstraction — different SSO providers implement it differently
- **Verify:** Login redirects to real SSO, comes back with real user identity.

### Phase D: Handlers wire up to real data
- Update handlers to actually call store methods (currently most templates render static HTML)
- Pass store data to templates via `PageData.Data`
- Templates render from `{{.Data.Credentials}}` instead of hardcoded HTML
- This is the largest change — every template that shows data needs to be updated
- Start with the issuer workspace (schemas, credentials, issuance)
- Then holder (wallet, sharing)
- Then verifier (verification results, proof detail)
- **Verify:** Each workspace shows data from the real DPG backend.

### Phase E: Docker Compose + Dockerfile
- Create `Dockerfile` (multi-stage: build Go binary, copy into minimal image)
- Create `docker-compose.yml` with vcplatform + chosen DPG services
- Create `config/docker.json` pointing to Docker service hostnames
- **Verify:** `docker compose up` starts everything, app connects to DPG, full lifecycle works.

### Phase F: Second DPG adaptor
- Create `store/credebl/` or `store/waltid/` implementing the same interfaces
- Different API mappings, same store contracts
- Swap by changing `config.backend.type`
- **Verify:** Same UI, different backend, same behavior.

## Key Design Decisions

1. **The mock store stays forever.** It's the exploration/demo mode. Real DPG adaptors are *additional* implementations, not replacements.

2. **Transport is the n8n/OpenFn seam.** The DPG adaptor doesn't know or care whether its HTTP calls go directly to Inji or through an n8n webhook. The `transport.Client` interface is the abstraction point. When the user configures `transport: "n8n"`, the webhook client serializes the API call as a JSON payload and POSTs it to n8n, which then calls the real DPG API and returns the response.

3. **OID4VCI/OID4VP are the common protocol.** All four DPGs speak OID4VCI for issuance and OID4VP for verification. The adaptor layer should use these as the primary wire protocol where possible, falling back to DPG-specific admin APIs only for operations not covered by the standards (schema management, DID management, trust registry admin).

4. **Auth is the hardest part.** The current app uses a simple session cookie. Real SSO requires OIDC redirect flows, token management, session validation. This should be done as a dedicated phase, not mixed with credential operations.

5. **Templates need to become data-driven.** Currently, most templates have hardcoded sample HTML. To show real data, templates need to iterate over `{{range .Data.Credentials}}` etc. This is a significant template rewrite but the handler/store/transport layers can be built and tested independently first.

## What NOT to Change

- The template layout system (layouts, partials, HTMX patterns) stays the same
- The CSS stays the same
- The feature flag system stays the same
- The white-label config stays the same
- The middleware chain stays the same (auth middleware gets enhanced, not replaced)
- The `custom/` override system stays the same
