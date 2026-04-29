package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/mock"
	"github.com/verifiably/verifiably-go/vctypes"
)

// erroringIssuer always fails ListSchemas — used to simulate walt.id being
// restarted by its own SaveCustomSchema hook. Embedding mock.MockAdapter
// gives us the rest of backend.Adapter for free.
type erroringIssuer struct {
	*mock.MockAdapter
}

func (e *erroringIssuer) ListSchemas(_ context.Context, _ string) ([]vctypes.Schema, error) {
	return nil, errors.New("dial tcp 172.24.0.15:7002: connect: connection refused")
}

// TestListSchemas_VendorErrorStillReturnsCustomSchemas guards against the
// regression where the schema browser went completely empty during walt.id
// restarts — the registry was returning early on vendor error and dropping
// the in-memory custom schemas the user had just saved.
//
// New contract: vendor error is returned alongside whatever custom schemas
// matched the DPG, so the handler can render a banner + custom-schema
// cards instead of bleeding an http.Error into the page body.
func TestListSchemas_VendorErrorStillReturnsCustomSchemas(t *testing.T) {
	reg := New()
	ad := &erroringIssuer{MockAdapter: mock.NewAdapter()}
	reg.issuers["Walt Community Stack"] = ad
	reg.issuerDPGs["Walt Community Stack"] = vctypes.DPG{}

	// Pre-seed a custom schema attached to the vendor.
	if err := reg.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID:     "custom-keep-me",
		Name:   "Keep Me",
		Std:    "w3c_vcdm_2",
		DPGs:   []string{"Walt Community Stack"},
		Custom: true,
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	got, err := reg.ListSchemas(context.Background(), "Walt Community Stack")
	if err == nil {
		t.Fatalf("expected vendor error to propagate")
	}
	if len(got) != 1 || got[0].ID != "custom-keep-me" {
		t.Errorf("expected custom-keep-me to survive vendor outage, got %v", got)
	}
}

// TestListSchemas_VendorOK_AppendsCustomSchemas keeps the happy-path
// behaviour intact: when the vendor returns its catalog, custom schemas
// for the same DPG are appended after.
func TestListSchemas_VendorOK_AppendsCustomSchemas(t *testing.T) {
	reg := New()
	ad := mock.NewAdapter()
	// Seed mock with one stock schema via its public Save path.
	_ = ad.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID:     "stock-1",
		Name:   "Stock",
		DPGs:   []string{"Walt Community Stack"},
		Custom: false,
	})
	reg.issuers["Walt Community Stack"] = ad
	reg.issuerDPGs["Walt Community Stack"] = vctypes.DPG{}

	if err := reg.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID:     "custom-1",
		Name:   "Custom",
		Std:    "w3c_vcdm_2",
		DPGs:   []string{"Walt Community Stack"},
		Custom: true,
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	got, err := reg.ListSchemas(context.Background(), "Walt Community Stack")
	if err != nil {
		t.Fatalf("ListSchemas: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected at least 2 schemas (stock + custom), got %d", len(got))
	}
	// The custom schema should appear in the result.
	foundCustom := false
	for _, s := range got {
		if s.ID == "custom-1" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Errorf("custom schema missing from happy-path result: %v", got)
	}
}

var _ backend.Adapter = (*erroringIssuer)(nil)
