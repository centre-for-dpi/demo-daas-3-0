package handler

import (
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

func (h *Handler) trustPage(w http.ResponseWriter, r *http.Request, tmpl, crumb string) {
	user := middleware.GetUser(r.Context())
	extra := map[string]any{
		"hasBackendAuth": user != nil && user.HasBackendAuth(),
		"backendType":    h.config.Backend.Type,
		"walletURL":      h.config.Backend.WalletURL,
		"issuerURL":      h.config.Backend.IssuerURL,
		"verifierURL":    h.config.Backend.VerifierURL,
	}
	data := h.pageData(r, "trust", extra)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Trust"},
		{Label: crumb, Active: true},
	}
	if err := h.render.Render(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) TrustSchemaRegistry(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	extra := map[string]any{
		"hasBackendAuth": user != nil && user.HasBackendAuth(),
		"backendType":    h.config.Backend.Type,
	}

	if user != nil && user.HasBackendAuth() {
		schemas, err := h.stores.Schemas.ListSchemas(r.Context())
		if err == nil {
			extra["schemas"] = schemas
		}
	}

	data := h.pageData(r, "trust", extra)
	data.Breadcrumb = []model.BreadcrumbItem{
		{Label: "Trust"},
		{Label: "Schema Registry", Active: true},
	}
	if err := h.render.Render(w, "trust/schema_registry", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) TrustSchemaPublish(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/schema_publish", "Publish Schema")
}

func (h *Handler) TrustIssuerDirectory(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/issuer_directory", "Issuer Directory")
}

func (h *Handler) TrustRegistryAdmin(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/trust_admin", "Registry Admin")
}

func (h *Handler) TrustVerifierDirectory(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/verifier_directory", "Verifier Directory")
}

func (h *Handler) TrustGovernance(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/governance", "Governance")
}

func (h *Handler) TrustAdaptorRegistry(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/adaptor_registry", "Adaptor Registry")
}

func (h *Handler) TrustAdaptorConfig(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/adaptor_config", "Adaptor Config")
}

func (h *Handler) TrustProtocolMonitor(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/protocol_monitor", "Protocol Monitor")
}

func (h *Handler) TrustBridge(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/trust_bridge", "Trust Bridge")
}

func (h *Handler) TrustSchemaHarmonize(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/schema_harmonize", "Schema Harmonization")
}

func (h *Handler) TrustChannelConfig(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/channel_config", "Channel Adaptors")
}

func (h *Handler) TrustOfflineSync(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/offline_sync", "Offline Sync")
}

func (h *Handler) TrustAgentMode(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/agent_mode", "Agent-Assisted")
}

func (h *Handler) TrustConnectivity(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/connectivity", "Connectivity Health")
}

func (h *Handler) TrustMultimodal(w http.ResponseWriter, r *http.Request) {
	h.trustPage(w, r, "trust/multimodal", "Multi-Modal Preview")
}
