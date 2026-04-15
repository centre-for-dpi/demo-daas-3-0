package localholder

// oid4vci_client.go — Minimal server-side OID4VCI Pre-Authorized Code flow
// client. Used by the localholder.holderStore to claim credentials from any
// OID4VCI-compliant issuer — our own /local-issuer, Inji Certify, or any
// other standards-conformant backend.
//
// The client:
//  1. Parses the openid-credential-offer:// URL
//  2. Fetches the credential_offer JSON
//  3. Fetches the issuer metadata (for token_endpoint, credential_endpoint)
//  4. Exchanges the pre-authorized_code for an access token + c_nonce
//  5. Generates an ECDSA P-256 holder keypair
//  6. Builds did:jwk from the public key and signs an OID4VCI proof JWT
//  7. POSTs the credential request to credential_endpoint
//  8. Returns the raw credential JSON for storage in the wallet bag
//
// Supports format=ldp_vc (JSON-LD proof) and credential_definition with type+@context.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// credentialOffer is the OID4VCI credential offer JSON (draft 13).
type credentialOffer struct {
	CredentialIssuer            string            `json:"credential_issuer"`
	CredentialConfigurationIDs  []string          `json:"credential_configuration_ids"`
	Grants                      map[string]any    `json:"grants"`
}

// tokenResponse is the /oauth/token reply.
type tokenResponse struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	ExpiresIn          int    `json:"expires_in"`
	CNonce             string `json:"c_nonce"`
	CNonceExpiresIn    int    `json:"c_nonce_expires_in"`
}

// ClaimOID4VCICredential runs the full OID4VCI Pre-Authorized Code flow for
// a single offer URL and returns the raw credential JSON the issuer returned.
//
// Works against any OID4VCI-compliant backend — our own /local-issuer,
// Inji Certify, or any standards-conformant issuer.
//
// Intentionally self-contained: no external JWT libraries, stdlib only.
func ClaimOID4VCICredential(ctx context.Context, offerURL string) ([]byte, error) {
	// Step 1: parse the offer URL → extract credential_offer_uri or credential_offer
	offerJSONURL, err := extractOfferURI(offerURL)
	if err != nil {
		return nil, fmt.Errorf("parse offer url: %w", err)
	}

	// Step 2: fetch the credential_offer JSON
	offer, err := fetchCredentialOffer(ctx, offerJSONURL)
	if err != nil {
		return nil, fmt.Errorf("fetch credential offer: %w", err)
	}
	if len(offer.CredentialConfigurationIDs) == 0 {
		return nil, fmt.Errorf("offer has no credential_configuration_ids")
	}
	configID := offer.CredentialConfigurationIDs[0]

	preAuth, ok := offer.Grants["urn:ietf:params:oauth:grant-type:pre-authorized_code"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("offer does not include a pre-authorized_code grant")
	}
	code, _ := preAuth["pre-authorized_code"].(string)
	if code == "" {
		return nil, fmt.Errorf("offer missing pre-authorized_code")
	}

	// Step 3: fetch issuer metadata
	issuerBase := strings.TrimRight(offer.CredentialIssuer, "/")
	// Rewrite internal hostname if set — Inji's configured domain_url is
	// certify-nginx:80 which isn't reachable from our host; swap for localhost:8091.
	issuerBase = rewriteForHost(issuerBase)

	md, err := fetchIssuerMetadata(ctx, issuerBase)
	if err != nil {
		return nil, fmt.Errorf("fetch issuer metadata: %w", err)
	}

	tokenEndpoint, _ := md["token_endpoint"].(string)
	if tokenEndpoint == "" {
		// Fallback: try Inji's known path
		tokenEndpoint = issuerBase + "/v1/certify/oauth/token"
	}
	tokenEndpoint = rewriteForHost(tokenEndpoint)

	credEndpoint, _ := md["credential_endpoint"].(string)
	if credEndpoint == "" {
		credEndpoint = issuerBase + "/v1/certify/issuance/credential"
	}
	credEndpoint = rewriteForHost(credEndpoint)

	// Step 4: exchange pre-auth code for access token
	tokResp, err := exchangePreAuthCode(ctx, tokenEndpoint, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	// Step 5: generate holder ECDSA P-256 keypair
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keygen: %w", err)
	}

	// Step 6: build did:jwk + sign proof JWT
	did, err := buildDIDJWK(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("did:jwk: %w", err)
	}
	proofJWT, err := signProofJWT(priv, did, offer.CredentialIssuer, tokResp.CNonce)
	if err != nil {
		return nil, fmt.Errorf("proof jwt: %w", err)
	}
	fmt.Printf("oid4vci: proof jwt (full): %s\n", proofJWT)

	// Step 7: build credential request body
	// Extract the config to read its format + credential_definition
	cfg, _ := extractConfig(md, configID)
	format, _ := cfg["format"].(string)
	if format == "" {
		format = "ldp_vc"
	}
	credentialDefinition, _ := cfg["credential_definition"].(map[string]any)
	if credentialDefinition == nil {
		credentialDefinition = map[string]any{
			"type":     []string{"VerifiableCredential"},
			"@context": []string{"https://www.w3.org/ns/credentials/v2"},
		}
	}
	// Inji's LdpVcCredentialRequestValidator requires @context + type.
	if _, ok := credentialDefinition["@context"]; !ok {
		credentialDefinition["@context"] = []string{"https://www.w3.org/ns/credentials/v2"}
	}
	if _, ok := credentialDefinition["type"]; !ok {
		credentialDefinition["type"] = []string{"VerifiableCredential"}
	}

	body := map[string]any{
		"format": format,
		"credential_definition": credentialDefinition,
		"proof": map[string]any{
			"proof_type": "jwt",
			"jwt":        proofJWT,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	// Step 8: POST credential request
	req, _ := http.NewRequestWithContext(ctx, "POST", credEndpoint, bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+tokResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credential request: %w", err)
	}
	defer resp.Body.Close()
	credBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, fmt.Errorf("credential request %d: %s", resp.StatusCode, truncate(string(credBody), 500))
	}

	// Inji returns {"credential": {...}, "format": "...", "c_nonce": "..."}.
	// Extract the credential so callers see a clean VC JSON.
	var wrapped map[string]any
	if err := json.Unmarshal(credBody, &wrapped); err == nil {
		if inner, ok := wrapped["credential"]; ok {
			if innerBytes, err := json.Marshal(inner); err == nil {
				return innerBytes, nil
			}
		}
	}
	return credBody, nil
}

// ---- URL + offer parsing ----------------------------------------------------

func extractOfferURI(u string) (string, error) {
	u = strings.TrimPrefix(u, "openid-credential-offer://")
	u = strings.TrimPrefix(u, "openid4vci://")
	u = strings.TrimPrefix(u, "?")
	// u is now a query string: credential_offer_uri=... or credential_offer=...
	values, err := url.ParseQuery(u)
	if err != nil {
		return "", err
	}
	if uri := values.Get("credential_offer_uri"); uri != "" {
		return uri, nil
	}
	if raw := values.Get("credential_offer"); raw != "" {
		// Inline offer — not supported in this minimal client yet.
		return "", fmt.Errorf("inline credential_offer not supported; expected credential_offer_uri")
	}
	return "", fmt.Errorf("offer url missing credential_offer_uri")
}

func fetchCredentialOffer(ctx context.Context, rawURL string) (*credentialOffer, error) {
	rawURL = rewriteForHost(rawURL)
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch offer %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	// Inji Certify (and other MOSIP services) wrap responses in a
	// {responseTime, response, errors} envelope and return HTTP 200 even
	// for business errors like "credential offer not found or expired".
	// Sniff for that shape first so we can surface the real error message
	// instead of misreading a null response as a malformed offer.
	var maybeEnvelope struct {
		Response any `json:"response"`
		Errors   []struct {
			ErrorCode    string `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"errors"`
	}
	hasEnvelope := false
	if err := json.Unmarshal(body, &maybeEnvelope); err == nil {
		// Treat as the MOSIP envelope if either field is present in any form.
		// `errors` being a non-nil slice (even empty) signals the wrapper.
		if maybeEnvelope.Errors != nil || maybeEnvelope.Response != nil {
			hasEnvelope = true
		}
	}
	if hasEnvelope {
		if len(maybeEnvelope.Errors) > 0 {
			e := maybeEnvelope.Errors[0]
			return nil, fmt.Errorf("issuer rejected offer fetch: %s — %s "+
				"(generate a fresh offer; pre-auth offers are short-lived "+
				"and don't survive an issuer restart)", e.ErrorCode, e.ErrorMessage)
		}
		// Unwrap and re-marshal the inner response so we can decode it as
		// the actual credential offer struct.
		if maybeEnvelope.Response == nil {
			return nil, fmt.Errorf("issuer returned an empty wrapped response with no errors — likely an offer-cache hit miss")
		}
		inner, err := json.Marshal(maybeEnvelope.Response)
		if err != nil {
			return nil, fmt.Errorf("re-marshal wrapped offer: %w", err)
		}
		var offer credentialOffer
		if err := json.Unmarshal(inner, &offer); err != nil {
			return nil, fmt.Errorf("parse wrapped offer: %w", err)
		}
		return &offer, nil
	}

	// Plain OID4VCI offer (this is what Inji Certify actually returns on the
	// happy path — the envelope is only used for errors).
	var offer credentialOffer
	if err := json.Unmarshal(body, &offer); err != nil {
		return nil, err
	}
	return &offer, nil
}

func fetchIssuerMetadata(ctx context.Context, base string) (map[string]any, error) {
	u := base + "/.well-known/openid-credential-issuer"
	// Inji Certify serves metadata at both /.well-known/... and /v1/certify/.well-known/...
	// Try the bare path first; fall back to the v1/certify path.
	md, err := fetchJSON(ctx, u)
	if err == nil {
		return md, nil
	}
	u2 := base + "/v1/certify/.well-known/openid-credential-issuer"
	return fetchJSON(ctx, u2)
}

func fetchJSON(ctx context.Context, u string) (map[string]any, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s → %d: %s", u, resp.StatusCode, truncate(string(body), 200))
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func extractConfig(md map[string]any, configID string) (map[string]any, bool) {
	ccs, ok := md["credential_configurations_supported"].(map[string]any)
	if !ok {
		return nil, false
	}
	cfg, ok := ccs[configID].(map[string]any)
	return cfg, ok
}

// rewriteForHost maps docker-only hostnames to their host-accessible
// equivalents. Needed because our Go server runs on the host, not inside
// the waltid_default docker network:
//
//   certify-nginx:80       → localhost:8091  (Inji Certify via nginx)
//   inji-certify:8090      → localhost:8090  (Inji Certify direct)
//   host.docker.internal:X → localhost:X     (our own OID4VCI server
//                                              advertised for docker wallets)
func rewriteForHost(u string) string {
	u = strings.ReplaceAll(u, "http://certify-nginx:80", "http://localhost:8091")
	u = strings.ReplaceAll(u, "https://certify-nginx:80", "https://localhost:8091")
	u = strings.ReplaceAll(u, "http://inji-certify:8090", "http://localhost:8090")
	u = strings.ReplaceAll(u, "http://host.docker.internal", "http://localhost")
	u = strings.ReplaceAll(u, "https://host.docker.internal", "https://localhost")
	return u
}

// ---- token exchange ---------------------------------------------------------

func exchangePreAuthCode(ctx context.Context, tokenEndpoint, code string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:pre-authorized_code")
	form.Set("pre-authorized_code", code)

	req, _ := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tr, nil
}

// ---- did:jwk + proof JWT ---------------------------------------------------

// buildDIDJWK encodes an ECDSA P-256 public key as a did:jwk.
// did:jwk = "did:jwk:" + base64url(json({"kty":"EC","crv":"P-256","x":...,"y":...}))
func buildDIDJWK(pub *ecdsa.PublicKey) (string, error) {
	// Pad coordinates to 32 bytes (P-256).
	xBytes := padTo32(pub.X.Bytes())
	yBytes := padTo32(pub.Y.Bytes())
	jwk := map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64url(xBytes),
		"y":   base64url(yBytes),
	}
	b, err := json.Marshal(jwk)
	if err != nil {
		return "", err
	}
	return "did:jwk:" + base64url(b), nil
}

func padTo32(b []byte) []byte {
	if len(b) == 32 {
		return b
	}
	if len(b) > 32 {
		return b[len(b)-32:]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

// signProofJWT builds and signs the OID4VCI proof JWT using ES256.
//
// Only `kid` is set in the header (pointing at the did:jwk) because Inji
// rejects proofs that include BOTH `jwk` and `kid`
// (error: proof_header_ambiguous_key).
// `iss` is intentionally omitted from the payload: OID4VCI draft-13 §7.2.1.1
// requires iss to be omitted when the access token was obtained via an
// anonymous Pre-Authorized Code Flow (no pre-registered client_id).
// Body: {"aud":issuer,"iat":now,"nonce":c_nonce}
func signProofJWT(priv *ecdsa.PrivateKey, did, audience, nonce string) (string, error) {
	_ = did // retained for potential future use when Inji supports iss=did
	header := map[string]any{
		"alg": "ES256",
		"typ": "openid4vci-proof+jwt",
		"kid": did + "#0",
	}
	payload := map[string]any{
		"aud":   audience,
		"iat":   time.Now().Unix(),
		"nonce": nonce,
	}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := base64url(hb) + "." + base64url(pb)

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		return "", err
	}
	// JWS ES256 signature is r||s as 32-byte big-endian integers (not ASN.1/DER).
	sig := make([]byte, 64)
	copy(sig[32-len(r.Bytes()):32], r.Bytes())
	copy(sig[64-len(s.Bytes()):64], s.Bytes())
	return signingInput + "." + base64url(sig), nil
}

func base64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// truncate shortens a string to n runes and appends an ellipsis. Used for
// logging bodies without flooding stdout.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Sanity check so math/big is actually used (paranoia on unused-import linting).
var _ = big.NewInt(0)
