// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"
)

var (
	ErrDisplaySVGLimit    = errors.New("layoutengine: display SVG capture limit exceeded")
	ErrDisplaySVGResource = errors.New("layoutengine: display SVG image resource is invalid")
)

const DisplayPlanSVGFormatVersion uint16 = 3

const (
	hardMaxDisplaySVGCommands uint64 = 1 << 16
	hardMaxDisplaySVGImages   uint64 = 1 << 12
	hardMaxDisplaySVGPaths    uint64 = 1 << 16
	hardMaxDisplaySVGSegments uint64 = 1 << 20
	hardMaxDisplaySVGSource   uint64 = 16 << 20
	hardMaxDisplaySVGOutput   uint64 = 32 << 20
)

type DisplaySVGImageSources map[ImageContentDigest][]byte

type DisplaySVGLimits struct {
	MaxCommands     uint64
	MaxImages       uint64
	MaxPaths        uint64
	MaxPathSegments uint64
	MaxSourceBytes  uint64
	MaxOutputBytes  uint64
}

func DefaultDisplaySVGLimits() DisplaySVGLimits {
	return DisplaySVGLimits{
		MaxCommands: hardMaxDisplaySVGCommands, MaxImages: hardMaxDisplaySVGImages,
		MaxPaths: hardMaxDisplaySVGPaths, MaxPathSegments: hardMaxDisplaySVGSegments,
		MaxSourceBytes: hardMaxDisplaySVGSource, MaxOutputBytes: hardMaxDisplaySVGOutput,
	}
}

type DisplayPlanSVGCapture struct {
	FormatVersion uint16
	Page          uint32
	PageBounds    Rect
	FixedScale    int64
	SVG           []byte
}

func CaptureDisplayPlanSVG(plan LayoutPlan, page uint32, sources DisplaySVGImageSources) (DisplayPlanSVGCapture, error) {
	return CaptureDisplayPlanSVGContext(context.Background(), plan, page, sources)
}

func CaptureDisplayPlanSVGContext(ctx context.Context, plan LayoutPlan, page uint32, sources DisplaySVGImageSources) (DisplayPlanSVGCapture, error) {
	return CaptureDisplayPlanSVGWithLimitsContext(ctx, plan, page, sources, DefaultDisplaySVGLimits())
}

// CaptureDisplayPlanSVGWithLimits verifies every encoded image before writing
// and maps every immutable display command into the SVG coordinate system.
// It performs no measuring, fitting, alignment, pagination, or path recovery.
func CaptureDisplayPlanSVGWithLimits(plan LayoutPlan, pageNumber uint32, sources DisplaySVGImageSources, limits DisplaySVGLimits) (DisplayPlanSVGCapture, error) {
	return CaptureDisplayPlanSVGWithLimitsContext(context.Background(), plan, pageNumber, sources, limits)
}

func CaptureDisplayPlanSVGWithLimitsContext(ctx context.Context, plan LayoutPlan, pageNumber uint32, sources DisplaySVGImageSources, limits DisplaySVGLimits) (DisplayPlanSVGCapture, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return DisplayPlanSVGCapture{}, err
	}
	if err := validateDisplaySVGLimits(limits); err != nil {
		return DisplayPlanSVGCapture{}, err
	}
	if pageNumber == 0 {
		return DisplayPlanSVGCapture{}, ErrDebugGeometryInvalidPage
	}
	if err := plan.ValidateDisplayPaintReady(); err != nil {
		return DisplayPlanSVGCapture{}, fmt.Errorf("layoutengine: display SVG preflight: %w", err)
	}
	if uint64(pageNumber) > uint64(len(plan.pages)) {
		return DisplayPlanSVGCapture{}, fmt.Errorf("%w: %d", ErrDebugGeometryPageNotFound, pageNumber)
	}
	if uint64(len(plan.commands)) > limits.MaxCommands || uint64(len(plan.images)) > limits.MaxImages || uint64(len(plan.paths)) > limits.MaxPaths {
		return DisplayPlanSVGCapture{}, ErrDisplaySVGLimit
	}
	var pathSegments uint64
	for index, path := range plan.paths {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return DisplayPlanSVGCapture{}, err
			}
		}
		count := uint64(len(path.Segments))
		if count > limits.MaxPathSegments-pathSegments {
			return DisplayPlanSVGCapture{}, ErrDisplaySVGLimit
		}
		pathSegments += count
	}
	encoded, err := preflightDisplaySVGSourcesContext(ctx, plan.imageResources, sources, limits.MaxSourceBytes)
	if err != nil {
		return DisplayPlanSVGCapture{}, err
	}

	page := plan.pages[pageNumber-1]
	commandEnd, _ := page.Commands.end(len(plan.commands))
	writer := debugGeometrySVGWriter{limit: int(limits.MaxOutputBytes)} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	writer.write("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	writer.write("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 ")
	writer.write(fixedSVGDecimal(page.Size.Width))
	writer.write(" ")
	writer.write(fixedSVGDecimal(page.Size.Height))
	writer.write("\" data-format=\"display-plan-preview\" data-format-version=\"")
	writer.write(fmt.Sprint(DisplayPlanSVGFormatVersion))
	writer.write("\" data-fixed-scale=\"")
	writer.write(fmt.Sprint(FixedScale))
	writer.write("\">")
	writer.write("<rect width=\"100%\" height=\"100%\" fill=\"white\"/>")
	graphics := []displaySVGGraphicsState{{}}
	for index := int(page.Commands.Start); index < commandEnd; index++ {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return DisplayPlanSVGCapture{}, err
			}
		}
		command := plan.commands[index]
		switch command.Kind {
		case CommandSaveState:
			writer.write("<g data-graphics-state=\"save\" data-command-index=\"")
			writer.write(strconv.Itoa(index))
			writer.write("\">")
			graphics = append(graphics, displaySVGGraphicsState{})
		case CommandRestoreState:
			writeDisplaySVGCloseEffects(&writer, &graphics[len(graphics)-1])
			writer.write("</g>")
			graphics = graphics[:len(graphics)-1]
		case CommandTransform:
			writeDisplaySVGTransform(&writer, index, plan.transforms[command.Payload])
			graphics[len(graphics)-1].effects++
		case CommandClip:
			clip := plan.clips[command.Payload]
			writeDisplaySVGClip(&writer, index, plan.paths[clip.Path], clip)
			graphics[len(graphics)-1].effects++
		case CommandFillPath:
			fill := plan.fills[command.Payload]
			writeDisplaySVGFill(&writer, index, plan.paths[fill.Path], fill)
		case CommandStrokePath:
			stroke := plan.strokes[command.Payload]
			writeDisplaySVGStroke(&writer, index, plan.paths[stroke.Path], stroke)
		case CommandGlyphRun:
			writeDisplaySVGGlyphRun(&writer, plan, command)
		case CommandImage:
			image := plan.images[command.Payload]
			resource := plan.imageResources[image.Resource-1]
			writeDisplaySVGImage(&writer, index, resource, image, encoded[resource.ID])
		case CommandLink:
			// Link annotations have no page-marking appearance.
		}
	}
	writeDisplaySVGCloseEffects(&writer, &graphics[0])
	writer.write("</svg>")
	if writer.err != nil {
		return DisplayPlanSVGCapture{}, ErrDisplaySVGLimit
	}
	return DisplayPlanSVGCapture{
		FormatVersion: DisplayPlanSVGFormatVersion, Page: page.Number,
		PageBounds: Rect{Width: page.Size.Width, Height: page.Size.Height},
		FixedScale: FixedScale, SVG: []byte(writer.builder.String()),
	}, nil
}

func validateDisplaySVGLimits(limits DisplaySVGLimits) error {
	if limits.MaxCommands == 0 || limits.MaxImages == 0 || limits.MaxPaths == 0 || limits.MaxPathSegments == 0 || limits.MaxSourceBytes == 0 || limits.MaxOutputBytes == 0 {
		return errors.New("layoutengine: display SVG limits must be positive")
	}
	if limits.MaxCommands > hardMaxDisplaySVGCommands || limits.MaxImages > hardMaxDisplaySVGImages || limits.MaxPaths > hardMaxDisplaySVGPaths || limits.MaxPathSegments > hardMaxDisplaySVGSegments ||
		limits.MaxSourceBytes > hardMaxDisplaySVGSource || limits.MaxOutputBytes > hardMaxDisplaySVGOutput {
		return errors.New("layoutengine: display SVG limits exceed hard caps")
	}
	return nil
}

type displaySVGGraphicsState struct{ effects uint32 }

func writeDisplaySVGCloseEffects(writer *debugGeometrySVGWriter, state *displaySVGGraphicsState) {
	for state.effects > 0 {
		writer.write("</g>")
		state.effects--
	}
}

func writeDisplaySVGTransform(writer *debugGeometrySVGWriter, commandIndex int, transform Transform) {
	writer.write("<g data-command-index=\"")
	writer.write(strconv.Itoa(commandIndex))
	writer.write("\" transform=\"matrix(")
	writer.write(fixedSVGScalarDecimal(transform.A) + " " + fixedSVGScalarDecimal(transform.B) + " " +
		fixedSVGScalarDecimal(transform.C) + " " + fixedSVGScalarDecimal(transform.D) + " " +
		fixedSVGDecimal(transform.TX) + " " + fixedSVGDecimal(transform.TY))
	writer.write(")\">")
}

func writeDisplaySVGClip(writer *debugGeometrySVGWriter, commandIndex int, path PlannedPath, clip PlannedClip) {
	id := "display-clip-" + strconv.Itoa(commandIndex)
	writer.write("<defs><clipPath id=\"" + id + "\" clipPathUnits=\"userSpaceOnUse\"><path d=\"")
	writeDisplaySVGPathData(writer, path)
	writer.write("\" clip-rule=\"" + displaySVGFillRule(clip.Rule) + "\"/></clipPath></defs>")
	writer.write("<g data-command-index=\"" + strconv.Itoa(commandIndex) + "\" clip-path=\"url(#" + id + ")\">")
}

func writeDisplaySVGFill(writer *debugGeometrySVGWriter, commandIndex int, path PlannedPath, fill PlannedFill) {
	writer.write("<path data-command-index=\"" + strconv.Itoa(commandIndex) + "\" d=\"")
	writeDisplaySVGPathData(writer, path)
	writer.write("\" fill=\"" + displaySVGColor(fill.Color) + "\" fill-rule=\"" + displaySVGFillRule(fill.Rule) + "\"")
	if fill.Opacity != 0 {
		writer.write(" fill-opacity=\"" + fixedSVGScalarDecimal(fill.Opacity) + "\"")
	}
	writer.write("/>")
}

func writeDisplaySVGStroke(writer *debugGeometrySVGWriter, commandIndex int, path PlannedPath, stroke PlannedStroke) {
	writer.write("<path data-command-index=\"" + strconv.Itoa(commandIndex) + "\" d=\"")
	writeDisplaySVGPathData(writer, path)
	writer.write("\" fill=\"none\" stroke=\"" + displaySVGColor(stroke.Color) + "\" stroke-width=\"" + fixedSVGDecimal(stroke.Width) + "\"")
	capStyle := stroke.LineCap
	if capStyle == "" {
		capStyle = StrokeCapButt
	}
	joinStyle := stroke.LineJoin
	if joinStyle == "" {
		joinStyle = StrokeJoinMiter
	}
	writer.write(" stroke-linecap=\"" + string(capStyle) + "\" stroke-linejoin=\"" + string(joinStyle) + "\"")
	if len(stroke.Dash) != 0 {
		writer.write(" stroke-dasharray=\"")
		for index, value := range stroke.Dash {
			if index != 0 {
				writer.write(" ")
			}
			writer.write(fixedSVGDecimal(value))
		}
		writer.write("\" stroke-dashoffset=\"" + fixedSVGDecimal(stroke.DashOffset) + "\"")
	}
	if stroke.Opacity != 0 {
		writer.write(" stroke-opacity=\"" + fixedSVGScalarDecimal(stroke.Opacity) + "\"")
	}
	writer.write("/>")
}

func writeDisplaySVGPathData(writer *debugGeometrySVGWriter, path PlannedPath) {
	for index, segment := range path.Segments {
		if index != 0 {
			writer.write(" ")
		}
		switch segment.Kind {
		case PathMoveTo:
			writer.write("M " + fixedSVGDecimal(segment.Point.X) + " " + fixedSVGDecimal(segment.Point.Y))
		case PathLineTo:
			writer.write("L " + fixedSVGDecimal(segment.Point.X) + " " + fixedSVGDecimal(segment.Point.Y))
		case PathCubicTo:
			writer.write("C " + fixedSVGDecimal(segment.Control1.X) + " " + fixedSVGDecimal(segment.Control1.Y) + " " +
				fixedSVGDecimal(segment.Control2.X) + " " + fixedSVGDecimal(segment.Control2.Y) + " " +
				fixedSVGDecimal(segment.Point.X) + " " + fixedSVGDecimal(segment.Point.Y))
		case PathClose:
			writer.write("Z")
		}
	}
}

func displaySVGFillRule(rule FillRule) string {
	if rule == FillEvenOdd {
		return "evenodd"
	}
	return "nonzero"
}

func displaySVGColor(color CoreRGBColor) string {
	const digits = "0123456789abcdef"
	value := [7]byte{'#', digits[color.R>>4], digits[color.R&15], digits[color.G>>4], digits[color.G&15], digits[color.B>>4], digits[color.B&15]}
	return string(value[:])
}

func fixedSVGScalarDecimal(value Fixed) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	var magnitude uint64
	if negative {
		magnitude = uint64(-(value + 1)) + 1 // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	} else {
		magnitude = uint64(value)
	}
	whole, remainder := magnitude/uint64(FixedScale), magnitude%uint64(FixedScale)
	var result strings.Builder
	if negative {
		result.WriteByte('-')
	}
	result.WriteString(strconv.FormatUint(whole, 10))
	if remainder == 0 {
		return result.String()
	}
	result.WriteByte('.')
	for remainder != 0 {
		remainder *= 10
		result.WriteByte(byte('0' + remainder/uint64(FixedScale)))
		remainder %= uint64(FixedScale)
	}
	return result.String()
}

func preflightDisplaySVGSources(resources []ImageResource, sources DisplaySVGImageSources, maxBytes uint64) (map[ImageResourceID]string, error) {
	return preflightDisplaySVGSourcesContext(context.Background(), resources, sources, maxBytes)
}

func preflightDisplaySVGSourcesContext(ctx context.Context, resources []ImageResource, sources DisplaySVGImageSources, maxBytes uint64) (map[ImageResourceID]string, error) {
	result := make(map[ImageResourceID]string, len(resources))
	var total uint64
	for index, resource := range resources {
		if index&31 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		value := sources[resource.Digest]
		if len(value) == 0 || uint64(len(value)) > maxBytes-total {
			return nil, fmt.Errorf("%w: missing or over-budget bytes for %s", ErrDisplaySVGResource, resource.Digest)
		}
		total += uint64(len(value))
		digest := sha256.New()
		for offset := 0; offset < len(value); offset += 64 << 10 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			end := offset + (64 << 10)
			if end > len(value) {
				end = len(value)
			}
			_, _ = digest.Write(value[offset:end])
		}
		if hex.EncodeToString(digest.Sum(nil)) != string(resource.Digest) {
			return nil, fmt.Errorf("%w: digest mismatch for %s", ErrDisplaySVGResource, resource.Digest)
		}
		config, format, err := image.DecodeConfig(&displaySVGContextReader{ctx: ctx, reader: bytes.NewReader(value)})
		if err != nil || uint32(config.Width) != resource.PixelWidth || uint32(config.Height) != resource.PixelHeight || // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			(format != "png" && format != "jpeg") {
			return nil, fmt.Errorf("%w: intrinsic dimensions or format mismatch for %s", ErrDisplaySVGResource, resource.Digest)
		}
		mime := "image/png"
		if resource.Format == ImageJPEG {
			mime = "image/jpeg"
		}
		var encoded bytes.Buffer
		encoded.WriteString("data:" + mime + ";base64,")
		encoder := base64.NewEncoder(base64.StdEncoding, &encoded)
		for offset := 0; offset < len(value); offset += 64 << 10 {
			if err := ctx.Err(); err != nil {
				_ = encoder.Close()
				return nil, err
			}
			end := offset + (64 << 10)
			if end > len(value) {
				end = len(value)
			}
			if _, err := encoder.Write(value[offset:end]); err != nil {
				return nil, err
			}
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
		result[resource.ID] = encoded.String()
	}
	return result, nil
}

type displaySVGContextReader struct {
	ctx    context.Context
	reader *bytes.Reader
}

func (r *displaySVGContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func writeDisplaySVGGlyphRun(writer *debugGeometrySVGWriter, plan LayoutPlan, command DisplayCommand) {
	run := plan.glyphRuns[command.Payload]
	font := plan.fonts[run.Font-1]
	cursor := run.Origin.X
	index := 0
	for _, code := range run.Codes {
		writer.write("<text x=\"")
		writer.write(fixedSVGDecimal(cursor))
		writer.write("\" y=\"")
		writer.write(fixedSVGDecimal(run.Origin.Y))
		writer.write("\" font-size=\"")
		writer.write(fixedSVGDecimal(run.FontSize))
		writer.write("\" font-family=\"")
		writer.attribute(displayFontFamily(font))
		writeCoreRGBSVGAttribute(writer, run.Color)
		if run.Opacity != 0 {
			writer.write("\" fill-opacity=\"" + fixedSVGScalarDecimal(run.Opacity))
		}
		writer.write("\">")
		writer.attribute(string(code))
		writer.write("</text>")
		cursor, _ = cursor.Add(run.Advances[index])
		index++
	}
}

func displayFontFamily(font CoreFontResource) string {
	if font.EmbeddedUTF8 != nil {
		return font.EmbeddedUTF8.Name
	}
	return string(font.Face)
}

func writeCoreRGBSVGAttribute(writer *debugGeometrySVGWriter, color CoreRGBColor) {
	if !color.Set {
		return
	}
	const hexadecimal = "0123456789abcdef"
	value := [7]byte{'#'}
	value[1], value[2] = hexadecimal[color.R>>4], hexadecimal[color.R&0x0f]
	value[3], value[4] = hexadecimal[color.G>>4], hexadecimal[color.G&0x0f]
	value[5], value[6] = hexadecimal[color.B>>4], hexadecimal[color.B&0x0f]
	writer.write("\" fill=\"")
	writer.write(string(value[:]))
}

func writeDisplaySVGImage(writer *debugGeometrySVGWriter, commandIndex int, resource ImageResource, image PlannedImage, href string) {
	intrinsic := Size{Width: Fixed(resource.PixelWidth), Height: Fixed(resource.PixelHeight)}
	source := Rect{Width: intrinsic.Width, Height: intrinsic.Height}
	clip := image.Bounds
	if image.Crop != nil {
		intrinsic, source, clip = image.Crop.Intrinsic, image.Crop.Source, image.Crop.Clip
	}
	writer.write("<svg class=\"planned-image\" data-command-index=\"")
	writer.write(fmt.Sprint(commandIndex))
	writer.write("\" x=\"")
	writer.write(fixedSVGDecimal(clip.X))
	writer.write("\" y=\"")
	writer.write(fixedSVGDecimal(clip.Y))
	writer.write("\" width=\"")
	writer.write(fixedSVGDecimal(clip.Width))
	writer.write("\" height=\"")
	writer.write(fixedSVGDecimal(clip.Height))
	if image.Opacity != 0 {
		writer.write("\" opacity=\"" + fixedSVGScalarDecimal(image.Opacity))
	}
	writer.write("\" viewBox=\"")
	writer.write(fixedSVGDecimal(source.X) + " " + fixedSVGDecimal(source.Y) + " " + fixedSVGDecimal(source.Width) + " " + fixedSVGDecimal(source.Height))
	writer.write("\" preserveAspectRatio=\"none\" overflow=\"hidden\">")
	writer.write("<image x=\"0\" y=\"0\" width=\"")
	writer.write(fixedSVGDecimal(intrinsic.Width))
	writer.write("\" height=\"")
	writer.write(fixedSVGDecimal(intrinsic.Height))
	writer.write("\" preserveAspectRatio=\"none\" href=\"")
	writer.attribute(href)
	writer.write("\"/></svg>")
}
