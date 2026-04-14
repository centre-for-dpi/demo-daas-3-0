package store

import (
	"context"

	"vcplatform/internal/model"
)

// AuthStore handles authentication against the backend SSO/wallet.
type AuthStore interface {
	// Login authenticates and returns a session with a backend token.
	Login(ctx context.Context, email, password string) (*model.SessionInfo, error)
	// Register creates a new wallet user account.
	Register(ctx context.Context, name, email, password string) error
}

// WalletStore manages the holder's wallet operations.
type WalletStore interface {
	// Name returns a short human-friendly name for the backend ("Walt.id", "Inji Web", ...).
	Name() string
	// Capabilities describes what this wallet backend supports.
	Capabilities() model.WalletCapabilities

	// GetWallets returns wallets for the authenticated user.
	GetWallets(ctx context.Context, token string) ([]model.WalletInfo, error)
	// ListCredentials returns credentials in a wallet.
	ListCredentials(ctx context.Context, token, walletID string) ([]model.WalletCredential, error)
	// ClaimCredential claims a credential offer into the wallet.
	ClaimCredential(ctx context.Context, token, walletID, offerURL string) error
	// ListDIDs returns DIDs in a wallet.
	ListDIDs(ctx context.Context, token, walletID string) ([]model.DIDInfo, error)
	// CreateDID creates a new DID in the wallet.
	CreateDID(ctx context.Context, token, walletID, method string) (string, error)
	// PresentCredential presents credentials to a verifier.
	PresentCredential(ctx context.Context, token, walletID string, req model.PresentRequest) error
}

// IssuerStore manages credential issuance operations.
type IssuerStore interface {
	// Name returns a short human-friendly name for the backend ("Walt.id", "Inji Certify", ...).
	Name() string
	// Capabilities describes what this issuer backend supports.
	Capabilities() model.IssuerCapabilities

	// OnboardIssuer generates a new issuer key pair and DID.
	OnboardIssuer(ctx context.Context, keyType string) (*model.OnboardIssuerResult, error)
	// IssueCredential issues a single credential and returns the OID4VCI offer URL.
	// format: "jwt_vc_json", "sdjwt_vc", "ldp_vc" etc.
	IssueCredential(ctx context.Context, issuer *model.OnboardIssuerResult, configID, format string, claims map[string]any) (string, error)
	// IssueBatch issues multiple credentials.
	IssueBatch(ctx context.Context, issuer *model.OnboardIssuerResult, configID, format string, records []map[string]any) (*model.BatchResult, error)
	// ListCredentialConfigs returns the credential configurations supported by this backend.
	ListCredentialConfigs(ctx context.Context) ([]model.CredentialConfig, error)
	// RegisterCredentialType registers a new credential type with the backend.
	// Returns the config ID. Not all backends support this — returns error if unsupported.
	RegisterCredentialType(ctx context.Context, typeName, displayName, description, format string) (string, error)
}

// VerifierStore manages credential verification operations.
type VerifierStore interface {
	// Name returns a short human-friendly name for the backend ("Walt.id", "Inji Verify", ...).
	Name() string
	// Capabilities describes what this verifier backend supports.
	Capabilities() model.VerifierCapabilities

	// CreateVerificationSession creates an OID4VP verification request.
	CreateVerificationSession(ctx context.Context, req model.VerifyRequest) (*model.VerifyResult, error)
	// GetSessionResult retrieves the verification result for a session.
	GetSessionResult(ctx context.Context, state string) (*model.VerifyResult, error)
	// ListPolicies returns available verification policies.
	ListPolicies(ctx context.Context) (map[string]string, error)
	// DirectVerify posts a raw credential to the backend's direct-verify endpoint.
	// Used for verifier backends (e.g. Inji Verify) that don't require an OID4VP
	// session dance. Returns nil result + error if unsupported.
	DirectVerify(ctx context.Context, credential []byte, contentType string) (*model.VerifyResult, error)
}

// SchemaStore manages credential schemas.
type SchemaStore interface {
	ListSchemas(ctx context.Context) ([]model.Schema, error)
	GetSchema(ctx context.Context, id string) (*model.Schema, error)
}

// NotificationStore manages notifications (typically local, not DPG-backed).
type NotificationStore interface {
	ListNotifications(ctx context.Context, role string) ([]model.Notification, error)
}

// AuditStore manages audit logs (typically local, not DPG-backed).
type AuditStore interface {
	ListEntries(ctx context.Context) ([]model.AuditEntry, error)
}

// Stores aggregates all store interfaces for dependency injection.
// Each service (Issuer, Wallet, Verifier) can be backed by a different DPG
// selected via ISSUER_DPG / WALLET_DPG / VERIFIER_DPG env vars.
type Stores struct {
	Auth          AuthStore
	Wallet        WalletStore
	Issuer        IssuerStore
	Verifier      VerifierStore
	Schemas       SchemaStore
	Notifications NotificationStore
	Audit         AuditStore
}

// Capabilities composes the per-service capability structs into the unified
// model.Capabilities the UI consumes via GET /api/capabilities.
func (s *Stores) Capabilities() model.Capabilities {
	caps := model.Capabilities{}
	if s.Issuer != nil {
		caps.Issuer = s.Issuer.Capabilities()
		caps.IssuerName = s.Issuer.Name()
	}
	if s.Wallet != nil {
		caps.Wallet = s.Wallet.Capabilities()
		caps.WalletName = s.Wallet.Name()
	}
	if s.Verifier != nil {
		caps.Verifier = s.Verifier.Capabilities()
		caps.VerifierName = s.Verifier.Name()
	}
	return caps
}
