package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed static
var staticEmbed embed.FS

//go:embed templates
var templateEmbed embed.FS

// StaticFS returns the embedded static filesystem rooted at "static/".
var StaticFS, _ = fs.Sub(staticEmbed, "static")

// TemplateFS returns the embedded template filesystem rooted at "templates/".
var TemplateFS, _ = fs.Sub(templateEmbed, "templates")

// StaticHandler returns an http.Handler that serves static files,
// checking the custom directory first, then falling back to embedded.
func StaticHandler(customDir string) http.Handler {
	customStatic := filepath.Join(customDir, "static")

	embedded := http.FileServerFS(StaticFS)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cache static assets for 1 hour (immutable content changes on rebuild)
		w.Header().Set("Cache-Control", "public, max-age=3600")

		// Try custom dir first
		localPath := filepath.Join(customStatic, r.URL.Path)
		if _, err := os.Stat(localPath); err == nil {
			http.ServeFile(w, r, localPath)
			return
		}
		// Fall back to embedded
		embedded.ServeHTTP(w, r)
	})
}
