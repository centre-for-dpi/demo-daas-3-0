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
	Demo        bool   `json:"demo"`  // true = mock/demo user, false = real authenticated user
	WalletToken string `json:"-"`
	WalletID    string `json:"-"`
}

type sessionData struct {
	Role        string `json:"r"`
	Name        string `json:"n"`
	Demo        bool   `json:"d,omitempty"`
	WalletToken string `json:"t,omitempty"`
}

func UserFromSession(session string) *User {
	if data, err := base64.RawURLEncoding.DecodeString(session); err == nil {
		var sd sessionData
		if json.Unmarshal(data, &sd) == nil && sd.Role != "" && sd.Name != "" {
			u := newUser(sd.Role, sd.Name, sd.WalletToken)
			u.Demo = sd.Demo
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

func EncodeSession(role, name string, demo bool, walletToken string) string {
	sd := sessionData{Role: role, Name: name, Demo: demo, WalletToken: walletToken}
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
