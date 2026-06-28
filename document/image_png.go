// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// parsepng extracts image information from PNG data.
func (p *imageParser) parsepng(r io.Reader, readdpi bool) (info *ImageInfo) {
	buf, err := bufferFromReaderLimit(r, p.sourceLimit)
	if err != nil {
		p.err = err
		return
	}
	return p.parsepngstream(buf, readdpi)
}

func (p *imageParser) readBeInt32(r io.Reader) (val int32) {
	err := binary.Read(r, binary.BigEndian, &val)
	if err != nil && !errors.Is(err, io.EOF) {
		p.err = err
	}
	return
}

func (p *imageParser) readByte(r io.Reader) (val byte) {
	err := binary.Read(r, binary.BigEndian, &val)
	if err != nil {
		p.err = err
	}
	return
}

func (p *imageParser) pngColorSpace(ct byte) (colspace string, colorVal int) {
	colorVal = 1
	switch ct {
	case 0, 4:
		colspace = "DeviceGray"
	case 2, 6:
		colspace = "DeviceRGB"
		colorVal = 3
	case 3:
		colspace = "Indexed"
	default:
		p.err = fmt.Errorf("unknown color type in PNG buffer: %d", ct)
	}
	return
}

func (p *imageParser) parsepngstream(buf *bytes.Buffer, readdpi bool) (info *ImageInfo) {
	info = p.newImageInfo()
	// Check the PNG signature.
	if string(buf.Next(8)) != "\x89PNG\x0d\x0a\x1a\x0a" {
		p.err = errors.New("not a PNG buffer")
		return
	}
	// Read the header chunk.
	_ = buf.Next(4)
	if string(buf.Next(4)) != "IHDR" {
		p.err = errors.New("incorrect PNG buffer")
		return
	}
	w := p.readBeInt32(buf)
	h := p.readBeInt32(buf)
	if w <= 0 || h <= 0 {
		p.err = fmt.Errorf("invalid PNG image size: %d x %d", w, h)
		return
	}
	if err := validateImageDimensions(int(w), int(h)); err != nil {
		p.err = fmt.Errorf("PNG dimensions exceed maximum image size: %w", err)
		return
	}
	bpc := p.readByte(buf)
	ct := p.readByte(buf)
	if !validPNGBitDepthForColorType(bpc, ct) {
		if bpc > 8 {
			p.err = errors.New("16-bit depth not supported in PNG file")
			return
		}
		p.err = fmt.Errorf("unsupported PNG bit depth %d for color type %d", bpc, ct)
		return
	}
	var colspace string
	var colorVal int
	colspace, colorVal = p.pngColorSpace(ct)
	if p.err != nil {
		return
	}
	if p.readByte(buf) != 0 {
		p.err = errors.New("'unknown compression method in PNG buffer")
		return
	}
	if p.readByte(buf) != 0 {
		p.err = errors.New("'unknown filter method in PNG buffer")
		return
	}
	if p.readByte(buf) != 0 {
		p.err = errors.New("interlacing not supported in PNG buffer")
		return
	}
	_ = buf.Next(4)
	dp := sprintf("/Predictor 15 /Colors %d /BitsPerComponent %d /Columns %d", colorVal, bpc, w)
	// Scan chunks looking for palette, transparency, and image data.
	pal := make([]byte, 0, 32)
	var trns []int
	data := make([]byte, 0, 32)
	loop := true
	for loop {
		if buf.Len() < 8 {
			p.err = errors.New("incorrect PNG buffer")
			return
		}
		n := int(p.readBeInt32(buf))
		if n < 0 || buf.Len() < n+8 {
			p.err = errors.New("incorrect PNG chunk length")
			return
		}
		chunkType := string(buf.Next(4))
		chunkData := buf.Next(n)
		_ = buf.Next(4)
		switch chunkType {
		case "PLTE":
			// Read the palette.
			pal = chunkData
		case "tRNS":
			// Read transparency information.
			switch ct {
			case 0:
				if len(chunkData) < 2 {
					p.err = errors.New("incorrect PNG tRNS chunk length")
					return
				}
				trns = []int{int(chunkData[1])} // ord(substr($t,1,1)));
			case 2:
				if len(chunkData) < 6 {
					p.err = errors.New("incorrect PNG tRNS chunk length")
					return
				}
				trns = []int{int(chunkData[1]), int(chunkData[3]), int(chunkData[5])} // array(ord(substr($t,1,1)), ord(substr($t,3,1)));
			default:
				pos := strings.Index(string(chunkData), "\x00")
				if pos >= 0 {
					trns = []int{pos} // array($pos);
				}
			}
		case "IDAT":
			// Read an image data block.
			data = append(data, chunkData...)
			if len(data) > p.sourceLimit {
				p.err = errors.New("PNG image data exceeds maximum size")
				return
			}
		case "IEND":
			loop = false
		case "pHYs":
			// PNG files can theoretically specify different x/y DPI values.
			// Ignore those files, but record the DPI when both values match.
			if len(chunkData) < 9 {
				p.err = errors.New("incorrect PNG pHYs chunk length")
				return
			}
			chunkBuf := bytes.NewBuffer(chunkData)
			x := int(p.readBeInt32(chunkBuf))
			y := int(p.readBeInt32(chunkBuf))
			units := chunkBuf.Next(1)[0]
			// Only modify the info block when the caller requested DPI metadata.
			if x == y && readdpi {
				switch units {
				// Unit value 1 means pixels per meter.
				case 1:
					info.dpi = float64(x) / 39.3701 // Pixels per inch.
				default:
					info.dpi = float64(x)
				}
			}
		}
		if loop {
			loop = n > 0
		}
	}
	if colspace == "Indexed" && len(pal) == 0 {
		p.err = errors.New("missing palette in PNG buffer")
	}
	if len(data) == 0 {
		p.err = errors.New("missing image data in PNG buffer")
		return
	}
	info.w = float64(w)
	info.h = float64(h)
	info.cs = colspace
	info.bpc = int(bpc)
	info.f = "FlateDecode"
	info.dp = dp
	info.pal = pal
	info.trns = trns
	if ct >= 4 {
		// Separate alpha and color channels.
		bytesPerPixel := int64(2)
		if ct == 6 {
			bytesPerPixel = 4
		}
		rowLen := 1 + bytesPerPixel*int64(w)
		if rowLen <= 0 || int64(h) > int64(math.MaxInt)/rowLen {
			p.err = errors.New("invalid PNG alpha channel size")
			return
		}
		expectedLen := rowLen * int64(h)
		if expectedLen > int64(p.decodedLimit) {
			p.err = errors.New("PNG alpha channel exceeds maximum decoded size")
			return
		}
		var err error
		data, err = sliceUncompress(data, int(expectedLen))
		if err != nil {
			p.err = err
			return
		}
		var color, alpha []byte
		if ct == 4 {
			// Gray image.
			width := int(w)
			height := int(h)
			length := 2 * width
			if len(data) < (1+length)*height {
				p.err = errors.New("incorrect PNG alpha channel data")
				return
			}
			color = make([]byte, 0, height*(1+width))
			alpha = make([]byte, 0, height*(1+width))
			var pos, elPos int
			for i := range height {
				pos = (1 + length) * i
				color = append(color, data[pos])
				alpha = append(alpha, data[pos])
				elPos = pos + 1
				for range width {
					color = append(color, data[elPos])
					alpha = append(alpha, data[elPos+1])
					elPos += 2
				}
			}
		} else {
			// RGB image.
			width := int(w)
			height := int(h)
			length := 4 * width
			if len(data) < (1+length)*height {
				p.err = errors.New("incorrect PNG alpha channel data")
				return
			}
			color = make([]byte, 0, height*(1+3*width))
			alpha = make([]byte, 0, height*(1+width))
			var pos, elPos int
			for i := range height {
				pos = (1 + length) * i
				color = append(color, data[pos])
				alpha = append(alpha, data[pos])
				elPos = pos + 1
				for range width {
					color = append(color, data[elPos:elPos+3]...)
					alpha = append(alpha, data[elPos+3])
					elPos += 4
				}
			}
		}
		data = p.compressBytes(color)
		if p.err != nil {
			return
		}
		info.smask = p.compressBytes(alpha)
		if p.err != nil {
			return
		}
		if pdfVersionLess(p.pdfVersion, "1.4") {
			p.pdfVersion = "1.4"
		}
	}
	info.data = data
	return
}

func validPNGBitDepthForColorType(bpc, ct byte) bool {
	switch ct {
	case 0, 3:
		return bpc == 1 || bpc == 2 || bpc == 4 || bpc == 8
	case 2, 4, 6:
		return bpc == 8
	default:
		return false
	}
}
