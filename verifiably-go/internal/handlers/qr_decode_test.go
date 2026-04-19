package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	qr "github.com/skip2/go-qrcode"
)

// Exercises the real QR encode/decode path used by the upload verifier flow.
// Generates a PNG QR with a known payload in-memory, posts it to a recording
// request, runs decodeUploadedQR, and asserts the payload round-trips.
func TestDecodeUploadedQR_RoundTrip(t *testing.T) {
	payload := "openid-credential-offer://?credential_offer_uri=http://example.com/offer/abc"
	png, err := qr.Encode(payload, qr.Medium, 256)
	if err != nil {
		t.Fatalf("encode qr: %v", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("credential_image", "qr.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(png); err != nil {
		t.Fatalf("write png: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequest("POST", "/verifier/verify/direct", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}

	decoded, err := decodeUploadedQR(req)
	if err != nil {
		t.Fatalf("decodeUploadedQR: %v", err)
	}
	if decoded != payload {
		t.Fatalf("payload mismatch:\n want: %s\n got:  %s", payload, decoded)
	}
}

// Uploading a file that isn't a decodable image should return an error
// rather than crash or return an empty payload.
func TestDecodeUploadedQR_NotAnImage(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("credential_image", "garbage.png")
	fw.Write([]byte("not actually an image"))
	_ = mw.Close()

	req := httptest.NewRequest("POST", "/verifier/verify/direct", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	_ = req.ParseMultipartForm(1 << 20)

	_, err := decodeUploadedQR(req)
	if err == nil {
		t.Fatal("expected error for garbage upload, got nil")
	}
}
