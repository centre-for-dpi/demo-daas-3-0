package registry

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/mock"
	"github.com/verifiably/verifiably-go/vctypes"
)

// dispatchSpy wraps mock.MockAdapter to record SaveCustomSchema /
// DeleteCustomSchema calls so dispatch tests can assert which adapters were
// invoked and with what schema. Pre-Phase-1 the registry never called the
// adapter hooks at all (Save/Delete only mutated the registry's own slice),
// so this guard locks in the new behaviour.
type dispatchSpy struct {
	*mock.MockAdapter
	mu       sync.Mutex
	saved    []vctypes.Schema
	deleted  []string
	saveErr  error
	delError error
}

func newDispatchSpy() *dispatchSpy { return &dispatchSpy{MockAdapter: mock.NewAdapter()} }

func (d *dispatchSpy) SaveCustomSchema(ctx context.Context, s vctypes.Schema) error {
	d.mu.Lock()
	d.saved = append(d.saved, s)
	d.mu.Unlock()
	if d.saveErr != nil {
		return d.saveErr
	}
	return d.MockAdapter.SaveCustomSchema(ctx, s)
}

func (d *dispatchSpy) DeleteCustomSchema(ctx context.Context, id string) error {
	d.mu.Lock()
	d.deleted = append(d.deleted, id)
	d.mu.Unlock()
	if d.delError != nil {
		return d.delError
	}
	return d.MockAdapter.DeleteCustomSchema(ctx, id)
}

func (d *dispatchSpy) savedCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.saved)
}

func (d *dispatchSpy) deletedCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.deleted)
}

// TestSaveCustomSchemaDispatchesToDPGAdapters guards against a regression of
// the pre-Phase-1 bug where Registry.SaveCustomSchema was a pure in-memory
// append and never told the adapter — so walt.id's catalog edit + restart
// hook never fired, and custom schemas always took the borrow-trick path.
func TestSaveCustomSchemaDispatchesToDPGAdapters(t *testing.T) {
	reg := New()
	walt := newDispatchSpy()
	inji := newDispatchSpy()
	reg.issuers["Walt Community Stack"] = walt
	reg.issuers["Inji Certify"] = inji

	schema := vctypes.Schema{
		ID:     "custom-disp1",
		Name:   "Dispatch Test",
		Std:    "w3c_vcdm_2",
		DPGs:   []string{"Walt Community Stack"},
		Custom: true,
	}
	if err := reg.SaveCustomSchema(context.Background(), schema); err != nil {
		t.Fatalf("SaveCustomSchema: %v", err)
	}
	if walt.savedCount() != 1 {
		t.Errorf("walt should have received 1 save, got %d", walt.savedCount())
	}
	if inji.savedCount() != 0 {
		t.Errorf("inji should NOT have received the save (not in DPGs), got %d", inji.savedCount())
	}
	walt.mu.Lock()
	if walt.saved[0].ID != schema.ID {
		t.Errorf("dispatched schema ID = %q, want %q", walt.saved[0].ID, schema.ID)
	}
	if !walt.saved[0].Custom {
		t.Errorf("Custom flag should be true on dispatched schema")
	}
	walt.mu.Unlock()
}

// TestSaveCustomSchemaMultiDPG verifies a schema authored against two DPGs
// fans out to both adapters. The current UI doesn't surface multi-DPG custom
// schemas, but the Schema struct supports it and the dispatch code handles
// it — so we lock the contract in.
func TestSaveCustomSchemaMultiDPG(t *testing.T) {
	reg := New()
	a1 := newDispatchSpy()
	a2 := newDispatchSpy()
	reg.issuers["A"] = a1
	reg.issuers["B"] = a2

	if err := reg.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID: "x", Name: "X", Std: "w3c_vcdm_2", DPGs: []string{"A", "B"}, Custom: true,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if a1.savedCount() != 1 || a2.savedCount() != 1 {
		t.Errorf("multi-DPG fan-out: a1=%d a2=%d, want 1/1", a1.savedCount(), a2.savedCount())
	}
}

// TestSaveCustomSchemaAdapterErrorPropagates ensures a failure inside a
// vendor adapter's SaveCustomSchema (e.g. catalog write fails, restart
// times out) surfaces to the caller rather than being swallowed. Operators
// running /issuer/schema/build need to see the failure or the schema list
// goes out of sync with walt.id's catalog.
func TestSaveCustomSchemaAdapterErrorPropagates(t *testing.T) {
	reg := New()
	spy := newDispatchSpy()
	spy.saveErr = errors.New("simulated catalog write failure")
	reg.issuers["A"] = spy

	err := reg.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID: "y", Name: "Y", Std: "w3c_vcdm_2", DPGs: []string{"A"}, Custom: true,
	})
	if err == nil || err.Error() != "simulated catalog write failure" {
		t.Errorf("expected adapter error to propagate; got %v", err)
	}
}

// TestDeleteCustomSchemaDispatchesToDPGAdapters mirrors save: an explicit
// delete must reach the per-DPG adapter so the catalog entry is removed
// from walt.id along with the in-memory record.
func TestDeleteCustomSchemaDispatchesToDPGAdapters(t *testing.T) {
	reg := New()
	walt := newDispatchSpy()
	reg.issuers["Walt Community Stack"] = walt

	schema := vctypes.Schema{ID: "custom-del", Name: "Del", Std: "w3c_vcdm_2", DPGs: []string{"Walt Community Stack"}, Custom: true}
	if err := reg.SaveCustomSchema(context.Background(), schema); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := reg.DeleteCustomSchema(context.Background(), schema.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if walt.deletedCount() != 1 {
		t.Errorf("walt should have received 1 delete, got %d", walt.deletedCount())
	}
}

// TestDeleteCustomSchemaUnknownIDStillErrors keeps the pre-existing contract
// that deleting a missing ID is loud (not silent), so the UI's inevitable
// race conditions surface as toasts rather than ghost-state.
func TestDeleteCustomSchemaUnknownIDStillErrors(t *testing.T) {
	reg := New()
	reg.issuers["A"] = newDispatchSpy()
	if err := reg.DeleteCustomSchema(context.Background(), "nope"); err == nil {
		t.Error("expected error deleting unknown ID")
	}
}

// TestSaveCustomSchemaUnknownDPGSilentlySkipsDispatch preserves the lenient
// behaviour for orphaned DPGs (e.g. backends.json disabled the vendor since
// the schema was authored). The schema still lands in the registry's slice;
// only the adapter dispatch is skipped. Tested because the lookup uses a
// map[string]Adapter — a missing key would otherwise nil-deref.
func TestSaveCustomSchemaUnknownDPGSilentlySkipsDispatch(t *testing.T) {
	reg := New()
	reg.issuers["A"] = newDispatchSpy()

	err := reg.SaveCustomSchema(context.Background(), vctypes.Schema{
		ID: "z", Name: "Z", Std: "w3c_vcdm_2", DPGs: []string{"DisabledVendor"}, Custom: true,
	})
	if err != nil {
		t.Errorf("save with orphan DPG should not error, got %v", err)
	}
	// Schema must still be in the registry's in-memory slice.
	got, _ := reg.ListAllSchemas(context.Background())
	found := false
	for _, s := range got {
		if s.ID == "z" {
			found = true
			break
		}
	}
	if !found {
		t.Error("schema with orphan DPG should still land in registry")
	}
}

// TestSaveCustomSchemaConcurrentRace runs many concurrent saves to confirm
// the registry's internal state stays consistent. Important because the
// new dispatch path takes r.mu.Unlock before calling the adapter — verify
// that doesn't race with another goroutine mutating customSchemas.
func TestSaveCustomSchemaConcurrentRace(t *testing.T) {
	reg := New()
	spy := newDispatchSpy()
	reg.issuers["A"] = spy

	var wg sync.WaitGroup
	const n = 50
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = reg.SaveCustomSchema(context.Background(), vctypes.Schema{
				ID:     "c-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)),
				Name:   "Race",
				Std:    "w3c_vcdm_2",
				DPGs:   []string{"A"},
				Custom: true,
			})
		}(i)
	}
	wg.Wait()
	all, _ := reg.ListAllSchemas(context.Background())
	if len(all) == 0 {
		t.Errorf("expected at least one schema after concurrent saves")
	}
	if spy.savedCount() != n {
		t.Errorf("adapter should have seen %d saves, got %d", n, spy.savedCount())
	}
}

// Compile-time check: dispatchSpy still satisfies backend.Adapter so it can
// drop into reg.issuers without surprises.
var _ backend.Adapter = (*dispatchSpy)(nil)
