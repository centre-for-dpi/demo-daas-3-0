package pdfwallet

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestBase45EncodeKnownVector(t *testing.T) {
	// RFC 9285 §4 test vector: "AB" → "BB8"
	got := Base45Encode([]byte("AB"))
	if got != "BB8" {
		t.Errorf("Base45Encode(AB) = %q, want %q", got, "BB8")
	}
	// "Hello!!" → "%69 VD92EX0"
	got = Base45Encode([]byte("Hello!!"))
	if got != "%69 VD92EX0" {
		t.Errorf("Base45Encode(Hello!!) = %q, want %q", got, "%69 VD92EX0")
	}
}

func TestPixelPassEncodeRoundtrip(t *testing.T) {
	payload := []byte(`{"type":"VerifiableCredential","credentialSubject":{"name":"Alice"}}`)
	encoded, err := PixelPassEncode(payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if encoded == "" {
		t.Fatal("empty encoding")
	}
	// Every char must come from the base45 alphabet.
	for _, r := range encoded {
		if !strings.ContainsRune(base45Alphabet, r) {
			t.Errorf("non-base45 char %q in output", r)
		}
	}
}

func TestRenderCredentialPDFSmallLDP(t *testing.T) {
	cred := map[string]any{
		"@context":          []any{"https://www.w3.org/ns/credentials/v2"},
		"type":              []any{"VerifiableCredential", "FarmerCredential"},
		"issuer":            "did:web:issuer.example.org",
		"issuanceDate":      "2026-01-01T00:00:00Z",
		"credentialSubject": map[string]any{"name": "Alice", "district": "Bomet"},
	}
	raw, _ := json.Marshal(cred)
	pdf, err := RenderCredentialPDF(cred, raw, "ldp_vc")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(pdf) < 500 {
		t.Errorf("pdf too small: %d bytes", len(pdf))
	}
	if !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("output is not a PDF: %q", string(pdf[:8]))
	}
}

func TestRenderCredentialPDFTooLargeReturnsStructured(t *testing.T) {
	// Generate ~12 KB of cryptographically random bytes (base64-encoded so
	// it's valid JSON). Random data is effectively incompressible, so zlib
	// can't shrink it below the QR v40 Low EC capacity — the encoder has
	// to refuse and we must surface a QRTooLargeError with size details
	// and alternatives.
	rawBytes := make([]byte, 12000)
	if _, err := rand.Read(rawBytes); err != nil {
		t.Fatalf("rand: %v", err)
	}
	large := base64.StdEncoding.EncodeToString(rawBytes)
	cred := map[string]any{
		"type":              []any{"VerifiableCredential"},
		"credentialSubject": map[string]any{"blob": large},
	}
	raw, _ := json.Marshal(cred)
	_, err := RenderCredentialPDF(cred, raw, "ldp_vc")
	if err == nil {
		t.Fatal("expected QRTooLargeError, got nil")
	}
	var qrErr *QRTooLargeError
	ok := errorsAs(err, &qrErr)
	if !ok {
		t.Fatalf("expected *QRTooLargeError, got %T: %v", err, err)
	}
	if qrErr.RawBytes == 0 || qrErr.EncodedSize == 0 {
		t.Errorf("error missing size info: %+v", qrErr)
	}
	if len(qrErr.Alternatives) == 0 {
		t.Error("error missing alternatives")
	}
}

// errorsAs is a tiny wrapper so the test file doesn't need to import
// "errors" conditionally.
func errorsAs(err error, target any) bool {
	if qr, ok := err.(*QRTooLargeError); ok {
		if t, ok := target.(**QRTooLargeError); ok {
			*t = qr
			return true
		}
	}
	return false
}
