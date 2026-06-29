// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"image/color"
	"image/jpeg"
	"io"
)

// parsejpg extracts image information from an io.Reader containing JPEG data.
// Thank you, Bruno Michel, for providing this code.
func (p *imageParser) parsejpg(r io.Reader) (info *ImageInfo) {
	info = p.newImageInfo()
	var (
		data bytes.Buffer
		err  error
	)
	_, err = data.ReadFrom(io.LimitReader(r, int64(p.sourceLimit)+1))
	if err != nil {
		p.err = err
		return
	}
	if data.Len() > p.sourceLimit {
		p.err = fmt.Errorf("%w: image data exceeds maximum size", ErrImageTooLarge)
		return
	}
	info.data = data.Bytes()
	config, err := jpeg.DecodeConfig(bytes.NewReader(info.data))
	if err != nil {
		p.err = err
		return
	}
	info.w = float64(config.Width)
	info.h = float64(config.Height)
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		p.err = fmt.Errorf("JPEG dimensions exceed maximum image size: %w", err)
		return
	}
	info.f = "DCTDecode"
	info.bpc = 8
	switch config.ColorModel {
	case color.GrayModel:
		info.cs = "DeviceGray"
	case color.YCbCrModel:
		info.cs = "DeviceRGB"
	case color.CMYKModel:
		info.cs = "DeviceCMYK"
	default:
		p.err = fmt.Errorf("image JPEG buffer has unsupported color space (%v)", config.ColorModel)
		return
	}
	return
}
