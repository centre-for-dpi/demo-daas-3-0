package waltid

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/httpx"
	"github.com/verifiably/verifiably-go/vctypes"
)

// base64urlDecode accepts base64url-encoded input with or without padding,
// as JWT spec specifies unpadded but some implementations add it.
func base64urlDecode(s string) ([]byte, error) {
	// RawURLEncoding handles both with-padding and without via the Strict
	// check being off; but to be safe, pad explicitly.
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.URLEncoding.DecodeString(s)
}

// accountRequest is walt.id's AccountRequest body shape; the email variant is
// what this adapter uses. Walt.id distinguishes variants by the fields present.
type accountRequest struct {
	Type     string `json:"type,omitempty"` // "email" etc.; walt.id matches on fields so this is advisory
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type walletListing struct {
	Account string      `json:"account"`
	Wallets []walletRef `json:"wallets"`
}

type walletRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CreatedOn   string `json:"createdOn"`
	AddedOn     string `json:"addedOn"`
	Permission  string `json:"permission"`
}

// ensureWalletSession registers-or-logs-in the configured demo account and
// caches a session token + wallet id. All wallet calls funnel through this.
func (a *Adapter) ensureWalletSession(ctx context.Context) (*walletSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session != nil {
		return a.session, nil
	}

	acc := a.cfg.DemoAccount
	if acc.Email == "" {
		acc.Email = "verifiably-demo@example.org"
	}
	if acc.Password == "" {
		acc.Password = generatePassword()
	}
	if acc.Name == "" {
		acc.Name = "Verifiably Demo"
	}
	body := accountRequest{
		Type:     "email",
		Name:     acc.Name,
		Email:    acc.Email,
		Password: acc.Password,
	}

	// Register first. Ignore "already exists" — login below will succeed.
	_ = a.wallet.DoJSON(ctx, "POST", "/wallet-api/auth/register", body, nil, nil)

	var tok loginResponse
	if err := a.wallet.DoJSON(ctx, "POST", "/wallet-api/auth/login", body, &tok, nil); err != nil {
		return nil, fmt.Errorf("wallet login: %w", err)
	}
	if tok.Token == "" {
		return nil, fmt.Errorf("wallet login: empty token")
	}

	// Wallet listing requires the session token in the Authorization header.
	authCtx := httpx.WithToken(ctx, tok.Token)
	var wl walletListing
	if err := a.wallet.DoJSON(authCtx, "GET", "/wallet-api/wallet/accounts/wallets", nil, &wl, nil); err != nil {
		return nil, fmt.Errorf("list wallets: %w", err)
	}
	if len(wl.Wallets) == 0 {
		return nil, fmt.Errorf("wallet login: no wallets for account")
	}
	a.session = &walletSession{Token: tok.Token, WalletID: wl.Wallets[0].ID}
	return a.session, nil
}

// ListWalletCredentials returns held credentials for the demo wallet.
func (a *Adapter) ListWalletCredentials(ctx context.Context) ([]vctypes.Credential, error) {
	sess, err := a.ensureWalletSession(ctx)
	if err != nil {
		return nil, err
	}
	var raw []map[string]json.RawMessage
	if err := a.wallet.DoJSON(httpx.WithToken(ctx, sess.Token), "GET",
		fmt.Sprintf("/wallet-api/wallet/%s/credentials", sess.WalletID),
		nil, &raw, nil); err != nil {
		return nil, err
	}
	out := make([]vctypes.Credential, 0, len(raw))
	for _, c := range raw {
		cred := walletCredentialToVctype(c)
		if cred.ID == "" {
			continue
		}
		out = append(out, cred)
	}
	return out, nil
}

// ListExampleOffers is used by the "paste example" helper. The registry drives
// bootstrap offers; this method stays here for adapter symmetry but returns an
// empty slice — the registry's ListExampleOffers aggregates BootstrapOffers
// across all issuer adapters instead.
func (a *Adapter) ListExampleOffers(_ context.Context) ([]string, error) {
	return nil, nil
}

// ParseOffer resolves an offer URI via /exchange/resolveCredentialOffer.
// Walt.id accepts the raw offer string as the request body (plain text) and
// returns a parsed CredentialOffer JSON we surface as a "pending" credential
// the operator can accept or reject.
//
// Errors from walt.id are surfaced in the returned error message so the UI
// toast tells the operator what went wrong — previously this swallowed the
// body and made paste failures look like "nothing happened".
func (a *Adapter) ParseOffer(ctx context.Context, offerURI string) (vctypes.Credential, error) {
	sess, err := a.ensureWalletSession(ctx)
	if err != nil {
		return vctypes.Credential{}, err
	}
	body, err := a.wallet.DoRaw(httpx.WithToken(ctx, sess.Token), "POST",
		fmt.Sprintf("/wallet-api/wallet/%s/exchange/resolveCredentialOffer", sess.WalletID),
		strings.NewReader(offerURI), "text/plain", nil)
	if err != nil {
		// Surface walt.id's own error body so the UI can explain why the
		// paste failed (e.g. unknown issuer, unparseable offer, signature
		// mismatch). Still wraps ErrOfferUnresolvable so handlers can branch
		// on typed error if needed.
		return vctypes.Credential{}, fmt.Errorf("%w: %v", backend.ErrOfferUnresolvable, err)
	}

	// Parse what we can out of the returned offer JSON to surface meaningful
	// preview text — credential type(s), issuer id — instead of an opaque
	// "Incoming credential" label.
	var parsed struct {
		CredentialIssuer              string   `json:"credential_issuer"`
		CredentialConfigurationIds    []string `json:"credential_configuration_ids"`
		Credentials                   []any    `json:"credentials"` // older shape
		Grants                        map[string]any `json:"grants"`
	}
	_ = json.Unmarshal(body, &parsed)

	title := "Incoming credential"
	configID := firstOr(parsed.CredentialConfigurationIds, "")
	if configID != "" {
		title = humanise(strings.SplitN(configID, "_", 2)[0])
	}
	issuer := parsed.CredentialIssuer
	if issuer == "" {
		issuer = "(unknown issuer)"
	}

	fields := map[string]string{
		"offer_uri": offerURI,
		"config_id": configID,
	}
	// Best-effort: fetch the issuer's well-known openid-credential-issuer
	// metadata and copy in the display name + claim slots the holder will
	// receive if they accept. The pending card has no claim VALUES — the
	// wallet only learns those after claiming — but knowing WHICH fields
	// are coming + the issuer's display name is meaningful context.
	if issuer != "" && configID != "" {
		if slots, display := fetchCredentialSlots(ctx, issuer, configID); display != "" || len(slots) > 0 {
			if display != "" {
				title = display
			}
			if len(slots) > 0 {
				fields["offered_fields"] = strings.Join(slots, ", ")
			}
		}
	}

	return vctypes.Credential{
		ID:     "pending-" + randomHex(4),
		Title:  title,
		Issuer: issuer,
		Type:   "w3c_vcdm_2",
		Status: "pending",
		Fields: fields,
	}, nil
}

// fetchCredentialSlots reads the issuer's well-known
// openid-credential-issuer document, looks up the configID, and returns
// the display name + claim keys. Best-effort: any network error returns
// empty values so the caller falls back to the offer-only preview.
func fetchCredentialSlots(ctx context.Context, issuerBase, configID string) (slots []string, display string) {
	u := strings.TrimRight(issuerBase, "/") + "/.well-known/openid-credential-issuer"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, ""
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ""
	}
	var doc struct {
		CredentialConfigurationsSupported map[string]struct {
			Display []struct {
				Name string `json:"name"`
			} `json:"display"`
			CredentialDefinition struct {
				CredentialSubject map[string]any `json:"credentialSubject"`
			} `json:"credential_definition"`
			Claims map[string]any `json:"claims"`
		} `json:"credential_configurations_supported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, ""
	}
	cfg, ok := doc.CredentialConfigurationsSupported[configID]
	if !ok {
		return nil, ""
	}
	if len(cfg.Display) > 0 && cfg.Display[0].Name != "" {
		display = cfg.Display[0].Name
	}
	// Prefer credential_definition.credentialSubject (W3C VCDM shape);
	// fall back to the flat claims map (SD-JWT VC shape).
	pool := cfg.CredentialDefinition.CredentialSubject
	if len(pool) == 0 {
		pool = cfg.Claims
	}
	for k := range pool {
		slots = append(slots, k)
	}
	return slots, display
}

func firstOr(xs []string, fallback string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return fallback
}

// ClaimCredential consummates the offer via /exchange/useOfferRequest. Walt.id
// accepts the offer URI as plain text body; query params control the did used
// and whether user input is required.
//
// After the claim succeeds, we re-list the wallet and find the credential we
// just added so the returned vctypes.Credential carries its real
// credentialSubject fields. Without this, the card that replaces the pending
// one would still only show offer metadata — the claim values would only
// appear after a subsequent /holder/wallet fetch.
func (a *Adapter) ClaimCredential(ctx context.Context, cred vctypes.Credential) (vctypes.Credential, error) {
	sess, err := a.ensureWalletSession(ctx)
	if err != nil {
		return cred, err
	}
	offerURI := cred.Fields["offer_uri"]
	if offerURI == "" {
		return cred, fmt.Errorf("claim credential: missing offer_uri on pending cred")
	}
	q := url.Values{"requireUserInput": {"false"}}
	path := fmt.Sprintf("/wallet-api/wallet/%s/exchange/useOfferRequest?%s", sess.WalletID, q.Encode())
	_, err = a.wallet.DoRaw(httpx.WithToken(ctx, sess.Token), "POST", path,
		strings.NewReader(offerURI), "text/plain", nil)
	if err != nil {
		return cred, friendlyClaimError(err, cred.Fields["config_id"])
	}
	cred.Status = "accepted"

	// Best-effort: list the wallet and pick the newest credential whose
	// config id matches this offer — that's almost always the one we just
	// claimed. Walt.id's useOfferRequest response doesn't echo the stored
	// credential's id, so we can't look up by primary key.
	held, err := a.ListWalletCredentials(ctx)
	if err != nil || len(held) == 0 {
		return cred, nil
	}
	configID := cred.Fields["config_id"]
	var match *vctypes.Credential
	for i := range held {
		h := &held[i]
		if configID != "" && h.Fields["config_id"] == configID {
			match = h
			break
		}
	}
	if match == nil {
		// No config_id match — fall back to the last credential listed, which
		// walt.id emits in insertion order.
		match = &held[len(held)-1]
	}
	// Preserve the pending card's ID so the HTMX swap replaces the right card,
	// but copy over everything else from the just-claimed credential.
	pendingID := cred.ID
	cred = *match
	cred.ID = pendingID
	cred.Status = "accepted"
	return cred, nil
}

// PresentCredential responds to an OID4VP request via /exchange/usePresentationRequest.
//
// Two call shapes are tried in order to cover walt.id's wallet-api versions:
//
//   1. Match-then-present. Calls /exchange/matchCredentialsForPresentationDefinition
//      first so the wallet resolves the PD URL, fetches the definition, and
//      returns the credentials that match. If that succeeds we submit with
//      the wallet's own canonical credential-id (which can differ from the
//      id surfaced by ListWalletCredentials when walt.id re-emits ids
//      per-presentation). If the match call fails we continue to step 2 —
//      some older wallet-api builds don't expose the match endpoint.
//
//   2. Direct submit with the caller-provided CredentialID. This is the
//      original code path; kept as a fallback because it works on builds
//      where matchCredentialsForPresentationDefinition is missing.
//
// Either way, the raw 400 body is surfaced verbatim to the caller so the
// UI toast shows the walt.id error (previously the user saw
// "Bad Request" with no detail).
func (a *Adapter) PresentCredential(ctx context.Context, req backend.PresentCredentialRequest) (backend.PresentCredentialResult, error) {
	sess, err := a.ensureWalletSession(ctx)
	if err != nil {
		return backend.PresentCredentialResult{}, err
	}
	authCtx := httpx.WithToken(ctx, sess.Token)

	// Pre-flight: fetch the verifier's PD and ask the wallet which of its
	// credentials satisfy it. If none do, fail fast with an error message
	// that explains exactly which credential type + format the verifier
	// wanted — otherwise the submit would produce a raw walt.id 400 saying
	// only "presentationDefinitionMatch:false" which gives the user no
	// hint about what to fix.
	pd := a.fetchPresentationDefinition(authCtx, req.RequestURI)
	credID := req.CredentialID
	if pd != nil {
		matched, ok := a.matchPD(authCtx, sess.WalletID, pd)
		if ok && len(matched) == 0 {
			wantType, wantFormat := describePD(pd)
			return backend.PresentCredentialResult{}, fmt.Errorf(
				"your wallet has no credential matching this request (verifier asked for %s in %s format); accept a matching offer first, then try again",
				wantType, wantFormat)
		}
		if ok {
			credID = pickBestMatch(matched, req.CredentialID)
		}
	}

	body := map[string]any{
		"presentationRequest": req.RequestURI,
		"selectedCredentials": []string{credID},
	}
	if len(req.DisclosedClaim) > 0 {
		body["disclosures"] = map[string][]string{credID: req.DisclosedClaim}
	}
	respRaw, err := a.wallet.DoRaw(authCtx, "POST",
		fmt.Sprintf("/wallet-api/wallet/%s/exchange/usePresentationRequest", sess.WalletID),
		jsonReaderBytes(mustJSON(body)), "application/json", nil)
	if err != nil {
		return backend.PresentCredentialResult{}, friendlyPresentError(err)
	}
	redirectURI := ""
	if len(respRaw) > 0 {
		var parsed struct {
			RedirectURI string `json:"redirectUri"`
		}
		_ = json.Unmarshal(respRaw, &parsed)
		redirectURI = parsed.RedirectURI
	}
	return backend.PresentCredentialResult{
		Success:       true,
		Method:        "OID4VP · via wallet API",
		SharedClaims:  req.DisclosedClaim,
		VerifierState: redirectURI,
	}, nil
}

// vpFormatRank returns a score for a credential format based on whether
// walt.id's wallet-api has a tested VP submit path for it. Formats
// outside the canonical two (jwt_vc_json, vc+sd-jwt) crash the wallet's
// internal SD-JWT-suffix assertion when built into a vp_token.
func vpFormatRank(f string) int {
	switch f {
	case "jwt_vc_json":
		return 100
	case "vc+sd-jwt":
		return 90
	case "dc+sd-jwt":
		return 85
	case "mso_mdoc":
		return 70
	default:
		return 0
	}
}

// fetchPresentationDefinition extracts presentation_definition_uri from an
// openid4vp:// request URI, GETs the PD from walt.id's verifier, and
// returns the decoded JSON object. The GET goes through a.verifier
// (the httpx.Client bound to the docker-internal verifier URL) so the
// fetch works from inside the verifiably-go container — `localhost:7003`
// in the request URI doesn't resolve inside the container, but the
// path after it is the same on both host and container views of the
// verifier.
func (a *Adapter) fetchPresentationDefinition(ctx context.Context, requestURI string) map[string]any {
	idx := strings.Index(requestURI, "presentation_definition_uri=")
	if idx < 0 {
		return nil
	}
	encoded := requestURI[idx+len("presentation_definition_uri="):]
	if amp := strings.IndexByte(encoded, '&'); amp >= 0 {
		encoded = encoded[:amp]
	}
	pdURL, err := url.QueryUnescape(encoded)
	if err != nil {
		return nil
	}
	// Strip the scheme+host so the path lands on whatever URL a.verifier
	// was configured with (docker-internal name when containerized, host
	// URL otherwise). Path shape is the same on both forms.
	if u, err := url.Parse(pdURL); err == nil {
		pdURL = u.Path
		if u.RawQuery != "" {
			pdURL += "?" + u.RawQuery
		}
	}
	var out map[string]any
	if err := a.verifier.DoJSON(ctx, http.MethodGet, pdURL, nil, &out, nil); err != nil {
		return nil
	}
	return out
}

// matchPD calls walt.id's match endpoint with an inline PD. Returns
// (matches, ok). ok=false means the endpoint itself errored (network
// fault, older wallet-api build without the endpoint); callers must
// NOT treat that as "no matches found" — fall back to submitting with
// the user-picked id and let walt.id return its own error.
func (a *Adapter) matchPD(ctx context.Context, walletID string, pd map[string]any) ([]map[string]json.RawMessage, bool) {
	var matched []map[string]json.RawMessage
	if err := a.wallet.DoJSON(ctx, "POST",
		fmt.Sprintf("/wallet-api/wallet/%s/exchange/matchCredentialsForPresentationDefinition", walletID),
		pd, &matched, nil); err != nil {
		log.Printf("waltid: matchPD failed: %v", err)
		return nil, false
	}
	return matched, true
}

// pickBestMatch picks the highest-ranked (walt.id-tested format) id from
// the match results. Falls back to the user-picked id if the ranking
// produced no winner (all 0-ranked).
func pickBestMatch(matched []map[string]json.RawMessage, fallback string) string {
	best := -1
	bestID := fallback
	for _, row := range matched {
		var id, fmtVal string
		_ = json.Unmarshal(row["id"], &id)
		_ = json.Unmarshal(row["format"], &fmtVal)
		if id == "" {
			continue
		}
		rank := vpFormatRank(fmtVal)
		log.Printf("waltid: match candidate id=%s format=%s rank=%d", id, fmtVal, rank)
		if rank > best {
			best = rank
			bestID = id
		}
	}
	log.Printf("waltid: picked id=%s rank=%d from %d matches", bestID, best, len(matched))
	return bestID
}

// describePD extracts a human-readable (type, format) pair from a PD's
// first input descriptor. Used to explain failures when the wallet has
// nothing matching — the UI can tell the user "you need a <type> in
// <format> format" instead of the opaque walt.id error.
func describePD(pd map[string]any) (typeName, format string) {
	typeName = "VerifiableCredential"
	format = "any"
	descriptors, _ := pd["input_descriptors"].([]any)
	if len(descriptors) == 0 {
		return
	}
	d, _ := descriptors[0].(map[string]any)
	if d == nil {
		return
	}
	// Format is a map keyed by the VP format name (e.g. "vc+sd-jwt").
	if fm, ok := d["format"].(map[string]any); ok {
		for k := range fm {
			format = k
			break
		}
	}
	// Type is declared via constraints.fields[*].filter.const|pattern or
	// via the input-descriptor's id on walt.id's generated PDs.
	if id, ok := d["id"].(string); ok && id != "" {
		typeName = id
	}
	return
}

// friendlyClaimError surfaces the wallet's error verbatim; empirically
// walt.id v0.18.2 claims every advertised format (jwt_vc_json, jwt_vc_json-ld,
// ldp_vc, jwt_vc, vc+sd-jwt) cleanly when the issuer-side body is correct.
// dc+sd-jwt is the one exception — issuance there requires the issuer DID
// for the VC-subject id and walt.id's wallet-api 400s with
// "Issuer DID was not given in issuance request" when it isn't. That's an
// issuer-API quirk, not a format deficiency.
func friendlyClaimError(err error, _ string) error {
	return fmt.Errorf("wallet claim failed: %s", truncateClaim(err.Error(), 200))
}

// formatFromConfigID extracts the walt.id format key from a configuration
// id like "AlpsTourReservation_jwt_vc_json-ld" → "jwt_vc_json-ld". Returns
// "" when none of the known suffixes match (e.g. when configID is empty).
func formatFromConfigID(configID string) string {
	for _, suf := range []string{
		"jwt_vc_json-ld", "jwt_vc_json", "vc+sd-jwt", "dc+sd-jwt",
		"mso_mdoc", "ldp_vc", "jwt_vc",
	} {
		if strings.HasSuffix(configID, "_"+suf) {
			return suf
		}
	}
	return ""
}

func truncateClaim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// friendlyPresentError translates walt.id's wallet 400 body into a message
// a non-walt.id-literate operator can act on. Empirically (against
// walt.id Community Stack v0.18.2):
//
//   - "JsonArray is not a JsonPrimitive" fires when the wallet tries to
//     build a vp_token for a credential in a format whose VP payload is a
//     JSON object/array (ldp_vc, jwt_vc_json-ld) rather than a compact
//     JWT string. walt.id's VP submit path calls .jsonPrimitive on the
//     vpToken unconditionally (SSIKit2WalletService.kt) — fine for
//     jwt_vc_json / vc+sd-jwt, throws for the others.
//   - "VCFormat does not contain element with name 'jwt_vc_json-ld'" —
//     the verifier's Kotlin format enum is missing that literal, so
//     creating the verifier session for a jwt_vc_json-ld filter fails.
//   - "presentationDefinitionMatch" + "false" — no held credential
//     satisfies the PD; the pre-flight in PresentCredential usually
//     catches this first with a more specific message.
func friendlyPresentError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "JsonArray") && strings.Contains(msg, "JsonPrimitive"):
		return fmt.Errorf(
			"walt.id's wallet-api v0.18.2 can't build a verifiable presentation for this credential format — its VP submit path only handles compact-JWT formats (jwt_vc_json, vc+sd-jwt). For jwt_vc_json-ld and ldp_vc the vpToken is a JSON object and the wallet throws internally. Re-issue the credential in JWT · W3C (jwt_vc_json) or SD-JWT · VC (vc+sd-jwt) and retry")
	case strings.Contains(msg, "VCFormat does not contain"):
		return fmt.Errorf(
			"walt.id's verifier doesn't recognise this format (e.g. jwt_vc_json-ld is not in its Kotlin format enum). Re-issue the credential in a format the verifier supports — jwt_vc_json, ldp_vc, jwt_vc, or vc+sd-jwt all round-trip end-to-end")
	case strings.Contains(msg, "presentationDefinitionMatch") && strings.Contains(msg, "false"):
		return fmt.Errorf("the wallet couldn't build a presentation the verifier would accept — typically the held credential's format doesn't match what was requested")
	case strings.Contains(msg, "signature") && strings.Contains(msg, "policy"):
		return fmt.Errorf("the credential's signature could not be verified by the verifier (the issuer's key may not be trusted by this verifier)")
	}
	return err
}

// jsonReaderBytes adapts a precomputed []byte for DoRaw's io.Reader argument.
func jsonReaderBytes(b []byte) *strings.Reader { return strings.NewReader(string(b)) }

// mustJSON marshals v or returns a JSON null on error. Used when we're
// building a small, known-safe map in code — failures are programmer bugs,
// not runtime errors.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

// walletCredentialToVctype maps a walt.id WalletCredential onto vctypes.Credential.
// The walt.id shape varies by credential format; we extract claims from
// whichever body field is populated:
//   - `parsedDocument` for JSON-LD VCs (already decoded)
//   - `document` / `jwt` for JWT-style VCs (a compact JWS whose payload holds `vc`)
//
// All scalar claim types (string, number, boolean) are rendered into a
// string for display; object/array values (address, dependents, etc.) are
// JSON-encoded so the card still surfaces them instead of silently
// dropping them, which was the symptom users saw for VCs that carry
// anything other than flat strings.
func walletCredentialToVctype(raw map[string]json.RawMessage) vctypes.Credential {
	var id string
	_ = json.Unmarshal(raw["id"], &id)
	var format string
	_ = json.Unmarshal(raw["format"], &format)

	cred := vctypes.Credential{
		ID:     id,
		Status: "accepted",
		Type:   stdFromFormat(format),
		Fields: map[string]string{},
	}

	// Prefer parsedDocument (JSON-LD, already-decoded). Fall back to
	// decoding the `document` or `jwt` compact JWS and reading the `vc`
	// payload claim.
	parsed := pickParsedDocument(raw)
	if parsed != nil {
		if issuer := parsed["issuer"]; issuer != nil {
			cred.Issuer = issuerString(issuer)
		}
		var types []string
		if err := json.Unmarshal(parsed["type"], &types); err == nil && len(types) > 1 {
			cred.Title = humanise(types[len(types)-1])
		}
		var subject map[string]any
		if err := json.Unmarshal(parsed["credentialSubject"], &subject); err == nil {
			for k, v := range subject {
				if k == "id" {
					continue
				}
				cred.Fields[k] = stringifyClaim(v)
			}
		}
	}
	if cred.Title == "" {
		cred.Title = "Credential"
	}
	if cred.Issuer == "" {
		cred.Issuer = "Unknown issuer"
	}
	return cred
}

// pickParsedDocument returns the decoded VC body from whichever walt.id
// field carries it. JSON-LD VCs use parsedDocument directly; JWT-encoded
// VCs store a compact three-dot JWS in `document` or `jwt`, whose payload
// holds the VC claim either at the top level or nested under `vc`.
func pickParsedDocument(raw map[string]json.RawMessage) map[string]json.RawMessage {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw["parsedDocument"], &parsed); err == nil && len(parsed) > 0 {
		return parsed
	}
	for _, field := range []string{"document", "jwt"} {
		var jws string
		if err := json.Unmarshal(raw[field], &jws); err != nil || jws == "" {
			continue
		}
		if payload := decodeJWSPayload(jws); payload != nil {
			if vcRaw, ok := payload["vc"]; ok {
				var vc map[string]json.RawMessage
				if err := json.Unmarshal(vcRaw, &vc); err == nil {
					return vc
				}
			}
			return payload
		}
	}
	return nil
}

// decodeJWSPayload base64url-decodes the middle segment of a compact JWS
// and unmarshals it as a JSON object. Returns nil on any error.
func decodeJWSPayload(jws string) map[string]json.RawMessage {
	parts := strings.SplitN(jws, ".", 3)
	if len(parts) < 2 {
		return nil
	}
	body, err := base64urlDecode(parts[1])
	if err != nil {
		return nil
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

// stringifyClaim coerces any JSON claim value into a human-readable string.
// Scalars render as themselves; containers (objects, arrays) are
// JSON-encoded so the card still surfaces them rather than silently
// dropping non-string claim values.
func stringifyClaim(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// json.Unmarshal into any gives float64 for numbers; trim the
		// trailing .0 for integers so "25" doesn't render as "25.0".
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func issuerString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if id, ok := obj["id"].(string); ok {
			return id
		}
	}
	return ""
}

func stdFromFormat(format string) string {
	switch format {
	case "jwt_vc_json", "jwt_vc_json-ld", "ldp_vc":
		return "w3c_vcdm_2"
	case "vc+sd-jwt", "dc+sd-jwt":
		return "sd_jwt_vc (IETF)"
	case "mso_mdoc":
		return "mso_mdoc"
	default:
		return "w3c_vcdm_2"
	}
}

// humanise converts CamelCase to "Camel Case".
func humanise(s string) string {
	var out []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	return string(out)
}

func generatePassword() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// referenced to keep the import used
var _ http.Request
