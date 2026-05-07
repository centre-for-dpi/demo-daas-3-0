package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/verifiably/verifiably-go/vctypes"
)

// SchemaStore persists Registry.customSchemas to a JSON file so that
// per-schema metadata (Custom flag, OwnerKey, IssuerDisplayName,
// AdditionalTypes) survives container restarts. Mirrors auth.UserStore:
// single file mounted RW into the container, overwrite-in-place writes
// (rename would break the Docker single-file bind mount), defaults to
// empty on missing file so first-boot is non-fatal.
//
// Walt.id's HOCON catalog also persists schema definitions on disk —
// what THIS store adds is the verifiably-go-specific metadata that
// doesn't round-trip through walt.id's wellknown:
//   - OwnerKey       — per-OIDC-subject scoping for ListSchemas
//   - IssuerDisplayName — human-readable attribution string
//   - Custom flag    — distinguishes user-built from stock walt.id types
//   - DPGs slice     — which adapter(s) the schema was saved through
//
// Without this store, after a restart the catalog still has the type
// definitions, but the metadata is gone — scoping breaks, attribution
// disappears, and a stock walt.id type can't be told apart from a
// user-built one.
type SchemaStore struct {
	path string
	mu   sync.Mutex
}

// NewSchemaStore returns a store pointed at path. Constructor stays
// infallible so callers can wire it during startup unconditionally.
func NewSchemaStore(path string) *SchemaStore {
	return &SchemaStore{path: path}
}

// Path returns the file path the store writes to.
func (s *SchemaStore) Path() string { return s.path }

// Load reads the schema file. Missing file → empty slice (not an
// error). Malformed file → error so the boot loader can decide whether
// to fail loudly or fall back.
func (s *SchemaStore) Load() ([]vctypes.Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *SchemaStore) loadLocked() ([]vctypes.Schema, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", s.path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}
	var out []vctypes.Schema
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	return out, nil
}

// Save overwrites the file in place. See auth.UserStore.Save for the
// rationale (rename breaks Docker single-file bind mounts).
func (s *SchemaStore) Save(schemas []vctypes.Schema) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(schemas)
}

func (s *SchemaStore) saveLocked(schemas []vctypes.Schema) error {
	if schemas == nil {
		schemas = []vctypes.Schema{}
	}
	b, err := json.MarshalIndent(schemas, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schemas: %w", err)
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(s.path), err)
	}
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return nil
}

// Upsert inserts schema (replacing any existing entry with the same
// ID) and writes the resulting slice. Returns the full updated set so
// the registry can use it without a follow-up Load.
func (s *SchemaStore) Upsert(schema vctypes.Schema) ([]vctypes.Schema, error) {
	if strings.TrimSpace(schema.ID) == "" {
		return nil, fmt.Errorf("schema id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	replaced := false
	for i := range cur {
		if cur[i].ID == schema.ID {
			cur[i] = schema
			replaced = true
			break
		}
	}
	if !replaced {
		cur = append(cur, schema)
	}
	if err := s.saveLocked(cur); err != nil {
		return nil, err
	}
	return cur, nil
}

// Remove drops the schema with the given id. Returns (remaining, true)
// when a row was removed, (remaining, false) when no entry matched.
// Idempotent: a second Remove of the same id is a no-op write.
func (s *SchemaStore) Remove(id string) ([]vctypes.Schema, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.loadLocked()
	if err != nil {
		return nil, false, err
	}
	idx := -1
	for i, x := range cur {
		if x.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return cur, false, nil
	}
	cur = append(cur[:idx], cur[idx+1:]...)
	if err := s.saveLocked(cur); err != nil {
		return nil, false, err
	}
	return cur, true, nil
}
