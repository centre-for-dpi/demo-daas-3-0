package model

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Initials    string `json:"initials"`
	Demo        bool   `json:"demo"` // true = mock/demo user, false = real authenticated user
	WalletToken string `json:"-"`
	WalletID    string `json:"-"`

	// Per-user DPG preferences, chosen during onboarding. Empty = fall back
	// to the server default. Handler helpers (issuerFor/walletFor/verifierFor)
	// resolve the right store at request time based on these fields.
	IssuerDPG   string `json:"issuerDpg,omitempty"`
	WalletDPG   string `json:"walletDpg,omitempty"`
	VerifierDPG string `json:"verifierDpg,omitempty"`
	// OnboardingStep is the current step the user is on in the onboarding
	// wizard for whichever role they signed up with. "" / "done" means the
	// user has finished onboarding and should go straight to the portal.
	OnboardingStep string `json:"onboardingStep,omitempty"`
}

type sessionData struct {
	Role           string `json:"r"`
	Name           string `json:"n"`
	Email          string `json:"e,omitempty"`
	Demo           bool   `json:"d,omitempty"`
	WalletToken    string `json:"t,omitempty"`
	IssuerDPG      string `json:"i,omitempty"`
	WalletDPG      string `json:"w,omitempty"`
	VerifierDPG    string `json:"v,omitempty"`
	OnboardingStep string `json:"o,omitempty"`
}

func UserFromSession(session string) *User {
	if data, err := base64.RawURLEncoding.DecodeString(session); err == nil {
		var sd sessionData
		if json.Unmarshal(data, &sd) == nil && sd.Role != "" && sd.Name != "" {
			u := newUser(sd.Role, sd.Name, sd.WalletToken)
			u.Demo = sd.Demo
			u.IssuerDPG = sd.IssuerDPG
			u.WalletDPG = sd.WalletDPG
			u.VerifierDPG = sd.VerifierDPG
			u.OnboardingStep = sd.OnboardingStep
			if sd.Email != "" {
				u.Email = sd.Email
			}
			return u
		}
	}
	// Legacy fallback
	parts := strings.SplitN(session, ":", 2)
	if len(parts) == 2 {
		u := newUser(parts[0], parts[1], "")
		u.Demo = true
		return u
	}
	return nil
}

// EncodeSession serializes a minimal session cookie. Kept backwards-compatible
// with the simpler 4-arg form used by existing callers.
func EncodeSession(role, name string, demo bool, walletToken string) string {
	return EncodeSessionFull(role, name, "", demo, walletToken, "", "")
}

// EncodeSessionFull is the full variant that includes the user's chosen
// issuance DPG and onboarding step. Kept for backwards compatibility with
// existing call sites; for new code prefer EncodeSessionFromUser.
func EncodeSessionFull(role, name, email string, demo bool, walletToken, issuerDPG, onboardingStep string) string {
	sd := sessionData{
		Role:           role,
		Name:           name,
		Email:          email,
		Demo:           demo,
		WalletToken:    walletToken,
		IssuerDPG:      issuerDPG,
		OnboardingStep: onboardingStep,
	}
	data, _ := json.Marshal(sd)
	return base64.RawURLEncoding.EncodeToString(data)
}

// EncodeSessionFromUser serializes a session cookie from a *User struct,
// preserving all per-user DPG preferences.
func EncodeSessionFromUser(u *User) string {
	if u == nil {
		return ""
	}
	sd := sessionData{
		Role:           u.Role,
		Name:           u.Name,
		Email:          u.Email,
		Demo:           u.Demo,
		WalletToken:    u.WalletToken,
		IssuerDPG:      u.IssuerDPG,
		WalletDPG:      u.WalletDPG,
		VerifierDPG:    u.VerifierDPG,
		OnboardingStep: u.OnboardingStep,
	}
	data, _ := json.Marshal(sd)
	return base64.RawURLEncoding.EncodeToString(data)
}

func newUser(role, name, walletToken string) *User {
	initials := ""
	for _, w := range strings.Fields(name) {
		if len(w) > 0 {
			initials += strings.ToUpper(w[:1])
		}
	}
	if len(initials) > 2 {
		initials = initials[:2]
	}
	email := strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@example.com"
	return &User{
		ID:          role + "-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		Name:        name,
		Email:       email,
		Role:        role,
		Initials:    initials,
		WalletToken: walletToken,
	}
}

// HasBackendAuth returns true if the user has a real backend token.
func (u *User) HasBackendAuth() bool {
	return u != nil && u.WalletToken != "" && !u.Demo
}

// IsDemo returns true if this is a demo/mock user.
func (u *User) IsDemo() bool {
	return u != nil && u.Demo
}
