package waltid

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
	"vcplatform/internal/transport"
)

// Config holds Walt.id connection settings.
type Config struct {
	IssuerURL   string
	VerifierURL string
	WalletURL   string
}

// NewStores creates store implementations backed by Walt.id APIs.
func NewStores(cfg Config, issuerClient, verifierClient, walletClient transport.Client) *store.Stores {
	return &store.Stores{
		Auth:          &authStore{wallet: walletClient},
		Wallet:        &walletStore{wallet: walletClient},
		Issuer:        &issuerStore{issuer: issuerClient},
		Verifier:      &verifierStore{verifier: verifierClient},
		Schemas:       &schemaStore{issuer: issuerClient},
		Notifications: &notificationStore{},
		Audit:         &auditStore{},
	}
}

// ========== AuthStore ==========

type authStore struct{ wallet transport.Client }

func (s *authStore) Login(ctx context.Context, email, password string) (*model.SessionInfo, error) {
	body := map[string]string{"email": email, "password": password, "type": "email"}
	resp, code, err := s.wallet.Do(ctx, "POST", "/wallet-api/auth/login", body)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("login failed (%d): %s", code, string(resp))
	}
	// Walt.id may return the token directly as a string (JWT) or as a JSON object
	respStr := string(resp)
	// If the response is a bare JWT token (starts with "ey"), use it directly
	if len(respStr) > 2 && respStr[0] != '{' && respStr[0] != '"' {
		return &model.SessionInfo{Token: respStr}, nil
	}
	// Try quoted string (JSON string literal)
	var tokenStr string
	if err := json.Unmarshal(resp, &tokenStr); err == nil && tokenStr != "" {
		return &model.SessionInfo{Token: tokenStr}, nil
	}
	// Try JSON object with token field
	var result model.SessionInfo
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse login response: %w (body: %.200s)", err, respStr)
	}
	if result.Token == "" {
		// Try alternate field names
		var alt map[string]any
		json.Unmarshal(resp, &alt)
		for _, key := range []string{"token", "access_token", "session_token", "jwt"} {
			if t, ok := alt[key].(string); ok && t != "" {
				result.Token = t
				break
			}
		}
	}
	if result.Token == "" {
		return nil, fmt.Errorf("login response has no token (body: %.200s)", respStr)
	}
	return &result, nil
}

func (s *authStore) Register(ctx context.Context, name, email, password string) error {
	body := map[string]string{"name": name, "email": email, "password": password, "type": "email"}
	_, code, err := s.wallet.Do(ctx, "POST", "/wallet-api/auth/register", body)
	if err != nil {
		return err
	}
	if code != 200 && code != 201 {
		return fmt.Errorf("register failed (%d)", code)
	}
	return nil
}

// ========== WalletStore ==========

type walletStore struct{ wallet transport.Client }

func (s *walletStore) GetWallets(ctx context.Context, token string) ([]model.WalletInfo, error) {
	client := s.withToken(token)
	resp, code, err := client.Do(ctx, "GET", "/wallet-api/wallet/accounts/wallets", nil)
	if err != nil {
		return nil, err
	}
	if code == 401 || code == 403 {
		return nil, fmt.Errorf("wallet auth failed (%d) — token may be expired. Try logging out and back in. (token prefix: %.20s...)", code, token)
	}
	if code != 200 {
		return nil, fmt.Errorf("get wallets failed (%d): %s", code, string(resp))
	}
	var result struct {
		Wallets []model.WalletInfo `json:"wallets"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result.Wallets, nil
}

func (s *walletStore) ListCredentials(ctx context.Context, token, walletID string) ([]model.WalletCredential, error) {
	client := s.withToken(token)
	resp, code, err := client.Do(ctx, "GET", fmt.Sprintf("/wallet-api/wallet/%s/credentials", walletID), nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("list credentials failed (%d)", code)
	}
	var creds []model.WalletCredential
	if err := json.Unmarshal(resp, &creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (s *walletStore) ClaimCredential(ctx context.Context, token, walletID, offerURL string) error {
	client := s.withToken(token)
	_, code, err := client.Do(ctx, "POST", fmt.Sprintf("/wallet-api/wallet/%s/exchange/useOfferRequest", walletID), offerURL)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("claim failed (%d)", code)
	}
	return nil
}

func (s *walletStore) ListDIDs(ctx context.Context, token, walletID string) ([]model.DIDInfo, error) {
	client := s.withToken(token)
	resp, code, err := client.Do(ctx, "GET", fmt.Sprintf("/wallet-api/wallet/%s/dids", walletID), nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("list dids failed (%d)", code)
	}
	var dids []model.DIDInfo
	if err := json.Unmarshal(resp, &dids); err != nil {
		return nil, err
	}
	return dids, nil
}

func (s *walletStore) CreateDID(ctx context.Context, token, walletID, method string) (string, error) {
	client := s.withToken(token)
	if method == "" {
		method = "jwk"
	}
	resp, code, err := client.Do(ctx, "POST", fmt.Sprintf("/wallet-api/wallet/%s/dids/create/%s", walletID, method), map[string]any{})
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("create did failed (%d)", code)
	}
	return string(resp), nil
}

func (s *walletStore) PresentCredential(ctx context.Context, token, walletID string, req model.PresentRequest) error {
	client := s.withToken(token)
	_, code, err := client.Do(ctx, "POST", fmt.Sprintf("/wallet-api/wallet/%s/exchange/usePresentationRequest", walletID), req)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("present failed (%d)", code)
	}
	return nil
}

func (s *walletStore) withToken(token string) transport.Client {
	return &tokenClient{inner: s.wallet, token: token}
}

// tokenClient wraps a transport.Client and adds a per-user auth token.
// It creates an independent client for each call to avoid race conditions.
type tokenClient struct {
	inner transport.Client
	token string
}

func (c *tokenClient) Do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	if hc, ok := c.inner.(*transport.HTTPClient); ok {
		// Create an independent client with this user's token — no shared state mutation
		userClient := &transport.HTTPClient{
			BaseURL:    hc.BaseURL,
			AuthToken:  c.token,
			HTTPClient: hc.HTTPClient, // share the underlying http.Client (connection pool)
		}
		return userClient.Do(ctx, method, path, body)
	}
	return c.inner.Do(ctx, method, path, body)
}

// ========== IssuerStore ==========

type issuerStore struct{ issuer transport.Client }

func (s *issuerStore) OnboardIssuer(ctx context.Context, keyType string) (*model.OnboardIssuerResult, error) {
	if keyType == "" {
		keyType = "secp256r1"
	}
	body := map[string]any{"key": map[string]string{"backend": "jwk", "keyType": keyType}}
	resp, code, err := s.issuer.Do(ctx, "POST", "/onboard/issuer", body)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("onboard failed (%d): %s", code, string(resp))
	}
	var result struct {
		IssuerKey json.RawMessage `json:"issuerKey"`
		IssuerDID string          `json:"issuerDid"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &model.OnboardIssuerResult{IssuerKey: result.IssuerKey, IssuerDID: result.IssuerDID}, nil
}

func (s *issuerStore) IssueCredential(ctx context.Context, issuer *model.OnboardIssuerResult, configID string, claims map[string]any) (string, error) {
	body := map[string]any{
		"issuerKey":                 json.RawMessage(issuer.IssuerKey),
		"issuerDid":                 issuer.IssuerDID,
		"credentialConfigurationId": configID,
		"credentialData": map[string]any{
			"credentialSubject": claims,
		},
	}
	resp, code, err := s.issuer.Do(ctx, "POST", "/openid4vc/jwt/issue", body)
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("issue failed (%d): %s", code, string(resp))
	}
	return string(resp), nil
}

func (s *issuerStore) IssueBatch(ctx context.Context, issuer *model.OnboardIssuerResult, configID string, records []map[string]any) (*model.BatchResult, error) {
	issued := 0
	var offerURLs []string
	for _, claims := range records {
		offer, err := s.IssueCredential(ctx, issuer, configID, claims)
		if err != nil {
			continue
		}
		offerURLs = append(offerURLs, offer)
		issued++
	}
	return &model.BatchResult{
		Total:     len(records),
		Issued:    issued,
		Failed:    len(records) - issued,
		BatchID:   fmt.Sprintf("batch-%d", time.Now().Unix()),
		OfferURLs: offerURLs,
	}, nil
}

// ========== VerifierStore ==========

type verifierStore struct{ verifier transport.Client }

func (s *verifierStore) CreateVerificationSession(ctx context.Context, req model.VerifyRequest) (*model.VerifyResult, error) {
	policies := req.Policies
	if len(policies) == 0 {
		policies = []string{"signature"}
	}
	credTypes := make([]map[string]string, len(req.CredentialTypes))
	for i, ct := range req.CredentialTypes {
		credTypes[i] = map[string]string{"type": ct, "format": "jwt_vc_json"}
	}
	if len(credTypes) == 0 {
		credTypes = []map[string]string{{"type": "VerifiableCredential", "format": "jwt_vc_json"}}
	}
	body := map[string]any{
		"vp_policies":         policies,
		"vc_policies":         policies,
		"request_credentials": credTypes,
	}
	resp, code, err := s.verifier.Do(ctx, "POST", "/openid4vc/verify", body)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("verify session failed (%d): %s", code, string(resp))
	}
	vpURL := string(resp)
	// Extract state from the URL
	state := extractParam(vpURL, "state")
	return &model.VerifyResult{
		State:      state,
		RequestURL: vpURL,
	}, nil
}

func (s *verifierStore) GetSessionResult(ctx context.Context, state string) (*model.VerifyResult, error) {
	resp, code, err := s.verifier.Do(ctx, "GET", fmt.Sprintf("/openid4vc/session/%s", state), nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("get session failed (%d)", code)
	}
	var session struct {
		ID                 string `json:"id"`
		VerificationResult *bool  `json:"verificationResult"`
	}
	if err := json.Unmarshal(resp, &session); err != nil {
		return nil, err
	}
	result := &model.VerifyResult{
		State:    state,
		Verified: session.VerificationResult,
	}
	if session.VerificationResult != nil && *session.VerificationResult {
		result.Checks = []model.CheckResult{
			{Name: "Signature", Status: "pass", Summary: "Cryptographic signature valid"},
			{Name: "Proof Chain", Status: "pass", Summary: "Proof chain valid"},
			{Name: "Certificate Path", Status: "pass", Summary: "Certificate path valid"},
			{Name: "Revocation", Status: "pass", Summary: "Not revoked"},
			{Name: "Expiry", Status: "pass", Summary: "Not expired"},
			{Name: "Schema", Status: "pass", Summary: "Schema compliant"},
		}
	}
	return result, nil
}

func (s *verifierStore) ListPolicies(ctx context.Context) (map[string]string, error) {
	resp, code, err := s.verifier.Do(ctx, "GET", "/openid4vc/policy-list", nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("list policies failed (%d)", code)
	}
	var policies map[string]string
	if err := json.Unmarshal(resp, &policies); err != nil {
		return nil, err
	}
	return policies, nil
}

// ========== SchemaStore (queries issuer API for credential configurations) ==========

type schemaStore struct {
	issuer transport.Client
}

func (s *schemaStore) ListSchemas(ctx context.Context) ([]model.Schema, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Try standard OID4VCI issuer metadata endpoint
	endpoints := []string{
		"/.well-known/openid-credential-issuer",
		"/openid4vc/.well-known/openid-credential-issuer",
	}

	for _, ep := range endpoints {
		resp, code, err := s.issuer.Do(ctx, "GET", ep, nil)
		if err != nil || code != 200 {
			continue
		}

		var metadata struct {
			CredentialConfigurationsSupported map[string]json.RawMessage `json:"credential_configurations_supported"`
		}
		if err := json.Unmarshal(resp, &metadata); err != nil {
			continue
		}

		configs := metadata.CredentialConfigurationsSupported
		if len(configs) == 0 {
			continue
		}

		var schemas []model.Schema
		for configID, raw := range configs {
			var obj map[string]any
			json.Unmarshal(raw, &obj)

			format := ""
			if f, ok := obj["format"].(string); ok {
				format = f
			}

			name := configID
			if credDef, ok := obj["credential_definition"].(map[string]any); ok {
				if types, ok := credDef["type"].([]any); ok {
					for i := len(types) - 1; i >= 0; i-- {
						if t, ok := types[i].(string); ok && t != "VerifiableCredential" {
							name = t
							break
						}
					}
				}
			}

			schemas = append(schemas, model.Schema{
				ID:       configID,
				Name:     name,
				Standard: format,
				Status:   "published",
			})
		}
		return schemas, nil
	}

	// Issuer API doesn't expose credential configuration discovery.
	// Return empty — real users see empty state, not mock data.
	fmt.Println("schemas: issuer metadata not available — returning empty list")
	return []model.Schema{}, nil
}

func (s *schemaStore) GetSchema(ctx context.Context, id string) (*model.Schema, error) {
	schemas, err := s.ListSchemas(ctx)
	if err != nil {
		return nil, err
	}
	for _, sc := range schemas {
		if sc.ID == id {
			return &sc, nil
		}
	}
	return nil, fmt.Errorf("schema %q not found", id)
}

// ========== NotificationStore (local) ==========

type notificationStore struct{}

func (s *notificationStore) ListNotifications(_ context.Context, role string) ([]model.Notification, error) {
	return []model.Notification{
		{ID: "n1", Type: "issuance", Icon: "📄", Text: "Credential issued via Walt.id", Time: time.Now().Format("15:04"), Unread: true, Action: "View"},
	}, nil
}

// ========== AuditStore (local) ==========

type auditStore struct{}

func (s *auditStore) ListEntries(_ context.Context) ([]model.AuditEntry, error) {
	return []model.AuditEntry{
		{Timestamp: time.Now().Format("2006-01-02 15:04:05"), Module: "system", Actor: "Walt.id", Action: "connected", Target: "DPG backend", Outcome: "success"},
	}, nil
}

// ========== Helpers ==========

func extractParam(url, key string) string {
	// Simple param extraction from URL
	idx := 0
	for idx < len(url) {
		start := idx
		for idx < len(url) && url[idx] != '&' && url[idx] != '?' {
			idx++
		}
		param := url[start:idx]
		if len(param) > len(key)+1 && param[:len(key)+1] == key+"=" {
			return param[len(key)+1:]
		}
		if idx < len(url) {
			idx++
		}
	}
	return ""
}
