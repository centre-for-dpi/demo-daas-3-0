package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/verifiably/verifiably-go/vctypes"
)

// ShowSchemaBrowser renders the schema-browse page.
func (h *H) ShowSchemaBrowser(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.IssuerDpg == "" {
		h.redirect(w, r, "/issuer/dpg")
		return
	}
	data := h.schemaBrowserData(w, r, sess)
	h.render(w, r, "issuer_schema", h.pageData(sess, data))
}

type schemaBrowserData struct {
	Schemas      []vctypes.Schema
	Stds         []string
	Filter       string
	Query        string
	ExpandedID   string
	SelectedID   string
	ExpandedJSON string
}

func (h *H) schemaBrowserData(w http.ResponseWriter, r *http.Request, sess *Session) schemaBrowserData {
	ctx := r.Context()
	schemas, err := h.Adapter.ListSchemas(ctx, sess.IssuerDpg)
	if err != nil {
		h.errorToast(w, r, err.Error())
		return schemaBrowserData{}
	}
	// Build the std-chip list from the UNFILTERED schemas so the chips don't
	// vanish when a filter selection would empty the list.
	stds := []string{"all"}
	seen := map[string]bool{"all": true}
	for _, s := range schemas {
		if !seen[s.Std] {
			seen[s.Std] = true
			stds = append(stds, s.Std)
		}
	}

	if sess.SchemaFilter == "" {
		sess.SchemaFilter = "all"
	}
	q := strings.ToLower(sess.SchemaQuery)
	filtered := []vctypes.Schema{}
	for _, s := range schemas {
		if sess.SchemaFilter != "all" && s.Std != sess.SchemaFilter {
			continue
		}
		if q != "" {
			hay := strings.ToLower(s.Name + " " + s.Desc + " " + s.Std)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		filtered = append(filtered, s)
	}

	// Look up expanded JSON against the full list, not the filtered one,
	// so a currently-expanded card keeps its preview even if filter/search would hide it.
	expandedJSON := ""
	if sess.ExpandedSchemaID != "" {
		for _, s := range schemas {
			if s.ID == sess.ExpandedSchemaID {
				expandedJSON = buildJSONSchema(s)
				break
			}
		}
	}
	return schemaBrowserData{
		Schemas:      filtered,
		Stds:         stds,
		Filter:       sess.SchemaFilter,
		Query:        sess.SchemaQuery,
		ExpandedID:   sess.ExpandedSchemaID,
		SelectedID:   sess.SchemaID,
		ExpandedJSON: expandedJSON,
	}
}

// SchemaSearch handles HTMX search requests. Updates session query and returns the list fragment.
func (h *H) SchemaSearch(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	sess.SchemaQuery = r.URL.Query().Get("q")
	data := h.schemaBrowserData(w, r, sess)
	h.renderFragment(w, "fragment_schema_list", data)
}

// SetSchemaFilter updates the active chip filter.
func (h *H) SetSchemaFilter(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	f := r.FormValue("filter")
	if f == "" {
		f = r.URL.Query().Get("filter")
	}
	if f == "" {
		f = "all"
	}
	sess.SchemaFilter = f
	data := h.schemaBrowserData(w, r, sess)
	// Re-render the whole browser body so chip active state + list stay in sync
	h.renderFragment(w, "fragment_schema_browser_body", data)
}

// ToggleSchemaExpand toggles a schema card's expanded state and re-renders the list.
func (h *H) ToggleSchemaExpand(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	id := r.FormValue("id")
	if sess.ExpandedSchemaID == id {
		sess.ExpandedSchemaID = ""
	} else {
		sess.ExpandedSchemaID = id
	}
	data := h.schemaBrowserData(w, r, sess)
	h.renderFragment(w, "fragment_schema_list", data)
}

// SelectSchema marks a schema as chosen for the downstream issuance flow.
// Re-renders the browser body AND pushes an OOB update for the page-level
// Continue button (its enabled state depends on SelectedID).
func (h *H) SelectSchema(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	id := r.FormValue("id")
	sess.SchemaID = id
	data := h.schemaBrowserData(w, r, sess)
	w.Header().Set("HX-Trigger", `{"toast":"Schema selected — click Continue"}`)
	h.renderFragments(w, data, "fragment_schema_browser_body", "fragment_schema_continue_oob")
}

// ShowSchemaBuilder renders the schema-builder page.
func (h *H) ShowSchemaBuilder(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if sess.IssuerDpg == "" {
		h.redirect(w, r, "/issuer/dpg")
		return
	}
	// Default: two blank fields
	data := builderData{
		Fields: []vctypes.FieldSpec{{Datatype: "string", Required: true}, {Datatype: "string", Required: true}},
		Std:    "w3c_vcdm_2",
	}
	data.PreviewJSON = buildJSONSchema(currentBuilderSchema(sess, data))
	h.render(w, r, "issuer_schema_builder", h.pageData(sess, data))
}

type builderData struct {
	Name        string
	Desc        string
	ExtraType   string
	Std         string
	Fields      []vctypes.FieldSpec
	PreviewJSON string
}

// SchemaPreview is called on every keystroke in the builder — returns the updated JSON preview
// and re-renders the field rows if the fields array changed (add/remove).
func (h *H) SchemaPreview(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	data := extractBuilderData(r)
	data.PreviewJSON = buildJSONSchema(currentBuilderSchema(sess, data))
	h.renderFragment(w, "fragment_schema_preview", data)
}

// AddSchemaField adds a blank field row and re-renders.
func (h *H) AddSchemaField(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	_ = r.ParseForm()
	data := extractBuilderData(r)
	data.Fields = append(data.Fields, vctypes.FieldSpec{Datatype: "string"})
	data.PreviewJSON = buildJSONSchema(currentBuilderSchema(sess, data))
	h.renderFragment(w, "fragment_schema_builder_form", data)
}

// RemoveSchemaField removes a field row by index.
func (h *H) RemoveSchemaField(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	_ = r.ParseForm()
	data := extractBuilderData(r)
	idx, _ := strconv.Atoi(r.FormValue("idx"))
	if idx >= 0 && idx < len(data.Fields) {
		data.Fields = append(data.Fields[:idx], data.Fields[idx+1:]...)
	}
	data.PreviewJSON = buildJSONSchema(currentBuilderSchema(sess, data))
	h.renderFragment(w, "fragment_schema_builder_form", data)
}

// SaveSchema persists a custom schema and returns to the browser.
// ?use=1 also selects it.
func (h *H) SaveSchema(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	_ = r.ParseForm()
	data := extractBuilderData(r)
	if strings.TrimSpace(data.Name) == "" {
		h.errorToast(w, r, "Schema needs a name")
		return
	}
	if len(data.Fields) == 0 || allBlank(data.Fields) {
		h.errorToast(w, r, "Add at least one claim field")
		return
	}
	schema := currentBuilderSchema(sess, data)
	if err := h.Adapter.SaveCustomSchema(r.Context(), schema); err != nil {
		h.errorToast(w, r, err.Error())
		return
	}
	sess.ExpandedSchemaID = schema.ID
	if r.URL.Query().Get("use") == "1" {
		sess.SchemaID = schema.ID
	}
	h.redirect(w, r, "/issuer/schema")
}

// DeleteSchema removes a custom schema from the session.
// Deleting the currently-selected schema clears the selection, so this also
// pushes an OOB update for the page-level Continue button.
func (h *H) DeleteSchema(w http.ResponseWriter, r *http.Request) {
	sess := h.Sessions.MustGet(w, r)
	id := r.FormValue("id")
	_ = h.Adapter.DeleteCustomSchema(r.Context(), id)
	if sess.SchemaID == id {
		sess.SchemaID = ""
	}
	if sess.ExpandedSchemaID == id {
		sess.ExpandedSchemaID = ""
	}
	data := h.schemaBrowserData(w, r, sess)
	h.renderFragments(w, data, "fragment_schema_browser_body", "fragment_schema_continue_oob")
}

func extractBuilderData(r *http.Request) builderData {
	d := builderData{
		Name:      r.FormValue("name"),
		Desc:      r.FormValue("desc"),
		ExtraType: r.FormValue("extra_type"),
		Std:       r.FormValue("std"),
	}
	if d.Std == "" {
		d.Std = "w3c_vcdm_2"
	}
	// Field rows come as field_name_0, field_datatype_0, field_required_0, ...
	for i := 0; i < 50; i++ {
		name := r.FormValue(fmt.Sprintf("field_name_%d", i))
		dt := r.FormValue(fmt.Sprintf("field_datatype_%d", i))
		if dt == "" && name == "" && r.Form[fmt.Sprintf("field_name_%d", i)] == nil {
			break
		}
		req := r.FormValue(fmt.Sprintf("field_required_%d", i)) == "on"
		if dt == "" {
			dt = "string"
		}
		f := vctypes.FieldSpec{Name: strings.TrimSpace(name), Datatype: dt, Required: req}
		if strings.Contains(dt, ":") {
			parts := strings.SplitN(dt, ":", 2)
			f.Datatype = parts[0]
			f.Format = parts[1]
		}
		d.Fields = append(d.Fields, f)
	}
	return d
}

func currentBuilderSchema(sess *Session, d builderData) vctypes.Schema {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = "Untitled schema"
	}
	desc := strings.TrimSpace(d.Desc)
	if desc == "" {
		desc = "—"
	}
	s := vctypes.Schema{
		ID:              "custom-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		Name:            name,
		Desc:            desc,
		Std:             d.Std,
		DPGs:            []string{sess.IssuerDpg},
		Custom:          true,
		AdditionalTypes: []string{},
	}
	if strings.TrimSpace(d.ExtraType) != "" {
		s.AdditionalTypes = []string{strings.TrimSpace(d.ExtraType)}
	}
	for _, f := range d.Fields {
		if strings.TrimSpace(f.Name) != "" {
			s.FieldsSpec = append(s.FieldsSpec, f)
		}
	}
	return s
}

func allBlank(fs []vctypes.FieldSpec) bool {
	for _, f := range fs {
		if strings.TrimSpace(f.Name) != "" {
			return false
		}
	}
	return true
}

// buildJSONSchema returns a pretty-printed JSON Schema (draft 2020-12) for the given schema.
// Mirrors the JS buildJsonSchema function. Returns the string so templates can put it
// straight into a <pre> block (escaping happens via html/template).
func buildJSONSchema(s vctypes.Schema) string {
	isW3C := strings.HasPrefix(s.Std, "w3c_vcdm")
	v2 := s.Std == "w3c_vcdm_2"

	types := []string{"VerifiableCredential"}
	types = append(types, s.AdditionalTypes...)

	fields := s.FieldsSpec

	// Build credentialSubject properties
	props := orderedMap{}
	required := []string{}
	for _, f := range fields {
		if f.Name == "" {
			continue
		}
		prop := orderedMap{{"type", f.Datatype}}
		if f.Format != "" {
			prop = append(prop, kv{"format", f.Format})
		}
		props = append(props, kv{f.Name, prop})
		if f.Required {
			required = append(required, f.Name)
		}
	}

	// Build root schema
	schema := orderedMap{
		{"$schema", "https://json-schema.org/draft/2020-12/schema"},
		{"$id", "https://schemas.verifiably.local/" + s.ID + ".json"},
		{"title", s.Name},
		{"description", s.Desc},
		{"type", "object"},
	}

	properties := orderedMap{}

	if isW3C {
		contextURL := "https://www.w3.org/2018/credentials/v1"
		if v2 {
			contextURL = "https://www.w3.org/ns/credentials/v2"
		}
		vocabMap := orderedMap{{"@vocab", "https://vocab.verifiably.local/"}}
		for _, f := range fields {
			if f.Name != "" {
				vocabMap = append(vocabMap, kv{f.Name, "https://vocab.verifiably.local/" + f.Name})
			}
		}
		properties = append(properties,
			kv{"@context", orderedMap{
				{"type", "array"},
				{"const", []any{contextURL, vocabMap}},
			}},
			kv{"type", orderedMap{
				{"type", "array"},
				{"items", orderedMap{{"type", "string"}}},
				{"const", types},
			}},
			kv{"issuer", orderedMap{
				{"type", []string{"string", "object"}},
				{"description", "DID or URL of the issuer"},
			}},
		)
		dateKey := "issuanceDate"
		if v2 {
			dateKey = "validFrom"
		}
		properties = append(properties,
			kv{dateKey, orderedMap{{"type", "string"}, {"format", "date-time"}}},
			kv{"credentialSubject", orderedMap{
				{"type", "object"},
				{"properties", props},
				{"required", required},
			}},
		)
	} else if strings.HasPrefix(s.Std, "sd_jwt_vc") {
		properties = append(properties,
			kv{"type", orderedMap{{"type", "array"}, {"const", types}}},
			kv{"vct", orderedMap{{"type", "string"}, {"const", "https://vct.verifiably.local/" + s.ID}}},
			kv{"iss", orderedMap{{"type", "string"}, {"description", "Issuer identifier"}}},
			kv{"iat", orderedMap{{"type", "integer"}}},
		)
		for _, p := range props {
			properties = append(properties, p)
		}
	} else if s.Std == "jwt_vc" {
		properties = append(properties,
			kv{"type", orderedMap{{"type", "array"}, {"const", types}}},
			kv{"iss", orderedMap{{"type", "string"}}},
			kv{"sub", orderedMap{{"type", "string"}}},
			kv{"vc", orderedMap{
				{"type", "object"},
				{"properties", orderedMap{
					{"type", orderedMap{{"type", "array"}}},
					{"credentialSubject", orderedMap{{"type", "object"}, {"properties", props}}},
				}},
			}},
		)
	} else if s.Std == "mso_mdoc" {
		nsKey := "org.verifiably." + s.ID
		properties = append(properties,
			kv{"type", orderedMap{{"type", "array"}, {"const", types}}},
			kv{"docType", orderedMap{{"type", "string"}, {"const", nsKey}}},
			kv{"nameSpaces", orderedMap{
				{"type", "object"},
				{"properties", orderedMap{{nsKey, orderedMap{{"type", "object"}, {"properties", props}}}}},
			}},
		)
	} else {
		properties = append(properties, kv{"type", orderedMap{{"type", "array"}, {"const", types}}})
	}

	schema = append(schema, kv{"properties", properties})
	if isW3C {
		schema = append(schema, kv{"required", []string{"@context", "type", "issuer", "credentialSubject"}})
	} else {
		schema = append(schema, kv{"required", []string{"type"}})
	}

	b, _ := json.MarshalIndent(schema, "", "  ")
	return string(b)
}

// orderedMap is a [][2]any alias that marshals JSON in insertion order.
// Used so the generated JSON Schema fields appear in a deterministic, readable order.
type orderedMap []kv
type kv struct {
	K string
	V any
}

// MarshalJSON for orderedMap and kv — emits a JSON object.
func (o orderedMap) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{")
	for i, entry := range o {
		if i > 0 {
			b.WriteString(",")
		}
		key, _ := json.Marshal(entry.K)
		b.Write(key)
		b.WriteString(":")
		val, err := json.Marshal(entry.V)
		if err != nil {
			return nil, err
		}
		b.Write(val)
	}
	b.WriteString("}")
	return []byte(b.String()), nil
}

// MarshalJSON for kv — never called directly (kv is always inside an orderedMap),
// but present so encoding/json doesn't complain if someone marshals one.
func (k kv) MarshalJSON() ([]byte, error) {
	return json.Marshal(orderedMap{k})
}
