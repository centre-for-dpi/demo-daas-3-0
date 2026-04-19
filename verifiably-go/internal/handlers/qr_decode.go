package handlers

import (
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"net/http"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// decodeUploadedQR reads a multipart-form file (field name "credential_image")
// and returns the payload encoded in the QR code it contains. Returns an
// error if no file was uploaded, the image can't be decoded, or no QR was
// found in the frame.
//
// Accepts PNG and JPEG. PDF upload is out of scope at this milestone — users
// who want to verify a credential PDF should extract the QR as an image
// first (or use their camera).
func decodeUploadedQR(r *http.Request) (string, error) {
	file, header, err := r.FormFile("credential_image")
	if err != nil {
		return "", fmt.Errorf("no credential_image uploaded")
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", header.Filename, err)
	}
	_ = format

	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", fmt.Errorf("binary bitmap: %w", err)
	}
	reader := qrcode.NewQRCodeReader()
	result, err := reader.Decode(bmp, nil)
	if err != nil {
		return "", fmt.Errorf("decode QR: %w", err)
	}
	return result.GetText(), nil
}
