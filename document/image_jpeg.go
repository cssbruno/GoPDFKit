// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"image/jpeg"
	"io"
)

// parsejpg extracts image information from an io.Reader containing JPEG data.
// Thank you, Bruno Michel, for providing this code.
func (f *Document) parsejpg(r io.Reader) (info *ImageInfo) {
	info = f.newImageInfo()
	var (
		data bytes.Buffer
		err  error
	)
	_, err = data.ReadFrom(io.LimitReader(r, maxImageSourceBytes+1))
	if err != nil {
		f.err = err
		return
	}
	if data.Len() > maxImageSourceBytes {
		f.err = errors.New("image data exceeds maximum size")
		return
	}
	info.data = data.Bytes()
	config, err := jpeg.DecodeConfig(bytes.NewReader(info.data))
	if err != nil {
		f.err = err
		return
	}
	info.w = float64(config.Width)
	info.h = float64(config.Height)
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		f.err = fmt.Errorf("JPEG dimensions exceed maximum image size: %w", err)
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
		f.err = fmt.Errorf("image JPEG buffer has unsupported color space (%v)", config.ColorModel)
		return
	}
	return
}
