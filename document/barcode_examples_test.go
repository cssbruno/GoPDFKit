// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"testing"

	"github.com/boombuler/barcode/code128"
	"github.com/cssbruno/gopdfkit/barcode"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/testsupport/example"
)

func createPdf() (pdf *document.Document) {
	pdf = document.New("L", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetFillColor(200, 200, 220)
	pdf.AddPage()
	return
}

func mustBarcode(key string, err error) string {
	if err != nil {
		panic(err)
	}
	return key
}

func Example_barcodeRegister() {
	pdf := createPdf()

	fileStr := example.Filename("barcode_Register")

	bcode, err := code128.Encode("gopdfkit")

	if err == nil {
		key := barcode.Register(bcode)
		width := 100.0
		height := 10.0
		pdf.BarcodeUnscalable(key, 15, 15, &width, &height, false)
	}

	err = pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_Register.pdf
}

func Example_barcodeCodabar() {
	pdf := createPdf()

	key := mustBarcode(barcode.Codabar("A1234B"))
	var width float64 = 100
	var height float64 = 10
	pdf.BarcodeUnscalable(key, 15, 15, &width, &height, false)

	fileStr := example.Filename("barcode_RegisterCodabar")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterCodabar.pdf
}

func Example_barcodeAztec() {
	pdf := createPdf()

	key := mustBarcode(barcode.Aztec("aztec", 33, 0))
	pdf.Barcode(key, 15, 15, 100, 100, false)

	fileStr := example.Filename("barcode_RegisterAztec")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterAztec.pdf
}

func Example_barcodeCode128() {
	pdf := createPdf()

	key := mustBarcode(barcode.Code128("code128"))
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterCode128")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterCode128.pdf
}

func Example_barcodeCode39() {
	pdf := createPdf()

	key := mustBarcode(barcode.Code39("CODE39", false, true))
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterCode39")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterCode39.pdf
}

func Example_barcodeDataMatrix() {
	pdf := createPdf()

	key := mustBarcode(barcode.DataMatrix("datamatrix"))
	pdf.Barcode(key, 15, 15, 20, 20, false)

	fileStr := example.Filename("barcode_RegisterDataMatrix")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterDataMatrix.pdf
}

func Example_barcodeEAN() {
	pdf := createPdf()

	key := mustBarcode(barcode.EAN("96385074"))
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterEAN")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterEAN.pdf
}

func Example_barcodeQR() {
	pdf := createPdf()

	key := mustBarcode(barcode.QR("qrcode", barcode.High, barcode.Unicode))
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterQR")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterQR.pdf
}

func Example_barcodeTwoOfFive() {
	pdf := createPdf()

	key := mustBarcode(barcode.TwoOfFive("1234567895", true))
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterTwoOfFive")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterTwoOfFive.pdf
}

func Example_barcodePDF417() {
	pdf := createPdf()

	key := mustBarcode(barcode.PDF417("1234567895", 5))
	pdf.Barcode(key, 15, 15, 100, 10, false)

	fileStr := example.Filename("barcode_RegisterPdf417")
	err := pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/barcode_RegisterPdf417.pdf
}

// TestRegisterCode128 ensures invalid input returns an error instead of panicking.
func TestRegisterCode128(t *testing.T) {
	if _, err := barcode.Code128("Invalid character: é"); err == nil {
		t.Fatal("Code128 accepted invalid input")
	}
}

// TestBarcodeUnscalable exercises optional width and height scaling.
func TestBarcodeUnscalable(t *testing.T) {
	pdf := createPdf()

	key := mustBarcode(barcode.Code128("code128"))
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

	key := mustBarcode(barcode.QR("qrcode", barcode.High, barcode.Unicode))
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
	pdf := document.New("L", "in", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetFillColor(200, 200, 220)
	pdf.AddPage()

	key := mustBarcode(barcode.QR("qrcode", barcode.High, barcode.Unicode))
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
