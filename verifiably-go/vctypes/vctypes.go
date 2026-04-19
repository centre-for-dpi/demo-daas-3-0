// Package vctypes holds the domain types the UI and adapters traffic in.
// These are pure data types — no behavior, no dependencies on either the
// mock or any specific backend implementation. Import this package from
// your own adapter code.
package vctypes

import "time"

// DPG describes a Digital Public Good's capabilities.
// The same shape serves issuer, holder, and verifier DPGs; role-specific
// flags are simply unused when they don't apply.
type DPG struct {
	Vendor   string
	Version  string
	Tag      string // short label like "API-based" or "MOSIP"
	Tagline  string // one-line description
	Formats  []string
	Caveats  string // plain-language warning text shown in the UI
	Redirect bool   // holder/verifier only — if true, selecting this DPG hands off to its own UI
	// UIURL is the public URL of the vendor's own UI, used as the target
	// address the redirect-notice page links to. Populated by the registry
	// from backends.json; empty for non-redirect DPGs.
	UIURL string

	// Issuer-specific capability flags
	FlowPreAuth                 bool
	FlowAuthCode                bool
	FlowPresentationDuringIssue bool
	FlowPlain                   string // plain-language explanation of flows
	FormatsPlain                string // plain-language explanation of formats
	DirectPDF                   bool
	DirectPDFPlain              string

	// Structured capability list. Renders on the DPG picker card and drives
	// downstream screen branching via Kind/Key pairs (no string matches on
	// vendor names in handlers). Empty for legacy/mock DPGs; registry-backed
	// DPGs populate this from backends.json.
	Capabilities []Capability
}

// Capability is one structured "what you'll actually experience" item on a DPG
// card. Handlers may look up capabilities by Kind to decide whether to enable a
// downstream option (e.g. Kind="mode" Key="pdf" to enable the PDF destination).
// Title/Body render to the user; Kind/Key drive logic.
type Capability struct {
	Title string // short headline, e.g. "User logs in at the identity provider"
	Body  string // one-sentence plain-language description
	Kind  string // "flow" | "data" | "wallet" | "token" | "mode" | "limitation"
	Key   string // machine key: e.g. "auth_code", "pre_auth", "pdf", "identity_lookup"
}

// Schema is a credential schema available to an issuer.
type Schema struct {
	ID     string
	Name   string
	Std    string   // e.g. "w3c_vcdm_2", "sd_jwt_vc (IETF)", "jwt_vc", "mso_mdoc"
	DPGs   []string // vendor names that support this schema
	Desc   string
	Custom bool // true for user-built schemas

	// Custom-schema extras (empty for pre-configured schemas)
	AdditionalTypes []string
	FieldsSpec      []FieldSpec
}

// FieldSpec describes one claim in a credential schema.
type FieldSpec struct {
	Name     string
	Datatype string // "string" | "number" | "integer" | "boolean"
	Format   string // optional: "date" | "uri" | ...
	Required bool
}

// Credential is the wallet/verifier-side view of an issued credential.
// For pending offers (not yet accepted into a wallet), Status is "pending".
type Credential struct {
	ID     string
	Title  string
	Issuer string
	Type   string
	Status string // "pending" | "accepted"
	Source string // "scan" | "paste" | "inbox" — how the holder received it
	Fields map[string]string
}

// OID4VPTemplate is a preset presentation request a verifier can issue.
type OID4VPTemplate struct {
	Title      string
	Fields     []string
	Format     string
	Disclosure string // plain-language disclosure summary shown to the verifier operator
}

// IssuerIdentity is the operator's own display identity (the "who's issuing this").
type IssuerIdentity struct {
	Name string
	DID  string
}

// Duration-safe helpers for adapters that work with time.Duration naturally.
// ExpiresIn on IssueToWalletResult is a time.Duration; helper converts to seconds.
func SecondsFromDuration(d time.Duration) int { return int(d / time.Second) }
