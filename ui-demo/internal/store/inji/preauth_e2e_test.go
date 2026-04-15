package inji_test

// preauth_e2e_test.go — end-to-end integration test for the OID4VCI
// Pre-Authorized Code flow against a running inji-certify-preauth
// instance. Skipped by default; run by setting INJI_PREAUTH_E2E=1.
//
// The test exercises the full chain that the PDF wallet and in-process
// Local holder rely on when a holder pastes an Inji-issued offer URL:
//
//   1. POST /v1/certify/pre-authorized-data with claims + tx_code →
//      certify caches the staged claims on the resulting session
//   2. fetchCredentialOffer(credential_offer_uri) → returns the
//      pre-authorized_code and a tx_code descriptor
//   3. POST /v1/certify/oauth/token (form: pre-authorized_code +
//      tx_code) → access token + c_nonce
//   4. localholder.ClaimOID4VCICredential builds a did:jwk + ECDSA
//      P-256 proof JWT, POSTs the credential request, unwraps the
//      result, and returns the raw VC JSON
//
// Two preconditions for this test to pass against the second instance
// (inji-certify-preauth on host port 8094 by default):
//
//   * The instance must have its credential endpoint validating
//     against its OWN JWKS (no esignet rewire). If the JWKS trust
//     list points elsewhere, step 4 fails with 401
//     InsufficientAuthenticationException because the token was
//     signed by Inji's own keymanager.
//
//   * The instance must use mosip.certify.integration.data-provider-plugin
//     = PreAuthDataProviderPlugin (NOT MockCSVDataProviderPlugin).
//     The Pre-Auth plugin reads cached claims from VCICacheService
//     keyed by the access token hash; the Mock CSV plugin tries to
//     look up an `individualId` in a CSV file and 400s with
//     ERROR_FETCHING_IDENTITY_DATA. The bean is defined inside
//     certify-service.jar at io.mosip.certify.services.PreAuthIssuanceServiceImpl
//     activated via @ConditionalOnProperty(havingValue="PreAuthDataProviderPlugin").
//
// To run:
//
//	cd ui-demo
//	INJI_PREAUTH_E2E=1 go test ./internal/store/inji/ -run TestPreAuthE2E -v
//
// Override the target URL with INJI_PREAUTH_URL (default http://localhost:8094).
//
// This is the test that proves the whole second-instance scaffold works.

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"vcplatform/internal/store/localholder"
)

func TestPreAuthE2E(t *testing.T) {
	if os.Getenv("INJI_PREAUTH_E2E") == "" {
		t.Skip("set INJI_PREAUTH_E2E=1 to run the live Inji Certify Pre-Auth integration test")
	}

	base := os.Getenv("INJI_PREAUTH_URL")
	if base == "" {
		base = "http://localhost:8094"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	body := strings.NewReader(`{"credential_configuration_id":"FarmerCredential","expires_in":600,"tx_code":"12345","claims":{"fullName":"Pre Auth E2E","mobileNumber":"7550166914","dateOfBirth":"24-01-1998","gender":"Female","state":"Karnataka","district":"Bangalore","villageOrTown":"Koramangala","postalCode":"560068","landArea":"5 acres","landOwnershipType":"Self-owned","primaryCropType":"Cotton","secondaryCropType":"Barley","farmerID":"4567538771"}}`)
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/v1/certify/pre-authorized-data", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pre-auth-data POST: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	line := string(buf[:n])
	i := strings.Index(line, `"credential_offer_uri":"`)
	if i < 0 {
		t.Fatalf("unexpected pre-auth response: %s", line)
	}
	start := i + len(`"credential_offer_uri":"`)
	end := strings.Index(line[start:], `"`)
	offer := line[start : start+end]

	credJSON, err := localholder.ClaimOID4VCICredential(ctx, offer)
	if err != nil {
		t.Fatalf("ClaimOID4VCICredential: %v", err)
	}
	if len(credJSON) < 200 {
		t.Errorf("credential too small (%d bytes), expected at least 200", len(credJSON))
	}
	if !strings.Contains(string(credJSON), "credentialSubject") {
		t.Errorf("credential missing credentialSubject:\n%s", string(credJSON))
	}
	if !strings.Contains(string(credJSON), "Pre Auth E2E") {
		t.Errorf("credential missing the staged fullName claim — the data-provider plugin may not be PreAuthDataProviderPlugin:\n%s", string(credJSON))
	}
}
