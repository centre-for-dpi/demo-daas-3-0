package waltid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/httpx"
	"github.com/verifiably/verifiably-go/vctypes"
)

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
	if len(parsed.CredentialConfigurationIds) > 0 {
		title = humanise(strings.SplitN(parsed.CredentialConfigurationIds[0], "_", 2)[0])
	}
	issuer := parsed.CredentialIssuer
	if issuer == "" {
		issuer = "(unknown issuer)"
	}

	return vctypes.Credential{
		ID:     "pending-" + randomHex(4),
		Title:  title,
		Issuer: issuer,
		Type:   "w3c_vcdm_2",
		Status: "pending",
		Fields: map[string]string{
			"offer_uri": offerURI,
			"config_id": firstOr(parsed.CredentialConfigurationIds, ""),
		},
	}, nil
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
		return cred, err
	}
	cred.Status = "accepted"
	return cred, nil
}

// PresentCredential responds to an OID4VP request via /exchange/usePresentationRequest.
func (a *Adapter) PresentCredential(ctx context.Context, req backend.PresentCredentialRequest) (backend.PresentCredentialResult, error) {
	sess, err := a.ensureWalletSession(ctx)
	if err != nil {
		return backend.PresentCredentialResult{}, err
	}
	body := map[string]any{
		"presentationRequest": req.RequestURI,
		"selectedCredentials": []string{req.CredentialID},
	}
	if len(req.DisclosedClaim) > 0 {
		body["disclosures"] = map[string][]string{req.CredentialID: req.DisclosedClaim}
	}
	var resp struct {
		RedirectURI string `json:"redirectUri"`
	}
	if err := a.wallet.DoJSON(httpx.WithToken(ctx, sess.Token), "POST",
		fmt.Sprintf("/wallet-api/wallet/%s/exchange/usePresentationRequest", sess.WalletID),
		body, &resp, nil); err != nil {
		return backend.PresentCredentialResult{}, err
	}
	return backend.PresentCredentialResult{
		Success:       true,
		Method:        "OID4VP · via wallet API",
		SharedClaims:  req.DisclosedClaim,
		VerifierState: resp.RedirectURI,
	}, nil
}

// walletCredentialToVctype maps a walt.id WalletCredential onto vctypes.Credential.
// The walt.id shape varies by credential format; we extract the common fields
// (id, parsedDocument, document) and fall back gracefully.
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

	// parsedDocument holds the VC body; pull out credentialSubject + type + issuer.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw["parsedDocument"], &parsed); err == nil {
		var issuer json.RawMessage
		if issuer = parsed["issuer"]; issuer != nil {
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
				if s, ok := v.(string); ok {
					cred.Fields[k] = s
				}
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
