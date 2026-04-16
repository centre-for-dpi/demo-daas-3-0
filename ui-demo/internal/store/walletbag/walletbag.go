// Package walletbag is a shared credential bag keyed by the user's wallet
// token. The local holder and any other in-process wallet store credentials
// here so they appear in "My Credentials".
//
// Persistence: the bag writes to a JSON file on every Add and loads it on
// startup. Credentials survive server restarts.
package walletbag

import (
	"encoding/json"
	"log"
	"os"
	"sync"

	"vcplatform/internal/model"
)

// Bag is a thread-safe map of walletToken → credentials, backed by a file.
type Bag struct {
	mu   sync.RWMutex
	items map[string][]model.WalletCredential
	path  string // empty = no persistence
}

// Shared is the process-wide default bag. Call InitShared to set the file path.
var Shared = &Bag{items: map[string][]model.WalletCredential{}}

// InitShared sets the persistence file and loads any existing data.
func InitShared(path string) {
	Shared.mu.Lock()
	defer Shared.mu.Unlock()
	Shared.path = path
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("walletbag: load %s: %v", path, err)
		}
		return
	}
	var items map[string][]model.WalletCredential
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("walletbag: parse %s: %v", path, err)
		return
	}
	Shared.items = items
	total := 0
	for _, creds := range items {
		total += len(creds)
	}
	log.Printf("walletbag: loaded %d credential(s) across %d token(s) from %s", total, len(items), path)
}

// Add appends a credential and persists.
func (b *Bag) Add(token string, cred model.WalletCredential) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[token] = append(b.items[token], cred)
	b.persistLocked()
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

// persistLocked writes the bag to disk. Caller must hold b.mu.
func (b *Bag) persistLocked() {
	if b.path == "" {
		return
	}
	data, err := json.Marshal(b.items)
	if err != nil {
		log.Printf("walletbag: marshal: %v", err)
		return
	}
	if err := os.WriteFile(b.path, data, 0644); err != nil {
		log.Printf("walletbag: write %s: %v", b.path, err)
	}
}
