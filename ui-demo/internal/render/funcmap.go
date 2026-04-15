package render

import (
	"encoding/json"
	"html/template"
	"reflect"

	"vcplatform/internal/config"
)

func buildFuncMap(cfg *config.Config) template.FuncMap {
	return template.FuncMap{
		// roleIs checks if the user's role matches any of the provided roles.
		"roleIs": func(userRole string, allowed ...string) bool {
			for _, r := range allowed {
				if userRole == r {
					return true
				}
			}
			return false
		},

		// hasWorkspace checks if a workspace is enabled.
		"hasWorkspace": func(workspace string) bool {
			switch workspace {
			case "issuer":
				return cfg.Features.Workspaces.Issuer
			case "holder":
				return cfg.Features.Workspaces.Holder
			case "verifier":
				return cfg.Features.Workspaces.Verifier
			case "trust":
				return cfg.Features.Workspaces.Trust
			case "admin":
				return cfg.Features.Workspaces.Admin
			}
			return false
		},

		// safeURL marks a string as a safe URL (prevents encoding of & and + in URLs).
		"safeURL": func(s string) template.URL {
			return template.URL(s)
		},

		// safeHTML marks a string as safe HTML.
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},

		// safeCSS marks a string as safe CSS.
		"safeCSS": func(s string) template.CSS {
			return template.CSS(s)
		},

		// activeNav returns "active" if the current page matches.
		"activeNav": func(activePage, target string) string {
			if activePage == target {
				return "active"
			}
			return ""
		},

		// hasItems returns true if a slice/array has at least one item.
		"hasItems": func(v any) bool {
			if v == nil {
				return false
			}
			// Use reflect for any slice type
			rv := reflect.ValueOf(v)
			if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
				return rv.Len() > 0
			}
			return false
		},

		// dataMap safely casts .Data to map[string]any.
		"dataMap": func(data any) map[string]any {
			if m, ok := data.(map[string]any); ok {
				return m
			}
			return nil
		},

		// marshal serializes a value as JSON for inlining into a <script>
		// block. Returns template.JS so html/template doesn't escape quotes.
		"marshal": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},

		// mapGet retrieves a key from a map[string]any.
		"mapGet": func(m map[string]any, key string) any {
			if m == nil {
				return nil
			}
			return m[key]
		},

		// mapStr retrieves a string value from a map[string]any.
		"mapStr": func(m map[string]any, key string) string {
			if m == nil {
				return ""
			}
			v, _ := m[key].(string)
			return v
		},

		// credTypes extracts the type array from a parsed credential document.
		"credTypes": func(pd map[string]any) string {
			types, ok := pd["type"].([]any)
			if !ok || len(types) == 0 {
				return "Credential"
			}
			// Return the last type (most specific)
			return types[len(types)-1].(string)
		},

		// credSubject extracts credentialSubject from parsed doc.
		"credSubject": func(pd map[string]any) map[string]any {
			subj, ok := pd["credentialSubject"].(map[string]any)
			if !ok {
				return map[string]any{}
			}
			return subj
		},
	}
}
