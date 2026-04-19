package backend

import "context"

// holderDpgCtxKey is the context key under which handlers attach the selected
// holder vendor so a multi-holder adapter (the Registry in particular) can
// route holder-scoped calls that don't carry a DPG argument in their
// signature — ParseOffer, ClaimCredential, ListWalletCredentials.
type holderDpgCtxKey struct{}

// WithHolderDpg returns a derived context that carries the selected holder
// vendor (e.g. "Walt Community Stack"). Handlers wrap r.Context() with this
// before calling adapter methods that lack a DPG field in their signature.
func WithHolderDpg(ctx context.Context, vendor string) context.Context {
	if vendor == "" {
		return ctx
	}
	return context.WithValue(ctx, holderDpgCtxKey{}, vendor)
}

// HolderDpgFromContext reads the selected holder vendor back, returning ""
// when no vendor has been attached.
func HolderDpgFromContext(ctx context.Context) string {
	v, _ := ctx.Value(holderDpgCtxKey{}).(string)
	return v
}
