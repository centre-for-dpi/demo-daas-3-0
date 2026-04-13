package handler

import (
	"encoding/json"
	"net/http"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
)

// API handlers return JSON data that HTMX templates can consume,
// or that can be used by future client-side components.

// APIIssuerOnboard handles POST /api/issuer/onboard — creates a new issuer key+DID.
func (h *Handler) APIIssuerOnboard(w http.ResponseWriter, r *http.Request) {
	keyType := "secp256r1"
	if kt := r.URL.Query().Get("keyType"); kt != "" {
		keyType = kt
	}

	result, err := h.stores.Issuer.OnboardIssuer(r.Context(), keyType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Persist the issuer DID so it appears in DID & Key Manager
	user := middleware.GetUser(r.Context())
	if user != nil && result.IssuerDID != "" {
		h.addIssuerDID(user, result.IssuerDID, keyType)
	}

	writeJSON(w, http.StatusOK, result)
}

// APIIssueCredential handles POST /api/issuer/issue — issues a credential.
func (h *Handler) APIIssueCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyType  string         `json:"keyType"`
		ConfigID string         `json:"configId"`
		Format   string         `json:"format"`
		Claims   map[string]any `json:"claims"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ConfigID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "configId required"})
		return
	}
	if req.Format == "" {
		req.Format = "sdjwt_vc"
	}

	// Onboard issuer (in production, this would be cached)
	issuer, err := h.stores.Issuer.OnboardIssuer(r.Context(), req.KeyType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	offerURL, err := h.stores.Issuer.IssueCredential(r.Context(), issuer, req.ConfigID, req.Format, req.Claims)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"offerUrl":  offerURL,
		"issuerDid": issuer.IssuerDID,
	})
}

// APIVerify handles POST /api/verifier/verify — creates a verification session.
func (h *Handler) APIVerify(w http.ResponseWriter, r *http.Request) {
	var req model.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = model.VerifyRequest{CredentialTypes: []string{"VerifiableCredential"}, Policies: []string{"signature"}}
	}

	result, err := h.stores.Verifier.CreateVerificationSession(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// APIVerifyResult handles GET /api/verifier/session/{state} — gets verification result.
func (h *Handler) APIVerifyResult(w http.ResponseWriter, r *http.Request) {
	state := r.PathValue("state")
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state required"})
		return
	}

	result, err := h.stores.Verifier.GetSessionResult(r.Context(), state)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// APIWalletCredentials handles GET /api/wallet/credentials — lists wallet credentials.
func (h *Handler) APIWalletCredentials(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, http.StatusOK, []any{}) // Return empty for mock
		return
	}

	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	creds, err := h.stores.Wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, creds)
}

// APIWalletClaim handles POST /api/wallet/claim — claims a credential offer.
func (h *Handler) APIWalletClaim(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no backend auth"})
		return
	}

	var req struct {
		OfferURL string `json:"offerUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no wallet"})
		return
	}

	err = h.stores.Wallet.ClaimCredential(r.Context(), user.WalletToken, wallets[0].ID, req.OfferURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "claimed"})
}

// APIWalletPresent handles POST /api/wallet/present — presents credential to verifier via OID4VP.
func (h *Handler) APIWalletPresent(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sign in via SSO to link your wallet"})
		return
	}

	var req model.PresentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.PresentationRequest == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "presentationRequest (OID4VP URL) required"})
		return
	}

	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no wallet found"})
		return
	}

	// Auto-fill holder DID if not provided
	if req.DID == "" {
		dids, err := h.stores.Wallet.ListDIDs(r.Context(), user.WalletToken, wallets[0].ID)
		if err == nil && len(dids) > 0 {
			req.DID = dids[0].DID
		}
	}

	err = h.stores.Wallet.PresentCredential(r.Context(), user.WalletToken, wallets[0].ID, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "presentation failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "presented"})
}

// APIWalletDIDs handles GET /api/wallet/dids — lists DIDs.
func (h *Handler) APIWalletDIDs(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	dids, err := h.stores.Wallet.ListDIDs(r.Context(), user.WalletToken, wallets[0].ID)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, dids)
}

// APISchemas handles GET /api/schemas — lists schemas.
func (h *Handler) APISchemas(w http.ResponseWriter, r *http.Request) {
	schemas, err := h.stores.Schemas.ListSchemas(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, schemas)
}

// APIPolicies handles GET /api/verifier/policies — lists verification policies.
func (h *Handler) APIPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.stores.Verifier.ListPolicies(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

// APIExportCredentialJSON handles GET /api/wallet/export — downloads all wallet credentials as JSON.
func (h *Handler) APIExportCredentialJSON(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		http.Error(w, "unauthorized", 401)
		return
	}
	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		http.Error(w, "no wallet", 500)
		return
	}
	creds, err := h.stores.Wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=credentials.json")
	json.NewEncoder(w).Encode(creds)
}

// APICreateDID handles POST /api/wallet/dids/create — creates a new DID in the wallet.
func (h *Handler) APICreateDID(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		http.Error(w, "unauthorized", 401)
		return
	}
	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		http.Error(w, "no wallet", 500)
		return
	}
	method := r.URL.Query().Get("method")
	if method == "" {
		method = "jwk"
	}
	did, err := h.stores.Wallet.CreateDID(r.Context(), user.WalletToken, wallets[0].ID, method)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"did": did})
}

// APIIssueCredentialOffer handles POST /api/credential/issue — creates an OID4VCI offer.
// Returns the offer URL for the holder to claim (via QR code, deep link, or direct push).
func (h *Handler) APIIssueCredentialOffer(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		ConfigID string         `json:"configId"`
		Format   string         `json:"format"`
		Claims   map[string]any `json:"claims"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.ConfigID == "" {
		writeJSON(w, 400, map[string]string{"error": "configId required — create a schema first"})
		return
	}
	if req.Format == "" {
		req.Format = "sdjwt_vc"
	}
	if req.Claims == nil {
		req.Claims = map[string]any{"name": user.Name}
	}

	// Onboard issuer (creates ephemeral DID+key for this issuance)
	issuer, err := h.stores.Issuer.OnboardIssuer(r.Context(), "secp256r1")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "onboard: " + err.Error()})
		return
	}

	// Issue credential → returns OID4VCI offer URL
	offerURL, err := h.stores.Issuer.IssueCredential(r.Context(), issuer, req.ConfigID, req.Format, req.Claims)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "issue: " + err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{
		"status":    "offer_created",
		"offerUrl":  offerURL,
		"issuerDid": issuer.IssuerDID,
	})
}

// APIWalletClaimOffer handles POST /api/wallet/claim-offer — holder claims an OID4VCI offer.
func (h *Handler) APIWalletClaimOffer(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, 401, map[string]string{"error": "unauthorized — sign in via SSO to link your wallet"})
		return
	}

	var req struct {
		OfferURL string `json:"offerUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.OfferURL == "" {
		writeJSON(w, 400, map[string]string{"error": "offerUrl required"})
		return
	}

	wallets, err := h.stores.Wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, 500, map[string]string{"error": "no wallet found"})
		return
	}

	err = h.stores.Wallet.ClaimCredential(r.Context(), user.WalletToken, wallets[0].ID, req.OfferURL)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "claim failed: " + err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "claimed"})
}

// APICreateShareSession handles POST /api/share/create-session — creates an OID4VP
// verification session for credential sharing.
func (h *Handler) APICreateShareSession(w http.ResponseWriter, r *http.Request) {
	result, err := h.stores.Verifier.CreateVerificationSession(r.Context(), model.VerifyRequest{
		CredentialTypes: []string{"VerifiableCredential"},
		Policies:        []string{"signature"},
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}

// APICreateSchema handles POST /api/schemas/create — saves a schema from the Schema Builder.
func (h *Handler) APICreateSchema(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		ConfigID string `json:"configId"` // Backend credential configuration ID
		Version  string `json:"version"`
		Format   string `json:"format"`
		Standard string `json:"standard"`
		Fields   []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Required bool   `json:"required"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.ID == "" || req.ConfigID == "" {
		writeJSON(w, 400, map[string]string{"error": "id and credential configuration are required"})
		return
	}

	fields := make([]SchemaField, len(req.Fields))
	for i, f := range req.Fields {
		fields[i] = SchemaField{Name: f.Name, Type: f.Type, Required: f.Required}
	}

	schema := CredentialSchema{
		ID:       req.ID,
		Name:     req.Name,
		ConfigID: req.ConfigID,
		Version:  req.Version,
		Format:   req.Format,
		Standard: req.Standard,
		Fields:   fields,
	}
	h.addSchema(user, schema)

	writeJSON(w, 200, map[string]string{"status": "created", "id": req.ID})
}

// APIListSchemas handles GET /api/schemas/list — returns schemas created by this user.
func (h *Handler) APIListSchemas(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || user.Demo {
		writeJSON(w, 200, []any{})
		return
	}
	writeJSON(w, 200, h.getSchemas(user))
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
