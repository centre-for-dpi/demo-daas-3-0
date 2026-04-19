// verifiably-go — Go + HTMX port of the Verifiable Credentials prototype.
//
// Architecture: this app is deliberately thin. Every handler is small, every
// template is focused, and every piece of fake data lives in internal/mock.
// Swap the backend by implementing the backend.Adapter interface and replacing
// the `mock.NewAdapter()` call below with your own.
//
// See README.md for structure and INTEGRATION.md for endpoint-mapping details.
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
	"strings"

	"github.com/verifiably/verifiably-go/internal/handlers"
	"github.com/verifiably/verifiably-go/internal/mock"
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
	// Replace this line with your own backend.Adapter implementation to go live:
	//     adapter := myadapter.New(apiURL, token)
	adapter := mock.NewAdapter()

	h := &handlers.H{
		Adapter:   adapter,
		Sessions:  handlers.NewStore(),
		Templates: tmpl,
		Debug:     debug,
	}

	mux := http.NewServeMux()

	// Static files
	staticFS := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFS))

	// Landing + auth
	mux.HandleFunc("GET /{$}", h.Landing)
	mux.HandleFunc("POST /role", h.PickRole)
	mux.HandleFunc("GET /auth", h.Auth)
	mux.HandleFunc("POST /auth", h.CompleteAuth)

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
	mux.HandleFunc("POST /holder/present/simulate", h.SimulatePresent)

	// Verifier
	mux.HandleFunc("GET /verifier/dpg", h.ShowVerifierDpgs)
	mux.HandleFunc("POST /verifier/dpg", h.PickVerifierDpg)
	mux.HandleFunc("POST /verifier/dpg/toggle", h.ToggleVerifierDpg)
	mux.HandleFunc("GET /verifier/verify", h.ShowVerify)
	mux.HandleFunc("POST /verifier/verify/request", h.GenerateRequest)
	mux.HandleFunc("POST /verifier/verify/response", h.SimulateResponse)
	mux.HandleFunc("POST /verifier/verify/direct", h.VerifyDirect)

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
	}
}
