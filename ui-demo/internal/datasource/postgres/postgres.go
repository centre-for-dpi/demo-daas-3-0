// Package postgres implements the DataSource interface against a Postgres
// table. It's the primary data source for the v1 demo, backed by the
// citizens database (200 seeded records covering Kenya + Trinidad).
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"

	"vcplatform/internal/datasource"
)

// Config describes a Postgres data source: connection plus which table/columns to expose.
type Config struct {
	// Name shown in the UI ("Citizens DB", "Farmer Registry", ...).
	DisplayName string

	// Summary text shown in the UI.
	Summary string

	// DSN is the Postgres connection string, e.g.
	//   "host=citizens-postgres port=5432 user=citizens password=citizens dbname=citizens sslmode=disable"
	DSN string

	// Table is the table name to query.
	Table string

	// PrimaryKey is the column used to fetch a single record (e.g. "national_id").
	PrimaryKey string

	// SearchFields are the columns Search() matches against with ILIKE.
	// Typical values: ["national_id","first_name","last_name","email","student_id"].
	// If empty, Search() only matches the PrimaryKey.
	SearchFields []string

	// Fields lists the columns this source exposes. If empty, all columns are exposed.
	Fields []datasource.FieldDescriptor

	// SuggestedMappings maps credential type name → claim → column.
	SuggestedMappings map[string]map[string]string
}

// Source is the live Postgres data source.
type Source struct {
	cfg Config
	db  *sql.DB
}

// New creates a new Postgres data source. It opens the connection lazily —
// the constructor does not error on unreachable databases so the rest of the
// app can boot even when Postgres is still warming up.
func New(cfg Config) *Source {
	if cfg.DisplayName == "" {
		cfg.DisplayName = "Postgres: " + cfg.Table
	}
	if cfg.PrimaryKey == "" {
		cfg.PrimaryKey = "id"
	}
	return &Source{cfg: cfg}
}

// connect opens the database lazily. Subsequent calls reuse the open *sql.DB.
func (s *Source) connect() error {
	if s.db != nil {
		return nil
	}
	db, err := sql.Open("postgres", s.cfg.DSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping postgres: %w", err)
	}
	s.db = db
	return nil
}

func (s *Source) Name() string { return s.cfg.DisplayName }
func (s *Source) Kind() string { return "postgres" }

// Describe returns metadata about this source — column list and credential mappings.
func (s *Source) Describe(ctx context.Context) (*datasource.Description, error) {
	desc := &datasource.Description{
		DisplayName:       s.cfg.DisplayName,
		Summary:           s.cfg.Summary,
		Fields:            s.cfg.Fields,
		SuggestedMappings: s.cfg.SuggestedMappings,
	}
	if err := s.connect(); err == nil {
		row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+s.cfg.Table)
		var count int
		if err := row.Scan(&count); err == nil {
			desc.TotalRecords = count
		}
	}
	return desc, nil
}

// FetchRecord returns a single record by primary key.
func (s *Source) FetchRecord(ctx context.Context, id string) (datasource.Record, error) {
	if err := s.connect(); err != nil {
		return nil, err
	}
	q := fmt.Sprintf("SELECT * FROM %s WHERE %s = $1 LIMIT 1", s.cfg.Table, s.cfg.PrimaryKey)
	rows, err := s.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	records, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, datasource.ErrNotFound
	}
	return records[0], nil
}

// ListRecords returns multiple records matching an optional filter.
func (s *Source) ListRecords(ctx context.Context, f datasource.Filter) ([]datasource.Record, error) {
	if err := s.connect(); err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	var (
		q    strings.Builder
		args []any
	)
	q.WriteString("SELECT * FROM ")
	q.WriteString(s.cfg.Table)
	if f.Field != "" && f.Equals != nil {
		q.WriteString(" WHERE ")
		q.WriteString(quoteIdent(f.Field))
		q.WriteString(" = $1")
		args = append(args, f.Equals)
	}
	q.WriteString(fmt.Sprintf(" ORDER BY %s LIMIT %d OFFSET %d", s.cfg.PrimaryKey, limit, f.Offset))

	rows, err := s.db.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// SearchByField is a convenience wrapper around ListRecords with field filter.
func (s *Source) SearchByField(ctx context.Context, field string, value any) ([]datasource.Record, error) {
	return s.ListRecords(ctx, datasource.Filter{Field: field, Equals: value})
}

// Search runs a free-text query across all configured SearchFields using
// ILIKE %q%. Falls back to exact PrimaryKey match if SearchFields is empty.
// A limit of 0 defaults to 25.
func (s *Source) Search(ctx context.Context, query string, limit int) ([]datasource.Record, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []datasource.Record{}, nil
	}
	if err := s.connect(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 25
	}

	fields := s.cfg.SearchFields
	if len(fields) == 0 {
		fields = []string{s.cfg.PrimaryKey}
	}

	var (
		whereParts []string
		args       []any
	)
	like := "%" + query + "%"
	for _, f := range fields {
		args = append(args, like)
		whereParts = append(whereParts, fmt.Sprintf("%s::text ILIKE $%d", quoteIdent(f), len(args)))
	}
	q := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s ORDER BY %s LIMIT %d",
		s.cfg.Table,
		strings.Join(whereParts, " OR "),
		s.cfg.PrimaryKey,
		limit,
	)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// scanRows reads all rows from a *sql.Rows into a slice of Records, mapping
// each column to its Go type via sql.RawBytes-aware scanning.
func scanRows(rows *sql.Rows) ([]datasource.Record, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}
	out := []datasource.Record{}
	for rows.Next() {
		// Use *any per column so the driver decodes to native Go types.
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		rec := datasource.Record{}
		for i, c := range cols {
			rec[c] = normalize(vals[i])
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// normalize converts driver-returned types to JSON-friendly Go types.
// []byte (from text columns) becomes string; time.Time stays as time.Time
// (encoded later by encoding/json as RFC3339).
func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case nil:
		return nil
	default:
		return x
	}
}

// quoteIdent does a minimal SQL identifier quote — enough for column names from
// our config (we don't accept user input for column names).
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
