package handler

import (
	"net/http"
	"sync"
	"time"

	"vcplatform/internal/auth"
	"vcplatform/internal/config"
	"vcplatform/internal/datasource"
	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/onboarding"
	"vcplatform/internal/render"
	"vcplatform/internal/store"
	"vcplatform/internal/store/ldpsigner"
)

// IssuerDIDEntry represents a persisted issuer DID from the onboarding flow.
type IssuerDIDEntry struct {
	DID       string `json:"did"`
	KeyType   string `json:"keyType"`
	CreatedAt string `json:"createdAt"`
}

// CredentialSchema represents a credential schema created in the Schema Builder.
type CredentialSchema struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	ConfigID  string              `json:"configId"` // Backend credential configuration ID
	Version   string              `json:"version"`
	Format    string              `json:"format"`   // jwt_vc_json, sdjwt_vc, ldp_vc, mso_mdoc
	Standard  string              `json:"standard"` // W3C-VCDM 2.0, SD-JWT, etc.
	Fields    []model.SchemaField `json:"fields"`
	CreatedAt string              `json:"createdAt"`
}

// Handler holds shared dependencies for all route handlers.
type Handler struct {
	render      *render.Renderer
	stores      *store.Stores
	config      *config.Config
	ssoRegistry *auth.Registry
	dataSources *datasource.Registry
	ldpSigner   *ldpsigner.Signer
	localIssuer *LocalIssuer
	onboarding  onboarding.Store

	// issuerRegistry / walletRegistry / verifierRegistry hold a store
	// instance for every DPG the deployment supports. They let the handler
	// resolve the right store at request time based on the logged-in user's
	// chosen DPG, rather than any server-level env var.
	issuerRegistry   map[string]store.IssuerStore
	walletRegistry   map[string]store.WalletStore
	verifierRegistry map[string]store.VerifierStore

	// In-memory stores, keyed by user ID.
	// In production these would be persisted to a database.
	issuerDIDsMu sync.RWMutex
	issuerDIDs   map[string][]IssuerDIDEntry

	schemasMu sync.RWMutex
	schemas   map[string][]CredentialSchema

	// agentArtifacts holds chatbot-generated documents (pitch decks, advisory
	// notes, technical scopes) saved from the chat surface for the Outputs page.
	// In-memory and shared across all browsers in this exploration deployment;
	// when we add real users this becomes user-keyed.
	agentArtifacts *agentArtifactStore

	// shareStore holds credentials the holder has proactively staged for a
	// verifier to view via a short-lived /share/v/{id} link.
	shareStore *shareStore
}

// New creates a new Handler.
func New(r *render.Renderer, s *store.Stores, c *config.Config, sso *auth.Registry, ds *datasource.Registry) *Handler {
	if ds == nil {
		ds = datasource.NewRegistry()
	}
	// The in-process LDP_VC signer is used to produce JSON-LD credentials
	// with Ed25519Signature2020 proofs when a DPG (e.g. walt.id issuer-api)
	// doesn't natively expose an ldp_vc issuance endpoint. The key is
	// process-lifetime ephemeral — regenerated on each server start.
	signer, err := ldpsigner.New()
	if err != nil {
		// Non-fatal: LDP issuance simply won't be available until restart.
		signer = nil
	}
	// LocalIssuer wraps the signer in a real OID4VCI Pre-Authorized Code
	// flow surface, so any compliant wallet can claim LDP_VC credentials
	// through the standard protocol instead of our old `local-bag://` shortcut.
	var localIssuer *LocalIssuer
	if signer != nil {
		localIssuer = NewLocalIssuer(DefaultLocalIssuerBaseURL(), signer)
	}
	return &Handler{
		render:           r,
		stores:           s,
		config:           c,
		ssoRegistry:      sso,
		dataSources:      ds,
		ldpSigner:        signer,
		localIssuer:      localIssuer,
		onboarding:       onboarding.NewMemoryStore(),
		issuerRegistry:   map[string]store.IssuerStore{},
		walletRegistry:   map[string]store.WalletStore{},
		verifierRegistry: map[string]store.VerifierStore{},
		issuerDIDs:       make(map[string][]IssuerDIDEntry),
		schemas:          make(map[string][]CredentialSchema),
		agentArtifacts:   newAgentArtifactStore(),
		shareStore:       newShareStore(),
	}
}

// SetIssuerRegistry installs the map of DPG name → IssuerStore. Called once
// at startup from main.go with all DPGs the deployment has enabled.
func (h *Handler) SetIssuerRegistry(reg map[string]store.IssuerStore) {
	h.issuerRegistry = reg
}

// SetWalletRegistry installs the map of DPG name → WalletStore.
func (h *Handler) SetWalletRegistry(reg map[string]store.WalletStore) {
	h.walletRegistry = reg
}

// SetVerifierRegistry installs the map of DPG name → VerifierStore.
func (h *Handler) SetVerifierRegistry(reg map[string]store.VerifierStore) {
	h.verifierRegistry = reg
}

// issuerFor returns the issuer store for the given user. Priority:
//
//  1. The DPG the user picked during onboarding (user.IssuerDPG), if the
//     deployment has a registered issuer for that DPG.
//  2. The server-wide default (h.stores.Issuer — picked by ISSUER_DPG env).
//
// This is the core of per-user backend routing.
func (h *Handler) issuerFor(user *model.User) store.IssuerStore {
	if user != nil && user.IssuerDPG != "" {
		if s, ok := h.issuerRegistry[user.IssuerDPG]; ok && s != nil {
			return s
		}
	}
	// Fallback: the cookie might have been re-written without DPG fields
	// (e.g. a session encoded via the legacy EncodeSession helper), but the
	// onboarding store still remembers what the user picked in the wizard.
	// Honor that before dropping to the server default so users never see
	// a silent backend switch between requests.
	if user != nil && user.ID != "" {
		if st := h.onboarding.Get(user.ID); st != nil && st.IssuerDPG != "" {
			if s, ok := h.issuerRegistry[st.IssuerDPG]; ok && s != nil {
				return s
			}
		}
	}
	return h.stores.Issuer
}

// walletFor returns the wallet store for the given user. Priority:
//  1. The DPG the user picked during holder onboarding (user.WalletDPG).
//  2. The server-wide default (h.stores.Wallet).
func (h *Handler) walletFor(user *model.User) store.WalletStore {
	if user != nil && user.WalletDPG != "" {
		if s, ok := h.walletRegistry[user.WalletDPG]; ok && s != nil {
			return s
		}
	}
	// See issuerFor — honor the onboarding-store pick before falling back
	// to the server default so a re-logged-in SSO user never silently
	// switches wallet backend behind their back.
	if user != nil && user.ID != "" {
		if st := h.onboarding.Get(user.ID); st != nil && st.WalletDPG != "" {
			if s, ok := h.walletRegistry[st.WalletDPG]; ok && s != nil {
				return s
			}
		}
	}
	return h.stores.Wallet
}

// verifierFor returns the verifier store for the given user. Priority:
//  1. The DPG the user picked during verifier onboarding (user.VerifierDPG).
//  2. The server-wide default (h.stores.Verifier).
func (h *Handler) verifierFor(user *model.User) store.VerifierStore {
	if user != nil && user.VerifierDPG != "" {
		if s, ok := h.verifierRegistry[user.VerifierDPG]; ok && s != nil {
			return s
		}
	}
	// See issuerFor — honor the onboarding-store pick before falling back
	// to the server default so a re-logged-in SSO user never silently
	// switches verifier backend behind their back.
	if user != nil && user.ID != "" {
		if st := h.onboarding.Get(user.ID); st != nil && st.VerifierDPG != "" {
			if s, ok := h.verifierRegistry[st.VerifierDPG]; ok && s != nil {
				return s
			}
		}
	}
	return h.stores.Verifier
}

// htmxAwareRedirect issues a redirect that works cleanly for both HTMX-
// driven navigation and plain browser requests.
//
// The problem with plain http.Redirect inside an HTMX flow: HTMX's
// underlying XHR transparently follows the 303, but the follow-up
// request loses the HX-Request header, so the server renders a FULL
// HTML page instead of the fragment HTMX expected. HTMX then tries to
// swap the full page into the target element and ends up with blank /
// inconsistent content — the symptom being "I can reach the wizard
// only via a hard refresh, not by clicking in the sidebar".
//
// HTMX's HX-Location response header is the correct fix: the server
// returns 200 with HX-Location set to the destination URL, and HTMX
// does a client-side AJAX navigation to that URL, preserving the SPA
// behavior. For non-HTMX requests we fall back to the standard 303.
func (h *Handler) htmxAwareRedirect(w http.ResponseWriter, r *http.Request, location string) {
	if middleware.IsHTMX(r.Context()) {
		w.Header().Set("HX-Location", location)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

// issuerRegistryNames returns the DPG identifiers of every registered
// issuer backend. Used by the onboarding wizard to render DPG cards.
func (h *Handler) issuerRegistryNames() []string {
	out := make([]string, 0, len(h.issuerRegistry))
	for k := range h.issuerRegistry {
		out = append(out, k)
	}
	return out
}

func (h *Handler) addIssuerDID(user *model.User, did, keyType string) {
	h.issuerDIDsMu.Lock()
	defer h.issuerDIDsMu.Unlock()
	h.issuerDIDs[user.ID] = append(h.issuerDIDs[user.ID], IssuerDIDEntry{
		DID:       did,
		KeyType:   keyType,
		CreatedAt: time.Now().Format("2006-01-02 15:04"),
	})
}

func (h *Handler) getIssuerDIDs(user *model.User) []IssuerDIDEntry {
	h.issuerDIDsMu.RLock()
	defer h.issuerDIDsMu.RUnlock()
	return h.issuerDIDs[user.ID]
}

func (h *Handler) addSchema(user *model.User, schema CredentialSchema) {
	h.schemasMu.Lock()
	defer h.schemasMu.Unlock()
	schema.CreatedAt = time.Now().Format("2006-01-02 15:04")
	h.schemas[user.ID] = append(h.schemas[user.ID], schema)
}

func (h *Handler) getSchemas(user *model.User) []CredentialSchema {
	h.schemasMu.RLock()
	defer h.schemasMu.RUnlock()
	return h.schemas[user.ID]
}

// RegisterRoutes wires all routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	cfg := h.config

	// Landing & Exploration (no auth required)
	if cfg.Features.ExplorationMode {
		mux.HandleFunc("GET /{$}", h.Landing)
		mux.HandleFunc("GET /explainer", h.Explainer)
		mux.HandleFunc("GET /mockup", h.Mockup)
		mux.HandleFunc("GET /tiers", h.Tiers)
		mux.HandleFunc("GET /roles", h.Roles)
		mux.HandleFunc("GET /auditor", h.Auditor)
		mux.HandleFunc("GET /agent-output", h.AgentOutput)
		// Agent chat proxy — forwards browser requests to the n8n webhook.
		// Unauthenticated because the chatbot is available on the public landing page.
		mux.HandleFunc("POST /api/agent/chat", h.AgentChat)

		// Agent outputs — saved chatbot artifacts displayed on /agent-output.
		mux.HandleFunc("POST /api/agent/outputs", h.AgentOutputsCreate)
		mux.HandleFunc("GET /api/agent/outputs", h.AgentOutputsList)
		mux.HandleFunc("GET /api/agent/outputs/{id}", h.AgentOutputsGet)
		mux.HandleFunc("PUT /api/agent/outputs/{id}", h.AgentOutputsUpdate)
		mux.HandleFunc("DELETE /api/agent/outputs/{id}", h.AgentOutputsDelete)
		mux.HandleFunc("GET /api/agent/outputs/{id}/export", h.AgentOutputsExport)
	} else {
		// Production mode: / redirects to /login
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		})
	}

	// Auth
	mux.HandleFunc("GET /login", h.LoginForm)
	mux.HandleFunc("POST /login", h.LoginSubmit)
	mux.HandleFunc("GET /logout", h.Logout)
	mux.HandleFunc("GET /auth/redirect", h.OIDCRedirect)
	mux.HandleFunc("GET /auth/callback", h.OIDCCallback)

	// --- Middleware helpers ---
	auth := func(fn http.HandlerFunc) http.Handler {
		return middleware.AuthRequired(http.HandlerFunc(fn))
	}
	adminOnly := func(fn http.HandlerFunc) http.Handler {
		return middleware.AuthRequired(middleware.RequireRole("admin")(http.HandlerFunc(fn)))
	}
	auditAccess := func(fn http.HandlerFunc) http.Handler {
		return middleware.AuthRequired(middleware.RequireRole("admin", "auditor")(http.HandlerFunc(fn)))
	}

	// Portal routes — auth required (all roles)
	mux.Handle("GET /portal", auth(h.PortalDashboard))
	mux.Handle("GET /portal/dashboard", auth(h.PortalDashboard))
	mux.Handle("GET /portal/profile", auth(h.Profile))
	mux.Handle("GET /portal/notifications", auth(h.Notifications))

	// Admin only
	mux.Handle("GET /portal/notif-config", adminOnly(h.NotifConfig))
	mux.Handle("GET /portal/rbac", adminOnly(h.RBAC))
	mux.Handle("GET /portal/users", adminOnly(h.Users))

	// Admin + auditor
	mux.Handle("GET /portal/audit-log", auditAccess(h.AuditLog))
	mux.Handle("GET /portal/activity", auditAccess(h.Activity))

	// Issuer workspace — feature-flagged
	if cfg.Features.Workspaces.Issuer {
		issuerAccess := func(fn http.HandlerFunc) http.Handler {
			return middleware.AuthRequired(middleware.RequireRole("admin", "issuer")(http.HandlerFunc(fn)))
		}
		mux.Handle("GET /portal/issuer/onboard-form", issuerAccess(h.IssuerOnboardForm))
		mux.Handle("GET /portal/issuer/onboard-queue", issuerAccess(h.IssuerOnboardQueue))
		mux.Handle("GET /portal/issuer/did-keys", issuerAccess(h.IssuerDIDKeys))
		mux.Handle("GET /portal/issuer/schemas", issuerAccess(h.IssuerSchemas))
		mux.Handle("GET /portal/issuer/schema-builder", issuerAccess(h.IssuerSchemaBuilder))
		mux.Handle("GET /portal/issuer/templates", issuerAccess(h.IssuerTemplates))
		mux.Handle("GET /portal/issuer/template-editor", issuerAccess(h.IssuerTemplateEditor))
		mux.Handle("GET /portal/issuer/cred-preview", issuerAccess(h.IssuerCredPreview))
		mux.Handle("GET /portal/issuer/campaign", issuerAccess(h.IssuerCampaign))
		mux.Handle("GET /portal/issuer/batch", issuerAccess(h.IssuerBatch))
		mux.Handle("GET /portal/issuer/issue-review", issuerAccess(h.IssuerReview))
		mux.Handle("GET /portal/issuer/dispatch", issuerAccess(h.IssuerDispatch))
		mux.Handle("GET /portal/issuer/single-issue", issuerAccess(h.IssuerSingleIssue))
		mux.Handle("GET /portal/issuer/issued-creds", issuerAccess(h.IssuerCredentials))
		mux.Handle("GET /portal/issuer/cred-detail", issuerAccess(h.IssuerCredDetail))
		mux.Handle("GET /portal/issuer/revocation", issuerAccess(h.IssuerRevocation))
	}

	// Holder workspace — feature-flagged
	if cfg.Features.Workspaces.Holder {
		holderAccess := func(fn http.HandlerFunc) http.Handler {
			return middleware.AuthRequired(middleware.RequireRole("admin", "holder")(http.HandlerFunc(fn)))
		}
		mux.Handle("GET /portal/holder/wallet", holderAccess(h.HolderWallet))
		mux.Handle("GET /portal/holder/cred-detail", holderAccess(h.HolderCredDetail))
		mux.Handle("GET /portal/holder/claim", holderAccess(h.HolderClaim))
		mux.Handle("GET /portal/holder/dependents", holderAccess(h.HolderDependents))
		mux.Handle("GET /portal/holder/inbox", holderAccess(h.HolderInbox))
		mux.Handle("GET /portal/holder/disclosure", holderAccess(h.HolderDisclosure))
		mux.Handle("GET /portal/holder/presentation", holderAccess(h.HolderPresentation))
		mux.Handle("GET /portal/holder/share", holderAccess(h.HolderShare))
		mux.Handle("GET /portal/holder/catalog", holderAccess(h.HolderCatalog))
		mux.Handle("GET /portal/holder/request-form", holderAccess(h.HolderRequestForm))
		mux.Handle("GET /portal/holder/request-tracker", holderAccess(h.HolderRequestTracker))
		mux.Handle("GET /portal/holder/timeline", holderAccess(h.HolderTimeline))
		mux.Handle("GET /portal/holder/export", holderAccess(h.HolderExport))
	}

	// Verifier workspace — feature-flagged
	if cfg.Features.Workspaces.Verifier {
		verifierAccess := func(fn http.HandlerFunc) http.Handler {
			return middleware.AuthRequired(middleware.RequireRole("admin", "verifier")(http.HandlerFunc(fn)))
		}
		mux.Handle("GET /portal/verifier/verify", verifierAccess(h.VerifierVerify))
		mux.Handle("GET /portal/verifier/request-builder", verifierAccess(h.VerifierRequestBuilder))
		mux.Handle("GET /portal/verifier/qr-generator", verifierAccess(h.VerifierQRGenerator))
		mux.Handle("GET /portal/verifier/portal", verifierAccess(h.VerifierPortal))
		mux.Handle("GET /portal/verifier/dashboard", verifierAccess(h.VerifierDashboard))
		mux.Handle("GET /portal/verifier/proof-detail", verifierAccess(h.VerifierProofDetail))
		mux.Handle("GET /portal/verifier/history", verifierAccess(h.VerifierHistory))
		mux.Handle("GET /portal/verifier/offline", verifierAccess(h.VerifierOffline))
		mux.Handle("GET /portal/verifier/sdk-guide", verifierAccess(h.VerifierSDKGuide))
		mux.Handle("GET /portal/verifier/integration", verifierAccess(h.VerifierIntegration))
	}

	// Trust & Interop workspace — feature-flagged, admin only
	if cfg.Features.Workspaces.Trust {
		mux.Handle("GET /portal/trust/schemas", adminOnly(h.TrustSchemaRegistry))
		mux.Handle("GET /portal/trust/schema-publish", adminOnly(h.TrustSchemaPublish))
		mux.Handle("GET /portal/trust/issuer-directory", adminOnly(h.TrustIssuerDirectory))
		mux.Handle("GET /portal/trust/trust-admin", adminOnly(h.TrustRegistryAdmin))
		mux.Handle("GET /portal/trust/verifier-directory", adminOnly(h.TrustVerifierDirectory))
		mux.Handle("GET /portal/trust/governance", adminOnly(h.TrustGovernance))
		mux.Handle("GET /portal/trust/adaptor-registry", adminOnly(h.TrustAdaptorRegistry))
		mux.Handle("GET /portal/trust/adaptor-config", adminOnly(h.TrustAdaptorConfig))
		mux.Handle("GET /portal/trust/protocol-monitor", adminOnly(h.TrustProtocolMonitor))
		mux.Handle("GET /portal/trust/trust-bridge", adminOnly(h.TrustBridge))
		mux.Handle("GET /portal/trust/schema-harmonize", adminOnly(h.TrustSchemaHarmonize))
		mux.Handle("GET /portal/trust/channel-config", adminOnly(h.TrustChannelConfig))
		mux.Handle("GET /portal/trust/offline-sync", adminOnly(h.TrustOfflineSync))
		mux.Handle("GET /portal/trust/agent-mode", adminOnly(h.TrustAgentMode))
		mux.Handle("GET /portal/trust/connectivity", adminOnly(h.TrustConnectivity))
		mux.Handle("GET /portal/trust/multimodal", adminOnly(h.TrustMultimodal))
	}

	// Platform Admin workspace — feature-flagged, admin only
	if cfg.Features.Workspaces.Admin {
		mux.Handle("GET /portal/admin/intake", adminOnly(h.AdminIntake))
		mux.Handle("GET /portal/admin/guided-schema", adminOnly(h.AdminSchemaWizard))
		mux.Handle("GET /portal/admin/preview-gen", adminOnly(h.AdminPreviewGen))
		mux.Handle("GET /portal/admin/sandbox", adminOnly(h.AdminSandbox))
		mux.Handle("GET /portal/admin/approval-queue", adminOnly(h.AdminApprovalQueue))
		mux.Handle("GET /portal/admin/deployment", adminOnly(h.AdminDeployment))
		mux.Handle("GET /portal/admin/portability", adminOnly(h.AdminPortability))
		mux.Handle("GET /portal/admin/reporting", adminOnly(h.AdminReporting))
		mux.Handle("GET /portal/admin/training", adminOnly(h.AdminTraining))
		mux.Handle("GET /portal/admin/health", adminOnly(h.AdminHealth))
	}

	// JSON API endpoints — for real DPG backend interaction
	mux.Handle("POST /api/issuer/onboard", auth(h.APIIssuerOnboard))
	mux.Handle("POST /api/issuer/issue", auth(h.APIIssueCredential))
	mux.Handle("POST /api/verifier/verify", auth(h.APIVerify))
	mux.Handle("POST /api/verifier/direct-verify", auth(h.APIDirectVerify))
	mux.Handle("GET /api/verifier/session/{state}", auth(h.APIVerifyResult))
	mux.Handle("GET /api/wallet/credentials", auth(h.APIWalletCredentials))
	mux.Handle("POST /api/wallet/claim", auth(h.APIWalletClaim))
	mux.Handle("POST /api/wallet/present", auth(h.APIWalletPresent))
	mux.Handle("GET /api/wallet/dids", auth(h.APIWalletDIDs))
	mux.Handle("GET /api/wallet/export", auth(h.APIExportCredentialJSON))
	mux.Handle("GET /api/wallet/pdf", auth(h.APIWalletPDF))
	mux.Handle("POST /api/wallet/dids/create", auth(h.APICreateDID))
	mux.Handle("POST /api/credential/issue", auth(h.APIIssueCredentialOffer))
	mux.Handle("POST /api/wallet/claim-offer", auth(h.APIWalletClaimOffer))
	mux.Handle("POST /api/wallet/self-issue", auth(h.APIIssueToSelf))
	mux.Handle("POST /api/share/create-session", auth(h.APICreateShareSession))
	mux.Handle("POST /api/share/proactive", auth(h.APIShareProactive))
	mux.HandleFunc("GET /share/v/{id}", h.ShareView)
	mux.HandleFunc("POST /share/v/{id}/verify", h.APIShareVerify)
	mux.Handle("GET /api/schemas", auth(h.APISchemas))
	mux.Handle("POST /api/schemas/create", auth(h.APICreateSchema))
	mux.Handle("GET /api/schemas/list", auth(h.APIListSchemas))
	mux.Handle("GET /api/verifier/policies", auth(h.APIPolicies))
	mux.Handle("GET /api/credential-types", auth(h.APIListCredentialConfigs))
	mux.Handle("POST /api/credential-types/register", auth(h.APIRegisterCredentialType))
	mux.HandleFunc("POST /api/translate", h.APITranslate)
	mux.HandleFunc("GET /api/translate/config", h.APITranslationConfig)
	mux.HandleFunc("GET /api/capabilities", h.APICapabilities)
	mux.HandleFunc("GET /api/datasources", h.APIListDataSources)
	mux.Handle("GET /api/datasources/record", auth(h.APIFetchDataSourceRecord))
	mux.Handle("GET /api/datasources/search", auth(h.APIDataSourceSearch))
	mux.Handle("GET /api/datasources/sample", auth(h.APIDataSourceSample))

	// Issuer onboarding wizard — per-user state machine that captures
	// credential categories, DPG choice, pre-built schemas, and issuance mode.
	mux.Handle("GET /portal/onboarding", auth(h.OnboardingPage))
	mux.Handle("GET /api/onboarding/state", auth(h.APIOnboardingState))
	mux.Handle("GET /api/onboarding/dpgs", auth(h.APIDPGCatalog))
	mux.Handle("POST /api/onboarding/dpg", auth(h.APIOnboardingDPG))
	mux.Handle("POST /api/onboarding/confirm", auth(h.APIOnboardingConfirm))
	mux.Handle("POST /api/onboarding/issuance-mode", auth(h.APIOnboardingIssuanceMode))
	// Pre-built schema catalog (Phase 3) — see schemas.go.
	mux.Handle("GET /api/schemas/catalog", auth(h.APISchemaCatalog))
	mux.Handle("GET /api/schemas/catalog/{id}", auth(h.APISchemaCatalogEntry))
	mux.Handle("POST /api/schemas/catalog/publish", auth(h.APIPublishCatalogSchema))
	// Bulk issuance (Phase 4)
	mux.Handle("GET /portal/issuer/bulk", auth(h.IssuerBulkPage))
	mux.Handle("POST /api/issuer/bulk-csv", auth(h.APIIssueBulkCSV))
	mux.Handle("POST /api/issuer/bulk-api", auth(h.APIIssueBulkAPI))
	mux.Handle("POST /api/issuer/bulk-datasource", auth(h.APIIssueBulkDataSource))

	// Inji proxy — unauthenticated, used by external wallet OID4VCI clients
	// to translate Inji's metadata shape into something Walt.id wallet can parse.
	h.RegisterInjiProxy(mux)

	// Real OID4VCI issuer surface wrapping our URDNA2015 LDP signer.
	// Unauthenticated by design — any wallet (walt.id, inji holder, a real
	// SSI wallet) can run a full Pre-Authorized Code claim flow against it.
	if h.localIssuer != nil {
		h.localIssuer.RegisterRoutes(mux)
	}
}

// pageData builds the standard PageData for a request.
func (h *Handler) pageData(r *http.Request, page string, data any) render.PageData {
	user := middleware.GetUser(r.Context())
	return render.PageData{
		Config:     h.config,
		User:       user,
		Mode:       h.config.Mode,
		IsHTMX:     middleware.IsHTMX(r.Context()),
		ActivePage: page,
		Data:       data,
	}
}
