package middleware

import "net/http"

// Chain composes middleware functions in the order given.
// The first middleware wraps the outermost layer.
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(final http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return final
	}
}
