package main

import (
	"log"
	"os"

	"github.com/verifiably/verifiably-go/internal/adapters/libretranslate"
	"github.com/verifiably/verifiably-go/internal/handlers"
)

// buildTranslator returns a handlers.Translator backed by a LibreTranslate
// client. When LIBRETRANSLATE_URL is unset or empty, returns nil — the UI
// falls back to English-only.
func buildTranslator() handlers.Translator {
	url := os.Getenv("LIBRETRANSLATE_URL")
	if url == "" {
		url = "http://localhost:5000"
	}
	dir := os.Getenv("VERIFIABLY_LOCALES_DIR")
	if dir == "" {
		dir = "locales"
	}
	c := libretranslate.New(url, dir)
	log.Printf("i18n: LibreTranslate at %s, locales dir %s", url, dir)
	return c
}
