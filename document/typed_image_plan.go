// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

type paperMeasuredImage struct {
	resource      layoutengine.ImageResource
	encoded       []byte
	width         layoutengine.Fixed
	height        layoutengine.Fixed
	contentWidth  layoutengine.Fixed
	contentHeight layoutengine.Fixed
	insets        [4]layoutengine.Fixed
	margins       [4]layoutengine.Fixed
	background    layoutengine.CoreRGBColor
	borders       [4]typedTableBorder
	fit           layout.ImageFitMode
	focusX        float64
	focusY        float64
	align         string
}

func validateTypedPlanningImage(block layout.ImageBlock, path string) error {
	box := block.EffectiveBox()
	plain := box
	plain.KeepTogether, plain.KeepWithNext, plain.Orphans, plain.Widows = false, false, 0, 0
	if box.KeepWithNext {
		return fmt.Errorf("%s: image keep-with-next is unsupported", path)
	}
	if block.Source != "" {
		return fmt.Errorf("%s.source: ambient file or registered-name image sources are unsupported; use Data or DataRef", path)
	}
	if len(block.ImageData()) == 0 {
		return fmt.Errorf("%s.data: inline image bytes are required", path)
	}
	switch strings.ToLower(strings.TrimSpace(block.Format)) {
	case "png", "jpg", "jpeg":
	default:
		return fmt.Errorf("%s.format: %q is unsupported; use png, jpg, or jpeg", path, block.Format)
	}
	if block.Fit != layout.ImageFitAuto && block.Fit != layout.ImageFitContain && block.Fit != layout.ImageFitCover {
		return fmt.Errorf("%s.fit: %q is unsupported", path, block.Fit)
	}
	if block.FocusSet && (!finiteNumbers(block.FocusX, block.FocusY) || block.FocusX < 0 || block.FocusX > 1 || block.FocusY < 0 || block.FocusY > 1) {
		return fmt.Errorf("%s.focus: image focus must be finite and between 0 and 1", path)
	}
	if !finiteNumbers(block.Width, block.Height, block.MaxWidth, block.MaxHeight, block.DPI) ||
		block.Width < 0 || block.Height < 0 || block.MaxWidth < 0 || block.MaxHeight < 0 || block.DPI < 0 {
		return fmt.Errorf("%s: image dimensions and DPI must be finite and non-negative", path)
	}
	if block.WidthPercent > 100_000_000 || block.MaxWidthPercent > 100_000_000 ||
		block.Width > 0 && block.WidthPercent != 0 || block.MaxWidth > 0 && block.MaxWidthPercent != 0 {
		return fmt.Errorf("%s: image width constraints must choose one fixed or container-relative value from 0%% through 100%%", path)
	}
	if block.DPI != 0 {
		return fmt.Errorf("%s.dpi: explicit DPI overrides are unsupported by the exact image plan", path)
	}
	switch strings.ToLower(strings.TrimSpace(block.Align)) {
	case "", "l", "left", "c", "center", "r", "right":
	default:
		return fmt.Errorf("%s.align: %q is unsupported", path, block.Align)
	}
	if _, _, _, err := typedImageBoxDecoration(box, path, func(value float64) float64 { return value }); err != nil {
		return err
	}
	if _, err := typedImageMargins(box, path, func(value float64) float64 { return value }); err != nil {
		return err
	}
	return nil
}

func (f *Document) measureTypedPlanningImageContext(ctx context.Context, block layout.ImageBlock, contentWidth float64) (paperMeasuredImage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return paperMeasuredImage{}, err
	}
	data := append([]byte(nil), block.ImageData()...)
	format := strings.ToLower(strings.TrimSpace(block.Format))
	engineFormat := layoutengine.ImagePNG
	imageType := "png"
	if format == "jpg" || format == "jpeg" {
		engineFormat, imageType = layoutengine.ImageJPEG, "jpg"
	}
	info, _, err := parseImageOptionsReaderWithLimitsContext(ctx,
		ImageOptions{ImageType: imageType, ReadDpi: block.DPI > 0}, bytes.NewReader(data),
		f.k, f.compressLevel, f.pdfVersion, f.imageSourceLimit(), f.imageDecodedLimit(),
	)
	if err != nil {
		return paperMeasuredImage{}, fmt.Errorf("decode inline %s image: %w", imageType, err)
	}
	if info == nil || info.w <= 0 || info.h <= 0 || info.w > math.MaxUint32 || info.h > math.MaxUint32 ||
		math.Trunc(info.w) != info.w || math.Trunc(info.h) != info.h {
		return paperMeasuredImage{}, errors.New("decoded image has invalid intrinsic pixel dimensions")
	}
	insets, background, borders, err := typedImageBoxDecoration(block.EffectiveBox(), "image", f.UnitToPointConvert)
	if err != nil {
		return paperMeasuredImage{}, err
	}
	margins, err := typedImageMargins(block.EffectiveBox(), "image", f.UnitToPointConvert)
	if err != nil {
		return paperMeasuredImage{}, err
	}
	horizontal := insets[1].Points() + insets[3].Points() + margins[1].Points() + margins[3].Points()
	availableContentWidth := f.PointConvert(f.UnitToPointConvert(contentWidth) - horizontal)
	if availableContentWidth <= 0 {
		return paperMeasuredImage{}, errors.New("image box decorations leave no content width")
	}
	percentWidth, percentErr := f.typedContainerPercentUnits(availableContentWidth, block.WidthPercent)
	percentMaximum, maximumErr := f.typedContainerPercentUnits(availableContentWidth, block.MaxWidthPercent)
	if percentErr != nil || maximumErr != nil {
		return paperMeasuredImage{}, errors.New("image percentage width is outside the representable range")
	}
	width := layout.FirstPositive(percentWidth, block.Width, percentMaximum, block.MaxWidth, availableContentWidth)
	maximumWidth := layout.FirstPositive(percentMaximum, block.MaxWidth)
	if maximumWidth > 0 && width > maximumWidth {
		width = maximumWidth
	}
	if width > availableContentWidth {
		width = availableContentWidth
	}
	intrinsicHeight := width * info.h / info.w
	height := layout.FirstPositive(block.Height, block.MaxHeight, intrinsicHeight)
	if block.MaxHeight > 0 && height > block.MaxHeight {
		height = block.MaxHeight
	}
	widthFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(width))
	if err != nil || widthFixed <= 0 {
		return paperMeasuredImage{}, errors.New("resolved image width is invalid")
	}
	heightFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(height))
	if err != nil || heightFixed <= 0 {
		return paperMeasuredImage{}, errors.New("resolved image height is invalid")
	}
	digest := sha256.Sum256(data)
	outerWidth, err := widthFixed.Add(insets[1])
	if err == nil {
		outerWidth, err = outerWidth.Add(insets[3])
	}
	outerHeight, heightErr := heightFixed.Add(insets[0])
	if heightErr == nil {
		outerHeight, heightErr = outerHeight.Add(insets[2])
	}
	if err != nil || heightErr != nil {
		return paperMeasuredImage{}, errors.New("resolved decorated image size overflows")
	}
	flowWidth, flowErr := outerWidth.Add(margins[1])
	if flowErr == nil {
		flowWidth, flowErr = flowWidth.Add(margins[3])
	}
	flowHeight, flowHeightErr := outerHeight.Add(margins[0])
	if flowHeightErr == nil {
		flowHeight, flowHeightErr = flowHeight.Add(margins[2])
	}
	if flowErr != nil || flowHeightErr != nil || flowWidth <= 0 || flowHeight <= 0 {
		return paperMeasuredImage{}, errors.New("resolved image margin box overflows")
	}
	return paperMeasuredImage{
		resource: layoutengine.ImageResource{
			Digest: layoutengine.ImageContentDigest(hex.EncodeToString(digest[:])), Format: engineFormat,
			PixelWidth: uint32(info.w), PixelHeight: uint32(info.h),
		},
		encoded: data, width: outerWidth, height: outerHeight,
		contentWidth: widthFixed, contentHeight: heightFixed, insets: insets, margins: margins,
		background: background, borders: borders, fit: block.Fit,
		focusX: imageFocus(block.FocusX, block.FocusSet), focusY: imageFocus(block.FocusY, block.FocusSet),
		align: strings.ToLower(strings.TrimSpace(block.Align)),
	}, nil
}

func typedImageMargins(box layout.BoxStyle, path string, toPoints func(float64) float64) ([4]layoutengine.Fixed, error) {
	values := []float64{box.Margin.Top, box.Margin.Right, box.Margin.Bottom, box.Margin.Left}
	var result [4]layoutengine.Fixed
	for index, value := range values {
		fixed, err := layoutengine.FixedFromPoints(toPoints(value))
		if err != nil || fixed < 0 {
			return result, fmt.Errorf("%s.box.margin: values must be finite and non-negative", path)
		}
		result[index] = fixed
	}
	return result, nil
}

func imageFocus(value float64, set bool) float64 {
	if !set {
		return 0.5
	}
	return value
}

func typedImageBoxDecoration(box layout.BoxStyle, path string, toPoints func(float64) float64) ([4]layoutengine.Fixed, layoutengine.CoreRGBColor, [4]typedTableBorder, error) {
	var insets [4]layoutengine.Fixed
	var borders [4]typedTableBorder
	padding := []float64{box.Padding.Top, box.Padding.Right, box.Padding.Bottom, box.Padding.Left}
	sides := []layout.BorderSide{box.Border.Top, box.Border.Right, box.Border.Bottom, box.Border.Left}
	for index := range insets {
		fixed, err := layoutengine.FixedFromPoints(toPoints(padding[index]))
		if err != nil || fixed < 0 {
			return insets, layoutengine.CoreRGBColor{}, borders, fmt.Errorf("%s.box.padding: values must be finite and non-negative", path)
		}
		insets[index] = fixed
		side := sides[index]
		if side.Width == 0 && side.Style == "" && !side.Color.Set {
			continue
		}
		style := strings.ToLower(strings.TrimSpace(side.Style))
		if side.Width <= 0 || (style != "" && style != "solid") {
			return insets, layoutengine.CoreRGBColor{}, borders, fmt.Errorf("%s.box.border: visible borders require a positive width and optional solid style", path)
		}
		width, err := layoutengine.FixedFromPoints(toPoints(side.Width))
		if err != nil || width <= 0 {
			return insets, layoutengine.CoreRGBColor{}, borders, fmt.Errorf("%s.box.border: width must be finite and positive", path)
		}
		colorValue := side.Color
		if !colorValue.Set {
			colorValue.Set = true
		}
		color, err := typedImageColor(colorValue, path+".box.border.color")
		if err != nil {
			return insets, layoutengine.CoreRGBColor{}, borders, err
		}
		borders[index] = typedTableBorder{width: width, color: color}
		insets[index], err = insets[index].Add(width)
		if err != nil {
			return insets, layoutengine.CoreRGBColor{}, borders, fmt.Errorf("%s.box: decoration size overflows", path)
		}
	}
	background, err := typedImageColor(box.BackgroundColor, path+".box.background")
	return insets, background, borders, err
}

func typedImageColor(color layout.DocumentColor, path string) (layoutengine.CoreRGBColor, error) {
	if !color.Set {
		return layoutengine.CoreRGBColor{}, nil
	}
	if color.R < 0 || color.R > 255 || color.G < 0 || color.G > 255 || color.B < 0 || color.B > 255 {
		return layoutengine.CoreRGBColor{}, fmt.Errorf("%s: RGB components must be between 0 and 255", path)
	}
	return layoutengine.CoreRGBColor{R: uint8(color.R), G: uint8(color.G), B: uint8(color.B), Set: true}, nil
}

func (image paperMeasuredImage) targetX(body layoutengine.Rect) (layoutengine.Fixed, error) {
	remaining, err := body.Width.Sub(image.flowWidth())
	if err != nil || remaining < 0 {
		return 0, errors.New("resolved image width exceeds the body")
	}
	offset := layoutengine.Fixed(0)
	switch image.align {
	case "c", "center":
		offset = remaining / 2
	case "r", "right":
		offset = remaining
	}
	return body.X.Add(offset)
}

func (image paperMeasuredImage) flowWidth() layoutengine.Fixed {
	return image.width + image.margins[1] + image.margins[3]
}

func (image paperMeasuredImage) flowHeight() layoutengine.Fixed {
	return image.height + image.margins[0] + image.margins[2]
}

func (image paperMeasuredImage) boxes(x, y layoutengine.Fixed) (layoutengine.Rect, layoutengine.Rect, error) {
	marginBox, err := layoutengine.NewRect(x, y, image.flowWidth(), image.flowHeight())
	if err != nil {
		return layoutengine.Rect{}, layoutengine.Rect{}, err
	}
	borderX, err := x.Add(image.margins[3])
	if err != nil {
		return layoutengine.Rect{}, layoutengine.Rect{}, err
	}
	borderY, err := y.Add(image.margins[0])
	if err != nil {
		return layoutengine.Rect{}, layoutengine.Rect{}, err
	}
	borderBox, err := layoutengine.NewRect(borderX, borderY, image.width, image.height)
	return marginBox, borderBox, err
}

func (image paperMeasuredImage) contentBox(outer layoutengine.Rect) (layoutengine.Rect, error) {
	x, err := outer.X.Add(image.insets[3])
	if err != nil {
		return layoutengine.Rect{}, err
	}
	y, err := outer.Y.Add(image.insets[0])
	if err != nil {
		return layoutengine.Rect{}, err
	}
	return layoutengine.NewRect(x, y, image.contentWidth, image.contentHeight)
}

func (image paperMeasuredImage) placement(fragment layoutengine.FragmentID, target layoutengine.Rect) (layoutengine.PlannedImage, error) {
	var err error
	target, err = image.contentBox(target)
	if err != nil {
		return layoutengine.PlannedImage{}, err
	}
	result := layoutengine.PlannedImage{Resource: image.resource.ID, Fragment: fragment, Bounds: target}
	if image.fit == layout.ImageFitAuto {
		return result, nil
	}
	fitted := layout.FitImage(float64(image.resource.PixelWidth), float64(image.resource.PixelHeight),
		target.Width.Points(), target.Height.Points(), image.fit)
	if fitted.Width <= 0 || fitted.Height <= 0 {
		return layoutengine.PlannedImage{}, errors.New("image fit produced empty geometry")
	}
	if image.fit == layout.ImageFitContain {
		xOffset, _ := layoutengine.FixedFromPoints(fitted.OffsetX)
		yOffset, _ := layoutengine.FixedFromPoints(fitted.OffsetY)
		width, _ := layoutengine.FixedFromPoints(fitted.Width)
		height, _ := layoutengine.FixedFromPoints(fitted.Height)
		x, err := target.X.Add(xOffset)
		if err != nil {
			return layoutengine.PlannedImage{}, err
		}
		y, err := target.Y.Add(yOffset)
		if err != nil {
			return layoutengine.PlannedImage{}, err
		}
		result.Bounds, err = layoutengine.NewRect(x, y, width, height)
		return result, err
	}
	intrinsicW := layoutengine.Fixed(uint64(image.resource.PixelWidth) * 1024)
	intrinsicH := layoutengine.Fixed(uint64(image.resource.PixelHeight) * 1024)
	scale := math.Max(target.Width.Points()/float64(image.resource.PixelWidth), target.Height.Points()/float64(image.resource.PixelHeight))
	sourceW := float64(image.resource.PixelWidth) * target.Width.Points() / (float64(image.resource.PixelWidth) * scale)
	sourceH := float64(image.resource.PixelHeight) * target.Height.Points() / (float64(image.resource.PixelHeight) * scale)
	sourceX := (float64(image.resource.PixelWidth) - sourceW) * image.focusX
	sourceY := (float64(image.resource.PixelHeight) - sourceH) * image.focusY
	toIntrinsic := func(value float64) layoutengine.Fixed { return layoutengine.Fixed(math.Round(value * 1024)) }
	intrinsic, err := layoutengine.NewSize(intrinsicW, intrinsicH)
	if err != nil {
		return layoutengine.PlannedImage{}, err
	}
	source, err := layoutengine.NewRect(toIntrinsic(sourceX), toIntrinsic(sourceY), toIntrinsic(sourceW), toIntrinsic(sourceH))
	if err != nil {
		return layoutengine.PlannedImage{}, err
	}
	result.Crop = &layoutengine.ImageCrop{Intrinsic: intrinsic, Source: source, Clip: target}
	return result, nil
}

const maxPlannedImageResources = 1 << 18

func typedLayoutImageSourcesContext(ctx context.Context, doc *layout.LayoutDocument, maxBytes uint64) (plannedImageSources, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(plannedImageSources)
	if doc == nil {
		return result, nil
	}
	var total uint64
	add := func(data []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		hash := sha256.New()
		for offset := 0; offset < len(data); offset += 64 << 10 {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := offset + (64 << 10)
			if end > len(data) {
				end = len(data)
			}
			_, _ = hash.Write(data[offset:end])
		}
		key := layoutengine.ImageContentDigest(hex.EncodeToString(hash.Sum(nil)))
		if _, exists := result[key]; exists {
			return nil
		}
		if uint64(len(data)) > maxBytes-total {
			return fmt.Errorf("document: cumulative planned image source bytes exceed limit: %d > %d", total+uint64(len(data)), maxBytes)
		}
		if len(result) >= maxPlannedImageResources {
			return fmt.Errorf("document: planned image resource count exceeds limit: %d", maxPlannedImageResources)
		}
		copyOf := make([]byte, len(data))
		for offset := 0; offset < len(data); offset += 64 << 10 {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := offset + (64 << 10)
			if end > len(data) {
				end = len(data)
			}
			copy(copyOf[offset:end], data[offset:end])
		}
		result[key] = copyOf
		total += uint64(len(data))
		return nil
	}
	var visit func([]layout.Block) error
	visit = func(blocks []layout.Block) error {
		for _, candidate := range layout.NormalizeBlocks(blocks) {
			if err := ctx.Err(); err != nil {
				return err
			}
			switch block := candidate.(type) {
			case layout.ImageBlock:
				data := block.ImageData()
				if len(data) != 0 {
					if err := add(data); err != nil {
						return err
					}
				}
			case layout.QRVerificationBlock:
				payload := strings.TrimSpace(block.QR.URL)
				if payload == "" {
					payload = strings.TrimSpace(block.QR.Value)
				}
				data, err := QRCodePNG(payload, defaultQRCodeSizePx)
				if err == nil {
					if err := add(data); err != nil {
						return err
					}
				}
			case layout.SectionBlock:
				if err := visit(block.Blocks); err != nil {
					return err
				}
			case layout.ClauseBlock:
				if err := visit(block.Blocks); err != nil {
					return err
				}
			case layout.NoteBoxBlock:
				if err := visit(block.Body); err != nil {
					return err
				}
			case layout.ListBlock:
				for _, item := range block.Items {
					if err := visit(item.Blocks); err != nil {
						return err
					}
				}
			case layout.RowColumnBlock:
				children := make([]layout.Block, 0, len(block.Items))
				for _, item := range block.Items {
					if item.Block != nil {
						children = append(children, item.Block)
					}
				}
				if err := visit(children); err != nil {
					return err
				}
			case layout.TableBlock:
				rows := make([]layout.TableRow, 0, len(block.Header)+len(block.Body)+len(block.Footer))
				rows = append(rows, block.Header...)
				rows = append(rows, block.Body...)
				rows = append(rows, block.Footer...)
				for _, row := range rows {
					for _, cell := range row.Cells {
						if err := visit(cell.Blocks); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	if err := visit(doc.Body); err != nil {
		return nil, err
	}
	for _, blocks := range typedPageTemplateBlockSets(doc.PageTemplate) {
		if err := visit(blocks); err != nil {
			return nil, err
		}
	}
	return result, nil
}
