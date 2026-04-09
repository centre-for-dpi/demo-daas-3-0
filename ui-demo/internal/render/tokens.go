package render

import (
	"fmt"
	"net/http"
	"strings"

	"vcplatform/internal/config"
)

// GenerateTokensCSS produces a CSS string from the config theme values.
func GenerateTokensCSS(cfg *config.Config) string {
	var b strings.Builder
	l := cfg.Theme.Light
	d := cfg.Theme.Dark

	b.WriteString(":root {\n")
	b.WriteString(fmt.Sprintf("  --font-display: %s;\n", cfg.Typography.FontDisplay))
	b.WriteString(fmt.Sprintf("  --font-body: %s;\n", cfg.Typography.FontBody))
	b.WriteString(fmt.Sprintf("  --font-mono: %s;\n", cfg.Typography.FontMono))
	writeColor(&b, "bg", l.BG)
	writeColor(&b, "bg-surface", l.BGSurface)
	writeColor(&b, "bg-surface-alt", l.BGSurfaceAlt)
	writeColor(&b, "bg-overlay", l.BGOverlay)
	writeColor(&b, "text", l.Text)
	writeColor(&b, "text-secondary", l.TextSecondary)
	writeColor(&b, "text-tertiary", l.TextTertiary)
	writeColor(&b, "border", l.Border)
	writeColor(&b, "border-strong", l.BorderStrong)
	writeColor(&b, "accent", l.Accent)
	writeColor(&b, "accent-hover", l.AccentHover)
	writeColor(&b, "accent-surface", l.AccentSurface)
	writeColor(&b, "accent-text", l.AccentText)
	writeColor(&b, "tag-bg", l.TagBG)
	writeColor(&b, "tag-text", l.TagText)
	writeColor(&b, "success", l.Success)
	writeColor(&b, "warning", l.Warning)
	writeColor(&b, "warning-surface", l.WarningSurface)
	writeColor(&b, "error", l.Error)
	writeColor(&b, "error-surface", l.ErrorSurface)
	writeColor(&b, "info", l.Info)
	writeColor(&b, "info-surface", l.InfoSurface)
	b.WriteString(fmt.Sprintf("  --shadow-sm: %s;\n", l.ShadowSm))
	b.WriteString(fmt.Sprintf("  --shadow-md: %s;\n", l.ShadowMd))
	b.WriteString(fmt.Sprintf("  --shadow-lg: %s;\n", l.ShadowLg))
	b.WriteString("  --radius-sm: 4px;\n")
	b.WriteString("  --radius-md: 8px;\n")
	b.WriteString("  --radius-lg: 16px;\n")
	b.WriteString("  --sidebar-width: 240px;\n")
	b.WriteString("  --topbar-height: 52px;\n")
	b.WriteString("  --grain: url(\"data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.03'/%3E%3C/svg%3E\");\n")
	b.WriteString("}\n\n")

	b.WriteString("[data-theme=\"dark\"] {\n")
	writeColor(&b, "bg", d.BG)
	writeColor(&b, "bg-surface", d.BGSurface)
	writeColor(&b, "bg-surface-alt", d.BGSurfaceAlt)
	writeColor(&b, "bg-overlay", d.BGOverlay)
	writeColor(&b, "text", d.Text)
	writeColor(&b, "text-secondary", d.TextSecondary)
	writeColor(&b, "text-tertiary", d.TextTertiary)
	writeColor(&b, "border", d.Border)
	writeColor(&b, "border-strong", d.BorderStrong)
	writeColor(&b, "accent", d.Accent)
	writeColor(&b, "accent-hover", d.AccentHover)
	writeColor(&b, "accent-surface", d.AccentSurface)
	writeColor(&b, "accent-text", d.AccentText)
	writeColor(&b, "tag-bg", d.TagBG)
	writeColor(&b, "tag-text", d.TagText)
	writeColor(&b, "success", d.Success)
	writeColor(&b, "warning", d.Warning)
	writeColor(&b, "warning-surface", d.WarningSurface)
	writeColor(&b, "error", d.Error)
	writeColor(&b, "error-surface", d.ErrorSurface)
	writeColor(&b, "info", d.Info)
	writeColor(&b, "info-surface", d.InfoSurface)
	b.WriteString(fmt.Sprintf("  --shadow-sm: %s;\n", d.ShadowSm))
	b.WriteString(fmt.Sprintf("  --shadow-md: %s;\n", d.ShadowMd))
	b.WriteString(fmt.Sprintf("  --shadow-lg: %s;\n", d.ShadowLg))
	b.WriteString("}\n")

	return b.String()
}

func writeColor(b *strings.Builder, name, value string) {
	if value != "" {
		b.WriteString(fmt.Sprintf("  --%s: %s;\n", name, value))
	}
}

// TokensCSSHandler returns an HTTP handler that serves the generated tokens.css.
func TokensCSSHandler(cfg *config.Config) http.HandlerFunc {
	css := GenerateTokensCSS(cfg)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		fmt.Fprint(w, css)
	}
}
