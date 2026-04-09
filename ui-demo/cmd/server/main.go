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
	"vcplatform/internal/handler"
	"vcplatform/internal/middleware"
	"vcplatform/internal/render"
	"vcplatform/internal/store"
	"vcplatform/internal/store/mock"
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

	h := handler.New(renderer, stores, cfg, ssoRegistry)

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

func initStores(cfg *config.Config) *store.Stores {
	switch cfg.Backend.Type {
	case "waltid":
		issuerClient := newTransportClient(cfg.Backend.IssuerURL, "", cfg)
		verifierClient := newTransportClient(cfg.Backend.VerifierURL, "", cfg)
		walletClient := newTransportClient(cfg.Backend.WalletURL, "", cfg)
		return waltid.NewStores(
			waltid.Config{
				IssuerURL:   cfg.Backend.IssuerURL,
				VerifierURL: cfg.Backend.VerifierURL,
				WalletURL:   cfg.Backend.WalletURL,
			},
			issuerClient, verifierClient, walletClient,
		)
	default:
		return mock.NewStores()
	}
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
