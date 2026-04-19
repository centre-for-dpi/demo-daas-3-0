package registry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/mock"
	"github.com/verifiably/verifiably-go/vctypes"
)

// TestMultiHolderRouting guards against the "unknown DPG: holder not selected"
// regression that surfaced once scenario=all registered both walt.id and
// Inji Web Wallet as holders. currentHolder() must route via the DPG attached
// to ctx; when none is attached and multiple are registered, it errors rather
// than silently picking. We test the resolver directly because stubbing the
// full backend.Adapter interface adds no signal beyond the routing check.
func TestMultiHolderRouting(t *testing.T) {
	reg := New()
	waltAd := mock.NewAdapter()
	injiAd := mock.NewAdapter()
	reg.holders["Walt Community Stack"] = waltAd
	reg.holders["Inji Web Wallet"] = injiAd
	reg.holderDPGs["Walt Community Stack"] = vctypes.DPG{}
	reg.holderDPGs["Inji Web Wallet"] = vctypes.DPG{}

	// Ambiguous: two holders, no ctx hint → ErrUnknownDPG.
	if _, err := reg.currentHolder(context.Background()); !errors.Is(err, backend.ErrUnknownDPG) ||
		!strings.Contains(err.Error(), "holder not selected") {
		t.Fatalf("ambiguous case: want ErrUnknownDPG / holder not selected, got %v", err)
	}

	// Ctx names an unregistered holder → different error path.
	ctx := backend.WithHolderDpg(context.Background(), "Nope")
	if _, err := reg.currentHolder(ctx); !errors.Is(err, backend.ErrUnknownDPG) ||
		!strings.Contains(err.Error(), `holder "Nope"`) {
		t.Fatalf("unknown holder: want ErrUnknownDPG mentioning Nope, got %v", err)
	}

	// Ctx picks walt.id → walt adapter (pointer identity).
	ctx = backend.WithHolderDpg(context.Background(), "Walt Community Stack")
	got, err := reg.currentHolder(ctx)
	if err != nil || got != waltAd {
		t.Fatalf("walt routing: got=%v err=%v", got, err)
	}

	// Ctx picks inji-web → inji adapter.
	ctx = backend.WithHolderDpg(context.Background(), "Inji Web Wallet")
	if got, err := reg.currentHolder(ctx); err != nil || got != injiAd {
		t.Fatalf("inji routing: got=%v err=%v", got, err)
	}

	// Single-holder shortcut still works (scenario=waltid or =inji alone).
	single := New()
	single.holders["Walt Community Stack"] = waltAd
	if _, err := single.currentHolder(context.Background()); err != nil {
		t.Fatalf("single-holder shortcut broke: %v", err)
	}
}
