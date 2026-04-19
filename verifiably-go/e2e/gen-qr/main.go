// gen-qr generates a PNG QR code to a path. Used by the scan/upload e2e test
// to exercise the real server-side gozxing decoder.
package main

import (
	"fmt"
	"os"

	qr "github.com/skip2/go-qrcode"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen-qr <payload> <output.png>")
		os.Exit(2)
	}
	payload := os.Args[1]
	out := os.Args[2]
	png, err := qr.Encode(payload, qr.Medium, 512)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, png, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(png))
}
