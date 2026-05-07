package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verifiably/verifiably-go/vctypes"
)

func TestSchemaStore_LoadMissingIsEmpty(t *testing.T) {
	s := NewSchemaStore(filepath.Join(t.TempDir(), "schemas.json"))
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}

func TestSchemaStore_UpsertAndLoadRoundtrip(t *testing.T) {
	s := NewSchemaStore(filepath.Join(t.TempDir(), "schemas.json"))
	in := vctypes.Schema{
		ID:                "custom-abc",
		Name:              "Farmer",
		IssuerDisplayName: "Ministry of Agriculture",
		OwnerKey:          "did:jwk:abc",
		Custom:            true,
		DPGs:              []string{"walt_community"},
	}
	if _, err := s.Upsert(in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if got[0].ID != in.ID || got[0].IssuerDisplayName != in.IssuerDisplayName ||
		got[0].OwnerKey != in.OwnerKey || got[0].Custom != true {
		t.Errorf("metadata didn't round-trip: %+v", got[0])
	}
}

func TestSchemaStore_UpsertReplacesByID(t *testing.T) {
	s := NewSchemaStore(filepath.Join(t.TempDir(), "schemas.json"))
	if _, err := s.Upsert(vctypes.Schema{ID: "x", Name: "First", Custom: true}); err != nil {
		t.Fatalf("upsert#1: %v", err)
	}
	if _, err := s.Upsert(vctypes.Schema{ID: "x", Name: "Second", Custom: true}); err != nil {
		t.Fatalf("upsert#2: %v", err)
	}
	got, _ := s.Load()
	if len(got) != 1 {
		t.Fatalf("expected single entry after upsert, got %d", len(got))
	}
	if got[0].Name != "Second" {
		t.Errorf("upsert did not replace; got %q", got[0].Name)
	}
}

func TestSchemaStore_RemoveReportsHit(t *testing.T) {
	s := NewSchemaStore(filepath.Join(t.TempDir(), "schemas.json"))
	_, _ = s.Upsert(vctypes.Schema{ID: "a", Custom: true})
	_, _ = s.Upsert(vctypes.Schema{ID: "b", Custom: true})

	rest, ok, err := s.Remove("a")
	if err != nil {
		t.Fatalf("remove a: %v", err)
	}
	if !ok || len(rest) != 1 || rest[0].ID != "b" {
		t.Fatalf("first remove unexpected: ok=%v rest=%+v", ok, rest)
	}
	rest, ok, err = s.Remove("a")
	if err != nil {
		t.Fatalf("idempotent remove: %v", err)
	}
	if ok {
		t.Fatal("second Remove(a) returned ok=true; want false")
	}
	if len(rest) != 1 {
		t.Errorf("rest changed unexpectedly: %+v", rest)
	}
}

// TestSchemaStore_OverwriteInPlace pins the inode-stable guarantee —
// Save must write to the same inode every time so the deploy.sh
// single-file bind mount stays valid. Mirrors the equivalent test on
// auth.UserStore.
func TestSchemaStore_OverwriteInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schemas.json")
	s := NewSchemaStore(path)
	if err := s.Save([]vctypes.Schema{{ID: "first", Custom: true}}); err != nil {
		t.Fatalf("save#1: %v", err)
	}
	first, _ := os.Stat(path)
	if err := s.Save([]vctypes.Schema{{ID: "second", Custom: true}}); err != nil {
		t.Fatalf("save#2: %v", err)
	}
	second, _ := os.Stat(path)
	if !os.SameFile(first, second) {
		t.Error("Save changed inode — would break a Docker single-file bind mount")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("unexpected scratch file %q in dir", e.Name())
		}
	}
}

// TestRegistry_AttachSchemaStore_ReplaysFile pins the boot path:
// AttachSchemaStore reads the file and seeds Registry.customSchemas
// before any adapter is registered, so a new container picks up where
// the previous one left off. Non-Custom rows are filtered out
// defensively (the file is meant for user-built entries only).
func TestRegistry_AttachSchemaStore_ReplaysFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schemas.json")
	preseed := NewSchemaStore(path)
	if err := preseed.Save([]vctypes.Schema{
		{ID: "alice-cred", Name: "Alice", Custom: true, IssuerDisplayName: "MoH"},
		{ID: "ghost", Name: "Stock", Custom: false}, // must be filtered
	}); err != nil {
		t.Fatal(err)
	}
	r := New()
	if err := r.AttachSchemaStore(NewSchemaStore(path)); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if len(r.customSchemas) != 1 {
		t.Fatalf("expected 1 schema replayed (Custom-only), got %d (%+v)", len(r.customSchemas), r.customSchemas)
	}
	if r.customSchemas[0].ID != "alice-cred" {
		t.Errorf("wrong schema replayed: %+v", r.customSchemas[0])
	}
	if r.customSchemas[0].IssuerDisplayName != "MoH" {
		t.Errorf("metadata didn't replay: %+v", r.customSchemas[0])
	}
}
