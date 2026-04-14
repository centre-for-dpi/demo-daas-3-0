// Package datasource defines the plugin interface for pulling credential
// subject data from real-world backend systems (Postgres, CSV, HTTP APIs,
// Sunbird RC, etc.). Data sources are orthogonal to DPG adaptors — an
// issuance flow fetches data from a DataSource then hands it to the
// configured IssuerStore.
package datasource

import (
	"context"
	"fmt"
	"sync"
)

// Record is a single row from a data source, keyed by column/field name.
type Record map[string]any

// Filter describes a query against a DataSource. All fields are optional.
type Filter struct {
	Field  string // e.g. "country_code"
	Equals any    // e.g. "KE"
	Limit  int    // max rows to return; 0 = default 100
	Offset int
}

// DataSource is the interface every backing data store implements.
// Adaptors: postgres, csv, httpapi, manual.
type DataSource interface {
	// Name returns a short human-friendly identifier ("Postgres: Citizens", "CSV upload", ...).
	Name() string

	// Kind returns the adaptor kind ("postgres", "csv", "httpapi", "manual").
	Kind() string

	// Describe returns metadata about what this data source provides,
	// including the list of available fields and how they map to credential claims.
	Describe(ctx context.Context) (*Description, error)

	// FetchRecord fetches a single record by primary key (e.g. national_id).
	FetchRecord(ctx context.Context, id string) (Record, error)

	// ListRecords returns multiple records matching an optional filter.
	ListRecords(ctx context.Context, f Filter) ([]Record, error)

	// SearchByField finds records where field == value (convenience).
	SearchByField(ctx context.Context, field string, value any) ([]Record, error)
}

// Description documents what fields a data source provides. This drives
// the issuance UI: when a user picks a data source + credential type,
// the form shows the available fields and lets them map to credential claims.
type Description struct {
	// Display name for the UI.
	DisplayName string `json:"displayName"`

	// Short description of what this source contains.
	Summary string `json:"summary"`

	// Total records available (0 if unknown).
	TotalRecords int `json:"totalRecords"`

	// Fields lists the available columns/JSON keys with their types.
	Fields []FieldDescriptor `json:"fields"`

	// SuggestedMappings maps a credential type name → field mappings
	// (credential claim name → source field name).
	SuggestedMappings map[string]map[string]string `json:"suggestedMappings,omitempty"`
}

// FieldDescriptor describes a single field/column in the data source.
type FieldDescriptor struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "string", "number", "date", "boolean"
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Registry holds all currently registered data sources, keyed by name.
// Sources are typically registered at startup from config.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]DataSource
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{sources: map[string]DataSource{}}
}

// Register adds a data source to the registry under its Name().
func (r *Registry) Register(ds DataSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[ds.Name()] = ds
}

// Get returns the named data source or nil if not found.
func (r *Registry) Get(name string) DataSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sources[name]
}

// List returns all registered data sources.
func (r *Registry) List() []DataSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]DataSource, 0, len(r.sources))
	for _, ds := range r.sources {
		out = append(out, ds)
	}
	return out
}

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = fmt.Errorf("record not found")
