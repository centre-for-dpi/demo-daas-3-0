package pdfwallet

// pixelpass.go — PixelPass encoding: zlib-compress the credential, then
// base45 per RFC 9285. The result fits QR alphanumeric mode, which is the
// densest QR mode that doesn't require binary safety.

import (
	"bytes"
	"compress/zlib"
	"fmt"
)

// PixelPassEncode compresses cred with zlib (default compression level)
// and encodes the result as base45.
func PixelPassEncode(cred []byte) (string, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(cred); err != nil {
		return "", fmt.Errorf("zlib write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("zlib close: %w", err)
	}
	return Base45Encode(buf.Bytes()), nil
}
