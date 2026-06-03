// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"image"
	"image/jpeg"
	"strconv"

	"github.com/cssbruno/gopdfkit/barcode"
)

// Barcode places a registered barcode on the current page.
func (f *Document) Barcode(code string, x, y, w, h float64, flow bool) {
	f.printBarcode(code, x, y, &w, &h, flow)
}

// BarcodeUnscalable places a registered barcode with optional dimensions.
//
// A nil width or height leaves that dimension unscaled.
func (f *Document) BarcodeUnscalable(code string, x, y float64, w, h *float64, flow bool) {
	f.printBarcode(code, x, y, w, h, flow)
}

// GetUnscaledBarcodeDimensions returns the natural barcode width and height in
// the current PDF unit.
func (f *Document) GetUnscaledBarcodeDimensions(code string) (w, h float64) {
	width, height, err := barcode.Size(code)
	if err != nil {
		f.SetError(err)
		return 0, 0
	}

	return f.convertBarcodeFrom96DPI(float64(width)),
		f.convertBarcodeFrom96DPI(float64(height))
}

func (f *Document) printBarcode(code string, x, y float64, w, h *float64, flow bool) {
	scaleToWidth, scaleToHeight, err := barcode.Size(code)
	if err != nil {
		f.SetError(err)
		return
	}

	name := uniqueBarcodeName(code, x, y)

	if f.GetImageInfo(name) == nil {
		bcode, err := barcode.ScaledImage(code, scaleToWidth, scaleToHeight)
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

func (f *Document) registerScaledBarcode(code string, bcode image.Image) error {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, bcode, nil); err != nil {
		return err
	}
	f.RegisterImageOptionsReader(code, ImageOptions{ImageType: "jpg"}, bytes.NewReader(buf.Bytes()))
	return nil
}

func (f *Document) convertBarcodeFrom96DPI(value float64) float64 {
	return value / f.GetConversionRatio() * 72 / 96
}

func uniqueBarcodeName(code string, x, y float64) string {
	xStr := strconv.FormatFloat(x, 'E', -1, 64)
	yStr := strconv.FormatFloat(y, 'E', -1, 64)
	return "barcode-" + code + "-" + xStr + yStr
}
