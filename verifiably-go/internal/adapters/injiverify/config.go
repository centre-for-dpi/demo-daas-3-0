// Package injiverify implements backend.Adapter against Inji Verify v0.16.0.
// Endpoint map (verified against /tmp/inji-verify @ tag v0.16.0):
//   - POST /v1/verify/vc-verification        — synchronous VC verification
//   - POST /v1/verify/vc-submission          — returns a transactionId used to
//     poll /vp-result/{id}; handles formats VC verification can't (e.g. some
//     SD-JWT VC variants)
//   - GET  /v1/verify/vp-result/{txid}       — poll SD-JWT-VC / VP outcome
//   - POST /v1/verify/vp-request             — create OID4VP authorization
//     request (cross-device verifier flow)
//   - GET  /v1/verify/vp-request/{id}/status — long-poll request status
//
// INJIVER-1131 mitigation: this adapter applies a post-verification
// credential-type check — the template's expected Format and Fields must
// intersect the returned VC's claims. If not, the adapter demotes Valid=false
// regardless of what the service reports.
package injiverify

import (
	"encoding/json"
	"fmt"
)

// Config is the per-backend config blob the registry passes in.
type Config struct {
	// BaseURL is the Inji Verify service base, e.g. http://localhost:8082.
	BaseURL string `json:"baseUrl"`

	// ClientID is the OID4VP client_id the adapter uses when creating
	// vp-request sessions. For self-contained demos any non-empty string works;
	// production deployments should set this to the actual RP identifier.
	ClientID string `json:"clientId"`
}

// UnmarshalConfig extracts a Config from a raw json.RawMessage with defaults.
func UnmarshalConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) == 0 {
		return c, fmt.Errorf("injiverify: empty config")
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	if c.BaseURL == "" {
		return c, fmt.Errorf("injiverify: baseUrl is required")
	}
	if c.ClientID == "" {
		c.ClientID = "verifiably-demo"
	}
	return c, nil
}
