// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"log"

	barcodelib "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	code, err := qr.Encode("https://example.test/verify", qr.M, qr.Auto)
	if err != nil {
		log.Fatal(err)
	}
	code, err = barcodelib.Scale(code, 256, 256)
	if err != nil {
		log.Fatal(err)
	}
	qrImage := image.NewRGBA(code.Bounds())
	draw.Draw(qrImage, qrImage.Bounds(), code, code.Bounds().Min, draw.Src)

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, qrImage); err != nil {
		log.Fatal(err)
	}

	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(40, 10, "External QR code as an image")
	pdf.Ln(14)

	options := document.ImageOptions{ImageType: "png"}
	pdf.RegisterImageOptionsReader("qr-code", options, bytes.NewReader(pngData.Bytes()))
	pdf.ImageOptions("qr-code", 20, 30, 35, 35, false, options, 0, "")

	if err := pdf.OutputFileAndClose(outpath.File("qr-code.pdf")); err != nil {
		log.Fatal(err)
	}
}
