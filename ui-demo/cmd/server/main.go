package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"vcplatform/internal/auth"
	"vcplatform/internal/config"
	"vcplatform/internal/datasource"
	dspg "vcplatform/internal/datasource/postgres"
	dsmanual "vcplatform/internal/datasource/manual"
	"vcplatform/internal/handler"
	"vcplatform/internal/middleware"
	"vcplatform/internal/render"
	"vcplatform/internal/store"
	adapterstore "vcplatform/internal/store/adapter"
	"vcplatform/internal/store/credebl"
	"vcplatform/internal/store/inji"
	"vcplatform/internal/store/injiweb"
	"vcplatform/internal/store/localholder"
	"vcplatform/internal/store/mock"
	"vcplatform/internal/store/pdfwallet"
	"vcplatform/internal/store/waltid"
	"vcplatform/internal/transport"
	"vcplatform/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	configPath := flag.String("config", "config/default.json", "path to config file")
	customDir := flag.String("custom", "custom", "path to custom overrides directory")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v, using defaults\n", err)
		cfg = config.Default()
	}

	renderer, err := render.New(web.StaticFS, web.TemplateFS, *customDir, cfg)
	if err != nil {
		log.Fatalf("render: %v", err)
	}

	stores := initStores(cfg)

	// Initialize SSO registry
	if cfg.SSO.CACertPath != "" || cfg.SSO.SkipTLSVerify {
		auth.InitHTTPClient(cfg.SSO.CACertPath, cfg.SSO.SkipTLSVerify)
	}
	var ssoRegistry *auth.Registry
	if cfg.SSO.Enabled && len(cfg.SSO.Providers) > 0 {
		providers := make([]auth.Provider, len(cfg.SSO.Providers))
		for i, p := range cfg.SSO.Providers {
			clientID := p.ClientID
			// Support auto-resolved client IDs: "auto:/path/to/file"
			if strings.HasPrefix(clientID, "auto:") {
				filePath := strings.TrimPrefix(clientID, "auto:")
				if data, err := os.ReadFile(filePath); err == nil {
					clientID = strings.TrimSpace(string(data))
					fmt.Printf("sso: resolved %s client_id from %s: %s\n", p.Name, filePath, clientID)
				} else {
					fmt.Printf("sso: could not read %s client_id from %s: %v\n", p.Name, filePath, err)
					clientID = ""
				}
			}
			providers[i] = auth.Provider{
				Name:         p.Name,
				DiscoveryURL: p.DiscoveryURL,
				ClientID:     clientID,
				ClientSecret: p.ClientSecret,
				Scopes:       p.Scopes,
			}
		}
		baseURL := cfg.SSO.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost" + *addr
		}
		ssoRegistry = auth.NewRegistry(providers, baseURL)
		ssoRegistry.DiscoverAll(context.Background())
	}

	// Data source registry — data sources are registered per-deployment and are
	// orthogonal to DPG adaptors.
	dataSources := datasource.NewRegistry()
	registerDataSources(dataSources)

	// Cross-DPG OID4VCI metadata proxy: when ISSUER_DPG=inji, our Go proxy
	// exposes /inji-proxy/.well-known/openid-credential-issuer which returns
	// Inji's metadata transformed to a shape Walt.id's strict parser accepts.
	// certify-nginx routes metadata requests through us so wallet-api sees the
	// translated response without changing any upstream URLs. No offer rewrite
	// is needed since credential_issuer stays pointing at certify-nginx.
	_ = inji.SetProxyRewrite // retained for future cases; not activated here.

	h := handler.New(renderer, stores, cfg, ssoRegistry, dataSources)

	// Build per-DPG registries for issuer, wallet, and verifier so the
	// handler can resolve the right store at request time based on the
	// logged-in user's onboarding choice. Every DPG the deployment supports
	// gets an instance. There is no server-wide "mode" — users pick their
	// own backend for each role they have.
	h.SetIssuerRegistry(map[string]store.IssuerStore{
		"waltid":  pickIssuerStore("waltid", cfg),
		"inji":    pickIssuerStore("inji", cfg),
		"credebl": pickIssuerStore("credebl", cfg),
	})
	h.SetWalletRegistry(map[string]store.WalletStore{
		"waltid":   pickWalletStore("waltid", cfg),
		"local":    pickWalletStore("local", cfg),
		"credebl":  pickWalletStore("credebl", cfg),
		"pdf":      pickWalletStore("pdf", cfg),
		"inji_web": pickWalletStore("inji_web", cfg),
	})
	h.SetVerifierRegistry(map[string]store.VerifierStore{
		"waltid":  pickVerifierStore("waltid", cfg),
		"inji":    pickVerifierStore("inji", cfg),
		"adapter": pickVerifierStore("adapter", cfg),
		"credebl": pickVerifierStore("credebl", cfg),
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	staticHandler := web.StaticHandler(*customDir)
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler))

	tokensCSSHandler := render.TokensCSSHandler(cfg)
	mux.HandleFunc("GET /static/css/tokens.css", tokensCSSHandler)

	stack := middleware.Chain(
		middleware.ConfigInjector(cfg),
		middleware.HTMXDetector,
	)

	fmt.Printf("starting server on %s\n", *addr)
	fmt.Printf("mode: %s | brand: %s | backend: %s\n", cfg.Mode, cfg.Brand.Name, backendType(cfg))
	if ssoRegistry != nil {
		fmt.Printf("sso: %d provider(s) configured\n", len(cfg.SSO.Providers))
	}
	log.Fatal(http.ListenAndServe(*addr, stack(mux)))
}

// initStores constructs per-service store implementations. Each service
// (Issuer, Wallet, Verifier) can be backed by a different DPG selected via
// environment variables ISSUER_DPG / WALLET_DPG / VERIFIER_DPG, falling back
// to cfg.Backend.Type when unset.
func initStores(cfg *config.Config) *store.Stores {
	issuerDPG := pickDPG("ISSUER_DPG", cfg.Backend.Type)
	walletDPG := pickDPG("WALLET_DPG", cfg.Backend.Type)
	verifierDPG := pickDPG("VERIFIER_DPG", cfg.Backend.Type)

	stores := &store.Stores{
		Schemas:       newMockSchemaStore(cfg),
		Notifications: newMockNotificationStore(),
		Audit:         newMockAuditStore(),
	}

	// Auth store piggybacks on the wallet DPG (since wallets own user identity).
	stores.Auth = pickAuthStore(walletDPG, cfg)
	stores.Issuer = pickIssuerStore(issuerDPG, cfg)
	stores.Wallet = pickWalletStore(walletDPG, cfg)
	stores.Verifier = pickVerifierStore(verifierDPG, cfg)

	// Hybrid fallback: when the primary verifier is the backend-agnostic
	// adapter, we also pin a walt.id verifier + wallet pair so that credential
	// formats the adapter can't handle (JWT_VC, SD-JWT without x5c) can fall
	// back to walt.id's OID4VP session flow.
	if verifierDPG == "adapter" {
		stores.FallbackVerifier = pickVerifierStore("waltid", cfg)
		stores.FallbackWallet = pickWalletStore("waltid", cfg)
	}

	fmt.Printf("stores: issuer=%s wallet=%s verifier=%s",
		stores.Issuer.Name(), stores.Wallet.Name(), stores.Verifier.Name())
	if stores.FallbackVerifier != nil {
		fmt.Printf(" (fallback: %s)", stores.FallbackVerifier.Name())
	}
	fmt.Println()

	return stores
}

// pickDPG reads a per-service DPG selector from environment, falling back to
// the config-wide Backend.Type.
func pickDPG(envVar, fallback string) string {
	if v := os.Getenv(envVar); v != "" {
		return strings.ToLower(v)
	}
	if fallback != "" {
		return strings.ToLower(fallback)
	}
	return "mock"
}

func pickIssuerStore(dpg string, cfg *config.Config) store.IssuerStore {
	switch dpg {
	case "waltid":
		client := newTransportClient(cfg.Backend.IssuerURL, "", cfg)
		return waltid.NewIssuerStore(client)
	case "inji":
		url := envOr("INJI_CERTIFY_URL", "http://localhost:8090")
		publicURL := envOr("INJI_CERTIFY_PUBLIC_URL", "http://certify-nginx:80")
		client := newTransportClient(url, "", cfg)
		return inji.NewIssuerStore(client, publicURL)
	case "credebl":
		return credebl.NewIssuerStore()
	default:
		return mock.NewIssuerStore()
	}
}

func pickWalletStore(dpg string, cfg *config.Config) store.WalletStore {
	switch dpg {
	case "waltid":
		client := newTransportClient(cfg.Backend.WalletURL, "", cfg)
		return waltid.NewWalletStore(client)
	case "local", "inji":
		// "local" is the canonical name; "inji" kept as a deprecated alias
		// for Trinidad-era demo scripts. Both select our in-process OID4VCI
		// holder in internal/store/localholder — it has nothing to do with
		// MOSIP Inji Web or Inji Mobile.
		client := newTransportClient("", "", cfg)
		return localholder.NewHolderStore(client)
	case "pdf":
		// Print PDF wallet: real WalletStore that runs the OID4VCI flow,
		// then generates an offline-verifiable printable PDF with a
		// PixelPass-encoded self-verifying QR per claimed credential.
		return pdfwallet.New()
	case "inji_web", "injiweb":
		// MOSIP Inji Web — a browser-hosted wallet. ClaimCredential returns
		// a redirect URL the holder opens in a new tab to complete the
		// OID4VCI flow inside Inji Web. Credentials live there, not here.
		return injiweb.New(envOr("INJI_WEB_URL", ""))
	case "credebl":
		return credebl.NewWalletStore()
	default:
		return mock.NewWalletStore()
	}
}

func pickVerifierStore(dpg string, cfg *config.Config) store.VerifierStore {
	switch dpg {
	case "waltid":
		client := newTransportClient(cfg.Backend.VerifierURL, "", cfg)
		return waltid.NewVerifierStore(client)
	case "inji":
		url := envOr("INJI_VERIFY_URL", "http://localhost:8082")
		client := newTransportClient(url, "", cfg)
		return inji.NewVerifierStore(client)
	case "adapter":
		// Backend-agnostic verifier: routes LDP_VC / SD-JWT / JWT credentials
		// through the standalone verification-adapter, which handles URDNA2015
		// canonicalization, DID resolution, and format-specific verification.
		url := envOr("VC_ADAPTER_URL", "http://localhost:8085")
		return adapterstore.New(url)
	case "credebl":
		return credebl.NewVerifierStore()
	default:
		return mock.NewVerifierStore()
	}
}

func pickAuthStore(walletDPG string, cfg *config.Config) store.AuthStore {
	switch walletDPG {
	case "waltid":
		client := newTransportClient(cfg.Backend.WalletURL, "", cfg)
		return waltid.NewAuthStore(client)
	case "local", "inji":
		return localholder.NewAuthStore()
	case "credebl":
		return credebl.NewAuthStore()
	default:
		return mock.NewAuthStore()
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// newMockSchemaStore, newMockNotificationStore, newMockAuditStore:
// these stores are local (not DPG-backed) so we always use the mock package.
func newMockSchemaStore(cfg *config.Config) store.SchemaStore {
	ms := mock.NewStores()
	return ms.Schemas
}
func newMockNotificationStore() store.NotificationStore {
	ms := mock.NewStores()
	return ms.Notifications
}
func newMockAuditStore() store.AuditStore {
	ms := mock.NewStores()
	return ms.Audit
}

func newTransportClient(baseURL, token string, cfg *config.Config) transport.Client {
	switch cfg.Backend.Transport {
	case "n8n", "openfn":
		return transport.NewWebhookClient(cfg.Backend.WebhookURL, "")
	default:
		return transport.NewHTTPClient(baseURL, token)
	}
}

func backendType(cfg *config.Config) string {
	if cfg.Backend.Type != "" {
		return cfg.Backend.Type
	}
	return "mock"
}

// registerDataSources wires real data sources at startup. The default
// deployment registers the Citizens Postgres database (mock government
// registry) and the Manual passthrough. Additional sources can be added
// via config in future iterations.
func registerDataSources(reg *datasource.Registry) {
	// Always register manual entry as a fallback.
	reg.Register(dsmanual.New())

	// Citizens Postgres database — connection via env vars with sensible defaults.
	dsn := envOr("CITIZENS_DB_DSN", "host=localhost port=5435 user=citizens password=citizens dbname=citizens sslmode=disable")
	citizens := dspg.New(dspg.Config{
		DisplayName: "Citizens Database",
		Summary:     "Mock government citizen registry — 200 records (KE + TT) covering birth records, university degrees, and farmer registrations.",
		DSN:         dsn,
		Table:       "citizens",
		PrimaryKey:  "national_id",
		// Columns searched by /api/datasources/search. Free-text ILIKE across
		// all of these, so a user can type "Jelagat" or "KE-NID-8101" or a
		// student ID and get matching rows.
		SearchFields: []string{
			"national_id",
			"first_name",
			"last_name",
			"email",
			"phone",
			"student_id",
			"farm_id",
			"birth_registration_number",
		},
		Fields: []datasource.FieldDescriptor{
			{Name: "national_id", Type: "string", Required: true, Description: "Unique national identifier"},
			{Name: "country_code", Type: "string"},
			{Name: "first_name", Type: "string"},
			{Name: "middle_name", Type: "string"},
			{Name: "last_name", Type: "string"},
			{Name: "gender", Type: "string"},
			{Name: "date_of_birth", Type: "date"},
			{Name: "place_of_birth", Type: "string"},
			{Name: "nationality", Type: "string"},
			{Name: "address", Type: "string"},
			{Name: "phone", Type: "string"},
			{Name: "email", Type: "string"},
			{Name: "birth_registration_number", Type: "string"},
			{Name: "birth_registration_date", Type: "date"},
			{Name: "mother_name", Type: "string"},
			{Name: "father_name", Type: "string"},
			{Name: "university", Type: "string"},
			{Name: "degree_type", Type: "string"},
			{Name: "major", Type: "string"},
			{Name: "graduation_date", Type: "date"},
			{Name: "gpa", Type: "number"},
			{Name: "student_id", Type: "string"},
			{Name: "farm_id", Type: "string"},
			{Name: "farm_location", Type: "string"},
			{Name: "farm_size_hectares", Type: "number"},
			{Name: "primary_crops", Type: "string"},
			{Name: "farm_registration_date", Type: "date"},
		},
		SuggestedMappings: map[string]map[string]string{
			"UniversityDegree": {
				"name":           "first_name",
				"holderName":     "first_name",
				"nationalId":     "national_id",
				"institution":    "university",
				"degree":         "degree_type",
				"major":          "major",
				"graduationDate": "graduation_date",
				"gpa":            "gpa",
				"studentId":      "student_id",
			},
			"FarmerCredential": {
				"fullName":          "first_name",
				"mobileNumber":      "phone",
				"dateOfBirth":       "date_of_birth",
				"gender":            "gender",
				"district":          "place_of_birth",
				"villageOrTown":     "place_of_birth",
				"landArea":          "farm_size_hectares",
				"primaryCropType":   "primary_crops",
				"farmerID":          "farm_id",
			},
			"BirthCertificate": {
				"holderName":         "first_name",
				"nationalId":         "national_id",
				"dateOfBirth":        "date_of_birth",
				"placeOfBirth":       "place_of_birth",
				"gender":             "gender",
				"nationality":        "nationality",
				"motherName":         "mother_name",
				"fatherName":         "father_name",
				"registrationNumber": "birth_registration_number",
				"registrationDate":   "birth_registration_date",
			},
		},
	})
	reg.Register(citizens)
}
