package model

import "encoding/json"

// CredentialConfig represents a credential configuration supported by the backend.
type CredentialConfig struct {
	ID       string `json:"id"`       // e.g. "UniversityDegree_jwt_vc_json"
	Name     string `json:"name"`     // e.g. "University Degree"
	Category string `json:"category"` // e.g. "Education", "Identity", "Finance"
	Format   string `json:"format"`   // e.g. "jwt_vc_json", "vc+sd-jwt", "mso_mdoc"
}

// IssueRequest for single credential issuance.
type IssueRequest struct {
	SchemaID string         `json:"schema_id"`
	Claims   map[string]any `json:"claims"`
}

// BatchIssueRequest for batch issuance.
type BatchIssueRequest struct {
	SchemaID string           `json:"schema_id"`
	Records  []map[string]any `json:"records"`
}

// BatchResult from batch issuance.
type BatchResult struct {
	Total     int    `json:"total"`
	Issued    int    `json:"issued"`
	Failed    int    `json:"failed"`
	BatchID   string `json:"batch_id"`
	OfferURLs []string `json:"offer_urls,omitempty"`
}

// OnboardIssuerRequest for issuer onboarding.
type OnboardIssuerRequest struct {
	KeyType string `json:"key_type"` // secp256r1, Ed25519
}

// OnboardIssuerResult from issuer onboarding.
type OnboardIssuerResult struct {
	IssuerKey json.RawMessage `json:"issuerKey"`
	IssuerDID string          `json:"issuerDid"`
}

// VerifyRequest for creating a verification session.
type VerifyRequest struct {
	CredentialTypes []string `json:"credential_types"`
	Policies        []string `json:"policies"`
}

// VerifyResult from verification.
type VerifyResult struct {
	SessionID       string            `json:"session_id"`
	State           string            `json:"state"`
	RequestURL      string            `json:"request_url"` // openid4vp:// URL
	Verified        *bool             `json:"verified,omitempty"`
	Checks          []CheckResult     `json:"checks,omitempty"`
	CredentialType  string            `json:"credentialType,omitempty"`
	IssuerDID       string            `json:"issuerDid,omitempty"`
	HolderDID       string            `json:"holderDid,omitempty"`
	Claims          map[string]any    `json:"claims,omitempty"`
	IssuedAt        string            `json:"issuedAt,omitempty"`
	ExpiresAt       string            `json:"expiresAt,omitempty"`
}

type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, fail, pending
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// PresentRequest for wallet credential presentation.
type PresentRequest struct {
	DID                string   `json:"did"`
	PresentationRequest string  `json:"presentationRequest"`
	SelectedCredentials []string `json:"selectedCredentials"`
}

// CreateDIDRequest for DID creation.
type CreateDIDRequest struct {
	Method string `json:"method"` // jwk, web, key
}

// WalletCredential is a credential as stored in a wallet.
type WalletCredential struct {
	ID             string         `json:"id"`
	Format         string         `json:"format"`
	AddedOn        string         `json:"addedOn"`
	Document       string         `json:"document"`
	ParsedDocument map[string]any `json:"parsedDocument"`
}

// DIDInfo is a DID with metadata.
type DIDInfo struct {
	DID     string `json:"did"`
	Alias   string `json:"alias"`
	KeyID   string `json:"keyId"`
	Default bool   `json:"default"`
}

// WalletInfo from wallet-api.
type WalletInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SessionInfo from wallet login.
type SessionInfo struct {
	UserID   string `json:"id"`
	Token    string `json:"token"`
	Username string `json:"username"`
}
