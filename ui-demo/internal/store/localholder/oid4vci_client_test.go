package localholder

import (
	"strings"
	"testing"
)

// TestUnwrapCredentialResponse_JWTString locks in the fix for the PDF-wallet
// "invalid credential" bug: when an OID4VCI issuer returns a JWT VC, the
// credential field is a STRING, and unwrapCredentialResponse must return the
// raw string bytes (no JSON quotes). A regression would re-json-marshal the
// string and add surrounding `"..."` which then breaks format detection and
// signature verification at any downstream verifier that scans the QR.
func TestUnwrapCredentialResponse_JWTString(t *testing.T) {
	body := []byte(`{"credential":"eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ4In0.sig","format":"jwt_vc_json","c_nonce":"abc"}`)
	got := unwrapCredentialResponse(body)
	want := "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ4In0.sig"
	if string(got) != want {
		t.Errorf("unwrap returned %q, want %q", string(got), want)
	}
	if strings.HasPrefix(string(got), `"`) {
		t.Error("unwrap wrapped the JWT in quotes — this is the regression we're guarding against")
	}
}

// TestUnwrapCredentialResponse_LDPObject confirms the object path still
// works: a JSON-LD VC inside the credential field is re-marshaled and
// returned as a clean JSON object, not the whole wrapper.
func TestUnwrapCredentialResponse_LDPObject(t *testing.T) {
	body := []byte(`{"credential":{"@context":["https://www.w3.org/ns/credentials/v2"],"type":["VerifiableCredential"],"issuer":"did:web:example.org"},"format":"ldp_vc","c_nonce":"abc"}`)
	got := unwrapCredentialResponse(body)
	gotStr := string(got)
	if !strings.HasPrefix(gotStr, `{`) {
		t.Errorf("unwrap did not return a JSON object: %q", gotStr)
	}
	if !strings.Contains(gotStr, `"issuer":"did:web:example.org"`) {
		t.Errorf("unwrap lost the issuer field: %q", gotStr)
	}
	if strings.Contains(gotStr, `"c_nonce"`) {
		t.Errorf("unwrap leaked the wrapper: %q", gotStr)
	}
}

// TestUnwrapCredentialResponse_Passthrough confirms non-wrapped bodies
// flow through unchanged. Some issuers might return the VC directly.
func TestUnwrapCredentialResponse_Passthrough(t *testing.T) {
	body := []byte(`{"@context":["https://www.w3.org/ns/credentials/v2"],"type":["VerifiableCredential"]}`)
	got := unwrapCredentialResponse(body)
	if string(got) != string(body) {
		t.Errorf("passthrough changed body:\n got: %q\nwant: %q", string(got), string(body))
	}
}
