// Package localholder is an in-process OID4VCI holder — a Go struct that
// acts as a credential wallet for the server itself. It is NOT related to
// MOSIP Inji Web or Inji Mobile. It was extracted from the inji package
// (where it lived historically) because it's a generic OID4VCI Pre-Auth
// Code flow client + in-memory credential bag, not specific to any DPG.
//
// What it does:
//
//   - parses an openid-credential-offer:// URL
//   - runs the full OID4VCI Pre-Authorized Code flow against any compliant
//     issuer (our own /local-issuer endpoint, Inji Certify, or anything else
//     that speaks the standard protocol)
//   - generates an ephemeral ECDSA P-256 holder key + did:jwk
//   - signs an OID4VCI proof JWT (ES256)
//   - stores the resulting credential in a process-wide shared walletbag
//
// The holder key is ephemeral — regenerated each server start. Credentials
// persist only in memory. This is demo-grade; production deployments would
// swap the bag for a real wallet backend (a walt.id wallet-api, Credebl
// agent, or persistent SQL store).
package localholder

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
	"vcplatform/internal/store/walletbag"
	"vcplatform/internal/transport"
)

// =============================================================================
// HolderStore — in-process OID4VCI holder.
// =============================================================================

type holderStore struct {
	client transport.Client
}

// NewHolderStore creates an in-process OID4VCI holder wallet. The transport
// client is accepted for API parity with other wallet stores but is not
// used — the holder talks to issuers directly via net/http.
func NewHolderStore(client transport.Client) store.WalletStore {
	return &holderStore{client: client}
}

func (s *holderStore) Name() string {
	return "In-Process OID4VCI Holder (demo)"
}

func (s *holderStore) Capabilities() model.WalletCapabilities {
	return model.WalletCapabilities{
		ClaimOffer:                    true,
		CreateDIDs:                    true,
		HolderInitiatedPresentation:   true,
		VerifierInitiatedPresentation: true,
		SelectiveDisclosure:           false,
		DIDMethods:                    []string{"did:jwk"},
	}
}

func (s *holderStore) GetWallets(ctx context.Context, token string) ([]model.WalletInfo, error) {
	return []model.WalletInfo{{ID: "local-holder", Name: "In-Process Holder"}}, nil
}

// ClaimCredential runs the full OID4VCI pre-auth flow against whatever
// issuer the offer URL points at, then stores the resulting credential
// in the shared walletbag. Works against our own /local-issuer endpoint,
// Inji Certify, or any OID4VCI-compliant backend.
func (s *holderStore) ClaimCredential(ctx context.Context, token, walletID, offerURL string) error {
	credJSON, err := ClaimOID4VCICredential(ctx, offerURL)
	if err != nil {
		return err
	}

	var parsed map[string]any
	_ = json.Unmarshal(credJSON, &parsed)

	// Derive a display ID: prefer the credential's own id, then fall back.
	id := randomID(8)
	if parsed != nil {
		if s, ok := parsed["id"].(string); ok && s != "" {
			id = s
		}
	}
	format := "ldp_vc"
	if strings.HasPrefix(strings.TrimSpace(string(credJSON)), "ey") {
		format = "jwt_vc"
	}

	walletbag.Shared.Add(token, model.WalletCredential{
		ID:             id,
		Format:         format,
		AddedOn:        time.Now().Format("2006-01-02 15:04"),
		Document:       string(credJSON),
		ParsedDocument: parsed,
	})
	return nil
}

func (s *holderStore) ListCredentials(ctx context.Context, token, walletID string) ([]model.WalletCredential, error) {
	return walletbag.Shared.List(token), nil
}

func (s *holderStore) ListDIDs(ctx context.Context, token, walletID string) ([]model.DIDInfo, error) {
	return []model.DIDInfo{
		{DID: "did:jwk:ephemeral", Alias: "Local Holder", Default: true},
	}, nil
}

func (s *holderStore) CreateDID(ctx context.Context, token, walletID, method string) (string, error) {
	return "did:jwk:ephemeral", nil
}

func (s *holderStore) PresentCredential(ctx context.Context, token, walletID string, req model.PresentRequest) error {
	// v1: the handler-level /api/verifier/direct-verify path uses the raw
	// credential from the wallet bag, so this method is a no-op kept for
	// interface compliance.
	return nil
}

// =============================================================================
// AuthStore — shim auth for the local holder so email/password signups work.
// =============================================================================

// authStore is a pretend email/password authenticator. Every email gets a
// deterministic token derived from the address, so the in-memory wallet bag
// is stable across requests for the same user.
type authStore struct{}

// NewAuthStore returns a shim AuthStore for the local holder.
func NewAuthStore() store.AuthStore { return &authStore{} }

func (s *authStore) Login(ctx context.Context, email, password string) (*model.SessionInfo, error) {
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	return &model.SessionInfo{
		UserID:   "local-" + email,
		Username: email,
		Token:    "local-token-" + email,
	}, nil
}

func (s *authStore) Register(ctx context.Context, name, email, password string) error {
	if email == "" {
		return fmt.Errorf("email required")
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

var randSrc = rand.New(rand.NewSource(time.Now().UnixNano()))

func randomID(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[randSrc.Intn(len(charset))]
	}
	return string(b)
}
