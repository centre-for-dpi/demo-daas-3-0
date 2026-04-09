package handler

import (
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/render"
)

// landingPage is a helper that renders a landing screen template.
func (h *Handler) landingPage(w http.ResponseWriter, r *http.Request, page, tmpl string) {
	data := render.PageData{
		Config:     h.config,
		Mode:       h.config.Mode,
		IsHTMX:     middleware.IsHTMX(r.Context()),
		ActivePage: page,
	}
	if err := h.render.Render(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Landing serves the landing page (screen 78).
func (h *Handler) Landing(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "home", "landing/home")
}

// Explainer serves the VC explainer journey (screen 79).
func (h *Handler) Explainer(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "explainer", "landing/explainer")
}

// Mockup serves the interactive mockup launcher (screens 80-82).
func (h *Handler) Mockup(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "mockup", "landing/mockup")
}

// Tiers serves the tiered package mapper (screen 83).
func (h *Handler) Tiers(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "tiers", "landing/tiers")
}

// Roles serves the role selector (screen 84).
func (h *Handler) Roles(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "roles", "landing/roles")
}

// Auditor serves the auditor dashboard (screen 85).
func (h *Handler) Auditor(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "auditor", "landing/auditor")
}

// AgentOutput serves the agent output viewer (screen 87).
func (h *Handler) AgentOutput(w http.ResponseWriter, r *http.Request) {
	h.landingPage(w, r, "agent-output", "landing/agent_output")
}
