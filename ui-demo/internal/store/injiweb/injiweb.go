// Package injiweb is a WalletStore adaptor for MOSIP Inji Web — a
// browser-hosted wallet served by the `injistack/inji-web` container
// (frontend) + `injistack/mimoto` container (BFF) pair defined under
// docker/injiweb/ in this repo.
//
// Important: Inji Web is NOT offer-driven. Unlike walt.id wallet-api or
// Credebl, it does not accept an OpenID4VCI `credential_offer` URL as
// input. Its issuance flow is catalog-initiated: the holder opens Inji
// Web, picks an issuer from a list Mimoto loads out of
// `mimoto-issuers-config.json`, authenticates via that issuer's
// esignet/OAuth, and Mimoto runs the OID4VCI exchange itself.
//
// What that means for this adaptor: ClaimCredential returns a
// RedirectClaimError pointing at the Inji Web issuer catalog page
// (/issuers). The offer URL our issuer backend generated is discarded —
// the holder re-picks the same issuer on Inji Web's side. The
// explanation field makes this explicit so the UI can render a "Open
// Inji Web's issuer catalog →" button instead of pretending the offer
// is being handed off.
//
// Credentials end up inside Inji Web/Mimoto, not in our shared
// walletbag. ListCredentials always returns empty; the wallet page
// renders a banner pointing the holder back at Inji Web.
package injiweb

import (
	"context"
	"fmt"
	"strings"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
)

// Store is the Inji Web wallet adaptor.
type Store struct {
	baseURL string // e.g. http://localhost:3004 (local container)
}

// RedirectClaimError signals to the handler layer that the credential is
// claimed out-of-band via a browser redirect. The handler returns a 200
// with a structured redirect payload the UI can render as a button.
type RedirectClaimError struct {
	Wallet      string // friendly name of the wallet backend
	URL         string // target URL the holder should open
	Explanation string // short user-facing explanation
}

func (e *RedirectClaimError) Error() string {
	return fmt.Sprintf("open in %s: %s", e.Wallet, e.URL)
}

// New constructs an Inji Web wallet adaptor pointing at baseURL. When
// baseURL is empty, defaults to the local Inji Web container defined in
// docker/injiweb/ — http://localhost:3004. No silent public-instance
// fallback: if you haven't stood up the container, the redirect will
// fail and you'll know why.
func New(baseURL string) *Store {
	if baseURL == "" {
		baseURL = "http://localhost:3004"
	}
	return &Store{baseURL: strings.TrimRight(baseURL, "/")}
}

func (s *Store) Name() string {
	return "Inji Web (browser wallet)"
}

func (s *Store) Capabilities() model.WalletCapabilities {
	return model.WalletCapabilities{
		ClaimOffer:                    true,
		CreateDIDs:                    false,
		HolderInitiatedPresentation:   false,
		VerifierInitiatedPresentation: true,
		SelectiveDisclosure:           true,
		DIDMethods:                    []string{"did:jwk"},
	}
}

// GetWallets returns a single placeholder wallet entry. Inji Web doesn't
// expose a wallet-list API, so we stub one out so the handler layer has
// a walletID to pass around.
func (s *Store) GetWallets(ctx context.Context, token string) ([]model.WalletInfo, error) {
	return []model.WalletInfo{{ID: "inji-web", Name: "Inji Web"}}, nil
}

// ListCredentials always returns empty — credentials live inside the
// holder's Inji Web session, not in our server. The wallet page renders
// a redirect banner when this store is active.
func (s *Store) ListCredentials(ctx context.Context, token, walletID string) ([]model.WalletCredential, error) {
	return []model.WalletCredential{}, nil
}

// ClaimCredential redirects the holder to Inji Web's issuer catalog. The
// offer URL our issuer backend just generated is intentionally discarded:
// Inji Web (via Mimoto) runs its own OID4VCI exchange against whichever
// issuer the holder picks in /issuers, using the config pre-registered
// in mimoto-issuers-config.json. There is no way to hand it an external
// credential_offer URL — this is a limitation of Inji Web, not a design
// choice on our side.
func (s *Store) ClaimCredential(ctx context.Context, token, walletID, offerURL string) error {
	return &RedirectClaimError{
		Wallet: s.Name(),
		URL:    s.baseURL + "/issuers",
		Explanation: "Inji Web is a catalog-driven browser wallet — it doesn't accept " +
			"external OID4VCI offer URLs. Open its issuer catalog, pick the issuer " +
			"you want to claim from, and complete the OAuth flow there. The " +
			"credential lives inside Inji Web / Mimoto, not on this server.",
	}
}

func (s *Store) ListDIDs(ctx context.Context, token, walletID string) ([]model.DIDInfo, error) {
	return []model.DIDInfo{}, nil
}

func (s *Store) CreateDID(ctx context.Context, token, walletID, method string) (string, error) {
	return "", fmt.Errorf("Inji Web manages its own DIDs — create one inside the browser wallet")
}

// PresentCredential redirects the holder to Inji Web's presentation
// screen. Unlike issuance, Inji Web DOES handle OID4VP authorize URLs
// directly via its /authorize route, but we route through /user so the
// holder lands on their credential list and can pick which one to
// present. Our server never sees the presentation.
func (s *Store) PresentCredential(ctx context.Context, token, walletID string, req model.PresentRequest) error {
	return &RedirectClaimError{
		Wallet:      s.Name(),
		URL:         s.baseURL + "/user",
		Explanation: "Open Inji Web to respond to the verifier's presentation request from your wallet.",
	}
}

// BaseURL returns the configured Inji Web base URL so the handler can
// render a "Open wallet" link on the holder's wallet page.
func (s *Store) BaseURL() string { return s.baseURL }

var _ store.WalletStore = (*Store)(nil)
