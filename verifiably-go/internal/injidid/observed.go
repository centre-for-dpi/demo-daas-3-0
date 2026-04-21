// Package injidid owns the runtime set of DID kids observed in VCs Inji
// Certify has signed through this process. Split out of internal/handlers
// so both the in-process inji-proxy (handlers/inji_proxy.go) AND the
// direct-to-PDF pre-auth issuance path (adapters/injicertify/pdf.go) can
// contribute kids — and so the did.json handler has a single source of
// truth regardless of which issuance path emitted the VC.
//
// Background: Inji Certify v0.14.0 publishes did:web:certify-nginx in its
// issuer metadata but signs VCs with a kid that isn't always on the
// upstream's did.json. The inji-proxy handler patches its served did.json
// to advertise every kid we've seen so Inji Verify's strict kid matcher
// can resolve the signing key. This package holds the observation state.
package injidid

import (
	"encoding/json"
	"strings"
	"sync"
)

var (
	mu   sync.RWMutex
	kids = map[string]struct{}{}
)

// Remember scans a credential-issuance response body (JSON) for any
// proof.verificationMethod of the shape did:web:…#kid and records the
// kid fragment. Tolerant to any JSON shape — if the body isn't JSON or
// has no proof, this is a no-op. Called from two producers: the
// inji-proxy credential forwarder, and the pre-auth direct-to-PDF path.
func Remember(body []byte) {
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
					mu.Lock()
					kids[kid] = struct{}{}
					mu.Unlock()
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

// Add manually inserts a kid. Used by the INJI_PROXY_EXTRA_KIDS seed +
// anywhere we already know a kid without a VC body to parse.
func Add(kid string) {
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return
	}
	mu.Lock()
	kids[kid] = struct{}{}
	mu.Unlock()
}

// Snapshot returns every kid observed so far — consumed by the did.json
// handler to synthesize verificationMethod aliases.
func Snapshot() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(kids))
	for k := range kids {
		out = append(out, k)
	}
	return out
}
