// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
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
func (p *imageParser) parsegif(r io.Reader) (info *ImageInfo) {
	data, err := bufferFromReaderLimit(r, p.sourceLimit)
	if err != nil {
		p.err = err
		return
	}
	config, err := gif.DecodeConfig(bytes.NewReader(data.Bytes()))
	if err != nil {
		p.err = err
		return
	}
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		p.err = fmt.Errorf("GIF dimensions exceed maximum image size: %w", err)
		return
	}
	var img image.Image
	img, err = gif.Decode(bytes.NewReader(data.Bytes()))
	if err != nil {
		p.err = err
		return
	}
	pngBuf := new(bytes.Buffer)
	err = png.Encode(pngBuf, img)
	if err != nil {
		p.err = err
		return
	}
	return p.parsepngstream(pngBuf, false)
}
