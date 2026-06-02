/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2015 Jelmer Snoeck (Gmail: jelmer.snoeck)                  *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package barcode

import (
	"bytes"
	"errors"
	"image/jpeg"
	"io"
	"strconv"
	"sync"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/aztec"
	"github.com/boombuler/barcode/codabar"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/pdf417"
	"github.com/boombuler/barcode/qr"
	"github.com/boombuler/barcode/twooffive"
	"github.com/cssbruno/gopdfkit"
)

// barcodes represents the barcodes that have been registered through this
// package. They will later be used to be scaled and put on the page.
// RubenN: made this a struct with a mutex to prevent race condition
var barcodes struct {
	sync.Mutex
	cache map[string]barcode.Barcode
}

// barcodePdf is a partial PDF implementation that only implements a subset of
// functions that are required to add the barcode to the PDF.
type barcodePdf interface {
	GetConversionRatio() float64
	GetImageInfo(imageStr string) *gopdfkit.ImageInfo
	ImageOptions(imageNameStr string, x, y, w, h float64, flow bool, options gopdfkit.ImageOptions, link int, linkStr string)
	RegisterImageOptionsReader(imgName string, options gopdfkit.ImageOptions, r io.Reader) *gopdfkit.ImageInfo
	SetError(err error)
}

// printBarcode internally prints the scaled or unscaled barcode to the PDF. Used by both
// Barcode() and BarcodeUnscalable().
func printBarcode(pdf barcodePdf, code string, x, y float64, w, h *float64, flow bool) {
	barcodes.Lock()
	unscaled, ok := barcodes.cache[code]
	barcodes.Unlock()

	if !ok {
		err := errors.New("Barcode not found")
		pdf.SetError(err)
		return
	}

	bname := uniqueBarcodeName(code, x, y)
	info := pdf.GetImageInfo(bname)
	scaleToWidth := unscaled.Bounds().Dx()
	scaleToHeight := unscaled.Bounds().Dy()

	if info == nil {
		bcode, err := barcode.Scale(
			unscaled,
			scaleToWidth,
			scaleToHeight,
		)

		if err != nil {
			pdf.SetError(err)
			return
		}

		err = registerScaledBarcode(pdf, bname, bcode)
		if err != nil {
			pdf.SetError(err)
			return
		}
	}

	scaleToWidthF := float64(scaleToWidth)
	scaleToHeightF := float64(scaleToHeight)

	if w != nil {
		scaleToWidthF = *w
	}
	if h != nil {
		scaleToHeightF = *h
	}

	pdf.ImageOptions(bname, x, y, scaleToWidthF, scaleToHeightF, flow, gopdfkit.ImageOptions{ImageType: "jpg"}, 0, "")

}

// BarcodeUnscalable puts a registered barcode in the current page.
//
// Its arguments work in the same way as that of Barcode(). However, it allows for an unscaled
// barcode in the width and/or height dimensions. This can be useful if you want to prevent
// side effects of upscaling.
func BarcodeUnscalable(pdf barcodePdf, code string, x, y float64, w, h *float64, flow bool) {
	printBarcode(pdf, code, x, y, w, h, flow)
}

// Barcode puts a registered barcode in the current page.
//
// The size should be specified in the units used to create the PDF document.
// If width or height are left unspecfied, the barcode is not scaled in the unspecified dimensions.
//
// Positioning with x, y and flow is inherited from Fpdf.ImageOptions().
func Barcode(pdf barcodePdf, code string, x, y, w, h float64, flow bool) {
	printBarcode(pdf, code, x, y, &w, &h, flow)
}

// GetUnscaledBarcodeDimensions returns the width and height of the
// unscaled barcode associated with the given code.
func GetUnscaledBarcodeDimensions(pdf barcodePdf, code string) (w, h float64) {
	barcodes.Lock()
	unscaled, ok := barcodes.cache[code]
	barcodes.Unlock()

	if !ok {
		err := errors.New("Barcode not found")
		pdf.SetError(err)
		return
	}

	return convertFrom96Dpi(pdf, float64(unscaled.Bounds().Dx())),
		convertFrom96Dpi(pdf, float64(unscaled.Bounds().Dy()))
}

// Register registers a barcode but does not put it on the page. Use Barcode()
// with the same code to put the barcode on the PDF page.
func Register(bcode barcode.Barcode) string {
	barcodes.Lock()
	if len(barcodes.cache) == 0 {
		barcodes.cache = make(map[string]barcode.Barcode)
	}

	key := barcodeKey(bcode)
	barcodes.cache[key] = bcode
	barcodes.Unlock()

	return key
}

// RegisterAztec registers a barcode of type Aztec to the PDF, but not to
// the page. Use Barcode() with the return value to put the barcode on the page.
// code is the string to be encoded. minECCPercent is the error correction percentage. 33 is the default.
// userSpecifiedLayers can be a value between -4 and 32 inclusive.
func RegisterAztec(pdf barcodePdf, code string, minECCPercent int, userSpecifiedLayers int) string {
	bcode, err := aztec.Encode([]byte(code), minECCPercent, userSpecifiedLayers)
	return registerBarcode(pdf, bcode, err)
}

// RegisterCodabar registers a barcode of type Codabar to the PDF, but not to
// the page. Use Barcode() with the return value to put the barcode on the page.
func RegisterCodabar(pdf barcodePdf, code string) string {
	bcode, err := codabar.Encode(code)
	return registerBarcode(pdf, bcode, err)
}

// RegisterCode128 registers a barcode of type Code128 to the PDF, but not to
// the page. Use Barcode() with the return value to put the barcode on the page.
func RegisterCode128(pdf barcodePdf, code string) string {
	bcode, err := code128.Encode(code)
	return registerBarcode(pdf, bcode, err)
}

// RegisterCode39 registers a barcode of type Code39 to the PDF, but not to
// the page. Use Barcode() with the return value to put the barcode on the page.
//
// includeChecksum and fullASCIIMode are inherited from code39.Encode().
func RegisterCode39(pdf barcodePdf, code string, includeChecksum, fullASCIIMode bool) string {
	bcode, err := code39.Encode(code, includeChecksum, fullASCIIMode)
	return registerBarcode(pdf, bcode, err)
}

// RegisterDataMatrix registers a barcode of type DataMatrix to the PDF, but not
// to the page. Use Barcode() with the return value to put the barcode on the
// page.
func RegisterDataMatrix(pdf barcodePdf, code string) string {
	bcode, err := datamatrix.Encode(code)
	return registerBarcode(pdf, bcode, err)
}

// RegisterPdf417 registers a barcode of type PDF417 to the PDF, but not to the
// page. code is the string to be encoded. columns is kept for API
// compatibility and ignored because boombuler/barcode chooses the final
// dimensions automatically. securityLevel specifies an error correction level
// between zero and 8 inclusive. Use Barcode() with the return value to put the
// barcode on the page.
func RegisterPdf417(pdf barcodePdf, code string, columns int, securityLevel int) string {
	if securityLevel < 0 || securityLevel > 8 {
		pdf.SetError(errors.New("PDF417 security level must be between 0 and 8"))
		return ""
	}
	bcode, err := pdf417.Encode(code, byte(securityLevel))
	return registerBarcode(pdf, bcode, err)
}

// RegisterEAN registers a barcode of type EAN to the PDF, but not to the page.
// It will automatically detect if the barcode is EAN8 or EAN13. Use Barcode()
// with the return value to put the barcode on the page.
func RegisterEAN(pdf barcodePdf, code string) string {
	bcode, err := ean.Encode(code)
	return registerBarcode(pdf, bcode, err)
}

// RegisterQR registers a barcode of type QR to the PDF, but not to the page.
// Use Barcode() with the return value to put the barcode on the page.
//
// The ErrorCorrectionLevel and Encoding mode are inherited from qr.Encode().
func RegisterQR(pdf barcodePdf, code string, ecl qr.ErrorCorrectionLevel, mode qr.Encoding) string {
	bcode, err := qr.Encode(code, ecl, mode)
	return registerBarcode(pdf, bcode, err)
}

// RegisterTwoOfFive registers a barcode of type TwoOfFive to the PDF, but not
// to the page. Use Barcode() with the return value to put the barcode on the
// page.
//
// The interleaved bool is inherited from twooffive.Encode().
func RegisterTwoOfFive(pdf barcodePdf, code string, interleaved bool) string {
	bcode, err := twooffive.Encode(code, interleaved)
	return registerBarcode(pdf, bcode, err)
}

// registerBarcode registers a barcode internally using the Register() function.
// In case of an error generating the barcode it will not be registered and will
// set an error on the PDF. It will return a unique key for the barcode type and
// content that can be used to put the barcode on the page.
func registerBarcode(pdf barcodePdf, bcode barcode.Barcode, err error) string {
	if err != nil {
		pdf.SetError(err)
		return ""
	}

	return Register(bcode)
}

// uniqueBarcodeName makes sure every barcode has a unique name for its
// dimensions. Scaling a barcode image results in quality loss, which could be
// a problem for barcode readers.
func uniqueBarcodeName(code string, x, y float64) string {
	xStr := strconv.FormatFloat(x, 'E', -1, 64)
	yStr := strconv.FormatFloat(y, 'E', -1, 64)

	return "barcode-" + code + "-" + xStr + yStr
}

// barcodeKey combines the code type and code value into a unique identifier for
// a barcode type. This is so that we can store several barcodes with the same
// code but different type in the barcodes map.
func barcodeKey(bcode barcode.Barcode) string {
	return bcode.Metadata().CodeKind + bcode.Content()
}

// registerScaledBarcode registers a barcode with its exact dimensions to the
// PDF but does not put it on the page. Use Fpdf.ImageOptions() with the same
// code to add the barcode to the page.
func registerScaledBarcode(pdf barcodePdf, code string, bcode barcode.Barcode) error {
	buf := new(bytes.Buffer)
	err := jpeg.Encode(buf, bcode, nil)

	if err != nil {
		return err
	}

	reader := bytes.NewReader(buf.Bytes())
	pdf.RegisterImageOptionsReader(code, gopdfkit.ImageOptions{ImageType: "jpg"}, reader)

	return nil
}

// convertTo96DPI converts the given value, which is based on a 72 DPI value
// like the rest of the PDF document, to a 96 DPI value that is required for
// an image.
//
// Doing this through the image placement function would mean that it uses a 72
// DPI value and stretches it to a 96 DPI value. This results in quality loss
// which could be problematic for barcode scanners.
func convertTo96Dpi(pdf barcodePdf, value float64) float64 {
	return value * pdf.GetConversionRatio() / 72 * 96
}

// convertFrom96Dpi converts the given value, which is based on a 96 DPI value
// required for an Image, to a 72 DPI value like the rest of the PDF document.
func convertFrom96Dpi(pdf barcodePdf, value float64) float64 {
	return value / pdf.GetConversionRatio() * 72 / 96
}
