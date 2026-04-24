// Package injidid owns the runtime set of DID kids observed in VCs Inji
// Certify has signed through this process. Split out of internal/handlers
// so both the in-process inji-proxy (handlers/inji_proxy.go) AND the
// direct-to-PDF pre-auth issuance path (adapters/injicertify/pdf.go) can
// contribute kids — and so each did.json handler has a single source of
// truth for the instance it serves.
//
// Background: Inji Certify v0.14.0 publishes a kid in its issuer metadata
// that isn't always the kid that actually signs the next VC. The did.json
// handlers patch their served document to advertise every kid we've seen
// so Inji Verify's strict kid matcher can resolve the signing key.
//
// TWO observers, one per Inji Certify instance. The primary (auth-code)
// instance's kids live in Primary; the pre-auth instance's kids live in
// Preauth. Callers MUST pick the right one — there's no package-level
// default on purpose. Since the primary and pre-auth instances advertise
// DISTINCT DIDs now (did:web:certify-nginx vs did:web:certify-preauth-nginx),
// mixing the two observers would re-introduce the collision the split was
// designed to eliminate.
package injidid

import (
	"encoding/json"
	"strings"
	"sync"
)

// Observer is a thread-safe set of kid fragments observed on VCs signed
// by one Inji Certify instance. Each instance gets its own *Observer so
// the did.json document we serve for that instance advertises only its
// own keys.
type Observer struct {
	mu   sync.RWMutex
	kids map[string]struct{}
}

// New returns a freshly-initialised Observer. Exported so tests can
// construct isolated instances; the runtime uses the Primary + Preauth
// package-level singletons below.
func New() *Observer {
	return &Observer{kids: map[string]struct{}{}}
}

// Remember scans a credential-issuance response body (JSON) for any
// proof.verificationMethod of the shape did:web:…#kid and records the
// kid fragment against this observer. Tolerant to any JSON shape — if
// the body isn't JSON or has no proof, this is a no-op.
func (o *Observer) Remember(body []byte) {
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
					o.mu.Lock()
					o.kids[kid] = struct{}{}
					o.mu.Unlock()
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

// Add manually inserts a kid. Used by the env-var seed paths + anywhere
// we already know a kid without a VC body to parse.
func (o *Observer) Add(kid string) {
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return
	}
	o.mu.Lock()
	o.kids[kid] = struct{}{}
	o.mu.Unlock()
}

// Snapshot returns every kid this observer has seen — consumed by the
// did.json handler to synthesize verificationMethod aliases.
func (o *Observer) Snapshot() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]string, 0, len(o.kids))
	for k := range o.kids {
		out = append(out, k)
	}
	return out
}

// Primary tracks kids observed on VCs signed by the AUTH-CODE Inji
// Certify instance (container inji-certify, DID did:web:certify-nginx).
// Populated by the inji-proxy credential + status-list handlers.
var Primary = New()

// Preauth tracks kids observed on VCs signed by the PRE-AUTH Inji
// Certify instance (container inji-certify-preauth-backend, DID
// did:web:certify-preauth-nginx). Populated by the direct-to-PDF
// issuance path in adapters/injicertify/pdf.go.
var Preauth = New()
