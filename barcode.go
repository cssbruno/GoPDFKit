// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"errors"
	"image/jpeg"
	"strconv"
	"sync"

	boombulerbarcode "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/aztec"
	"github.com/boombuler/barcode/codabar"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/pdf417"
	"github.com/boombuler/barcode/qr"
	"github.com/boombuler/barcode/twooffive"
)

// QRBarcodeErrorCorrectionLevel configures QR barcode recovery data.
type QRBarcodeErrorCorrectionLevel = qr.ErrorCorrectionLevel

// QRBarcodeEncoding configures QR barcode content encoding.
type QRBarcodeEncoding = qr.Encoding

const (
	// QRBarcodeLow uses low QR error correction.
	QRBarcodeLow QRBarcodeErrorCorrectionLevel = qr.L
	// QRBarcodeMedium uses medium QR error correction.
	QRBarcodeMedium QRBarcodeErrorCorrectionLevel = qr.M
	// QRBarcodeQuartile uses quartile QR error correction.
	QRBarcodeQuartile QRBarcodeErrorCorrectionLevel = qr.Q
	// QRBarcodeHigh uses high QR error correction.
	QRBarcodeHigh QRBarcodeErrorCorrectionLevel = qr.H

	// QRBarcodeAuto lets the QR encoder choose the content encoding.
	QRBarcodeAuto QRBarcodeEncoding = qr.Auto
	// QRBarcodeNumeric encodes numeric QR content.
	QRBarcodeNumeric QRBarcodeEncoding = qr.Numeric
	// QRBarcodeAlphaNumeric encodes alphanumeric QR content.
	QRBarcodeAlphaNumeric QRBarcodeEncoding = qr.AlphaNumeric
	// QRBarcodeUnicode encodes Unicode QR content.
	QRBarcodeUnicode QRBarcodeEncoding = qr.Unicode
)

var barcodeRegistry struct {
	sync.Mutex
	cache map[string]boombulerbarcode.Barcode
}

// RegisterBarcode registers a barcode image for later placement on a page.
func RegisterBarcode(bcode boombulerbarcode.Barcode) string {
	if bcode == nil {
		return ""
	}
	barcodeRegistry.Lock()
	defer barcodeRegistry.Unlock()

	if len(barcodeRegistry.cache) == 0 {
		barcodeRegistry.cache = make(map[string]boombulerbarcode.Barcode)
	}

	key := barcodeKey(bcode)
	barcodeRegistry.cache[key] = bcode
	return key
}

// RegisterAztecBarcode registers an Aztec barcode for later placement.
func (f *Fpdf) RegisterAztecBarcode(code string, minECCPercent int, userSpecifiedLayers int) string {
	bcode, err := aztec.Encode([]byte(code), minECCPercent, userSpecifiedLayers)
	return f.registerBarcode(bcode, err)
}

// RegisterCodabarBarcode registers a Codabar barcode for later placement.
func (f *Fpdf) RegisterCodabarBarcode(code string) string {
	bcode, err := codabar.Encode(code)
	return f.registerBarcode(bcode, err)
}

// RegisterCode128Barcode registers a Code128 barcode for later placement.
func (f *Fpdf) RegisterCode128Barcode(code string) string {
	bcode, err := code128.Encode(code)
	return f.registerBarcode(bcode, err)
}

// RegisterCode39Barcode registers a Code39 barcode for later placement.
func (f *Fpdf) RegisterCode39Barcode(code string, includeChecksum, fullASCIIMode bool) string {
	bcode, err := code39.Encode(code, includeChecksum, fullASCIIMode)
	return f.registerBarcode(bcode, err)
}

// RegisterDataMatrixBarcode registers a DataMatrix barcode for later placement.
func (f *Fpdf) RegisterDataMatrixBarcode(code string) string {
	bcode, err := datamatrix.Encode(code)
	return f.registerBarcode(bcode, err)
}

// RegisterEANBarcode registers an EAN8 or EAN13 barcode for later placement.
func (f *Fpdf) RegisterEANBarcode(code string) string {
	bcode, err := ean.Encode(code)
	return f.registerBarcode(bcode, err)
}

// RegisterPDF417Barcode registers a PDF417 barcode for later placement.
//
// columns is kept for API compatibility and is ignored because the backing
// barcode library chooses the final dimensions automatically.
func (f *Fpdf) RegisterPDF417Barcode(code string, columns int, securityLevel int) string {
	if securityLevel < 0 || securityLevel > 8 {
		f.SetError(errors.New("PDF417 security level must be between 0 and 8"))
		return ""
	}
	bcode, err := pdf417.Encode(code, byte(securityLevel))
	return f.registerBarcode(bcode, err)
}

// RegisterQRBarcode registers a QR barcode for later placement.
func (f *Fpdf) RegisterQRBarcode(code string, ecl QRBarcodeErrorCorrectionLevel, mode QRBarcodeEncoding) string {
	bcode, err := qr.Encode(code, ecl, mode)
	return f.registerBarcode(bcode, err)
}

// RegisterTwoOfFiveBarcode registers a TwoOfFive barcode for later placement.
func (f *Fpdf) RegisterTwoOfFiveBarcode(code string, interleaved bool) string {
	bcode, err := twooffive.Encode(code, interleaved)
	return f.registerBarcode(bcode, err)
}

// Barcode places a registered barcode on the current page.
func (f *Fpdf) Barcode(code string, x, y, w, h float64, flow bool) {
	f.printBarcode(code, x, y, &w, &h, flow)
}

// BarcodeUnscalable places a registered barcode with optional dimensions.
//
// A nil width or height leaves that dimension unscaled.
func (f *Fpdf) BarcodeUnscalable(code string, x, y float64, w, h *float64, flow bool) {
	f.printBarcode(code, x, y, w, h, flow)
}

// GetUnscaledBarcodeDimensions returns the natural barcode width and height in
// the current PDF unit.
func (f *Fpdf) GetUnscaledBarcodeDimensions(code string) (w, h float64) {
	barcodeRegistry.Lock()
	unscaled, ok := barcodeRegistry.cache[code]
	barcodeRegistry.Unlock()

	if !ok {
		f.SetError(errors.New("Barcode not found"))
		return 0, 0
	}
	if unscaled == nil {
		f.SetError(errors.New("Barcode is invalid"))
		return 0, 0
	}

	return f.convertBarcodeFrom96DPI(float64(unscaled.Bounds().Dx())),
		f.convertBarcodeFrom96DPI(float64(unscaled.Bounds().Dy()))
}

func (f *Fpdf) registerBarcode(bcode boombulerbarcode.Barcode, err error) string {
	if err != nil {
		f.SetError(err)
		return ""
	}
	if bcode == nil {
		f.SetError(errors.New("Barcode encoder returned no barcode"))
		return ""
	}
	return RegisterBarcode(bcode)
}

func (f *Fpdf) printBarcode(code string, x, y float64, w, h *float64, flow bool) {
	barcodeRegistry.Lock()
	unscaled, ok := barcodeRegistry.cache[code]
	barcodeRegistry.Unlock()

	if !ok {
		f.SetError(errors.New("Barcode not found"))
		return
	}
	if unscaled == nil {
		f.SetError(errors.New("Barcode is invalid"))
		return
	}

	name := uniqueBarcodeName(code, x, y)
	scaleToWidth := unscaled.Bounds().Dx()
	scaleToHeight := unscaled.Bounds().Dy()

	if f.GetImageInfo(name) == nil {
		bcode, err := boombulerbarcode.Scale(unscaled, scaleToWidth, scaleToHeight)
		if err != nil {
			f.SetError(err)
			return
		}
		if err := f.registerScaledBarcode(name, bcode); err != nil {
			f.SetError(err)
			return
		}
	}

	width := float64(scaleToWidth)
	height := float64(scaleToHeight)
	if w != nil {
		width = *w
	}
	if h != nil {
		height = *h
	}

	f.ImageOptions(name, x, y, width, height, flow, ImageOptions{ImageType: "jpg"}, 0, "")
}

func (f *Fpdf) registerScaledBarcode(code string, bcode boombulerbarcode.Barcode) error {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, bcode, nil); err != nil {
		return err
	}
	f.RegisterImageOptionsReader(code, ImageOptions{ImageType: "jpg"}, bytes.NewReader(buf.Bytes()))
	return nil
}

func (f *Fpdf) convertBarcodeFrom96DPI(value float64) float64 {
	return value / f.GetConversionRatio() * 72 / 96
}

func uniqueBarcodeName(code string, x, y float64) string {
	xStr := strconv.FormatFloat(x, 'E', -1, 64)
	yStr := strconv.FormatFloat(y, 'E', -1, 64)
	return "barcode-" + code + "-" + xStr + yStr
}

func barcodeKey(bcode boombulerbarcode.Barcode) string {
	return bcode.Metadata().CodeKind + bcode.Content()
}
