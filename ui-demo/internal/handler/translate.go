package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var translateClient = &http.Client{Timeout: 30 * time.Second}

// Server-side translation cache: "lang:text" -> translated
var (
	transCache   = map[string]string{}
	transCacheMu sync.RWMutex
)

// DeepL supported target languages (subset — full list at deepl.com/docs-api)
var deeplLangs = map[string]bool{
	"BG": true, "CS": true, "DA": true, "DE": true, "EL": true,
	"EN": true, "EN-GB": true, "EN-US": true, "ES": true, "ET": true,
	"FI": true, "FR": true, "HU": true, "ID": true, "IT": true,
	"JA": true, "KO": true, "LT": true, "LV": true, "NB": true,
	"NL": true, "PL": true, "PT": true, "PT-BR": true, "PT-PT": true,
	"RO": true, "RU": true, "SK": true, "SL": true, "SV": true,
	"TR": true, "UK": true, "ZH": true,
}

// APITranslate handles POST /api/translate.
// Tries DeepL first (better quality), falls back to LibreTranslate for unsupported languages.
func (h *Handler) APITranslate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Texts  []string `json:"texts"`
		Target string   `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if len(req.Texts) == 0 || req.Target == "" {
		writeJSON(w, 400, map[string]string{"error": "texts and target are required"})
		return
	}
	if len(req.Texts) > 200 {
		req.Texts = req.Texts[:200]
	}

	target := strings.ToUpper(req.Target)
	deeplKey := h.config.Translation.DeepLAPIKey
	libreURL := h.config.Translation.LibreTranslateURL

	// Check server cache — split into cached hits and uncached misses
	results := make([]string, len(req.Texts))
	var uncached []string
	var uncachedIdx []int

	transCacheMu.RLock()
	for i, text := range req.Texts {
		key := target + ":" + text
		if cached, ok := transCache[key]; ok {
			results[i] = cached
		} else {
			uncachedIdx = append(uncachedIdx, i)
			uncached = append(uncached, text)
		}
	}
	transCacheMu.RUnlock()

	// All cached — return immediately
	if len(uncached) == 0 {
		writeJSON(w, 200, map[string]any{"translations": results, "provider": "cache"})
		return
	}

	// Translate only uncached strings
	var translations []string
	var provider string

	if deeplKey != "" && deeplLangs[target] {
		t, err := callDeepL(deeplKey, uncached, target)
		if err == nil {
			translations = t
			provider = "deepl"
		} else {
			fmt.Printf("translate: DeepL failed for %s: %v\n", target, err)
		}
	}

	if translations == nil && libreURL != "" {
		t, err := callLibreTranslate(libreURL, uncached, target)
		if err == nil {
			translations = t
			provider = "libretranslate"
		} else {
			fmt.Printf("translate: LibreTranslate failed for %s: %v\n", target, err)
		}
	}

	if translations != nil {
		// Merge translated strings back and cache them
		transCacheMu.Lock()
		for i, idx := range uncachedIdx {
			if i < len(translations) {
				results[idx] = translations[i]
				transCache[target+":"+uncached[i]] = translations[i]
			}
		}
		transCacheMu.Unlock()

		writeJSON(w, 200, map[string]any{"translations": results, "provider": provider})
		return
	}

	// Neither provider available
	if deeplKey == "" && libreURL == "" {
		writeJSON(w, 503, map[string]string{"error": "no translation provider configured — set DEEPL_API_KEY or LIBRETRANSLATE_URL"})
	} else {
		writeJSON(w, 502, map[string]string{"error": fmt.Sprintf("translation to %s failed — language may not be supported", target)})
	}
}

// APITranslationConfig returns available languages and providers.
func (h *Handler) APITranslationConfig(w http.ResponseWriter, r *http.Request) {
	available := h.config.Translation.Languages
	if len(available) == 0 {
		available = []string{"EN", "ES", "FR", "PT-BR", "HI", "SW"}
	}
	providers := []string{}
	if h.config.Translation.DeepLAPIKey != "" {
		providers = append(providers, "deepl")
	}
	if h.config.Translation.LibreTranslateURL != "" {
		providers = append(providers, "libretranslate")
	}
	writeJSON(w, 200, map[string]any{
		"enabled":   len(providers) > 0,
		"providers": providers,
		"languages": available,
	})
}

// ========== DeepL ==========

func callDeepL(apiKey string, texts []string, targetLang string) ([]string, error) {
	apiURL := "https://api.deepl.com/v2/translate"
	if strings.Contains(apiKey, ":fx") {
		apiURL = "https://api-free.deepl.com/v2/translate"
	}

	form := url.Values{}
	for _, t := range texts {
		form.Add("text", t)
	}
	form.Set("target_lang", targetLang)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "DeepL-Auth-Key "+apiKey)

	resp, err := translateClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	out := make([]string, len(result.Translations))
	for i, t := range result.Translations {
		out[i] = t.Text
	}
	return out, nil
}

// ========== LibreTranslate ==========

func callLibreTranslate(baseURL string, texts []string, targetLang string) ([]string, error) {
	target := strings.ToLower(targetLang)
	if idx := strings.Index(target, "-"); idx > 0 {
		target = target[:idx]
	}

	// LibreTranslate supports batch: pass array of strings in "q"
	body := map[string]any{
		"q":      texts,
		"source": "en",
		"target": target,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", baseURL+"/translate", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := translateClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	// Response is {"translatedText": "..."} for single, or {"translatedText": ["...", "..."]} for batch
	var single struct {
		TranslatedText string `json:"translatedText"`
	}
	if json.Unmarshal(respBody, &single) == nil && single.TranslatedText != "" {
		return []string{single.TranslatedText}, nil
	}

	var batch struct {
		TranslatedText []string `json:"translatedText"`
	}
	if err := json.Unmarshal(respBody, &batch); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return batch.TranslatedText, nil
}
