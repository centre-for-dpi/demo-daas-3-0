package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/vctypes"
)

// ShowVerify renders the verifier page (OID4VP request generator + direct
// verify). Also surfaces the schema catalog so the "Build a custom request"
// card can offer them as a starting point.
func (h *H) ShowVerify(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.VerifierDpg == "" {
		h.redirect(w, r, "/verifier/dpg")
		return
	}
	templates, err := h.Adapter.ListOID4VPTemplates(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	dpgs, _ := h.Adapter.ListVerifierDpgs(r.Context())
	schemas, _ := h.Adapter.ListAllSchemas(r.Context())
	body := map[string]any{
		"Templates":       templates,
		"VerifierDpgObj":  dpgs[sess.VerifierDpg],
		"Schemas":         schemas,
		"CustomTemplate":  sess.CustomOID4VPTemplate,
		"CustomSchemaID":  sess.CustomOID4VPSchemaID,
	}
	h.render(w, r, "verifier_verify", h.pageData(sess, body))
}

// GenerateRequest creates an OID4VP presentation request. Two modes:
//
//   - Preset: form.template is one of the curated ListOID4VPTemplates keys.
//
//   - Custom: form.template == "custom" and the form ALSO carries schema_id,
//     field_key[], and disclosure from the "Build a custom request" card.
//     Handler assembles an inline OID4VPTemplate on the fly so the user
//     clicks ONE button (no intermediate "Assemble template" step that
//     would leave the dropdown stale).
func (h *H) GenerateRequest(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if err := r.ParseForm(); err != nil {
		h.errorToast(w, r, "Bad form: "+err.Error())
		return
	}
	template := r.FormValue("template")
	if template == "" {
		template = "age"
	}
	req := backend.PresentationRequest{
		VerifierDpg: sess.VerifierDpg,
		TemplateKey: template,
	}
	if template == "custom" {
		tpl, err := h.assembleCustomTemplate(r)
		if err != nil {
			h.errorToast(w, r, err.Error())
			return
		}
		sess.CustomOID4VPTemplate = &tpl
		sess.CustomOID4VPSchemaID = r.FormValue("schema_id")
		req.Template = &tpl
	}
	res, err := h.Adapter.RequestPresentation(r.Context(), req)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	sess.CurrentOID4VPLink = res.RequestURI
	sess.CurrentOID4VPState = res.State
	sess.CurrentOID4VPTemplate = template
	h.renderFragment(w, r, "fragment_oid4vp_request_output", res)
}

// assembleCustomTemplate builds a OID4VPTemplate from the custom-request
// form fields on r: schema_id picks the schema, field_key[] is the list of
// fields to request (defaults to all schema fields if none are checked),
// disclosure is "selective" or "full".
func (h *H) assembleCustomTemplate(r *http.Request) (vctypes.OID4VPTemplate, error) {
	schemaID := r.FormValue("schema_id")
	if schemaID == "" {
		return vctypes.OID4VPTemplate{}, fmt.Errorf("pick a schema first")
	}
	schemas, err := h.Adapter.ListAllSchemas(r.Context())
	if err != nil {
		return vctypes.OID4VPTemplate{}, fmt.Errorf("could not load schemas: %w", err)
	}
	var picked *vctypes.Schema
	for i := range schemas {
		if schemas[i].ID == schemaID {
			picked = &schemas[i]
			break
		}
	}
	if picked == nil {
		return vctypes.OID4VPTemplate{}, fmt.Errorf("unknown schema %q", schemaID)
	}
	fields := r.Form["field_key"]
	if len(fields) == 0 {
		// No boxes checked → request every field the schema declares.
		for _, f := range picked.FieldsSpec {
			fields = append(fields, f.Name)
		}
	}
	// Reject fields that aren't in the schema.
	valid := make(map[string]bool, len(picked.FieldsSpec))
	for _, f := range picked.FieldsSpec {
		valid[f.Name] = true
	}
	cleaned := fields[:0]
	for _, f := range fields {
		if valid[f] {
			cleaned = append(cleaned, f)
		}
	}
	fields = cleaned

	disclosure := r.FormValue("disclosure")
	if disclosure == "" {
		disclosure = "full"
	}
	return vctypes.OID4VPTemplate{
		Title:      picked.Name,
		Fields:     fields,
		Format:     picked.Std,
		Disclosure: disclosureSummary(disclosure, fields),
	}, nil
}

// BuildVerifierTemplate renders the field-picker fragment for the schema
// the user selected. Fires from the schema dropdown's change event so the
// checkboxes appear immediately; the actual template is re-assembled at
// Generate time (via assembleCustomTemplate) rather than stashed here —
// that keeps the dropdown and the built template from drifting out of
// sync when the user changes their mind.
func (h *H) BuildVerifierTemplate(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.VerifierDpg == "" {
		h.redirect(w, r, "/verifier/dpg")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.errorToast(w, r, "Bad form: "+err.Error())
		return
	}
	schemaID := r.FormValue("schema_id")
	if schemaID == "" {
		// Empty dropdown → clear the preview area.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(""))
		return
	}
	schemas, err := h.Adapter.ListAllSchemas(r.Context())
	if err != nil {
		h.errorToast(w, r, "Could not load schemas: "+err.Error())
		return
	}
	var picked *vctypes.Schema
	for i := range schemas {
		if schemas[i].ID == schemaID {
			picked = &schemas[i]
			break
		}
	}
	if picked == nil {
		h.errorToast(w, r, "Unknown schema "+schemaID)
		return
	}
	// Pre-check fields already selected on the prior render (if any) so
	// switching schemas doesn't erase selections the user made on the
	// same schema. Otherwise default to every field.
	var selected []string
	if raw := r.Form["field_key"]; len(raw) > 0 {
		valid := make(map[string]bool, len(picked.FieldsSpec))
		for _, f := range picked.FieldsSpec {
			valid[f.Name] = true
		}
		for _, f := range raw {
			if valid[f] {
				selected = append(selected, f)
			}
		}
	}
	if len(selected) == 0 {
		for _, f := range picked.FieldsSpec {
			selected = append(selected, f.Name)
		}
	}
	sess.CustomOID4VPSchemaID = schemaID
	// Build a preview template purely so the fragment can show a "will
	// request: …" line; Generate rebuilds from the live form values.
	preview := vctypes.OID4VPTemplate{
		Title:      picked.Name,
		Fields:     selected,
		Format:     picked.Std,
		Disclosure: disclosureSummary(r.FormValue("disclosure"), selected),
	}
	h.renderFragment(w, r, "fragment_custom_oid4vp_preview", map[string]any{
		"Template": preview,
		"SchemaID": schemaID,
		"Schema":   *picked,
		"Selected": fieldSet(selected),
	})
}

// fieldSet returns a lookup from field name → true so templates can
// render the right "checked" state without iterating the slice per field.
func fieldSet(xs []string) map[string]bool {
	out := make(map[string]bool, len(xs))
	for _, x := range xs {
		out[x] = true
	}
	return out
}

// disclosureSummary renders the plain-language string shown on the request
// preview — "selective — only X, Y are shared" or "full credential shared".
func disclosureSummary(mode string, fields []string) string {
	if mode == "selective" && len(fields) > 0 {
		return "selective — only " + strings.Join(fields, ", ") + " shared"
	}
	return "full credential shared"
}

// SimulateResponse fetches the (simulated) verification result for the current OID4VP session.
func (h *H) SimulateResponse(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.CurrentOID4VPState == "" {
		h.errorToast(w, r, "Generate a request first")
		return
	}
	res, err := h.Adapter.FetchPresentationResult(r.Context(), sess.CurrentOID4VPState, sess.CurrentOID4VPTemplate)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragment(w, r, "fragment_verify_result", res)
}

// VerifyDirect handles scan/upload/paste direct verification.
//
//   - paste: credential_data carries the raw VC string.
//   - scan:  credential_data carries the QR text the front-end decoded with
//     jsQR from the camera feed. Server does no additional decoding.
//   - upload: the form posts multipart/form-data with a PNG/JPG image of the
//     QR code. Server decodes it with gozxing, then proceeds exactly like
//     scan/paste.
func (h *H) VerifyDirect(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	// Large uploads are intentionally capped at 8 MB; real QR images fit well
	// under 1 MB but browsers sometimes attach arbitrary sidecars.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		_ = r.ParseForm() // fall back for non-multipart submissions
	}
	method := r.FormValue("method")
	credData := strings.TrimSpace(r.FormValue("credential_data"))

	if method == "upload" && credData == "" {
		decoded, err := decodeUploadedQR(r)
		if err != nil {
			h.errorToast(w, r, "Could not read QR from upload: "+err.Error())
			return
		}
		credData = decoded
	}
	if method == "paste" && credData == "" {
		h.errorToast(w, r, "Paste a credential first")
		return
	}
	if method == "scan" && credData == "" {
		h.errorToast(w, r, "Scanner did not return a credential payload")
		return
	}
	res, err := h.Adapter.VerifyDirect(r.Context(), backend.DirectVerifyRequest{
		VerifierDpg: sess.VerifierDpg, Method: method, CredentialData: credData,
	})
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragment(w, r, "fragment_verify_result", res)
}
