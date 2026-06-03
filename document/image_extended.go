// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/webp"
)

// ImageCrop describes a rectangular viewport in the rendered image's units.
type ImageCrop struct {
	X float64 // Crop origin X coordinate.
	Y float64 // Crop origin Y coordinate.
	W float64 // Crop width.
	H float64 // Crop height.
}

// ExtendedImageOptions configures additional image placement options.
type ExtendedImageOptions struct {
	X                float64      // Image X coordinate.
	Y                float64      // Image Y coordinate.
	W                float64      // Requested image width.
	H                float64      // Requested image height.
	Flow             bool         // Whether placement advances the current Y position.
	Options          ImageOptions // Image parsing and placement options.
	Link             int          // Internal link identifier.
	LinkString       string       // External link target.
	Rotation         float64      // Clockwise rotation in degrees.
	HorizontalFlip   bool         // Whether to mirror the image horizontally.
	VerticalFlip     bool         // Whether to mirror the image vertically.
	Crop             *ImageCrop   // Optional crop rectangle.
	MaskImage        string       // Optional soft-mask image name or path.
	MaskImageOptions ImageOptions // Options for the soft-mask image.
}

// ImageOptionsExtended places an image with optional rotation, flipping,
// cropping, and an external soft-mask image.
func (f *Document) ImageOptionsExtended(name string, opts ExtendedImageOptions) {
	if f.err != nil {
		return
	}
	info := f.RegisterImageOptions(name, opts.Options)
	if f.err != nil {
		return
	}
	if info == nil {
		f.SetErrorf("image parser returned no image info")
		return
	}
	if opts.MaskImage != "" {
		f.applyExternalImageMask(info, opts.MaskImage, opts.MaskImageOptions)
		if f.err != nil {
			return
		}
	}
	placement, ok := f.resolveImagePlacement(info, opts.X, opts.Y, opts.W, opts.H, opts.Options.AllowNegativePosition, false)
	if !ok {
		return
	}
	draw := placement
	if opts.Crop != nil {
		crop := *opts.Crop
		if crop.W == 0 {
			crop.W = placement.w - crop.X
		}
		if crop.H == 0 {
			crop.H = placement.h - crop.Y
		}
		if crop.X < 0 || crop.Y < 0 || crop.W <= 0 || crop.H <= 0 || !finiteNumbers(crop.X, crop.Y, crop.W, crop.H) {
			f.SetErrorf("invalid image crop rectangle")
			return
		}
		draw.w = crop.W
		draw.h = crop.H
	}
	if opts.Flow {
		if f.y+draw.h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
			x2 := f.x
			f.addPageFormatRotation(f.curOrientation, f.curPageSize, f.curRotation)
			if f.err != nil {
				return
			}
			f.x = x2
		}
		placement.y = f.y
		draw.y = f.y
		f.y += draw.h
	}
	if !finiteNumbers(placement.x, placement.y, placement.w, placement.h, draw.w, draw.h, opts.Rotation) {
		f.SetErrorf("invalid extended image placement")
		return
	}
	transform := opts.Rotation != 0 || opts.HorizontalFlip || opts.VerticalFlip
	if transform {
		f.TransformBegin()
		centerX := draw.x + draw.w/2
		centerY := draw.y + draw.h/2
		if opts.Rotation != 0 {
			f.TransformRotate(opts.Rotation, centerX, centerY)
		}
		if opts.HorizontalFlip || opts.VerticalFlip {
			scaleX := 100.0
			scaleY := 100.0
			if opts.HorizontalFlip {
				scaleX = -100
			}
			if opts.VerticalFlip {
				scaleY = -100
			}
			f.TransformScale(scaleX, scaleY, centerX, centerY)
		}
	}
	if opts.Crop != nil {
		crop := *opts.Crop
		if crop.W == 0 {
			crop.W = draw.w
		}
		if crop.H == 0 {
			crop.H = draw.h
		}
		f.ClipRect(draw.x, draw.y, crop.W, crop.H, false)
		f.drawImageXObject(info.i, placement.x-crop.X, placement.y-crop.Y, placement.w, placement.h)
		f.ClipEnd()
	} else {
		f.drawImageXObject(info.i, placement.x, placement.y, placement.w, placement.h)
	}
	if transform {
		f.TransformEnd()
	}
	if opts.Link > 0 || opts.LinkString != "" {
		f.newLink(draw.x, draw.y, draw.w, draw.h, opts.Link, opts.LinkString)
	}
}

func (f *Document) applyExternalImageMask(info *ImageInfo, maskPath string, options ImageOptions) {
	if info == nil {
		f.SetErrorf("image mask target is missing")
		return
	}
	mask, err := decodeMaskImage(maskPath, options)
	if err != nil {
		f.SetError(err)
		return
	}
	bounds := mask.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width != int(info.w) || height != int(info.h) {
		f.SetErrorf("image mask dimensions %d x %d do not match image %.0f x %.0f", width, height, info.w, info.h)
		return
	}
	var raw bytes.Buffer
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		raw.WriteByte(0)
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			color := mask.At(x, y)
			if color == nil {
				f.SetErrorf("image mask has nil pixel at %d,%d", x, y)
				return
			}
			r, g, b, a := color.RGBA()
			gray := (299*r + 587*g + 114*b) / 1000
			gray = gray * a / 0xffff
			raw.WriteByte(byte(gray >> 8))
		}
	}
	info.smask = f.compressBytes(raw.Bytes())
	if f.err != nil {
		return
	}
	if f.pdfVersion < "1.4" {
		f.pdfVersion = "1.4"
	}
}

func decodeMaskImage(path string, options ImageOptions) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	imageType := strings.ToLower(options.ImageType)
	if imageType == "jpeg" {
		imageType = "jpg"
	}
	if imageType == "" {
		imageType = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		if imageType == "jpeg" {
			imageType = "jpg"
		}
	}
	if imageType == "webp" {
		img, err := webp.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if img == nil {
			return nil, fmt.Errorf("invalid WebP mask image")
		}
		return img, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image mask: %w", err)
	}
	if img == nil {
		return nil, fmt.Errorf("invalid mask image")
	}
	return img, nil
}
