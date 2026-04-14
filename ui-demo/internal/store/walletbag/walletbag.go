// Package walletbag is a shared, in-memory credential bag keyed by the
// current user's wallet token. Both the in-process Inji holder wallet and
// the Walt.id holder wallet read from it, so credentials minted in-process
// (e.g. our LDP_VC signer) surface in the same list as credentials claimed
// via external wallet APIs.
//
// The bag is a simple singleton — there is exactly one per server process.
// Entries are lost on restart. For v1 demo purposes this is fine; a
// production deployment would persist them.
package walletbag

import (
	"sync"

	"vcplatform/internal/model"
)

// Bag is a thread-safe map of walletToken → credentials.
type Bag struct {
	mu    sync.RWMutex
	items map[string][]model.WalletCredential
}

// Shared is the process-wide default bag.
var Shared = &Bag{items: map[string][]model.WalletCredential{}}

// Add appends a credential to the slot for the given token.
func (b *Bag) Add(token string, cred model.WalletCredential) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[token] = append(b.items[token], cred)
}

// List returns a copy of the credentials stored for the given token.
func (b *Bag) List(token string) []model.WalletCredential {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]model.WalletCredential, len(b.items[token]))
	copy(out, b.items[token])
	return out
}

// Find returns the credential matching the given id in the token's slot,
// or nil if not found.
func (b *Bag) Find(token, id string) *model.WalletCredential {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, c := range b.items[token] {
		if c.ID == id {
			out := c
			return &out
		}
	}
	return nil
}
