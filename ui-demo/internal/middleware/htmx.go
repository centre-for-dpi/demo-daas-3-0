package middleware

import (
	"context"
	"net/http"
)

const htmxKey ctxKey = 1

// HTMXDetector checks for the HX-Request header and stores the result in context.
func HTMXDetector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isHTMX := r.Header.Get("HX-Request") == "true"
		ctx := context.WithValue(r.Context(), htmxKey, isHTMX)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// IsHTMX returns true if the request was made by HTMX.
func IsHTMX(ctx context.Context) bool {
	v, _ := ctx.Value(htmxKey).(bool)
	return v
}
