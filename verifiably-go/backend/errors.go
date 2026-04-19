package backend

import "errors"

// ErrNotSupported is returned when a DPG adapter is asked to perform an
// operation the underlying backend doesn't support (e.g. PDF issuance on a
// DPG that has no direct-PDF export path).
var ErrNotSupported = errors.New("operation not supported by this DPG")

// ErrNotApplicable is returned when an operation doesn't make sense for a DPG's
// role (e.g. PresentCredential called on a verifier-only DPG).
var ErrNotApplicable = errors.New("operation not applicable to this DPG")

// ErrUnknownDPG is returned by the registry when a request names a DPG that
// isn't configured in backends.json.
var ErrUnknownDPG = errors.New("unknown DPG")

// ErrNotLinked is returned when an operation requires a redirect-wallet
// session that hasn't been established yet — e.g. reading held credentials
// from a redirect-wallet DPG before the user has linked their account.
var ErrNotLinked = errors.New("DPG requires account linking")

// ErrOfferUnresolvable is returned when an offer URI can't be resolved against
// any configured issuer — maps to a user-friendly toast.
var ErrOfferUnresolvable = errors.New("credential offer not resolvable")
