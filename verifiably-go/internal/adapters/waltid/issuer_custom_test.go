package waltid

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/httpx"
	"github.com/verifiably/verifiably-go/vctypes"
)

// fakeIssuer mounts the minimal subset of walt.id endpoints the adapter
// touches: /onboard/issuer (returns a fixed key/DID), the issuer-metadata
// endpoint (controls whether borrowConfigIDFor finds a fallback), and
// /openid4vc/jwt/issue (records the request body so we can assert the
// configID the adapter sent).
type fakeIssuer struct {
	t      *testing.T
	mu     sync.Mutex
	bodies []issuanceRequest

	// metadataConfigs controls /draft13/.well-known/openid-credential-issuer's
	// credential_configurations_supported. Empty map ⇒ borrow path errors.
	metadataConfigs map[string]credentialConfigurationEntry
}

func (f *fakeIssuer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/onboard/issuer", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(onboardingResponse{
			IssuerKey: json.RawMessage(`{"type":"jwk","jwk":{"kty":"OKP"}}`),
			IssuerDID: "did:jwk:test",
		})
	})
	mux.HandleFunc("/draft13/.well-known/openid-credential-issuer", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(credentialIssuerMetadata{
			CredentialIssuer:                  "http://localhost",
			CredentialConfigurationsSupported: f.metadataConfigs,
		})
	})
	mux.HandleFunc("/openid4vc/jwt/issue", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ir issuanceRequest
		if err := json.Unmarshal(body, &ir); err != nil {
			f.t.Errorf("issuance body did not decode: %v\nbody=%s", err, body)
			http.Error(w, "bad body", 400)
			return
		}
		f.mu.Lock()
		f.bodies = append(f.bodies, ir)
		f.mu.Unlock()
		_, _ = w.Write([]byte("openid-credential-offer://example?credential_offer=test"))
	})
	return mux
}

func (f *fakeIssuer) lastBody(t *testing.T) issuanceRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		t.Fatalf("fakeIssuer received no issuance requests")
	}
	return f.bodies[len(f.bodies)-1]
}

func newAdapterWithIssuer(t *testing.T, fake *fakeIssuer) *Adapter {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	a, err := New(Config{
		IssuerBaseURL:   srv.URL,
		VerifierBaseURL: srv.URL,
		WalletBaseURL:   srv.URL,
		StandardVersion: "draft13",
	}, "Walt Community Stack")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.issuer = httpx.New(srv.URL)
	return a
}

// TestIssueToWallet_UsesRegisteredConfigID locks in the Phase 1 contract:
// once SaveCustomSchema appends a catalog entry, the next IssueToWallet
// call sends the registered configID — NOT a borrowed one. This is the
// payoff for the whole catalog-edit pipeline; without this assertion a
// regression that re-routed through borrowConfigIDFor would silently
// downgrade us back to the pre-Phase-1 behaviour and the issued VC's type
// would revert to the borrowed credential's name.
func TestIssueToWallet_UsesRegisteredConfigID(t *testing.T) {
	fake := &fakeIssuer{t: t}
	a := newAdapterWithIssuer(t, fake)

	// Pretend SaveCustomSchema already ran for this schema.
	a.registeredConfigIDs = map[string]string{
		"custom-rt": "MyCustomCred_jwt_vc_json",
	}

	res, err := a.IssueToWallet(context.Background(), backend.IssueRequest{
		Schema: vctypes.Schema{
			ID:     "custom-rt",
			Name:   "MyCustomCred",
			Std:    "w3c_vcdm_2",
			Custom: true,
		},
		SubjectData: map[string]string{"holder": "alice"},
		Flow:        "pre_auth",
	})
	if err != nil {
		t.Fatalf("IssueToWallet: %v", err)
	}
	if !strings.HasPrefix(res.OfferURI, "openid-credential-offer://") {
		t.Errorf("offer URI not propagated: %q", res.OfferURI)
	}
	got := fake.lastBody(t).CredentialConfigurationId
	if got != "MyCustomCred_jwt_vc_json" {
		t.Errorf("configID sent to walt.id = %q, want MyCustomCred_jwt_vc_json", got)
	}
}

// TestIssueToWallet_FallsBackToBorrowWhenNotRegistered verifies that
// schemas saved in Phase-2-only formats (or saved before this adapter
// shipped its catalog hook) keep working. The borrow path queries
// issuer-metadata, picks a format-compatible configID, and uses it. If
// metadata is empty the borrow returns a clear error — also checked.
func TestIssueToWallet_FallsBackToBorrowWhenNotRegistered(t *testing.T) {
	fake := &fakeIssuer{
		t: t,
		metadataConfigs: map[string]credentialConfigurationEntry{
			"BankId_jwt_vc_json": {Format: "jwt_vc_json"},
		},
	}
	a := newAdapterWithIssuer(t, fake)
	// No registeredConfigIDs entry — must fall back to borrow.

	if _, err := a.IssueToWallet(context.Background(), backend.IssueRequest{
		Schema: vctypes.Schema{
			ID:     "custom-borrow",
			Name:   "BorrowMe",
			Std:    "w3c_vcdm_2",
			Custom: true,
		},
		SubjectData: map[string]string{"holder": "bob"},
		Flow:        "pre_auth",
	}); err != nil {
		t.Fatalf("IssueToWallet (borrow path): %v", err)
	}
	got := fake.lastBody(t).CredentialConfigurationId
	if got != "BankId_jwt_vc_json" {
		t.Errorf("borrow configID = %q, want BankId_jwt_vc_json", got)
	}
}

// TestIssueToWallet_BorrowFailsWhenNoCompatibleConfig surfaces a clear
// error when neither the registered map nor the issuer-metadata advertise
// a usable configID — better than a 400 from walt.id with an opaque
// "credentialConfigurationId not recognised" string.
func TestIssueToWallet_BorrowFailsWhenNoCompatibleConfig(t *testing.T) {
	fake := &fakeIssuer{t: t} // empty metadata
	a := newAdapterWithIssuer(t, fake)

	_, err := a.IssueToWallet(context.Background(), backend.IssueRequest{
		Schema: vctypes.Schema{ID: "x", Name: "X", Std: "w3c_vcdm_2", Custom: true},
		Flow:   "pre_auth",
	})
	if err == nil || !strings.Contains(err.Error(), "no configuration") {
		t.Errorf("expected helpful 'no configuration' error, got %v", err)
	}
}

// TestSaveCustomSchema_NoOpWithoutCatalogPath documents the deliberate
// soft-fail in dev setups (no bind-mounted catalog file). Without this,
// every developer running `go test ./...` outside the docker-compose stack
// would hit a CatalogPath-required error path.
func TestSaveCustomSchema_NoOpWithoutCatalogPath(t *testing.T) {
	a, _ := New(Config{
		IssuerBaseURL:   "http://x",
		VerifierBaseURL: "http://x",
		WalletBaseURL:   "http://x",
	}, "Walt Community Stack")
	err := a.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID: "x", Name: "X", Std: "w3c_vcdm_2", Custom: true,
	})
	if err != nil {
		t.Errorf("expected no-op when CatalogPath empty, got %v", err)
	}
}

// TestSaveCustomSchema_NoOpForUnsupportedFormat covers the Phase-1 partial:
// non-jwt_vc_json schemas (mso_mdoc, sd_jwt_vc) still save into the registry
// but skip the catalog edit. The borrow trick takes over at issuance time.
func TestSaveCustomSchema_NoOpForUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credential-issuer-metadata.conf")
	if err := os.WriteFile(path, []byte(seedCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := New(Config{
		IssuerBaseURL:   "http://x",
		VerifierBaseURL: "http://x",
		WalletBaseURL:   "http://x",
		CatalogPath:     path,
	}, "Walt Community Stack")
	err := a.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID: "y", Name: "Y", Std: "mso_mdoc", Custom: true,
	})
	if err != nil {
		t.Errorf("Phase-2 format should no-op, got %v", err)
	}
	// File must be untouched.
	got, _ := os.ReadFile(path)
	if string(got) != seedCatalog {
		t.Errorf("catalog file should not have been modified for mso_mdoc save")
	}
	// And no configID should have been registered (so issue falls back to borrow).
	if a.registeredConfigIDs["y"] != "" {
		t.Errorf("registeredConfigIDs should be empty for unsupported-format save")
	}
}

// TestSaveCustomSchema_NonCustomNoOps protects against an accidental edit
// path being triggered for a stock schema (a renamed BankId, etc). Stock
// schemas already exist in the catalog as walt.id seeded them — re-writing
// them would create a duplicate that breaks HOCON.
func TestSaveCustomSchema_NonCustomNoOps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credential-issuer-metadata.conf")
	_ = os.WriteFile(path, []byte(seedCatalog), 0o644)
	a, _ := New(Config{
		IssuerBaseURL:   "http://x",
		VerifierBaseURL: "http://x",
		WalletBaseURL:   "http://x",
		CatalogPath:     path,
	}, "Walt Community Stack")

	if err := a.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID: "BankId_jwt_vc_json", Name: "BankId", Std: "w3c_vcdm_2", Custom: false,
	}); err != nil {
		t.Errorf("non-custom save should no-op, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != seedCatalog {
		t.Errorf("catalog file should not have been modified for non-custom save")
	}
}

// TestUnmarshalConfig_EnvFallthrough verifies deploy.sh's wiring: setting
// WALTID_CATALOG_PATH and WALTID_ISSUER_SERVICE in the container env makes
// them appear on the parsed Config without backends.json mentioning them.
func TestUnmarshalConfig_EnvFallthrough(t *testing.T) {
	t.Setenv("WALTID_CATALOG_PATH", "/tmp/test-catalog.conf")
	t.Setenv("WALTID_ISSUER_SERVICE", "test-issuer")

	cfg, err := UnmarshalConfig([]byte(`{
        "issuerBaseUrl": "http://localhost:7002",
        "verifierBaseUrl": "http://localhost:7003",
        "walletBaseUrl": "http://localhost:7001"
    }`))
	if err != nil {
		t.Fatalf("UnmarshalConfig: %v", err)
	}
	if cfg.CatalogPath != "/tmp/test-catalog.conf" {
		t.Errorf("CatalogPath = %q, want env value", cfg.CatalogPath)
	}
	if cfg.IssuerServiceName != "test-issuer" {
		t.Errorf("IssuerServiceName = %q, want env value", cfg.IssuerServiceName)
	}
}

// TestUnmarshalConfig_JSONOverridesEnv ensures backends.json wins when both
// are set — handy for one-off overrides without changing deployment env.
func TestUnmarshalConfig_JSONOverridesEnv(t *testing.T) {
	t.Setenv("WALTID_CATALOG_PATH", "/tmp/from-env.conf")
	cfg, err := UnmarshalConfig([]byte(`{
        "issuerBaseUrl": "http://x",
        "verifierBaseUrl": "http://x",
        "walletBaseUrl": "http://x",
        "catalogPath": "/tmp/from-json.conf"
    }`))
	if err != nil {
		t.Fatalf("UnmarshalConfig: %v", err)
	}
	if cfg.CatalogPath != "/tmp/from-json.conf" {
		t.Errorf("CatalogPath = %q, want JSON value", cfg.CatalogPath)
	}
}
