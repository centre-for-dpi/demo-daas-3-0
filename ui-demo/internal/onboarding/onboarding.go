// Package onboarding holds per-user state for the issuer onboarding wizard.
//
// This is intentionally in-memory and not persistent: for v1 we trade
// durability for simplicity. On server restart, users fall back to the
// first step of the wizard. A production deployment would back this with
// a relational store or KV (the Store interface is the seam).
package onboarding

import (
	"sync"
	"time"
)

// State is a single user's onboarding progress.
type State struct {
	UserID string `json:"userId"`
	// Role is the user's role at the time they started onboarding: "issuer",
	// "holder", or "verifier". Drives which wizard steps are shown.
	Role string `json:"role"`
	// CredentialCategories is the list of high-level credential types the
	// issuer expects to handle (e.g. "education", "identity", "health").
	// Only populated for issuers.
	CredentialCategories []string `json:"credentialCategories"`
	// Per-role DPG choices. Only one is populated per onboarding session
	// (matching Role), but the State struct carries all three so a single
	// user can onboard into multiple roles over time.
	IssuerDPG   string `json:"issuerDpg"`
	WalletDPG   string `json:"walletDpg"`
	VerifierDPG string `json:"verifierDpg"`
	// Confirmed is true once the user clicks "Confirm & Continue" on the
	// DPG choice screen.
	Confirmed bool `json:"confirmed"`
	// PublishedSchemaIDs is the list of schema IDs the user has saved to the
	// schema registry via the catalog flow (issuer-only).
	PublishedSchemaIDs []string `json:"publishedSchemaIds"`
	// IssuanceMode is the mode the user picked at the last step
	// ("single" or "bulk"). Issuer-only.
	IssuanceMode string `json:"issuanceMode"`
	// Step is the current wizard step. See Steps constants.
	Step string `json:"step"`
	// UpdatedAt is the last time the state changed.
	UpdatedAt time.Time `json:"updatedAt"`
}

// Wizard step identifiers. The UI routes and handler dispatch on these.
const (
	StepSignup        = "signup"
	StepCategories    = "categories"
	StepDPGChoice     = "dpg-choice"
	StepDPGConfirm    = "dpg-confirm"
	StepSchemaCatalog = "schema-catalog"
	StepSchemaEdit    = "schema-edit"
	StepIssuanceMode  = "issuance-mode"
	StepDone          = "done"
)

// NextStep returns the step that should come after the current one, given
// the state. Linear for now — no branching.
func NextStep(current string) string {
	switch current {
	case StepSignup, "":
		return StepCategories
	case StepCategories:
		return StepDPGChoice
	case StepDPGChoice:
		return StepDPGConfirm
	case StepDPGConfirm:
		return StepSchemaCatalog
	case StepSchemaCatalog, StepSchemaEdit:
		return StepIssuanceMode
	case StepIssuanceMode:
		return StepDone
	default:
		return StepDone
	}
}

// Store is the interface for persisting per-user onboarding state.
type Store interface {
	Get(userID string) *State
	Put(state *State)
	Delete(userID string)
}

// MemoryStore is the v1 in-memory implementation.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]*State
}

// NewMemoryStore returns an empty in-memory onboarding store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]*State{}}
}

func (s *MemoryStore) Get(userID string) *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.items[userID]; ok {
		cp := *v
		return &cp
	}
	return nil
}

func (s *MemoryStore) Put(state *State) {
	if state == nil || state.UserID == "" {
		return
	}
	state.UpdatedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *state
	s.items[state.UserID] = &cp
}

func (s *MemoryStore) Delete(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, userID)
}
