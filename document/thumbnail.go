// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // Register GIF decoding with image.Decode.
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"
)

const (
	// ThumbnailFormatPNG encodes thumbnails as PNG.
	ThumbnailFormatPNG = "png"
	// ThumbnailFormatJPEG encodes thumbnails as JPEG.
	ThumbnailFormatJPEG = "jpg"

	defaultMaxSize     = 256
	defaultJPEGQuality = 85
)

// ThumbnailOptions configures thumbnail generation.
type ThumbnailOptions struct {
	// MaxWidth is the maximum thumbnail width in pixels. If both MaxWidth and
	// MaxHeight are zero, both default to 256.
	MaxWidth int
	// MaxHeight is the maximum thumbnail height in pixels. If both MaxWidth and
	// MaxHeight are zero, both default to 256.
	MaxHeight int
	// Format may be "png", "jpg", "jpeg", or empty/"auto". Auto keeps JPEG
	// sources as JPEG and encodes all other sources as PNG.
	Format string
	// JPEGQuality controls JPEG output quality. Zero uses the default quality 85.
	JPEGQuality int
	// Upscale permits thumbnails larger than the source image.
	Upscale bool
}

type pdfImageRegistrar interface {
	RegisterImageOptionsReader(imgName string, options ImageOptions, r io.Reader) *ImageInfo
	SetError(err error)
}

// GenerateThumbnail decodes an image from r and returns encoded thumbnail bytes
// plus the gopdfkit image type string.
func GenerateThumbnail(r io.Reader, options ThumbnailOptions) ([]byte, string, error) {
	if r == nil {
		return nil, "", errors.New("thumbnail source reader is nil")
	}
	data, err := bufferFromReaderLimit(r, maxImageSourceBytes)
	if err != nil {
		return nil, "", err
	}
	config, sourceFormat, err := image.DecodeConfig(bytes.NewReader(data.Bytes()))
	if err != nil {
		return nil, "", fmt.Errorf("decode thumbnail source config: %w", err)
	}
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		return nil, "", fmt.Errorf("thumbnail dimensions exceed maximum image size: %w", err)
	}
	src, sourceFormat, err := image.Decode(bytes.NewReader(data.Bytes()))
	if err != nil {
		return nil, "", fmt.Errorf("decode thumbnail source: %w", err)
	}
	return GenerateThumbnailImage(src, sourceFormat, options)
}

// GenerateThumbnailImage returns encoded thumbnail bytes for src plus the
// gopdfkit image type string.
func GenerateThumbnailImage(src image.Image, sourceFormat string, options ThumbnailOptions) ([]byte, string, error) {
	if src == nil {
		return nil, "", errors.New("thumbnail source image is nil")
	}
	format, quality, err := normalizeEncodeOptions(sourceFormat, options)
	if err != nil {
		return nil, "", err
	}
	resized, err := resize(src, options)
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	switch format {
	case ThumbnailFormatPNG:
		err = png.Encode(&buf, resized)
	case ThumbnailFormatJPEG:
		err = jpeg.Encode(&buf, flattenToRGB(resized), &jpeg.Options{Quality: quality})
	default:
		err = fmt.Errorf("unsupported thumbnail format: %s", format)
	}
	if err != nil {
		return nil, "", fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), format, nil
}

// RegisterThumbnail creates a thumbnail from r and registers it with pdf under
// name.
func RegisterThumbnail(pdf pdfImageRegistrar, name string, r io.Reader, options ThumbnailOptions) (*ImageInfo, error) {
	if pdf == nil {
		return nil, errors.New("pdf registrar is nil")
	}
	if strings.TrimSpace(name) == "" {
		err := errors.New("thumbnail image name is empty")
		pdf.SetError(err)
		return nil, err
	}
	data, format, err := GenerateThumbnail(r, options)
	if err != nil {
		pdf.SetError(err)
		return nil, err
	}
	return pdf.RegisterImageOptionsReader(name, ImageOptions{ImageType: format}, bytes.NewReader(data)), nil
}

// RegisterThumbnail creates a thumbnail from r and registers it on this PDF
// document under name.
func (f *Document) RegisterThumbnail(name string, r io.Reader, options ThumbnailOptions) (*ImageInfo, error) {
	return RegisterThumbnail(f, name, r, options)
}

func resize(src image.Image, options ThumbnailOptions) (image.Image, error) {
	if src == nil {
		return nil, errors.New("thumbnail source image is nil")
	}
	width, height, err := targetSize(src.Bounds(), options)
	if err != nil {
		return nil, err
	}
	return resizeBilinear(src, width, height), nil
}

func normalizeEncodeOptions(sourceFormat string, options ThumbnailOptions) (format string, jpegQuality int, err error) {
	if options.JPEGQuality < 0 || options.JPEGQuality > 100 {
		return "", 0, errors.New("thumbnail JPEG quality must be between 0 and 100")
	}
	jpegQuality = options.JPEGQuality
	if jpegQuality == 0 {
		jpegQuality = defaultJPEGQuality
	}

	format = strings.ToLower(strings.TrimSpace(options.Format))
	format = strings.TrimPrefix(format, "image/")
	switch format {
	case "", "auto":
		switch strings.ToLower(sourceFormat) {
		case "jpg", "jpeg":
			format = ThumbnailFormatJPEG
		default:
			format = ThumbnailFormatPNG
		}
	case "jpg", "jpeg":
		format = ThumbnailFormatJPEG
	case "png":
		format = ThumbnailFormatPNG
	default:
		return "", 0, fmt.Errorf("unsupported thumbnail format: %s", options.Format)
	}
	return format, jpegQuality, nil
}

func targetSize(bounds image.Rectangle, options ThumbnailOptions) (width, height int, err error) {
	if options.MaxWidth < 0 || options.MaxHeight < 0 {
		return 0, 0, errors.New("thumbnail dimensions must be non-negative")
	}
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return 0, 0, errors.New("thumbnail source image has invalid dimensions")
	}

	maxWidth := options.MaxWidth
	maxHeight := options.MaxHeight
	if maxWidth == 0 && maxHeight == 0 {
		maxWidth = defaultMaxSize
		maxHeight = defaultMaxSize
	}

	scale := math.Inf(1)
	if maxWidth > 0 {
		scale = math.Min(scale, float64(maxWidth)/float64(sourceWidth))
	}
	if maxHeight > 0 {
		scale = math.Min(scale, float64(maxHeight)/float64(sourceHeight))
	}
	if math.IsInf(scale, 1) {
		scale = 1
	}
	if !options.Upscale && scale > 1 {
		scale = 1
	}

	width = max(1, int(math.Round(float64(sourceWidth)*scale)))
	height = max(1, int(math.Round(float64(sourceHeight)*scale)))
	if maxWidth > 0 && width > maxWidth {
		width = maxWidth
		height = max(1, int(math.Round(float64(sourceHeight)*float64(width)/float64(sourceWidth))))
	}
	if maxHeight > 0 && height > maxHeight {
		height = maxHeight
		width = max(1, int(math.Round(float64(sourceWidth)*float64(height)/float64(sourceHeight))))
	}
	return width, height, nil
}

func resizeBilinear(src image.Image, width, height int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	sourceBounds := src.Bounds()
	sourceWidth := sourceBounds.Dx()
	sourceHeight := sourceBounds.Dy()
	if width == sourceWidth && height == sourceHeight {
		draw.Draw(dst, dst.Bounds(), src, sourceBounds.Min, draw.Src)
		return dst
	}

	for y := range height {
		sourceY := (float64(y)+0.5)*float64(sourceHeight)/float64(height) - 0.5
		y0 := int(math.Floor(sourceY))
		weightY := sourceY - float64(y0)
		if y0 < 0 {
			y0 = 0
			weightY = 0
		}
		y1 := min(y0+1, sourceHeight-1)

		for x := range width {
			sourceX := (float64(x)+0.5)*float64(sourceWidth)/float64(width) - 0.5
			x0 := int(math.Floor(sourceX))
			weightX := sourceX - float64(x0)
			if x0 < 0 {
				x0 = 0
				weightX = 0
			}
			x1 := min(x0+1, sourceWidth-1)

			c00 := nrgbaAt(src, sourceBounds.Min.X+x0, sourceBounds.Min.Y+y0)
			c10 := nrgbaAt(src, sourceBounds.Min.X+x1, sourceBounds.Min.Y+y0)
			c01 := nrgbaAt(src, sourceBounds.Min.X+x0, sourceBounds.Min.Y+y1)
			c11 := nrgbaAt(src, sourceBounds.Min.X+x1, sourceBounds.Min.Y+y1)
			dst.SetNRGBA(x, y, mixColors(c00, c10, c01, c11, weightX, weightY))
		}
	}
	return dst
}

func nrgbaAt(src image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
}

func mixColors(c00, c10, c01, c11 color.NRGBA, weightX, weightY float64) color.NRGBA {
	top := mixColor(c00, c10, weightX)
	bottom := mixColor(c01, c11, weightX)
	return mixColor(top, bottom, weightY)
}

func mixColor(left, right color.NRGBA, weight float64) color.NRGBA {
	return color.NRGBA{
		R: mixChannel(left.R, right.R, weight),
		G: mixChannel(left.G, right.G, weight),
		B: mixChannel(left.B, right.B, weight),
		A: mixChannel(left.A, right.A, weight),
	}
}

func mixChannel(left, right uint8, weight float64) uint8 {
	return uint8(math.Round(float64(left)*(1-weight) + float64(right)*weight))
}

func flattenToRGB(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Over)
	return dst
}
