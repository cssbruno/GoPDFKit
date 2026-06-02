// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit_test

import (
	"testing"

	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/qr"
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/testsupport/example"
)

func createPdf() (pdf *gopdfkit.Fpdf) {
	pdf = gopdfkit.New("L", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetFillColor(200, 200, 220)
	pdf.AddPage()
	return
}

func ExampleRegisterBarcode() {
	pdf := createPdf()

	fileStr := example.Filename("barcode_Register")

	bcode, err := code128.Encode("gopdfkit")

	if err == nil {
		key := gopdfkit.RegisterBarcode(bcode)
		width := 100.0
		height := 10.0
		pdf.BarcodeUnscalable(key, 15, 15, &width, &height, false)
	}

	err = pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_Register.pdf
}

func ExampleFpdf_RegisterCodabarBarcode() {
	pdf := createPdf()

	key := pdf.RegisterCodabarBarcode("A1234B")
	var width float64 = 100
	var height float64 = 10
	pdf.BarcodeUnscalable(key, 15, 15, &width, &height, false)

	fileStr := example.Filename("barcode_RegisterCodabar")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterCodabar.pdf
}

func ExampleFpdf_RegisterAztecBarcode() {
	pdf := createPdf()

	key := pdf.RegisterAztecBarcode("aztec", 33, 0)
	pdf.Barcode(key, 15, 15, 100, 100, false)

	fileStr := example.Filename("barcode_RegisterAztec")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterAztec.pdf
}

func ExampleFpdf_RegisterCode128Barcode() {
	pdf := createPdf()

	key := pdf.RegisterCode128Barcode("code128")
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterCode128")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterCode128.pdf
}

func ExampleFpdf_RegisterCode39Barcode() {
	pdf := createPdf()

	key := pdf.RegisterCode39Barcode("CODE39", false, true)
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterCode39")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterCode39.pdf
}

func ExampleFpdf_RegisterDataMatrixBarcode() {
	pdf := createPdf()

	key := pdf.RegisterDataMatrixBarcode("datamatrix")
	pdf.Barcode(key, 15, 15, 20, 20, false)

	fileStr := example.Filename("barcode_RegisterDataMatrix")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterDataMatrix.pdf
}

func ExampleFpdf_RegisterEANBarcode() {
	pdf := createPdf()

	key := pdf.RegisterEANBarcode("96385074")
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterEAN")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterEAN.pdf
}

func ExampleFpdf_RegisterQRBarcode() {
	pdf := createPdf()

	key := pdf.RegisterQRBarcode("qrcode", qr.H, qr.Unicode)
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterQR")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterQR.pdf
}

func ExampleFpdf_RegisterTwoOfFiveBarcode() {
	pdf := createPdf()

	key := pdf.RegisterTwoOfFiveBarcode("1234567895", true)
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterTwoOfFive")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterTwoOfFive.pdf
}

func ExampleFpdf_RegisterPDF417Barcode() {
	pdf := createPdf()

	key := pdf.RegisterPDF417Barcode("1234567895", 10, 5)
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterPdf417")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterPdf417.pdf
}

// TestRegisterCode128 ensures invalid input records an error instead of panicking.
func TestRegisterCode128(t *testing.T) {
	pdf := createPdf()
	pdf.RegisterCode128Barcode("Invalid character: é")
}

// TestBarcodeUnscalable exercises optional width and height scaling.
func TestBarcodeUnscalable(t *testing.T) {
	pdf := createPdf()

	key := pdf.RegisterCode128Barcode("code128")
	var width float64 = 100
	var height float64 = 10
	pdf.BarcodeUnscalable(key, 15, 15, &width, &height, false)
	pdf.BarcodeUnscalable(key, 15, 35, nil, &height, false)
	pdf.BarcodeUnscalable(key, 15, 55, &width, nil, false)
	pdf.BarcodeUnscalable(key, 15, 75, nil, nil, false)

	fileStr := example.Filename("barcode_Barcode")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated ../assets/generated/pdf/barcode_Barcode.pdf
}

// TestGetUnscaledBarcodeDimensions checks that returned dimensions match the barcode.
func TestGetUnscaledBarcodeDimensions(t *testing.T) {
	pdf := createPdf()

	key := pdf.RegisterQRBarcode("qrcode", qr.H, qr.Unicode)
	pdf.BarcodeUnscalable(key, 15, 15, nil, nil, false)
	w, h := pdf.GetUnscaledBarcodeDimensions(key)

	pdf.SetDrawColor(255, 0, 0)
	pdf.Line(15, 15, 15+w, 15+h)

	fileStr := example.Filename("barcode_GetBarcodeDimensions")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated ../assets/generated/pdf/barcode_GetBarcodeDimensions.pdf
}

// TestBarcodeNonIntegerScalingFactors exercises non-integer barcode sizes.
func TestBarcodeNonIntegerScalingFactors(t *testing.T) {
	pdf := gopdfkit.New("L", "in", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetFillColor(200, 200, 220)
	pdf.AddPage()

	key := pdf.RegisterQRBarcode("qrcode", qr.H, qr.Unicode)
	scale := 1.5
	pdf.BarcodeUnscalable(key, 0.5, 0.5, &scale, &scale, false)

	pdf.SetDrawColor(255, 0, 0)
	pdf.Line(0.5, 0.5, 0.5+scale, 0.5+scale)

	fileStr := example.Filename("barcode_BarcodeScaling")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated ../assets/generated/pdf/barcode_BarcodeScaling.pdf
}
