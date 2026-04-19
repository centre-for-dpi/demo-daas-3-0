package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/verifiably/verifiably-go/internal/auth"
	"github.com/verifiably/verifiably-go/internal/auth/oidc"
	"github.com/verifiably/verifiably-go/internal/handlers"
)

// authProvidersFile is the path to the JSON file listing configured OIDC
// providers. Default points at config/auth-providers.json; override with the
// VERIFIABLY_AUTH_PROVIDERS_FILE env var.
func authProvidersFile() string {
	if v := os.Getenv("VERIFIABLY_AUTH_PROVIDERS_FILE"); v != "" {
		return v
	}
	return "config/auth-providers.json"
}

// buildAuthRegistry loads providers from authProvidersFile() and returns a
// registry. Missing file → empty registry (no-OIDC demo mode).
func buildAuthRegistry() *auth.Registry {
	reg := auth.NewRegistry()
	path := authProvidersFile()
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("auth: no providers file at %s — demo mode", path)
		return reg
	}
	var cfgs []auth.ProviderConfig
	if err := json.Unmarshal(b, &cfgs); err != nil {
		log.Fatalf("auth: parse %s: %v", path, err)
	}
	for _, c := range cfgs {
		switch c.Type {
		case "oidc", "":
			p, err := oidc.New(oidc.Config{
				ID:                 c.ID,
				DisplayName:        c.DisplayName,
				Kind:               c.Kind,
				IssuerURL:          c.IssuerURL,
				PublicIssuerURL:    c.PublicIssuerURL,
				ClientID:           c.ClientID,
				ClientSecret:       c.ClientSecret,
				Scopes:             c.Scopes,
				InsecureSkipVerify: c.InsecureSkipVerify,
			})
			if err != nil {
				log.Fatalf("auth: build %q: %v", c.ID, err)
			}
			reg.Register(p)
			log.Printf("auth: registered provider %q (type=oidc, issuer=%s)", c.ID, c.IssuerURL)
		default:
			log.Printf("auth: unknown provider type %q — skipping %q", c.Type, c.ID)
		}
	}
	return reg
}

// wireAuthHelpers swaps out the indirection hooks used by handlers to get a
// random state and PKCE verifier. Separated here so handlers/ stays free of
// imports from the oidc subpackage.
func wireAuthHelpers() {
	handlers.SetOIDCHelpers(oidc.NewState, oidc.NewPKCEVerifier)
}
