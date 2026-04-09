package render

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"vcplatform/internal/config"
	"vcplatform/internal/model"
)

// PageData is the standard data passed to every template.
type PageData struct {
	Config     *config.Config
	User       *model.User
	Mode       string
	Breadcrumb []model.BreadcrumbItem
	IsHTMX     bool
	ActivePage string
	Data       any
}

// Renderer holds parsed template sets.
type Renderer struct {
	templates map[string]*template.Template
	funcMap   template.FuncMap
}

// landingDirs use the landing layout; everything else uses portal.
var landingDirs = map[string]bool{
	"landing": true,
	"auth":    true,
}

// New initializes the renderer by parsing all templates from embedded and custom directories.
func New(_ fs.FS, templateFS fs.FS, customDir string, cfg *config.Config) (*Renderer, error) {
	r := &Renderer{
		templates: make(map[string]*template.Template),
		funcMap:   buildFuncMap(cfg),
	}

	partials, err := fs.Glob(templateFS, "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob partials: %w", err)
	}

	dirs := []string{"landing", "auth", "portal", "issuer", "holder", "verifier", "trust", "admin"}
	for _, dir := range dirs {
		entries, err := fs.Glob(templateFS, dir+"/*.html")
		if err != nil {
			continue
		}

		// Pick the correct layout for this directory
		layout := "layouts/portal.html"
		if landingDirs[dir] {
			layout = "layouts/landing.html"
		}

		// Build the set of base files: base layout + specific layout + all partials
		baseFiles := []string{"layouts/base.html", layout}
		baseFiles = append(baseFiles, partials...)

		for _, entry := range entries {
			name := strings.TrimSuffix(entry, ".html")

			tmplContent, err := r.readTemplate(templateFS, customDir, entry)
			if err != nil {
				continue
			}

			t := template.New("base").Funcs(r.funcMap)

			// Parse base files in order
			for _, bf := range baseFiles {
				content, bfErr := r.readTemplate(templateFS, customDir, bf)
				if bfErr != nil {
					continue
				}
				if _, parseErr := t.Parse(content); parseErr != nil {
					return nil, fmt.Errorf("parse %s for %s: %w", bf, entry, parseErr)
				}
			}

			// Parse the screen template last (overrides "content" block)
			if _, parseErr := t.Parse(tmplContent); parseErr != nil {
				return nil, fmt.Errorf("parse screen %s: %w", entry, parseErr)
			}

			r.templates[name] = t
		}
	}

	return r, nil
}

// readTemplate reads a template file, checking custom dir first then embedded.
func (r *Renderer) readTemplate(embeddedFS fs.FS, customDir string, name string) (string, error) {
	customPath := filepath.Join(customDir, "templates", name)
	if data, err := os.ReadFile(customPath); err == nil {
		return string(data), nil
	}
	data, err := fs.ReadFile(embeddedFS, name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Render executes a named template. If IsHTMX is true, renders only the "content" block
// plus OOB swaps for breadcrumb.
func (r *Renderer) Render(w io.Writer, name string, data PageData) error {
	t, ok := r.templates[name]
	if !ok {
		if hw, isHttp := w.(http.ResponseWriter); isHttp {
			http.Error(hw, fmt.Sprintf("template %q not found", name), http.StatusInternalServerError)
		}
		return fmt.Errorf("template %q not found", name)
	}

	if data.IsHTMX {
		// Render main content
		if err := t.ExecuteTemplate(w, "content", data); err != nil {
			return err
		}
		// Append OOB breadcrumb swap
		return r.renderBreadcrumbOOB(w, data)
	}
	return t.ExecuteTemplate(w, "base", data)
}

// renderBreadcrumbOOB appends an out-of-band swap for the breadcrumb element.
func (r *Renderer) renderBreadcrumbOOB(w io.Writer, data PageData) error {
	if len(data.Breadcrumb) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(`<div id="breadcrumb" class="topbar-breadcrumb" hx-swap-oob="true">`)
	for _, item := range data.Breadcrumb {
		if item.Active {
			b.WriteString("<strong>")
			b.WriteString(template.HTMLEscapeString(item.Label))
			b.WriteString("</strong>")
		} else {
			b.WriteString(template.HTMLEscapeString(item.Label))
			b.WriteString(" / ")
		}
	}
	b.WriteString("</div>")
	_, err := io.WriteString(w, b.String())
	return err
}
