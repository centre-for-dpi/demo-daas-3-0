package model

// Capabilities describes what a given DPG backend supports across all services.
// Each adaptor contributes its portion; the Stores layer composes them into
// a single struct the UI can query via GET /api/capabilities.
type Capabilities struct {
	Issuer   IssuerCapabilities   `json:"issuer"`
	Wallet   WalletCapabilities   `json:"wallet"`
	Verifier VerifierCapabilities `json:"verifier"`

	// Friendly names for the active backend per service.
	IssuerName   string `json:"issuerName"`
	WalletName   string `json:"walletName"`
	VerifierName string `json:"verifierName"`

	// Beta flags — when true, the UI shows a "Beta / Coming soon" banner for that service.
	IssuerBeta   bool `json:"issuerBeta"`
	WalletBeta   bool `json:"walletBeta"`
	VerifierBeta bool `json:"verifierBeta"`
}

// IssuerCapabilities describes what the issuer backend supports.
type IssuerCapabilities struct {
	// IssuerInitiated: the issuer creates an OID4VCI credential offer the holder claims.
	IssuerInitiated bool `json:"issuerInitiated"`

	// Batch: the issuer supports batch/campaign issuance of many credentials at once.
	Batch bool `json:"batch"`

	// Deferred: the issuer supports deferred issuance (holder claims later).
	Deferred bool `json:"deferred"`

	// SelectiveDisclosure: the issuer can produce SD-JWT credentials.
	SelectiveDisclosure bool `json:"selectiveDisclosure"`

	// CustomTypes: the issuer supports registering new credential types at runtime.
	CustomTypes bool `json:"customTypes"`

	// Revocation: the issuer supports revoking issued credentials.
	Revocation bool `json:"revocation"`

	// Formats supported (e.g. "jwt_vc_json", "vc+sd-jwt", "mso_mdoc", "ldp_vc").
	Formats []string `json:"formats"`
}

// WalletCapabilities describes what the wallet backend supports.
type WalletCapabilities struct {
	// ClaimOffer: can claim OID4VCI offers into the wallet.
	ClaimOffer bool `json:"claimOffer"`

	// CreateDIDs: can generate new DIDs in the wallet.
	CreateDIDs bool `json:"createDids"`

	// HolderInitiatedPresentation: can create a presentation proactively (not just respond).
	HolderInitiatedPresentation bool `json:"holderInitiatedPresentation"`

	// VerifierInitiatedPresentation: can respond to OID4VP requests.
	VerifierInitiatedPresentation bool `json:"verifierInitiatedPresentation"`

	// SelectiveDisclosure: can choose which claims to disclose.
	SelectiveDisclosure bool `json:"selectiveDisclosure"`

	// DIDMethods supported (e.g. "did:jwk", "did:key", "did:web").
	DIDMethods []string `json:"didMethods"`
}

// VerifierCapabilities describes what the verifier backend supports.
type VerifierCapabilities struct {
	// CreateRequest: can create OID4VP verification requests.
	CreateRequest bool `json:"createRequest"`

	// DirectVerify: can verify a credential presented directly (without a session).
	DirectVerify bool `json:"directVerify"`

	// PresentationDefinition: supports DIF Presentation Exchange.
	PresentationDefinition bool `json:"presentationDefinition"`

	// PolicyEngine: has a configurable policy engine.
	PolicyEngine bool `json:"policyEngine"`

	// RevocationCheck: checks revocation status list during verification.
	RevocationCheck bool `json:"revocationCheck"`

	// DIDMethods the verifier can resolve for issuer validation.
	DIDMethods []string `json:"didMethods"`
}
