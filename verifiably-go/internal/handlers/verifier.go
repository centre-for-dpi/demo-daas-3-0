package handlers

import (
	"net/http"
	"strings"

	"github.com/verifiably/verifiably-go/backend"
)

// ShowVerify renders the verifier page (OID4VP request generator + direct verify).
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
	body := map[string]any{
		"Templates":       templates,
		"VerifierDpgObj": dpgs[sess.VerifierDpg],
	}
	h.render(w, r, "verifier_verify", h.pageData(sess, body))
}

// GenerateRequest creates an OID4VP presentation request.
func (h *H) GenerateRequest(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	template := r.FormValue("template")
	if template == "" {
		template = "age"
	}
	res, err := h.Adapter.RequestPresentation(r.Context(), backend.PresentationRequest{
		VerifierDpg: sess.VerifierDpg, TemplateKey: template,
	})
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	sess.CurrentOID4VPLink = res.RequestURI
	sess.CurrentOID4VPState = res.State
	sess.CurrentOID4VPTemplate = template
	h.renderFragment(w, r, "fragment_oid4vp_request_output", res)
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
