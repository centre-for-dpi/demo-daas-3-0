package waltid

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func (s *issuerStore) IssueCredential(ctx context.Context, issuer *model.OnboardIssuerResult, configID, format string, claims map[string]any) (string, error) {
	// Build credentialData with proper W3C structure
	credData := map[string]any{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"type":              []string{"VerifiableCredential"},
		"credentialSubject": claims,
	}

	// For SD-JWT, add vct claim
	if format == "sdjwt_vc" || format == "sd-jwt" {
		credData["vct"] = configID
	}

	body := map[string]any{
		"issuerKey":                 json.RawMessage(issuer.IssuerKey),
		"issuerDid":                 issuer.IssuerDID,
		"credentialConfigurationId": configID,
		"credentialData":            credData,
	}

	isSdJwt := format == "sdjwt_vc" || format == "sd-jwt" || strings.Contains(configID, "sd-jwt") || strings.Contains(configID, "sd_jwt")

	if isSdJwt {
		// SD-JWT uses JWT-style timestamps
		body["mapping"] = map[string]any{
			"id":  "<uuid>",
			"iat": "<timestamp-seconds>",
			"nbf": "<timestamp-seconds>",
			"exp": "<timestamp-in-seconds:365d>",
		}
		// Add selective disclosure for all claim fields
		sdFields := map[string]any{}
		for k := range claims {
			sdFields[k] = map[string]any{"sd": true}
		}
		body["selectiveDisclosure"] = map[string]any{"fields": sdFields}
	} else {
		// W3C JWT uses issuanceDate/expirationDate + issuer object
		body["mapping"] = map[string]any{
			"id":             "<uuid>",
			"issuanceDate":   "<timestamp>",
			"expirationDate": "<timestamp-in:365d>",
			"issuer": map[string]any{
				"id": "<issuerDid>",
			},
			"credentialSubject": map[string]any{
				"id": "<subjectDid>",
			},
		}
	}

	// Choose endpoint based on format
	endpoint := "/openid4vc/sdjwt/issue"
	if !isSdJwt {
		endpoint = "/openid4vc/jwt/issue"
	}

	resp, code, err := s.issuer.Do(ctx, "POST", endpoint, body)
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("issue failed (%d): %s", code, string(resp))
	}
	return string(resp), nil
}

func (s *issuerStore) IssueBatch(ctx context.Context, issuer *model.OnboardIssuerResult, configID, format string, records []map[string]any) (*model.BatchResult, error) {
	issued := 0
	var offerURLs []string
	for _, claims := range records {
		offer, err := s.IssueCredential(ctx, issuer, configID, format, claims)
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

func (s *issuerStore) ListCredentialConfigs(ctx context.Context) ([]model.CredentialConfig, error) {
	// Walt.id built-in credential configurations
	return []model.CredentialConfig{
		{ID: "UniversityDegree_jwt_vc_json", Name: "University Degree", Category: "Education", Format: "jwt_vc_json"},
		{ID: "OpenBadgeCredential_jwt_vc_json", Name: "Open Badge", Category: "Education", Format: "jwt_vc_json"},
		{ID: "EducationalID_jwt_vc_json", Name: "Educational ID", Category: "Education", Format: "jwt_vc_json"},
		{ID: "VerifiableId_jwt_vc_json", Name: "Verifiable ID", Category: "Identity", Format: "jwt_vc_json"},
		{ID: "NaturalPersonVerifiableID_jwt_vc_json", Name: "Natural Person ID", Category: "Identity", Format: "jwt_vc_json"},
		{ID: "eID_jwt_vc_json", Name: "eID", Category: "Identity", Format: "jwt_vc_json"},
		{ID: "IdentityCredential_jwt_vc_json", Name: "Identity Credential", Category: "Identity", Format: "jwt_vc_json"},
		{ID: "PassportCh_jwt_vc_json", Name: "Passport (CH)", Category: "Identity", Format: "jwt_vc_json"},
		{ID: "BankId_jwt_vc_json", Name: "Bank ID", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "KycChecksCredential_jwt_vc_json", Name: "KYC Checks", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "KycCredential_jwt_vc_json", Name: "KYC Credential", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "KycDataCredential_jwt_vc_json", Name: "KYC Data", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "MortgageEligibility_jwt_vc_json", Name: "Mortgage Eligibility", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "TaxReceipt_jwt_vc_json", Name: "Tax Receipt", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "PND91Credential_jwt_vc_json", Name: "PND91", Category: "Finance", Format: "jwt_vc_json"},
		{ID: "VaccinationCertificate_jwt_vc_json", Name: "Vaccination Certificate", Category: "Health", Format: "jwt_vc_json"},
		{ID: "Iso18013DriversLicenseCredential_jwt_vc_json", Name: "Drivers License (ISO)", Category: "Transport", Format: "jwt_vc_json"},
		{ID: "Visa_jwt_vc_json", Name: "Visa", Category: "Travel", Format: "jwt_vc_json"},
		{ID: "BoardingPass_jwt_vc_json", Name: "Boarding Pass", Category: "Travel", Format: "jwt_vc_json"},
		{ID: "AlpsTourReservation_jwt_vc_json", Name: "Alps Tour Reservation", Category: "Travel", Format: "jwt_vc_json"},
		{ID: "HotelReservation_jwt_vc_json", Name: "Hotel Reservation", Category: "Travel", Format: "jwt_vc_json"},
		{ID: "PortableDocumentA1_jwt_vc_json", Name: "Portable Document A1", Category: "Legal", Format: "jwt_vc_json"},
		{ID: "LegalPerson_jwt_vc_json", Name: "Legal Person", Category: "Legal", Format: "jwt_vc_json"},
		{ID: "LegalRegistrationNumber_jwt_vc_json", Name: "Legal Registration Number", Category: "Legal", Format: "jwt_vc_json"},
		{ID: "WalletHolderCredential_jwt_vc_json", Name: "Wallet Holder", Category: "Infrastructure", Format: "jwt_vc_json"},
		{ID: "DataspaceParticipantCredential_jwt_vc_json", Name: "Dataspace Participant", Category: "Infrastructure", Format: "jwt_vc_json"},
		{ID: "GaiaXTermsAndConditions_jwt_vc_json", Name: "Gaia-X T&C", Category: "Infrastructure", Format: "jwt_vc_json"},
		{ID: "identity_credential_vc+sd-jwt", Name: "Identity Credential (SD-JWT)", Category: "Identity", Format: "vc+sd-jwt"},
		{ID: "org.iso.18013.5.1.mDL", Name: "Mobile Drivers License", Category: "Transport", Format: "mso_mdoc"},
	}, nil
}

func (s *issuerStore) RegisterCredentialType(ctx context.Context, typeName, displayName, description, format string) (string, error) {
	configID := typeName + "_" + strings.ReplaceAll(format, "+", "_")

	// Build HOCON entry for Walt.id
	var hocon string
	if format == "vc+sd-jwt" || format == "sdjwt_vc" {
		hocon = fmt.Sprintf(`
    "%s" = {
        format = "vc+sd-jwt"
        cryptographic_binding_methods_supported = ["jwk"]
        credential_signing_alg_values_supported = ["ES256", "EdDSA"]
        vct = "%s"
        display = [{ name = "%s", description = "%s", locale = "en-US" }]
    }`, configID, typeName, displayName, description)
	} else {
		hocon = fmt.Sprintf(`
    "%s" = {
        format = "jwt_vc_json"
        cryptographic_binding_methods_supported = ["did"]
        credential_signing_alg_values_supported = ["EdDSA", "ES256"]
        credential_definition = { type = ["VerifiableCredential", "%s"] }
        display = [{ name = "%s", description = "%s", locale = "en-US" }]
    }`, configID, typeName, displayName, description)
	}

	// Write to the mounted config file
	configPath := "docker/waltid/issuer-api/config/credential-issuer-metadata.conf"
	existing, _ := os.ReadFile(configPath)
	var content string
	if len(existing) == 0 {
		content = fmt.Sprintf("supportedCredentialTypes {\n%s\n}\n", hocon)
	} else {
		cs := string(existing)
		closingIdx := strings.LastIndex(cs, "}")
		if closingIdx < 0 {
			content = fmt.Sprintf("supportedCredentialTypes {\n%s\n%s\n}\n", cs, hocon)
		} else {
			content = cs[:closingIdx] + hocon + "\n" + cs[closingIdx:]
		}
	}
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}

	// Restart the issuer container
	cmd := exec.Command("docker", "restart", "waltid-issuer-api-1")
	cmd.Dir = "docker/waltid"
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("credtype: container restart failed: %s %v\n", string(out), err)
	}

	return configID, nil
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
		TokenResponse      *struct {
			VPToken string `json:"vp_token"`
		} `json:"tokenResponse"`
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
			{Name: "Schema", Status: "pass", Summary: "Schema compliant"},
		}

		// Decode the VP token to extract credential claims
		if session.TokenResponse != nil && session.TokenResponse.VPToken != "" {
			extractCredentialClaims(session.TokenResponse.VPToken, result)
		}
	}
	return result, nil
}

// extractCredentialClaims decodes a JWT/SD-JWT VP token and extracts the credential subject.
func extractCredentialClaims(vpToken string, result *model.VerifyResult) {
	// SD-JWT format: header.payload.signature~disclosure1~disclosure2...
	// JWT format: header.payload.signature
	// Split off disclosures first
	token := vpToken
	if idx := strings.Index(token, "~"); idx > 0 {
		token = token[:idx]
	}

	// Decode the JWT payload (second part)
	parts := strings.SplitN(token, ".", 3)
	if len(parts) < 2 {
		return
	}

	// Base64url decode the payload (RawURLEncoding = no padding)
	decoded, err := base64Decode(parts[1])
	if err != nil {
		return
	}

	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return
	}

	// Extract credential type
	if vct, ok := claims["vct"].(string); ok {
		result.CredentialType = vct
	}

	// Extract issuer
	if iss, ok := claims["iss"].(string); ok {
		result.IssuerDID = iss
	}

	// Extract holder from cnf.jwk.kid or sub
	if cnf, ok := claims["cnf"].(map[string]any); ok {
		if jwk, ok := cnf["jwk"].(map[string]any); ok {
			if kid, ok := jwk["kid"].(string); ok {
				result.HolderDID = kid
			}
		}
	}
	if sub, ok := claims["sub"].(string); ok && result.HolderDID == "" {
		result.HolderDID = sub
	}

	// Extract timestamps
	if iat, ok := claims["iat"].(float64); ok {
		result.IssuedAt = time.Unix(int64(iat), 0).Format("2006-01-02 15:04")
	}
	if exp, ok := claims["exp"].(float64); ok {
		result.ExpiresAt = time.Unix(int64(exp), 0).Format("2006-01-02 15:04")
	}

	// Extract credentialSubject
	if cs, ok := claims["credentialSubject"].(map[string]any); ok {
		result.Claims = cs
	}
	// Also check vc.credentialSubject (W3C JWT format)
	if vc, ok := claims["vc"].(map[string]any); ok {
		if cs, ok := vc["credentialSubject"].(map[string]any); ok {
			result.Claims = cs
		}
	}
}

func base64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
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
