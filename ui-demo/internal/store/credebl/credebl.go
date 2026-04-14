// Package credebl provides a beta/stub implementation of the DPG store interfaces
// against Credebl's ACA-Py-based agent stack. For v1 the adaptor shell exists
// and reports "beta" capabilities (all false) so the UI hides flows that aren't
// yet wired. Full integration follows in a later release.
//
// What's working in v1:
//   - DPG selector shows "Credebl" as an option
//   - Capability matrix reports the correct beta state
//   - UI shows a "Credebl integration in beta" banner when selected
//
// What's NOT working in v1:
//   - Credential issuance
//   - Wallet claim/present
//   - Verification
//
// Credebl agent endpoints (for future wiring) — see the credebl-inji-adapter
// reference repo, backends.json:
//
//   URL:      http://host.docker.internal:8004
//   Verify:   POST /agent/credential/verify
//   Token:    POST /agent/token
//   Auth:     API key header
//   DIDs:     did:polygon, did:indy, did:sov, did:peer
package credebl

import (
	"context"
	"fmt"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
)

// =============================================================================
// IssuerStore
// =============================================================================

type issuerStore struct{}

// NewIssuerStore returns a beta/stub Credebl issuer store.
func NewIssuerStore() store.IssuerStore { return &issuerStore{} }

func (s *issuerStore) Name() string { return "Credebl Agent (Beta)" }

func (s *issuerStore) Capabilities() model.IssuerCapabilities {
	// All false — UI will show a "Beta / Coming soon" banner.
	return model.IssuerCapabilities{
		Formats: []string{},
	}
}

func (s *issuerStore) OnboardIssuer(_ context.Context, _ string) (*model.OnboardIssuerResult, error) {
	return nil, fmt.Errorf("credebl issuer integration is in beta — not available in v1")
}

func (s *issuerStore) IssueCredential(_ context.Context, _ *model.OnboardIssuerResult, _, _ string, _ map[string]any) (string, error) {
	return "", fmt.Errorf("credebl credential issuance is in beta — not available in v1")
}

func (s *issuerStore) IssueBatch(_ context.Context, _ *model.OnboardIssuerResult, _, _ string, records []map[string]any) (*model.BatchResult, error) {
	return nil, fmt.Errorf("credebl batch issuance is in beta — not available in v1")
}

func (s *issuerStore) ListCredentialConfigs(_ context.Context) ([]model.CredentialConfig, error) {
	// Return empty — UI shows empty state with beta banner.
	return []model.CredentialConfig{}, nil
}

func (s *issuerStore) RegisterCredentialType(_ context.Context, _, _, _, _ string) (string, error) {
	return "", fmt.Errorf("credebl credential type registration is in beta — not available in v1")
}

// =============================================================================
// WalletStore
// =============================================================================

type walletStore struct{}

// NewWalletStore returns a beta/stub Credebl wallet store.
func NewWalletStore() store.WalletStore { return &walletStore{} }

func (s *walletStore) Name() string { return "Credebl Wallet (Beta)" }

func (s *walletStore) Capabilities() model.WalletCapabilities {
	// All false — UI will show a "Beta / Coming soon" banner.
	return model.WalletCapabilities{
		DIDMethods: []string{},
	}
}

func (s *walletStore) GetWallets(_ context.Context, _ string) ([]model.WalletInfo, error) {
	return nil, fmt.Errorf("credebl wallet integration is in beta — not available in v1")
}

func (s *walletStore) ListCredentials(_ context.Context, _, _ string) ([]model.WalletCredential, error) {
	return []model.WalletCredential{}, nil
}

func (s *walletStore) ClaimCredential(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("credebl claim is in beta — not available in v1")
}

func (s *walletStore) ListDIDs(_ context.Context, _, _ string) ([]model.DIDInfo, error) {
	return []model.DIDInfo{}, nil
}

func (s *walletStore) CreateDID(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("credebl DID creation is in beta — not available in v1")
}

func (s *walletStore) PresentCredential(_ context.Context, _, _ string, _ model.PresentRequest) error {
	return fmt.Errorf("credebl presentation is in beta — not available in v1")
}

// =============================================================================
// VerifierStore
// =============================================================================

type verifierStore struct{}

// NewVerifierStore returns a beta/stub Credebl verifier store.
func NewVerifierStore() store.VerifierStore { return &verifierStore{} }

func (s *verifierStore) Name() string { return "Credebl Verifier (Beta)" }

func (s *verifierStore) Capabilities() model.VerifierCapabilities {
	// All false — UI will show a "Beta / Coming soon" banner.
	return model.VerifierCapabilities{
		DIDMethods: []string{},
	}
}

func (s *verifierStore) CreateVerificationSession(_ context.Context, _ model.VerifyRequest) (*model.VerifyResult, error) {
	return nil, fmt.Errorf("credebl verification is in beta — not available in v1")
}

func (s *verifierStore) GetSessionResult(_ context.Context, _ string) (*model.VerifyResult, error) {
	return nil, fmt.Errorf("credebl verification is in beta — not available in v1")
}

func (s *verifierStore) ListPolicies(_ context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *verifierStore) DirectVerify(_ context.Context, _ []byte, _ string) (*model.VerifyResult, error) {
	return nil, fmt.Errorf("credebl verifier is in beta — not available in v1")
}

// =============================================================================
// AuthStore
// =============================================================================

type authStore struct{}

// NewAuthStore returns a beta/stub Credebl auth store.
func NewAuthStore() store.AuthStore { return &authStore{} }

func (s *authStore) Login(_ context.Context, _, _ string) (*model.SessionInfo, error) {
	return nil, fmt.Errorf("credebl auth is in beta — use SSO for v1")
}

func (s *authStore) Register(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("credebl auth is in beta — use SSO for v1")
}
