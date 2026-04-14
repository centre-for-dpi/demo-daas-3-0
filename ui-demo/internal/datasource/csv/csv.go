// Package csv implements the DataSource interface against a CSV file.
// Useful for ministries that have CSV exports as the only available data source.
package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"vcplatform/internal/datasource"
)

// Config describes a CSV data source.
type Config struct {
	DisplayName string
	Summary     string
	FilePath    string // path to the CSV file (must include header row)
	PrimaryKey  string // column name used as the unique identifier
}

// Source is a live CSV data source.
type Source struct {
	cfg     Config
	headers []string
	rows    []datasource.Record
}

// New creates and loads a CSV data source.
func New(cfg Config) (*Source, error) {
	if cfg.DisplayName == "" {
		cfg.DisplayName = "CSV: " + cfg.FilePath
	}
	if cfg.PrimaryKey == "" {
		cfg.PrimaryKey = "id"
	}
	s := &Source{cfg: cfg}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Source) load() error {
	f, err := os.Open(s.cfg.FilePath)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	all, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}
	if len(all) < 2 {
		return fmt.Errorf("csv must have header + at least one data row")
	}
	s.headers = all[0]
	for _, row := range all[1:] {
		rec := datasource.Record{}
		for i, h := range s.headers {
			if i < len(row) {
				rec[h] = row[i]
			}
		}
		s.rows = append(s.rows, rec)
	}
	return nil
}

func (s *Source) Name() string { return s.cfg.DisplayName }
func (s *Source) Kind() string { return "csv" }

func (s *Source) Describe(ctx context.Context) (*datasource.Description, error) {
	fields := make([]datasource.FieldDescriptor, 0, len(s.headers))
	for _, h := range s.headers {
		fields = append(fields, datasource.FieldDescriptor{Name: h, Type: "string"})
	}
	return &datasource.Description{
		DisplayName:  s.cfg.DisplayName,
		Summary:      s.cfg.Summary,
		TotalRecords: len(s.rows),
		Fields:       fields,
	}, nil
}

func (s *Source) FetchRecord(ctx context.Context, id string) (datasource.Record, error) {
	for _, r := range s.rows {
		if v, ok := r[s.cfg.PrimaryKey]; ok && fmt.Sprintf("%v", v) == id {
			return r, nil
		}
	}
	return nil, datasource.ErrNotFound
}

func (s *Source) ListRecords(ctx context.Context, f datasource.Filter) ([]datasource.Record, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	out := []datasource.Record{}
	skipped := 0
	for _, r := range s.rows {
		if f.Field != "" && f.Equals != nil {
			v, ok := r[f.Field]
			if !ok || fmt.Sprintf("%v", v) != fmt.Sprintf("%v", f.Equals) {
				continue
			}
		}
		if skipped < f.Offset {
			skipped++
			continue
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Source) SearchByField(ctx context.Context, field string, value any) ([]datasource.Record, error) {
	return s.ListRecords(ctx, datasource.Filter{Field: field, Equals: value})
}

// _ keeps strings import alive for any future helpers.
var _ = strings.HasPrefix
