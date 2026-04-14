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
	"vcplatform/internal/render"
	"vcplatform/internal/store"
)

// IssuerDIDEntry represents a persisted issuer DID from the onboarding flow.
type IssuerDIDEntry struct {
	DID       string `json:"did"`
	KeyType   string `json:"keyType"`
	CreatedAt string `json:"createdAt"`
}

// CredentialSchema represents a credential schema created in the Schema Builder.
type CredentialSchema struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ConfigID  string            `json:"configId"`  // Backend credential configuration ID
	Version   string            `json:"version"`
	Format    string            `json:"format"`    // jwt_vc_json, sdjwt_vc, ldp_vc, mso_mdoc
	Standard  string            `json:"standard"`  // W3C-VCDM 2.0, SD-JWT, etc.
	Fields    []SchemaField     `json:"fields"`
	CreatedAt string            `json:"createdAt"`
}

// SchemaField is a single field in a credential schema.
type SchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// Handler holds shared dependencies for all route handlers.
type Handler struct {
	render      *render.Renderer
	stores      *store.Stores
	config      *config.Config
	ssoRegistry *auth.Registry
	dataSources *datasource.Registry

	// In-memory stores, keyed by user ID.
	// In production these would be persisted to a database.
	issuerDIDsMu sync.RWMutex
	issuerDIDs   map[string][]IssuerDIDEntry

	schemasMu sync.RWMutex
	schemas   map[string][]CredentialSchema
}

// New creates a new Handler.
func New(r *render.Renderer, s *store.Stores, c *config.Config, sso *auth.Registry, ds *datasource.Registry) *Handler {
	if ds == nil {
		ds = datasource.NewRegistry()
	}
	return &Handler{
		render:      r,
		stores:      s,
		config:      c,
		ssoRegistry: sso,
		dataSources: ds,
		issuerDIDs:  make(map[string][]IssuerDIDEntry),
		schemas:     make(map[string][]CredentialSchema),
	}
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
		mux.Handle("GET /portal/holder/retrieval", holderAccess(h.HolderRetrieval))
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
	mux.Handle("POST /api/wallet/dids/create", auth(h.APICreateDID))
	mux.Handle("POST /api/credential/issue", auth(h.APIIssueCredentialOffer))
	mux.Handle("POST /api/wallet/claim-offer", auth(h.APIWalletClaimOffer))
	mux.Handle("POST /api/share/create-session", auth(h.APICreateShareSession))
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

	// Inji proxy — unauthenticated, used by external wallet OID4VCI clients
	// to translate Inji's metadata shape into something Walt.id wallet can parse.
	h.RegisterInjiProxy(mux)
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
