package handler

import (
	"context"
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

// getWalletData fetches credentials and DIDs for an authenticated user.
func (h *Handler) getWalletData(ctx context.Context, user *model.User) map[string]any {
	if user == nil || !user.HasBackendAuth() {
		return nil
	}
	result := map[string]any{}
	wallets, err := h.stores.Wallet.GetWallets(ctx, user.WalletToken)
	if err != nil || len(wallets) == 0 {
		return nil
	}
	result["walletID"] = wallets[0].ID

	creds, err := h.stores.Wallet.ListCredentials(ctx, user.WalletToken, wallets[0].ID)
	if err == nil {
		result["credentials"] = creds
	}

	dids, err := h.stores.Wallet.ListDIDs(ctx, user.WalletToken, wallets[0].ID)
	if err == nil {
		result["dids"] = dids
	}

	return result
}

// HolderWallet fetches real credentials from the backend and passes them to the template.
func (h *Handler) HolderWallet(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Credentials", Active: true},
	}

	// Fetch real credentials if user has backend auth
	if user != nil && user.HasBackendAuth() {
		wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
		if err == nil && len(wallets) > 0 {
			creds, err := h.stores.Wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
			if err == nil {
				data.Data = map[string]any{
					"credentials": creds,
					"walletID":    wallets[0].ID,
					"live":        true,
				}
			}
		}
	}

	if err := h.render.Render(w, "holder/wallet", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// HolderCredDetail fetches a single credential's details from the wallet.
func (h *Handler) HolderCredDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Credential Detail", Active: true},
	}

	credID := r.URL.Query().Get("id")

	// Fetch real credential if user has backend auth and an ID was provided
	if credID != "" && user != nil && user.HasBackendAuth() {
		wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
		if err == nil && len(wallets) > 0 {
			creds, err := h.stores.Wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
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
	data := h.pageData(r, "holder", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Claim Credential", Active: true},
	}
	if err := h.render.Render(w, "holder/claim", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderRetrieval(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Retrieve", Active: true},
	}
	if walletData := h.getWalletData(r.Context(), user); walletData != nil {
		data.Data = walletData
	}
	// Also fetch schemas as available credential types for the retrieval form
	if user != nil && !user.Demo {
		schemas, err := h.stores.Schemas.ListSchemas(r.Context())
		if err == nil {
			if data.Data == nil {
				data.Data = map[string]any{}
			}
			dm := data.Data.(map[string]any)
			dm["schemas"] = schemas
		}
	}
	if err := h.render.Render(w, "holder/retrieval", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderDependents(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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

// HolderCatalog shows available credential types from the schema registry.
func (h *Handler) HolderCatalog(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Holder"},
		{Label: "Credential Catalog", Active: true},
	}

	// Fetch schemas as available credential types
	if user != nil && !user.Demo {
		schemas, err := h.stores.Schemas.ListSchemas(r.Context())
		if err == nil {
			data.Data = map[string]any{
				"schemas": schemas,
				"live":    true,
			}
		}
	}

	if err := h.render.Render(w, "holder/catalog", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) HolderRequestForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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
	data := h.pageData(r, "holder", nil)
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
