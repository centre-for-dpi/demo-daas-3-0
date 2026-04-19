package handlers

import (
	"net/http"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/vctypes"
)

type modeData struct {
	Dpg         vctypes.DPG
	SelectedScale string
	SelectedDest  string
}

// ShowIssuanceMode renders the scale + destination choice screen.
func (h *H) ShowIssuanceMode(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.IssuerDpg == "" || sess.SchemaID == "" {
		h.redirect(w, r, "/issuer/dpg")
		return
	}
	dpgs, err := h.Adapter.ListIssuerDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	data := modeData{
		Dpg:           dpgs[sess.IssuerDpg],
		SelectedScale: sess.Scale,
		SelectedDest:  sess.Dest,
	}
	// Auto-force dest=wallet if DPG doesn't support PDF
	if !data.Dpg.DirectPDF && sess.Dest == "pdf" {
		sess.Dest = "wallet"
		data.SelectedDest = "wallet"
	}
	h.render(w, r, "issuer_mode", h.pageData(sess, data))
}

// SetIssuanceMode accepts scale/dest POST and redirects to /issuer/issue.
func (h *H) SetIssuanceMode(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if scale := r.FormValue("scale"); scale != "" {
		sess.Scale = scale
	}
	if dest := r.FormValue("dest"); dest != "" {
		sess.Dest = dest
	}
	h.redirect(w, r, "/issuer/issue")
}

type issueData struct {
	Schema       vctypes.Schema
	Scale        string
	Dest         string
	IssuerDpg    string
	SingleSource string // "manual" | "api" | "mosip" | "db" | "pdi"
	FieldValues  map[string]string
	Fields       []string
}

// ShowIssue renders the issuance-form screen.
func (h *H) ShowIssue(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.IssuerDpg == "" || sess.SchemaID == "" {
		h.redirect(w, r, "/issuer/dpg")
		return
	}
	schemas, _ := h.Adapter.ListAllSchemas(r.Context())
	var schema vctypes.Schema
	for _, s := range schemas {
		if s.ID == sess.SchemaID {
			schema = s
			break
		}
	}
	if schema.ID == "" {
		h.errorToast(w, r, "selected schema missing")
		return
	}
	vals, _ := h.Adapter.PrefillSubjectFields(r.Context(), schema)
	data := issueData{
		Schema:       schema,
		Scale:        sess.Scale,
		Dest:         sess.Dest,
		IssuerDpg:    sess.IssuerDpg,
		SingleSource: "manual",
		FieldValues:  vals,
		Fields:       schemaFieldsOfH(schema),
	}
	h.render(w, r, "issuer_issue", h.pageData(sess, data))
}

// SubmitIssue performs the issuance and returns a result fragment.
func (h *H) SubmitIssue(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	_ = r.ParseForm()
	schemas, _ := h.Adapter.ListAllSchemas(r.Context())
	var schema vctypes.Schema
	for _, s := range schemas {
		if s.ID == sess.SchemaID {
			schema = s
			break
		}
	}
	// Gather subject data from form (falls back to prefill)
	subject := map[string]string{}
	for _, f := range schemaFieldsOfH(schema) {
		v := r.FormValue("field_" + f)
		subject[f] = v
	}
	req := backend.IssueRequest{IssuerDpg: sess.IssuerDpg, Schema: schema, SubjectData: subject}

	if sess.Dest == "wallet" {
		res, err := h.Adapter.IssueToWallet(r.Context(), req)
		if err != nil {
			h.errorToast(w, r, err.Error())
			return
		}
		h.renderFragment(w, "fragment_issue_wallet_result", res)
		return
	}
	// PDF
	res, err := h.Adapter.IssueAsPDF(r.Context(), req)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragment(w, "fragment_issue_pdf_result", map[string]any{
		"Schema":    schema,
		"PDFResult": res,
		"Fields":    schemaFieldsOfH(schema),
	})
}

// SetSingleSource switches the issuance form's source (manual/API/MOSIP/DB/PDI).
func (h *H) SetSingleSource(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	source := r.FormValue("source")
	if source == "" {
		source = "manual"
	}
	schemas, _ := h.Adapter.ListAllSchemas(r.Context())
	var schema vctypes.Schema
	for _, s := range schemas {
		if s.ID == sess.SchemaID {
			schema = s
			break
		}
	}
	vals, _ := h.Adapter.PrefillSubjectFields(r.Context(), schema)
	data := issueData{
		Schema:       schema,
		IssuerDpg:    sess.IssuerDpg,
		SingleSource: source,
		FieldValues:  vals,
		Fields:       schemaFieldsOfH(schema),
	}
	h.renderFragment(w, "fragment_issue_single_form", data)
}

// SimulateCSV renders the bulk-CSV preview.
func (h *H) SimulateCSV(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	schemas, _ := h.Adapter.ListAllSchemas(r.Context())
	var schema vctypes.Schema
	for _, s := range schemas {
		if s.ID == sess.SchemaID {
			schema = s
			break
		}
	}
	res, _ := h.Adapter.IssueBulk(r.Context(), backend.IssueBulkRequest{
		IssuerDpg: sess.IssuerDpg, Schema: schema, RowCount: 247,
	})
	vals, _ := h.Adapter.PrefillSubjectFields(r.Context(), schema)
	h.renderFragment(w, "fragment_issue_csv_preview", map[string]any{
		"Schema":    schema,
		"Fields":    schemaFieldsOfH(schema),
		"Values":    vals,
		"Total":     247,
		"Accepted":  res.Accepted,
		"Rejected":  res.Rejected,
		"Errors":    res.Errors,
	})
}

// PreviewPDF opens the PDF preview modal.
func (h *H) PreviewPDF(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	schemas, _ := h.Adapter.ListAllSchemas(r.Context())
	var schema vctypes.Schema
	for _, s := range schemas {
		if s.ID == sess.SchemaID {
			schema = s
			break
		}
	}
	vals, _ := h.Adapter.PrefillSubjectFields(r.Context(), schema)
	res, err := h.Adapter.IssueAsPDF(r.Context(), backend.IssueRequest{
		IssuerDpg: sess.IssuerDpg, Schema: schema, SubjectData: vals,
	})
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragment(w, "fragment_pdf_preview_modal", map[string]any{
		"Schema":    schema,
		"Fields":    schemaFieldsOfH(schema),
		"PDFResult": res,
	})
}

// schemaFieldsOfH returns the field names for a schema. Works for both custom
// and pre-configured schemas because both populate FieldsSpec now.
func schemaFieldsOfH(s vctypes.Schema) []string {
	out := make([]string, 0, len(s.FieldsSpec))
	for _, f := range s.FieldsSpec {
		out = append(out, f.Name)
	}
	return out
}
