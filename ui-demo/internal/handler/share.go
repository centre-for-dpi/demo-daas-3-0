package handler

// share.go — holder-initiated credential sharing.
//
// Scope: the holder already has a credential in their wallet and wants to
// hand it to a verifier WITHOUT waiting for the verifier to open an
// OID4VP session. Two delivery modes:
//
//  1. Offline-verifiable QR. Reuses the PixelPass encoder from the PDF
//     wallet package — base45(zlib(credJSON)). A verifier scans, decodes
//     locally, and runs cryptographic verification with no server
//     round-trip. QR may refuse large credentials; we surface the same
//     QRTooLargeError the PDF wallet does.
//
//  2. Short-lived share link. The server stashes the credential in an
//     in-memory cache keyed by a random share ID. The verifier opens
//     /share/v/{id} in a browser, sees the claims + a Verify button that
//     posts the credential to /api/verifier/direct-verify.
//
// Wallet.PresentCredential() is intentionally avoided here — walt.id's
// wallet-api usePresentationRequest is picky about credential
// provenance (it only presents credentials in its own SQL store, not
// ones surfaced from the shared walletbag), and the in-process holder's
// PresentCredential is a no-op. Going around that layer gives us a
// sharing path that works for any credential the holder can list.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/store/pdfwallet"
)

// sharedCredential is a credential the holder has staged for a verifier
// to view. Entries expire after shareTTL.
type sharedCredential struct {
	OwnerID    string
	Credential model.WalletCredential
	CreatedAt  time.Time
}

const shareTTL = 30 * time.Minute

// shareStore is a per-process, in-memory store for shared credentials.
// Keyed by a random shareID → sharedCredential. Demo-grade; production
// deployments would back this with Redis or similar.
type shareStore struct {
	mu    sync.RWMutex
	items map[string]sharedCredential
}

func newShareStore() *shareStore {
	return &shareStore{items: map[string]sharedCredential{}}
}

func (s *shareStore) put(id string, sc sharedCredential) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = sc
}

func (s *shareStore) get(id string) (sharedCredential, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc, ok := s.items[id]
	if !ok {
		return sharedCredential{}, false
	}
	if time.Since(sc.CreatedAt) > shareTTL {
		return sharedCredential{}, false
	}
	return sc, true
}

func randShareID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// APIShareProactive handles POST /api/share/proactive — the holder picks a
// credential from their wallet and asks the server to generate an
// offline-verifiable QR payload + a short-lived share URL.
func (h *Handler) APIShareProactive(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, 401, map[string]string{"error": "unauthorized — sign in via SSO to share credentials"})
		return
	}
	if user.WalletDPG == "" {
		writeJSON(w, 400, map[string]any{
			"error":    "pick a wallet backend before sharing credentials",
			"action":   "onboarding",
			"redirect": "/portal/onboarding",
		})
		return
	}

	var req struct {
		CredentialID string `json:"credentialId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.CredentialID == "" {
		writeJSON(w, 400, map[string]string{"error": "credentialId required"})
		return
	}

	cred, err := h.lookupHolderCredential(r.Context(), user, req.CredentialID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": err.Error()})
		return
	}

	// Stash in the share store so /share/v/{id} can render a public page.
	shareID := randShareID()
	h.shareStore.put(shareID, sharedCredential{
		OwnerID:    user.ID,
		Credential: cred,
		CreatedAt:  time.Now(),
	})

	resp := map[string]any{
		"status":    "shared",
		"shareId":   shareID,
		"shareUrl":  buildShareURL(r, shareID),
		"expiresIn": int(shareTTL.Seconds()),
		"format":    cred.Format,
	}

	// Best-effort PixelPass encoding for an offline QR payload. We try
	// the same way pdfwallet does — Highest → Low EC — and surface a
	// structured error if the credential is too big for a single QR.
	// The share link still works even when the QR doesn't.
	payload, err := pdfwallet.PixelPassEncode([]byte(cred.Document))
	if err != nil {
		resp["qrWarning"] = "failed to encode credential for offline QR: " + err.Error()
		writeJSON(w, 200, resp)
		return
	}
	resp["qrPayload"] = payload
	resp["qrPayloadSize"] = len(payload)
	resp["rawBytes"] = len(cred.Document)

	// We can also render a QR server-side to sanity-check it fits. If it
	// doesn't, return the share link anyway and flag the offline path as
	// unavailable so the UI can choose whether to hide the QR panel.
	if _, err := pdfwallet.RenderCredentialPDF(cred.ParsedDocument, []byte(cred.Document), cred.Format); err != nil {
		var qrTooLarge *pdfwallet.QRTooLargeError
		if errors.As(err, &qrTooLarge) {
			resp["offlineQrAvailable"] = false
			resp["offlineQrReason"] = qrTooLarge.Error()
		} else {
			resp["offlineQrAvailable"] = false
			resp["offlineQrReason"] = err.Error()
		}
	} else {
		resp["offlineQrAvailable"] = true
	}

	writeJSON(w, 200, resp)
}

// ShareView handles GET /share/v/{id} — a public page that renders the
// shared credential's claims and a Verify button. No auth required —
// anyone with the link can see the credential the holder chose to share.
func (h *Handler) ShareView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing share id", http.StatusBadRequest)
		return
	}
	sc, ok := h.shareStore.get(id)
	if !ok {
		http.Error(w, "share not found or expired", http.StatusNotFound)
		return
	}
	data := h.pageData(r, "share-view", map[string]any{
		"credential": sc.Credential,
		"shareId":    id,
	})
	if err := h.render.Render(w, "share/view", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// APIShareVerify handles POST /share/v/{id}/verify — re-reads the stored
// credential and drives the configured verifier's direct-verify endpoint.
// Called by the JS on the share-view page.
func (h *Handler) APIShareVerify(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc, ok := h.shareStore.get(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "share not found or expired"})
		return
	}
	contentType := "application/vc+ld+json"
	doc := sc.Credential.Document
	if len(doc) > 0 && doc[0] == 'e' {
		if len(doc) > 5 && contains(doc, "~") {
			contentType = "application/vc+sd-jwt"
		} else {
			contentType = "application/jwt"
		}
	}
	// Use the verifier the holder has configured for themselves. For a
	// truly anonymous verify link we could pin to the server-default
	// verifier instead; for now, the server-default is fine since no
	// per-user auth is required to access the share link.
	result, err := h.stores.Verifier.DirectVerify(r.Context(), []byte(doc), contentType)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "verify failed: " + err.Error()})
		return
	}
	writeJSON(w, 200, result)
}

// lookupHolderCredential pulls a specific credential out of the holder's
// wallet by ID, resolving through walletFor(user) so it honors the
// holder's DPG choice.
func (h *Handler) lookupHolderCredential(ctx context.Context, user *model.User, credID string) (model.WalletCredential, error) {
	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(ctx, user.WalletToken)
	if err != nil || len(wallets) == 0 {
		return model.WalletCredential{}, fmt.Errorf("no wallet found")
	}
	creds, err := wallet.ListCredentials(ctx, user.WalletToken, wallets[0].ID)
	if err != nil {
		return model.WalletCredential{}, fmt.Errorf("list credentials: %w", err)
	}
	for _, c := range creds {
		if c.ID == credID {
			return c, nil
		}
	}
	return model.WalletCredential{}, fmt.Errorf("credential %q not found in wallet", credID)
}

// buildShareURL constructs an absolute URL for the share view. Uses the
// incoming request's host so the link is reachable from wherever the
// holder is (dev: localhost:8080, prod: deployed hostname).
func buildShareURL(r *http.Request, id string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/share/v/%s", scheme, r.Host, id)
}

// contains is a tiny substring helper so we don't drag in strings for one
// call site (the file imports "strings" indirectly but keeping this local
// avoids hoisting another import just for detection).
func contains(s, sub string) bool {
	n, m := len(s), len(sub)
	if m == 0 {
		return true
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return true
		}
	}
	return false
}
