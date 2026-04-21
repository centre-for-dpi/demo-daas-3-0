package injicertify

// pixelpass.go — QR payload encoder compatible with MOSIP's PixelPass
// format, the one Inji Verify's QR decoder expects.
//
// Pipeline (mirrors @mosip/pixelpass/src/index.js:generateQRData):
//   1. Parse input as JSON → CBOR-encode the parsed object. If the input
//      isn't JSON, deflate the raw bytes directly.
//   2. zlib-deflate the CBOR (or raw) bytes.
//   3. Base45-encode per RFC 9285.
//   4. Optionally prepend a header prefix (not used here — Inji Verify's
//      decoder doesn't require one for unsigned raw data).
//
// Inji Verify then runs decode: base45 → inflate → CBOR-decode → text.
// When the original was a JSON VC, the CBOR-decode step rehydrates it
// and the verifier sees a proper VC object.
//
// Why CBOR? pixelpass's encoder reaches for the smallest wire format —
// CBOR is significantly tighter than JSON for VCs with many fields. The
// zlib step then compresses the CBOR. Without CBOR the payload tends to
// exceed QR version 40's 2953-byte limit for real credentials.

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// base45Alphabet per RFC 9285 §4: 0..9, A..Z, then ten specific symbols.
const base45Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// encodePixelPass returns the base45-encoded, zlib-deflated,
// CBOR-(if-JSON)-encoded form of the given VC bytes. The result is plain
// ASCII safe to embed directly into a QR payload.
func encodePixelPass(vc []byte) (string, error) {
	if len(vc) == 0 {
		return "", fmt.Errorf("empty input")
	}
	var payload []byte
	// Try JSON → CBOR. PixelPass's JS encoder uses JSON.parse, which
	// tolerates any JSON value (object, array, string). We mirror that.
	var anyVal any
	if err := json.Unmarshal(vc, &anyVal); err == nil {
		cborBytes, cerr := cbor.Marshal(anyVal)
		if cerr == nil {
			payload = cborBytes
		}
	}
	if payload == nil {
		payload = vc
	}

	// zlib-deflate at max level (pixelpass uses level 9).
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return "", fmt.Errorf("zlib writer: %w", err)
	}
	if _, err := zw.Write(payload); err != nil {
		return "", fmt.Errorf("zlib write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("zlib close: %w", err)
	}

	return base45Encode(buf.Bytes()), nil
}

// base45Encode maps arbitrary bytes to the base45 alphabet per RFC 9285.
// Every 2 input bytes → 3 output chars; a trailing odd byte → 2 chars.
func base45Encode(src []byte) string {
	// Preallocate: 3 chars per 2 bytes, +2 for trailing odd byte.
	out := make([]byte, 0, (len(src)/2)*3+((len(src)%2)*2))
	i := 0
	for ; i+1 < len(src); i += 2 {
		n := int(src[i])*256 + int(src[i+1])
		// n = c*45*45 + b*45 + a  →  (a, b, c)
		a := n % 45
		n /= 45
		b := n % 45
		c := n / 45
		out = append(out, base45Alphabet[a], base45Alphabet[b], base45Alphabet[c])
	}
	if i < len(src) {
		n := int(src[i])
		a := n % 45
		b := n / 45
		out = append(out, base45Alphabet[a], base45Alphabet[b])
	}
	return string(out)
}
