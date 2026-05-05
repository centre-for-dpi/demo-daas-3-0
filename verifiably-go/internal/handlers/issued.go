package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/issuance"
	"github.com/verifiably/verifiably-go/vctypes"
)

// statusListKindFor maps a Schema.Std to the status list kind verifiably-go
// hosts for that taxonomy. Returns "" for credentials whose revocation
// flow we don't model (mso_mdoc → MSO/IACA, legacy jwt_vc → out of scope).
func statusListKindFor(std string) string {
	switch std {
	case "w3c_vcdm_2":
		return "bitstring"
	case "sd_jwt_vc (IETF)", "sd_jwt_vc":
		return "token"
	default:
		return ""
	}
}

// allocateStatusListBinding picks the right Store for a schema's Std and
// reserves an index. Returns (binding, store) so the caller can roll back
// on issuance failure — Allocate persists the next-free counter
// immediately, and we don't want the index to drift past the last
// successful issuance just because walt.id 5xx'd.
//
// Returns (nil, nil, nil) when the schema's Std doesn't support a status
// list OR when the corresponding Store isn't configured. Issuance proceeds
// without a binding in that case.
func (h *H) allocateStatusListBinding(schema vctypes.Schema) (*backend.StatusListBinding, error) {
	kind := statusListKindFor(schema.Std)
	if kind == "" {
		return nil, nil
	}
	var store = h.BitstringStore
	if kind == "token" {
		store = h.TokenStore
	}
	if store == nil {
		return nil, nil
	}
	idx, err := store.Allocate()
	if err != nil {
		return nil, fmt.Errorf("status list allocate: %w", err)
	}
	return &backend.StatusListBinding{
		Type:       store.Kind,
		ListID:     store.ListID,
		Index:      idx,
		PublishURL: store.PublishURL,
	}, nil
}

// recordIssuance writes the audit-log entry after a successful walt.id
// issuance. Invoked from the issuance handler. The HolderHint is the
// first non-empty of a small allowlist of "looks like a name / id" field
// names so the list page is searchable by holder without us guessing
// per-schema. Subject fields are stored verbatim — they're already in the
// session anyway.
func (h *H) recordIssuance(schema vctypes.Schema, issuerDpg string, subject map[string]string, offerURI string, binding *backend.StatusListBinding) {
	if h.IssuanceLog == nil {
		return
	}
	id := newIssuanceID()
	hint := ""
	for _, k := range []string{"fullName", "name", "given_name", "id", "individualId", "vehicleNumber", "farmerID"} {
		if v := strings.TrimSpace(subject[k]); v != "" {
			hint = v
			break
		}
	}
	// Resolve the wire format from the active variant. Schema.ID after
	// ApplyVariant matches the variant's ID; Schema itself doesn't carry
	// a top-level Format. Fall back to Std so the log column is never empty.
	format := schema.Std
	for _, v := range schema.Variants {
		if v.ID == schema.ID {
			format = v.Format
			break
		}
	}
	rec := issuance.IssuedCredential{
		ID:            id,
		SchemaID:      schema.ID,
		SchemaName:    schema.Name,
		Std:           schema.Std,
		Format:        format,
		IssuerDpg:     issuerDpg,
		HolderHint:    hint,
		SubjectFields: subject,
		OfferURI:      offerURI,
	}
	if binding != nil {
		rec.StatusList = &issuance.StatusListEntry{
			Type:   binding.Type,
			ListID: binding.ListID,
			Index:  binding.Index,
		}
	}
	if _, err := h.IssuanceLog.Append(rec); err != nil {
		// Don't fail the issuance — the credential is in the holder's
		// wallet either way. Just log and move on so the operator's flow
		// stays smooth.
		fmt.Printf("issuance log: append %s: %v\n", id, err)
	}
}

// newIssuanceID mints a stable identifier for the IssuanceLog entry. The
// prefix lets `grep vc-` find them in logs; the millisecond timestamp
// makes them sort-friendly even before the JSON is materialized.
func newIssuanceID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("vc-%d-%s", time.Now().UTC().UnixMilli(), hex.EncodeToString(b[:]))
}

// issuedCredentialsData feeds the /issuer/credentials list page.
type issuedCredentialsData struct {
	Items   []issuance.IssuedCredential
	Stats   issuance.Stats
	Filter  issuance.Filter
	Stds    []string // chip row
	Formats []string
}

// ShowIssuedCredentials renders the list page.
func (h *H) ShowIssuedCredentials(w http.ResponseWriter, r *http.Request) {
	if h.IssuanceLog == nil {
		http.Error(w, "issuance log not configured", http.StatusNotFound)
		return
	}
	sess := h.Sessions.MustGet(w, r)
	data := h.issuedCredentialsBody(sess, r)
	h.render(w, r, "issuer_credentials", h.pageData(sess, data))
}

// IssuedCredentialsSearch handles HTMX search/filter on the same data.
func (h *H) IssuedCredentialsSearch(w http.ResponseWriter, r *http.Request) {
	if h.IssuanceLog == nil {
		http.Error(w, "issuance log not configured", http.StatusNotFound)
		return
	}
	sess := h.Sessions.MustGet(w, r)
	// Capture the filter state on the session so a Revoke action's re-
	// render preserves what the user was looking at instead of resetting.
	q := r.URL.Query().Get("q")
	if v := r.FormValue("q"); v != "" {
		q = v
	}
	sess.IssuedQuery = q
	if v := r.FormValue("std"); v != "" || r.URL.Query().Has("std") {
		sess.IssuedStd = strings.TrimSpace(r.FormValue("std"))
		if sess.IssuedStd == "" {
			sess.IssuedStd = strings.TrimSpace(r.URL.Query().Get("std"))
		}
		if sess.IssuedStd == "all" {
			sess.IssuedStd = ""
		}
	}
	if v := r.FormValue("format"); v != "" || r.URL.Query().Has("format") {
		sess.IssuedFormat = strings.TrimSpace(r.FormValue("format"))
		if sess.IssuedFormat == "" {
			sess.IssuedFormat = strings.TrimSpace(r.URL.Query().Get("format"))
		}
		if sess.IssuedFormat == "all" {
			sess.IssuedFormat = ""
		}
	}
	if v := r.FormValue("state"); v != "" || r.URL.Query().Has("state") {
		sess.IssuedState = strings.TrimSpace(r.FormValue("state"))
		if sess.IssuedState == "" {
			sess.IssuedState = strings.TrimSpace(r.URL.Query().Get("state"))
		}
		if sess.IssuedState == "all" {
			sess.IssuedState = ""
		}
	}
	data := h.issuedCredentialsBody(sess, r)
	h.renderFragment(w, r, "fragment_issued_credentials_list", data)
}

// RevokeIssuedCredential flips the bit on the credential's status list
// entry, marks the log row revoked, and re-renders the row fragment so
// HTMX can swap it in place.
func (h *H) RevokeIssuedCredential(w http.ResponseWriter, r *http.Request) {
	if h.IssuanceLog == nil {
		http.Error(w, "issuance log not configured", http.StatusNotFound)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		id = r.FormValue("id")
	}
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	rec, ok := h.IssuanceLog.Get(id)
	if !ok {
		http.Error(w, "credential not found", http.StatusNotFound)
		return
	}
	if rec.StatusList == nil {
		// No status list bound — Revoke is meaningless. Surface the reason
		// instead of silently no-op'ing so the operator can tell why
		// their button click didn't take.
		h.errorToast(w, r, "This credential has no status list binding (e.g. mdoc) and cannot be revoked through verifiably-go.")
		return
	}
	store := h.storeForKind(rec.StatusList.Type)
	if store == nil {
		h.errorToast(w, r, "Status list "+rec.StatusList.Type+" not configured.")
		return
	}
	if err := store.Revoke(rec.StatusList.Index); err != nil {
		h.errorToast(w, r, "Revoke: "+err.Error())
		return
	}
	if _, err := h.IssuanceLog.MarkRevoked(id); err != nil {
		h.errorToast(w, r, "Mark revoked: "+err.Error())
		return
	}
	// HTMX caller targets the row by id so a single-row fragment is enough
	// to reflect the new state. Re-fetch for the latest RevokedAt.
	rec, _ = h.IssuanceLog.Get(id)
	h.renderFragment(w, r, "fragment_issued_credentials_row", rec)
}

func (h *H) storeForKind(kind string) interface {
	Revoke(int) error
} {
	switch kind {
	case "bitstring":
		if h.BitstringStore != nil {
			return h.BitstringStore
		}
	case "token":
		if h.TokenStore != nil {
			return h.TokenStore
		}
	}
	return nil
}

func (h *H) issuedCredentialsBody(sess *Session, _ *http.Request) issuedCredentialsData {
	filter := issuance.Filter{
		Query:  sess.IssuedQuery,
		Std:    sess.IssuedStd,
		Format: sess.IssuedFormat,
		State:  sess.IssuedState,
	}
	items := h.IssuanceLog.List(filter)
	stats := h.IssuanceLog.Summary()
	stds := []string{"all"}
	for s := range stats.ByStd {
		stds = append(stds, s)
	}
	formats := []string{"all"}
	for f := range stats.ByFormat {
		formats = append(formats, f)
	}
	return issuedCredentialsData{
		Items:   items,
		Stats:   stats,
		Filter:  filter,
		Stds:    stds,
		Formats: formats,
	}
}
