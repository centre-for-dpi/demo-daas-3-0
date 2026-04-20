// verifiably-go — Go + HTMX port of the Verifiable Credentials prototype.
//
// Architecture: this app is deliberately thin. Every handler is small, every
// template is focused, and every piece of fake data lives in internal/mock.
// Swap the backend by implementing the backend.Adapter interface and replacing
// the `mock.NewAdapter()` call below with your own.
//
// See README.md + docs/architecture.md for structure and docs/integration.md
// for endpoint-mapping details.
package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verifiably/verifiably-go/internal/adapters/factory"
	"github.com/verifiably/verifiably-go/internal/adapters/registry"
	"github.com/verifiably/verifiably-go/internal/handlers"
	"github.com/verifiably/verifiably-go/vctypes"
)

func main() {
	addr := os.Getenv("VERIFIABLY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	debug := os.Getenv("VERIFIABLY_DEBUG_MOCK_MARKERS") == "1"

	tmpl, err := loadTemplates("templates")
	if err != nil {
		log.Fatalf("template load: %v", err)
	}

	// --- The adapter swap seam ---
	// Set VERIFIABLY_ADAPTER=registry to use live DPG backends declared in
	// config/backends.json; default "mock" keeps the in-memory demo adapter.
	adapter := selectAdapter()

	authReg := buildAuthRegistry()
	wireAuthHelpers()
	translator := buildTranslator()
	h := &handlers.H{
		Adapter:    adapter,
		Sessions:   handlers.NewStore(),
		Templates:  tmpl,
		AuthReg:    authReg,
		Translator: translator,
		Debug:      debug,
	}

	mux := http.NewServeMux()

	// Static files
	staticFS := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFS))

	// Offer-hosting route for adapters that stage credential_offer JSON
	// locally. Dispatches on /offers/{slug}/{id}; adapters store offers and
	// serve them by id through factory.OffersHandler.
	if reg, ok := adapter.(*registry.Registry); ok {
		mux.Handle("/offers/", factory.OffersHandler(reg))
	}

	// Landing + auth
	mux.HandleFunc("GET /{$}", h.Landing)
	mux.HandleFunc("POST /role", h.PickRole)
	mux.HandleFunc("GET /auth", h.Auth)
	mux.HandleFunc("POST /auth", h.CompleteAuth)
	mux.HandleFunc("POST /auth/start", h.StartAuth)
	mux.HandleFunc("GET /auth/callback", h.AuthCallback)
	mux.HandleFunc("GET /lang", h.SetLang)
	mux.HandleFunc("POST /lang", h.SetLang)
	mux.HandleFunc("GET /qr", h.QRImage)

	// Inji Web integration: certify-nginx routes POST /v1/certify/issuance/credential
	// back to us at host.docker.internal:8080/inji-proxy/issuance/credential. We
	// forward straight to inji-certify:8090, patching the request body for wallets
	// that omit credential_definition.@context.
	mux.HandleFunc("POST /inji-proxy/issuance/credential", h.InjiProxyCredential)
	// did:web resolution for did:web:certify-nginx. Inji Certify's upstream
	// did.json advertises a kid that doesn't match the kid its own signer
	// uses, so Inji Verify fails with PublicKeyResolutionFailed. We serve a
	// patched did.json that advertises every kid we've seen signed VCs use.
	mux.HandleFunc("GET /inji-proxy/.well-known/did.json", h.InjiProxyDidJSON)
	// Bitstring status-list credentials are signed with a DIFFERENT kid than
	// the main VC (both derive from the same key, different code paths).
	// Proxy this endpoint too so rememberSigningKids() records the status-list
	// kid and our did.json advertises it before Inji Verify tries to resolve.
	mux.HandleFunc("GET /inji-proxy/credentials/status-list/{id}", h.InjiProxyStatusList)

	// Issuer
	mux.HandleFunc("GET /issuer/dpg", h.ShowIssuerDpgs)
	mux.HandleFunc("POST /issuer/dpg", h.PickIssuerDpg)
	mux.HandleFunc("POST /issuer/dpg/toggle", h.ToggleIssuerDpg)
	mux.HandleFunc("GET /issuer/schema", h.ShowSchemaBrowser)
	mux.HandleFunc("GET /issuer/schema/search", h.SchemaSearch)
	mux.HandleFunc("POST /issuer/schema/filter", h.SetSchemaFilter)
	mux.HandleFunc("POST /issuer/schema/expand", h.ToggleSchemaExpand)
	mux.HandleFunc("POST /issuer/schema/select", h.SelectSchema)
	mux.HandleFunc("POST /issuer/schema/delete", h.DeleteSchema)
	mux.HandleFunc("GET /issuer/schema/build", h.ShowSchemaBuilder)
	mux.HandleFunc("POST /issuer/schema/build/preview", h.SchemaPreview)
	mux.HandleFunc("POST /issuer/schema/build/add-field", h.AddSchemaField)
	mux.HandleFunc("POST /issuer/schema/build/remove-field", h.RemoveSchemaField)
	mux.HandleFunc("POST /issuer/schema/build/save", h.SaveSchema)
	mux.HandleFunc("GET /issuer/mode", h.ShowIssuanceMode)
	mux.HandleFunc("POST /issuer/mode", h.SetIssuanceMode)
	mux.HandleFunc("GET /issuer/issue", h.ShowIssue)
	mux.HandleFunc("POST /issuer/issue", h.SubmitIssue)
	mux.HandleFunc("POST /issuer/issue/source", h.SetSingleSource)
	mux.HandleFunc("POST /issuer/issue/csv", h.SimulateCSV)
	mux.HandleFunc("POST /issuer/issue/preview-pdf", h.PreviewPDF)

	// Holder / Wallet
	mux.HandleFunc("GET /holder/dpg", h.ShowHolderDpgs)
	mux.HandleFunc("POST /holder/dpg", h.PickHolderDpg)
	mux.HandleFunc("POST /holder/dpg/toggle", h.ToggleHolderDpg)
	mux.HandleFunc("GET /holder/wallet", h.ShowWallet)
	mux.HandleFunc("POST /holder/wallet/scan", h.ScanOffer)
	mux.HandleFunc("POST /holder/wallet/paste", h.PasteOffer)
	mux.HandleFunc("POST /holder/wallet/example", h.PrefillExample)
	mux.HandleFunc("POST /holder/wallet/accept", h.AcceptCred)
	mux.HandleFunc("POST /holder/wallet/reject", h.RejectCred)
	mux.HandleFunc("GET /holder/present", h.ShowPresent)
	mux.HandleFunc("POST /holder/present/confirm", h.ConfirmPresent)
	mux.HandleFunc("POST /holder/present/submit", h.SubmitPresent)
	mux.HandleFunc("POST /holder/present/decline", h.DeclinePresent)
	mux.HandleFunc("POST /holder/wallet/delete", h.DeleteCredential)

	// Verifier
	mux.HandleFunc("GET /verifier/dpg", h.ShowVerifierDpgs)
	mux.HandleFunc("POST /verifier/dpg", h.PickVerifierDpg)
	mux.HandleFunc("POST /verifier/dpg/toggle", h.ToggleVerifierDpg)
	mux.HandleFunc("GET /verifier/verify", h.ShowVerify)
	mux.HandleFunc("POST /verifier/verify/request", h.GenerateRequest)
	mux.HandleFunc("POST /verifier/verify/response", h.SimulateResponse)
	mux.HandleFunc("POST /verifier/verify/direct", h.VerifyDirect)
	mux.HandleFunc("POST /verifier/verify/build", h.BuildVerifierTemplate)

	log.Printf("verifiably-go listening on %s (debug markers: %v)", addr, debug)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// loadTemplates walks templates/ and parses every *.html file into a single tree
// with template names matching their {{define}} directives.
func loadTemplates(root string) (*template.Template, error) {
	var tmpl *template.Template
	fns := funcMap()
	// render lets the layout dispatch to a content sub-template by name
	// (html/template's built-in {{template}} action requires a constant name).
	fns["render"] = func(name string, data any) (template.HTML, error) {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
			return "", err
		}
		return template.HTML(buf.String()), nil
	}
	tmpl = template.New("").Funcs(fns)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = tmpl.Parse(string(b))
		return err
	})
	return tmpl, err
}

// funcMap exposes small helpers to templates. Kept minimal — if this grows
// past a dozen entries, move to its own file.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"titleIf": func(cond bool, s string) string {
			if cond {
				return s
			}
			return ""
		},
		"hasPrefix":         strings.HasPrefix,
		"replaceUnderscore": func(s string) string { return strings.ReplaceAll(s, "_", " ") },
		"trimPrefix":        strings.TrimPrefix,

		// dict builds a map[string]any from alternating key/value args so templates
		// can pass multiple named params into sub-templates.
		// Usage: {{template "partial" (dict "K1" v1 "K2" v2)}}
		// t is the translation helper bound at parse time. Takes (text, lang)
		// — the current lang is passed in via `$.Lang` in templates.
		// handlers.TranslateFunc looks up the request-scoped translator +
		// context via package state set in handlers.(*H).render before
		// template execution.
		"t": handlers.TranslateFunc,

		// hasCapability returns true if the given DPG declares a capability
		// with the given Kind+Key. Templates use it to hide flow-specific UI
		// surfaces when the backing DPG doesn't support them, e.g. hiding the
		// "paste credential" card on a verifier that has no direct-verify
		// endpoint.
		"hasCapability": func(dpg vctypes.DPG, kind, key string) bool {
			for _, c := range dpg.Capabilities {
				if c.Kind == kind && c.Key == key {
					return true
				}
			}
			return false
		},

		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict requires even number of args, got %d", len(pairs))
			}
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict key at position %d is not a string", i)
				}
				m[key] = pairs[i+1]
			}
			return m, nil
		},

		// deref dereferences a pointer-to-struct so templates can feed the
		// value into sub-template calls. Returns the zero value if nil.
		"deref": func(p *vctypes.OID4VPTemplate) vctypes.OID4VPTemplate {
			if p == nil {
				return vctypes.OID4VPTemplate{}
			}
			return *p
		},

		// indexSchemas looks up a schema by ID (or variant id) in a slice.
		// After the grouped-by-name refactor, the primary Schema.ID only
		// matches the default variant; non-default variant ids live on the
		// Variants slice, so match against either and return the schema
		// with ID+Std swapped to the picked variant.
		"indexSchemas": func(schemas []vctypes.Schema, id string) vctypes.Schema {
			for _, s := range schemas {
				if s.HasVariantID(id) {
					return s.ApplyVariant(id)
				}
			}
			return vctypes.Schema{}
		},

		// fieldSet returns a lookup map from a []string so templates can
		// use {{index $.Selected .Name}} without iterating the slice per field.
		"fieldSet": func(xs []string) map[string]bool {
			out := make(map[string]bool, len(xs))
			for _, x := range xs {
				out[x] = true
			}
			return out
		},

		// schemaStdList returns the distinct Std values across a []Schema
		// slice in stable sorted order. Used to render the filter chips
		// above the verifier's schema picker without hardcoding format
		// names in the template.
		"schemaStdList": func(schemas []vctypes.Schema) []string {
			seen := map[string]struct{}{}
			for _, s := range schemas {
				if s.Std != "" {
					seen[s.Std] = struct{}{}
				}
			}
			out := make([]string, 0, len(seen))
			for k := range seen {
				out = append(out, k)
			}
			sort.Strings(out)
			return out
		},

		// lowerStr lowercases a string so a data-attribute can hold a
		// pre-normalised search corpus — the client-side filter does a
		// case-insensitive substring match without per-option work.
		"lowerStr": strings.ToLower,

		// uniqueTitles returns the distinct Title values across a credential
		// list, sorted alphabetically — used to populate the wallet's
		// type-filter dropdown.
		"uniqueTitles": func(creds []vctypes.Credential) []string {
			seen := map[string]bool{}
			out := []string{}
			for _, c := range creds {
				if c.Title != "" && !seen[c.Title] {
					seen[c.Title] = true
					out = append(out, c.Title)
				}
			}
			sort.Strings(out)
			return out
		},
	}
}
