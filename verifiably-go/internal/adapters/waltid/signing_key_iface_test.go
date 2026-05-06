package waltid

import (
	"context"
	"testing"
)

// TestIssuerSigningKeyMatchesHandlerInterface pins the method signature
// that handlers/status_list.go's signingKeyAdapter expects. Go's interface
// satisfaction is invariant on named types — a method returning
// json.RawMessage doesn't satisfy an interface declaring []byte, even
// though both share an underlying type. The previous version of this
// method returned json.RawMessage, which silently failed the type
// assertion in registry.IssuerSigningKey + handlers.resolveSigningKey
// and surfaced as a 503 from /status-list/* (handlers cached the "no
// adapter exposes IssuerSigningKey" error and walt.id's verifier saw
// "URL returned unexpected status: 503 Service Unavailable" when
// dereferencing the published list).
//
// If anyone changes the return type back to json.RawMessage this test
// fails at compile time, before the runtime regression has a chance to
// reach a verifier.
func TestIssuerSigningKeyMatchesHandlerInterface(t *testing.T) {
	type signingKeyAdapter interface {
		IssuerSigningKey(ctx context.Context) (raw []byte, did string, err error)
	}
	var _ signingKeyAdapter = (*Adapter)(nil)
}
