package handler

// bulk.go — handlers for bulk issuance.
//
// Three honestly-labelled sub-modes the UI surfaces depending on what
// the active DPG actually supports:
//
//  1. CSV upload — universal. The UI uploads a CSV, the user maps
//     columns to credential claim fields, and the server streams each
//     row through the issuer's single-issue endpoint. Available on
//     every DPG because the iteration happens in our process.
//
//  2. Database / preconfigured data source — universal whenever the
//     deployment has registered at least one datasource.DataSource
//     (Postgres, Sunbird RC, etc.). The server queries the source via
//     ListRecords and feeds the rows into the same single-issue loop
//     as the CSV path. For Inji Certify this is the spiritual
//     equivalent of the csvdp-* data-provider plugins — same data
//     model, just with the iteration driven by the platform instead
//     of by Inji's per-issuance lookup hook.
//
//  3. HTTP batch API — backend-native. Only enabled when the active
//     issuer's Capabilities().Batch is true (walt.id today). One POST
//     fans out to N credentials inside the DPG itself.
//
// All three paths respect per-user DPG choice via h.issuerFor(user)
// and the in-process LDP signer short-circuit for ldp_vc format.

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"vcplatform/internal/datasource"
	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/render"
	"vcplatform/internal/store/walletbag"
)

// IssuerBulkPage renders the bulk issuance page. All three sub-mode
// tabs (spreadsheet upload, connected database, REST API) are universal
// because every backend bulk path is implemented as a per-row loop in
// the platform — none of the supported DPGs exposes a true batch
// endpoint today, so there's nothing capability-specific to gate.
//
// The only conditional rendering is the database tab, which is hidden
// when no datasource.DataSource is registered in this deployment so
// the user isn't shown an empty picker.
func (h *Handler) IssuerBulkPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	issuer := h.issuerFor(user)

	// Build a lightweight summary of every registered data source so the
	// template can render a picker without making a second round-trip
	// through APIListDataSources.
	rawSources := h.dataSources.List()
	type dsSummary struct {
		Name         string `json:"name"`
		Kind         string `json:"kind"`
		DisplayName  string `json:"displayName"`
		Summary      string `json:"summary"`
		TotalRecords int    `json:"totalRecords"`
	}
	sources := make([]dsSummary, 0, len(rawSources))
	for _, ds := range rawSources {
		s := dsSummary{Name: ds.Name(), Kind: ds.Kind(), DisplayName: ds.Name()}
		if desc, err := ds.Describe(r.Context()); err == nil && desc != nil {
			if desc.DisplayName != "" {
				s.DisplayName = desc.DisplayName
			}
			s.Summary = desc.Summary
			s.TotalRecords = desc.TotalRecords
		}
		sources = append(sources, s)
	}

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
			"supportsDataProvider": len(sources) > 0,
			"dataSources":          sources,
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

// APIIssueBulkDataSource handles POST /api/issuer/bulk-datasource — the
// "Database / preconfigured data source" tab on the bulk page.
//
// The platform queries the named data source via ListRecords, optionally
// remaps each row's field names to credential claim names using the
// caller-supplied mapping, and feeds the records through the same
// per-row issuance loop the CSV path uses.
//
// Body shape:
//
//	{
//	  "source":   "Citizens Database",  // datasource.Name()
//	  "configId": "FarmerCredential",
//	  "format":   "ldp_vc",
//	  "limit":    500,                  // optional, default 100, max 5000
//	  "mapping":  {"fullName": "first_name", ...}  // optional; verbatim if absent
//	}
func (h *Handler) APIIssueBulkDataSource(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		Source   string            `json:"source"`
		ConfigID string            `json:"configId"`
		Format   string            `json:"format"`
		Limit    int               `json:"limit"`
		Mapping  map[string]string `json:"mapping"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Source == "" || req.ConfigID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source and configId required"})
		return
	}
	if req.Format == "" {
		req.Format = "ldp_vc"
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 5000 {
		req.Limit = 5000
	}

	ds := h.dataSources.Get(req.Source)
	if ds == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "data source not found: " + req.Source})
		return
	}

	rows, err := ds.ListRecords(r.Context(), datasource.Filter{Limit: req.Limit})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list records: " + err.Error()})
		return
	}
	if len(rows) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "data source returned 0 records"})
		return
	}

	records := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		rec := map[string]any{}
		if len(req.Mapping) == 0 {
			// No mapping — use source field names verbatim as claim names.
			for k, v := range row {
				rec[k] = v
			}
		} else {
			for claim, sourceField := range req.Mapping {
				if v, ok := row[sourceField]; ok {
					rec[claim] = v
				}
			}
		}
		records = append(records, rec)
	}

	out := h.issueMany(r, user, req.ConfigID, req.Format, records)
	out["source"] = ds.Name()
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
