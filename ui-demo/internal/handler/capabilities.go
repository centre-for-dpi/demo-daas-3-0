package handler

import (
	"context"
	"net/http"
	"strconv"

	"vcplatform/internal/datasource"
	"vcplatform/internal/model"
)

// APICapabilities returns the composed capability matrix for the active
// backends — reflecting the logged-in user's chosen issuer DPG when set.
// The UI fetches this on page load to decide which flows to render.
//
// This endpoint is unauthenticated (landing pages call it too), so it
// parses the session cookie directly instead of relying on middleware.
func (h *Handler) APICapabilities(w http.ResponseWriter, r *http.Request) {
	caps := h.stores.Capabilities()

	// Parse the session cookie directly — no auth middleware on this route.
	var user *model.User
	if c, err := r.Cookie("session"); err == nil {
		user = model.UserFromSession(c.Value)
	}

	// Override each capability block with the user's per-role DPG choice.
	if user != nil {
		if user.IssuerDPG != "" {
			if issuer := h.issuerFor(user); issuer != nil {
				caps.Issuer = issuer.Capabilities()
				caps.IssuerName = issuer.Name()
			}
		}
		if user.WalletDPG != "" {
			if wallet := h.walletFor(user); wallet != nil {
				caps.Wallet = wallet.Capabilities()
				caps.WalletName = wallet.Name()
			}
		}
		if user.VerifierDPG != "" {
			if verifier := h.verifierFor(user); verifier != nil {
				caps.Verifier = verifier.Capabilities()
				caps.VerifierName = verifier.Name()
			}
		}
	}

	// Beta flags: an adaptor can return empty capabilities (all false + no formats)
	// to indicate it's a stub. The UI shows a "Beta / Coming soon" banner for that service.
	if caps.Issuer.IssuerInitiated == false && len(caps.Issuer.Formats) == 0 {
		caps.IssuerBeta = true
	}
	if caps.Wallet.ClaimOffer == false && len(caps.Wallet.DIDMethods) == 0 {
		caps.WalletBeta = true
	}
	if caps.Verifier.CreateRequest == false && caps.Verifier.DirectVerify == false {
		caps.VerifierBeta = true
	}

	writeJSON(w, http.StatusOK, caps)
}

var _ = model.User{} // ensure model import stays used even if future edits remove the ref

// APIListDataSources returns all registered data sources with their descriptions.
// Used by the issuance flow to let the user pick a source of citizen data.
func (h *Handler) APIListDataSources(w http.ResponseWriter, r *http.Request) {
	sources := h.dataSources.List()
	out := make([]map[string]any, 0, len(sources))
	for _, ds := range sources {
		desc, err := ds.Describe(r.Context())
		entry := map[string]any{
			"name": ds.Name(),
			"kind": ds.Kind(),
		}
		if err == nil && desc != nil {
			entry["displayName"] = desc.DisplayName
			entry["summary"] = desc.Summary
			entry["totalRecords"] = desc.TotalRecords
			entry["fields"] = desc.Fields
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

// APIFetchDataSourceRecord returns a single record from a named data source.
// Used by the Single Issuance UI to auto-fill claims from the citizens database
// (or any other registered source) once the operator types a citizen ID and
// clicks "Lookup".
//
// Query params:
//   - source: data source name (e.g. "Citizens Database")
//   - id:     primary key value (e.g. "KE-NID-81016525")
//
// Response shape:
//
//	{
//	  "source": "Citizens Database",
//	  "record": { "national_id": "...", "first_name": "...", ... },
//	  "suggestedMappings": { "UniversityDegree": {"name":"first_name", ...}, ... }
//	}
func (h *Handler) APIFetchDataSourceRecord(w http.ResponseWriter, r *http.Request) {
	sourceName := r.URL.Query().Get("source")
	id := r.URL.Query().Get("id")
	if sourceName == "" || id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source and id query params required"})
		return
	}
	ds := h.dataSources.Get(sourceName)
	if ds == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "data source not found: " + sourceName})
		return
	}

	rec, err := ds.FetchRecord(r.Context(), id)
	if err != nil {
		if err == datasource.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no record with id " + id})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	out := map[string]any{
		"source": ds.Name(),
		"record": rec,
	}
	if desc, derr := ds.Describe(r.Context()); derr == nil && desc != nil && len(desc.SuggestedMappings) > 0 {
		out["suggestedMappings"] = desc.SuggestedMappings
	}
	writeJSON(w, http.StatusOK, out)
}

// APIDataSourceSearch returns records matching a free-text query across the
// data source's configured search fields. Used by the Single Issuance UI to
// drive the autocomplete picker.
//
// Query params:
//   - source: data source name
//   - q:      free-text query (matches any configured search column)
//   - limit:  max records to return (default 25)
func (h *Handler) APIDataSourceSearch(w http.ResponseWriter, r *http.Request) {
	sourceName := r.URL.Query().Get("source")
	query := r.URL.Query().Get("q")
	if sourceName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source query param required"})
		return
	}
	ds := h.dataSources.Get(sourceName)
	if ds == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "data source not found: " + sourceName})
		return
	}
	limit := 25
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	records, err := ds.Search(r.Context(), query, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":  ds.Name(),
		"query":   query,
		"count":   len(records),
		"records": records,
	})
}

// APIDataSourceSample returns the first N records from a data source, used by
// the UI to show a "Browse records" preview when the user hasn't typed a query
// yet.
//
// Query params:
//   - source: data source name
//   - limit:  max records (default 10, max 100)
func (h *Handler) APIDataSourceSample(w http.ResponseWriter, r *http.Request) {
	sourceName := r.URL.Query().Get("source")
	if sourceName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source query param required"})
		return
	}
	ds := h.dataSources.Get(sourceName)
	if ds == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "data source not found: " + sourceName})
		return
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	records, err := ds.ListRecords(r.Context(), datasource.Filter{Limit: limit})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":  ds.Name(),
		"count":   len(records),
		"records": records,
	})
}

// Ensure context import is used (some Go versions complain about unused if body is empty).
var _ = context.TODO
