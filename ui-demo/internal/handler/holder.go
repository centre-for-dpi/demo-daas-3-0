package handler

import (
	"context"
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

// getWalletData fetches credentials and DIDs for an authenticated user,
// resolving the wallet store per the user's WalletDPG choice.
//
// Also surfaces the active wallet + verifier names + the verifier's
// DPG identifier so templates (Present, Share) can render an honest
// "Verifying through: X" banner without making a second context-aware
// lookup. The verifier is the one the user picked in onboarding via
// h.verifierFor(user) — same routing used by APIShareVerify and the
// share/v/{id}/verify path.
func (h *Handler) getWalletData(ctx context.Context, user *model.User) map[string]any {
	if user == nil || !user.HasBackendAuth() {
		return nil
	}
	result := map[string]any{}
	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(ctx, user.WalletToken)
	if err != nil || len(wallets) == 0 {
		return nil
	}
	result["walletID"] = wallets[0].ID
	result["walletName"] = wallet.Name()
	result["walletDpg"] = user.WalletDPG

	verifier := h.verifierFor(user)
	if verifier != nil {
		result["verifierName"] = verifier.Name()
		caps := verifier.Capabilities()
		result["verifierCaps"] = map[string]any{
			"directVerify":           caps.DirectVerify,
			"createRequest":          caps.CreateRequest,
			"presentationDefinition": caps.PresentationDefinition,
			"didMethods":             caps.DIDMethods,
		}
	}
	result["verifierDpg"] = user.VerifierDPG

	creds, err := wallet.ListCredentials(ctx, user.WalletToken, wallets[0].ID)
	if err == nil {
		result["credentials"] = creds
	}

	dids, err := wallet.ListDIDs(ctx, user.WalletToken, wallets[0].ID)
	if err == nil {
		result["dids"] = dids
	}

	return result
}

// HolderWallet fetches real credentials from the backend and passes them to
// the template, along with the holder's chosen wallet DPG and the catalog of
// alternative wallet backends so the holder can switch backends without
// leaving the wallet page.
//
// Gate: a real holder who hasn't picked a WalletDPG yet is redirected to the
// onboarding wizard so they explicitly choose a backend before anything
// lands in their wallet. Without this, brand-new SSO users would silently
// claim into whichever server-default wallet the deployment picked, which
// contradicts the "holder picks their preferred wallet DPG" design goal.
func (h *Handler) HolderWallet(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())

	if user != nil && user.HasBackendAuth() && user.WalletDPG == "" {
		// HTMX-aware redirect: plain http.Redirect(303) breaks navigation
		// from a sidebar click because HTMX drops the HX-Request header
		// when following the redirect and ends up with a full HTML page
		// where it expected a fragment. See htmxAwareRedirect.
		h.htmxAwareRedirect(w, r, "/portal/onboarding")
		return
	}

	data := h.pageData(r, "holder-wallet", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Credentials", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		wallet := h.walletFor(user)
		payload := map[string]any{
			"live":       true,
			"walletName": wallet.Name(),
			"currentDPG": user.WalletDPG,
			"dpgCards":   h.filteredDPGCatalog("holder"),
		}
		wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
		if err == nil && len(wallets) > 0 {
			payload["walletID"] = wallets[0].ID
			creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
			if err == nil {
				payload["credentials"] = creds
			}
		}
		data.Data = payload
	}

	if err := h.render.Render(w, "holder/wallet", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// HolderCredDetail fetches a single credential's details from the wallet.
func (h *Handler) HolderCredDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-cred-detail", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Credential Detail", Active: true},
	}

	credID := r.URL.Query().Get("id")

	// Fetch real credential if user has backend auth and an ID was provided
	if credID != "" && user != nil && user.HasBackendAuth() {
		wallet := h.walletFor(user)
		wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
		if err == nil && len(wallets) > 0 {
			creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
			if err == nil {
				// Find the credential by ID
				for _, c := range creds {
					if c.ID == credID {
						data.Data = map[string]any{
							"credential": c,
							"walletID":   wallets[0].ID,
							"live":       true,
						}
						break
					}
				}
			}
		}
	}

	if err := h.render.Render(w, "holder/cred_detail", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderClaim(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "holder-claim", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Claim Credential", Active: true},
	}
	if err := h.render.Render(w, "holder/claim", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}


func (h *Handler) HolderDependents(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-dependents", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Dependents", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/dependents", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderInbox(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-inbox", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Verification Requests", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/inbox", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderDisclosure(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-disclosure", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Selective Disclosure", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/disclosure", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderPresentation(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-presentation", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Build Presentation", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/presentation", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderShare(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-share", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Share", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/share", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// HolderCatalog shows available credential types a holder can request. For
// real users, this is the live credential_configurations_supported list
// from every registered issuer DPG — not the mock schema store (which used
// to leak fixture rows like "BSc Degree", "Land Certificate" into new user
// sessions even on a fresh install). If no issuer exposes any config, the
// template falls through to an empty state.
func (h *Handler) HolderCatalog(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-catalog", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Credential Catalog", Active: true},
	}

	if user != nil && !user.Demo {
		type catalogEntry struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
			Format   string `json:"format"`
			Standard string `json:"standard"`
			Issuer   string `json:"issuer"`
		}
		var entries []catalogEntry
		seen := map[string]bool{}
		for dpgName, issuer := range h.issuerRegistry {
			if issuer == nil {
				continue
			}
			cfgs, err := issuer.ListCredentialConfigs(r.Context())
			if err != nil {
				continue
			}
			for _, c := range cfgs {
				key := dpgName + "/" + c.ID
				if seen[key] {
					continue
				}
				seen[key] = true
				entries = append(entries, catalogEntry{
					ID:       c.ID,
					Name:     c.Name,
					Category: c.Category,
					Format:   c.Format,
					Standard: "OID4VCI",
					Issuer:   issuer.Name(),
				})
			}
		}
		data.Data = map[string]any{
			"schemas": entries,
			"live":    true,
		}
	}

	if err := h.render.Render(w, "holder/catalog", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderRequestForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-request-form", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "New Request", Active: true},
	}
	if user != nil && !user.Demo {
		schemas, _ := h.stores.Schemas.ListSchemas(r.Context())
		data.Data = map[string]any{"schemas": schemas}
	}
	if err := h.render.Render(w, "holder/request_form", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderRequestTracker(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-request-tracker", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "My Requests", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/request_tracker", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderTimeline(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-timeline", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Timeline", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/timeline", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderExport(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder-export", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Export", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	if err := h.render.Render(w, "holder/export", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
