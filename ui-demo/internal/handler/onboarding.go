package handler

// onboarding.go — handlers for the issuer onboarding wizard.
//
// The wizard walks a new issuer through: credential categories →
// DPG choice → confirm → schema catalog → issuance mode.  State lives in
// h.onboarding (an in-memory store); the current step is mirrored in the
// session cookie so the middleware can redirect mid-flow.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/onboarding"
	"vcplatform/internal/render"
)

// -----------------------------------------------------------------------------
// Static metadata — DPG catalog for the UI cards.
// -----------------------------------------------------------------------------

// DPGCard is a single DPG entry the issuer can pick from.
type DPGCard struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Tagline         string   `json:"tagline"`
	Formats         []string `json:"formats"`
	IssuanceFlows   []string `json:"issuanceFlows"`
	BulkIssuance    string   `json:"bulkIssuance"` // "api" | "dataProvider" | "both" | "none"
	Beta            bool     `json:"beta"`
	Description     string   `json:"description"`
}

// dpgCatalogByRole holds the canonical DPG card lists per onboarding role.
// Each card is verified against the corresponding store registry before
// being shown to the user, so a deployment can disable a DPG just by not
// registering it.
var dpgCatalogByRole = map[string][]DPGCard{
	"issuer": {
		{
			ID:      "waltid",
			Name:    "Walt.id",
			Tagline: "OID4VCI + OID4VP enterprise suite",
			Formats: []string{"jwt_vc_json", "vc+sd-jwt", "ldp_vc", "mso_mdoc"},
			IssuanceFlows: []string{
				"issuer-initiated (offer → claim)",
				"holder-initiated (wallet → issuer)",
			},
			BulkIssuance: "api",
			Description: "Full OID4VCI + OID4VP stack with Issuer, Verifier, and " +
				"Wallet APIs. LDP_VC credentials are signed in-process using " +
				"URDNA2015 + Ed25519Signature2020.",
		},
		{
			ID:      "inji",
			Name:    "Inji Certify (Auth Code flow, via Inji Web)",
			Tagline: "MOSIP Inji Certify wired for OAuth2 Authorization Code flow through esignet",
			Formats: []string{"ldp_vc", "vc+sd-jwt", "mso_mdoc"},
			IssuanceFlows: []string{
				"Authorization Code flow — holder logs in via esignet (OTP) and redirects back through Mimoto",
			},
			BulkIssuance: "dataProvider",
			Description: "Pick this if you want the real end-to-end Inji Web + " +
				"Mimoto + esignet login experience: the holder clicks your " +
				"issuer in the Inji Web catalog, authenticates with an OTP at " +
				"esignet, and the credential flows back into their browser-hosted " +
				"wallet. This instance's credential endpoint only accepts Bearer " +
				"tokens signed by esignet, so pasted-offer / Pre-Auth flows won't " +
				"work against it — use the sibling \"Inji Certify (Pre-Auth flow)\" " +
				"option below for those.",
		},
		{
			ID:      "inji_preauth",
			Name:    "Inji Certify (Pre-Auth flow, for pasted offers)",
			Tagline: "MOSIP Inji Certify wired for the OID4VCI Pre-Authorized Code flow",
			Formats: []string{"ldp_vc", "vc+sd-jwt", "mso_mdoc"},
			IssuanceFlows: []string{
				"Pre-Authorized Code flow — holder pastes the offer URL into a wallet and claims without a login step",
			},
			BulkIssuance: "dataProvider",
			Description: "A separate Inji Certify deployment running in its own " +
				"container with zero shared state (different Postgres database, " +
				"different PKCS12 keystore). Two pieces of config make the " +
				"Pre-Authorized Code flow actually work end to end on this " +
				"instance: (1) it trusts its OWN JWKS at the credential " +
				"endpoint, so the Bearer token Inji hands out at /oauth/token " +
				"is accepted; (2) it runs the upstream PreAuthDataProviderPlugin " +
				"instead of MockCSVDataProviderPlugin, so the staged claims " +
				"posted via /pre-authorized-data are pulled from Inji's session " +
				"cache at credential-issuance time — no CSV lookup, no " +
				"individualId resolution. Pick this if you're testing the PDF " +
				"wallet, the in-process Local holder, or any other wallet that " +
				"claims from a pasted OID4VCI offer URL. The \"Auth Code flow\" " +
				"sibling above stays routed through esignet for the Inji Web " +
				"experience — neither instance interferes with the other.",
		},
		{
			ID:      "credebl",
			Name:    "Credebl Agent",
			Tagline: "Sovrin / did:indy ecosystem (beta)",
			Formats: []string{"ldp_vc"},
			IssuanceFlows: []string{
				"issuer-initiated only",
			},
			BulkIssuance: "api",
			Beta:         true,
			Description: "Credebl agent (beta). Issuer-initiated only for now; " +
				"data-provider bulk support planned for v2.",
		},
	},
	"holder": {
		{
			ID:      "waltid",
			Name:    "Walt.id Wallet",
			Tagline: "Walt.id wallet-api (persistent HTTP wallet)",
			Formats: []string{"jwt_vc_json", "vc+sd-jwt", "mso_mdoc"},
			IssuanceFlows: []string{
				"OID4VCI claim + OID4VP present",
			},
			Description: "Full wallet-api backend with persistent credential " +
				"storage, DID management, and OID4VP presentations. Best for " +
				"JWT-format credentials.",
		},
		{
			ID:      "local",
			Name:    "In-Process Holder",
			Tagline: "Go OID4VCI client (demo)",
			Formats: []string{"ldp_vc", "vc+sd-jwt", "jwt_vc_json"},
			IssuanceFlows: []string{
				"OID4VCI claim (Pre-Authorized Code flow)",
			},
			Description: "A Go struct inside this server that acts as a holder. " +
				"Holds credentials in an in-memory bag, runs a real OID4VCI " +
				"Pre-Auth client. Demo-only — credentials are lost on restart.",
		},
		{
			ID:      "inji_web",
			Name:    "Inji Web",
			Tagline: "MOSIP browser-hosted wallet (catalog-initiated)",
			Formats: []string{"ldp_vc", "vc+sd-jwt", "mso_mdoc"},
			IssuanceFlows: []string{
				"Catalog-driven OID4VCI via Mimoto BFF",
			},
			Description: "MOSIP Inji Web — the real upstream browser wallet (injistack/inji-web + " +
				"injistack/mimoto containers defined under docker/injiweb/). Inji Web is " +
				"catalog-initiated: it loads its own issuer list from mimoto-issuers-config.json " +
				"and runs the OID4VCI exchange itself. It does NOT accept external credential_offer " +
				"URLs — so when you claim a credential here we redirect you to Inji Web's /issuers " +
				"page, where you pick the issuer and complete the flow. Credentials live inside " +
				"Mimoto, not on this server.",
		},
		{
			ID:      "credebl",
			Name:    "Credebl Wallet",
			Tagline: "Sovrin / did:indy ecosystem (beta)",
			Formats: []string{"ldp_vc"},
			IssuanceFlows: []string{
				"AIP2 issue/present",
			},
			Beta: true,
			Description: "Credebl wallet agent (beta). Limited to AnonCreds/LDP credentials in v1.",
		},
		{
			ID:      "pdf",
			Name:    "Print PDF Wallet",
			Tagline: "Offline, self-verifying printable credential",
			Formats: []string{"ldp_vc", "jwt_vc_json", "vc+sd-jwt"},
			IssuanceFlows: []string{
				"OID4VCI claim → printable PDF + self-verifying QR",
			},
			Description: "Real WalletStore that runs the OID4VCI claim flow, " +
				"then generates a PDF containing the human-readable claims and " +
				"a PixelPass-encoded QR (base45(zlib(credJSON))) that any offline " +
				"verifier can decode and cryptographically check without contacting " +
				"the issuer. Best for holders without a smartphone or in low-connectivity " +
				"settings. Formats that don't fit in a single QR surface a clear error " +
				"at claim time with suggested alternatives.",
		},
	},
	"verifier": {
		{
			ID:      "waltid",
			Name:    "Walt.id Verifier",
			Tagline: "OID4VP session-based verifier",
			Formats: []string{"jwt_vc_json", "vc+sd-jwt", "mso_mdoc"},
			IssuanceFlows: []string{
				"OID4VP session + presentation submission",
			},
			Description: "OID4VP-compliant verifier with presentation definitions, " +
				"QR code flows, and policy engine. Best for JWT VCs.",
		},
		{
			ID:      "inji",
			Name:    "Inji Verify",
			Tagline: "MOSIP direct-verify endpoint",
			Formats: []string{"ldp_vc", "vc+sd-jwt"},
			IssuanceFlows: []string{
				"Direct POST of credential (no session)",
			},
			Description: "MOSIP Inji Verify. Accepts a credential via a single " +
				"POST and returns SUCCESS/INVALID. Best for LDP_VC and SD-JWT-x5c.",
		},
		{
			ID:      "adapter",
			Name:    "Verification Adapter",
			Tagline: "Backend-agnostic verifier (URDNA2015 + routing)",
			Formats: []string{"ldp_vc", "vc+sd-jwt", "jwt_vc_json"},
			IssuanceFlows: []string{
				"Direct + OID4VP fallback",
			},
			Description: "Go verification adapter that does its own URDNA2015 " +
				"+ Ed25519 verification for LDP_VC, routes SD-JWT by x5c, and " +
				"falls back to walt.id OID4VP for JWT VCs. Supports true air-gap.",
		},
		{
			ID:      "credebl",
			Name:    "Credebl Verifier",
			Tagline: "Sovrin / did:indy ecosystem (beta)",
			Formats: []string{"ldp_vc"},
			IssuanceFlows: []string{
				"AIP2 verify",
			},
			Beta:        true,
			Description: "Credebl verifier agent (beta).",
		},
	},
}

// -----------------------------------------------------------------------------
// State helpers.
// -----------------------------------------------------------------------------

// getOrCreateState loads the onboarding state for the given user, creating
// a fresh one if none exists yet. For new states, the initial step depends
// on the user's role: issuers start at Categories (they pick credential
// types first), holders and verifiers start directly at DPG choice.
func (h *Handler) getOrCreateState(user *model.User) *onboarding.State {
	if user == nil {
		return nil
	}
	if s := h.onboarding.Get(user.ID); s != nil {
		if s.Role == "" {
			s.Role = user.Role
			h.onboarding.Put(s)
		}
		return s
	}
	s := &onboarding.State{
		UserID: user.ID,
		Role:   user.Role,
		Step:   onboarding.StepDPGChoice,
	}
	h.onboarding.Put(s)
	return s
}

// persistState writes the state back AND mirrors all DPG choices + current
// step into the session cookie so middleware and subsequent requests see
// the user's backend preferences immediately.
func (h *Handler) persistState(w http.ResponseWriter, user *model.User, state *onboarding.State) {
	h.onboarding.Put(state)

	// Update the user struct so in-flight requests see the choices.
	user.IssuerDPG = state.IssuerDPG
	user.WalletDPG = state.WalletDPG
	user.VerifierDPG = state.VerifierDPG
	user.OnboardingStep = state.Step

	cookieVal := model.EncodeSessionFromUser(user)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    cookieVal,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(24 * time.Hour / time.Second),
	})
}

// -----------------------------------------------------------------------------
// GET /portal/onboarding
// -----------------------------------------------------------------------------

// OnboardingPage renders the wizard. The template dispatches on the current
// step stored in the user's state.
func (h *Handler) OnboardingPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	state := h.getOrCreateState(user)

	// Jump to an explicit step if the caller asked for one (e.g. the
	// "Back" buttons).
	if q := r.URL.Query().Get("step"); q != "" {
		state.Step = q
		h.persistState(w, user, state)
	}

	data := render.PageData{
		Config:     h.config,
		User:       user,
		Mode:       h.config.Mode,
		IsHTMX:     middleware.IsHTMX(r.Context()),
		ActivePage: "issuer-onboarding",
		Data: map[string]any{
			"state":       state,
			"role":        state.Role,
			"dpgCards":    h.filteredDPGCatalog(state.Role),
			"currentStep": state.Step,
		},
	}
	if err := h.render.Render(w, "onboarding/wizard", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// filteredDPGCatalog returns DPG cards for the given role, filtered to
// only those the deployment has actually registered a store for.
func (h *Handler) filteredDPGCatalog(role string) []DPGCard {
	cards := dpgCatalogByRole[role]
	if len(cards) == 0 {
		// Unknown role → default to issuer cards for safety.
		cards = dpgCatalogByRole["issuer"]
	}
	var registryHas func(string) bool
	switch role {
	case "holder":
		registryHas = func(id string) bool { _, ok := h.walletRegistry[id]; return ok }
	case "verifier":
		registryHas = func(id string) bool { _, ok := h.verifierRegistry[id]; return ok }
	default:
		registryHas = func(id string) bool { _, ok := h.issuerRegistry[id]; return ok }
	}
	out := make([]DPGCard, 0, len(cards))
	for _, c := range cards {
		if registryHas(c.ID) {
			out = append(out, c)
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// JSON POST handlers — one per wizard step.
// -----------------------------------------------------------------------------

// APIOnboardingDPG saves the user's DPG choice for whichever role they're
// onboarding as (issuer / holder / verifier), and advances to confirm.
func (h *Handler) APIOnboardingDPG(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		DPG string `json:"dpg"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Validate the DPG against the registry that matches the user's role.
	state := h.getOrCreateState(user)
	var (
		capsAny any
		name    string
	)
	switch state.Role {
	case "holder":
		if _, ok := h.walletRegistry[req.DPG]; !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown wallet DPG: " + req.DPG})
			return
		}
		state.WalletDPG = req.DPG
		user.WalletDPG = req.DPG
		if w := h.walletFor(user); w != nil {
			capsAny = w.Capabilities()
			name = w.Name()
		}
	case "verifier":
		if _, ok := h.verifierRegistry[req.DPG]; !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown verifier DPG: " + req.DPG})
			return
		}
		state.VerifierDPG = req.DPG
		user.VerifierDPG = req.DPG
		if v := h.verifierFor(user); v != nil {
			capsAny = v.Capabilities()
			name = v.Name()
		}
	default: // issuer, admin
		if _, ok := h.issuerRegistry[req.DPG]; !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown issuer DPG: " + req.DPG})
			return
		}
		state.IssuerDPG = req.DPG
		user.IssuerDPG = req.DPG
		if i := h.issuerFor(user); i != nil {
			capsAny = i.Capabilities()
			name = i.Name()
		}
	}
	// If the user already finished onboarding and is just switching DPG
	// from their workspace (e.g. holder changing wallet backend on the
	// wallet page), keep their step at Done so we don't rewind the wizard.
	if state.Step != onboarding.StepDone {
		state.Step = onboarding.StepDPGConfirm
	}
	h.persistState(w, user, state)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"next":         state.Step,
		"dpg":          req.DPG,
		"role":         state.Role,
		"capabilities": capsAny,
		"name":         name,
	})
}

// APIOnboardingConfirm is called when the user clicks "Confirm & Continue"
// on the DPG confirmation screen.
//
//   - Issuers proceed to the schema catalog step (to pick + edit starter schemas).
//   - Holders and verifiers are done right here — their onboarding is just
//     "pick a backend", and the portal takes over from there.
func (h *Handler) APIOnboardingConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	state := h.getOrCreateState(user)

	// Verify the user has picked the DPG appropriate to their role.
	var chosen string
	switch state.Role {
	case "holder":
		chosen = state.WalletDPG
	case "verifier":
		chosen = state.VerifierDPG
	default:
		chosen = state.IssuerDPG
	}
	if chosen == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pick a DPG first"})
		return
	}
	state.Confirmed = true

	// Issuers continue to schema catalog; everyone else is done.
	redirect := ""
	if state.Role == "issuer" || state.Role == "admin" {
		state.Step = onboarding.StepSchemaCatalog
	} else {
		state.Step = onboarding.StepDone
		if state.Role == "holder" {
			redirect = "/portal/holder/wallet"
		} else if state.Role == "verifier" {
			redirect = "/portal/verifier/verify"
		} else {
			redirect = "/portal/dashboard"
		}
	}
	h.persistState(w, user, state)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"next":     state.Step,
		"redirect": redirect,
	})
}

// APIOnboardingIssuanceMode is called when the user picks single vs bulk
// at the final step of the wizard. Marks onboarding done and returns the
// redirect target.
func (h *Handler) APIOnboardingIssuanceMode(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	mode := strings.ToLower(req.Mode)
	if mode != "single" && mode != "bulk" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be 'single' or 'bulk'"})
		return
	}
	state := h.getOrCreateState(user)
	state.IssuanceMode = mode
	state.Step = onboarding.StepDone
	h.persistState(w, user, state)

	target := "/portal/issuer/single-issue"
	if mode == "bulk" {
		target = "/portal/issuer/bulk"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"redirect": target,
	})
}

// APIOnboardingState returns the current state as JSON so the wizard's JS
// can render the right step on refresh without a full page reload.
func (h *Handler) APIOnboardingState(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	state := h.getOrCreateState(user)
	writeJSON(w, http.StatusOK, map[string]any{
		"state":    state,
		"dpgCards": h.filteredDPGCatalog(state.Role),
	})
}

// APIDPGCatalog returns the filtered DPG card list for the user's role
// with live capability info from each store. Called from the DPG choice
// step to render cards.
func (h *Handler) APIDPGCatalog(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	role := "issuer"
	if user != nil {
		role = user.Role
	}
	cards := h.filteredDPGCatalog(role)
	// Enrich with live capability info from the actual store instance.
	type enriched struct {
		DPGCard
		LiveFormats   []string `json:"liveFormats,omitempty"`
		SupportsBatch bool     `json:"supportsBatch"`
		SupportsSD    bool     `json:"supportsSelectiveDisclosure"`
	}
	out := make([]enriched, 0, len(cards))
	for _, c := range cards {
		e := enriched{DPGCard: c}
		switch role {
		case "holder":
			if s, ok := h.walletRegistry[c.ID]; ok && s != nil {
				caps := s.Capabilities()
				e.SupportsSD = caps.SelectiveDisclosure
			}
		case "verifier":
			if s, ok := h.verifierRegistry[c.ID]; ok && s != nil {
				// VerifierCapabilities doesn't carry Formats yet, but the
				// static card formats list is good enough for rendering.
				_ = s.Capabilities()
			}
		default:
			if s, ok := h.issuerRegistry[c.ID]; ok && s != nil {
				caps := s.Capabilities()
				e.LiveFormats = caps.Formats
				e.SupportsBatch = caps.Batch
				e.SupportsSD = caps.SelectiveDisclosure
				if len(e.LiveFormats) > 0 {
					e.Formats = e.LiveFormats
				}
			}
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// unused guard — keeps fmt import available for future debug logging.
var _ = fmt.Sprintf
