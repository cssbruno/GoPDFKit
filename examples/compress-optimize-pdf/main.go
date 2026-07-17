// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"log"
	"os"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	uncompressed := buildReport(false)
	compressed := buildReport(true)

	mustWrite(outpath.File("uncompressed-debug.pdf"), uncompressed)
	mustWrite(outpath.File("compressed-optimized.pdf"), compressed)

	fmt.Printf("uncompressed-debug.pdf: %d bytes\n", len(uncompressed))
	fmt.Printf("compressed-optimized.pdf: %d bytes\n", len(compressed))
}

func buildReport(optimize bool) []byte {
	options := []document.Option(nil)
	if optimize {
		options = append(options, document.WithBestCompression())
	}
	pdf := document.MustNew(options...)
	pdf.SetTitle("Compression Example", false)
	pdf.SetCreator("examples/compress-optimize-pdf", false)
	if !optimize {
		pdf.SetCompressionLevel(zlib.NoCompression)
	}

	for page := 1; page <= 6; page++ {
		pdf.AddPage()
		pdf.SetFillColor(35, 70, 120)
		pdf.Rect(0, 0, 210, 24, "F")
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 16)
		pdf.Text(14, 16, fmt.Sprintf("Compressed Stream Page %d", page))

		pdf.SetTextColor(35, 45, 55)
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetXY(14, 36)
		for i := 0; i < 48; i++ {
			pdf.MultiCell(182, 4.5, fmt.Sprintf("Row %02d: repeated report content with stable text, borders, and positioning for compression comparison.", i+1), "B", "L", false)
		}
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		log.Fatal(err)
	}
	return out.Bytes()
}

func mustWrite(path string, data []byte) {
	// #nosec G306 -- the generated example PDF is intentionally readable by the user.
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
}
