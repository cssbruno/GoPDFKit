// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
)

const (
	defaultQRCodeSizePx = 256
	qrCodeNameHashBytes = 8
)

// QRCodeImageName returns the deterministic image name used for a QR payload.
func QRCodeImageName(payload string) string {
	payload = strings.TrimSpace(payload)
	hash := sha256.Sum256([]byte(payload))

	return "qr_" + hex.EncodeToString(hash[:qrCodeNameHashBytes])
}

// QRCodePNG renders payload as PNG-encoded QR-code bytes.
func QRCodePNG(payload string, sizePx int) ([]byte, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, errors.New("qr payload is empty")
	}

	if sizePx <= 0 {
		sizePx = defaultQRCodeSizePx
	}

	code, err := qr.Encode(payload, qr.M, qr.Auto)
	if err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}

	scaled, err := barcode.Scale(code, sizePx, sizePx)
	if err != nil {
		return nil, fmt.Errorf("qr scale: %w", err)
	}

	gray := image.NewGray(scaled.Bounds())
	for y := 0; y < gray.Rect.Dy(); y++ {
		for x := 0; x < gray.Rect.Dx(); x++ {
			gray.Pix[y*gray.Stride+x] = color.GrayModel.Convert(scaled.At(x+gray.Rect.Min.X, y+gray.Rect.Min.Y)).(color.Gray).Y
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, gray); err != nil {
		return nil, fmt.Errorf("qr png encode: %w", err)
	}

	return buf.Bytes(), nil
}

// RegisterQRCodePNG registers a QR code as a PNG image and returns its image
// name for subsequent ImageOptions calls.
func (f *Document) RegisterQRCodePNG(payload string, sizePx int) (string, error) {
	if f == nil {
		return "", errors.New("document is nil")
	}

	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", errors.New("qr payload is empty")
	}

	name := QRCodeImageName(payload)
	if info, _ := f.ensureResourceStore().image(name); info != nil {
		return name, nil
	}

	data, err := QRCodePNG(payload, sizePx)
	if err != nil {
		return "", err
	}

	f.RegisterImageOptionsReader(name, ImageOptions{ImageType: "png", ReadDpi: true}, bytes.NewReader(data))

	return name, f.Error()
}
