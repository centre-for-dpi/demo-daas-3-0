// Package waltid implements backend.Adapter against walt.id Community Stack
// v0.18.2 (issuer-api + verifier-api + wallet-api).
//
// Endpoint mapping is grounded in the v0.18.2 source code (not guessed):
//   - Issuer-api routes: /onboard/issuer, /openid4vc/{jwt|sdjwt|mdoc}/issue,
//     /{standardVersion}/.well-known/openid-credential-issuer.
//   - Verifier-api routes: POST /openid4vc/verify, GET /openid4vc/session/{id}.
//     No direct-credential-verification endpoint exists at v0.18.2 — VerifyDirect
//     returns backend.ErrNotSupported.
//   - Wallet-api routes: /wallet-api/auth/{register,login},
//     /wallet-api/wallet/accounts/wallets,
//     /wallet-api/wallet/{walletId}/exchange/{resolveCredentialOffer,useOfferRequest,usePresentationRequest},
//     /wallet-api/wallet/{walletId}/credentials.
package waltid

import "encoding/json"

// Config is the per-backend config blob the registry passes in.
// Shape matches the "config" object under a "type":"waltid" backend in
// backends.json. All URLs are required; the credentials block is optional
// (if absent, the adapter registers a fresh demo account on first use).
type Config struct {
	IssuerBaseURL   string   `json:"issuerBaseUrl"`
	VerifierBaseURL string   `json:"verifierBaseUrl"`
	WalletBaseURL   string   `json:"walletBaseUrl"`
	StandardVersion string   `json:"standardVersion"` // "draft13" (default) or "draft11"
	DemoAccount     Account  `json:"demoAccount"`
	// IssuerKey / IssuerDID pin a stable onboarding result so every demo run
	// issues from the same DID. Both are optional — when empty, the adapter
	// onboards a new key on first use and caches it in-process. Shape of
	// IssuerKey mirrors /onboard/issuer response (a JWK wrapper).
	IssuerKey json.RawMessage `json:"issuerKey"`
	IssuerDID string          `json:"issuerDid"`
}

// Account holds credentials for the demo wallet user this adapter logs in as.
type Account struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UnmarshalConfig extracts a Config from a raw json.RawMessage.
func UnmarshalConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) == 0 {
		return c, nil
	}
	err := json.Unmarshal(raw, &c)
	if c.StandardVersion == "" {
		c.StandardVersion = "draft13"
	}
	return c, err
}
