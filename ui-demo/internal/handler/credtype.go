package handler

import (
	"encoding/json"
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

// APIListCredentialConfigs handles GET /api/credential-types — lists
// credential configurations available to the UI. Merges the active issuer's
// backend-advertised configs with the in-process LDP_VC signer's catalog,
// so users see the full format matrix (jwt_vc_json / vc+sd-jwt / ldp_vc /
// mso_mdoc) regardless of which DPG is the primary issuer.
func (h *Handler) APIListCredentialConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.stores.Issuer.ListCredentialConfigs(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Append in-process LDP_VC configs ONLY when the signer is available AND
	// the current issuer doesn't already advertise ldp_vc natively. Inji Certify
	// advertises ldp_vc natively, so there's no need to duplicate.
	if h.ldpSigner != nil {
		hasNativeLDP := false
		for _, c := range configs {
			if c.Format == "ldp_vc" {
				hasNativeLDP = true
				break
			}
		}
		if !hasNativeLDP {
			configs = append(configs,
				ldpConfig("UniversityDegree", "University Degree (LDP_VC)", "Education"),
				ldpConfig("FarmerCredential", "Farmer Credential (LDP_VC)", "Agriculture"),
				ldpConfig("BirthCertificate", "Birth Certificate (LDP_VC)", "Identity"),
			)
		}
	}
	writeJSON(w, 200, configs)
}

// ldpConfig builds a CredentialConfig entry for the in-process LDP_VC signer.
func ldpConfig(id, name, category string) model.CredentialConfig {
	return model.CredentialConfig{
		ID:       id,
		Name:     name,
		Category: category,
		Format:   "ldp_vc",
	}
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
