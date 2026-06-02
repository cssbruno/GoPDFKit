// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

func (f *Fpdf) pngColorSpace(ct byte) (colspace string, colorVal int) {
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
		f.err = fmt.Errorf("unknown color type in PNG buffer: %d", ct)
	}
	return
}

func (f *Fpdf) parsepngstream(buf *bytes.Buffer, readdpi bool) (info *ImageInfo) {
	info = f.newImageInfo()
	// Check the PNG signature.
	if string(buf.Next(8)) != "\x89PNG\x0d\x0a\x1a\x0a" {
		f.err = fmt.Errorf("not a PNG buffer")
		return
	}
	// Read the header chunk.
	_ = buf.Next(4)
	if string(buf.Next(4)) != "IHDR" {
		f.err = fmt.Errorf("incorrect PNG buffer")
		return
	}
	w := f.readBeInt32(buf)
	h := f.readBeInt32(buf)
	if w <= 0 || h <= 0 {
		f.err = fmt.Errorf("invalid PNG image size: %d x %d", w, h)
		return
	}
	bpc := f.readByte(buf)
	if bpc > 8 {
		f.err = fmt.Errorf("16-bit depth not supported in PNG file")
	}
	ct := f.readByte(buf)
	var colspace string
	var colorVal int
	colspace, colorVal = f.pngColorSpace(ct)
	if f.err != nil {
		return
	}
	if f.readByte(buf) != 0 {
		f.err = fmt.Errorf("'unknown compression method in PNG buffer")
		return
	}
	if f.readByte(buf) != 0 {
		f.err = fmt.Errorf("'unknown filter method in PNG buffer")
		return
	}
	if f.readByte(buf) != 0 {
		f.err = fmt.Errorf("interlacing not supported in PNG buffer")
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
			f.err = fmt.Errorf("incorrect PNG buffer")
			return
		}
		n := int(f.readBeInt32(buf))
		if n < 0 || buf.Len() < n+8 {
			f.err = fmt.Errorf("incorrect PNG chunk length")
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
					f.err = fmt.Errorf("incorrect PNG tRNS chunk length")
					return
				}
				trns = []int{int(chunkData[1])} // ord(substr($t,1,1)));
			case 2:
				if len(chunkData) < 6 {
					f.err = fmt.Errorf("incorrect PNG tRNS chunk length")
					return
				}
				trns = []int{int(chunkData[1]), int(chunkData[3]), int(chunkData[5])} // array(ord(substr($t,1,1)), ord(substr($t,3,1)), ord(substr($t,5,1)));
			default:
				pos := strings.Index(string(chunkData), "\x00")
				if pos >= 0 {
					trns = []int{pos} // array($pos);
				}
			}
		case "IDAT":
			// Read an image data block.
			data = append(data, chunkData...)
		case "IEND":
			loop = false
		case "pHYs":
			// PNG files can theoretically specify different x/y DPI values.
			// Ignore those files, but record the DPI when both values match.
			if len(chunkData) < 9 {
				f.err = fmt.Errorf("incorrect PNG pHYs chunk length")
				return
			}
			chunkBuf := bytes.NewBuffer(chunkData)
			x := int(f.readBeInt32(chunkBuf))
			y := int(f.readBeInt32(chunkBuf))
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
		f.err = fmt.Errorf("missing palette in PNG buffer")
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
			f.err = fmt.Errorf("invalid PNG alpha channel size")
			return
		}
		expectedLen := rowLen * int64(h)
		if expectedLen > maxImageDecodedBytes {
			f.err = fmt.Errorf("PNG alpha channel exceeds maximum decoded size")
			return
		}
		var err error
		data, err = sliceUncompress(data, int(expectedLen))
		if err != nil {
			f.err = err
			return
		}
		var color, alpha []byte
		if ct == 4 {
			// Gray image.
			width := int(w)
			height := int(h)
			length := 2 * width
			if len(data) < (1+length)*height {
				f.err = fmt.Errorf("incorrect PNG alpha channel data")
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
				f.err = fmt.Errorf("incorrect PNG alpha channel data")
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
		data = f.compressBytes(color)
		if f.err != nil {
			return
		}
		info.smask = f.compressBytes(alpha)
		if f.err != nil {
			return
		}
		if f.pdfVersion < "1.4" {
			f.pdfVersion = "1.4"
		}
	}
	info.data = data
	return
}
