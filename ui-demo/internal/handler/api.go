package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"vcplatform/internal/middleware"
	"vcplatform/internal/model"
	"vcplatform/internal/store/injiweb"
	"vcplatform/internal/store/pdfwallet"
)

// API handlers return JSON data that HTMX templates can consume,
// or that can be used by future client-side components.

// APIIssuerOnboard handles POST /api/issuer/onboard — creates a new issuer key+DID.
func (h *Handler) APIIssuerOnboard(w http.ResponseWriter, r *http.Request) {
	keyType := "secp256r1"
	if kt := r.URL.Query().Get("keyType"); kt != "" {
		keyType = kt
	}

	user := middleware.GetUser(r.Context())
	issuer := h.issuerFor(user)
	result, err := issuer.OnboardIssuer(r.Context(), keyType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Persist the issuer DID so it appears in DID & Key Manager
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

	// Onboard issuer (in production, this would be cached). Pick the
	// issuer store matching the logged-in user's DPG choice.
	user := middleware.GetUser(r.Context())
	issuerStore := h.issuerFor(user)

	issuer, err := issuerStore.OnboardIssuer(r.Context(), req.KeyType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	offerURL, err := issuerStore.IssueCredential(r.Context(), issuer, req.ConfigID, req.Format, req.Claims)
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

	user := middleware.GetUser(r.Context())
	result, err := h.verifierFor(user).CreateVerificationSession(r.Context(), req)
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

	user := middleware.GetUser(r.Context())
	result, err := h.verifierFor(user).GetSessionResult(r.Context(), state)
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

	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
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

	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no wallet"})
		return
	}

	err = wallet.ClaimCredential(r.Context(), user.WalletToken, wallets[0].ID, req.OfferURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "claimed"})
}

// APIDirectVerify handles POST /api/verifier/direct-verify — posts a stored
// wallet credential to the currently configured verifier's direct-verify
// endpoint. Used by the Trinidad mode flow (Inji issues → Inji verifies) and
// by the cross-DPG path (Walt.id issues → Inji verifies) where the UI wants
// to skip the OID4VP session dance entirely.
//
// Request: {"credentialId":"<id from wallet>"} OR {"credential":"<raw json>"}
func (h *Handler) APIDirectVerify(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in via SSO to link your wallet"})
		return
	}

	var req struct {
		CredentialID string `json:"credentialId"`
		Credential   string `json:"credential"` // raw VC JSON, used when no stored id
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Resolve the credential document: either from the wallet by id, or from
	// the raw body passed by the UI.
	doc := req.Credential
	if doc == "" && req.CredentialID != "" {
		wallet := h.walletFor(user)
		wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
		if err != nil || len(wallets) == 0 {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no wallet"})
			return
		}
		creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for _, c := range creds {
			if c.ID == req.CredentialID {
				doc = c.Document
				break
			}
		}
	}
	if doc == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credentialId or credential body required"})
		return
	}

	// Detect credential format. JWT VCs (three base64url parts) and SD-JWT
	// (same plus a trailing ~ for disclosures) are forwarded raw so the
	// verifier backend can do its own parse. JSON-LD credentials use
	// application/vc+ld+json.
	contentType := "application/vc+ld+json"
	trimmed := strings.TrimSpace(doc)
	isJWTLike := len(trimmed) > 0 && trimmed[0] == 'e' && strings.Count(trimmed, ".") >= 2
	if isJWTLike {
		if strings.Contains(trimmed, "~") {
			contentType = "application/vc+sd-jwt"
		} else {
			contentType = "application/jwt"
		}
	}

	result, err := h.verifierFor(user).DirectVerify(r.Context(), []byte(doc), contentType)
	if err == nil && result != nil && result.Verified != nil && *result.Verified {
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Fallback for JWT / SD-JWT credentials: the direct verifier couldn't
	// handle it, so drive walt.id's OID4VP session flow instead. This gives
	// the UI a single /api/verifier/direct-verify entry point that works
	// across all formats regardless of which DPG issued them.
	if isJWTLike {
		oid4vpResult, oid4vpErr := h.driveOID4VPVerify(r.Context(), user)
		if oid4vpErr == nil && oid4vpResult != nil && oid4vpResult.Verified != nil && *oid4vpResult.Verified {
			writeJSON(w, http.StatusOK, oid4vpResult)
			return
		}
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "verify failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// driveOID4VPVerify runs a full OID4VP session: create request → wallet
// present → poll result. Used as a fallback when the primary verifier
// can't handle a credential format (JWT, SD-JWT). Prefers the stores'
// FallbackVerifier/FallbackWallet pair (in hybrid mode that's walt.id);
// falls back to the primary verifier/wallet if no fallback pair exists.
func (h *Handler) driveOID4VPVerify(ctx context.Context, user *model.User) (*model.VerifyResult, error) {
	verifier := h.stores.FallbackVerifier
	if verifier == nil {
		verifier = h.stores.Verifier
	}
	wallet := h.stores.FallbackWallet
	if wallet == nil {
		wallet = h.stores.Wallet
	}

	session, err := verifier.CreateVerificationSession(ctx, model.VerifyRequest{
		CredentialTypes: []string{"VerifiableCredential"},
		Policies:        []string{"signature"},
	})
	if err != nil {
		return nil, err
	}
	if session.RequestURL == "" {
		return nil, nil // verifier doesn't support OID4VP sessions
	}

	wallets, err := wallet.GetWallets(ctx, user.WalletToken)
	if err != nil || len(wallets) == 0 {
		return nil, err
	}
	creds, err := wallet.ListCredentials(ctx, user.WalletToken, wallets[0].ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(creds))
	for _, c := range creds {
		ids = append(ids, c.ID)
	}

	presentReq := model.PresentRequest{
		PresentationRequest: session.RequestURL,
		SelectedCredentials: ids,
	}
	if dids, err := wallet.ListDIDs(ctx, user.WalletToken, wallets[0].ID); err == nil && len(dids) > 0 {
		presentReq.DID = dids[0].DID
	}
	if err := wallet.PresentCredential(ctx, user.WalletToken, wallets[0].ID, presentReq); err != nil {
		return nil, err
	}
	return verifier.GetSessionResult(ctx, session.State)
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

	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no wallet found"})
		return
	}

	// Auto-fill holder DID if not provided
	if req.DID == "" {
		dids, err := wallet.ListDIDs(r.Context(), user.WalletToken, wallets[0].ID)
		if err == nil && len(dids) > 0 {
			req.DID = dids[0].DID
		}
	}

	// Auto-select all wallet credentials if the UI didn't specify — Walt.id's
	// usePresentationRequest expects a non-null selectedCredentials array.
	if len(req.SelectedCredentials) == 0 {
		creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
		if err == nil {
			for _, c := range creds {
				req.SelectedCredentials = append(req.SelectedCredentials, c.ID)
			}
		}
	}

	err = wallet.PresentCredential(r.Context(), user.WalletToken, wallets[0].ID, req)
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

	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	dids, err := wallet.ListDIDs(r.Context(), user.WalletToken, wallets[0].ID)
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
	user := middleware.GetUser(r.Context())
	policies, err := h.verifierFor(user).ListPolicies(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

// APIWalletPDF handles GET /api/wallet/pdf?id=... — serves the printable
// PDF for a credential claimed into the PDF wallet. Only usable when the
// user's current wallet DPG is "pdf"; other backends don't generate PDFs.
func (h *Handler) APIWalletPDF(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		http.Error(w, "unauthorized", 401)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", 400)
		return
	}
	wallet := h.walletFor(user)
	pdfWallet, ok := wallet.(*pdfwallet.Store)
	if !ok {
		http.Error(w, "current wallet backend is not a PDF wallet — switch to the Print PDF Wallet first", 400)
		return
	}
	data, ok := pdfWallet.GetPDF(user.WalletToken, id)
	if !ok {
		http.Error(w, "pdf not found for that credential", 404)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=credential-"+id+".pdf")
	w.Write(data)
}

// APIExportCredentialJSON handles GET /api/wallet/export — downloads all wallet credentials as JSON.
func (h *Handler) APIExportCredentialJSON(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		http.Error(w, "unauthorized", 401)
		return
	}
	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		http.Error(w, "no wallet", 500)
		return
	}
	creds, err := wallet.ListCredentials(r.Context(), user.WalletToken, wallets[0].ID)
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
	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		http.Error(w, "no wallet", 500)
		return
	}
	method := r.URL.Query().Get("method")
	if method == "" {
		method = "jwk"
	}
	did, err := wallet.CreateDID(r.Context(), user.WalletToken, wallets[0].ID, method)
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

	// Resolve the issuer store for this user (honors per-user DPG choice).
	issuerStore := h.issuerFor(user)

	// LDP_VC (JSON-LD with Linked Data Proof) is issued via our in-process
	// OID4VCI Pre-Authorized Code flow server (`local-issuer`) ONLY when the
	// chosen issuer backend doesn't expose an ldp_vc endpoint. Walt.id's
	// issuer-api only offers jwt/sd-jwt/mdoc — we sign LDP in-process behind
	// a real OID4VCI facade, so any wallet can claim the credential using
	// the standard protocol. Inji Certify natively issues ldp_vc, so we
	// delegate to it instead of the local issuer.
	issuerName := strings.ToLower(issuerStore.Name())
	useLocalLDP := req.Format == "ldp_vc" && h.localIssuer != nil && !strings.Contains(issuerName, "inji")
	if useLocalLDP {
		types := deriveCredentialTypes(req.ConfigID)
		preAuthCode := h.localIssuer.StagePending(types, req.Claims)
		offerURL := h.localIssuer.BuildOfferURL(preAuthCode)
		writeJSON(w, 200, map[string]any{
			"status":    "offer_created",
			"offerUrl":  offerURL,
			"issuerDid": h.ldpSigner.DID(),
			"backend":   "in-process OID4VCI (LDP_VC)",
		})
		return
	}

	// Onboard issuer (creates ephemeral DID+key for this issuance)
	issuer, err := issuerStore.OnboardIssuer(r.Context(), "secp256r1")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "onboard: " + err.Error()})
		return
	}

	// Issue credential → returns OID4VCI offer URL
	offerURL, err := issuerStore.IssueCredential(r.Context(), issuer, req.ConfigID, req.Format, req.Claims)
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

// deriveCredentialTypes extracts the VC `type` list from a credential
// configuration ID. Convention: "<TypeName>_ldp_vc" or "<TypeName>" →
// ["<TypeName>"]. Stripped of format suffixes.
func deriveCredentialTypes(configID string) []string {
	base := configID
	for _, suffix := range []string{"_ldp_vc", "_jwt_vc_json", "_jwt_vc_json-ld", "_vc+sd-jwt", "_sd-jwt"} {
		base = strings.TrimSuffix(base, suffix)
	}
	if base == "" {
		base = "VerifiableCredential"
	}
	return []string{base}
}

// APIIssueToSelf handles POST /api/wallet/self-issue — a specialized
// "issue + claim in one round trip" flow that the PDF and Local wallets
// use to avoid the interop pitfalls of external issuers.
//
// Why this endpoint exists: an issuer operator using the PDF wallet (or
// the in-process Local holder) clicks "Claim to My Wallet" on the Single
// Issuance page. Normally the UI would POST the external issuer's offer
// URL to /api/wallet/claim-offer, which runs the full OID4VCI Pre-Auth
// dance against whatever DPG was picked in the issuance form.
//
// That's the honest path for wallets that can verify whatever the
// external issuer returns. But it breaks for PDF/Local because:
//
//  * walt.id's issuer-api only has /openid4vc/jwt/issue and
//    /openid4vc/sdjwt/issue — no LDP_VC endpoint. The credential that
//    comes back is a compact JWT VC. Inji Verify (and any other
//    standalone LDP-only verifier) can't verify it.
//  * The primary Inji Certify instance has its credential endpoint
//    rewired through esignet (so the Auth Code / Inji Web flow works)
//    and returns 401 on any Pre-Auth request from our localholder.
//
// So for the PDF/Local self-claim demo path we take a different route:
// stage the exact claims the operator entered into our in-process
// LocalIssuer (URDNA2015 + Ed25519Signature2020 → real ldp_vc with a
// real LDP proof block), build an offer URL pointing at our own
// OID4VCI surface, then have the wallet claim that URL. Result: a
// credential that Inji Verify verifies correctly every time, with an
// issuer DID our own platform resolves via did:key / did:web.
//
// This is NOT a bypass of the issuer layer — it's a second issuance
// path that's always available, labeled honestly on the UI as "the
// platform re-signs the credential with its own LDP signer so Inji
// Verify can verify it". The external-issuer Claim path is still
// there for wallets that need it.
//
// Only the real-user, pdf/local wallet case is allowed. Everything
// else is rejected with a clear reason.
func (h *Handler) APIIssueToSelf(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized — sign in via SSO to link your wallet"})
		return
	}
	if user.WalletDPG != "pdf" && user.WalletDPG != "local" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "self-issue is only available for the PDF Wallet and in-process Local holder. " +
				"For other wallets, use the standard 'Claim to My Wallet' flow with an offer URL.",
			"walletDpg": user.WalletDPG,
		})
		return
	}
	if h.localIssuer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "in-process LDP signer is not initialized — self-issue unavailable",
		})
		return
	}

	var req struct {
		CredentialType string         `json:"credentialType"`
		Claims         map[string]any `json:"claims"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.CredentialType == "" {
		req.CredentialType = "VerifiableCredential"
	}
	if req.Claims == nil {
		req.Claims = map[string]any{}
	}

	// Stage the credential on our in-process LocalIssuer and generate an
	// offer URL. The URL is then claimed by the holder's wallet via the
	// standard OID4VCI Pre-Auth flow — no shortcut, no bypass; we're
	// simply using our own OID4VCI surface as the issuer.
	types := []string{"VerifiableCredential"}
	if req.CredentialType != "VerifiableCredential" {
		types = append(types, req.CredentialType)
	}
	preAuthCode := h.localIssuer.StagePending(types, req.Claims)
	offerURL := h.localIssuer.BuildOfferURL(preAuthCode)

	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no wallet found"})
		return
	}
	if err := wallet.ClaimCredential(r.Context(), user.WalletToken, wallets[0].ID, offerURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "self-issue claim failed: " + err.Error(),
			"backend": "in-process OID4VCI (LDP_VC)",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "claimed",
		"format":    "ldp_vc",
		"issuerDid": h.ldpSigner.DID(),
		"backend":   "in-process OID4VCI (LDP_VC)",
		"note": "credential was signed by this platform's in-process Ed25519 signer " +
			"(URDNA2015 + Ed25519Signature2020). Inji Verify, walt.id's verifier-api, " +
			"and the verification adapter can all verify it directly.",
	})
}

// APIWalletClaimOffer handles POST /api/wallet/claim-offer — holder claims an
// OID4VCI offer with whichever wallet backend the holder picked during
// onboarding. No silent routing: if the chosen wallet can't speak the issuing
// DPG's proof-JWT flavor, the error surfaces with a clear explanation and
// recovery options (switch wallet DPG, ask issuer to re-issue, or use the
// Print-PDF wallet once published).
func (h *Handler) APIWalletClaimOffer(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil || !user.HasBackendAuth() {
		writeJSON(w, 401, map[string]string{"error": "unauthorized — sign in via SSO to link your wallet"})
		return
	}

	// A holder who hasn't picked a wallet DPG yet must not silently have
	// credentials land in whatever the server-default wallet happens to be.
	// Surface a structured error and point them at the onboarding wizard.
	if user.WalletDPG == "" {
		writeJSON(w, 400, map[string]any{
			"error":    "pick a wallet backend before claiming credentials",
			"action":   "onboarding",
			"redirect": "/portal/onboarding",
			"explanation": "You haven't chosen a wallet DPG yet. Finish the short holder " +
				"onboarding step so the credential lands in a backend you explicitly picked.",
		})
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

	wallet := h.walletFor(user)
	wallets, err := wallet.GetWallets(r.Context(), user.WalletToken)
	if err != nil || len(wallets) == 0 {
		writeJSON(w, 500, map[string]string{"error": "no wallet found"})
		return
	}

	if err := wallet.ClaimCredential(r.Context(), user.WalletToken, wallets[0].ID, req.OfferURL); err != nil {
		// Redirect wallets (Inji Web) complete the claim out-of-band in a
		// browser tab. Return 200 with a redirect payload so the UI opens
		// the URL instead of showing an error toast.
		var redirectErr *injiweb.RedirectClaimError
		if errors.As(err, &redirectErr) {
			writeJSON(w, 200, map[string]any{
				"status":      "redirect",
				"wallet":      redirectErr.Wallet,
				"walletDpg":   user.WalletDPG,
				"redirect":    redirectErr.URL,
				"explanation": redirectErr.Explanation,
			})
			return
		}
		writeJSON(w, 500, buildClaimErrorResponse(wallet.Name(), user.WalletDPG, req.OfferURL, err))
		return
	}

	writeJSON(w, 200, map[string]string{"status": "claimed"})
}

// buildClaimErrorResponse packages a wallet-claim failure into a structured
// error the UI can render with recovery options. We recognise two classes of
// failure specifically:
//
//  1. walt.id wallet ↔ Inji Certify proof-JWT mismatch — a known architectural
//     interop gap, surfaced so the holder can switch wallets without guessing.
//  2. PDF wallet QR-too-large — the credential is larger than a single QR can
//     carry even at Low error correction. We echo the size and the
//     alternatives the pdfwallet render layer generated.
func buildClaimErrorResponse(walletName, walletDPG, offerURL string, err error) map[string]any {
	resp := map[string]any{
		"error":     "claim failed: " + err.Error(),
		"wallet":    walletName,
		"walletDpg": walletDPG,
	}

	// PDF wallet: the credential didn't fit in a single QR. The rendering
	// layer gives us the raw + encoded sizes and alternatives it tried.
	var qrTooLarge *pdfwallet.QRTooLargeError
	if errors.As(err, &qrTooLarge) {
		resp["incompatibility"] = "credential-too-large-for-single-qr"
		resp["explanation"] = "The credential is too large to fit in a single offline-verifiable QR code, " +
			"even at the lowest error-correction level. This is usually a JWT VC with many claims; " +
			"JSON-LD (LDP_VC) credentials are typically 30–60% smaller."
		resp["format"] = qrTooLarge.Format
		resp["rawBytes"] = qrTooLarge.RawBytes
		resp["encodedBytes"] = qrTooLarge.EncodedSize
		resp["attempts"] = qrTooLarge.Attempts
		recovery := []map[string]string{}
		for _, alt := range qrTooLarge.Alternatives {
			recovery = append(recovery, map[string]string{"action": "suggestion", "label": alt})
		}
		recovery = append(recovery,
			map[string]string{
				"action": "switch-wallet",
				"to":     "waltid",
				"label":  "Switch to a walt.id wallet (no size limit, requires network)",
			},
			map[string]string{
				"action": "switch-wallet",
				"to":     "local",
				"label":  "Switch to the in-process holder (no size limit, demo-only storage)",
			},
		)
		resp["recovery"] = recovery
		return resp
	}

	// Known incompatibility: walt.id wallet's proof JWT carries an `iss` claim
	// that Inji Certify's OID4VCI validator rejects with invalid_proof.
	low := strings.ToLower(err.Error())
	isInjiOffer := strings.Contains(offerURL, "/v1/certify/") ||
		strings.Contains(offerURL, "certify-nginx")
	looksLikeProofIssue := strings.Contains(low, "invalid_proof") ||
		strings.Contains(low, "proof_header_ambiguous_key") ||
		(strings.Contains(low, "400") && isInjiOffer)

	if looksLikeProofIssue && isInjiOffer && walletDPG == "waltid" {
		resp["incompatibility"] = "waltid-wallet-inji-issuer"
		resp["explanation"] = "The walt.id wallet's proof JWT includes an `iss` claim that Inji Certify rejects. " +
			"This is a known interop gap between the two DPGs, not a server bug."
		resp["recovery"] = []map[string]string{
			{
				"action": "switch-wallet",
				"to":     "local",
				"label":  "Switch to the in-process holder (speaks Inji's proof-JWT dialect)",
			},
			{
				"action": "switch-wallet",
				"to":     "pdf",
				"label":  "Use the Print-PDF wallet (offline, self-verifying QR)",
			},
			{
				"action": "reissue",
				"label":  "Ask the issuer to re-issue from a walt.id-compatible DPG",
			},
		}
	}

	return resp
}

// APICreateShareSession handles POST /api/share/create-session — creates an OID4VP
// verification session for credential sharing.
func (h *Handler) APICreateShareSession(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	result, err := h.verifierFor(user).CreateVerificationSession(r.Context(), model.VerifyRequest{
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

	fields := make([]model.SchemaField, len(req.Fields))
	for i, f := range req.Fields {
		fields[i] = model.SchemaField{Name: f.Name, Type: f.Type, Required: f.Required}
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
