package registry

import (
	"context"
	"testing"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/mock"
	"github.com/verifiably/verifiably-go/vctypes"
)

// TestSaveCustomSchemaStampsOwnerKey pins that SaveCustomSchema reads
// the IssuerIdentity off ctx and stamps it onto the stored schema, so
// later ListSchemas calls can filter by it. Without the stamp the
// schema would stay globally visible — the regression we're guarding
// against here is "every issuer sees every other issuer's schemas".
func TestSaveCustomSchemaStampsOwnerKey(t *testing.T) {
	r := New()
	r.Register("walt.id", vctypes.DPG{Vendor: "walt.id"}, []string{"issuer"}, mock.NewAdapter())

	schema := vctypes.Schema{
		ID: "alice-1", Name: "Alice's Cred", Std: "w3c_vcdm_2",
		DPGs: []string{"walt.id"},
	}
	ctx := backend.WithIssuerIdentity(context.Background(), "alice")
	if err := r.SaveCustomSchema(ctx, schema); err != nil {
		t.Fatalf("SaveCustomSchema: %v", err)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.customSchemas) != 1 {
		t.Fatalf("want 1 stored schema, got %d", len(r.customSchemas))
	}
	if got := r.customSchemas[0].OwnerKey; got != "alice" {
		t.Fatalf("OwnerKey not stamped: got %q want %q", got, "alice")
	}
	if !r.customSchemas[0].Custom {
		t.Fatal("Custom flag should be set by SaveCustomSchema")
	}
}

// TestListSchemasFiltersByOwner confirms each issuer only sees their
// own custom schemas. Stock vendor schemas (no OwnerKey) stay shared.
// An admin context (no IssuerIdentity attached) sees every entry —
// preserves the CLI/migration use case.
func TestListSchemasFiltersByOwner(t *testing.T) {
	r := New()
	r.Register("walt.id", vctypes.DPG{Vendor: "walt.id"}, []string{"issuer"}, mock.NewAdapter())

	mustSave := func(ownerKey, id string) {
		ctx := backend.WithIssuerIdentity(context.Background(), ownerKey)
		err := r.SaveCustomSchema(ctx, vctypes.Schema{
			ID: id, Name: id, Std: "w3c_vcdm_2", DPGs: []string{"walt.id"},
		})
		if err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	mustSave("alice", "alice-1")
	mustSave("alice", "alice-2")
	mustSave("bob", "bob-1")

	// Inject a legacy custom schema (pre-scoping — empty OwnerKey).
	// Should be visible to everyone, just like a stock vendor entry.
	r.mu.Lock()
	r.customSchemas = append(r.customSchemas, vctypes.Schema{
		ID: "legacy-1", Name: "Legacy", Std: "w3c_vcdm_2",
		DPGs: []string{"walt.id"}, Custom: true,
	})
	r.mu.Unlock()

	hasID := func(list []vctypes.Schema, id string) bool {
		for _, s := range list {
			if s.ID == id {
				return true
			}
		}
		return false
	}

	// Alice's view.
	aliceCtx := backend.WithIssuerIdentity(context.Background(), "alice")
	got, _ := r.ListSchemas(aliceCtx, "walt.id")
	if !hasID(got, "alice-1") || !hasID(got, "alice-2") {
		t.Fatalf("alice should see her schemas: %+v", got)
	}
	if hasID(got, "bob-1") {
		t.Fatalf("alice MUST NOT see bob-1")
	}
	if !hasID(got, "legacy-1") {
		t.Fatalf("alice should see legacy (empty OwnerKey): %+v", got)
	}

	// Bob's view.
	bobCtx := backend.WithIssuerIdentity(context.Background(), "bob")
	got, _ = r.ListSchemas(bobCtx, "walt.id")
	if !hasID(got, "bob-1") {
		t.Fatalf("bob should see his own: %+v", got)
	}
	if hasID(got, "alice-1") || hasID(got, "alice-2") {
		t.Fatalf("bob MUST NOT see alice's schemas")
	}

	// Admin (no IssuerIdentity) sees everyone.
	got, _ = r.ListSchemas(context.Background(), "walt.id")
	for _, id := range []string{"alice-1", "alice-2", "bob-1", "legacy-1"} {
		if !hasID(got, id) {
			t.Fatalf("admin view missing %s: %+v", id, got)
		}
	}
}

// TestDeleteCustomSchemaOwnerCheck pins that issuer B can't delete
// issuer A's schema by guessing the id. The delete is surfaced as
// not-found so the existence isn't disclosed across owners.
func TestDeleteCustomSchemaOwnerCheck(t *testing.T) {
	r := New()
	r.Register("walt.id", vctypes.DPG{Vendor: "walt.id"}, []string{"issuer"}, mock.NewAdapter())

	aliceCtx := backend.WithIssuerIdentity(context.Background(), "alice")
	if err := r.SaveCustomSchema(aliceCtx, vctypes.Schema{
		ID: "alice-1", Name: "X", Std: "w3c_vcdm_2", DPGs: []string{"walt.id"},
	}); err != nil {
		t.Fatal(err)
	}

	// Bob attempts delete by id-guess.
	bobCtx := backend.WithIssuerIdentity(context.Background(), "bob")
	if err := r.DeleteCustomSchema(bobCtx, "alice-1"); err == nil {
		t.Fatal("bob should not be able to delete alice-1")
	}
	r.mu.RLock()
	if len(r.customSchemas) != 1 {
		r.mu.RUnlock()
		t.Fatalf("alice's schema should still be stored, got %d", len(r.customSchemas))
	}
	r.mu.RUnlock()

	// Alice can delete her own.
	if err := r.DeleteCustomSchema(aliceCtx, "alice-1"); err != nil {
		t.Fatalf("alice delete: %v", err)
	}
	r.mu.RLock()
	if len(r.customSchemas) != 0 {
		r.mu.RUnlock()
		t.Fatalf("schema should be gone after alice's delete")
	}
	r.mu.RUnlock()
}
