package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Brand       Brand       `json:"brand"`
	Theme       Theme       `json:"theme"`
	Typography  Typography  `json:"typography"`
	Features    Features    `json:"features"`
	Backend     Backend     `json:"backend"`
	SSO         SSO         `json:"sso"`
	Translation Translation `json:"translation"`
	Mode        string      `json:"mode"`
	Locale      string      `json:"locale"`
}

type Translation struct {
	DeepLAPIKey      string   `json:"deepl_api_key"`
	LibreTranslateURL string  `json:"libretranslate_url"` // e.g. http://localhost:5000
	Languages        []string `json:"languages"`          // e.g. ["EN","ES","FR","PT","HI","SW"]
}

type Brand struct {
	Name       string `json:"name"`
	NameAccent string `json:"name_accent"`
}

// NameParts splits the brand name around the accent character.
func (b Brand) NameBefore() string {
	for i, c := range b.Name {
		if string(c) == b.NameAccent {
			return b.Name[:i]
		}
	}
	return b.Name
}

func (b Brand) NameAfter() string {
	for i, c := range b.Name {
		if string(c) == b.NameAccent {
			return b.Name[i+len(b.NameAccent):]
		}
	}
	return ""
}

type ThemeColors struct {
	BG            string `json:"bg"`
	BGSurface     string `json:"bg_surface"`
	BGSurfaceAlt  string `json:"bg_surface_alt"`
	BGOverlay     string `json:"bg_overlay"`
	Text          string `json:"text"`
	TextSecondary string `json:"text_secondary"`
	TextTertiary  string `json:"text_tertiary"`
	Border        string `json:"border"`
	BorderStrong  string `json:"border_strong"`
	Accent        string `json:"accent"`
	AccentHover   string `json:"accent_hover"`
	AccentSurface string `json:"accent_surface"`
	AccentText    string `json:"accent_text"`
	TagBG         string `json:"tag_bg"`
	TagText       string `json:"tag_text"`
	Success       string `json:"success"`
	Warning       string `json:"warning"`
	WarningSurface string `json:"warning_surface"`
	Error         string `json:"error"`
	ErrorSurface  string `json:"error_surface"`
	Info          string `json:"info"`
	InfoSurface   string `json:"info_surface"`
	ShadowSm     string `json:"shadow_sm"`
	ShadowMd     string `json:"shadow_md"`
	ShadowLg     string `json:"shadow_lg"`
}

type Theme struct {
	Light ThemeColors `json:"light"`
	Dark  ThemeColors `json:"dark"`
}

type Typography struct {
	FontDisplay string   `json:"font_display"`
	FontBody    string   `json:"font_body"`
	FontMono    string   `json:"font_mono"`
	FontURLs    []string `json:"font_urls"`
}

type Workspaces struct {
	Issuer   bool `json:"issuer"`
	Holder   bool `json:"holder"`
	Verifier bool `json:"verifier"`
	Trust    bool `json:"trust"`
	Admin    bool `json:"admin"`
}

type Features struct {
	Workspaces      Workspaces `json:"workspaces"`
	ExplorationMode bool       `json:"exploration_mode"`
	Chatbot         bool       `json:"chatbot"`
}

type Backend struct {
	Type        string `json:"type"`         // "mock", "waltid"
	IssuerURL   string `json:"issuer_url"`   // e.g. http://localhost:7002
	VerifierURL string `json:"verifier_url"` // e.g. http://localhost:7003
	WalletURL   string `json:"wallet_url"`   // e.g. http://localhost:7001
	Transport   string `json:"transport"`    // "direct", "n8n", "openfn"
	WebhookURL  string `json:"webhook_url"`  // For n8n/openfn transport
}

type SSOProvider struct {
	Name         string `json:"name"`
	DiscoveryURL string `json:"discovery_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scopes       string `json:"scopes,omitempty"`
}

type SSO struct {
	Enabled        bool          `json:"enabled"`
	BaseURL        string        `json:"base_url"`          // e.g. http://localhost:8080
	CACertPath     string        `json:"ca_cert_path"`      // Path to custom CA cert
	SkipTLSVerify  bool          `json:"skip_tls_verify"`   // For local dev with expired/self-signed certs
	Providers      []SSOProvider `json:"providers"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand $ENV_VAR references in the JSON before parsing
	expanded := os.Expand(string(raw), func(key string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return "$" + key // leave unresolved vars as-is
	})

	cfg := Default()
	if err := json.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
