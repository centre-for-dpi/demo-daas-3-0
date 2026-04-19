// Package registry implements backend.Adapter by delegating to one concrete
// adapter per DPG, as declared in backends.json.
//
// backends.json is the single place where vendor-specific URLs, client IDs, and
// credentials live. Handlers and templates never import any adapter package or
// see vendor-specific strings — they go through registry's fan-out.
package registry

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/verifiably/verifiably-go/vctypes"
)

// Config is the shape of backends.json.
type Config struct {
	Backends []BackendEntry `json:"backends"`
}

// BackendEntry describes one configured DPG.
// Type selects which adapter package will handle it (waltid, injicertify,
// injiweb, injiverify). Roles lists which of issuer/holder/verifier this DPG
// participates in. DPG is the structured card data surfaced on the picker.
// Config is an opaque blob passed straight to the adapter — only that adapter
// package knows how to interpret it.
type BackendEntry struct {
	Vendor string          `json:"vendor"`
	Type   string          `json:"type"`
	Roles  []string        `json:"roles"`
	DPG    vctypes.DPG     `json:"dpg"`
	Config json.RawMessage `json:"config"`
}

// LoadConfig reads backends.json from path.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

// HasRole returns true if this entry participates in the given role.
func (e *BackendEntry) HasRole(role string) bool {
	for _, r := range e.Roles {
		if r == role {
			return true
		}
	}
	return false
}
