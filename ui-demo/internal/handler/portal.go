package handler

import (
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

func (h *Handler) portalPage(w http.ResponseWriter, r *http.Request, page, tmpl string, breadcrumb []model.BreadcrumbItem) {
	data := h.pageData(r, page, nil)
	data.Breadcrumb = breadcrumb
	if err := h.render.Render(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// PortalDashboard injects real data for ALL roles when authenticated.
func (h *Handler) PortalDashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "dashboard", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Platform"},
		{Label: "Dashboard", Active: true},
	}

	if user != nil && !user.Demo {
		dm := map[string]any{}

		// Fetch wallet credentials and DIDs (needs wallet token)
		if user.HasBackendAuth() {
			wallet := h.walletFor(user)
			wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
			if err == nil && len(wallets) > 0 {
				creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
				if err == nil {
					dm["credentials"] = creds
					dm["credCount"] = len(creds)
				}
				dids, err := wallet.ListDIDs(r.Context(), user.WalletToken, wallets[0].ID)
				if err == nil {
					dm["dids"] = dids
				}
			}
		}

		// Fetch user-created schemas and issuer DIDs
		userSchemas := h.getSchemas(user)
		dm["schemas"] = userSchemas
		dm["schemaCount"] = len(userSchemas)
		issuerDIDs := h.getIssuerDIDs(user)
		dm["issuerDIDs"] = issuerDIDs
		dm["issuerDIDCount"] = len(issuerDIDs)

		data.Data = dm
	}

	if err := h.render.Render(w, "portal/dashboard", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	h.portalPage(w, r, "profile", "portal/profile", []model.BreadcrumbItem{
		{Label: "Account"},
		{Label: "Profile", Active: true},
	})
}

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	h.portalPage(w, r, "notifications", "portal/notifications", []model.BreadcrumbItem{
		{Label: "Platform"},
		{Label: "Notifications", Active: true},
	})
}

func (h *Handler) NotifConfig(w http.ResponseWriter, r *http.Request) {
	h.portalPage(w, r, "notif-config", "portal/notif_config", []model.BreadcrumbItem{
		{Label: "Platform"},
		{Label: "Notification Config", Active: true},
	})
}

func (h *Handler) RBAC(w http.ResponseWriter, r *http.Request) {
	h.portalPage(w, r, "rbac", "portal/rbac", []model.BreadcrumbItem{
		{Label: "Administration"},
		{Label: "Roles & Access", Active: true},
	})
}

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	h.portalPage(w, r, "users", "portal/users", []model.BreadcrumbItem{
		{Label: "Administration"},
		{Label: "User Directory", Active: true},
	})
}

// AuditLog — real users see "no audit entries" empty state.
func (h *Handler) AuditLog(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "audit-log", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Audit"},
		{Label: "Audit Log", Active: true},
	}
	if user != nil && user.HasBackendAuth() {
		data.Data = map[string]any{"live": true}
	}
	if err := h.render.Render(w, "portal/audit_log", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	data := h.pageData(r, "activity", nil)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Audit"},
		{Label: "Activity Dashboard", Active: true},
	}
	if user != nil && user.HasBackendAuth() {
		data.Data = map[string]any{"live": true}
	}
	if err := h.render.Render(w, "portal/activity", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
