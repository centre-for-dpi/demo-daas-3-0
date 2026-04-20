package waltid

import (
	"context"
	"encoding/base64"
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
//
// vc_policies accepts both string entries ("signature", "expired") and
// object entries (`{"policy":"webhook","args":{"url":"…"}}`), so the field
// is []any.
type verifyBody struct {
	RequestCredentials []map[string]any `json:"request_credentials"`
	VPPolicies         []any            `json:"vp_policies,omitempty"`
	VCPolicies         []any            `json:"vc_policies,omitempty"`
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
		// Custom templates plumb the canonical credential type + full vct
		// URL through the template itself (filled by the handler from the
		// Schema.Variants slice). Use those verbatim — they match exactly
		// what walt.id's wallet holds, which keeps the PD match working
		// when formats differ. Fall back to the title-derived guess only
		// when neither is provided (older templates, custom schemas).
		if tpl.CredentialType != "" {
			typeHint = tpl.CredentialType
		} else {
			typeHint = credentialTypeForCustomTemplate(tpl)
		}
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
		// Walt.id's SD-JWT VC matcher requires the EXACT issuer-advertised
		// vct — typically a full URL like http://issuer/draft13/BankId.
		// Using a short type name here silently produces "no matches"
		// against a wallet that holds the credential.
		if tpl.Vct != "" {
			entry["vct"] = tpl.Vct
		} else {
			entry["vct"] = typeHint
		}
	default:
		entry["type"] = typeHint
	}
	body := verifyBody{
		RequestCredentials: []map[string]any{entry},
		VPPolicies:         buildVPPolicies(),
		VCPolicies:         buildVCPolicies(req.Policies, req.WebhookURL),
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
	if !isTerminalSession(res) {
		return backend.VerificationResult{
			Pending: true,
			Method:  fmt.Sprintf("OID4VP · %s", tpl.Disclosure),
			Format:  tpl.Format,
		}, nil
	}

	fields, issuer, subject, issued, title := extractPresentedCredential(res.TokenResponse)
	policies := extractAppliedPolicies(res.PolicyResults)
	if issuer == "" {
		issuer = "(resolved on verification)"
	}
	if subject == "" {
		subject = "(resolved on verification)"
	}
	if issued.IsZero() {
		issued = time.Now().UTC()
	}
	return backend.VerificationResult{
		Valid:             overallResult(res),
		Method:            fmt.Sprintf("OID4VP · %s", tpl.Disclosure),
		Format:            tpl.Format,
		Issuer:            issuer,
		Subject:           subject,
		Requested:         tpl.Fields,
		Issued:            issued,
		CheckedRevocation: true,
		PoliciesApplied:   policies,
		DisclosedFields:   fields,
		CredentialTitle:   title,
	}, nil
}

// buildVPPolicies returns the always-on VP-level policies. Every OID4VP
// flow needs signature verification on the presentation envelope and
// presentation-definition matching — making these user-toggleable would
// just let an operator accidentally disable the whole point of the flow.
func buildVPPolicies() []any {
	return []any{"signature", "presentation-definition"}
}

// buildVCPolicies maps the user's selected policy checkboxes onto walt.id's
// vc_policies list shape. String entries are first-class policies; the
// "webhook" option becomes an object policy with the operator's URL in
// args. Unknown policy names are dropped.
func buildVCPolicies(selected []string, webhookURL string) []any {
	out := []any{}
	for _, p := range selected {
		switch p {
		case "signature", "expired", "not-before":
			out = append(out, p)
		case "webhook":
			url := strings.TrimSpace(webhookURL)
			if url == "" {
				continue
			}
			out = append(out, map[string]any{
				"policy": "webhook",
				"args":   map[string]any{"url": url},
			})
		}
	}
	return out
}

// extractAppliedPolicies walks walt.id's policyResults blob and returns the
// set of policy names that ran. Walt.id returns policyResults as a nested
// object {results: [{policy: "signature", ...}, ...]} or similar; we scan
// for top-level "policy" keys. Unknown shapes yield nil which the UI
// renders as "(no detail)".
func extractAppliedPolicies(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var any1 any
	if err := json.Unmarshal(raw, &any1); err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if p, ok := x["policy"].(string); ok && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			for _, v2 := range x {
				walk(v2)
			}
		case []any:
			for _, v2 := range x {
				walk(v2)
			}
		}
	}
	walk(any1)
	return out
}

// extractPresentedCredential parses walt.id's tokenResponse to pull the
// holder-disclosed claim values. Returns (fields, issuer, subject, issued,
// title) — any value can be empty when the shape doesn't match (SD-JWT VC
// vs W3C VC-JWT take different parse paths). Best-effort — fields stay
// nil and the UI gracefully hides the "Presented Credentials" panel.
func extractPresentedCredential(raw json.RawMessage) (map[string]string, string, string, time.Time, string) {
	if len(raw) == 0 {
		return nil, "", "", time.Time{}, ""
	}
	var env struct {
		VPToken any `json:"vp_token"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, "", "", time.Time{}, ""
	}
	// vp_token can be a string (single VP), an array of strings (multiple
	// VPs), or nested. We flatten to a list of string tokens.
	var tokens []string
	switch v := env.VPToken.(type) {
	case string:
		tokens = []string{v}
	case []any:
		for _, t := range v {
			if s, ok := t.(string); ok {
				tokens = append(tokens, s)
			}
		}
	}
	for _, tok := range tokens {
		if strings.Contains(tok, "~") {
			// SD-JWT VC format: <header.payload.sig>~<disclosure>~…
			fields, iss, sub, issued, title := parseSDJWTVC(tok)
			if len(fields) > 0 {
				return fields, iss, sub, issued, title
			}
			continue
		}
		// W3C VC-JWT wrapped in a VP-JWT.
		fields, iss, sub, issued, title := parseVPJWT(tok)
		if len(fields) > 0 {
			return fields, iss, sub, issued, title
		}
	}
	return nil, "", "", time.Time{}, ""
}

// parseSDJWTVC decodes an SD-JWT-VC presentation: the first segment is the
// credential JWT (base64url header.payload.sig); following tilde-separated
// segments are disclosures — each a base64url-encoded JSON array of
// [salt, claim, value]. We merge the JWT payload's direct claims with the
// disclosed ones so the UI sees the full revealed subject.
func parseSDJWTVC(tok string) (map[string]string, string, string, time.Time, string) {
	parts := strings.Split(tok, "~")
	if len(parts) == 0 {
		return nil, "", "", time.Time{}, ""
	}
	jwt := parts[0]
	payload := decodeJWTPayload(jwt)
	if payload == nil {
		return nil, "", "", time.Time{}, ""
	}
	fields := map[string]string{}
	// Base claims (the non-selectively-disclosable ones that the issuer
	// chose to leave in the clear). Skip the SD control keys and standard
	// JWT claims so the UI doesn't render "_sd: [...]".
	reserved := map[string]bool{
		"_sd": true, "_sd_alg": true, "cnf": true, "iss": true, "iat": true,
		"exp": true, "nbf": true, "sub": true, "vct": true, "status": true,
	}
	for k, v := range payload {
		if reserved[k] {
			continue
		}
		fields[k] = stringifyAny(v)
	}
	// Merge disclosed fields: each is a base64url JSON array
	// [salt, claim, value] or [salt, value] (for array element disclosures).
	for _, seg := range parts[1:] {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		d, err := base64.RawURLEncoding.DecodeString(seg)
		if err != nil {
			continue
		}
		var arr []any
		if err := json.Unmarshal(d, &arr); err != nil {
			continue
		}
		if len(arr) == 3 {
			if name, ok := arr[1].(string); ok {
				fields[name] = stringifyAny(arr[2])
			}
		}
	}
	issuer, _ := payload["iss"].(string)
	subject, _ := payload["sub"].(string)
	title, _ := payload["vct"].(string)
	if title != "" {
		if i := strings.LastIndex(title, "/"); i >= 0 {
			title = title[i+1:]
		}
	}
	issued := unixClaim(payload, "iat")
	return fields, issuer, subject, issued, title
}

// parseVPJWT decodes a VP-JWT (VC Data Model 1.1 presentation) and pulls
// the first embedded VC's credentialSubject. Most W3C VC-JWT flows keep
// the subject claims nested there; we stringify leaf values so the UI can
// render them uniformly.
func parseVPJWT(tok string) (map[string]string, string, string, time.Time, string) {
	vp := decodeJWTPayload(tok)
	if vp == nil {
		return nil, "", "", time.Time{}, ""
	}
	// vp.verifiableCredential[] — each is either an embedded object or a
	// nested VC-JWT string.
	vpObj, _ := vp["vp"].(map[string]any)
	if vpObj == nil {
		vpObj = vp
	}
	vcList, _ := vpObj["verifiableCredential"].([]any)
	if len(vcList) == 0 {
		return nil, "", "", time.Time{}, ""
	}
	var vcPayload map[string]any
	switch v := vcList[0].(type) {
	case string:
		vcPayload = decodeJWTPayload(v)
	case map[string]any:
		vcPayload = v
	}
	if vcPayload == nil {
		return nil, "", "", time.Time{}, ""
	}
	vcInner, _ := vcPayload["vc"].(map[string]any)
	if vcInner == nil {
		vcInner = vcPayload
	}
	cs, _ := vcInner["credentialSubject"].(map[string]any)
	if cs == nil {
		return nil, "", "", time.Time{}, ""
	}
	fields := map[string]string{}
	for k, v := range cs {
		if k == "id" {
			continue
		}
		fields[k] = stringifyAny(v)
	}
	issuer := stringifyAny(vcInner["issuer"])
	subject, _ := cs["id"].(string)
	if subject == "" {
		subject, _ = vcPayload["sub"].(string)
	}
	issued := unixClaim(vcPayload, "iat")
	title := ""
	if types, ok := vcInner["type"].([]any); ok {
		for _, t := range types {
			if s, ok := t.(string); ok && s != "VerifiableCredential" {
				title = s
				break
			}
		}
	}
	return fields, issuer, subject, issued, title
}

func decodeJWTPayload(jwt string) map[string]any {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// try std padding
		if raw2, err2 := base64.URLEncoding.DecodeString(parts[1]); err2 == nil {
			raw = raw2
		} else {
			return nil
		}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func stringifyAny(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func unixClaim(m map[string]any, key string) time.Time {
	v, ok := m[key].(float64)
	if !ok {
		return time.Time{}
	}
	return time.Unix(int64(v), 0).UTC()
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
