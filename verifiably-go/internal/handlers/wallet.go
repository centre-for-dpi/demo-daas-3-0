package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/verifiably/verifiably-go/vctypes"
)

// ShowWallet renders the wallet home (receive + inbox + held credentials).
// First visit lazy-loads held credentials from the adapter.
func (h *H) ShowWallet(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.HolderDpg == "" {
		h.redirect(w, r, "/holder/dpg")
		return
	}
	if sess.WalletCreds == nil {
		creds, err := h.Adapter.ListWalletCredentials(r.Context())
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
	cred, err := h.Adapter.ParseOffer(r.Context(), uri)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	cred.Source = "scan"
	cred.ID = "pending-" + time.Now().Format("150405.000000")
	sess.WalletPending = append([]vctypes.Credential{cred}, sess.WalletPending...)
	h.renderFragment(w, "fragment_wallet_body", sess)
}

// PasteOffer processes a pasted offer URI.
func (h *H) PasteOffer(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	raw := strings.TrimSpace(r.FormValue("offer_uri"))
	if raw == "" {
		h.errorToast(w, r, "Paste an openid-credential-offer:// URI first")
		return
	}
	if !strings.HasPrefix(raw, "openid-credential-offer://") && !strings.HasPrefix(raw, "https://") {
		h.errorToast(w, r, "That doesn't look like a credential offer URI")
		return
	}
	cred, err := h.Adapter.ParseOffer(r.Context(), raw)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	cred.Source = "paste"
	cred.ID = "pending-" + time.Now().Format("150405.000000")
	sess.WalletPending = append([]vctypes.Credential{cred}, sess.WalletPending...)
	h.renderFragment(w, "fragment_wallet_body", sess)
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
	claimed, err := h.Adapter.ClaimCredential(r.Context(), pending)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	sess.WalletCreds = append([]vctypes.Credential{claimed}, sess.WalletCreds...)
	h.renderFragment(w, "fragment_wallet_body", sess)
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
	h.renderFragment(w, "fragment_wallet_body", sess)
}

// ShowPresent renders the OID4VP presentation entry screen for the holder.
func (h *H) ShowPresent(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.HolderDpg == "" {
		h.redirect(w, r, "/holder/dpg")
		return
	}
	h.render(w, r, "holder_present", h.pageData(sess, nil))
}

// SimulatePresent triggers the "verifier requested X" modal.
func (h *H) SimulatePresent(w http.ResponseWriter, r *http.Request) {
	h.renderFragment(w, "fragment_present_modal", nil)
}
