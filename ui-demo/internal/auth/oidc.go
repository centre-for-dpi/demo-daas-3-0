// Package auth provides OIDC authentication against any compliant provider.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// httpClient is the HTTP client used for OIDC operations.
// Initialized with default settings; call InitHTTPClient to add custom CA certs.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// InitHTTPClient configures the OIDC HTTP client.
// If caCertPath is set, trusts that CA cert.
// If skipTLS is true, skips all TLS verification (local dev only).
func InitHTTPClient(caCertPath string, skipTLS bool) {
	if skipTLS {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		fmt.Println("oidc: TLS verification disabled (local dev mode)")
		return
	}
	if caCertPath == "" {
		return
	}
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		fmt.Printf("oidc: CA cert %s not found, using system certs\n", caCertPath)
		return
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		fmt.Printf("oidc: failed to parse CA cert %s\n", caCertPath)
		return
	}
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
	fmt.Printf("oidc: loaded custom CA cert from %s\n", caCertPath)
}

// Provider represents a configured OIDC identity provider.
type Provider struct {
	Name         string `json:"name"`
	DiscoveryURL string `json:"discovery_url"` // .well-known/openid-configuration
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	Scopes       string `json:"scopes"` // space-separated

	// Cached discovery data
	mu            sync.Mutex
	authEndpoint  string
	tokenEndpoint string
	userEndpoint  string
	discovered    bool
}

// OIDCTokenResponse is the token endpoint response.
type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OIDCUserInfo is a minimal set of user info claims.
type OIDCUserInfo struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername  string `json:"preferred_username"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
}

// Discover fetches the OIDC discovery document and caches endpoints.
func (p *Provider) Discover(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.discovered {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", p.DiscoveryURL, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discover %s: %w", p.Name, err)
	}
	defer resp.Body.Close()

	var disco struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		UserinfoEndpoint      string `json:"userinfo_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&disco); err != nil {
		return fmt.Errorf("parse discovery %s: %w", p.Name, err)
	}

	p.authEndpoint = disco.AuthorizationEndpoint
	p.tokenEndpoint = disco.TokenEndpoint
	p.userEndpoint = disco.UserinfoEndpoint
	p.discovered = true
	return nil
}

// AuthorizeURL returns the URL to redirect the user to for authentication.
func (p *Provider) AuthorizeURL(state string) (string, error) {
	if !p.discovered {
		return "", fmt.Errorf("provider %s not discovered", p.Name)
	}
	scopes := p.Scopes
	if scopes == "" {
		scopes = "openid profile email"
	}
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {p.ClientID},
		"redirect_uri":  {p.RedirectURI},
		"scope":         {scopes},
		"state":         {state},
		"prompt":        {"login"},
	}
	return p.authEndpoint + "?" + params.Encode(), nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *Provider) ExchangeCode(ctx context.Context, code string) (*OIDCTokenResponse, error) {
	if !p.discovered {
		return nil, fmt.Errorf("provider %s not discovered", p.Name)
	}

	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {p.RedirectURI},
		"client_id":    {p.ClientID},
	}
	if p.ClientSecret != "" {
		data.Set("client_secret", p.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange %s: %w", p.Name, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange %s: %d %s", p.Name, resp.StatusCode, string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	return &tokenResp, nil
}

// GetUserInfo fetches user info using the access token.
func (p *Provider) GetUserInfo(ctx context.Context, accessToken string) (*OIDCUserInfo, error) {
	if p.userEndpoint == "" {
		return nil, fmt.Errorf("no userinfo endpoint for %s", p.Name)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", p.userEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info OIDCUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GenerateState creates a cryptographically random state parameter.
func GenerateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// Registry holds all configured OIDC providers.
type Registry struct {
	providers map[string]*Provider
}

// NewRegistry creates a registry from config.
func NewRegistry(providers []Provider, appBaseURL string) *Registry {
	r := &Registry{providers: make(map[string]*Provider)}
	for i := range providers {
		p := &providers[i]
		if p.RedirectURI == "" {
			p.RedirectURI = appBaseURL + "/auth/callback"
		}
		r.providers[strings.ToLower(p.Name)] = p
	}
	return r
}

// Get returns a provider by name (case-insensitive).
func (r *Registry) Get(name string) *Provider {
	return r.providers[strings.ToLower(name)]
}

// Names returns all registered provider names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for k := range r.providers {
		names = append(names, k)
	}
	return names
}

// DiscoverAll discovers all providers (best-effort, logs failures).
func (r *Registry) DiscoverAll(ctx context.Context) {
	for _, p := range r.providers {
		if err := p.Discover(ctx); err != nil {
			fmt.Printf("oidc: failed to discover %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("oidc: discovered %s (%s)\n", p.Name, p.DiscoveryURL)
		}
	}
}
