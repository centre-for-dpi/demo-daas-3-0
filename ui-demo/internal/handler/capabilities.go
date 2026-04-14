package handler

import (
	"context"
	"net/http"

	"vcplatform/internal/datasource"
)

// APICapabilities returns the composed capability matrix for the active backends.
// The UI fetches this on page load to decide which flows to render.
func (h *Handler) APICapabilities(w http.ResponseWriter, r *http.Request) {
	caps := h.stores.Capabilities()

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

// Ensure context import is used (some Go versions complain about unused if body is empty).
var _ = context.TODO
