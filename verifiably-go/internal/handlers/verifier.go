package handlers

import (
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

// GenerateRequest creates an OID4VP presentation request from a preset
// template key OR, when the user picked "custom", from the template
// assembled via BuildVerifierTemplate.
func (h *H) GenerateRequest(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	template := r.FormValue("template")
	if template == "" {
		template = "age"
	}
	req := backend.PresentationRequest{
		VerifierDpg: sess.VerifierDpg,
		TemplateKey: template,
	}
	if template == "custom" {
		if sess.CustomOID4VPTemplate == nil {
			h.errorToast(w, r, "Build a custom request first")
			return
		}
		req.Template = sess.CustomOID4VPTemplate
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

// BuildVerifierTemplate composes a custom OID4VPTemplate from a schema +
// user-selected fields + disclosure mode. Stashes it on the session so
// the Generate Request step can pick it up. Returns an HTMX fragment that
// previews the composed template with edit-in-place behavior.
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
		h.errorToast(w, r, "Pick a schema first")
		return
	}
	disclosure := r.FormValue("disclosure")
	if disclosure == "" {
		disclosure = "full"
	}
	fields := r.Form["field_key"] // repeated form field — one per checked box

	// Resolve the schema so we can validate field names + grab the format.
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
	// If the caller ticked no boxes, default to every field the schema
	// declares — a custom "full credential" template is still a valid
	// thing to build.
	if len(fields) == 0 {
		for _, f := range picked.FieldsSpec {
			fields = append(fields, f.Name)
		}
	}
	// Validate the picked fields are actually part of the schema.
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

	tpl := vctypes.OID4VPTemplate{
		Title:      picked.Name,
		Fields:     fields,
		Format:     picked.Std,
		Disclosure: disclosureSummary(disclosure, fields),
	}
	sess.CustomOID4VPTemplate = &tpl
	sess.CustomOID4VPSchemaID = schemaID

	h.renderFragment(w, r, "fragment_custom_oid4vp_preview", map[string]any{
		"Template": tpl,
		"SchemaID": schemaID,
		"Schema":   *picked,
		"Selected": fieldSet(fields),
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
