package handler

import (
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

func (h *Handler) issuerPage(w http.ResponseWriter, r *http.Request, tmpl, crumb string) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: crumb, Active: true},
	}
	if err := h.render.Render(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// issuerSchemaData returns user-created schemas as template data for real users.
func (h *Handler) issuerSchemaData(r *http.Request) map[string]any {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		return nil
	}
	schemas := h.getSchemas(user)
	return map[string]any{"schemas": schemas}
}

// IssuerSchemas fetches real schemas from the store.
func (h *Handler) IssuerSchemas(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: "Schemas", Active: true},
	}
	if sd := h.issuerSchemaData(r); sd != nil {
		data.Data = sd
	}
	if err := h.render.Render(w, "issuer/schema_list", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) IssuerOnboardForm(w http.ResponseWriter, r *http.Request) {
	h.issuerPage(w, r, "issuer/onboard_form", "Register Issuer")
}

func (h *Handler) IssuerOnboardQueue(w http.ResponseWriter, r *http.Request) {
	h.issuerPage(w, r, "issuer/onboard_queue", "Onboarding Queue")
}

// IssuerDIDKeys shows issuer-specific DIDs (created via the registration/onboarding flow).
func (h *Handler) IssuerDIDKeys(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: "DID & Keys", Active: true},
	}

	if user != nil && !user.Demo {
		dids := h.getIssuerDIDs(user)
		data.Data = map[string]any{
			"issuerDIDs": dids,
		}
	}

	if err := h.render.Render(w, "issuer/did_keys", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) IssuerSchemaBuilder(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: "Schema Builder", Active: true},
	}

	if user != nil && !user.Demo {
		schemas, err := h.stores.Schemas.ListSchemas(r.Context())
		if err == nil {
			data.Data = map[string]any{
				"schemas": schemas,
			}
		}
	}

	if err := h.render.Render(w, "issuer/schema_builder", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) IssuerTemplates(w http.ResponseWriter, r *http.Request) {
	h.issuerPage(w, r, "issuer/template_list", "Templates")
}

func (h *Handler) IssuerTemplateEditor(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Template Editor", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/template_editor", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerCredPreview(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Credential Preview", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/cred_preview", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerCampaign(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Campaign Builder", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/campaign", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerBatch(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Batch Upload", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/batch", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerReview(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Review & Sign", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/issue_review", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerDispatch(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Dispatch Monitor", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/dispatch", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerSingleIssue(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{{Label: "Issuer"}, {Label: "Single Issuance", Active: true}}
	if sd := h.issuerSchemaData(r); sd != nil { data.Data = sd }
	if err := h.render.Render(w, "issuer/single_issue", data); err != nil { http.Error(w, "template error", 500) }
}

func (h *Handler) IssuerCredentials(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: "Issued Credentials", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		if walletData := h.getWalletData(r.Context(), user); walletData != nil {
			data.Data = walletData
		}
	}

	if err := h.render.Render(w, "issuer/issued_creds", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) IssuerCredDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: "Credential Detail", Active: true},
	}

	credID := r.URL.Query().Get("id")

	if credID != "" && user != nil && user.HasBackendAuth() {
		wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
		if err == nil && len(wallets) > 0 {
			creds, err := h.stores.Wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
			if err == nil {
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

	if err := h.render.Render(w, "issuer/cred_detail", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) IssuerRevocation(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "issuer", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Issuer"},
		{Label: "Revocation", Active: true},
	}

	credID := r.URL.Query().Get("id")

	if credID != "" && user != nil && user.HasBackendAuth() {
		wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
		if err == nil && len(wallets) > 0 {
			creds, err := h.stores.Wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
			if err == nil {
				for _, c := range creds {
					if c.ID == credID {
						data.Data = map[string]any{
							"credential": c,
							"live":       true,
						}
						break
					}
				}
			}
		}
	}

	if err := h.render.Render(w, "issuer/revocation", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
