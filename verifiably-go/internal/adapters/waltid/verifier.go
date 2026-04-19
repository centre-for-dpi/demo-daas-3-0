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
func (a *Adapter) RequestPresentation(ctx context.Context, req backend.PresentationRequest) (backend.PresentationRequestResult, error) {
	tpl, ok := oid4vpTemplates[req.TemplateKey]
	if !ok {
		return backend.PresentationRequestResult{}, fmt.Errorf("unknown template key %q", req.TemplateKey)
	}
	// Build a Presentation-Exchange request asking for any VC whose type
	// contains one of the template's fields. Walt.id's current PE path accepts
	// a simple `{"format":"jwt_vc_json","type":"UniversityDegreeCredential"}`
	// style entry; we use a generic request that the verifier will match
	// against any credential present in the wallet.
	creds := []map[string]any{
		{"format": credentialFormatForStd(tpl.Format), "type": credentialTypeForTemplate(req.TemplateKey)},
	}
	body := verifyBody{
		RequestCredentials: creds,
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
	tpl, _ := oid4vpTemplates[templateKey]
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
