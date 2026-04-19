package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/verifiably/verifiably-go/backend"
)

// H is the handler struct; holds deps injected from main.
type H struct {
	Adapter   backend.Adapter
	Sessions  *Store
	Templates *template.Template
	Debug     bool // DEBUG_SHOW_MOCK_MARKERS equivalent
}

// isHTMX returns true if the request came from HTMX.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// render executes a template. For full page loads it wraps content_<page>
// inside the "layout" template. For HTMX boost targets it renders just the
// content block so it can replace the <main> element directly.
func (h *H) render(w http.ResponseWriter, r *http.Request, page string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data.ContentTemplate = "content_" + page
	if data.Title == "" {
		data.Title = titleFor(page)
	}
	if data.Crumb == "" {
		data.Crumb = crumbFor(page)
	}

	name := "layout"
	if isHTMX(r) && r.Header.Get("HX-Target") == "main" {
		// Full-page boost into <main>: render content block only
		name = data.ContentTemplate
	}

	if err := h.Templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error (page=%s, name=%s): %v", page, name, err)
		http.Error(w, "internal server error", 500)
	}
}

// renderFragment renders a named sub-template directly (for HTMX partial swaps).
func (h *H) renderFragment(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("fragment error (%s): %v", name, err)
		http.Error(w, "internal server error", 500)
	}
}

// renderFragments renders multiple named sub-templates to the response in order.
// Use when a handler needs to return a primary fragment + one or more hx-swap-oob
// fragments concatenated together.
func (h *H) renderFragments(w http.ResponseWriter, data any, names ...string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for _, name := range names {
		if err := h.Templates.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("fragment error (%s): %v", name, err)
			http.Error(w, "internal server error", 500)
			return
		}
	}
}

// PageData is the common view model passed to page templates.
type PageData struct {
	Title           string
	Crumb           string
	ContentTemplate string
	Debug           bool
	Session         *Session
	Body            any // page-specific sub-data
	FlashToast      string // one-shot toast message via HX-Trigger header alternative
}

func (h *H) pageData(sess *Session, body any) PageData {
	return PageData{
		Debug:   h.Debug,
		Session: sess,
		Body:    body,
	}
}

func titleFor(page string) string {
	return map[string]string{
		"landing":                "",
		"auth":                   "Sign in",
		"issuer_dpg":             "Issuer · DPG",
		"issuer_schema":          "Issuer · Schema",
		"issuer_schema_builder":  "Issuer · Build schema",
		"issuer_mode":            "Issuer · Mode",
		"issuer_issue":           "Issuer · Issue",
		"holder_dpg":             "Holder · Wallet",
		"holder_wallet":          "Wallet",
		"holder_present":         "Present credential",
		"verifier_dpg":           "Verifier · Engine",
		"verifier_verify":        "Verify",
		"redirect_notice":        "Redirect",
	}[page]
}

func crumbFor(page string) string {
	return map[string]string{
		"landing":               "",
		"auth":                  "role → auth",
		"issuer_dpg":            "issuer → dpg",
		"issuer_schema":         "issuer → schema",
		"issuer_schema_builder": "issuer → schema → build",
		"issuer_mode":           "issuer → mode",
		"issuer_issue":          "issuer → issue",
		"holder_dpg":            "holder → wallet",
		"holder_wallet":         "holder → wallet",
		"holder_present":        "holder → present",
		"verifier_dpg":          "verifier → engine",
		"verifier_verify":       "verifier → verify",
		"redirect_notice":       "redirect",
	}[page]
}

// --- Routes ---

func (h *H) Landing(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	sess := h.Sessions.MustGet(w, r)
	h.render(w, r, "landing", h.pageData(sess, nil))
}

// PickRole is POST /role — sets role and redirects to /auth.
func (h *H) PickRole(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	role := r.FormValue("role")
	if role != "issuer" && role != "holder" && role != "verifier" {
		http.Error(w, "invalid role", 400)
		return
	}
	sess.Role = role
	h.redirect(w, r, "/auth")
}

// Auth renders the auth page.
func (h *H) Auth(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.Role == "" {
		h.redirect(w, r, "/")
		return
	}
	h.render(w, r, "auth", h.pageData(sess, nil))
}

// CompleteAuth is POST /auth — stubs auth, routes to role-specific next page.
func (h *H) CompleteAuth(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.Role == "" {
		h.redirect(w, r, "/")
		return
	}
	sess.AuthOK = true

	next := map[string]string{
		"issuer":   "/issuer/dpg",
		"holder":   "/holder/dpg",
		"verifier": "/verifier/dpg",
	}[sess.Role]
	h.redirect(w, r, next)
}

// redirect issues a response appropriate to HTMX vs. plain browser.
// For HTMX we use HX-Redirect so the browser does a full nav; for plain
// requests we issue a 303 See Other.
func (h *H) redirect(w http.ResponseWriter, r *http.Request, to string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// --- DPG selection (shared across roles) ---

// ShowIssuerDpgs / ShowHolderDpgs / ShowVerifierDpgs render the DPG-pick page.
// PickIssuerDpg / PickHolderDpg / PickVerifierDpg handle POSTed selections.

func (h *H) ShowIssuerDpgs(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if !sess.AuthOK || sess.Role != "issuer" {
		h.redirect(w, r, "/")
		return
	}
	dpgs, err := h.Adapter.ListIssuerDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.render(w, r, "issuer_dpg", h.pageData(sess, map[string]any{
		"Dpgs":     dpgs,
		"Expanded": sess.ExpandedIssuerDpg,
	}))
}

// ToggleIssuerDpg expands/collapses a DPG card. Expanding also selects it.
func (h *H) ToggleIssuerDpg(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if !sess.AuthOK || sess.Role != "issuer" {
		h.redirect(w, r, "/")
		return
	}
	vendor := r.FormValue("vendor")
	if sess.ExpandedIssuerDpg == vendor {
		sess.ExpandedIssuerDpg = ""
	} else {
		sess.ExpandedIssuerDpg = vendor
	}
	dpgs, err := h.Adapter.ListIssuerDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragments(w, map[string]any{
		"Dpgs":     dpgs,
		"Expanded": sess.ExpandedIssuerDpg,
	}, "fragment_issuer_dpg_grid", "fragment_issuer_dpg_continue_oob")
}

// PickIssuerDpg commits the currently-expanded DPG and moves forward.
func (h *H) PickIssuerDpg(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.ExpandedIssuerDpg == "" {
		h.errorToast(w, r, "Select a DPG first")
		return
	}
	dpgs, err := h.Adapter.ListIssuerDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	if _, ok := dpgs[sess.ExpandedIssuerDpg]; !ok {
		http.Error(w, "unknown vendor", 400)
		return
	}
	sess.IssuerDpg = sess.ExpandedIssuerDpg
	h.redirect(w, r, "/issuer/schema")
}

func (h *H) ShowHolderDpgs(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if !sess.AuthOK || sess.Role != "holder" {
		h.redirect(w, r, "/")
		return
	}
	dpgs, err := h.Adapter.ListHolderDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.render(w, r, "holder_dpg", h.pageData(sess, map[string]any{
		"Dpgs":     dpgs,
		"Expanded": sess.ExpandedHolderDpg,
	}))
}

func (h *H) ToggleHolderDpg(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if !sess.AuthOK || sess.Role != "holder" {
		h.redirect(w, r, "/")
		return
	}
	vendor := r.FormValue("vendor")
	if sess.ExpandedHolderDpg == vendor {
		sess.ExpandedHolderDpg = ""
	} else {
		sess.ExpandedHolderDpg = vendor
	}
	dpgs, err := h.Adapter.ListHolderDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragments(w, map[string]any{
		"Dpgs":     dpgs,
		"Expanded": sess.ExpandedHolderDpg,
	}, "fragment_holder_dpg_grid", "fragment_holder_dpg_continue_oob")
}

func (h *H) PickHolderDpg(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.ExpandedHolderDpg == "" {
		h.errorToast(w, r, "Select a wallet first")
		return
	}
	dpgs, err := h.Adapter.ListHolderDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	dpg, ok := dpgs[sess.ExpandedHolderDpg]
	if !ok {
		http.Error(w, "unknown vendor", 400)
		return
	}
	sess.HolderDpg = sess.ExpandedHolderDpg
	if dpg.Redirect {
		h.render(w, r, "redirect_notice", h.pageData(sess, dpg))
		return
	}
	h.redirect(w, r, "/holder/wallet")
}

func (h *H) ShowVerifierDpgs(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if !sess.AuthOK || sess.Role != "verifier" {
		h.redirect(w, r, "/")
		return
	}
	dpgs, err := h.Adapter.ListVerifierDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.render(w, r, "verifier_dpg", h.pageData(sess, map[string]any{
		"Dpgs":     dpgs,
		"Expanded": sess.ExpandedVerifierDpg,
	}))
}

func (h *H) ToggleVerifierDpg(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if !sess.AuthOK || sess.Role != "verifier" {
		h.redirect(w, r, "/")
		return
	}
	vendor := r.FormValue("vendor")
	if sess.ExpandedVerifierDpg == vendor {
		sess.ExpandedVerifierDpg = ""
	} else {
		sess.ExpandedVerifierDpg = vendor
	}
	dpgs, err := h.Adapter.ListVerifierDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	h.renderFragments(w, map[string]any{
		"Dpgs":     dpgs,
		"Expanded": sess.ExpandedVerifierDpg,
	}, "fragment_verifier_dpg_grid", "fragment_verifier_dpg_continue_oob")
}

func (h *H) PickVerifierDpg(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.ExpandedVerifierDpg == "" {
		h.errorToast(w, r, "Select a verifier first")
		return
	}
	dpgs, err := h.Adapter.ListVerifierDpgs(r.Context())
	if err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	dpg, ok := dpgs[sess.ExpandedVerifierDpg]
	if !ok {
		http.Error(w, "unknown vendor", 400)
		return
	}
	sess.VerifierDpg = sess.ExpandedVerifierDpg
	if dpg.Redirect {
		h.render(w, r, "redirect_notice", h.pageData(sess, dpg))
		return
	}
	h.redirect(w, r, "/verifier/verify")
}

// errorToast sets the HX-Trigger header so the client shows a toast, and 200s.
// HX-Reswap: none tells HTMX not to swap the target — otherwise the empty
// response body replaces the target's content and the page appears to wipe.
// For non-HTMX it renders a plain error page.
func (h *H) errorToast(w http.ResponseWriter, r *http.Request, msg string) {
	if isHTMX(r) {
		w.Header().Set("HX-Trigger", "toast:"+strings.ReplaceAll(msg, `"`, `'`))
		w.Header().Set("HX-Reswap", "none")
		w.WriteHeader(200)
		return
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

// --- Schema browser + builder (issuer only) ---
//
// Split into separate files for readability: schema.go, issuance.go, wallet.go, verifier.go.
