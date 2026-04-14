// Package adapter implements a VerifierStore that routes verification
// requests through the standalone verification-adapter
// (~/cdpi/credebl-inji-adapter/verification-adapter). The adapter is a
// backend-agnostic verifier that:
//
//   - Routes LDP_VC online to Inji Verify / CREDEBL Agent / walt.id verifier
//     by DID method, per a declarative backends.json config.
//   - Verifies LDP_VC offline using URDNA2015 canonicalization + the two-hash
//     pattern, against a local issuer key cache.
//   - Forwards SD-JWT raw to any capable backend, or verifies locally via
//     the x5c certificate in the JWT header.
//   - Supports PixelPass (Base45+zlib) and JSON-XT-templated input decoding.
//
// This store gives our UI a single "backend-agnostic verifier" surface
// that works for credentials issued by any DPG (Walt.id, Inji, Credebl).
// It assumes the adapter is running at AdapterURL (default http://localhost:8085).
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
)

// VerifierStore is the adapter-backed verifier.
type VerifierStore struct {
	AdapterURL string // e.g. http://localhost:8085

	synced   map[string]bool
	syncedMu sync.Mutex
}

// New creates a VerifierStore pointing at the given adapter URL.
// If url is empty, defaults to http://localhost:8085.
func New(url string) *VerifierStore {
	if url == "" {
		url = "http://localhost:8085"
	}
	return &VerifierStore{
		AdapterURL: strings.TrimRight(url, "/"),
		synced:     map[string]bool{},
	}
}

// Ensure VerifierStore implements store.VerifierStore.
var _ store.VerifierStore = (*VerifierStore)(nil)

func (s *VerifierStore) Name() string { return "Verification Adapter (backend-agnostic)" }

func (s *VerifierStore) Capabilities() model.VerifierCapabilities {
	return model.VerifierCapabilities{
		CreateRequest:          false, // OID4VP session flow not supported by the adapter
		DirectVerify:           true,
		PresentationDefinition: false,
		PolicyEngine:           false,
		RevocationCheck:        true,
		DIDMethods:             []string{"did:key", "did:web", "did:jwk", "did:polygon", "did:indy"},
	}
}

// CreateVerificationSession: the adapter doesn't do OID4VP sessions. We
// return a pseudo-session so the UI's session-based flow still has a
// handle to display; actual verification happens via DirectVerify.
func (s *VerifierStore) CreateVerificationSession(ctx context.Context, req model.VerifyRequest) (*model.VerifyResult, error) {
	return &model.VerifyResult{
		State:      "adapter-pseudo",
		RequestURL: "",
	}, nil
}

func (s *VerifierStore) GetSessionResult(ctx context.Context, state string) (*model.VerifyResult, error) {
	return &model.VerifyResult{State: state}, nil
}

func (s *VerifierStore) ListPolicies(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"signature":  "Cryptographic signature check (URDNA2015 two-hash)",
		"revocation": "Revocation status list check",
		"expiry":     "issuanceDate / validFrom / validUntil check",
	}, nil
}

// DirectVerify submits a credential to the adapter. It tries online mode
// first; on failure it ensures the issuer DID is cached and retries via
// offline mode. The result is mapped to our VerifyResult shape.
func (s *VerifierStore) DirectVerify(ctx context.Context, credential []byte, contentType string) (*model.VerifyResult, error) {
	trimmed := bytes.TrimSpace(credential)
	isJWTish := len(trimmed) > 0 && trimmed[0] == 'e' && bytes.Count(trimmed, []byte(".")) >= 2

	// SD-JWT / JWT: forward raw with the appropriate content type.
	if isJWTish {
		if contentType == "" {
			if strings.Contains(string(trimmed), "~") {
				contentType = "application/vc+sd-jwt"
			} else {
				contentType = "application/jwt"
			}
		}
		return s.postAdapter(ctx, "/v1/verify/vc-verification", trimmed, contentType)
	}

	// JSON-LD: try online via /v1/verify/vc-verification (wrap in verifiableCredentials[]).
	// If it returns INVALID (common when Inji Verify can't resolve did:key), fall back
	// to /verify-offline after ensuring the issuer DID is synced.
	onlineResp, err := s.postOnlineWrapped(ctx, trimmed)
	if err == nil && isSuccess(onlineResp) {
		return onlineResp, nil
	}

	// Parse issuer DID and /sync it so the offline cache has the key.
	issuerDID := extractIssuerDID(trimmed)
	if issuerDID != "" {
		s.ensureSynced(ctx, issuerDID)
	}

	offlineResp, offErr := s.postAdapter(ctx, "/verify-offline", trimmed, "application/json")
	if offErr != nil {
		if err != nil {
			return nil, fmt.Errorf("online error: %v; offline error: %v", err, offErr)
		}
		return onlineResp, nil
	}
	return offlineResp, nil
}

// postOnlineWrapped wraps the credential in {"verifiableCredentials":[cred]}
// and submits to the adapter's /v1/verify/vc-verification.
func (s *VerifierStore) postOnlineWrapped(ctx context.Context, credJSON []byte) (*model.VerifyResult, error) {
	var cred map[string]any
	if err := json.Unmarshal(credJSON, &cred); err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{
		"verifiableCredentials": []any{cred},
	})
	return s.postAdapter(ctx, "/v1/verify/vc-verification", body, "application/json")
}

// postAdapter sends a body to an adapter endpoint and maps the response.
func (s *VerifierStore) postAdapter(ctx context.Context, path string, body []byte, contentType string) (*model.VerifyResult, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", s.AdapterURL+path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("adapter POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var parsed map[string]any
	_ = json.Unmarshal(respBody, &parsed)
	status, _ := parsed["verificationStatus"].(string)
	level, _ := parsed["verificationLevel"].(string)
	backend, _ := parsed["backend"].(string)
	// The adapter's `offline` field means "verified using cached keys without
	// a backend round-trip". It does NOT mean "air-gap"; URDNA2015 still
	// needs network access to fetch JSON-LD @context URLs. For true air-gap,
	// run the adapter with `docker run --network none` after pre-syncing.
	adapterLocal, _ := parsed["offline"].(bool)
	mode := "via backend"
	if adapterLocal {
		mode = "adapter-local crypto"
	}

	verified := status == "SUCCESS"
	result := &model.VerifyResult{
		Verified: &verified,
	}
	if verified {
		summary := fmt.Sprintf("verified via adapter (%s, %s)", mode, level)
		if backend != "" {
			summary += " → " + backend
		}
		result.Checks = []model.CheckResult{
			{Name: "Signature", Status: "pass", Summary: summary},
		}
	} else if status != "" {
		result.Checks = []model.CheckResult{
			{Name: "Signature", Status: "fail", Summary: status},
		}
	}
	return result, nil
}

func isSuccess(r *model.VerifyResult) bool {
	return r != nil && r.Verified != nil && *r.Verified
}

// ensureSynced calls /sync once per issuer DID to populate the adapter's
// offline-verification cache. Safe to call repeatedly — later calls are no-ops.
func (s *VerifierStore) ensureSynced(ctx context.Context, did string) {
	s.syncedMu.Lock()
	if s.synced[did] {
		s.syncedMu.Unlock()
		return
	}
	s.synced[did] = true
	s.syncedMu.Unlock()

	body, _ := json.Marshal(map[string]string{"did": did})
	req, _ := http.NewRequestWithContext(ctx, "POST", s.AdapterURL+"/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Mark as not-synced so a retry is possible.
		s.syncedMu.Lock()
		delete(s.synced, did)
		s.syncedMu.Unlock()
		return
	}
	resp.Body.Close()
}

// extractIssuerDID parses a credential JSON and returns its issuer DID.
// Handles both string and object forms: "issuer": "did:..." or "issuer": {"id": "did:..."}.
func extractIssuerDID(credJSON []byte) string {
	var cred map[string]any
	if err := json.Unmarshal(credJSON, &cred); err != nil {
		return ""
	}
	switch iss := cred["issuer"].(type) {
	case string:
		return iss
	case map[string]any:
		if id, ok := iss["id"].(string); ok {
			return id
		}
	}
	return ""
}
