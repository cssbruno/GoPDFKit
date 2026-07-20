// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"

	"golang.org/x/image/webp"
)

// parsewebp extracts image information from a WebP image by converting it to
// PNG first.
func (p *imageParser) parsewebp(r io.Reader) (info *ImageInfo) {
	data, err := bufferFromReaderLimit(r, p.sourceLimit)
	if err != nil {
		p.err = err
		return
	}
	config, err := webp.DecodeConfig(bytes.NewReader(data.Bytes()))
	if err != nil {
		p.err = err
		return
	}
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		p.err = fmt.Errorf("WebP dimensions exceed maximum image size: %w", err)
		return
	}
	img, err := webp.Decode(bytes.NewReader(data.Bytes()))
	if err != nil {
		p.err = err
		return
	}
	if img == nil {
		p.err = errors.New("invalid WebP image")
		return
	}
	bounds := img.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	pngBuf := new(bytes.Buffer)
	if err = png.Encode(pngBuf, rgba); err != nil {
		p.err = err
		return
	}
	return p.parsepngstream(pngBuf, false)
}
