package handler

import (
	"fmt"
	"net/http"
	"sync"

	"vcplatform/internal/auth"
	"vcplatform/internal/model"
)

type ssoState struct {
	Provider string
	Role     string
}

var (
	stateStore   = map[string]ssoState{}
	stateStoreMu sync.Mutex
)

// OIDCRedirect redirects the user to the selected SSO provider.
// GET /auth/redirect?provider=keycloak&role=issuer
func (h *Handler) OIDCRedirect(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "provider parameter required", http.StatusBadRequest)
		return
	}

	role := r.URL.Query().Get("role")
	if role == "" {
		role = "holder"
	}

	if h.ssoRegistry == nil {
		http.Error(w, "SSO not configured", http.StatusServiceUnavailable)
		return
	}

	provider := h.ssoRegistry.Get(providerName)
	if provider == nil {
		http.Error(w, "unknown provider: "+providerName, http.StatusBadRequest)
		return
	}

	if err := provider.Discover(r.Context()); err != nil {
		http.Error(w, "SSO discovery failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	state := auth.GenerateState()
	stateStoreMu.Lock()
	stateStore[state] = ssoState{Provider: providerName, Role: role}
	stateStoreMu.Unlock()

	authURL, err := provider.AuthorizeURL(state)
	if err != nil {
		http.Error(w, "failed to build auth URL: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// OIDCCallback handles the callback from the SSO provider.
// GET /auth/callback?code=XXX&state=YYY
func (h *Handler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		if errMsg == "" {
			errMsg = "missing code or state"
		}
		http.Error(w, "SSO callback error: "+errMsg, http.StatusBadRequest)
		return
	}

	stateStoreMu.Lock()
	saved, ok := stateStore[state]
	delete(stateStore, state)
	stateStoreMu.Unlock()

	if !ok {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	provider := h.ssoRegistry.Get(saved.Provider)
	if provider == nil {
		http.Error(w, "unknown provider", http.StatusInternalServerError)
		return
	}

	tokenResp, err := provider.ExchangeCode(r.Context(), code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	userInfo, err := provider.GetUserInfo(r.Context(), tokenResp.AccessToken)
	if err != nil {
		http.Error(w, "userinfo failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	name := userInfo.Name
	if name == "" {
		name = userInfo.PreferredUsername
	}
	if name == "" {
		name = userInfo.Email
	}

	role := saved.Role

	// Link SSO identity to backend wallet.
	// Use a deterministic password based on email so it's consistent across all SSO providers.
	walletToken := ""
	if userInfo.Email != "" {
		walletPass := "vcwallet-" + userInfo.Email
		session, err := h.stores.Auth.Login(r.Context(), userInfo.Email, walletPass)
		if err != nil {
			fmt.Printf("sso: wallet login failed for %s, attempting register: %v\n", userInfo.Email, err)
			regErr := h.stores.Auth.Register(r.Context(), name, userInfo.Email, walletPass)
			if regErr != nil {
				fmt.Printf("sso: wallet register failed for %s: %v\n", userInfo.Email, regErr)
				// User exists with old password — try legacy password formats
				for _, legacyPass := range []string{"sso-" + userInfo.Sub, userInfo.Email} {
					session, err = h.stores.Auth.Login(r.Context(), userInfo.Email, legacyPass)
					if err == nil {
						fmt.Printf("sso: wallet login succeeded with legacy password for %s\n", userInfo.Email)
						break
					}
				}
			} else {
				fmt.Printf("sso: wallet registered %s\n", userInfo.Email)
				session, err = h.stores.Auth.Login(r.Context(), userInfo.Email, walletPass)
			}
		}
		if err == nil && session != nil {
			walletToken = session.Token
			fmt.Printf("sso: wallet token acquired for %s (len=%d)\n", userInfo.Email, len(walletToken))
		} else if err != nil {
			fmt.Printf("sso: wallet token FAILED for %s: %v\n", userInfo.Email, err)
		}
	}

	cookieVal := model.EncodeSession(role, name, false, walletToken)
	h.setSessionCookie(w, cookieVal)
	fmt.Printf("sso: session created — role=%s name=%s demo=false hasToken=%v\n", role, name, walletToken != "")
	http.Redirect(w, r, "/portal", http.StatusSeeOther)
}
