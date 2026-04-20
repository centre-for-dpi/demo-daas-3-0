package handlers

// inji_proxy.go — minimal OID4VCI credential-request proxy for Inji Certify.
//
// Why this exists: certify-nginx (deploy/compose/stack/inji/certify-nginx/
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
//
// The did.json handler below is the second reason we need this proxy: Inji
// Certify v0.14.0 publishes did:web:certify-nginx#<kid_A> in its did.json but
// signs VCs with did:web:certify-nginx#<kid_B> — both derivations of the same
// Ed25519 key. Inji Verify's DidWebPublicKeyResolver strictly matches kid, so
// verification fails. We watch outgoing VC responses, extract whatever kid
// appears in the signature, and add it to the did.json we serve — as many
// aliases as needed, all mapped to the upstream publicKeyMultibase.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
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
// endpoint, patching in @context if the wallet omitted it. Also records any
// kid that appears in the signed VC so our did.json handler can advertise it.
func (h *H) InjiProxyCredential(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
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
	} else {
		rememberSigningKids(respBody)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// observedKids is the runtime set of kid fragments we've seen in VCs issued
// through this proxy. Populated by rememberSigningKids; consumed by the
// did.json handler. Keys are the fragment only (everything after '#').
var (
	observedKidsMu sync.RWMutex
	observedKids   = map[string]struct{}{}
)

// rememberSigningKids scans a credential-issuance response body for any
// proof.verificationMethod values of shape did:web:...#kid and records the
// kid fragment. Tolerant to any JSON shape — if the body isn't JSON or has
// no proof, this is a no-op.
func rememberSigningKids(body []byte) {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}
	var walk func(any)
	walk = func(v any) {
		switch vv := v.(type) {
		case map[string]any:
			if vm, ok := vv["verificationMethod"].(string); ok {
				if i := strings.IndexByte(vm, '#'); i >= 0 {
					kid := vm[i+1:]
					observedKidsMu.Lock()
					observedKids[kid] = struct{}{}
					observedKidsMu.Unlock()
				}
			}
			for _, c := range vv {
				walk(c)
			}
		case []any:
			for _, c := range vv {
				walk(c)
			}
		}
	}
	walk(parsed)
}

func observedKidsSnapshot() []string {
	observedKidsMu.RLock()
	defer observedKidsMu.RUnlock()
	out := make([]string, 0, len(observedKids))
	for k := range observedKids {
		out = append(out, k)
	}
	return out
}

// InjiProxyStatusList forwards a GET to Inji Certify's bitstring status-list
// credential endpoint. We tap it so rememberSigningKids() sees the kid that
// signed the status-list VC — which Inji Certify v0.14.0 derives differently
// from the kid it uses on regular VCs. Both are the SAME Ed25519 key, but
// Inji Verify's strict kid-matching fails on the status-list unless our
// did.json advertises both.
func (h *H) InjiProxyStatusList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	upstream := injiCertifyUpstream() + "/v1/certify/credentials/status-list/" + id
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, "build upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 400 {
		rememberSigningKids(body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// InjiProxyDidJSON serves /.well-known/did.json for did:web:certify-nginx,
// patched so verificationMethod includes every kid we've seen in signed VCs
// (plus the upstream's own advertised kid). Inji Verify's DidWebPublicKeyResolver
// matches kid exactly, so without this it fails with
// "Public key extraction failed for kid: did:web:certify-nginx#<signing-kid>".
//
// All synthesized verificationMethod entries point at the SAME Ed25519
// publicKeyMultibase — we're not multiplying keys, just publishing aliases
// for the one real key Inji Certify uses.
func (h *H) InjiProxyDidJSON(w http.ResponseWriter, r *http.Request) {
	upstream := injiCertifyUpstream() + "/v1/certify/.well-known/did.json"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, "build upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("inji-proxy: did.json upstream %d body=%s", resp.StatusCode, truncateForLog(string(body), 200))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		// Unparseable — return upstream verbatim.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	patchedDidDoc(doc, observedKidsSnapshot())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// patchedDidDoc mutates doc to add one verificationMethod per extra kid,
// cloning the upstream method's key material. The original method stays in
// place (some verifiers cache on first match). `extras` is allowed to include
// kids that already exist; duplicates are skipped.
func patchedDidDoc(doc map[string]any, extras []string) {
	didID, _ := doc["id"].(string)
	if didID == "" {
		return
	}
	methods, _ := doc["verificationMethod"].([]any)
	if len(methods) == 0 {
		return
	}
	template, _ := methods[0].(map[string]any)
	if template == nil {
		return
	}
	// Collect existing kid fragments so we don't duplicate.
	existing := map[string]struct{}{}
	for _, m := range methods {
		mm, _ := m.(map[string]any)
		if id, _ := mm["id"].(string); id != "" {
			if i := strings.IndexByte(id, '#'); i >= 0 {
				existing[id[i+1:]] = struct{}{}
			}
		}
	}
	for _, kid := range extras {
		if kid == "" {
			continue
		}
		if _, ok := existing[kid]; ok {
			continue
		}
		clone := map[string]any{}
		for k, v := range template {
			clone[k] = v
		}
		clone["id"] = didID + "#" + kid
		methods = append(methods, clone)
		existing[kid] = struct{}{}
	}
	doc["verificationMethod"] = methods
}

// SeedObservedKid pre-populates observedKids from a comma-separated env var.
// Lets operators survive a cold start: if an old VC is verified before any
// new issuance has run through this proxy, the kid is still known. The
// resolved value comes from INJI_PROXY_EXTRA_KIDS.
func init() {
	if v := os.Getenv("INJI_PROXY_EXTRA_KIDS"); v != "" {
		for _, k := range strings.Split(v, ",") {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			observedKids[k] = struct{}{}
		}
	}
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
