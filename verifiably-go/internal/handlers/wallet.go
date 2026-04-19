package handlers

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/vctypes"
)

// holderCtx wraps r.Context() with the selected holder DPG so the Registry
// can route holder-scoped adapter calls when multiple holders are registered.
// Used by every wallet.go handler that touches the Adapter; safe to call even
// when sess.HolderDpg is "" (WithHolderDpg is a no-op).
func holderCtx(r *http.Request, sess *Session) context.Context {
	return backend.WithHolderDpg(r.Context(), sess.HolderDpg)
}

// ShowWallet renders the wallet home (receive + inbox + held credentials).
// First visit lazy-loads held credentials from the adapter.
func (h *H) ShowWallet(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.HolderDpg == "" {
		h.redirect(w, r, "/holder/dpg")
		return
	}
	if sess.WalletCreds == nil {
		creds, err := h.Adapter.ListWalletCredentials(holderCtx(r, sess))
		if err != nil {
			h.errorToast(w, r, err.Error())
			return
		}
		sess.WalletCreds = creds
	}
	h.render(w, r, "holder_wallet", h.pageData(sess, nil))
}

// ScanOffer simulates scanning a QR — cycles through example offers from the adapter.
func (h *H) ScanOffer(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	examples, err := h.Adapter.ListExampleOffers(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	if len(examples) == 0 {
		h.errorToast(w, r, "no example offers available")
		return
	}
	uri := examples[sess.NextExampleIdx%len(examples)]
	sess.NextExampleIdx++
	cred, err := h.Adapter.ParseOffer(holderCtx(r, sess), uri)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	cred.Source = "scan"
	cred.ID = "pending-" + time.Now().Format("150405.000000")
	sess.WalletPending = append([]vctypes.Credential{cred}, sess.WalletPending...)
	h.renderFragment(w, r, "fragment_wallet_body", sess)
}

// PasteOffer processes a pasted offer URI. Renders the wallet body on both
// success and failure so the user gets a visible result either way — toasts
// can be missed (browser focus, quick fade) but an inline error banner
// stays until the next action.
func (h *H) PasteOffer(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	raw := strings.TrimSpace(r.FormValue("offer_uri"))
	if raw == "" {
		sess.LastWalletError = "Paste an openid-credential-offer:// URI first"
		h.renderFragment(w, r, "fragment_wallet_body", sess)
		return
	}
	if !strings.HasPrefix(raw, "openid-credential-offer://") && !strings.HasPrefix(raw, "https://") {
		sess.LastWalletError = "That doesn't look like a credential offer URI — it should start with openid-credential-offer:// or https://"
		h.renderFragment(w, r, "fragment_wallet_body", sess)
		return
	}
	cred, err := h.Adapter.ParseOffer(holderCtx(r, sess), raw)
	if err != nil {
		sess.LastWalletError = err.Error()
		h.renderFragment(w, r, "fragment_wallet_body", sess)
		return
	}
	sess.LastWalletError = ""
	cred.Source = "paste"
	cred.ID = "pending-" + time.Now().Format("150405.000000")
	sess.WalletPending = append([]vctypes.Credential{cred}, sess.WalletPending...)
	h.renderFragment(w, r, "fragment_wallet_body", sess)
}

// PrefillExample returns a textarea pre-populated with an example offer URI.
// HTMX swaps the existing #offer-paste textarea with this one.
func (h *H) PrefillExample(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	examples, err := h.Adapter.ListExampleOffers(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	if len(examples) == 0 {
		h.errorToast(w, r, "no example offers available")
		return
	}
	uri := examples[sess.NextExampleIdx%len(examples)]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("HX-Trigger", `{"toast":"Example offer URI pasted — click Process offer"}`)
	// URI may come from an untrusted adapter in the future, so escape.
	_, _ = w.Write([]byte(`<textarea id="offer-paste" name="offer_uri" rows="3" class="mono" style="font-size:0.78rem">` + template.HTMLEscapeString(uri) + `</textarea>`))
}

// AcceptCred moves a pending offer into held credentials.
func (h *H) AcceptCred(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	id := r.FormValue("id")
	idx := -1
	for i, c := range sess.WalletPending {
		if c.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		h.errorToast(w, r, "offer not found")
		return
	}
	pending := sess.WalletPending[idx]
	sess.WalletPending = append(sess.WalletPending[:idx], sess.WalletPending[idx+1:]...)
	claimed, err := h.Adapter.ClaimCredential(holderCtx(r, sess), pending)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	sess.WalletCreds = append([]vctypes.Credential{claimed}, sess.WalletCreds...)
	h.renderFragment(w, r, "fragment_wallet_body", sess)
}

// RejectCred discards a pending offer.
func (h *H) RejectCred(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	id := r.FormValue("id")
	found := false
	kept := make([]vctypes.Credential, 0, len(sess.WalletPending))
	for _, c := range sess.WalletPending {
		if c.ID == id {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	if !found {
		h.errorToast(w, r, "offer not found")
		return
	}
	sess.WalletPending = kept
	h.renderFragment(w, r, "fragment_wallet_body", sess)
}

// ShowPresent renders the OID4VP presentation entry screen for the holder.
func (h *H) ShowPresent(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.HolderDpg == "" {
		h.redirect(w, r, "/holder/dpg")
		return
	}
	dpgs, _ := h.Adapter.ListHolderDpgs(r.Context())
	// Pull the holder's accepted credentials so the UI can render a picker.
	// Use the session's cached list if present; otherwise do a fresh adapter
	// call. Non-linked DPGs surface as an empty list (the template renders
	// an "accept an offer first" hint in that case).
	creds := sess.WalletCreds
	if len(creds) == 0 {
		if c, err := h.Adapter.ListWalletCredentials(holderCtx(r, sess)); err == nil {
			creds = c
		}
	}
	h.render(w, r, "holder_present", h.pageData(sess, map[string]any{
		"HolderDpgObj": dpgs[sess.HolderDpg],
		"Credentials":  creds,
	}))
}

// SubmitPresent drives PresentCredential on the adapter for the chosen
// credential + the verifier's request URI. Renders a result fragment.
func (h *H) SubmitPresent(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.HolderDpg == "" {
		h.redirect(w, r, "/holder/dpg")
		return
	}
	credID := r.FormValue("credential_id")
	reqURI := r.FormValue("request_uri")
	if credID == "" || reqURI == "" {
		h.errorToast(w, r, "Pick a credential and paste the verifier's request URI")
		return
	}
	res, err := h.Adapter.PresentCredential(r.Context(), backend.PresentCredentialRequest{
		HolderDpg:    sess.HolderDpg,
		CredentialID: credID,
		RequestURI:   reqURI,
	})
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragment(w, r, "fragment_present_result", res)
}
