package issuance

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendListGet(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(filepath.Join(dir, "log.json"))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	a := IssuedCredential{
		ID: "vc-1", SchemaID: "sx", SchemaName: "Driver License",
		Std: "w3c_vcdm_2", Format: "ldp_vc", IssuerDpg: "Walt Community Stack",
		HolderHint: "Wanjiru",
		SubjectFields: map[string]string{"fullName": "Wanjiru", "id": "X"},
		StatusList: &StatusListEntry{Type: "bitstring", ListID: "v1", Index: 0},
	}
	if _, err := l.Append(a); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := l.Append(a); err == nil {
		t.Fatal("duplicate id should error")
	}
	got, ok := l.Get("vc-1")
	if !ok || got.ID != "vc-1" {
		t.Fatalf("Get: ok=%v got=%+v", ok, got)
	}
	if got.IssuedAt.IsZero() {
		t.Fatal("IssuedAt should auto-populate")
	}
}

func TestListFilter(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(filepath.Join(dir, "log.json"))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	mustAppend := func(c IssuedCredential) {
		if _, err := l.Append(c); err != nil {
			t.Fatalf("append %s: %v", c.ID, err)
		}
	}
	mustAppend(IssuedCredential{ID: "a", SchemaName: "Driver License", Std: "w3c_vcdm_2", Format: "ldp_vc", HolderHint: "Wanjiru"})
	mustAppend(IssuedCredential{ID: "b", SchemaName: "Health Card", Std: "sd_jwt_vc (IETF)", Format: "vc+sd-jwt", HolderHint: "Otieno", SubjectFields: map[string]string{"fullName": "Otieno"}})
	mustAppend(IssuedCredential{ID: "c", SchemaName: "Mobile DL", Std: "mso_mdoc", Format: "mso_mdoc", HolderHint: "Achieng"})

	if got := l.List(Filter{}); len(got) != 3 {
		t.Fatalf("no filter: got %d, want 3", len(got))
	}
	if got := l.List(Filter{Std: "w3c_vcdm_2"}); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("std filter: got %+v", got)
	}
	if got := l.List(Filter{Query: "wanjiru"}); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("query holder hint case-insensitive: got %+v", got)
	}
	if got := l.List(Filter{Query: "health"}); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("query schema name: got %+v", got)
	}
	if got := l.List(Filter{Query: "otieno"}); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("query into subject fields: got %+v", got)
	}
	if got := l.List(Filter{Format: "vc+sd-jwt"}); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("format filter: got %+v", got)
	}
}

func TestRevoke(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(filepath.Join(dir, "log.json"))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := l.Append(IssuedCredential{ID: "a", IssuedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := l.MarkRevoked("a")
	if err != nil {
		t.Fatalf("MarkRevoked: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("RevokedAt should be set")
	}
	// Revoking again is idempotent.
	if _, err := l.MarkRevoked("a"); err != nil {
		t.Fatalf("MarkRevoked again: %v", err)
	}
	if _, err := l.MarkRevoked("missing"); err == nil {
		t.Fatal("MarkRevoked on missing id should error")
	}
	// state filter
	if got := l.List(Filter{State: "active"}); len(got) != 0 {
		t.Fatalf("state=active should be 0: got %+v", got)
	}
	if got := l.List(Filter{State: "revoked"}); len(got) != 1 {
		t.Fatalf("state=revoked should be 1: got %+v", got)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.json")
	l1, err := NewLog(path)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := l1.Append(IssuedCredential{ID: "x", SchemaName: "Z"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	l2, err := NewLog(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := l2.List(Filter{})
	if len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("persistence: got %+v", got)
	}
}

func TestSummary(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(filepath.Join(dir, "log.json"))
	if err != nil {
		t.Fatal(err)
	}
	mustAppend := func(c IssuedCredential) {
		if _, err := l.Append(c); err != nil {
			t.Fatal(err)
		}
	}
	mustAppend(IssuedCredential{ID: "1", Std: "w3c_vcdm_2", Format: "ldp_vc"})
	mustAppend(IssuedCredential{ID: "2", Std: "w3c_vcdm_2", Format: "ldp_vc"})
	mustAppend(IssuedCredential{ID: "3", Std: "sd_jwt_vc (IETF)", Format: "vc+sd-jwt"})
	if _, err := l.MarkRevoked("3"); err != nil {
		t.Fatal(err)
	}
	s := l.Summary()
	if s.Total != 3 || s.Active != 2 || s.Revoked != 1 {
		t.Fatalf("totals: %+v", s)
	}
	if s.ByStd["w3c_vcdm_2"] != 2 || s.ByStd["sd_jwt_vc (IETF)"] != 1 {
		t.Fatalf("ByStd: %+v", s.ByStd)
	}
}
