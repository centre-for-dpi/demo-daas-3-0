package main

import (
	"log"
	"os"

	"github.com/verifiably/verifiably-go/backend"
	"github.com/verifiably/verifiably-go/internal/adapters/factory"
	"github.com/verifiably/verifiably-go/internal/adapters/registry"
	"github.com/verifiably/verifiably-go/internal/mock"
)

// selectAdapter returns the backend.Adapter to use, driven by
// VERIFIABLY_ADAPTER (default: "mock"). Values:
//   - "mock"     — in-memory demo adapter from internal/mock.
//   - "registry" — live adapter that reads backends.json and fans out per DPG.
func selectAdapter() backend.Adapter {
	mode := os.Getenv("VERIFIABLY_ADAPTER")
	if mode == "" {
		mode = "mock"
	}

	switch mode {
	case "mock":
		log.Printf("adapter: using in-memory mock")
		return mock.NewAdapter()
	case "registry":
		path := os.Getenv("VERIFIABLY_BACKENDS_FILE")
		if path == "" {
			path = "config/backends.json"
		}
		cfg, err := registry.LoadConfig(path)
		if err != nil {
			log.Fatalf("adapter: load backends config: %v", err)
		}
		reg := registry.New()
		for _, b := range cfg.Backends {
			ad, err := factory.Build(b)
			if err != nil {
				log.Fatalf("adapter: build %q: %v", b.Vendor, err)
			}
			if ad == nil {
				log.Printf("adapter: skipping %q (type=%q not yet implemented)", b.Vendor, b.Type)
				continue
			}
			reg.Register(b.Vendor, b.DPG, b.Roles, ad)
			log.Printf("adapter: registered %q (type=%s, roles=%v)", b.Vendor, b.Type, b.Roles)
		}
		log.Printf("adapter: registry ready with %d backend(s) from %s", len(cfg.Backends), path)
		return reg
	default:
		log.Fatalf("adapter: unknown VERIFIABLY_ADAPTER=%q (want mock|registry)", mode)
		return nil
	}
}
