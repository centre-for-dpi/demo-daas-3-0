// Package backend defines the Adapter interface that the handlers depend on.
// Implement this interface to connect a real backend (walt.id, Inji Certify,
// Inji Verify, or your own).
//
// Every request and response type is in this package. Domain types (DPG,
// Schema, Credential, ...) live in vctypes.
//
// To plug in your backend:
//
//	type MyAdapter struct { apiURL string; token string }
//	func (a *MyAdapter) ListIssuerDpgs(ctx context.Context) (map[string]vctypes.DPG, error) { ... }
//	// ... implement every method ...
//
//	// Then in cmd/server/main.go:
//	h := &handlers.H{ Adapter: myadapter.New(url, token), ... }
//
// See INTEGRATION.md for endpoint-level mapping per DPG vendor.
package backend

import (
	"context"
	"time"

	"github.com/verifiably/verifiably-go/vctypes"
)

// Adapter is the single seam between the UI and any backend.
//
// All methods take a context.Context so handlers can propagate request deadlines
// and cancellation. Errors bubble up to the HTMX layer as toasts; return
// descriptive error messages when something goes wrong.
type Adapter interface {
	// --- Catalogs: static capability metadata ---
	//
	// In a real deployment these return your gateway's list of supported DPGs
	// for each role, including plain-language capability descriptions that the
	// UI surfaces to end users. If you have only one vendor per role, return a
	// single-entry map.

	// ListIssuerDpgs returns issuer-capable DPGs keyed by vendor name.
	ListIssuerDpgs(ctx context.Context) (map[string]vctypes.DPG, error)

	// ListHolderDpgs returns wallet DPGs keyed by vendor name.
	ListHolderDpgs(ctx context.Context) (map[string]vctypes.DPG, error)

	// ListVerifierDpgs returns verifier DPGs keyed by vendor name.
	ListVerifierDpgs(ctx context.Context) (map[string]vctypes.DPG, error)

	// --- Schemas ---

	// ListSchemas returns schemas available to issue with the given issuer DPG.
	// Includes both pre-configured catalog schemas and any user-saved custom ones.
	ListSchemas(ctx context.Context, issuerDpg string) ([]vctypes.Schema, error)

	// ListAllSchemas returns every schema, regardless of which DPG supports it.
	// Used by lookup-by-id paths in the issuance flow.
	ListAllSchemas(ctx context.Context) ([]vctypes.Schema, error)

	// SaveCustomSchema persists a user-built schema. Called from the schema
	// builder's "Save" button. The schema's ID is already set by the caller.
	SaveCustomSchema(ctx context.Context, schema vctypes.Schema) error

	// DeleteCustomSchema removes a user-built schema. Pre-configured catalog
	// schemas should not be deletable; return an error if id refers to one.
	DeleteCustomSchema(ctx context.Context, id string) error

	// --- Issuance ---

	// PrefillSubjectFields returns default values for the single-subject form
	// when the operator chooses "Enter manually". For non-manual sources
	// (API / MOSIP / custom plugin / presentation-during-issuance), this is
	// not called.
	PrefillSubjectFields(ctx context.Context, schema vctypes.Schema) (map[string]string, error)

	// IssueToWallet generates a credential offer deliverable via OID4VCI.
	// Returns the offer URI that the holder's wallet will open.
	IssueToWallet(ctx context.Context, req IssueRequest) (IssueToWalletResult, error)

	// IssueAsPDF generates a printable credential with an embedded QR code.
	// Only called when the issuer DPG's DirectPDF capability is true.
	IssueAsPDF(ctx context.Context, req IssueRequest) (IssueAsPDFResult, error)

	// IssueBulk processes a CSV-worth of rows. Returns a summary; per-row
	// success/failure detail is for UI consumption (not every row needs to
	// be echoed back — errors + total counts suffice).
	IssueBulk(ctx context.Context, req IssueBulkRequest) (IssueBulkResult, error)

	// --- Wallet / holder ---

	// ListWalletCredentials returns credentials already held by this holder.
	// In the demo this includes a seed credential; in production, query your wallet store.
	ListWalletCredentials(ctx context.Context) ([]vctypes.Credential, error)

	// ListExampleOffers returns demo offer URIs that the wallet's "paste example"
	// helper cycles through. For production, return an empty slice (the feature
	// is a demo-only affordance) or a small list of test-environment URIs.
	ListExampleOffers(ctx context.Context) ([]string, error)

	// ParseOffer resolves an openid-credential-offer:// URI (inline or by-reference)
	// into a credential-for-review shape. Must return a Credential with Status="pending"
	// and a freshly-assigned ID the UI can target for accept/reject.
	ParseOffer(ctx context.Context, offerURI string) (vctypes.Credential, error)

	// ClaimCredential consummates an offer — the holder has approved it.
	// Return the claimed credential with Status="accepted".
	ClaimCredential(ctx context.Context, cred vctypes.Credential) (vctypes.Credential, error)

	// --- Verifier ---

	// ListOID4VPTemplates returns preset presentation requests the verifier can issue.
	// The UI populates a dropdown from this. For production, return your verifier's
	// configured templates.
	ListOID4VPTemplates(ctx context.Context) (map[string]vctypes.OID4VPTemplate, error)

	// RequestPresentation generates an OID4VP presentation request URI and
	// a server-side state token. The UI shows the URI as a QR + link; the
	// state is used to correlate the holder's response on a later call.
	RequestPresentation(ctx context.Context, req PresentationRequest) (PresentationRequestResult, error)

	// FetchPresentationResult retrieves the verification outcome for a previously
	// issued OID4VP request, identified by state. templateKey is passed through so
	// the adapter can reconstruct what was asked for (which fields, etc.).
	FetchPresentationResult(ctx context.Context, state, templateKey string) (VerificationResult, error)

	// VerifyDirect validates a credential the holder handed over (scan / upload / paste),
	// without an OID4VP round-trip. method is "scan" | "upload" | "paste". For "paste",
	// CredentialData is the raw credential string; for scan/upload it's empty (the
	// UI simulates; a real adapter with real input would receive the bytes here too).
	VerifyDirect(ctx context.Context, req DirectVerifyRequest) (VerificationResult, error)
}

// --- Request / response shapes ---

// IssueRequest is the input to both IssueToWallet and IssueAsPDF.
type IssueRequest struct {
	IssuerDpg   string
	Schema      vctypes.Schema
	SubjectData map[string]string
	Flow        string // "pre_auth" or "auth_code"; empty = adapter default
}

// IssueToWalletResult describes a generated credential offer.
type IssueToWalletResult struct {
	OfferURI  string        // the openid-credential-offer:// URI
	OfferID   string        // adapter-assigned id for tracing / retrieval
	Flow      string        // echoes the flow actually used (may differ from request)
	ExpiresIn time.Duration // how long until the offer becomes invalid
}

// IssueAsPDFResult describes a generated PDF credential.
type IssueAsPDFResult struct {
	IssuerName    string            // human-readable issuer name (for the PDF header)
	IssuerDID     string            // issuer DID (for the PDF header / QR payload)
	PayloadSizeKB int               // approximate QR payload size in kilobytes
	Fields        map[string]string // the subject fields as issued (echo of input)
}

// IssueBulkRequest is the input to IssueBulk.
type IssueBulkRequest struct {
	IssuerDpg string
	Schema    vctypes.Schema
	RowCount  int // number of rows in the input CSV (stand-in for actual row data in this prototype)
}

// IssueBulkResult summarizes a bulk-issue operation.
type IssueBulkResult struct {
	Accepted int
	Rejected int
	Errors   []BulkError // illustrative errors for UI display (not necessarily exhaustive)
}

// BulkError describes one row-level error in a bulk issuance.
type BulkError struct {
	Row    int
	Reason string
}

// PresentationRequest is the input to RequestPresentation.
type PresentationRequest struct {
	VerifierDpg string
	TemplateKey string // adapter-defined; maps to the verifier's stored presentation templates
}

// PresentationRequestResult describes a generated OID4VP request.
type PresentationRequestResult struct {
	RequestURI string // the openid4vp:// URI
	State      string // server-side correlation token; echo back to FetchPresentationResult
	Template   vctypes.OID4VPTemplate
}

// DirectVerifyRequest is the input to VerifyDirect.
type DirectVerifyRequest struct {
	VerifierDpg    string
	Method         string // "scan" | "upload" | "paste"
	CredentialData string // raw credential for paste; empty for scan/upload in this prototype
}

// VerificationResult is the output of both FetchPresentationResult and VerifyDirect.
type VerificationResult struct {
	Valid             bool
	Method            string    // human-readable: "OID4VP · selective — only age_over_18 is shared"
	Format            string    // credential format: "w3c_vcdm_2", "sd_jwt_vc (IETF)", etc.
	Issuer            string    // issuer identifier (DID or URL)
	Subject           string    // subject identifier (typically a DID)
	Requested         []string  // fields received (OID4VP only; nil for direct verify)
	Issued            time.Time // when the credential was originally issued
	CheckedRevocation bool      // false for offline scan (no status-list lookup possible)
}
