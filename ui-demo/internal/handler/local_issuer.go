package handler

// local_issuer.go — an in-process OID4VCI Pre-Authorized Code flow server
// that wraps our URDNA2015 LDP_VC signer. It lets any OID4VCI-capable wallet
// claim LDP_VC credentials that our Go app signs, using the real OID4VCI
// protocol — no synthetic "local-bag://" URLs, no shortcut.
//
// Endpoints:
//
//   GET  /local-issuer/.well-known/openid-credential-issuer
//   GET  /local-issuer/.well-known/oauth-authorization-server
//   GET  /local-issuer/offer/{id}
//   POST /local-issuer/token
//   POST /local-issuer/credential
//
// Walt.id's wallet-api, the in-process inji holder, or a real SSI wallet
// can all complete a full issue → claim round-trip against this surface.
//
// Advertised config: a single `LocalLDPCredential` config that accepts any
// credential_definition.type list in the credential request, so the handler
// can issue any VC type through one config by passing the types through the
// pre-authorized stash.
//
// Proof JWT handling: for v1 the proof JWT is parsed syntactically but NOT
// cryptographically verified. Every real OID4VCI issuer MUST verify the
// proof — this is a demo shortcut, not production behavior. Marked as a TODO
// at the validation site.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"vcplatform/internal/store/ldpsigner"
)

// LocalIssuer holds the state of the in-process OID4VCI issuer.
type LocalIssuer struct {
	// BaseURL is the externally-reachable URL of the /local-issuer namespace.
	// Must be the same host the credential_offer_uri resolves to, so wallets
	// (inside docker) can GET the metadata, POST the token endpoint, and
	// POST the credential endpoint without hostname rewrites.
	BaseURL string

	// Signer is the URDNA2015 Ed25519Signature2020 LDP_VC signer.
	Signer *ldpsigner.Signer

	mu      sync.Mutex
	pending map[string]*pendingIssuance   // keyed by pre-auth code
	tokens  map[string]*authorizedIssuance // keyed by access token
}

type pendingIssuance struct {
	PreAuthCode       string
	CredentialType    []string
	CredentialSubject map[string]any
	CreatedAt         time.Time
}

type authorizedIssuance struct {
	AccessToken string
	PendingID   string
	CNonce      string
	Expires     time.Time
}

// NewLocalIssuer constructs a LocalIssuer with the given base URL and signer.
// Example baseURL: "http://host.docker.internal:8080/local-issuer".
func NewLocalIssuer(baseURL string, signer *ldpsigner.Signer) *LocalIssuer {
	if signer == nil {
		return nil
	}
	return &LocalIssuer{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Signer:  signer,
		pending: map[string]*pendingIssuance{},
		tokens:  map[string]*authorizedIssuance{},
	}
}

// StagePending registers a pending issuance and returns a fresh pre-auth code.
// Call this from the issuer handler when an operator clicks "Sign & Issue" —
// it stashes the credential claims server-side so the wallet can pick them
// up later via the OID4VCI exchange.
//
// Also prunes anything older than 10 minutes from the pending map.
func (li *LocalIssuer) StagePending(types []string, claims map[string]any) (preAuthCode string) {
	li.mu.Lock()
	defer li.mu.Unlock()
	code := randBase64(24)
	li.pending[code] = &pendingIssuance{
		PreAuthCode:       code,
		CredentialType:    types,
		CredentialSubject: claims,
		CreatedAt:         time.Now(),
	}
	cutoff := time.Now().Add(-10 * time.Minute)
	for k, v := range li.pending {
		if v.CreatedAt.Before(cutoff) {
			delete(li.pending, k)
		}
	}
	return code
}

// BuildOfferURL returns the openid-credential-offer:// URL that carries the
// credential_offer_uri pointing at this local issuer's /offer/{code}.
func (li *LocalIssuer) BuildOfferURL(preAuthCode string) string {
	offerURI := li.BaseURL + "/offer/" + preAuthCode
	return "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(offerURI)
}

// RegisterRoutes attaches the local issuer's routes to the given mux.
// All routes are unauthenticated — they implement a public OID4VCI surface
// any wallet can talk to.
func (li *LocalIssuer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /local-issuer/.well-known/openid-credential-issuer", li.handleIssuerMetadata)
	mux.HandleFunc("GET /local-issuer/.well-known/oauth-authorization-server", li.handleAuthServerMetadata)
	mux.HandleFunc("GET /local-issuer/offer/{id}", li.handleOffer)
	mux.HandleFunc("POST /local-issuer/token", li.handleToken)
	mux.HandleFunc("POST /local-issuer/credential", li.handleCredential)
}

// -----------------------------------------------------------------------------
// Endpoint: GET /local-issuer/.well-known/openid-credential-issuer
// -----------------------------------------------------------------------------

func (li *LocalIssuer) handleIssuerMetadata(w http.ResponseWriter, r *http.Request) {
	// The shape here is intentionally minimal: walt.id wallet-api's strict
	// kotlinx parser rejects metadata that includes Inji-specific extensions
	// (display, proof_types_supported, credentialSubject claim descriptors, etc).
	md := map[string]any{
		"credential_issuer":      li.BaseURL,
		"authorization_servers":  []string{li.BaseURL},
		"credential_endpoint":    li.BaseURL + "/credential",
		"token_endpoint":         li.BaseURL + "/token",
		"credential_configurations_supported": map[string]any{
			"LocalLDPCredential": map[string]any{
				"format":                                 "ldp_vc",
				"cryptographic_binding_methods_supported": []string{"did:key", "did:jwk"},
				"credential_signing_alg_values_supported": []string{"EdDSA"},
				"credential_definition": map[string]any{
					"@context": []string{
						"https://www.w3.org/2018/credentials/v1",
						"https://w3id.org/security/suites/ed25519-2020/v1",
					},
					"type": []string{"VerifiableCredential"},
				},
			},
		},
	}
	writeJSONPlain(w, http.StatusOK, md)
}

// -----------------------------------------------------------------------------
// Endpoint: GET /local-issuer/.well-known/oauth-authorization-server
// -----------------------------------------------------------------------------

func (li *LocalIssuer) handleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	md := map[string]any{
		"issuer":                  li.BaseURL,
		"token_endpoint":          li.BaseURL + "/token",
		"grant_types_supported":   []string{"urn:ietf:params:oauth:grant-type:pre-authorized_code"},
		"response_types_supported": []string{"token"},
	}
	writeJSONPlain(w, http.StatusOK, md)
}

// -----------------------------------------------------------------------------
// Endpoint: GET /local-issuer/offer/{id}
// -----------------------------------------------------------------------------

func (li *LocalIssuer) handleOffer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	li.mu.Lock()
	pending, ok := li.pending[id]
	li.mu.Unlock()
	if !ok {
		http.Error(w, "offer not found or expired", http.StatusNotFound)
		return
	}
	offer := map[string]any{
		"credential_issuer":            li.BaseURL,
		"credential_configuration_ids": []string{"LocalLDPCredential"},
		"grants": map[string]any{
			"urn:ietf:params:oauth:grant-type:pre-authorized_code": map[string]any{
				"pre-authorized_code": pending.PreAuthCode,
			},
		},
	}
	writeJSONPlain(w, http.StatusOK, offer)
}

// -----------------------------------------------------------------------------
// Endpoint: POST /local-issuer/token
// -----------------------------------------------------------------------------

func (li *LocalIssuer) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSONPlain(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	grantType := r.PostForm.Get("grant_type")
	preAuth := r.PostForm.Get("pre-authorized_code")
	if grantType != "urn:ietf:params:oauth:grant-type:pre-authorized_code" {
		writeJSONPlain(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	if preAuth == "" {
		writeJSONPlain(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}

	li.mu.Lock()
	pending, ok := li.pending[preAuth]
	if !ok {
		li.mu.Unlock()
		writeJSONPlain(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	// Pre-auth codes are single-use — remove it now.
	delete(li.pending, preAuth)

	accessToken := randBase64(32)
	cNonce := randBase64(16)
	li.tokens[accessToken] = &authorizedIssuance{
		AccessToken: accessToken,
		PendingID:   pending.PreAuthCode,
		CNonce:      cNonce,
		Expires:     time.Now().Add(10 * time.Minute),
	}
	// Store the resolved pending back under the new access-token key so
	// /credential can fetch it without the pre-auth code.
	li.pending[accessToken] = pending
	li.mu.Unlock()

	writeJSONPlain(w, http.StatusOK, map[string]any{
		"access_token":       accessToken,
		"token_type":         "Bearer",
		"expires_in":         600,
		"c_nonce":            cNonce,
		"c_nonce_expires_in": 600,
	})
}

// -----------------------------------------------------------------------------
// Endpoint: POST /local-issuer/credential
// -----------------------------------------------------------------------------

func (li *LocalIssuer) handleCredential(w http.ResponseWriter, r *http.Request) {
	authHdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHdr, "Bearer ") {
		writeJSONPlain(w, http.StatusUnauthorized, map[string]string{"error": "missing_bearer_token"})
		return
	}
	accessToken := strings.TrimPrefix(authHdr, "Bearer ")

	li.mu.Lock()
	tok, okToken := li.tokens[accessToken]
	if okToken && time.Now().After(tok.Expires) {
		delete(li.tokens, accessToken)
		delete(li.pending, accessToken)
		okToken = false
	}
	pending, ok := li.pending[accessToken]
	if !okToken || !ok {
		li.mu.Unlock()
		writeJSONPlain(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}
	// Credential issuance is single-use per token.
	delete(li.pending, accessToken)
	delete(li.tokens, accessToken)
	li.mu.Unlock()

	// Parse the credential request body. We accept the wallet's requested
	// credential_definition.type list and ignore proof signature verification
	// for v1 (TODO: enforce proof JWT cryptography).
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Format               string         `json:"format"`
		CredentialDefinition map[string]any `json:"credential_definition"`
		Proof                map[string]any `json:"proof"`
	}
	_ = json.Unmarshal(body, &req)

	// Prefer the credential type list the wallet sent; fall back to the
	// staged types from the pending issuance.
	types := pending.CredentialType
	if cd := req.CredentialDefinition; cd != nil {
		if t, ok := cd["type"].([]any); ok && len(t) > 0 {
			merged := make([]string, 0, len(t))
			for _, v := range t {
				if s, ok := v.(string); ok && s != "VerifiableCredential" {
					merged = append(merged, s)
				}
			}
			if len(merged) > 0 {
				types = merged
			}
		}
	}

	// TODO: validate req.Proof.jwt cryptographically. For v1 we accept any
	// syntactically-valid (or absent) proof so demos work end-to-end.
	_ = req.Proof

	credJSON, err := li.Signer.SignJSON(types, pending.CredentialSubject, "")
	if err != nil {
		writeJSONPlain(w, http.StatusInternalServerError, map[string]string{"error": "sign: " + err.Error()})
		return
	}
	var credObj map[string]any
	_ = json.Unmarshal(credJSON, &credObj)

	writeJSONPlain(w, http.StatusOK, map[string]any{
		"credential":         credObj,
		"format":             "ldp_vc",
		"c_nonce":            randBase64(16),
		"c_nonce_expires_in": 600,
	})
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// writeJSONPlain serializes v as JSON to w with a Content-Type header. It's a
// local alternative to the package-level writeJSON to avoid any coupling.
func writeJSONPlain(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randBase64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("err-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DefaultLocalIssuerBaseURL returns the base URL other services should use
// to reach our local issuer from inside a docker network. Reads the
// LOCAL_ISSUER_URL env var, falls back to host.docker.internal:8080.
func DefaultLocalIssuerBaseURL() string {
	if v := os.Getenv("LOCAL_ISSUER_URL"); v != "" {
		return v
	}
	return "http://host.docker.internal:8080/local-issuer"
}
