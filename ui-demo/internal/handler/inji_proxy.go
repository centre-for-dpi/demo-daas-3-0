package handler

// inji_proxy.go — OID4VCI metadata + token + credential proxy for Inji Certify.
//
// Purpose: Walt.id's wallet-api uses a strict kotlinx.serialization parser
// that rejects Inji Certify's credential_configurations_supported shape
// (extra Inji-specific fields, claim-descriptor objects inside
// credential_definition.credentialSubject). To make cross-DPG flows work
// (Inji issues, Walt.id wallet claims, Walt.id verifier verifies), we
// interpose this lightweight proxy: we rewrite the offer URL to point at
// our own /inji-proxy/* endpoints, then forward token + credential calls
// through to Inji while returning a Walt.id-compatible metadata shape.
//
// Walt.id wallet-api (in docker) reaches this proxy via the "localhost"
// extra_host entry (see docker-compose.yml: localhost:host-gateway), so
// from inside the wallet container http://localhost:8080/inji-proxy/...
// resolves back to the host where our Go server runs.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// InjiProxy holds state for the metadata translation proxy.
type InjiProxy struct {
	// UpstreamURL is where we fetch Inji's metadata from (host side).
	// e.g. http://localhost:8091
	UpstreamURL string
	// PublicBase is the base URL of this proxy as seen by a wallet.
	// e.g. http://localhost:8080/inji-proxy
	PublicBase string
	// InternalBase is the URL Inji uses internally for credential_issuer.
	// We rewrite this to PublicBase inside transformed metadata.
	// e.g. http://certify-nginx:80
	InternalBase string
}

var (
	injiProxyInstance *InjiProxy
	injiProxyOnce     sync.Once
)

func getInjiProxy() *InjiProxy {
	injiProxyOnce.Do(func() {
		// UpstreamURL must point at inji-certify DIRECTLY (not certify-nginx),
		// because certify-nginx routes /v1/certify/issuance/credential back to
		// us to do @context injection; a loop results if we forward via nginx.
		injiProxyInstance = &InjiProxy{
			UpstreamURL:  envOrDefault("INJI_CERTIFY_UPSTREAM_URL", envOrDefault("INJI_CERTIFY_URL", "http://localhost:8090")),
			PublicBase:   envOrDefault("INJI_PROXY_URL", "http://localhost:8080/inji-proxy"),
			InternalBase: envOrDefault("INJI_CERTIFY_PUBLIC_URL", "http://certify-nginx:80"),
		}
	})
	return injiProxyInstance
}

func envOrDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// InjiProxyOfferURL returns the offer URL (openid-credential-offer://...) that
// points at this proxy instead of the raw Inji URL, so Walt.id wallet talks to us.
// It reads the raw Inji offer URL and swaps the credential_offer_uri host prefix.
func InjiProxyOfferURL(rawInjiOfferURL string) string {
	p := getInjiProxy()
	if p.PublicBase == "" {
		return rawInjiOfferURL
	}
	// Extract the credential_offer_uri (URL-encoded) and rewrite its path.
	// Inji format: openid-credential-offer://?credential_offer_uri=<encoded URL>
	idx := strings.Index(rawInjiOfferURL, "credential_offer_uri=")
	if idx < 0 {
		return rawInjiOfferURL
	}
	encoded := rawInjiOfferURL[idx+len("credential_offer_uri="):]
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		return rawInjiOfferURL
	}
	// The decoded URL looks like http://certify-nginx:80/v1/certify/credential-offer-data/UUID
	// Replace the prefix with our proxy's offer endpoint.
	uuid := ""
	if i := strings.LastIndex(decoded, "/"); i >= 0 {
		uuid = decoded[i+1:]
	}
	if uuid == "" {
		return rawInjiOfferURL
	}
	proxyOffer := p.PublicBase + "/offer/" + uuid
	return "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(proxyOffer)
}

// RegisterInjiProxy attaches proxy routes to the mux. These are UNAUTHENTICATED
// because they are called by a wallet's OID4VCI client, not the UI.
func (h *Handler) RegisterInjiProxy(mux *http.ServeMux) {
	mux.HandleFunc("GET /inji-proxy/offer/{id}", h.injiProxyOffer)
	mux.HandleFunc("GET /inji-proxy/.well-known/openid-credential-issuer", h.injiProxyIssuerMetadata)
	mux.HandleFunc("GET /inji-proxy/.well-known/oauth-authorization-server", h.injiProxyAuthServerMetadata)
	mux.HandleFunc("POST /inji-proxy/oauth/token", h.injiProxyToken)
	mux.HandleFunc("POST /inji-proxy/issuance/credential", h.injiProxyCredential)
}

// injiProxyOffer fetches the credential_offer JSON from Inji and rewrites
// credential_issuer to point at our proxy.
func (h *Handler) injiProxyOffer(w http.ResponseWriter, r *http.Request) {
	p := getInjiProxy()
	id := r.PathValue("id")
	upstream := p.UpstreamURL + "/v1/certify/credential-offer-data/" + id
	body, _, err := fetchJSON(upstream)
	if err != nil {
		http.Error(w, "upstream fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	var offer map[string]any
	if err := json.Unmarshal(body, &offer); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadGateway)
		return
	}
	// Rewrite credential_issuer to our proxy base.
	offer["credential_issuer"] = p.PublicBase
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(offer)
}

// injiProxyIssuerMetadata fetches Inji Certify's OID4VCI issuer metadata
// and serves it in one of two shapes depending on a query parameter:
//
//   ?client=waltid (default) — strips scope / display / proof_types_supported /
//       credential_definition.credentialSubject and other fields walt.id's
//       kotlinx parser can't tolerate. This is what the walt.id wallet fetches.
//
//   ?client=mimoto — passthrough. Mimoto's IssuersValidationConfig REQUIRES
//       scope, display, and proof_types_supported on every credential
//       configuration and rejects metadata without them (RESIDENT-APP-041).
//       So we return the Inji Certify response almost verbatim, only adding
//       the token_endpoint that Inji puts in a separate well-known doc.
//
// Same URL, different Accept/query — lets walt.id and Mimoto share the
// same upstream inji-certify instance without either breaking the other.
func (h *Handler) injiProxyIssuerMetadata(w http.ResponseWriter, r *http.Request) {
	p := getInjiProxy()
	upstream := p.UpstreamURL + "/v1/certify/.well-known/openid-credential-issuer"
	body, _, err := fetchJSON(upstream)
	if err != nil {
		http.Error(w, "upstream fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	var md map[string]any
	if err := json.Unmarshal(body, &md); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadGateway)
		return
	}
	// Default mode is passthrough (Mimoto-compatible, spec-accurate).
	// Walt.id wallets that need the stripped shape must fetch with
	// `?client=waltid`. We flipped the default because Mimoto drops
	// query params when it re-issues the request internally, so a
	// query-keyed opt-in for Mimoto doesn't work — walt.id can tolerate
	// the extra fields on a case-by-case basis via the adaptor layer.
	client := r.URL.Query().Get("client")
	if client == "waltid" {
		md = transformIssuerMetadata(md, p)
	} else {
		md = passthroughIssuerMetadata(md, p)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(md)
}

// passthroughIssuerMetadata returns Inji Certify's original metadata with
// only the fields Mimoto's validator actually needs added — it does NOT
// strip anything. If Mimoto has a problem with an Inji-specific extension
// we fix it here rather than by pre-stripping the whole response.
//
// Two active rewrites:
//
//  1. token_endpoint added if missing (Inji puts it in a separate
//     well-known doc; Mimoto expects it inline).
//
//  2. authorization_servers is rewritten to point at our local esignet
//     instance. Inji Certify advertises `["http://certify-nginx:80"]`
//     because that's where it thinks its own auth endpoints live, but
//     certify-nginx doesn't actually expose an auth-server well-known —
//     esignet does, at http://injiweb-esignet:8088/v1/esignet/oauth.
//     Mimoto derives the auth-server well-known URL as
//     `{authorization_servers[0]}/.well-known/oauth-authorization-server`,
//     so without this rewrite Mimoto fails with RESIDENT-APP-042
//     "Invalid Authorization Server well-known from server: well-known
//     api is not accessible".
func passthroughIssuerMetadata(md map[string]any, p *InjiProxy) map[string]any {
	base, _ := md["credential_issuer"].(string)
	if base == "" {
		base = p.InternalBase
	}
	if _, ok := md["token_endpoint"]; !ok {
		md["token_endpoint"] = base + "/v1/certify/oauth/token"
	}
	// Point Mimoto at our local esignet for the OIDC authorization server
	// well-known. Override via env var so deployments pointing at a
	// different esignet (e.g. MOSIP collab) can set the right URL.
	esignetBase := envOrDefault("INJI_PROXY_ESIGNET_BASE", "http://injiweb-esignet:8088/v1/esignet/oauth")
	md["authorization_servers"] = []any{esignetBase}
	return md
}

// transformIssuerMetadata strips Inji-specific fields that Walt.id's strict
// parser doesn't accept. It preserves Inji's original URLs (credential_issuer,
// credential_endpoint, authorization_servers) so that proof JWTs from the
// wallet still validate against Inji's configured domain URL.
func transformIssuerMetadata(md map[string]any, p *InjiProxy) map[string]any {
	// Walt.id's OID4VCI client needs token_endpoint inline in the issuer metadata
	// (or discoverable via authorization_servers). Inji puts it in the oauth-
	// authorization-server metadata, not the credential issuer metadata. Add it
	// directly, derived from the credential_issuer URL.
	base, _ := md["credential_issuer"].(string)
	if base == "" {
		base = p.InternalBase
	}
	if _, ok := md["token_endpoint"]; !ok {
		md["token_endpoint"] = base + "/v1/certify/oauth/token"
	}

	// Walk credential_configurations_supported and strip Inji-specific bits.
	if ccs, ok := md["credential_configurations_supported"].(map[string]any); ok {
		for key, raw := range ccs {
			if cfg, ok := raw.(map[string]any); ok {
				// Drop Inji-specific extensions that Walt.id's parser rejects.
				delete(cfg, "order")
				delete(cfg, "scope")
				delete(cfg, "display")
				delete(cfg, "proof_types_supported")
				// Inji uses a flat {claim_name: descriptor} map for claims, but Walt.id's
				// ClaimDescriptorNamespacedMapSerializer expects two-level nesting
				// (namespace → claim → descriptor). Just drop the claims metadata since
				// it's documentation, not functional.
				delete(cfg, "claims")
				// Inside credential_definition, strip credentialSubject (claim descriptors)
				// that use an Inji-specific shape.
				if cd, ok := cfg["credential_definition"].(map[string]any); ok {
					delete(cd, "credentialSubject")
					cfg["credential_definition"] = cd
				}
				ccs[key] = cfg
			}
		}
		md["credential_configurations_supported"] = ccs
	}

	// Strip top-level display — also Inji-specific and kotlinx parser chokes on it.
	delete(md, "display")
	return md
}

// injiProxyAuthServerMetadata forwards Inji's oauth-authorization-server metadata,
// rewriting token_endpoint to point at our proxy.
func (h *Handler) injiProxyAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	p := getInjiProxy()
	upstream := p.UpstreamURL + "/v1/certify/.well-known/oauth-authorization-server"
	body, _, err := fetchJSON(upstream)
	if err != nil {
		http.Error(w, "upstream fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	var md map[string]any
	if err := json.Unmarshal(body, &md); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadGateway)
		return
	}
	md["issuer"] = p.PublicBase
	md["token_endpoint"] = p.PublicBase + "/oauth/token"
	if _, ok := md["jwks_uri"].(string); ok {
		md["jwks_uri"] = p.PublicBase + "/jwks.json"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(md)
}

// injiProxyToken forwards a form-encoded pre-auth token request to Inji's real
// token endpoint.
func (h *Handler) injiProxyToken(w http.ResponseWriter, r *http.Request) {
	p := getInjiProxy()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Printf("inji-proxy: token REQ body=%s\n", truncate(string(body), 300))
	upstream := p.UpstreamURL + "/v1/certify/oauth/token"
	req, _ := http.NewRequestWithContext(r.Context(), "POST", upstream, strings.NewReader(string(body)))
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/x-www-form-urlencoded"
	}
	req.Header.Set("Content-Type", ct)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// injiProxyCredential forwards the credential request to Inji's issuance endpoint.
// It also patches the request body to satisfy Inji's LdpVcCredentialRequestValidator,
// which requires credential_definition.@context to be present. Walt.id's wallet
// omits @context from credential requests, so we inject a sensible default.
func (h *Handler) injiProxyCredential(w http.ResponseWriter, r *http.Request) {
	p := getInjiProxy()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	authHdr := r.Header.Get("Authorization")

	// Debug: log the Bearer token's payload so we can see what esignet
	// put in the iss / aud / scope claims during token exchange. Only
	// active when INJI_PROXY_LOG_TOKEN=1 is set on the Go server env.
	if os.Getenv("INJI_PROXY_LOG_TOKEN") == "1" && strings.HasPrefix(authHdr, "Bearer ") {
		parts := strings.Split(strings.TrimPrefix(authHdr, "Bearer "), ".")
		if len(parts) == 3 {
			payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
			fmt.Printf("inji-proxy: incoming token payload = %s\n", string(payload))
		}
	}

	// Parse the request, inject @context if missing, and re-serialize.
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if cd, ok := parsed["credential_definition"].(map[string]any); ok {
			if _, hasCtx := cd["@context"]; !hasCtx {
				cd["@context"] = []string{"https://www.w3.org/ns/credentials/v2"}
				parsed["credential_definition"] = cd
				if patched, err := json.Marshal(parsed); err == nil {
					body = patched
					fmt.Printf("inji-proxy: credential REQ injected @context\n")
				}
			}
		}
	}

	upstream := p.UpstreamURL + "/v1/certify/issuance/credential"
	req, _ := http.NewRequestWithContext(r.Context(), "POST", upstream, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if authHdr != "" {
		req.Header.Set("Authorization", authHdr)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("inji-proxy: credential RESP %d body=%s\n", resp.StatusCode, truncate(string(respBody), 400))
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// fetchJSON is a small helper to GET a JSON body from an upstream URL.
func fetchJSON(u string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return body, resp.StatusCode, fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, resp.StatusCode, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
