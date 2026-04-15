package handler

import (
	"net/http"
	"time"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/render"
)

func (h *Handler) LoginForm(w http.ResponseWriter, r *http.Request) {
	// Clear any existing session — visiting /login means the user wants to re-authenticate
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	role := r.URL.Query().Get("role")
	if role == "" {
		role = "holder"
	}
	ssoProviders := []string{}
	ssoEnabled := false
	if h.ssoRegistry != nil {
		ssoProviders = h.ssoRegistry.Names()
		ssoEnabled = len(ssoProviders) > 0
	}

	data := render.PageData{
		Config: h.config,
		Mode:   h.config.Mode,
		IsHTMX: middleware.IsHTMX(r.Context()),
		Data: map[string]any{
			"role":         role,
			"backendType":  h.config.Backend.Type,
			"ssoEnabled":   ssoEnabled,
			"ssoProviders": ssoProviders,
		},
	}
	if err := h.render.Render(w, "auth/login", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	role := r.FormValue("role")
	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")
	loginType := r.FormValue("login_type")   // "demo" or "real"
	authAction := r.FormValue("auth_action") // "signin" or "signup"
	if role == "" {
		role = "holder"
	}
	if name == "" {
		name = "Demo User"
	}

	// DEMO login — no backend auth, mock data
	if loginType == "demo" || (email == "" && password == "") {
		cookieVal := model.EncodeSession(role, name, true, "")
		h.setSessionCookie(w, cookieVal)
		http.Redirect(w, r, "/portal", http.StatusSeeOther)
		return
	}

	// SIGNUP — register first, then login. Fresh signups land in the
	// onboarding wizard so they can pick their backend DPG (and, for
	// issuers, credential categories + starter schemas + issuance mode).
	if authAction == "signup" {
		err := h.stores.Auth.Register(r.Context(), name, email, password)
		if err != nil {
			h.renderLoginWithError(w, r, role, "Registration failed. This backend may not support email registration.")
			return
		}
		session, err := h.stores.Auth.Login(r.Context(), email, password)
		if err != nil {
			h.renderLoginWithError(w, r, role, "Account created but sign in failed. Try signing in manually.")
			return
		}
		if name == "" || name == "Demo User" {
			name = email
		}
		// Every role now starts at DPG choice — the credential-categories
		// step was removed because nothing downstream consumed the picked
		// categories.
		cookieVal := model.EncodeSessionFull(role, name, email, false, session.Token, "", "dpg-choice")
		h.setSessionCookie(w, cookieVal)
		// All roles now go through the onboarding wizard — the wizard
		// adapts its steps to the user's role.
		http.Redirect(w, r, "/portal/onboarding", http.StatusSeeOther)
		return
	}

	// SIGNIN — authenticate against existing account, do NOT auto-register
	session, err := h.stores.Auth.Login(r.Context(), email, password)
	if err != nil {
		h.renderLoginWithError(w, r, role, "Sign in failed. Check your email and password, or sign up for a new account.")
		return
	}

	if name == "" || name == "Demo User" {
		name = email
	}

	cookieVal := model.EncodeSession(role, name, false, session.Token)
	h.setSessionCookie(w, cookieVal)
	http.Redirect(w, r, "/portal", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) renderLoginWithError(w http.ResponseWriter, r *http.Request, role, errMsg string) {
	ssoProviders := []string{}
	ssoEnabled := false
	if h.ssoRegistry != nil {
		ssoProviders = h.ssoRegistry.Names()
		ssoEnabled = len(ssoProviders) > 0
	}

	w.WriteHeader(http.StatusUnauthorized)
	data := render.PageData{
		Config: h.config,
		Mode:   h.config.Mode,
		IsHTMX: middleware.IsHTMX(r.Context()),
		Data: map[string]any{
			"role":         role,
			"backendType":  h.config.Backend.Type,
			"ssoEnabled":   ssoEnabled,
			"ssoProviders": ssoProviders,
			"error":        errMsg,
		},
	}
	h.render.Render(w, "auth/login", data)
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(24 * time.Hour / time.Second),
	})
}
