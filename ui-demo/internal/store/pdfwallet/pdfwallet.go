// Package pdfwallet is a WalletStore implementation whose ClaimCredential
// generates a printable, offline-verifiable PDF for each credential the
// holder receives. It is a first-class wallet DPG option alongside walt.id,
// the in-process OID4VCI holder, and Credebl — not a delivery channel on
// top of another wallet.
//
// How it works:
//
//  1. ClaimCredential runs the standard OID4VCI Pre-Authorized Code flow
//     against whichever issuer the offer URL points at (reusing the generic
//     client in internal/store/localholder). This gets us a real signed VC.
//  2. The credential is zlib-compressed and base45-encoded per the
//     PixelPass specification (RFC 9285), producing a dense QR-friendly
//     payload.
//  3. We attempt to render that payload into a single QR code. If the
//     payload exceeds the QR version 40 capacity at every recovery level,
//     ClaimCredential surfaces a structured error naming the format, the
//     payload size, and suggested alternatives. We do NOT blanket-refuse
//     any format — the QR library gets to be the authority on whether the
//     credential fits.
//  4. A PDF is generated with the human-readable claims at the top and the
//     self-verifying QR at the bottom. An offline verifier scans the QR,
//     decodes it (base45 → zlib → JSON), and runs cryptographic
//     verification without contacting the issuer.
//
// The PDF bytes are held in an in-memory cache keyed by (walletToken, credID)
// so the handler can serve them via GET /api/wallet/pdf?id=...
//
// Credentials also land in the shared walletbag so the holder sees them in
// "My Credentials" the same way they'd see any other claimed credential.
package pdfwallet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
	"vcplatform/internal/store/localholder"
	"vcplatform/internal/store/walletbag"
)

// Store is the PDF wallet WalletStore implementation.
type Store struct {
	mu   sync.RWMutex
	pdfs map[string][]byte // key = token + "\x00" + credID
}

// New creates a new PDF wallet store. No configuration needed — the store is
// a pure-Go generator with no upstream dependency.
func New() *Store {
	return &Store{pdfs: map[string][]byte{}}
}

func (s *Store) Name() string {
	return "Print PDF Wallet (offline, self-verifying)"
}

func (s *Store) Capabilities() model.WalletCapabilities {
	return model.WalletCapabilities{
		ClaimOffer:                    true,
		CreateDIDs:                    false,
		HolderInitiatedPresentation:   false,
		VerifierInitiatedPresentation: false,
		SelectiveDisclosure:           false,
		DIDMethods:                    []string{"did:jwk"},
	}
}

func (s *Store) GetWallets(ctx context.Context, token string) ([]model.WalletInfo, error) {
	return []model.WalletInfo{{ID: "pdf-wallet", Name: "Printable PDF Wallet"}}, nil
}

// ClaimCredential runs the OID4VCI Pre-Auth flow, generates a PDF with a
// PixelPass-encoded self-verifying QR, caches the PDF bytes in-memory, and
// records the credential metadata in the shared walletbag.
func (s *Store) ClaimCredential(ctx context.Context, token, walletID, offerURL string) error {
	credJSON, err := localholder.ClaimOID4VCICredential(ctx, offerURL)
	if err != nil {
		return fmt.Errorf("claim: %w", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(credJSON, &parsed)

	id := deriveCredID(parsed)
	format := detectFormat(credJSON)

	pdfBytes, err := RenderCredentialPDF(parsed, credJSON, format)
	if err != nil {
		return err // already a structured error for size/rendering failures
	}

	s.putPDF(token, id, pdfBytes)

	walletbag.Shared.Add(token, model.WalletCredential{
		ID:             id,
		Format:         format,
		AddedOn:        time.Now().Format("2006-01-02 15:04"),
		Document:       string(credJSON),
		ParsedDocument: parsed,
	})
	return nil
}

func (s *Store) ListCredentials(ctx context.Context, token, walletID string) ([]model.WalletCredential, error) {
	return walletbag.Shared.List(token), nil
}

func (s *Store) ListDIDs(ctx context.Context, token, walletID string) ([]model.DIDInfo, error) {
	return []model.DIDInfo{
		{DID: "did:jwk:pdf-wallet", Alias: "PDF Wallet", Default: true},
	}, nil
}

func (s *Store) CreateDID(ctx context.Context, token, walletID, method string) (string, error) {
	return "did:jwk:pdf-wallet", nil
}

// PresentCredential is a no-op — a printed PDF presents itself via the QR.
// Any OID4VP-style session flow is out of scope for an offline-only wallet.
func (s *Store) PresentCredential(ctx context.Context, token, walletID string, req model.PresentRequest) error {
	return nil
}

// GetPDF returns the cached PDF bytes for a credential in this token's slot.
// Used by the handler's GET /api/wallet/pdf endpoint.
func (s *Store) GetPDF(token, credID string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.pdfs[pdfKey(token, credID)]
	return b, ok
}

func (s *Store) putPDF(token, credID string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pdfs[pdfKey(token, credID)] = data
}

func pdfKey(token, credID string) string {
	return token + "\x00" + credID
}

// deriveCredID pulls a stable display ID from the parsed credential, or
// generates a new one if the credential doesn't include an `id` field.
func deriveCredID(parsed map[string]any) string {
	if parsed != nil {
		if s, ok := parsed["id"].(string); ok && s != "" {
			return s
		}
	}
	return "pdf-" + time.Now().Format("20060102-150405.000")
}

// detectFormat guesses the credential format from the raw bytes: a leading
// "ey" suggests a JWS (JWT VC or SD-JWT); everything else is treated as a
// JSON-LD LDP_VC.
func detectFormat(credJSON []byte) string {
	trimmed := strings.TrimSpace(string(credJSON))
	if strings.HasPrefix(trimmed, "ey") {
		if strings.Contains(trimmed, "~") {
			return "vc+sd-jwt"
		}
		return "jwt_vc_json"
	}
	return "ldp_vc"
}

// Compile-time assertion that *Store satisfies store.WalletStore.
var _ store.WalletStore = (*Store)(nil)
