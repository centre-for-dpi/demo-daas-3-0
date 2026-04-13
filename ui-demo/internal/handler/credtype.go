package handler

import (
	"encoding/json"
	"net/http"

	"vcplatform/internal/middleware"
)

// APIListCredentialConfigs handles GET /api/credential-types — lists configs from the backend.
func (h *Handler) APIListCredentialConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.stores.Issuer.ListCredentialConfigs(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, configs)
}

// APIRegisterCredentialType handles POST /api/credential-types/register — delegates to the backend.
func (h *Handler) APIRegisterCredentialType(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		TypeName    string `json:"typeName"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		Format      string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.TypeName == "" {
		writeJSON(w, 400, map[string]string{"error": "typeName required"})
		return
	}
	if req.Format == "" {
		req.Format = "jwt_vc_json"
	}
	if req.DisplayName == "" {
		req.DisplayName = req.TypeName
	}

	configID, err := h.stores.Issuer.RegisterCredentialType(r.Context(), req.TypeName, req.DisplayName, req.Description, req.Format)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{
		"status":   "registered",
		"configId": configID,
	})
}
