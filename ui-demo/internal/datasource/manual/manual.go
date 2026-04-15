// Package manual implements a passthrough DataSource where the issuer enters
// claims directly via the wizard. No backend storage. Default for issuance
// flows that don't need to pull from a real registry.
package manual

import (
	"context"

	"vcplatform/internal/datasource"
)

// Source is a manual passthrough — never returns records, never errors.
type Source struct{}

// New creates a manual data source.
func New() *Source { return &Source{} }

func (s *Source) Name() string { return "Manual entry" }
func (s *Source) Kind() string { return "manual" }

func (s *Source) Describe(ctx context.Context) (*datasource.Description, error) {
	return &datasource.Description{
		DisplayName: "Manual entry",
		Summary:     "Issuer enters credential claims directly via the wizard. No backend storage.",
	}, nil
}

func (s *Source) FetchRecord(_ context.Context, _ string) (datasource.Record, error) {
	return nil, datasource.ErrNotFound
}

func (s *Source) ListRecords(_ context.Context, _ datasource.Filter) ([]datasource.Record, error) {
	return []datasource.Record{}, nil
}

func (s *Source) SearchByField(_ context.Context, _ string, _ any) ([]datasource.Record, error) {
	return []datasource.Record{}, nil
}

func (s *Source) Search(_ context.Context, _ string, _ int) ([]datasource.Record, error) {
	return []datasource.Record{}, nil
}
