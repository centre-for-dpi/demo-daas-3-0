// Package inji implements the DPG store interfaces against MOSIP's Inji stack:
//
//   - Inji Certify (issuance)         — injistack/inji-certify-with-plugins:0.14.0
//   - Inji Verify (verification)      — mosipid/inji-verify-service:0.16.0
//   - Inji Web (wallet)               — web-based wallet UI
//
// Inji Certify uses the OID4VCI Pre-Authorized Code flow. The issuance sequence is:
//
//  1. POST /v1/certify/pre-authorized-data
//     → pre-stage the claims under a transaction ID.
//  2. GET  /v1/certify/credential-offer-data/{transactionId}
//     → returns the OID4VCI credential_offer object with a pre-authorized_code.
//
// Our IssueCredential returns the resulting openid-credential-offer:// URL.
// The holder's wallet then completes the flow:
//
//  3. POST /v1/certify/oauth/token  (grant_type=urn:ietf:params:oauth:grant-type:pre-authorized_code)
//  4. POST /v1/certify/issuance/credential  (with Bearer + proof JWT)
//
// This mirrors how Walt.id's flow works from our adaptor's point of view: the
// issuer produces an offer, the holder claims it.
package inji

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"strings"
	"time"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
	"vcplatform/internal/transport"
)

// Config holds Inji connection settings.
type Config struct {
	// CertifyURL is the base URL for Inji Certify's REST API.
	// e.g. http://localhost:8090 (the internal container port)
	CertifyURL string

	// CertifyPublicURL is the domain url Inji Certify was configured with
	// (mosip.certify.domain.url). Credential offers reference this URL so
	// holders can reach it. If different from CertifyURL, set both.
	// e.g. http://certify-nginx:80 or https://certify.example.com
	CertifyPublicURL string

	// VerifyURL is the base URL for Inji Verify's REST API.
	// e.g. http://localhost:8082
	VerifyURL string

	// WalletURL is the base URL for the Inji Web wallet (optional).
	WalletURL string
}

// =============================================================================
// IssuerStore — Inji Certify
// =============================================================================

type issuerStore struct {
	client    transport.Client
	publicURL string

	// ProxyRewrite is an optional function that takes a raw Inji offer URL and
	// returns a proxied version (same format, pointing at an internal OID4VCI
	// translation proxy). Used for cross-DPG wallet claims where the target
	// wallet can't parse Inji's metadata shape directly.
	proxyRewrite func(string) string
}

// NewIssuerStore creates an IssuerStore backed by Inji Certify.
func NewIssuerStore(certifyClient transport.Client, publicURL string) store.IssuerStore {
	return &issuerStore{client: certifyClient, publicURL: publicURL}
}

// SetProxyRewrite wires an offer-URL rewrite function, activated when
// INJI_PROXY_URL is set and the target wallet needs a translated metadata shape.
func SetProxyRewrite(s store.IssuerStore, fn func(string) string) {
	if is, ok := s.(*issuerStore); ok {
		is.proxyRewrite = fn
	}
}

func (s *issuerStore) Name() string { return "Inji Certify" }

func (s *issuerStore) Capabilities() model.IssuerCapabilities {
	return model.IssuerCapabilities{
		IssuerInitiated:     true,
		Batch:               false, // Certify supports it but not wired in v1
		Deferred:            true,  // Pre-Auth Code flow is inherently deferred
		SelectiveDisclosure: true,  // via SD-JWT configs
		CustomTypes:         true,  // via credential_config DB table
		Revocation:          true,  // Status List 2021 supported by Certify
		Formats:             []string{"ldp_vc", "vc+sd-jwt", "mso_mdoc"},
	}
}

// OnboardIssuer: Inji Certify uses pre-provisioned issuer DIDs (did:web:certify-nginx)
// configured via certify_init.sql. There is no runtime issuer onboarding endpoint.
// We return the default DID from the issuer metadata.
func (s *issuerStore) OnboardIssuer(ctx context.Context, keyType string) (*model.OnboardIssuerResult, error) {
	// Fetch issuer metadata to get the canonical issuer DID.
	resp, code, err := s.client.Do(ctx, "GET", "/v1/certify/.well-known/openid-credential-issuer", nil)
	if err != nil {
		return nil, fmt.Errorf("fetch issuer metadata: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("issuer metadata (%d): %s", code, truncate(string(resp), 200))
	}

	var md struct {
		CredentialIssuer string `json:"credential_issuer"`
	}
	if err := json.Unmarshal(resp, &md); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	// Inji Certify's DID is configured per deployment; we use the did:web
	// derived from the domain URL (did:web:certify-nginx by default).
	issuerDid := "did:web:certify-nginx"
	if strings.HasPrefix(md.CredentialIssuer, "http://") || strings.HasPrefix(md.CredentialIssuer, "https://") {
		host := extractHost(md.CredentialIssuer)
		if host != "" {
			issuerDid = "did:web:" + host
		}
	}

	return &model.OnboardIssuerResult{
		IssuerKey: json.RawMessage(`{"type":"inji-managed"}`), // Inji manages keys server-side
		IssuerDID: issuerDid,
	}, nil
}

// IssueCredential pre-stages credential data and returns the OID4VCI offer URL.
func (s *issuerStore) IssueCredential(ctx context.Context, issuer *model.OnboardIssuerResult, configID, format string, claims map[string]any) (string, error) {
	// Inji Certify pre-auth endpoint: snake_case field names.
	// It returns {"credential_offer_uri": "openid-credential-offer://?credential_offer_uri=..."}
	preAuthBody := map[string]any{
		"credential_configuration_id": configID,
		"claims":                      claims,
	}
	resp, code, err := s.client.Do(ctx, "POST", "/v1/certify/pre-authorized-data", preAuthBody)
	if err != nil {
		return "", fmt.Errorf("pre-auth data: %w", err)
	}
	if code != 200 && code != 201 {
		return "", fmt.Errorf("inji pre-auth (%d): %s", code, truncate(string(resp), 300))
	}

	var preAuthResp struct {
		CredentialOfferURI string `json:"credential_offer_uri"`
	}
	if err := json.Unmarshal(resp, &preAuthResp); err != nil {
		return "", fmt.Errorf("parse inji pre-auth response: %w", err)
	}
	if preAuthResp.CredentialOfferURI == "" {
		return "", fmt.Errorf("inji pre-auth returned empty offer: %s", truncate(string(resp), 200))
	}

	raw := sanitizeOfferURL(s.rewriteInternalURL(preAuthResp.CredentialOfferURI))
	// If a proxy rewrite is configured (cross-DPG flow), apply it so the
	// target wallet sees our proxy's URL instead of Inji's raw URL.
	if s.proxyRewrite != nil {
		return sanitizeOfferURL(s.proxyRewrite(raw)), nil
	}
	return raw, nil
}

// sanitizeOfferURL strips whitespace and control characters from an OID4VCI
// offer URL. Defensive — a stray newline will break url.Parse in wallets.
func sanitizeOfferURL(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// rewriteInternalURL replaces the internal Certify URL (as Certify sees itself)
// with a user-reachable URL (what the holder's wallet should use to fetch the offer).
// Certify puts its configured domain_url into the offer; we rewrite that domain
// to the publicURL we've been told about.
func (s *issuerStore) rewriteInternalURL(u string) string {
	if s.publicURL == "" {
		return u
	}
	publicURL := strings.TrimRight(s.publicURL, "/")

	// Internal bases (most specific first — longest match wins).
	// Don't include bare hostnames without port to avoid replacing suffixes inside longer URLs.
	internalBases := []string{
		"http://certify-nginx:80",
		"http://inji-certify:8090",
	}
	for _, h := range internalBases {
		u = strings.ReplaceAll(u, h, publicURL)
		encoded := urlEncode(h)
		encodedPublic := urlEncode(publicURL)
		u = strings.ReplaceAll(u, encoded, encodedPublic)
	}
	return u
}

func urlEncode(s string) string {
	r := strings.NewReplacer(
		":", "%3A",
		"/", "%2F",
	)
	return r.Replace(s)
}

func (s *issuerStore) IssueBatch(ctx context.Context, issuer *model.OnboardIssuerResult, configID, format string, records []map[string]any) (*model.BatchResult, error) {
	issued := 0
	var offers []string
	for _, claims := range records {
		offer, err := s.IssueCredential(ctx, issuer, configID, format, claims)
		if err == nil {
			offers = append(offers, offer)
			issued++
		}
	}
	return &model.BatchResult{
		Total:     len(records),
		Issued:    issued,
		Failed:    len(records) - issued,
		OfferURLs: offers,
	}, nil
}

// ListCredentialConfigs queries Inji Certify's OID4VCI metadata for supported configurations.
func (s *issuerStore) ListCredentialConfigs(ctx context.Context) ([]model.CredentialConfig, error) {
	resp, code, err := s.client.Do(ctx, "GET", "/v1/certify/.well-known/openid-credential-issuer", nil)
	if err != nil {
		fmt.Printf("inji: issuer metadata fetch failed: %v\n", err)
		return []model.CredentialConfig{}, nil
	}
	if code != 200 {
		fmt.Printf("inji: issuer metadata returned %d\n", code)
		return []model.CredentialConfig{}, nil
	}

	var metadata struct {
		CredentialConfigurationsSupported map[string]json.RawMessage `json:"credential_configurations_supported"`
	}
	if err := json.Unmarshal(resp, &metadata); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	configs := []model.CredentialConfig{}
	for configID, raw := range metadata.CredentialConfigurationsSupported {
		var obj map[string]any
		json.Unmarshal(raw, &obj)
		format := "ldp_vc"
		if f, ok := obj["format"].(string); ok {
			format = f
		}
		name := configID
		// Try to extract a display name
		if display, ok := obj["display"].([]any); ok && len(display) > 0 {
			if d, ok := display[0].(map[string]any); ok {
				if n, ok := d["name"].(string); ok {
					name = n
				}
			}
		}
		cat := categorizeCredentialType(configID, name)
		fields := extractFieldsFromMetadata(obj, format)
		configs = append(configs, model.CredentialConfig{
			ID:       configID,
			Name:     name,
			Category: cat,
			Format:   format,
			Fields:   fields,
		})
	}
	return configs, nil
}

// extractFieldsFromMetadata pulls the live claim-field list for a
// credential configuration out of Inji Certify's OID4VCI metadata.
//
// The field list lives in two different places depending on format:
//
//   - ldp_vc / mso_mdoc: credential_definition.credentialSubject is a
//     map whose keys are claim names (e.g. "fullName", "primaryCropType")
//     and whose values carry display labels. The keys are what we need
//     — they're the JSON claim keys Inji will accept at issue time.
//
//   - vc+sd-jwt: claims is a flat map with the same shape as above,
//     but promoted to the top level of the configuration because
//     SD-JWT doesn't use the W3C credential_definition wrapper.
//
// Returns nil if the metadata doesn't publish a per-type schema so the
// handler can fall back to a user-built schema or a static overlay.
// No type info is published by Inji — every field is recorded as a
// string with required=false; the user can refine before saving.
func extractFieldsFromMetadata(cfg map[string]any, format string) []model.SchemaField {
	var claims map[string]any
	if format == "vc+sd-jwt" || format == "sdjwt_vc" {
		if c, ok := cfg["claims"].(map[string]any); ok {
			claims = c
		}
	} else {
		if cd, ok := cfg["credential_definition"].(map[string]any); ok {
			if cs, ok := cd["credentialSubject"].(map[string]any); ok {
				claims = cs
			}
		}
	}
	if len(claims) == 0 {
		return nil
	}
	out := make([]model.SchemaField, 0, len(claims))
	for name := range claims {
		out = append(out, model.SchemaField{
			Name:     name,
			Type:     "string",
			Required: false,
		})
	}
	// Stable ordering so repeated calls produce the same wire output.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *issuerStore) RegisterCredentialType(ctx context.Context, typeName, displayName, description, format string) (string, error) {
	return "", fmt.Errorf("inji certify registers credential types via the certify.credential_config table (SQL seed); runtime registration is not supported in v1")
}

// =============================================================================
// VerifierStore — Inji Verify
// =============================================================================

type verifierStore struct {
	client transport.Client
}

// NewVerifierStore creates a VerifierStore backed by Inji Verify.
func NewVerifierStore(verifyClient transport.Client) store.VerifierStore {
	return &verifierStore{client: verifyClient}
}

func (s *verifierStore) Name() string { return "Inji Verify" }

func (s *verifierStore) Capabilities() model.VerifierCapabilities {
	return model.VerifierCapabilities{
		CreateRequest:          false, // Inji Verify is direct-verify: POST credential → result
		DirectVerify:           true,
		PresentationDefinition: false,
		PolicyEngine:           false,
		RevocationCheck:        true,
		DIDMethods:             []string{"did:web", "did:key", "did:jwk"},
	}
}

// CreateVerificationSession: Inji Verify does not use OID4VP sessions the same way
// Walt.id does. It offers direct verification of a submitted credential. For our UI,
// we return a "pseudo-session" with a client-generated state; the verification is
// performed synchronously when the holder presents.
func (s *verifierStore) CreateVerificationSession(ctx context.Context, req model.VerifyRequest) (*model.VerifyResult, error) {
	// Inji Verify uses a different flow — return a state and a pseudo URL that
	// our app can handle client-side. Verification happens via the direct-verify
	// endpoint when a credential is submitted.
	state := "inji-" + randomID(12)
	requestURL := "injiverify://verify?state=" + state
	return &model.VerifyResult{
		State:      state,
		RequestURL: requestURL,
	}, nil
}

// GetSessionResult returns an empty pending result. For Inji, verification results
// are obtained directly from the verify endpoint (see DirectVerify below).
func (s *verifierStore) GetSessionResult(ctx context.Context, state string) (*model.VerifyResult, error) {
	return &model.VerifyResult{
		State: state,
	}, nil
}

// DirectVerify sends a credential JWT/JSON directly to Inji Verify's vc-verification
// endpoint and returns the parsed result.
func (s *verifierStore) DirectVerify(ctx context.Context, credential []byte, contentType string) (*model.VerifyResult, error) {
	if contentType == "" {
		contentType = "application/vc+ld+json"
	}
	// transport.Client doesn't let us pass content type per request easily;
	// we pass the credential as a raw body and rely on the HTTPClient sniffing.
	resp, code, err := s.client.Do(ctx, "POST", "/v1/verify/vc-verification", string(credential))
	if err != nil {
		return nil, fmt.Errorf("inji verify: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("inji verify (%d): %s", code, truncate(string(resp), 300))
	}
	var out struct {
		VerificationStatus  string `json:"verificationStatus"`
		VerificationMessage string `json:"verificationMessage"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("parse verify response: %w", err)
	}
	verified := out.VerificationStatus == "SUCCESS"
	result := &model.VerifyResult{
		Verified: &verified,
	}
	if verified {
		result.Checks = []model.CheckResult{
			{Name: "Signature", Status: "pass", Summary: "Verified by Inji Verify"},
			{Name: "Proof", Status: "pass", Summary: "Proof chain valid"},
		}
	}
	return result, nil
}

func (s *verifierStore) ListPolicies(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"signature":  "Verify cryptographic signature",
		"revocation": "Check revocation status",
		"expiry":     "Verify validity dates",
	}, nil
}

// NOTE: The in-process holder/wallet store that used to live here has been
// moved to internal/store/localholder. It was never Inji-specific — it's a
// generic OID4VCI Pre-Auth Code flow client + in-memory credential bag,
// and naming it "Inji Holder" confused everyone.
//
// This file now only contains the Inji Certify issuer adaptor and the
// Inji Verify verifier adaptor — the parts that ARE Inji-specific.

// =============================================================================
// Helpers
// =============================================================================

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func extractHost(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	return host
}

func wrapOfferURL(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	// If the URI is already a full openid-credential-offer:// URL, return as-is.
	if strings.HasPrefix(uri, "openid-credential-offer://") || strings.HasPrefix(uri, "openid4vci://") {
		return uri
	}
	// Otherwise, build the standard wrapper.
	return "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(uri)
}

func categorizeCredentialType(id, name string) string {
	lower := strings.ToLower(id + " " + name)
	switch {
	case strings.Contains(lower, "degree") || strings.Contains(lower, "university") || strings.Contains(lower, "educational"):
		return "Education"
	case strings.Contains(lower, "farmer") || strings.Contains(lower, "farm"):
		return "Agriculture"
	case strings.Contains(lower, "birth") || strings.Contains(lower, "identity") || strings.Contains(lower, "national"):
		return "Identity"
	case strings.Contains(lower, "vaccin") || strings.Contains(lower, "health"):
		return "Health"
	case strings.Contains(lower, "license") || strings.Contains(lower, "driver"):
		return "Transport"
	default:
		return "Other"
	}
}

var randSrc = rand.New(rand.NewSource(time.Now().UnixNano()))

func randomID(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[randSrc.Intn(len(charset))]
	}
	return string(b)
}
