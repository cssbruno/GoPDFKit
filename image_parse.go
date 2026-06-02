/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/webp"
)

// parsejpg extracts info from io.Reader with JPEG data
// Thank you, Bruno Michel, for providing this code.

func (f *Fpdf) parsejpg(r io.Reader) (info *ImageInfo) {
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
		f.err = fmt.Errorf("image data exceeds maximum size")
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
	if !validImagePixelCount(config.Width, config.Height) {
		f.err = fmt.Errorf("JPEG dimensions exceed maximum image size")
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

// parsepng extracts info from a PNG data

func (f *Fpdf) parsepng(r io.Reader, readdpi bool) (info *ImageInfo) {
	buf, err := bufferFromReaderLimit(r, maxImageSourceBytes)
	if err != nil {
		f.err = err
		return
	}
	return f.parsepngstream(buf, readdpi)
}

func (f *Fpdf) readBeInt32(r io.Reader) (val int32) {
	err := binary.Read(r, binary.BigEndian, &val)
	if err != nil && err != io.EOF {
		f.err = err
	}
	return
}

func (f *Fpdf) readByte(r io.Reader) (val byte) {
	err := binary.Read(r, binary.BigEndian, &val)
	if err != nil {
		f.err = err
	}
	return
}

// parsegif extracts info from a GIF data (via PNG conversion)

func (f *Fpdf) parsegif(r io.Reader) (info *ImageInfo) {
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
	if !validImagePixelCount(config.Width, config.Height) {
		f.err = fmt.Errorf("GIF dimensions exceed maximum image size")
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

// parsewebp extracts info from a WebP image by converting it to PNG first.

func (f *Fpdf) parsewebp(r io.Reader) (info *ImageInfo) {
	data, err := bufferFromReaderLimit(r, maxImageSourceBytes)
	if err != nil {
		f.err = err
		return
	}
	config, err := webp.DecodeConfig(bytes.NewReader(data.Bytes()))
	if err != nil {
		f.err = err
		return
	}
	if !validImagePixelCount(config.Width, config.Height) {
		f.err = fmt.Errorf("WebP dimensions exceed maximum image size")
		return
	}
	img, err := webp.Decode(bytes.NewReader(data.Bytes()))
	if err != nil {
		f.err = err
		return
	}
	bounds := img.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	pngBuf := new(bytes.Buffer)
	if err = png.Encode(pngBuf, rgba); err != nil {
		f.err = err
		return
	}
	return f.parsepngstream(pngBuf, false)
}
