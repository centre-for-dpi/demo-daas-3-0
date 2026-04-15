package pdfwallet

// render.go — PDF + QR rendering for the PDF wallet.
//
// Rendering strategy:
//   1. PixelPass-encode the credential (zlib + base45).
//   2. Attempt to render the payload into a QR code at progressively
//      lower error-correction levels (Highest → High → Medium → Low).
//      We do NOT gate by format — any credential gets a try.
//   3. If every level fails, bail with a QRTooLargeError that names the
//      raw size, encoded size, and concrete alternatives the UI can show
//      the holder.
//   4. Generate the PDF: brand header, claims table, QR image, footer.
//
// The library choices are deliberately minimal and stable:
//   - github.com/skip2/go-qrcode for QR generation (PNG output, simple API)
//   - github.com/go-pdf/fpdf        for pure-Go PDF generation
//
// Neither library has CGO or runtime dependencies.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)

// QRTooLargeError is returned when every recovery level of the QR encoder
// refuses the PixelPass payload. The fields are machine-readable so the
// handler can surface a structured error to the UI.
type QRTooLargeError struct {
	Format      string   // credential format we tried (ldp_vc, jwt_vc_json, vc+sd-jwt)
	RawBytes    int      // size of the raw credential
	EncodedSize int      // size of the PixelPass payload we tried to render
	Attempts    []string // which error-correction levels we tried
	Alternatives []string // human-readable suggestions for the holder
	Underlying  error    // last error from the QR encoder
}

func (e *QRTooLargeError) Error() string {
	alts := ""
	if len(e.Alternatives) > 0 {
		alts = " Alternatives: " + strings.Join(e.Alternatives, "; ") + "."
	}
	return fmt.Sprintf(
		"credential too large for a single self-verifying QR code: format=%s rawBytes=%d encodedBytes=%d (tried %s).%s underlying=%v",
		e.Format, e.RawBytes, e.EncodedSize, strings.Join(e.Attempts, ","), alts, e.Underlying,
	)
}

func (e *QRTooLargeError) Unwrap() error { return e.Underlying }

// RenderCredentialPDF returns PDF bytes for a credential. parsed may be nil
// for JWT VCs that aren't JSON objects — in that case the claims table
// shows the raw credential string instead.
func RenderCredentialPDF(parsed map[string]any, credJSON []byte, format string) ([]byte, error) {
	// Step 1: PixelPass-encode the credential for the QR.
	encoded, err := PixelPassEncode(credJSON)
	if err != nil {
		return nil, fmt.Errorf("pixelpass encode: %w", err)
	}

	// Step 2: attempt QR generation at every recovery level, from High
	// (most robust, smallest capacity) down to Low (largest capacity).
	// If even Low refuses the payload, the credential genuinely doesn't
	// fit in a single QR and we surface a structured error with size
	// details and recovery suggestions.
	qrPNG, usedLevel, qrErr := bestEffortQR(encoded)
	if qrErr != nil {
		return nil, &QRTooLargeError{
			Format:      format,
			RawBytes:    len(credJSON),
			EncodedSize: len(encoded),
			Attempts:    []string{"Highest", "High", "Medium", "Low"},
			Alternatives: []string{
				"Ask the issuer to re-issue as LDP_VC (JSON-LD is typically 30–60% smaller than JWT)",
				"Reduce the claim set to the minimum required by the verifier",
				"Use a different wallet (walt.id, in-process holder) for this credential",
			},
			Underlying: qrErr,
		}
	}

	// Step 3: build the PDF.
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("Verifiable Credential", false)
	pdf.SetAuthor("vc.infra", false)
	pdf.SetCreator("vc.infra PDF Wallet", false)
	pdf.AddPage()

	// Header
	pdf.SetFont("Helvetica", "B", 20)
	pdf.CellFormat(0, 10, "Verifiable Credential", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(110, 110, 110)
	pdf.CellFormat(0, 6, "Offline-verifiable printable copy", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(3)

	// Metadata block
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 6, "Credential metadata", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)

	types := extractTypes(parsed)
	issuer := extractIssuer(parsed)
	issued := extractString(parsed, "issuanceDate", "validFrom")
	expires := extractString(parsed, "expirationDate", "validUntil")

	metaLine(pdf, "Type", types)
	metaLine(pdf, "Format", format)
	metaLine(pdf, "Issuer", issuer)
	if issued != "" {
		metaLine(pdf, "Issued", issued)
	}
	if expires != "" {
		metaLine(pdf, "Expires", expires)
	}
	pdf.Ln(3)

	// Claims block
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 6, "Claims", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)

	claims := extractClaims(parsed)
	if len(claims) == 0 {
		pdf.SetTextColor(130, 130, 130)
		pdf.MultiCell(0, 5, "No structured claims available — this credential is a JWS and must be decoded before inspection.", "", "", false)
		pdf.SetTextColor(0, 0, 0)
	} else {
		keys := make([]string, 0, len(claims))
		for k := range claims {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			metaLine(pdf, k, fmt.Sprintf("%v", claims[k]))
		}
	}
	pdf.Ln(5)

	// QR block: centred, ~80mm square. go-qrcode produces PNG bytes.
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 6, "Offline-verifiable QR ("+usedLevel+" EC)", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(110, 110, 110)
	pdf.MultiCell(0, 4, "This QR carries the complete signed credential (base45(zlib(credJSON)) per PixelPass). A verifier decodes it locally and cryptographically checks the proof — no network round-trip to the issuer required.", "", "", false)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(2)

	opts := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}
	pdf.RegisterImageOptionsReader("qr", opts, bytes.NewReader(qrPNG))
	qrX := 65.0
	qrY := pdf.GetY()
	pdf.ImageOptions("qr", qrX, qrY, 80, 80, false, opts, 0, "")
	pdf.SetY(qrY + 85)

	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(140, 140, 140)
	pdf.CellFormat(0, 5, fmt.Sprintf("Payload: %d bytes encoded (%d raw). Generated by vc.infra PDF Wallet.", len(encoded), len(credJSON)), "", 1, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output: %w", err)
	}
	return buf.Bytes(), nil
}

// bestEffortQR tries progressively lower error-correction levels until the
// payload fits. Low EC gives the largest capacity (≈2,953 bytes in binary
// mode, more in alphanumeric). Returns the PNG bytes, the level name we
// succeeded at, and a nil error. If every level fails, returns the last
// error for QRTooLargeError to wrap.
func bestEffortQR(content string) ([]byte, string, error) {
	attempts := []struct {
		name  string
		level qrcode.RecoveryLevel
	}{
		{"Highest", qrcode.Highest}, // 30% — most robust
		{"High", qrcode.High},       // 25%
		{"Medium", qrcode.Medium},   // 15%
		{"Low", qrcode.Low},         // 7% — largest capacity
	}
	var lastErr error
	for _, a := range attempts {
		png, err := qrcode.Encode(content, a.level, 512)
		if err == nil {
			return png, a.name, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

// metaLine renders a "Label: value" row with a fixed-width label column.
func metaLine(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(40, 5, label, "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	// MultiCell lets long values wrap without stomping the QR later.
	pdf.MultiCell(150, 5, value, "", "L", false)
}

// --- parsed credential helpers ----------------------------------------------

func extractTypes(parsed map[string]any) string {
	if parsed == nil {
		return "VerifiableCredential"
	}
	if raw, ok := parsed["type"]; ok {
		switch v := raw.(type) {
		case string:
			return v
		case []any:
			var out []string
			for _, t := range v {
				if s, ok := t.(string); ok {
					out = append(out, s)
				}
			}
			return strings.Join(out, ", ")
		}
	}
	return "VerifiableCredential"
}

func extractIssuer(parsed map[string]any) string {
	if parsed == nil {
		return "(unknown)"
	}
	switch v := parsed["issuer"].(type) {
	case string:
		return v
	case map[string]any:
		if id, ok := v["id"].(string); ok {
			return id
		}
	}
	return "(unknown)"
}

func extractString(parsed map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := parsed[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// extractClaims pulls the credentialSubject claims into a flat map suitable
// for tabular rendering. Nested objects are serialized as JSON.
func extractClaims(parsed map[string]any) map[string]any {
	if parsed == nil {
		return nil
	}
	subj, ok := parsed["credentialSubject"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for k, v := range subj {
		if k == "id" {
			continue
		}
		switch val := v.(type) {
		case string, float64, int, int64, bool:
			out[k] = val
		default:
			if b, err := json.Marshal(val); err == nil {
				out[k] = string(b)
			} else {
				out[k] = fmt.Sprintf("%v", val)
			}
		}
	}
	return out
}
