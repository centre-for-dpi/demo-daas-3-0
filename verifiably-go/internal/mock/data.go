// Package mock holds every hardcoded fake value the demo uses.
// This package is internal on purpose — real deployments should delete or
// ignore it and supply their own vctypes values via a real backend.Adapter.
//
// The mock adapter that wraps this data lives in internal/mock/adapter.go.
package mock

import "github.com/verifiably/verifiably-go/vctypes"

// IssuerDpgs returns issuer-capable DPGs keyed by vendor.
// Would become: GET {GATEWAY}/dpgs?role=issuer in production.
func IssuerDpgs() map[string]vctypes.DPG {
	return map[string]vctypes.DPG{
		"walt.id": {
			Vendor:         "walt.id",
			Version:        "Community Stack v0.18.2",
			Tag:            "API-based",
			Tagline:        "Open-source, API-driven credentialing stack.",
			FlowPreAuth:    true,
			FlowAuthCode:   true,
			FlowPlain:      `OID4VCI with pre-authorized code flow (issuer generates offer, holder scans) and authorization code flow. No "wallet-initiated" flow in the OID4VCI sense — wallet always scans an issuer-generated offer.`,
			Formats:        []string{"w3c_vcdm_1", "w3c_vcdm_2", "sd_jwt_vc (IETF)", "jwt_vc", "mso_mdoc"},
			FormatsPlain:   "W3C VCDM 1.1 and 2.0 signed as JWT, SD-JWT or JSON-LD, plus ISO 18013-5 mdoc credentials. SD-JWT VC follows the IETF draft.",
			DirectPDF:      false,
			DirectPDFPlain: `No documented "QR on PDF" export in the stack as of v0.18.2. Credentials are delivered via OID4VCI to a wallet. A PDF rendering would need to be built on top of the Issuer API.`,
			Caveats:        "OID4VP v1.0 verifier landed in the Verifier 2 API at v0.16, but wallet/demo OID4VP v1.0 support is still in progress at v0.18.2.",
		},
		"Inji Certify": {
			Vendor:                      "Inji Certify",
			Version:                     "v0.14.0",
			Tag:                         "MOSIP",
			Tagline:                     "MOSIP-ecosystem issuer with plugin-based data providers.",
			FlowPreAuth:                 true,
			FlowAuthCode:                true,
			FlowPresentationDuringIssue: true,
			FlowPlain:                   "OID4VCI draft 13 with pre-authorized code flow (new in v0.14.0), authorization code flow, and presentation-during-issuance — present an existing VC to obtain a new one.",
			Formats:                     []string{"w3c_vcdm_1", "w3c_vcdm_2", "sd_jwt_vc (IETF)", "mso_mdoc (mock only)"},
			FormatsPlain:                "W3C VCDM 1.1 and 2.0 in JSON-LD, plus IETF SD-JWT VC. mDoc/mDL exists but is MOCK-ONLY in v0.14.0 — full implementation in a future release.",
			DirectPDF:                   true,
			DirectPDFPlain:              "v0.14.0 introduces QR Code Specifications 1.1.0 data embedding within QR codes for credential download. Combined with the MOSIP Identity Plugin, this supports issuing a self-contained QR on a printable PDF.",
			Caveats:                     "mDoc support is mock-only in v0.14.0. Data providers require plugin setup (MOSIP Identity Plugin or custom, e.g. Sunbird RC).",
		},
	}
}

// HolderDpgs returns wallet DPGs.
func HolderDpgs() map[string]vctypes.DPG {
	return map[string]vctypes.DPG{
		"walt.id Web Wallet": {
			Vendor:    "walt.id Web Wallet",
			Version:   "Community Stack v0.18.2",
			Tag:       "API-based",
			Tagline:   "Custodial web wallet driven by walt.id Wallet API — flows run inline here.",
			FlowPlain: "You scan OID4VCI offer QRs or paste links directly in this UI. OID4VP presentation works with the older spec; full OID4VP v1.0 wallet support is still rolling out in the community stack.",
			Formats:   []string{"w3c_vcdm_2", "sd_jwt_vc (IETF)", "mso_mdoc"},
			Caveats:   "OID4VP v1.0 not yet fully supported in the wallet/demo apps at v0.18.2. Older OID4VP (Presentation Exchange-based) works.",
			Redirect:  false,
		},
		"Inji Web Wallet": {
			Vendor:    "Inji Web Wallet",
			Version:   "v0.16.0",
			Tag:       "Redirected",
			Tagline:   "Full MOSIP web wallet — opens in its own UI.",
			FlowPlain: "Redirects to the Inji Web Wallet app. v0.16.0 adds SVG-based VC rendering (via the inji-vc-renderer library) with SVG→PDF export, plus Claim 169 QR support for interpreting verifier-initiated MOSIP-spec presentation requests.",
			Formats:   []string{"w3c_vcdm_1", "w3c_vcdm_2"},
			Caveats:   "Tested-compatible with Inji Certify v0.13.1 (not v0.14.0) and Inji Verify v0.17.0 per the v0.16.0 compatibility matrix. Primarily handles W3C VCDM with Ed25519 and RSA signature suites. Known bug INJIWEB-1417: download failed for Ed25519Signature2018/RsaSignature2018 credentials (fixed in v0.16.0). SD-JWT VC and mDoc holding are not documented as supported.",
			Redirect:  true,
		},
	}
}

// VerifierDpgs returns verifier DPGs.
func VerifierDpgs() map[string]vctypes.DPG {
	return map[string]vctypes.DPG{
		"walt.id Verifier": {
			Vendor:    "walt.id Verifier",
			Version:   "Community Stack v0.18.2",
			Tag:       "API-based",
			Tagline:   "Verifier 2 API with OID4VP v1.0 and DCQL — inline verification here.",
			FlowPlain: "Stays in this UI. Generate OID4VP v1.0 presentation requests (QR or link) using DCQL queries, or verify a credential directly.",
			Formats:   []string{"w3c_vcdm_2", "sd_jwt_vc (IETF)", "mso_mdoc"},
			Caveats:   "Legacy Verifier (pre-v0.16 API) is being deprecated; Verifier 2 is the forward path with full OID4VP v1.0 compliance.",
			Redirect:  false,
		},
		"Inji Verify": {
			Vendor:    "Inji Verify",
			Version:   "v0.16.0",
			Tag:       "Redirected",
			Tagline:   "MOSIP verifier portal — opens in its own UI.",
			FlowPlain: "Redirects to Inji Verify. v0.16.0 adds real-time revocation checks (W3C BitString Status List 2021), multi-lingual rendering, SVG rendering, MOSIP UIN VC verification, and SD-JWT VC submission via the /vc-submission endpoint (INJIVER-1308).",
			Formats:   []string{"w3c_vcdm_1", "w3c_vcdm_2", "sd_jwt_vc (IETF)"},
			Caveats:   "Signature suites: Ed25519Signature2018, Ed25519Signature2020, RsaSignature2018. Tested-compatible with Inji Web v0.14.0 and Inji Wallet v0.20.0 (not Inji Web Wallet v0.16.0) per the v0.16.0 matrix. Known bug INJIVER-1131: cross-device OID4VP validates presentations as successful even when a wrong VC is submitted — mitigation is credential-type validation on the Relying Party side. Upstream UI is explicitly a reference implementation.",
			Redirect:  true,
		},
	}
}

// Schemas returns pre-configured credential schemas.
// Every builtin schema has its FieldsSpec populated so handlers don't need
// a separate "what fields does this schema have" lookup.
func Schemas() []vctypes.Schema {
	str := func(names ...string) []vctypes.FieldSpec {
		out := make([]vctypes.FieldSpec, len(names))
		for i, n := range names {
			out[i] = vctypes.FieldSpec{Name: n, Datatype: "string", Required: true}
		}
		return out
	}
	return []vctypes.Schema{
		{ID: "sch1", Name: "University Degree", Std: "w3c_vcdm_2", DPGs: []string{"walt.id", "Inji Certify"}, Desc: "Academic qualification with classification, conferred date, and awarding institution.",
			FieldsSpec: str("holder", "degree", "classification", "conferred")},
		{ID: "sch2", Name: "Driver's Licence", Std: "mso_mdoc", DPGs: []string{"walt.id"}, Desc: "ISO 18013-5 mobile driving licence with vehicle categories. Inji Certify mDoc is mock-only at v0.14.0.",
			FieldsSpec: str("holder", "licence_no", "categories", "expiry")},
		{ID: "sch3", Name: "Proof of Age", Std: "sd_jwt_vc (IETF)", DPGs: []string{"walt.id", "Inji Certify"}, Desc: `Selectively disclose only "over 18" without revealing date of birth. IETF SD-JWT VC.`,
			FieldsSpec: str("holder", "date_of_birth", "jurisdiction")},
		{ID: "sch4", Name: "Employment Record", Std: "w3c_vcdm_1", DPGs: []string{"walt.id", "Inji Certify"}, Desc: "Job title, employer, tenure dates. VCDM 1.1.",
			FieldsSpec: str("holder", "employer", "title", "start_date", "end_date")},
		{ID: "sch5", Name: "Vaccination Record", Std: "w3c_vcdm_2", DPGs: []string{"walt.id", "Inji Certify"}, Desc: "Vaccine type, manufacturer, dose, date, administering facility.",
			FieldsSpec: str("holder", "vaccine", "manufacturer", "dose", "date")},
		{ID: "sch6", Name: "Professional Licence", Std: "jwt_vc", DPGs: []string{"walt.id"}, Desc: "Simple JWT-VC for regulated profession licensure.",
			FieldsSpec: str("holder", "profession", "licence_no", "expiry")},
		{ID: "sch7", Name: "Digital ID Card", Std: "mso_mdoc", DPGs: []string{"walt.id"}, Desc: "Mobile ID for in-person and online identity verification. ISO 18013-5.",
			FieldsSpec: str("holder", "id_number", "dob", "photo_ref")},
		{ID: "sch8", Name: "Course Certificate", Std: "sd_jwt_vc (IETF)", DPGs: []string{"Inji Certify"}, Desc: "Completion certificate with optional selective disclosure of grades.",
			FieldsSpec: str("holder", "course", "grade", "completed")},
		{ID: "sch9", Name: "MOSIP UIN Credential", Std: "w3c_vcdm_2", DPGs: []string{"Inji Certify"}, Desc: "MOSIP-issued identity credential via the Identity Plugin (new in Certify v0.14.0).",
			FieldsSpec: str("holder", "uin", "dob", "gender", "address")},
	}
}

// SubjectValues are mock field values the issuer form pre-populates.
func SubjectValues() map[string]string {
	return map[string]string{
		"holder": "Achieng Otieno", "degree": "BSc Computer Science", "classification": "First Class",
		"conferred": "2024-07-14", "licence_no": "DL-2847-0013", "categories": "B, C1",
		"expiry": "2029-06-30", "date_of_birth": "1998-03-22", "jurisdiction": "KE",
		"employer": "Acme Ltd", "title": "Senior Engineer", "start_date": "2020-01-15", "end_date": "2024-03-31",
		"vaccine": "COVID-19", "manufacturer": "Pfizer", "dose": "3", "date": "2023-11-04",
		"profession": "Registered Nurse", "id_number": "ID-9284-7710", "dob": "1995-08-11",
		"photo_ref": "sha256:a4f…", "course": "Introduction to Cryptography", "grade": "Distinction",
		"completed": "2024-09-30",
		"uin":       "3547-0013-9920",
		"gender":    "F",
		"address":   "Nairobi, Kenya",
	}
}

// IssuerIdentities are mock operator identities per issuer DPG.
func IssuerIdentities() map[string]vctypes.IssuerIdentity {
	return map[string]vctypes.IssuerIdentity{
		"walt.id":      {Name: "Strathmore University", DID: "did:web:strathmore.edu"},
		"Inji Certify": {Name: "Ministry of Education", DID: "did:web:education.gov.mosip"},
	}
}

// SubjectDID is the mock subject DID shown in verifier results.
const SubjectDID = "did:key:z6Mk…m4n1"

// OfferURIHosts is the mock issuer hostname used in generated offer URIs.
func OfferURIHosts() map[string]string {
	return map[string]string{
		"walt.id":      "issuer.walt.id",
		"Inji Certify": "certify.mosip.net",
	}
}

// VerifierHosts is the mock verifier hostname used in OID4VP request URIs.
func VerifierHosts() map[string]string {
	return map[string]string{
		"walt.id Verifier": "verifier.walt-id.demo",
		"Inji Verify":      "verifier.mosip.demo",
	}
}

// ExampleOffer is a pre-populated incoming OID4VCI offer used as demo data.
type ExampleOffer struct {
	URI   string
	Offer vctypes.Credential
}

func ExampleOffers() []ExampleOffer {
	return []ExampleOffer{
		{
			URI: "openid-credential-offer://?credential_offer_uri=https://certify-demo.verifiably.local/offer/a4f2c9e1",
			Offer: vctypes.Credential{
				Title: "Professional Licence", Issuer: "Nursing Council of Kenya", Type: "jwt_vc",
				Fields: map[string]string{"holder": "Achieng Otieno", "profession": "Registered Nurse", "licence_no": "NCK-2024-18472", "expiry": "2027-03-15"},
			},
		},
		{
			URI: "openid-credential-offer://?credential_offer_uri=https://issuer.walt-id.demo/offer/8d1e0b",
			Offer: vctypes.Credential{
				Title: "University Transcript", Issuer: "Strathmore University", Type: "w3c_vcdm_2",
				Fields: map[string]string{"holder": "Achieng Otieno", "programme": "MSc Data Science", "gpa": "3.8 / 4.0", "graduated": "2025-11-20"},
			},
		},
		{
			URI: "openid-credential-offer://?credential_offer_uri=https://mosip-certify.demo/offer/3e7a11",
			Offer: vctypes.Credential{
				Title: "National ID", Issuer: "Ministry of Interior (MOSIP)", Type: "w3c_vcdm_2",
				Fields: map[string]string{"holder": "Achieng Otieno", "uin": "3547-0013-9920", "dob": "1998-03-22", "nationality": "Kenyan"},
			},
		},
	}
}

// OID4VPTemplates returns preset OID4VP presentation requests.
func OID4VPTemplates() map[string]vctypes.OID4VPTemplate {
	return map[string]vctypes.OID4VPTemplate{
		"age":     {Title: "Proof of age over 18", Fields: []string{"age_over_18"}, Format: "sd_jwt_vc (IETF)", Disclosure: "selective — only age_over_18 is shared"},
		"licence": {Title: "Professional licence", Fields: []string{"profession", "licence_no", "expiry"}, Format: "jwt_vc", Disclosure: "full credential shared"},
		"degree":  {Title: "University degree", Fields: []string{"degree", "classification", "conferred"}, Format: "w3c_vcdm_2", Disclosure: "full credential shared"},
		"id":      {Title: "Government ID", Fields: []string{"holder", "date_of_birth"}, Format: "w3c_vcdm_2", Disclosure: "full credential shared"},
	}
}

// VerificationIssuers returns mock issuer identities shown in verification results.
func VerificationIssuers() map[string]string {
	return map[string]string{
		"oid4vp": "did:web:nck.or.ke",
		"direct": "did:web:strathmore.edu",
	}
}

// SeedWalletCreds is the credential the wallet starts with on first visit.
func SeedWalletCreds() []vctypes.Credential {
	return []vctypes.Credential{
		{
			ID: "wc1", Title: "University Degree", Issuer: "Strathmore University", Type: "w3c_vcdm_2", Status: "accepted",
			Fields: map[string]string{"holder": "A. Otieno", "degree": "BSc Computer Science", "classification": "First Class", "conferred": "2024-07-14"},
		},
	}
}

// SchemaFields returns the canonical field list per pre-configured schema name.
// Custom schemas supply their own FieldsSpec; this is only used for built-ins.
func SchemaFields(schemaName string) []string {
	m := map[string][]string{
		"University Degree":    {"holder", "degree", "classification", "conferred"},
		"Driver's Licence":     {"holder", "licence_no", "categories", "expiry"},
		"Proof of Age":         {"holder", "date_of_birth", "jurisdiction"},
		"Employment Record":    {"holder", "employer", "title", "start_date", "end_date"},
		"Vaccination Record":   {"holder", "vaccine", "manufacturer", "dose", "date"},
		"Professional Licence": {"holder", "profession", "licence_no", "expiry"},
		"Digital ID Card":      {"holder", "id_number", "dob", "photo_ref"},
		"Course Certificate":   {"holder", "course", "grade", "completed"},
		"MOSIP UIN Credential": {"holder", "uin", "dob", "gender", "address"},
	}
	if f, ok := m[schemaName]; ok {
		return f
	}
	return []string{"holder", "field_1", "field_2"}
}
