package middleware

import (
	"context"
	"net/http"

	"vcplatform/internal/config"
)

type ctxKey int

const configKey ctxKey = iota

// ConfigInjector adds the application config to every request context.
func ConfigInjector(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), configKey, cfg)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetConfig retrieves config from the request context.
func GetConfig(ctx context.Context) *config.Config {
	cfg, _ := ctx.Value(configKey).(*config.Config)
	return cfg
}
