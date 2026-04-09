package handler

import (
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

func (h *Handler) adminPage(w http.ResponseWriter, r *http.Request, tmpl, crumb string) {
	user := middleware.GetUser(r.Context())
	extra := map[string]any{
		"hasBackendAuth": user != nil && user.HasBackendAuth(),
		"backendType":    h.config.Backend.Type,
		"walletURL":      h.config.Backend.WalletURL,
		"issuerURL":      h.config.Backend.IssuerURL,
		"verifierURL":    h.config.Backend.VerifierURL,
	}
	data := h.pageData(r, "platform-admin", extra)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Admin"},
		{Label: crumb, Active: true},
	}
	if err := h.render.Render(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) AdminIntake(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/intake", "Issuer Intake")
}

func (h *Handler) AdminSchemaWizard(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/guided_schema", "Schema Wizard")
}

func (h *Handler) AdminPreviewGen(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/preview_gen", "Preview Generator")
}

func (h *Handler) AdminSandbox(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/sandbox", "Sandbox")
}

func (h *Handler) AdminApprovalQueue(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/approval_queue", "Review & Approve")
}

func (h *Handler) AdminDeployment(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/deployment", "Deployment")
}

func (h *Handler) AdminPortability(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/portability", "Data Portability")
}

func (h *Handler) AdminReporting(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/reporting", "Reporting")
}

func (h *Handler) AdminTraining(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/training", "Training Hub")
}

func (h *Handler) AdminHealth(w http.ResponseWriter, r *http.Request) {
	h.adminPage(w, r, "admin/health", "Platform Health")
}
