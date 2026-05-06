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

// holderIdentityCtxKey carries a stable per-user key (derived from the OIDC
// subject/email) so adapters that back each verifiably-go user with an
// isolated upstream wallet account can look up or provision the right one.
// Without this the walt.id adapter cached a process-wide session and
// handed EVERY authenticated user the same walt.id wallet.
type holderIdentityCtxKey struct{}

// WithHolderIdentity attaches a stable per-user key to ctx. The handlers
// pass r.Context() through this before any adapter wallet call so the
// adapter can partition wallets per caller. Empty key is a no-op — the
// adapter falls back to the configured demo account (single-user mode).
func WithHolderIdentity(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, holderIdentityCtxKey{}, key)
}

// HolderIdentityFromContext reads the per-user key back. Adapters use this
// as the partition key for their wallet-session cache.
func HolderIdentityFromContext(ctx context.Context) string {
	v, _ := ctx.Value(holderIdentityCtxKey{}).(string)
	return v
}

// issuerIdentityCtxKey carries the same kind of stable per-user key as
// holderIdentityCtxKey, but for the issuer side — used to scope the
// custom-schema list and (downstream) the issued-credentials log so
// issuer A's session never sees issuer B's catalog or audit log.
//
// Held separately from the holder key so a user playing both roles in
// the demo can't accidentally have their issuer scope leak into the
// holder wallet-account cache (or vice versa) — the values would be
// the same string today, but coupling them risks a future divergence.
type issuerIdentityCtxKey struct{}

// WithIssuerIdentity attaches the issuer-side per-user key to ctx.
// Mirrors WithHolderIdentity. Empty key is a no-op (admin/CLI paths
// that operate on the global catalog without scoping).
func WithIssuerIdentity(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, issuerIdentityCtxKey{}, key)
}

// IssuerIdentityFromContext reads the issuer key back. Registry uses
// this to filter customSchemas so each issuer's schema browser shows
// only the schemas they themselves saved.
func IssuerIdentityFromContext(ctx context.Context) string {
	v, _ := ctx.Value(issuerIdentityCtxKey{}).(string)
	return v
}
