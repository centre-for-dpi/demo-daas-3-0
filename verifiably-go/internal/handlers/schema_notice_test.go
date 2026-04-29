package handlers

import (
	"errors"
	"strings"
	"testing"
)

// TestTransientCatalogNotice_RestartCase locks in the friendly banner used
// during the walt.id restart window that SaveCustomSchema itself triggers.
// Regression target: the previous error path called errorToast → http.Error
// (500) which wrote a plain-text body THEN the template render appended
// HTML, producing the wall-of-text bleed reported on 2026-04-29.
func TestTransientCatalogNotice_RestartCase(t *testing.T) {
	err := errors.New(`fetch issuer metadata: GET http://issuer-api:7002/draft13/.well-known/openid-credential-issuer: dial tcp 172.24.0.15:7002: connect: connection refused`)
	got := transientCatalogNotice(err)
	if !strings.Contains(got, "issuer-api may be restarting") {
		t.Errorf("expected restart hint, got %q", got)
	}
	if strings.Contains(got, "172.24.0.15") {
		t.Errorf("notice should NOT include raw network internals: %q", got)
	}
}

func TestTransientCatalogNotice_PassesThroughOtherErrors(t *testing.T) {
	// Non-transient errors (auth, misconfiguration) must surface verbatim
	// so a real failure mode isn't masked by the friendly banner.
	err := errors.New("fetch issuer metadata: HTTP 401 Unauthorized")
	got := transientCatalogNotice(err)
	if !strings.Contains(got, "401") {
		t.Errorf("expected raw error to surface, got %q", got)
	}
}
