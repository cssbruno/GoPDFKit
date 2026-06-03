// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package barcode

import (
	"errors"
	"image"
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

var (
	// ErrNotFound is returned when a barcode key is not registered.
	ErrNotFound = errors.New("barcode not found")
	// ErrInvalid is returned when a registered barcode is invalid.
	ErrInvalid = errors.New("barcode is invalid")
)

// ErrorCorrectionLevel configures QR barcode recovery data.
type ErrorCorrectionLevel = qr.ErrorCorrectionLevel

// Encoding configures QR barcode content encoding.
type Encoding = qr.Encoding

const (
	// Low uses low QR error correction.
	Low ErrorCorrectionLevel = qr.L
	// Medium uses medium QR error correction.
	Medium ErrorCorrectionLevel = qr.M
	// Quartile uses quartile QR error correction.
	Quartile ErrorCorrectionLevel = qr.Q
	// High uses high QR error correction.
	High ErrorCorrectionLevel = qr.H

	// Auto lets the QR encoder choose the content encoding.
	Auto Encoding = qr.Auto
	// Numeric encodes numeric QR content.
	Numeric Encoding = qr.Numeric
	// AlphaNumeric encodes alphanumeric QR content.
	AlphaNumeric Encoding = qr.AlphaNumeric
	// Unicode encodes Unicode QR content.
	Unicode Encoding = qr.Unicode
)

var registry struct {
	sync.Mutex
	cache map[string]boombulerbarcode.Barcode
}

// Register stores a prebuilt barcode image and returns its key.
func Register(code boombulerbarcode.Barcode) string {
	if code == nil {
		return ""
	}
	registry.Lock()
	defer registry.Unlock()

	if len(registry.cache) == 0 {
		registry.cache = make(map[string]boombulerbarcode.Barcode)
	}

	key := Key(code)
	registry.cache[key] = code
	return key
}

// Aztec registers an Aztec barcode and returns its key.
func Aztec(code string, minECCPercent int, userSpecifiedLayers int) (string, error) {
	return registerEncoded(aztec.Encode([]byte(code), minECCPercent, userSpecifiedLayers))
}

// Codabar registers a Codabar barcode and returns its key.
func Codabar(code string) (string, error) {
	return registerEncoded(codabar.Encode(code))
}

// Code128 registers a Code128 barcode and returns its key.
func Code128(code string) (string, error) {
	return registerEncoded(code128.Encode(code))
}

// Code39 registers a Code39 barcode and returns its key.
func Code39(code string, includeChecksum, fullASCIIMode bool) (string, error) {
	return registerEncoded(code39.Encode(code, includeChecksum, fullASCIIMode))
}

// DataMatrix registers a DataMatrix barcode and returns its key.
func DataMatrix(code string) (string, error) {
	return registerEncoded(datamatrix.Encode(code))
}

// EAN registers an EAN8 or EAN13 barcode and returns its key.
func EAN(code string) (string, error) {
	return registerEncoded(ean.Encode(code))
}

// PDF417 registers a PDF417 barcode and returns its key.
func PDF417(code string, securityLevel int) (string, error) {
	if securityLevel < 0 || securityLevel > 8 {
		return "", errors.New("PDF417 security level must be between 0 and 8")
	}
	return registerEncoded(pdf417.Encode(code, byte(securityLevel)))
}

// QR registers a QR barcode and returns its key.
func QR(code string, level ErrorCorrectionLevel, encoding Encoding) (string, error) {
	return registerEncoded(qr.Encode(code, level, encoding))
}

// TwoOfFive registers a TwoOfFive barcode and returns its key.
func TwoOfFive(code string, interleaved bool) (string, error) {
	return registerEncoded(twooffive.Encode(code, interleaved))
}

// Size returns the natural barcode dimensions in pixels.
func Size(key string) (width, height int, err error) {
	code, err := lookup(key)
	if err != nil {
		return 0, 0, err
	}
	return code.Bounds().Dx(), code.Bounds().Dy(), nil
}

// ScaledImage returns a registered barcode scaled to width and height pixels.
func ScaledImage(key string, width, height int) (image.Image, error) {
	code, err := lookup(key)
	if err != nil {
		return nil, err
	}
	return boombulerbarcode.Scale(code, width, height)
}

// Key returns the registry key for code.
func Key(code boombulerbarcode.Barcode) string {
	return code.Metadata().CodeKind + code.Content()
}

func registerEncoded(code boombulerbarcode.Barcode, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if code == nil {
		return "", ErrInvalid
	}
	return Register(code), nil
}

func lookup(key string) (boombulerbarcode.Barcode, error) {
	registry.Lock()
	code, ok := registry.cache[key]
	registry.Unlock()

	if !ok {
		return nil, ErrNotFound
	}
	if code == nil {
		return nil, ErrInvalid
	}
	return code, nil
}
