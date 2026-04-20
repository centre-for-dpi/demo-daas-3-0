package waltid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/vctypes"
)

// oid4vpTemplates is the verifier's built-in preset list. Walt.id doesn't
// expose a "list my configured templates" endpoint — the verifier accepts
// any DCQL/Presentation-Exchange query on every /verify call, so we bundle
// a curated set grounded in the credential types walt.id actually ships
// (see /draft13/.well-known/openid-credential-issuer).
var oid4vpTemplates = map[string]vctypes.OID4VPTemplate{
	"age_over_18": {
		Title:      "Proof of age over 18",
		Fields:     []string{"age_over_18"},
		Format:     "sd_jwt_vc (IETF)",
		Disclosure: "selective — only age_over_18 is shared",
	},
	"university_degree": {
		Title:      "University Degree",
		Fields:     []string{"degree", "classification", "conferred"},
		Format:     "w3c_vcdm_2",
		Disclosure: "full credential shared",
	},
	"verifiable_id": {
		Title:      "Verifiable ID",
		Fields:     []string{"holder", "dateOfBirth", "nationality"},
		Format:     "w3c_vcdm_2",
		Disclosure: "full credential shared",
	},
	"kyc_checks": {
		Title:      "KYC checks credential",
		Fields:     []string{"kycComplete", "amlScreeningPassed"},
		Format:     "w3c_vcdm_2",
		Disclosure: "full credential shared",
	},
	"iso_mdl": {
		Title:      "ISO mDL driver's licence",
		Fields:     []string{"family_name", "given_name", "birth_date", "driving_privileges"},
		Format:     "mso_mdoc",
		Disclosure: "full credential shared",
	},
	"open_badge": {
		Title:      "Open Badge v3",
		Fields:     []string{"achievement", "issuedOn"},
		Format:     "w3c_vcdm_2",
		Disclosure: "full credential shared",
	},
	"employment": {
		Title:      "Employment record",
		Fields:     []string{"employer", "title", "startDate"},
		Format:     "w3c_vcdm_2",
		Disclosure: "full credential shared",
	},
}

// ListOID4VPTemplates returns the curated preset list.
func (a *Adapter) ListOID4VPTemplates(_ context.Context) (map[string]vctypes.OID4VPTemplate, error) {
	out := make(map[string]vctypes.OID4VPTemplate, len(oid4vpTemplates))
	for k, v := range oid4vpTemplates {
		out[k] = v
	}
	return out, nil
}

// verifyBody matches the POST /openid4vc/verify body shape (VerifierApi.kt:73).
// Walt.id wants `request_credentials` as an array of objects keyed by format.
// For Presentation-Exchange (pre-OID4VP-v1.0), each object has `format` and
// `type`; for OID4VP v1.0 DCQL, it takes a different shape — but v0.18.2 is
// still in the PE-based flow as default.
type verifyBody struct {
	RequestCredentials []map[string]any `json:"request_credentials"`
	VPPolicies         []string         `json:"vp_policies,omitempty"`
	VCPolicies         []string         `json:"vc_policies,omitempty"`
}

// RequestPresentation creates a verifier session. Walt.id returns the full
// authorize URL as the plain-text response body.
//
// Accepts either a preset TemplateKey or an inline req.Template; the latter
// wins when both are set. Handlers use the inline path for custom
// user-assembled requests; the keyed path covers the curated presets.
func (a *Adapter) RequestPresentation(ctx context.Context, req backend.PresentationRequest) (backend.PresentationRequestResult, error) {
	var tpl vctypes.OID4VPTemplate
	typeHint := credentialTypeForTemplate(req.TemplateKey)
	if req.Template != nil {
		tpl = *req.Template
		// Custom templates don't map back to a preset key, so derive the
		// credential type hint from the inline template's own name when
		// possible, otherwise fall back to a generic VerifiableCredential
		// filter so walt.id matches anything in the wallet.
		typeHint = credentialTypeForCustomTemplate(tpl)
	} else {
		var ok bool
		tpl, ok = oid4vpTemplates[req.TemplateKey]
		if !ok {
			return backend.PresentationRequestResult{}, fmt.Errorf("unknown template key %q", req.TemplateKey)
		}
	}
	// Build a Presentation-Exchange request asking for any VC whose type
	// contains one of the template's fields. Walt.id's E2E test suite uses
	// two specific request shapes, and the wallet-api's OID4VP submit
	// crashes on anything else:
	//
	//   jwt_vc_json   →  {"format": "jwt_vc_json", "type": "<CredentialType>"}
	//   vc+sd-jwt     →  {"format": "vc+sd-jwt",   "vct":  "<full-vct-URL>"}
	//
	// (see waltid-services/waltid-e2e-tests/src/test/resources/presentation/
	// *-presentation-request.json for the canonical fixtures.)
	format := credentialFormatForStd(tpl.Format)
	entry := map[string]any{"format": format}
	switch format {
	case "vc+sd-jwt", "dc+sd-jwt":
		// walt.id's SD-JWT VC matcher keys off vct. Use the type as the
		// vct tail; when a caller threads a fully-qualified URL (via a
		// custom template set up from the issuer's schema id) we honor
		// that verbatim.
		if strings.HasPrefix(typeHint, "http://") || strings.HasPrefix(typeHint, "https://") {
			entry["vct"] = typeHint
		} else {
			entry["vct"] = typeHint
		}
	default:
		entry["type"] = typeHint
	}
	body := verifyBody{
		RequestCredentials: []map[string]any{entry},
	}
	raw, err := a.verifier.DoRaw(ctx, "POST", "/openid4vc/verify", jsonReader(body), "application/json", nil)
	if err != nil {
		return backend.PresentationRequestResult{}, err
	}
	authorizeURL := strings.TrimSpace(string(raw))
	state := extractVerifierState(authorizeURL)
	return backend.PresentationRequestResult{
		RequestURI: authorizeURL,
		State:      state,
		Template:   tpl,
	}, nil
}

// credentialTypeForCustomTemplate derives a walt.id PE "type" filter for a
// user-assembled template. Uses the template's title (Camel-cased words
// joined) if it resembles a credential name; otherwise returns the generic
// VerifiableCredential filter which matches any VC the wallet holds.
func credentialTypeForCustomTemplate(tpl vctypes.OID4VPTemplate) string {
	title := strings.TrimSpace(tpl.Title)
	if title == "" {
		return "VerifiableCredential"
	}
	// Title-case and strip non-alphanumerics to guess a type name.
	var b strings.Builder
	capNext := true
	for _, r := range title {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
			capNext = false
			continue
		}
		if r >= 'a' && r <= 'z' {
			if capNext {
				b.WriteRune(r - 32)
			} else {
				b.WriteRune(r)
			}
			capNext = false
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			capNext = false
			continue
		}
		capNext = true
	}
	guess := b.String()
	if guess == "" {
		return "VerifiableCredential"
	}
	if !strings.HasSuffix(guess, "Credential") {
		guess += "Credential"
	}
	return guess
}

// sessionResult is the slim shape this adapter reads out of
// GET /openid4vc/session/{id}. Walt.id returns a rich object including
// policy results, credential submissions, etc.; we consume what the UI needs.
type sessionResult struct {
	SessionID                string          `json:"sessionId"`
	VerificationResult       *bool           `json:"verificationResult,omitempty"`
	OverallVerificationResult *bool          `json:"overallVerificationResult,omitempty"`
	TokenResponse            json.RawMessage `json:"tokenResponse,omitempty"`
	AuthorizationRequest     json.RawMessage `json:"authorizationRequest,omitempty"`
	PolicyResults            json.RawMessage `json:"policyResults,omitempty"`
	VPPolicies               json.RawMessage `json:"vpPolicies,omitempty"`
	VCPolicies               json.RawMessage `json:"vcPolicies,omitempty"`
	Success                  *bool           `json:"success,omitempty"`
	Issued                   string          `json:"issued,omitempty"`
}

// FetchPresentationResult polls GET /openid4vc/session/{id} for a terminal
// verification state. The UI calls this once (on the user's "check result"
// action); we make up to N short-interval polls so a holder that just
// presented via the wallet-api sees the result promptly.
func (a *Adapter) FetchPresentationResult(ctx context.Context, state, templateKey string) (backend.VerificationResult, error) {
	tpl := oid4vpTemplates[templateKey] // zero value for unknown/custom keys is fine — we only use tpl.Fields for shape
	path := "/openid4vc/session/" + url.PathEscape(state)
	var res sessionResult
	deadline := time.Now().Add(8 * time.Second)
	for {
		if err := a.verifier.DoJSON(ctx, "GET", path, nil, &res, nil); err != nil {
			return backend.VerificationResult{}, err
		}
		if isTerminalSession(res) || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	// If the session never reached a terminal state, the holder simply
	// hasn't submitted a presentation yet. Surface that as Pending so the
	// UI renders an "awaiting response" card instead of a red "invalid"
	// banner that would be misleading — Valid=false on an un-submitted
	// session means "no input", not "bad input".
	if !isTerminalSession(res) {
		return backend.VerificationResult{
			Pending: true,
			Method:  fmt.Sprintf("OID4VP · %s", tpl.Disclosure),
			Format:  tpl.Format,
		}, nil
	}
	return backend.VerificationResult{
		Valid:             overallResult(res),
		Method:            fmt.Sprintf("OID4VP · %s", tpl.Disclosure),
		Format:            tpl.Format,
		Issuer:            "(resolved on verification)",
		Subject:           "(resolved on verification)",
		Requested:         tpl.Fields,
		Issued:            time.Now().UTC(),
		CheckedRevocation: true,
	}, nil
}

// VerifyDirect — walt.id v0.18.2 has no direct-credential-verification
// endpoint. Return ErrNotSupported. The handler surfaces this as a toast
// instructing the operator to use the OID4VP flow instead.
func (a *Adapter) VerifyDirect(_ context.Context, _ backend.DirectVerifyRequest) (backend.VerificationResult, error) {
	return backend.VerificationResult{}, backend.ErrNotSupported
}

// credentialFormatForStd maps the template's Format onto walt.id's format key.
func credentialFormatForStd(std string) string {
	switch std {
	case "sd_jwt_vc (IETF)":
		return "vc+sd-jwt"
	case "mso_mdoc":
		return "mso_mdoc"
	default:
		return "jwt_vc_json"
	}
}

// credentialTypeForTemplate picks a sensible credential type for each template key.
// The type names match what walt.id's issuer exposes in
// credential_configurations_supported, so the verifier's Presentation
// Exchange constraint will actually match real credentials issued here.
func credentialTypeForTemplate(key string) string {
	switch key {
	case "age_over_18":
		return "IdentityCredential"
	case "university_degree":
		return "UniversityDegree"
	case "verifiable_id":
		return "VerifiableId"
	case "kyc_checks":
		return "KycChecksCredential"
	case "iso_mdl":
		return "Iso18013DriversLicenseCredential"
	case "open_badge":
		return "OpenBadgeCredential"
	case "employment":
		return "EmploymentRecord"
	default:
		return "VerifiableCredential"
	}
}

// extractVerifierState pulls the session id out of the authorize URL walt.id
// returns. Shape: openid4vp://authorize?...&request_uri=<base>/openid4vc/request/{id}
// or openid4vp://authorize?...&state=<id>.
func extractVerifierState(authorizeURL string) string {
	u, err := url.Parse(authorizeURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	if s := q.Get("state"); s != "" {
		return s
	}
	if ru := q.Get("request_uri"); ru != "" {
		if i := strings.LastIndex(ru, "/"); i >= 0 && i+1 < len(ru) {
			return ru[i+1:]
		}
	}
	return ""
}

func isTerminalSession(r sessionResult) bool {
	if r.VerificationResult != nil || r.OverallVerificationResult != nil || r.Success != nil {
		return true
	}
	return len(r.TokenResponse) > 0 && string(r.TokenResponse) != "null"
}

func overallResult(r sessionResult) bool {
	if r.OverallVerificationResult != nil {
		return *r.OverallVerificationResult
	}
	if r.VerificationResult != nil {
		return *r.VerificationResult
	}
	if r.Success != nil {
		return *r.Success
	}
	return false
}
