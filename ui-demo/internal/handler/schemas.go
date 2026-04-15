package handler

// schemas.go — Phase 3 handlers for the pre-built schema catalog.
//
// Schemas live as JSON files under web/schemas/ and are embedded into the
// binary via web.SchemaFS. The catalog is filtered at request time by the
// user's chosen DPG (state.IssuerDPG) so the UI only offers schemas the
// active backend can actually sign.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/web"
)

// CatalogSchema is one entry in the starter-schema catalog.
type CatalogSchema struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Category       string        `json:"category"`
	CredentialType string        `json:"credentialType"`
	Format         string        `json:"format"`
	Standard       string        `json:"standard"`
	CompatibleDPGs []string      `json:"compatibleDPGs"`
	Fields         []SchemaField `json:"fields"`
}

var (
	catalogOnce sync.Once
	catalogList []CatalogSchema
	catalogErr  error
)

// loadCatalog reads every schema JSON file from web.SchemaFS on first use
// and caches the parsed result for the process lifetime.
func loadCatalog() ([]CatalogSchema, error) {
	catalogOnce.Do(func() {
		entries, err := fs.ReadDir(web.SchemaFS, ".")
		if err != nil {
			catalogErr = fmt.Errorf("read schemas dir: %w", err)
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := fs.ReadFile(web.SchemaFS, e.Name())
			if err != nil {
				continue
			}
			var s CatalogSchema
			if err := json.Unmarshal(data, &s); err != nil {
				continue
			}
			catalogList = append(catalogList, s)
		}
		sort.Slice(catalogList, func(i, j int) bool {
			return catalogList[i].Name < catalogList[j].Name
		})
	})
	return catalogList, catalogErr
}

// APISchemaCatalog handles GET /api/schemas/catalog — returns the credential
// types the user's chosen issuer DPG actually supports, enriched with
// field definitions from our static starter-schema files where available.
//
// Why this is live and not static: every DPG has its own set of configured
// credential types. Inji Certify reads them from a SQL seed table, Walt.id
// ships a 29-entry OID4VCI metadata file, Credebl agents expose whatever is
// in their schema registry. We query each one and let the backend speak for
// itself, so the UI never offers a type the DPG can't actually issue.
func (h *Handler) APISchemaCatalog(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())

	// Resolve which DPG to query: explicit override, user's onboarding
	// choice, or the server default.
	dpg := r.URL.Query().Get("dpg")
	if dpg == "" && user != nil {
		dpg = user.IssuerDPG
	}
	if dpg == "" && user != nil {
		if st := h.onboarding.Get(user.ID); st != nil {
			dpg = st.IssuerDPG
		}
	}

	// Load the static starter schemas as a map keyed by credentialType so
	// we can overlay field definitions onto live configs.
	staticList, _ := loadCatalog()
	staticByType := map[string]CatalogSchema{}
	for _, s := range staticList {
		if s.CredentialType != "" {
			staticByType[s.CredentialType] = s
		}
		// Also key by the catalog ID for /catalog/{id} lookups.
	}

	// Query the user's chosen issuer for its live credential configurations.
	var issuer = h.issuerFor(user)
	liveConfigs, err := issuer.ListCredentialConfigs(r.Context())
	if err != nil {
		// Don't fail the whole catalog — fall back to the static list
		// filtered by compatibleDPGs so the user still sees something.
		out := make([]CatalogSchema, 0, len(staticList))
		for _, s := range staticList {
			if dpg == "" || containsString(s.CompatibleDPGs, dpg) {
				out = append(out, s)
			}
		}
		writeJSON(w, 200, out)
		return
	}

	// Build the catalog from LIVE configs. For each live config, try to
	// match it to a static starter schema by credentialType — if found,
	// we get a nice description + pre-defined field list. If not, the
	// schema is blank and the user builds it from scratch in the editor.
	out := make([]CatalogSchema, 0, len(liveConfigs))
	for _, cfg := range liveConfigs {
		entry := CatalogSchema{
			ID:             cfg.ID,
			Name:           cfg.Name,
			Description:    "", // filled in below from static overlay if present
			Category:       cfg.Category,
			CredentialType: cfg.ID, // use the live ID verbatim so issue calls match
			Format:         cfg.Format,
			Standard:       "W3C-VCDM",
			CompatibleDPGs: []string{dpg},
		}
		if entry.Name == "" {
			entry.Name = cfg.ID
		}
		if entry.Category == "" {
			entry.Category = "General"
		}

		// Try each candidate credentialType (live ID, then progressively
		// stripped variants) against the static starter catalog and use
		// the first match to overlay fields + description.
		matched := false
		for _, ct := range credentialTypeCandidates(cfg.ID) {
			if overlay, ok := staticByType[ct]; ok {
				entry.Description = overlay.Description
				entry.Fields = overlay.Fields
				if entry.Standard == "" {
					entry.Standard = overlay.Standard
				}
				if entry.Category == "General" && overlay.Category != "" {
					entry.Category = overlay.Category
				}
				matched = true
				break
			}
		}
		if !matched {
			entry.Description = "Live credential type from " + issuer.Name() +
				". Define the claim fields in the editor."
		}
		out = append(out, entry)
	}

	// Sort the output for stable UI.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, 200, out)
}

// credentialTypeCandidates expands a live credential configuration ID into
// a list of candidate credentialType names we'll try matching against the
// static starter-schema catalog (in priority order). A live config ID may
// carry format suffixes, version suffixes, or both — e.g.:
//
//	"FarmerCredentialV2"             → ["FarmerCredentialV2", "FarmerCredential"]
//	"FarmerCredentialSdJwt"          → ["FarmerCredentialSdJwt", "FarmerCredential"]
//	"UniversityDegree_jwt_vc_json"   → ["UniversityDegree_jwt_vc_json", "UniversityDegree"]
//	"KycCredential_ldp_vc"           → ["KycCredential_ldp_vc", "KycCredential"]
//
// The caller tries each candidate in order against the static catalog and
// uses the first match.
func credentialTypeCandidates(id string) []string {
	out := []string{id}
	cur := id

	// Strip OID4VCI-style format suffixes (joined by underscore).
	formatSuffixes := []string{
		"_jwt_vc_json-ld",
		"_jwt_vc_json",
		"_ldp_vc",
		"_vc+sd-jwt",
		"_sd-jwt",
		"_sdjwt",
		"_mso_mdoc",
	}
	for _, s := range formatSuffixes {
		if strings.HasSuffix(cur, s) {
			cur = strings.TrimSuffix(cur, s)
			break
		}
	}

	// Strip CamelCase format/variant suffixes common in Inji configs.
	camelSuffixes := []string{"SdJwt", "SDJWT", "JwtVc", "JWT", "LdpVc", "LDP", "Mdoc", "MDOC"}
	for _, s := range camelSuffixes {
		if strings.HasSuffix(cur, s) {
			cur = strings.TrimSuffix(cur, s)
			break
		}
	}

	// Strip trailing version markers like "V2", "V3", "v10".
	if trimmed := stripTrailingVersion(cur); trimmed != "" {
		cur = trimmed
	}

	if cur != "" && cur != id {
		out = append(out, cur)
	}
	return out
}

// stripTrailingVersion removes a trailing "V" + digits marker from an
// identifier ("FarmerCredentialV2" → "FarmerCredential"). Returns "" if
// no version marker is present.
func stripTrailingVersion(s string) string {
	if len(s) < 2 {
		return ""
	}
	i := len(s) - 1
	for i >= 0 && s[i] >= '0' && s[i] <= '9' {
		i--
	}
	if i == len(s)-1 {
		return ""
	}
	if s[i] == 'V' || s[i] == 'v' {
		return s[:i]
	}
	return ""
}

// APISchemaCatalogEntry handles GET /api/schemas/catalog/{id}. The id can
// be either a live DPG config ID or a static starter-schema ID.
func (h *Handler) APISchemaCatalogEntry(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	id := r.PathValue("id")

	// Try the live configs first — the user most likely picked one from
	// the live catalog, so the ID matches a live config.
	if user != nil {
		if issuer := h.issuerFor(user); issuer != nil {
			if liveConfigs, err := issuer.ListCredentialConfigs(r.Context()); err == nil {
				for _, cfg := range liveConfigs {
					if cfg.ID != id {
						continue
					}
					// Build the same merged entry APISchemaCatalog returns.
					staticList, _ := loadCatalog()
					staticByType := map[string]CatalogSchema{}
					for _, s := range staticList {
						if s.CredentialType != "" {
							staticByType[s.CredentialType] = s
						}
					}
					entry := CatalogSchema{
						ID:             cfg.ID,
						Name:           cfg.Name,
						Category:       cfg.Category,
						CredentialType: cfg.ID,
						Format:         cfg.Format,
						Standard:       "W3C-VCDM",
					}
					if entry.Name == "" {
						entry.Name = cfg.ID
					}
					matched := false
					for _, ct := range credentialTypeCandidates(cfg.ID) {
						if overlay, ok := staticByType[ct]; ok {
							entry.Description = overlay.Description
							entry.Fields = overlay.Fields
							if entry.Standard == "" {
								entry.Standard = overlay.Standard
							}
							if entry.Category == "" && overlay.Category != "" {
								entry.Category = overlay.Category
							}
							matched = true
							break
						}
					}
					if !matched {
						entry.Description = "Live credential type from " + issuer.Name() + "."
					}
					writeJSON(w, 200, entry)
					return
				}
			}
		}
	}

	// Fall back to static catalog for IDs like "university-degree-ldp".
	all, err := loadCatalog()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for _, s := range all {
		if s.ID == id {
			writeJSON(w, 200, s)
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "schema not found"})
}

// APIPublishCatalogSchema takes an edited catalog schema (either a starter
// overlay or a fully custom definition) and saves it into the per-user
// schema registry.
//
// Critical for custom schemas: BEFORE saving, we try to register the
// credential type with the user's chosen issuer DPG. Walt.id's issuer-api
// maintains an allowlist of credential_configuration_ids in its metadata
// file; Inji Certify keeps them in a SQL seed table. A schema whose
// configId isn't in that allowlist will always fail at issue time with
// `Invalid Credential Configuration Id` (400) — so we refuse to publish
// anything the backend wouldn't accept.
//
// Starter schemas (where CredentialType matches an existing live config ID
// from ListCredentialConfigs) are saved directly; no registration is
// needed because the backend already knows them.
func (h *Handler) APIPublishCatalogSchema(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		CatalogID      string        `json:"catalogId"`
		Name           string        `json:"name"`
		CredentialType string        `json:"credentialType"`
		Description    string        `json:"description"`
		Format         string        `json:"format"`
		Fields         []SchemaField `json:"fields"`
		Custom         bool          `json:"custom"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name required"})
		return
	}
	if req.Format == "" {
		req.Format = "ldp_vc"
	}
	if req.CredentialType == "" {
		// Derive a camelCased type name from the schema name, stripping
		// spaces and punctuation. We refuse the generic VerifiableCredential
		// alias further down — it's not a real config ID on any DPG.
		req.CredentialType = sanitizeTypeName(req.Name)
	}
	if req.CredentialType == "" || strings.EqualFold(req.CredentialType, "VerifiableCredential") {
		writeJSON(w, 400, map[string]string{
			"error": "credential type name is required and must not be 'VerifiableCredential' — pick a specific type name like 'TestaID' or 'FarmerCredential'",
		})
		return
	}

	issuerStore := h.issuerFor(user)

	// Resolve the final configId:
	//   1. If CredentialType matches an existing live config, use that ID
	//      verbatim (starter-catalog path — no registration needed).
	//   2. Otherwise, try issuer.RegisterCredentialType. On success, use
	//      the backend-returned configId.
	//   3. On registration failure, return a structured error — do NOT
	//      save a schema the user can't actually issue.
	finalConfigID := ""
	registrationWarning := ""

	liveConfigs, listErr := issuerStore.ListCredentialConfigs(r.Context())
	if listErr == nil {
		for _, cfg := range liveConfigs {
			// Match either directly or via the credentialTypeCandidates
			// heuristic so format-suffixed IDs count (e.g. the user picks
			// "FarmerCredential" and the DPG has "FarmerCredential_ldp_vc").
			if cfg.ID == req.CredentialType {
				finalConfigID = cfg.ID
				break
			}
			for _, cand := range credentialTypeCandidates(cfg.ID) {
				if cand == req.CredentialType {
					finalConfigID = cfg.ID
					break
				}
			}
			if finalConfigID != "" {
				break
			}
		}
	}

	if finalConfigID == "" {
		// Need to register this as a brand new type with the backend.
		caps := issuerStore.Capabilities()
		if !caps.CustomTypes {
			writeJSON(w, 400, map[string]any{
				"error": fmt.Sprintf(
					"%s doesn't allow new credential types to be registered at runtime. "+
						"Pick one of the starter schemas (they're driven by the DPG's live "+
						"credential_configurations_supported), or switch to an issuer DPG "+
						"that exposes a register endpoint.",
					issuerStore.Name(),
				),
				"suggestion":       "pick-starter",
				"availableConfigs": liveConfigs,
			})
			return
		}
		registered, regErr := issuerStore.RegisterCredentialType(
			r.Context(),
			req.CredentialType,
			req.Name,
			req.Description,
			req.Format,
		)
		if regErr != nil {
			writeJSON(w, 500, map[string]string{
				"error": fmt.Sprintf(
					"failed to register %q with %s: %v. Pick a starter schema instead, or rename your type to match one of the existing config IDs.",
					req.CredentialType, issuerStore.Name(), regErr,
				),
			})
			return
		}
		finalConfigID = registered
		registrationWarning = fmt.Sprintf(
			"Registered %q with %s — backend may need a few seconds to pick up the new type.",
			finalConfigID, issuerStore.Name(),
		)
	}

	schema := CredentialSchema{
		ID:       req.CatalogID + "-" + user.ID,
		Name:     req.Name,
		ConfigID: finalConfigID,
		Format:   req.Format,
		Standard: "W3C-VCDM",
		Fields:   req.Fields,
	}
	h.addSchema(user, schema)

	// Update the onboarding state so the wizard's Next button unlocks.
	st := h.getOrCreateState(user)
	if !containsString(st.PublishedSchemaIDs, schema.ID) {
		st.PublishedSchemaIDs = append(st.PublishedSchemaIDs, schema.ID)
		h.persistState(w, user, st)
	}

	resp := map[string]any{
		"status": "published",
		"schema": schema,
	}
	if registrationWarning != "" {
		resp["warning"] = registrationWarning
	}
	writeJSON(w, 200, resp)
}

// sanitizeTypeName converts a free-form schema name into a valid credential
// type identifier: strip spaces, punctuation, and any leading digits. Keeps
// the original casing so "TestaID" stays "TestaID".
func sanitizeTypeName(name string) string {
	var b strings.Builder
	for i, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i > 0 {
				b.WriteRune(r)
			}
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsString(ss []string, needle string) bool {
	for _, s := range ss {
		if s == needle {
			return true
		}
	}
	return false
}

// Ensure model import stays referenced.
var _ = model.User{}
