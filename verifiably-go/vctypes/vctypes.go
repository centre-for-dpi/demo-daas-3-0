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

	// Issuer-specific capability flags
	FlowPreAuth                 bool
	FlowAuthCode                bool
	FlowPresentationDuringIssue bool
	FlowPlain                   string // plain-language explanation of flows
	FormatsPlain                string // plain-language explanation of formats
	DirectPDF                   bool
	DirectPDFPlain              string
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
