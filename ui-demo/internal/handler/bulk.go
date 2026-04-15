package handler

// bulk.go — Phase 4 handlers for bulk issuance.
//
// Two sub-modes:
//
//  1. API — expose a POST endpoint that accepts a JSON array of claim records
//     and issues one credential per record. The UI shows the user a curl
//     snippet + live "try it" button.
//
//  2. Data-provider plugin — the UI uploads a CSV, the user maps columns to
//     credential claim fields, and the server streams each row through the
//     issuer. This is how Inji Certify and similar DPGs support bulk
//     issuance without a HTTP batch endpoint.
//
// Both paths respect per-user DPG choice via h.issuerFor(user) and the
// in-process LDP signer short-circuit for ldp_vc format.

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/render"
	"vcplatform/internal/store/walletbag"
)

// IssuerBulkPage renders the bulk issuance page with API + data-provider tabs.
func (h *Handler) IssuerBulkPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Determine which bulk modes the current DPG supports based on capabilities.
	issuer := h.issuerFor(user)
	caps := issuer.Capabilities()
	supportsAPI := caps.Batch // batch = HTTP bulk endpoint at the backend level
	// Data-provider plugin is always available if we have at least one
	// registered data source (the UI uses our Go DataSource interface, not
	// the backend's bulk endpoint).
	supportsDataProvider := len(h.dataSources.List()) > 0

	schemas := h.getSchemas(user)

	data := render.PageData{
		Config:     h.config,
		User:       user,
		Mode:       h.config.Mode,
		IsHTMX:     middleware.IsHTMX(r.Context()),
		ActivePage: "issuer-bulk",
		Data: map[string]any{
			"schemas":              schemas,
			"dpgName":              issuer.Name(),
			"supportsAPI":          supportsAPI,
			"supportsDataProvider": supportsDataProvider,
		},
	}
	if err := h.render.Render(w, "issuer/bulk", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// APIIssueBulkAPI handles POST /api/issuer/bulk-api — accepts a JSON array of
// claim records and issues one credential per record.
//
// Body: {"configId": "...", "format": "ldp_vc", "records": [ {...}, {...} ]}
func (h *Handler) APIIssueBulkAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		ConfigID string           `json:"configId"`
		Format   string           `json:"format"`
		Records  []map[string]any `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.ConfigID == "" || len(req.Records) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "configId and non-empty records required"})
		return
	}
	if req.Format == "" {
		req.Format = "ldp_vc"
	}

	out := h.issueMany(r, user, req.ConfigID, req.Format, req.Records)
	writeJSON(w, http.StatusOK, out)
}

// APIIssueBulkCSV handles POST /api/issuer/bulk-csv — multipart form with:
//
//   - file:      the CSV file
//   - configId:  credential configuration ID
//   - format:    credential format
//   - mapping:   JSON object of {claimName: csvColumn}
func (h *Handler) APIIssueBulkCSV(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	configID := r.FormValue("configId")
	format := r.FormValue("format")
	mappingJSON := r.FormValue("mapping")
	if configID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "configId required"})
		return
	}
	if format == "" {
		format = "ldp_vc"
	}

	// Parse the column→claim mapping ({claimName: csvColumn}).
	var mapping map[string]string
	if mappingJSON != "" {
		_ = json.Unmarshal([]byte(mappingJSON), &mapping)
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read CSV header: " + err.Error()})
		return
	}
	headerIdx := map[string]int{}
	for i, h := range header {
		headerIdx[strings.TrimSpace(h)] = i
	}

	records := []map[string]any{}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		rec := map[string]any{}
		if len(mapping) == 0 {
			// No mapping — use CSV headers verbatim as claim names.
			for name, idx := range headerIdx {
				if idx < len(row) {
					rec[name] = row[idx]
				}
			}
		} else {
			for claim, col := range mapping {
				if idx, ok := headerIdx[col]; ok && idx < len(row) {
					rec[claim] = row[idx]
				}
			}
		}
		records = append(records, rec)
	}

	out := h.issueMany(r, user, configID, format, records)
	writeJSON(w, http.StatusOK, out)
}

// issueMany runs the batch through whichever issuance path the current DPG
// + format combination supports, short-circuiting ldp_vc through our
// in-process signer when the backend can't natively issue it.
//
// Returns a summary with per-record results.
func (h *Handler) issueMany(r *http.Request, user *model.User, configID, format string, records []map[string]any) map[string]any {
	issuerStore := h.issuerFor(user)
	issuerName := strings.ToLower(issuerStore.Name())
	useLocalLDP := format == "ldp_vc" && h.ldpSigner != nil && !strings.Contains(issuerName, "inji")

	results := make([]map[string]any, 0, len(records))
	issued := 0
	failed := 0

	// Warm up the issuer DID once for backends that need it.
	var issuer *model.OnboardIssuerResult
	if !useLocalLDP {
		r0, err := issuerStore.OnboardIssuer(r.Context(), "secp256r1")
		if err != nil {
			return map[string]any{
				"issued": 0,
				"failed": len(records),
				"error":  "onboard: " + err.Error(),
			}
		}
		issuer = r0
	}

	types := deriveCredentialTypes(configID)
	for i, rec := range records {
		if useLocalLDP {
			credJSON, err := h.ldpSigner.SignJSON(types, rec, "")
			if err != nil {
				failed++
				results = append(results, map[string]any{"index": i, "error": err.Error()})
				continue
			}
			var parsed map[string]any
			_ = json.Unmarshal(credJSON, &parsed)
			credID, _ := parsed["id"].(string)
			walletbag.Shared.Add(user.WalletToken, model.WalletCredential{
				ID:             credID,
				Format:         "ldp_vc",
				AddedOn:        time.Now().Format("2006-01-02 15:04"),
				Document:       string(credJSON),
				ParsedDocument: parsed,
			})
			issued++
			results = append(results, map[string]any{"index": i, "credentialId": credID, "status": "issued"})
			continue
		}

		offer, err := issuerStore.IssueCredential(r.Context(), issuer, configID, format, rec)
		if err != nil {
			failed++
			results = append(results, map[string]any{"index": i, "error": err.Error()})
			continue
		}
		issued++
		results = append(results, map[string]any{"index": i, "offerUrl": offer, "status": "issued"})
	}
	return map[string]any{
		"issued":  issued,
		"failed":  failed,
		"total":   len(records),
		"results": results,
	}
}
