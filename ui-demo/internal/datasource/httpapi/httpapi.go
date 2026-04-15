// Package httpapi implements the DataSource interface against a generic
// REST API. Useful for ministries with custom citizen registries exposed
// over HTTP, including Sunbird RC instances.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"vcplatform/internal/datasource"
)

// Config describes an HTTP API data source.
type Config struct {
	DisplayName string
	Summary     string

	// BaseURL is the API base, e.g. "https://registry.example.gov/api/v1"
	BaseURL string

	// FetchPath is the path template for fetching one record.
	// Use {id} as a placeholder for the primary key.
	// e.g. "/citizens/{id}"
	FetchPath string

	// ListPath is the path for listing records.
	// e.g. "/citizens"
	ListPath string

	// AuthHeader is an optional Authorization header value (e.g. "Bearer xxx").
	AuthHeader string

	// PrimaryKey is the JSON field name used as the unique identifier.
	PrimaryKey string

	// RecordsRoot is an optional JSONPath-like prefix for list responses.
	// e.g. "data.records" → response["data"]["records"]
	RecordsRoot string
}

// Source is a live HTTP API data source.
type Source struct {
	cfg    Config
	client *http.Client
}

// New creates a new HTTP API data source.
func New(cfg Config) *Source {
	if cfg.DisplayName == "" {
		cfg.DisplayName = "API: " + cfg.BaseURL
	}
	if cfg.PrimaryKey == "" {
		cfg.PrimaryKey = "id"
	}
	return &Source{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Source) Name() string { return s.cfg.DisplayName }
func (s *Source) Kind() string { return "httpapi" }

func (s *Source) Describe(ctx context.Context) (*datasource.Description, error) {
	return &datasource.Description{
		DisplayName: s.cfg.DisplayName,
		Summary:     s.cfg.Summary,
	}, nil
}

func (s *Source) FetchRecord(ctx context.Context, id string) (datasource.Record, error) {
	path := strings.ReplaceAll(s.cfg.FetchPath, "{id}", id)
	body, err := s.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var rec datasource.Record
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, fmt.Errorf("parse record: %w", err)
	}
	return rec, nil
}

func (s *Source) ListRecords(ctx context.Context, f datasource.Filter) ([]datasource.Record, error) {
	body, err := s.do(ctx, "GET", s.cfg.ListPath, nil)
	if err != nil {
		return nil, err
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse list: %w", err)
	}
	// Walk to the records root if specified
	cur := raw
	if s.cfg.RecordsRoot != "" {
		for _, p := range strings.Split(s.cfg.RecordsRoot, ".") {
			if m, ok := cur.(map[string]any); ok {
				cur = m[p]
			} else {
				return nil, fmt.Errorf("records root path %q not found", s.cfg.RecordsRoot)
			}
		}
	}
	arr, ok := cur.([]any)
	if !ok {
		return nil, fmt.Errorf("response is not an array")
	}
	out := make([]datasource.Record, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, datasource.Record(m))
		}
	}
	return out, nil
}

func (s *Source) SearchByField(ctx context.Context, field string, value any) ([]datasource.Record, error) {
	all, err := s.ListRecords(ctx, datasource.Filter{})
	if err != nil {
		return nil, err
	}
	var out []datasource.Record
	for _, r := range all {
		if v, ok := r[field]; ok && fmt.Sprintf("%v", v) == fmt.Sprintf("%v", value) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Search runs a case-insensitive substring match across every string-valued
// field in every record. Fine for small datasets fetched via ListRecords;
// real HTTP APIs should override this with a remote query.
func (s *Source) Search(ctx context.Context, query string, limit int) ([]datasource.Record, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return []datasource.Record{}, nil
	}
	all, err := s.ListRecords(ctx, datasource.Filter{})
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 25
	}
	var out []datasource.Record
	for _, r := range all {
		for _, v := range r {
			if sv, ok := v.(string); ok && strings.Contains(strings.ToLower(sv), query) {
				out = append(out, r)
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Source) do(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if s.cfg.AuthHeader != "" {
		req.Header.Set("Authorization", s.cfg.AuthHeader)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
