package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

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

// loadProviderConfigs returns the OIDC provider configs, prioritising env
// vars over the JSON file. Three layered configuration mechanisms, in
// precedence order:
//
//  1. VERIFIABLY_OIDC_PROVIDERS — a JSON array passed as a single env var.
//     Replaces the file entirely. Use this to point the whole stack at a
//     different IdP without touching any JSON: one-line update in .env or
//     `docker run -e`. Example:
//
//       VERIFIABLY_OIDC_PROVIDERS='[{"id":"my_idp","type":"oidc",
//         "displayName":"My IdP","issuerUrl":"https://idp.example.com",
//         "clientId":"foo","clientSecret":"bar",
//         "scopes":["openid","profile","email"]}]'
//
//  2. config/auth-providers.json (or VERIFIABLY_AUTH_PROVIDERS_FILE) — the
//     existing file-based mechanism deploy.sh writes for the Keycloak +
//     WSO2IS demo defaults. Used when (1) is empty.
//
//  3. Per-field overrides VERIFIABLY_OIDC_<ID>_{ISSUER_URL,CLIENT_ID,
//     CLIENT_SECRET,DISPLAY_NAME,SCOPES,INSECURE_SKIP_VERIFY} — applied AFTER
//     loading from (1) or (2). Lets an operator swap one provider's issuer
//     to a custom server without re-typing the whole JSON. <ID> is upper-
//     cased and non-alphanumerics become underscores so an id "my-idp"
//     becomes VERIFIABLY_OIDC_MY_IDP_ISSUER_URL. SCOPES is comma-separated.
//
// Returns (configs, sourceLabel) for logging.
func loadProviderConfigs() ([]auth.ProviderConfig, string) {
	if raw := os.Getenv("VERIFIABLY_OIDC_PROVIDERS"); strings.TrimSpace(raw) != "" {
		var cfgs []auth.ProviderConfig
		if err := json.Unmarshal([]byte(raw), &cfgs); err != nil {
			log.Fatalf("auth: VERIFIABLY_OIDC_PROVIDERS is set but not valid JSON: %v", err)
		}
		return applyEnvOverrides(cfgs), "VERIFIABLY_OIDC_PROVIDERS env"
	}
	path := authProvidersFile()
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("auth: no providers file at %s — demo mode", path)
		return applyEnvOverrides(nil), "demo (no providers)"
	}
	var cfgs []auth.ProviderConfig
	if err := json.Unmarshal(b, &cfgs); err != nil {
		log.Fatalf("auth: parse %s: %v", path, err)
	}
	return applyEnvOverrides(cfgs), path
}

// applyEnvOverrides layers per-provider scalar env vars on top of an already-
// loaded config slice. Walks each provider, looks up VERIFIABLY_OIDC_<ID>_*
// env vars, and overrides the matching fields when set. Untouched fields
// keep their loaded value, so an operator can override only what changed
// (typically issuerUrl + client secret) without re-declaring the rest.
func applyEnvOverrides(cfgs []auth.ProviderConfig) []auth.ProviderConfig {
	for i, c := range cfgs {
		prefix := "VERIFIABLY_OIDC_" + envSafeID(c.ID) + "_"
		if v := os.Getenv(prefix + "ISSUER_URL"); v != "" {
			cfgs[i].IssuerURL = v
		}
		if v := os.Getenv(prefix + "PUBLIC_ISSUER_URL"); v != "" {
			cfgs[i].PublicIssuerURL = v
		}
		if v := os.Getenv(prefix + "CLIENT_ID"); v != "" {
			cfgs[i].ClientID = v
		}
		if v := os.Getenv(prefix + "CLIENT_SECRET"); v != "" {
			cfgs[i].ClientSecret = v
		}
		if v := os.Getenv(prefix + "DISPLAY_NAME"); v != "" {
			cfgs[i].DisplayName = v
		}
		if v := os.Getenv(prefix + "KIND"); v != "" {
			cfgs[i].Kind = v
		}
		if v := os.Getenv(prefix + "SCOPES"); v != "" {
			cfgs[i].Scopes = splitCSV(v)
		}
		if v := os.Getenv(prefix + "INSECURE_SKIP_VERIFY"); v != "" {
			cfgs[i].InsecureSkipVerify = v == "1" || strings.EqualFold(v, "true")
		}
	}
	return cfgs
}

// envSafeID upper-cases an id and replaces non-alphanumerics with "_" so
// "my-idp" → "MY_IDP" and slots safely into a VERIFIABLY_OIDC_<ID>_<FIELD>
// env-var name.
func envSafeID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32)
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// splitCSV trims whitespace around each comma-separated entry; empty
// entries are dropped so a trailing comma doesn't produce a blank scope.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// buildAuthRegistry loads providers (env first, then file) and returns a
// registry. Missing config → empty registry (no-OIDC demo mode).
func buildAuthRegistry() *auth.Registry {
	reg := auth.NewRegistry()
	cfgs, source := loadProviderConfigs()
	if len(cfgs) == 0 {
		log.Printf("auth: no providers configured (source=%s)", source)
		return reg
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
			log.Printf("auth: registered provider %q (type=oidc, issuer=%s, source=%s)", c.ID, c.IssuerURL, source)
		default:
			log.Printf("auth: unknown provider type %q — skipping %q", c.Type, c.ID)
		}
	}
	return reg
}

// wireAuthHelpers swaps out the indirection hooks used by handlers to get a
// random state, PKCE verifier, and runtime custom-provider builder.
// Separated here so handlers/ stays free of imports from the oidc
// subpackage.
//
// The build hook backing /auth/custom is just oidc.New wrapped to take
// the handler-package's CustomProviderInput shape — coreos/go-oidc's
// NewProvider does the OIDC discovery, so any URL that doesn't serve
// /.well-known/openid-configuration fails fast inside oidc.New() and the
// handler surfaces the error verbatim as a toast.
func wireAuthHelpers() {
	build := func(_ context.Context, in handlers.CustomProviderInput) (auth.Provider, error) {
		return oidc.New(oidc.Config{
			ID:                 in.ID,
			DisplayName:        in.DisplayName,
			Kind:               "OIDC",
			IssuerURL:          in.IssuerURL,
			ClientID:           in.ClientID,
			ClientSecret:       in.ClientSecret,
			Scopes:             in.Scopes,
			InsecureSkipVerify: in.InsecureSkipVerify,
		})
	}
	handlers.SetOIDCHelpers(oidc.NewState, oidc.NewPKCEVerifier, build)
}
