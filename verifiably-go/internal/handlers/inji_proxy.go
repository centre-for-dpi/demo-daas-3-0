package handlers

// inji_proxy.go — minimal OID4VCI credential-request proxy for Inji Certify.
//
// Why this exists: certify-nginx (ui-demo/docker/stack/inji/certify-nginx/
// nginx.conf) routes POST /v1/certify/issuance/credential to
// http://host.docker.internal:8080/inji-proxy/issuance/credential. That's our
// port. When the route doesn't resolve, Mimoto fails the credential download
// with a 404, and Inji Web shows "An Error Occurred — unable to download the
// card". So we expose the endpoint and forward to inji-certify directly.
//
// We ALSO inject `credential_definition.@context` if the wallet omitted it —
// Inji Certify's LdpVcCredentialRequestValidator rejects w3c_vcdm_2 requests
// without an @context array, and some wallets (walt.id in particular) don't
// send one. Mimoto usually includes it; the injection is a no-op for them.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func injiCertifyUpstream() string {
	if v := os.Getenv("INJI_CERTIFY_UPSTREAM_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	// Default matches the docker-compose service name, since this handler
	// runs inside the verifiably-go container sharing the waltid_default
	// network.
	return "http://inji-certify:8090"
}

// InjiProxyCredential forwards a POST to Inji Certify's issuance/credential
// endpoint, patching in @context if the wallet omitted it.
func (h *H) InjiProxyCredential(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Inject credential_definition.@context if missing so Inji's ldp-vc
	// validator doesn't reject the request. No-op when the wallet already
	// sent one.
	if patched, ok := injectCredentialContext(body); ok {
		body = patched
	}

	upstream := injiCertifyUpstream() + "/v1/certify/issuance/credential"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream, strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, "build upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if ah := r.Header.Get("Authorization"); ah != "" {
		req.Header.Set("Authorization", ah)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("inji-proxy: upstream error: %v", err)
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		log.Printf("inji-proxy: credential RESP %d body=%s", resp.StatusCode, truncateForLog(string(respBody), 400))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// injectCredentialContext parses the request body as JSON and adds
// credential_definition.@context when absent. Returns the patched bytes and
// true if it modified the body; otherwise returns the original bytes and false.
func injectCredentialContext(body []byte) ([]byte, bool) {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body, false
	}
	cd, ok := parsed["credential_definition"].(map[string]any)
	if !ok {
		return body, false
	}
	if _, hasCtx := cd["@context"]; hasCtx {
		return body, false
	}
	cd["@context"] = []string{"https://www.w3.org/ns/credentials/v2"}
	parsed["credential_definition"] = cd
	patched, err := json.Marshal(parsed)
	if err != nil {
		return body, false
	}
	return patched, true
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s…(%d more)", s[:n], len(s)-n)
}
