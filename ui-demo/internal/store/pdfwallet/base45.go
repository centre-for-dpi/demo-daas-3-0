package pdfwallet

// base45.go — RFC 9285 base45 encoder.
//
// base45 is the encoding used by the EU Digital COVID Certificate and the
// PixelPass specification. It maps three bytes of binary into five ASCII
// characters from a 45-character alphabet that fits QR alphanumeric mode
// (QR alphanumeric mode is ~40% denser than binary mode for the same QR
// version, so the base45 inflation is a net win for QR density).
//
// We only need the encoder; verifiers decode on their side.

const base45Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// Base45Encode encodes b per RFC 9285.
// Every 2 input bytes become 3 output characters; a trailing odd byte
// becomes 2 characters.
func Base45Encode(b []byte) string {
	out := make([]byte, 0, (len(b)/2)*3+2)
	i := 0
	for ; i+1 < len(b); i += 2 {
		x := uint32(b[i])<<8 | uint32(b[i+1])
		c := x / (45 * 45)
		x -= c * (45 * 45)
		d := x / 45
		e := x - d*45
		out = append(out, base45Alphabet[e], base45Alphabet[d], base45Alphabet[c])
	}
	if i < len(b) {
		x := uint32(b[i])
		d := x / 45
		e := x - d*45
		out = append(out, base45Alphabet[e], base45Alphabet[d])
	}
	return string(out)
}
