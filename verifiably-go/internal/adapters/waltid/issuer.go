package waltid

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/vctypes"
)

// onboardingRequest matches /onboard/issuer body shape (walt.id v0.18.2).
type onboardingRequest struct {
	Key onboardingKey `json:"key"`
	DID onboardingDID `json:"did"`
}

type onboardingKey struct {
	Backend string `json:"backend"`
	KeyType string `json:"keyType"`
}

type onboardingDID struct {
	Method string `json:"method"`
}

type onboardingResponse struct {
	IssuerKey json.RawMessage `json:"issuerKey"`
	IssuerDID string          `json:"issuerDid"`
}

// ensureIssuerKey ensures the adapter has an issuer key + DID, onboarding a
// fresh one on the first call if config didn't pin them.
func (a *Adapter) ensureIssuerKey(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.issuerKey) > 0 && a.issuerDID != "" {
		return nil
	}
	body := onboardingRequest{
		Key: onboardingKey{Backend: "jwk", KeyType: "secp256r1"},
		DID: onboardingDID{Method: "jwk"},
	}
	var resp onboardingResponse
	if err := a.issuer.DoJSON(ctx, "POST", "/onboard/issuer", body, &resp, nil); err != nil {
		return fmt.Errorf("onboard issuer: %w", err)
	}
	a.issuerKey = resp.IssuerKey
	a.issuerDID = resp.IssuerDID
	return nil
}

// credentialIssuerMetadata is a slim view of /draft13/.well-known/openid-credential-issuer
// — only the fields this adapter reads.
type credentialIssuerMetadata struct {
	CredentialIssuer                  string                                    `json:"credential_issuer"`
	CredentialConfigurationsSupported map[string]credentialConfigurationEntry   `json:"credential_configurations_supported"`
	Display                           []map[string]json.RawMessage              `json:"display,omitempty"`
}

type credentialConfigurationEntry struct {
	Format               string                       `json:"format"`
	Scope                string                       `json:"scope,omitempty"`
	CredentialDefinition *credentialDefinitionEntry   `json:"credential_definition,omitempty"`
	Vct                  string                       `json:"vct,omitempty"`
	DocType              string                       `json:"doctype,omitempty"`
	// Display is the per-configuration human-readable label walt.id
	// advertises (one entry per locale). displayNameFor prefers display[0].name
	// when present because it's the cleanest label.
	Display []struct {
		Name   string `json:"name"`
		Locale string `json:"locale,omitempty"`
	} `json:"display,omitempty"`
}

type credentialDefinitionEntry struct {
	Type []string `json:"type"`
}

// ListSchemas fetches the issuer's credential configurations and maps each one
// onto a vctypes.Schema. Schema ID is the configuration id (e.g.
// "UniversityDegree_jwt_vc_json"); Std is derived from the format.
//
// Walt.id exposes the same credential TYPE under multiple configuration ids —
// one per format (jwt_vc_json, jwt_vc_json-ld, ldp_vc, vc+sd-jwt, dc+sd-jwt,
// mso_mdoc) and multiple map to the same Std in our taxonomy. If we surfaced
// all of them each type appears 3-6 times and, worse, the dedup-by-name
// approach would nondeterministically pick a format walt.id's OID4VP
// verify/present pipeline doesn't exercise end-to-end.
//
// Walt.id's own E2E test suite (waltid-e2e-tests) ships two canonical pairs:
//
//   - jwt_vc_json   ↔ OpenBadgeCredential_jwt_vc_json
//   - vc+sd-jwt     ↔ identity_credential_vc+sd-jwt
//
// We follow that precedent: for each (Name, Std) tuple we prefer the format
// walt.id has tested (see `preferredFormatForStd`), dropping the others.
// Result: schema picker shows one card per walt.id-blessed combo, and the
// verifier's OID4VP request asks for the exact format the holder has.
func (a *Adapter) ListSchemas(ctx context.Context, issuerDpg string) ([]vctypes.Schema, error) {
	var meta credentialIssuerMetadata
	path := fmt.Sprintf("/%s/.well-known/openid-credential-issuer", a.cfg.StandardVersion)
	if err := a.issuer.DoJSON(ctx, "GET", path, nil, &meta, nil); err != nil {
		return nil, fmt.Errorf("fetch issuer metadata: %w", err)
	}
	// Bucket all entries per (Name, Std) so we can rank-pick within a bucket.
	type entry struct {
		id  string
		cfg credentialConfigurationEntry
	}
	buckets := map[string][]entry{}
	order := []string{} // preserves first-seen order so output is stable
	for id, cfg := range meta.CredentialConfigurationsSupported {
		std := formatToStd(cfg.Format)
		if std == "" {
			continue // skip VP-only configurations
		}
		name := displayNameFor(id, cfg)
		key := name + "\x00" + std
		if _, ok := buckets[key]; !ok {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], entry{id, cfg})
	}
	out := make([]vctypes.Schema, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		pick := bucket[0]
		for _, e := range bucket[1:] {
			if formatRank(e.cfg.Format) > formatRank(pick.cfg.Format) {
				pick = e
			}
		}
		name := displayNameFor(pick.id, pick.cfg)
		out = append(out, vctypes.Schema{
			ID:         pick.id,
			Name:       name,
			Std:        formatToStd(pick.cfg.Format),
			DPGs:       []string{issuerDpg},
			Desc:       fmt.Sprintf("Live credential configuration served by %s.", issuerDpg),
			FieldsSpec: fieldsForCredentialType(pick.id),
		})
	}
	return out, nil
}

// formatRank returns a score for a walt.id format; higher wins the
// dedup-pick. The ranking mirrors the combinations walt.id's own
// waltid-e2e-tests suite exercises end-to-end, so the formats we surface
// are the ones with a tested issue → claim → present → verify path.
func formatRank(f string) int {
	switch f {
	case "jwt_vc_json":
		return 100 // walt.id's canonical jwt test (OpenBadgeCredential_jwt_vc_json)
	case "vc+sd-jwt":
		return 90 // walt.id's canonical SD-JWT test (identity_credential_vc+sd-jwt)
	case "dc+sd-jwt":
		return 85 // newer SD-JWT variant walt.id is moving toward
	case "mso_mdoc":
		return 80 // mdoc is its own Std so doesn't actually compete
	case "jwt_vc_json-ld":
		return 30
	case "ldp_vc":
		return 20
	case "jwt_vc":
		return 10
	}
	return 0
}

// ListAllSchemas delegates to ListSchemas — the registry handles aggregation
// across DPGs, so per-adapter "all" is just "mine".
func (a *Adapter) ListAllSchemas(ctx context.Context) ([]vctypes.Schema, error) {
	return a.ListSchemas(ctx, a.Vendor)
}

// SaveCustomSchema is a no-op at the adapter level — registry owns custom
// schemas as a cross-vendor store.
func (a *Adapter) SaveCustomSchema(_ context.Context, _ vctypes.Schema) error {
	return nil
}

// DeleteCustomSchema is a no-op for the same reason.
func (a *Adapter) DeleteCustomSchema(_ context.Context, _ string) error {
	return nil
}

// PrefillSubjectFields returns empty: walt.id doesn't carry an identity plugin
// like MOSIP, so the operator fills the form. This is an honest answer — the
// UI's "Manual entry" source is the intended input mode.
func (a *Adapter) PrefillSubjectFields(_ context.Context, _ vctypes.Schema) (map[string]string, error) {
	return map[string]string{}, nil
}

// issuanceRequest mirrors IssuanceRequest (waltid-issuer-api v0.18.2,
// IssuanceRequests.kt:79). Only the fields this adapter sets are included;
// walt.id ignores unknown fields.
type issuanceRequest struct {
	IssuerKey                 json.RawMessage `json:"issuerKey"`
	CredentialConfigurationId string          `json:"credentialConfigurationId"`
	CredentialData            json.RawMessage `json:"credentialData,omitempty"`
	Vct                       string          `json:"vct,omitempty"`
	MdocData                  json.RawMessage `json:"mdocData,omitempty"`
	AuthenticationMethod      string          `json:"authenticationMethod,omitempty"`
	IssuerDid                 string          `json:"issuerDid,omitempty"`
	StandardVersion           string          `json:"standardVersion,omitempty"`
}

// IssueToWallet issues a credential to the holder via OID4VCI. Walt.id returns
// the offer URI as a plain-text response body.
func (a *Adapter) IssueToWallet(ctx context.Context, req backend.IssueRequest) (backend.IssueToWalletResult, error) {
	if err := a.ensureIssuerKey(ctx); err != nil {
		return backend.IssueToWalletResult{}, err
	}
	path, err := issuePathFor(req.Schema.Std)
	if err != nil {
		return backend.IssueToWalletResult{}, err
	}

	credentialData, err := buildCredentialData(req.Schema, req.SubjectData)
	if err != nil {
		return backend.IssueToWalletResult{}, err
	}
	ir := issuanceRequest{
		IssuerKey:                 a.issuerKey,
		CredentialConfigurationId: req.Schema.ID,
		IssuerDid:                 a.issuerDID,
		AuthenticationMethod:      authenticationMethod(req.Flow),
		StandardVersion:           strings.ToUpper(a.cfg.StandardVersion),
	}
	switch req.Schema.Std {
	case "mso_mdoc":
		ir.MdocData = credentialData
	case "sd_jwt_vc (IETF)":
		ir.Vct = req.Schema.ID
		ir.CredentialData = credentialData
	default:
		ir.CredentialData = credentialData
	}

	raw, err := a.issuer.DoRaw(ctx, "POST", path, jsonReader(ir), "application/json", nil)
	if err != nil {
		return backend.IssueToWalletResult{}, err
	}
	return backend.IssueToWalletResult{
		OfferURI: strings.TrimSpace(string(raw)),
		OfferID:  req.Schema.ID + "-" + req.IssuerDpg,
		Flow:     req.Flow,
	}, nil
}

// IssueAsPDF — walt.id Community Stack v0.18.2 has no documented QR-on-PDF
// export path. Return ErrNotSupported; the UI disables PDF destination via
// DPG.DirectPDF=false.
func (a *Adapter) IssueAsPDF(_ context.Context, _ backend.IssueRequest) (backend.IssueAsPDFResult, error) {
	return backend.IssueAsPDFResult{}, backend.ErrNotSupported
}

// IssueBulk iterates Rows and calls IssueToWallet per row.
func (a *Adapter) IssueBulk(ctx context.Context, req backend.IssueBulkRequest) (backend.IssueBulkResult, error) {
	if len(req.Rows) == 0 {
		return backend.IssueBulkResult{}, fmt.Errorf("waltid: no rows supplied")
	}
	accepted := 0
	rejected := 0
	var errs []backend.BulkError
	for i, row := range req.Rows {
		_, err := a.IssueToWallet(ctx, backend.IssueRequest{
			IssuerDpg:   req.IssuerDpg,
			Schema:      req.Schema,
			SubjectData: row,
			Flow:        "pre_auth",
		})
		if err != nil {
			rejected++
			errs = append(errs, backend.BulkError{Row: i + 1, Reason: truncate(err.Error(), 140)})
			continue
		}
		accepted++
	}
	return backend.IssueBulkResult{Accepted: accepted, Rejected: rejected, Errors: errs}, nil
}

// BootstrapOffers issues a single canned credential against whatever schema is
// declared first on the issuer so the Wallet "paste example" helper has a real
// URI to cycle through. If issuance fails, returns an empty slice + nil error
// so startup doesn't block.
func (a *Adapter) BootstrapOffers(ctx context.Context) ([]string, error) {
	schemas, err := a.ListSchemas(ctx, a.Vendor)
	if err != nil || len(schemas) == 0 {
		return nil, nil
	}
	// Prefer UniversityDegree for consistency, else first in list.
	pick := schemas[0]
	for _, s := range schemas {
		if strings.HasPrefix(s.ID, "UniversityDegree") {
			pick = s
			break
		}
	}
	res, err := a.IssueToWallet(ctx, backend.IssueRequest{
		IssuerDpg: a.Vendor,
		Schema:    pick,
		SubjectData: map[string]string{
			"holder": "Demo Holder",
		},
		Flow: "pre_auth",
	})
	if err != nil {
		return nil, nil
	}
	return []string{res.OfferURI}, nil
}

// --- helpers ---

// formatToStd maps walt.id's credential-format keys to vctypes.Schema.Std.
// Returns "" for VP-only entries (which aren't issuance schemas).
func formatToStd(format string) string {
	switch format {
	// All W3C VC encodings (JSON, JSON-LD, LDP, and the legacy opaque
	// JWT wrap) surface under a single Std so the dedup collapses them
	// down to one card per credential type.
	case "jwt_vc_json", "jwt_vc_json-ld", "ldp_vc", "jwt_vc":
		return "w3c_vcdm_2"
	case "vc+sd-jwt", "dc+sd-jwt":
		return "sd_jwt_vc (IETF)"
	case "mso_mdoc":
		return "mso_mdoc"
	default:
		return ""
	}
}

// issuePathFor returns the /openid4vc/{format}/issue endpoint for a standard.
// walt.id routes jwt/sdjwt/mdoc into distinct paths in v0.18.2.
func issuePathFor(std string) (string, error) {
	switch std {
	case "w3c_vcdm_1", "w3c_vcdm_2", "jwt_vc":
		return "/openid4vc/jwt/issue", nil
	case "sd_jwt_vc (IETF)":
		return "/openid4vc/sdjwt/issue", nil
	case "mso_mdoc":
		return "/openid4vc/mdoc/issue", nil
	default:
		return "", fmt.Errorf("waltid: unsupported schema standard %q", std)
	}
}

// authenticationMethod maps the UI's flow choice onto walt.id's enum.
func authenticationMethod(flow string) string {
	switch flow {
	case "auth_code", "authorization_code":
		return "NONE"
	case "pre_auth", "pre_authorized_code", "":
		return "PRE_AUTHORIZED"
	default:
		return strings.ToUpper(flow)
	}
}

// buildCredentialData constructs a VCDM 2.0-shaped JSON object from the
// operator's subject input. Types come from the schema id prefix
// (the canonical type before the `_format` suffix).
func buildCredentialData(schema vctypes.Schema, subject map[string]string) (json.RawMessage, error) {
	baseType := strings.SplitN(schema.ID, "_", 2)[0]
	credSubject := make(map[string]any, len(subject))
	for k, v := range subject {
		credSubject[k] = v
	}
	doc := map[string]any{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://www.w3.org/ns/credentials/examples/v1",
		},
		"type":              []string{"VerifiableCredential", baseType},
		"credentialSubject": credSubject,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// fieldsForCredentialType returns a curated FieldSpec list for the walt.id
// credential configurations we know about. Walt.id's issuer metadata doesn't
// expose per-claim types, so hand-rolling the list is the only way to get
// meaningful input types (date, number, etc.) in the UI. Unknown ids get a
// minimal {holder} fallback.
func fieldsForCredentialType(id string) []vctypes.FieldSpec {
	base := strings.SplitN(id, "_", 2)[0]
	str := func(name string) vctypes.FieldSpec {
		return vctypes.FieldSpec{Name: name, Datatype: "string", Required: true}
	}
	date := func(name string) vctypes.FieldSpec {
		return vctypes.FieldSpec{Name: name, Datatype: "string", Format: "date", Required: true}
	}
	switch base {
	case "UniversityDegree", "UniversityDegreeCredential":
		return []vctypes.FieldSpec{str("holder"), str("degree"), str("classification"), date("conferred")}
	case "VerifiableId", "VerifiableID", "NaturalPersonVerifiableID":
		return []vctypes.FieldSpec{str("holder"), date("dateOfBirth"), str("nationality"), str("placeOfBirth")}
	case "KycChecksCredential", "KycCredential", "KycDataCredential":
		return []vctypes.FieldSpec{str("holder"), str("kycComplete"), str("amlScreeningPassed"), date("checkedOn")}
	case "Iso18013DriversLicenseCredential":
		return []vctypes.FieldSpec{
			str("family_name"), str("given_name"), date("birth_date"),
			str("document_number"), str("driving_privileges"), date("expiry_date"),
		}
	case "OpenBadgeCredential":
		return []vctypes.FieldSpec{str("holder"), str("achievement"), date("issuedOn")}
	case "BankId":
		return []vctypes.FieldSpec{str("holder"), str("accountNumber"), str("institution")}
	case "VaccinationCertificate":
		return []vctypes.FieldSpec{str("holder"), str("vaccine"), str("manufacturer"), date("administeredOn")}
	case "ePassportCredential", "PassportCh":
		return []vctypes.FieldSpec{
			str("given_name"), str("family_name"), str("passport_number"),
			date("date_of_birth"), str("nationality"), date("expires_at"),
		}
	case "TaxCredential", "TaxReceipt":
		return []vctypes.FieldSpec{str("holder"), str("taxId"), date("period")}
	case "EducationalID":
		return []vctypes.FieldSpec{str("holder"), str("institution"), str("studentId")}
	case "IdentityCredential":
		return []vctypes.FieldSpec{
			str("holder"), date("date_of_birth"),
			{Name: "age_over_18", Datatype: "boolean", Required: false},
		}
	default:
		return []vctypes.FieldSpec{str("holder")}
	}
}

// displayNameFor converts an id like "UniversityDegree_jwt_vc_json" into a
// human-readable schema name ("University Degree"). Falls back to the raw id.
// knownWaltidFormatSuffixes are the `_<format>` trailers walt.id appends to
// every credential configuration id. Stripping them reveals the base type
// name, which can itself contain underscores (e.g. "identity_credential").
// Order matters — longer suffixes first so we don't prematurely match a
// prefix of a longer one ("_jwt_vc" would chop "_jwt_vc_json" otherwise).
var knownWaltidFormatSuffixes = []string{
	"_jwt_vc_json-ld",
	"_jwt_vp_json-ld",
	"_jwt_vc_json",
	"_jwt_vp_json",
	"_vc+sd-jwt",
	"_dc+sd-jwt",
	"_mso_mdoc",
	"_ldp_vc",
	"_ldp_vp",
	"_jwt_vc",
	"_jwt_vp",
}

func displayNameFor(id string, cfg credentialConfigurationEntry) string {
	// 1. Prefer the configuration's declared display name if walt.id
	//    provides one — that's the cleanest possible label.
	if len(cfg.Display) > 0 && strings.TrimSpace(cfg.Display[0].Name) != "" {
		return strings.TrimSpace(cfg.Display[0].Name)
	}
	// 2. Strip the known format suffix from the id. walt.id's config ids
	//    all end with `_<format>`, but the type itself can contain
	//    underscores (see `identity_credential_vc+sd-jwt`), so splitting
	//    on the first underscore is wrong — suffix stripping preserves
	//    the full type name.
	base := id
	for _, suf := range knownWaltidFormatSuffixes {
		if strings.HasSuffix(base, suf) {
			base = strings.TrimSuffix(base, suf)
			break
		}
	}
	if base == "" {
		return id
	}
	// 3. Humanise: split snake_case on `_` and insert spaces before
	//    inner capitals on CamelCase. "identity_credential" →
	//    "Identity Credential"; "IdentityCredential" → "Identity
	//    Credential"; "KycDataCredential" → "Kyc Data Credential".
	var parts []string
	for _, word := range strings.Split(base, "_") {
		if word == "" {
			continue
		}
		var out []rune
		for i, r := range word {
			if i > 0 && r >= 'A' && r <= 'Z' {
				out = append(out, ' ')
			}
			if i == 0 && r >= 'a' && r <= 'z' {
				r -= 32 // title-case first letter
			}
			out = append(out, r)
		}
		parts = append(parts, string(out))
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
