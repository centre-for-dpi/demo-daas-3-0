package handler

import (
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

func (h *Handler) verifierPage(w http.ResponseWriter, r *http.Request, tmpl, crumb string) {
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: crumb, Active: true},
	}
	if err := h.render.Render(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierPortal passes policies for real users.
func (h *Handler) VerifierPortal(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "Verification Portal", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		policies, err := h.stores.Verifier.ListPolicies(r.Context())
		if err == nil {
			data.Data = map[string]any{
				"policies": policies,
				"live":     true,
			}
		}
	}

	if err := h.render.Render(w, "verifier/portal", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierRequestBuilder passes policies and schemas for real users.
func (h *Handler) VerifierRequestBuilder(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "Request Builder", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		dm := map[string]any{"live": true}
		policies, err := h.stores.Verifier.ListPolicies(r.Context())
		if err == nil {
			dm["policies"] = policies
		}
		schemas, err := h.stores.Schemas.ListSchemas(r.Context())
		if err == nil {
			dm["schemas"] = schemas
		}
		data.Data = dm
	}

	if err := h.render.Render(w, "verifier/request_builder", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierQRGenerator passes live flag for real users.
func (h *Handler) VerifierQRGenerator(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "QR / Deep Link", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		data.Data = map[string]any{
			"live": true,
		}
	}

	if err := h.render.Render(w, "verifier/qr_generator", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierDashboard shows verification policies if available.
func (h *Handler) VerifierDashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "Results", Active: true},
	}

	// Fetch verification policies if backend is available
	if user != nil && user.HasBackendAuth() {
		policies, err := h.stores.Verifier.ListPolicies(r.Context())
		if err == nil {
			data.Data = map[string]any{
				"policies": policies,
				"live":     true,
			}
		}
	}

	if err := h.render.Render(w, "verifier/dashboard", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierProofDetail passes live flag for real users.
func (h *Handler) VerifierProofDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "Proof Detail", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		data.Data = map[string]any{
			"live": true,
		}
	}

	if err := h.render.Render(w, "verifier/proof_detail", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierHistory passes live flag for real users.
func (h *Handler) VerifierHistory(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "History", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		data.Data = map[string]any{
			"live": true,
		}
	}

	if err := h.render.Render(w, "verifier/history", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// VerifierOffline passes policies for real users.
func (h *Handler) VerifierOffline(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "Online / Offline", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		dm := map[string]any{"live": true}
		policies, err := h.stores.Verifier.ListPolicies(r.Context())
		if err == nil {
			dm["policies"] = policies
		}
		data.Data = dm
	}

	if err := h.render.Render(w, "verifier/offline", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) VerifierSDKGuide(w http.ResponseWriter, r *http.Request) {
	h.verifierPage(w, r, "verifier/sdk_guide", "SDK Integration")
}

// VerifierIntegration passes backend config info for real users.
func (h *Handler) VerifierIntegration(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "verifier", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Verifier"},
		{Label: "Integration Status", Active: true},
	}

	if user != nil && user.HasBackendAuth() {
		data.Data = map[string]any{
			"live":        true,
			"backendType": h.config.Backend.Type,
			"verifierURL": h.config.Backend.VerifierURL,
			"issuerURL":   h.config.Backend.IssuerURL,
			"walletURL":   h.config.Backend.WalletURL,
			"transport":   h.config.Backend.Transport,
		}
	}

	if err := h.render.Render(w, "verifier/integration", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
