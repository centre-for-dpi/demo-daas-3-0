package middleware

import (
	"context"
	"net/http"

	"vcplatform/internal/model"
)

const userKey ctxKey = 2

// AuthRequired checks for a valid session cookie.
// Redirects to /login if no session is found.
func AuthRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// In exploration mode, the session cookie value is "role:name"
		// Parse it to build a user for the context.
		user := model.UserFromSession(cookie.Value)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole checks that the authenticated user has one of the allowed roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUser(r.Context())
			if user == nil || !allowed[user.Role] {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}

// GetUser retrieves the authenticated user from context.
func GetUser(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}
