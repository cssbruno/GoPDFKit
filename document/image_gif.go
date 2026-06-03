// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"io"
)

// parsegif extracts image information from GIF data via PNG conversion.
func (f *Document) parsegif(r io.Reader) (info *ImageInfo) {
	data, err := bufferFromReaderLimit(r, maxImageSourceBytes)
	if err != nil {
		f.err = err
		return
	}
	config, err := gif.DecodeConfig(bytes.NewReader(data.Bytes()))
	if err != nil {
		f.err = err
		return
	}
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		f.err = fmt.Errorf("GIF dimensions exceed maximum image size: %w", err)
		return
	}
	var img image.Image
	img, err = gif.Decode(bytes.NewReader(data.Bytes()))
	if err != nil {
		f.err = err
		return
	}
	pngBuf := new(bytes.Buffer)
	err = png.Encode(pngBuf, img)
	if err != nil {
		f.err = err
		return
	}
	return f.parsepngstream(pngBuf, false)
}
