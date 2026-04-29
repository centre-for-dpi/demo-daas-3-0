package waltid

import (
	"strings"
	"testing"

	"github.com/verifiably/verifiably-go/vctypes"
)

// TestSummariseHeldForDiagnostic_SDJWT exercises the diagnostic suffix the
// "no match" path now appends — the operator sees exactly what their
// wallet holds vs what the verifier asked for, so they can tell whether
// they're staring at a stale credential, a vct mismatch, or genuinely
// no credential of the right shape.
func TestSummariseHeldForDiagnostic_SDJWT(t *testing.T) {
	held := []vctypes.Credential{
		{Title: "Farmer Credential", Format: "vc+sd-jwt", Fields: map[string]string{"vct": "FarmerCredential"}},
		{Title: "Bank Id", Format: "jwt_vc_json"},
	}
	got := summariseHeldForDiagnostic(held, "vc+sd-jwt")
	if !strings.Contains(got, `vct="FarmerCredential"`) {
		t.Errorf("expected vct surfaced verbatim, got %q", got)
	}
	if !strings.Contains(got, `"Bank Id"`) {
		t.Errorf("expected non-SD-JWT credentials surfaced too, got %q", got)
	}
	if !strings.Contains(got, `re-issue to get the canonical type name`) {
		t.Errorf("expected stale-credential hint for SD-JWT requests, got %q", got)
	}
}

// TestSummariseHeldForDiagnostic_StaleCustomVCT covers the user-reported
// scenario from 2026-04-29: the wallet holds a credential issued before
// the type/vct alignment fix, so its vct is "custom-..." and the
// verifier (now asking for "FarmerCredential") can't match. The
// diagnostic should make that situation obvious.
func TestSummariseHeldForDiagnostic_StaleCustomVCT(t *testing.T) {
	held := []vctypes.Credential{
		{Title: "Farmer Credential", Format: "vc+sd-jwt", Fields: map[string]string{"vct": "custom-di5upc0ecas3"}},
	}
	got := summariseHeldForDiagnostic(held, "vc+sd-jwt")
	if !strings.Contains(got, `custom-di5upc0ecas3`) {
		t.Errorf("expected stale vct in output, got %q", got)
	}
	if !strings.Contains(got, "re-issue") {
		t.Errorf("expected re-issue hint, got %q", got)
	}
}

// TestSummariseHeldForDiagnostic_EmptyWalletReturnsEmpty leaves the
// pre-existing "your wallet has no credential ..." message to handle the
// genuinely-empty case without piling on diagnostic noise.
func TestSummariseHeldForDiagnostic_EmptyWalletReturnsEmpty(t *testing.T) {
	if got := summariseHeldForDiagnostic(nil, "vc+sd-jwt"); got != "" {
		t.Errorf("expected empty diagnostic when wallet is empty, got %q", got)
	}
}

// TestSummariseHeldForDiagnostic_MDocSurfacesDoctype verifies the mdoc
// branch picks the doctype claim instead of vct. mdoc credentials don't
// carry a vct.
func TestSummariseHeldForDiagnostic_MDocSurfacesDoctype(t *testing.T) {
	held := []vctypes.Credential{
		{Title: "Drivers License", Format: "mso_mdoc", Fields: map[string]string{"doctype": "org.iso.18013.5.1.mDL"}},
	}
	got := summariseHeldForDiagnostic(held, "mso_mdoc")
	if !strings.Contains(got, `doctype="org.iso.18013.5.1.mDL"`) {
		t.Errorf("expected doctype surfaced, got %q", got)
	}
	// SD-JWT-specific stale-vct hint should NOT appear for mdoc.
	if strings.Contains(got, "re-issue") {
		t.Errorf("mdoc diagnostic should not include SD-JWT vct hint, got %q", got)
	}
}
