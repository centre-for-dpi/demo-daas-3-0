package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"vcplatform/internal/model"
	"vcplatform/internal/store"
)

func NewStores() *store.Stores {
	return &store.Stores{
		Auth:          &authStore{},
		Wallet:        &walletStore{},
		Issuer:        &issuerStore{},
		Verifier:      &verifierStore{},
		Schemas:       &schemaStore{},
		Notifications: &notificationStore{},
		Audit:         &auditStore{},
	}
}

// --- AuthStore ---
type authStore struct{}

func (s *authStore) Login(_ context.Context, email, password string) (*model.SessionInfo, error) {
	return nil, fmt.Errorf("no backend configured — use Demo mode or configure a real backend (waltid, keycloak, etc.)")
}
func (s *authStore) Register(_ context.Context, name, email, password string) error {
	return fmt.Errorf("no backend configured — use Demo mode or configure a real backend")
}

// --- WalletStore ---
type walletStore struct{}

func (s *walletStore) GetWallets(_ context.Context, token string) ([]model.WalletInfo, error) {
	return []model.WalletInfo{{ID: "mock-wallet", Name: "Mock Wallet"}}, nil
}

func (s *walletStore) ListCredentials(_ context.Context, token, walletID string) ([]model.WalletCredential, error) {
	return []model.WalletCredential{
		{ID: "mock-cred-1", Format: "jwt_vc_json", AddedOn: "2025-06-15", ParsedDocument: map[string]any{
			"type": []any{"VerifiableCredential", "UniversityDegree"},
			"credentialSubject": map[string]any{"name": "Jane Doe", "degree": "BSc Computer Science", "university": "University of the West Indies", "graduationDate": "2025-06-15", "gpa": "3.7", "studentId": "816003245"},
		}},
		{ID: "mock-cred-2", Format: "jwt_vc_json", AddedOn: "2026-01-10", ParsedDocument: map[string]any{
			"type": []any{"VerifiableCredential", "ProfessionalCertificate"},
			"credentialSubject": map[string]any{"name": "Jane Doe", "certificate": "Data Analytics", "institution": "WeLearnTT"},
		}},
	}, nil
}

func (s *walletStore) ClaimCredential(_ context.Context, token, walletID, offerURL string) error {
	return nil
}

func (s *walletStore) ListDIDs(_ context.Context, token, walletID string) ([]model.DIDInfo, error) {
	return []model.DIDInfo{
		{DID: "did:jwk:mock-key-1", Alias: "Primary", KeyID: "key-1", Default: true},
	}, nil
}

func (s *walletStore) CreateDID(_ context.Context, token, walletID, method string) (string, error) {
	return "did:jwk:mock-new-key", nil
}

func (s *walletStore) PresentCredential(_ context.Context, token, walletID string, req model.PresentRequest) error {
	return nil
}

// --- IssuerStore ---
type issuerStore struct{ counter int }

func (s *issuerStore) OnboardIssuer(_ context.Context, keyType string) (*model.OnboardIssuerResult, error) {
	return &model.OnboardIssuerResult{
		IssuerKey: json.RawMessage(`{"type":"jwk","jwk":{"kty":"EC","crv":"P-256"}}`),
		IssuerDID: "did:jwk:mock-issuer",
	}, nil
}

func (s *issuerStore) IssueCredential(_ context.Context, issuer *model.OnboardIssuerResult, configID string, claims map[string]any) (string, error) {
	s.counter++
	return fmt.Sprintf("openid-credential-offer://mock?id=mock-offer-%d", s.counter), nil
}

func (s *issuerStore) IssueBatch(_ context.Context, issuer *model.OnboardIssuerResult, configID string, records []map[string]any) (*model.BatchResult, error) {
	return &model.BatchResult{Total: len(records), Issued: len(records), Failed: 0, BatchID: "mock-batch-1"}, nil
}

// --- VerifierStore ---
type verifierStore struct{ counter int }

func (s *verifierStore) CreateVerificationSession(_ context.Context, req model.VerifyRequest) (*model.VerifyResult, error) {
	s.counter++
	return &model.VerifyResult{
		SessionID:  fmt.Sprintf("mock-session-%d", s.counter),
		State:      fmt.Sprintf("mock-state-%d", s.counter),
		RequestURL: fmt.Sprintf("openid4vp://authorize?state=mock-state-%d", s.counter),
	}, nil
}

func (s *verifierStore) GetSessionResult(_ context.Context, state string) (*model.VerifyResult, error) {
	verified := true
	return &model.VerifyResult{
		State:    state,
		Verified: &verified,
		Checks: []model.CheckResult{
			{Name: "Signature", Status: "pass", Summary: "Signature valid"},
			{Name: "Proof Chain", Status: "pass", Summary: "Proof chain valid"},
			{Name: "Certificate Path", Status: "pass", Summary: "Certificate path valid"},
			{Name: "Revocation", Status: "pass", Summary: "Not revoked"},
			{Name: "Expiry", Status: "pass", Summary: "Not expired"},
			{Name: "Schema", Status: "pass", Summary: "Schema valid"},
		},
	}, nil
}

func (s *verifierStore) ListPolicies(_ context.Context) (map[string]string, error) {
	return map[string]string{
		"signature": "Checks cryptographic signature",
		"expired":   "Checks expiration date",
	}, nil
}

// --- SchemaStore ---
type schemaStore struct{}

func (s *schemaStore) ListSchemas(_ context.Context) ([]model.Schema, error) {
	return []model.Schema{
		{ID: "bsc-degree-v2", Name: "BSc Degree", Version: "2.0", Standard: "W3C-VCDM 2.0", Status: "published", FieldCount: 12, CreatedAt: "2026-01-15"},
		{ID: "msc-degree-v1", Name: "MSc Degree", Version: "1.0", Standard: "W3C-VCDM 2.0", Status: "published", FieldCount: 14, CreatedAt: "2026-02-01"},
		{ID: "professional-cert-v2", Name: "Professional Certificate", Version: "2.0", Standard: "SD-JWT", Status: "published", FieldCount: 9, CreatedAt: "2026-02-20"},
		{ID: "land-cert-v1", Name: "Land Certificate", Version: "1.0", Standard: "W3C-VCDM 2.0", Status: "draft", FieldCount: 18, CreatedAt: "2026-03-05"},
	}, nil
}

func (s *schemaStore) GetSchema(_ context.Context, id string) (*model.Schema, error) {
	schemas, _ := s.ListSchemas(nil)
	for _, sc := range schemas {
		if sc.ID == id {
			return &sc, nil
		}
	}
	return nil, fmt.Errorf("schema %q not found", id)
}

// --- NotificationStore ---
type notificationStore struct{}

func (s *notificationStore) ListNotifications(_ context.Context, role string) ([]model.Notification, error) {
	return []model.Notification{
		{ID: "n1", Type: "issuance", Icon: "📄", Text: "Batch issuance completed — 342 credentials issued", Time: "2 minutes ago", Unread: true, Action: "View"},
		{ID: "n2", Type: "verification", Icon: "✓", Text: "Credential verified for holder 816***245 — VALID", Time: "14 minutes ago", Unread: true, Action: "View"},
		{ID: "n3", Type: "system", Icon: "🏛", Text: "Ministry of Agriculture onboarded as new issuer", Time: "3 hours ago", Unread: false, Action: "Configure"},
	}, nil
}

// --- AuditStore ---
type auditStore struct{}

func (s *auditStore) ListEntries(_ context.Context) ([]model.AuditEntry, error) {
	return []model.AuditEntry{
		{Timestamp: "2026-04-08 14:23:01", Module: "issuance", Actor: "UWI", Action: "batch_issue", Target: "bsc-degree-v2 (342)", Outcome: "success"},
		{Timestamp: "2026-04-08 14:09:17", Module: "verification", Actor: "EmployTT", Action: "verify", Target: "holder 816***245", Outcome: "success"},
		{Timestamp: "2026-04-08 13:15:44", Module: "issuance", Actor: "UTT", Action: "revoke", Target: "tt-utt-2024-4421", Outcome: "success"},
	}, nil
}
