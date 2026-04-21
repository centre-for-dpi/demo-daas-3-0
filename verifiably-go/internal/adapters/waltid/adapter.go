package waltid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/httpx"
	"github.com/verifiably/verifiably-go/vctypes"
)

// jsonReader marshals v to JSON and wraps it in an io.Reader — convenience for
// the DoRaw path where we still want to send JSON in the body.
func jsonReader(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// Adapter holds the three role clients and lazily-bootstrapped session state.
type Adapter struct {
	cfg Config
	// Vendor is the key this adapter is registered under (e.g. "walt.id").
	// Used when PrefillSubjectFields / ListSchemas need to compare against
	// Schema.DPGs. Provided by main.go at construction.
	Vendor string

	issuer   *httpx.Client
	verifier *httpx.Client
	wallet   *httpx.Client

	mu        sync.Mutex
	issuerKey json.RawMessage // JWK wrapper from /onboard/issuer
	issuerDID string
	// sessions partitions walt.id wallet state by the per-user identity
	// key the handler injects via backend.WithHolderIdentity. Each caller
	// gets their own walt.id account + walletId so one user's credentials
	// never leak into another's inbox. Empty key hits the legacy shared
	// demo account — covers the pre-OIDC single-user demo mode.
	sessions map[string]*walletSession
}

// walletSession is the bootstrapped wallet-api state: a session JWT + a
// walletId to issue API calls against. Populated on first wallet call.
type walletSession struct {
	Token    string
	WalletID string
}

// New constructs an Adapter from Config. Validates required URLs but defers
// onboarding + wallet login until the first call that needs them — so startup
// stays fast and a missing/unreachable backend surfaces as a per-request error
// rather than crashing the whole app.
func New(cfg Config, vendor string) (*Adapter, error) {
	if cfg.IssuerBaseURL == "" || cfg.VerifierBaseURL == "" || cfg.WalletBaseURL == "" {
		return nil, fmt.Errorf("waltid: issuerBaseUrl, verifierBaseUrl, and walletBaseUrl are required")
	}
	return &Adapter{
		cfg:       cfg,
		Vendor:    vendor,
		issuer:    httpx.New(cfg.IssuerBaseURL),
		verifier:  httpx.New(cfg.VerifierBaseURL),
		wallet:    httpx.New(cfg.WalletBaseURL),
		issuerKey: cfg.IssuerKey,
		issuerDID: cfg.IssuerDID,
		sessions:  map[string]*walletSession{},
	}, nil
}

// Compile-time check: Adapter satisfies backend.Adapter.
var _ backend.Adapter = (*Adapter)(nil)

// The catalog methods below exist only to satisfy backend.Adapter. The
// registry's own catalog takes precedence — it builds the DPG map from
// backends.json and never delegates to concrete adapters for ListXDpgs.
// Returning empty maps here is both honest and harmless.

func (a *Adapter) ListIssuerDpgs(_ context.Context) (map[string]vctypes.DPG, error) {
	return nil, nil
}

func (a *Adapter) ListHolderDpgs(_ context.Context) (map[string]vctypes.DPG, error) {
	return nil, nil
}

func (a *Adapter) ListVerifierDpgs(_ context.Context) (map[string]vctypes.DPG, error) {
	return nil, nil
}
