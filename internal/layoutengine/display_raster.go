// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"sort"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
	"golang.org/x/text/encoding/charmap"
)

const (
	DisplayRasterManifestVersion uint16 = 1
	DisplayRasterRendererVersion        = "layoutengine/go-display-raster@2"

	DisplayRasterHardMaxPixels      uint64 = 64 << 20
	DisplayRasterHardMaxSourceBytes uint64 = 64 << 20
	DisplayRasterHardMaxPNGBytes    uint64 = 64 << 20
)

var (
	ErrDisplayRasterRequest     = errors.New("layoutengine: invalid display raster request")
	ErrDisplayRasterLimit       = errors.New("layoutengine: display raster limit exceeded")
	ErrDisplayRasterResource    = errors.New("layoutengine: display raster resource is invalid")
	ErrDisplayRasterUnsupported = errors.New("layoutengine: display raster operation is unsupported")
)

// DisplayRasterProfile pins every renderer choice which can affect normative
// review pixels. Version 1 deliberately has one profile rather than accepting
// loosely interpreted strings.
type DisplayRasterProfile struct {
	DPI             uint32 `json:"dpi"`
	ColorSpace      string `json:"color_space"`
	AlphaMode       string `json:"alpha_mode"`
	Background      string `json:"background"`
	Antialiasing    string `json:"antialiasing"`
	CropRounding    string `json:"crop_rounding"`
	CoordinateRound string `json:"coordinate_rounding"`
	ImageResampling string `json:"image_resampling"`
	PNGCompression  string `json:"png_compression"`
	Renderer        string `json:"renderer"`
}

func DefaultDisplayRasterProfile() DisplayRasterProfile {
	return DisplayRasterProfile{
		DPI: 144, ColorSpace: "srgb_8bit", AlphaMode: "opaque", Background: "#ffffff",
		Antialiasing: "coverage_8bit", CropRounding: "exact_bounds_nearest_pixel_count_half_away_from_zero",
		CoordinateRound: "26.6_fixed_half_away_from_zero", ImageResampling: "nearest_neighbor",
		PNGCompression: "go_png_best_compression", Renderer: DisplayRasterRendererVersion,
	}
}

func (profile DisplayRasterProfile) validate() error {
	want := DefaultDisplayRasterProfile()
	if profile.DPI < 36 || profile.DPI > 600 {
		return fmt.Errorf("%w: DPI must be between 36 and 600", ErrDisplayRasterRequest)
	}
	want.DPI = profile.DPI
	if profile != want {
		return fmt.Errorf("%w: raster profile contains an unsupported renderer choice", ErrDisplayRasterRequest)
	}
	return nil
}

type DisplayRasterLimits struct {
	MaxPixels      uint64             `json:"max_pixels"`
	MaxSourceBytes uint64             `json:"max_source_bytes"`
	MaxPNGBytes    uint64             `json:"max_png_bytes"`
	Paint          DisplayPaintLimits `json:"paint"`
}

func DefaultDisplayRasterLimits() DisplayRasterLimits {
	return DisplayRasterLimits{MaxPixels: 16 << 20, MaxSourceBytes: 32 << 20, MaxPNGBytes: 32 << 20, Paint: DefaultDisplayPaintLimits()}
}

func (limits DisplayRasterLimits) validate() error {
	if limits.MaxPixels == 0 || limits.MaxPixels > DisplayRasterHardMaxPixels ||
		limits.MaxSourceBytes == 0 || limits.MaxSourceBytes > DisplayRasterHardMaxSourceBytes ||
		limits.MaxPNGBytes == 0 || limits.MaxPNGBytes > DisplayRasterHardMaxPNGBytes {
		return fmt.Errorf("%w: limits must be positive and within hard caps", ErrDisplayRasterRequest)
	}
	return nil
}

// DisplayRasterSources are immutable bytes needed to paint the retained plan.
// Core-font metrics digests identify the planned metrics; FontPrograms supplies
// the actual preview outlines and their independent hashes are recorded. This
// is direct preview evidence, not a claim that a PDF consumer uses those same
// outlines.
type DisplayRasterSources struct {
	FontPrograms map[CoreFontMetricsDigest][]byte
	Images       DisplaySVGImageSources
}

type DisplayRasterRequest struct {
	Page        uint32
	Crop        *Rect
	Profile     DisplayRasterProfile
	Limits      DisplayRasterLimits
	Revisions   ViewerRevisionIdentityInput
	PageProfile string
}

type DisplayRasterResourceDigest struct {
	Kind       string `json:"kind"`
	Identity   string `json:"identity"`
	SHA256     string `json:"sha256"`
	ByteLength uint64 `json:"byte_length"`
}

// DisplayRasterTransform maps exact page coordinates to zero-based raster
// coordinates: pixel = (page-capture_origin) * numerator / denominator.
type DisplayRasterTransform struct {
	OriginX      Fixed  `json:"origin_x"`
	OriginY      Fixed  `json:"origin_y"`
	XNumerator   uint32 `json:"x_numerator"`
	XDenominator Fixed  `json:"x_denominator"`
	YNumerator   uint32 `json:"y_numerator"`
	YDenominator Fixed  `json:"y_denominator"`
}

type DisplayRasterManifest struct {
	FormatVersion     uint16                        `json:"format_version"`
	PlanSchemaVersion uint16                        `json:"plan_schema_version"`
	PlanHash          string                        `json:"plan_hash"`
	Identity          ViewerIdentity                `json:"identity"`
	PageProfile       string                        `json:"page_profile"`
	ArtifactKind      string                        `json:"artifact_kind"`
	AuthoritativePDF  bool                          `json:"authoritative_pdf_raster"`
	Disclosure        string                        `json:"disclosure"`
	ContainsContent   bool                          `json:"contains_rendered_content"`
	MediaType         string                        `json:"media_type"`
	Page              uint32                        `json:"page"`
	PageBounds        Rect                          `json:"page_bounds"`
	CaptureBounds     Rect                          `json:"capture_bounds"`
	PixelWidth        uint32                        `json:"pixel_width"`
	PixelHeight       uint32                        `json:"pixel_height"`
	PixelTransform    DisplayRasterTransform        `json:"pixel_transform"`
	Profile           DisplayRasterProfile          `json:"profile"`
	Limits            DisplayRasterLimits           `json:"limits"`
	Resources         []DisplayRasterResourceDigest `json:"resources,omitempty"`
	PNGByteLength     uint64                        `json:"png_byte_length"`
	PNGSHA256         string                        `json:"png_sha256"`
}

func (manifest DisplayRasterManifest) CanonicalJSON() ([]byte, error) {
	if err := manifest.Profile.validate(); err != nil {
		return nil, err
	}
	if err := manifest.Limits.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

type DisplayRasterArtifact struct {
	manifest DisplayRasterManifest
	png      []byte
}

func (artifact DisplayRasterArtifact) Manifest() DisplayRasterManifest {
	result := artifact.manifest
	result.Resources = append([]DisplayRasterResourceDigest(nil), artifact.manifest.Resources...)
	return result
}
func (artifact DisplayRasterArtifact) PNG() []byte { return append([]byte(nil), artifact.png...) }
func (artifact DisplayRasterArtifact) CanonicalManifestJSON() ([]byte, error) {
	return artifact.manifest.CanonicalJSON()
}

func CaptureDisplayPlanPNG(plan LayoutPlan, sources DisplayRasterSources, request DisplayRasterRequest) (DisplayRasterArtifact, error) {
	return CaptureDisplayPlanPNGContext(context.Background(), plan, sources, request)
}

// CaptureDisplayPlanPNGContext paints an immutable display list into a
// lossless PNG. It never measures, wraps, fits, fragments, or paginates.
// Version 2 supports core glyphs, images, links (non-marking), translations,
// save/restore, filled paths, and deterministic straight-segment strokes.
// Rectangular nonzero clips are supported. Curved/non-rectangular clips,
// curved strokes, and non-translation affine transforms fail during preflight.
func CaptureDisplayPlanPNGContext(ctx context.Context, plan LayoutPlan, sources DisplayRasterSources, request DisplayRasterRequest) (DisplayRasterArtifact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if err := request.Profile.validate(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if err := request.Limits.validate(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if request.Page == 0 || uint64(request.Page) > uint64(len(plan.pages)) {
		return DisplayRasterArtifact{}, ErrDebugGeometryPageNotFound
	}
	if err := request.Revisions.validate(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if err := validateDigestString(request.PageProfile); err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: page profile must be a lowercase SHA-256 digest", ErrDisplayRasterRequest)
	}
	if err := ValidateDisplayPaintPlan(plan, request.Limits.Paint); err != nil {
		return DisplayRasterArtifact{}, err
	}
	page := plan.pages[request.Page-1]
	capture := Rect{Width: page.Size.Width, Height: page.Size.Height}
	if request.Crop != nil {
		capture = *request.Crop
		if err := capture.Validate(); err != nil || capture.IsEmpty() {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: crop must be non-empty valid page geometry", ErrDisplayRasterRequest)
		}
		if !rectContainsRect(Rect{Width: page.Size.Width, Height: page.Size.Height}, capture) {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: crop lies outside the page", ErrDisplayRasterRequest)
		}
	}
	width, err := rasterPixelExtent(capture.Width, request.Profile.DPI)
	if err != nil {
		return DisplayRasterArtifact{}, err
	}
	height, err := rasterPixelExtent(capture.Height, request.Profile.DPI)
	if err != nil {
		return DisplayRasterArtifact{}, err
	}
	if uint64(width) > request.Limits.MaxPixels/uint64(height) {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: pixels", ErrDisplayRasterLimit)
	}

	fonts, images, resources, err := preflightDisplayRaster(ctx, plan, sources, request, page)
	if err != nil {
		return DisplayRasterArtifact{}, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	state := rasterPaintState{transform: IdentityTransform(), clip: canvas.Bounds()}
	stack := []rasterPaintState{state}
	end, _ := page.Commands.end(len(plan.commands))
	for commandIndex := int(page.Commands.Start); commandIndex < end; commandIndex++ {
		if commandIndex&255 == 0 {
			if err := ctx.Err(); err != nil {
				return DisplayRasterArtifact{}, err
			}
		}
		command := plan.commands[commandIndex]
		switch command.Kind {
		case CommandSaveState:
			stack = append(stack, state)
		case CommandRestoreState:
			state = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case CommandTransform:
			state.transform, _ = state.transform.Then(plan.transforms[command.Payload])
		case CommandClip:
			clip := plan.clips[command.Payload]
			box := plan.paths[clip.Path].Bounds
			origin, _ := state.transform.Apply(Point{X: box.X, Y: box.Y})
			box.X, box.Y = origin.X, origin.Y
			right, _ := box.Right()
			bottom, _ := box.Bottom()
			x0, y0 := pageToRasterFloat(box.X, box.Y, capture, canvas.Bounds())
			x1, y1 := pageToRasterFloat(right, bottom, capture, canvas.Bounds())
			state.clip = state.clip.Intersect(image.Rect(int(math.Floor(float64(x0))), int(math.Floor(float64(y0))), int(math.Ceil(float64(x1))), int(math.Ceil(float64(y1)))))
		case CommandFillPath:
			fill := plan.fills[command.Payload]
			if err := rasterFillPath(canvas, plan.paths[fill.Path], fill, state.transform, capture, state.clip); err != nil {
				return DisplayRasterArtifact{}, err
			}
		case CommandStrokePath:
			stroke := plan.strokes[command.Payload]
			if err := rasterStrokePath(canvas, plan.paths[stroke.Path], stroke, state.transform, capture, state.clip); err != nil {
				return DisplayRasterArtifact{}, err
			}
		case CommandGlyphRun:
			run := plan.glyphRuns[command.Payload]
			if err := rasterGlyphRun(canvas, fonts[run.Font], plan.fonts[run.Font-1], run, state.transform, capture, request.Profile.DPI, state.clip); err != nil {
				return DisplayRasterArtifact{}, err
			}
		case CommandImage:
			placement := plan.images[command.Payload]
			if err := rasterImage(canvas, images[placement.Resource], placement, state.transform, capture, state.clip); err != nil {
				return DisplayRasterArtifact{}, err
			}
		case CommandLink:
			// Link annotations have no page-marking appearance.
		}
	}
	var encoded bytes.Buffer
	limited := &rasterLimitWriter{writer: &encoded, remaining: request.Limits.MaxPNGBytes}
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(limited, canvas); err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: PNG: %v", ErrDisplayRasterLimit, err)
	}
	pngBytes := encoded.Bytes()
	pngHash := sha256.Sum256(pngBytes)
	planHash, _ := plan.Hash()
	identity, err := viewerIdentityForPlan(plan, DisplayRasterRendererVersion, request.Revisions)
	if err != nil {
		return DisplayRasterArtifact{}, err
	}
	manifest := DisplayRasterManifest{
		FormatVersion: DisplayRasterManifestVersion, PlanSchemaVersion: LayoutPlanSchemaVersion, PlanHash: planHash.String(), Identity: identity,
		PageProfile: request.PageProfile, ArtifactKind: "direct_display_list_preview", AuthoritativePDF: false, MediaType: "image/png",
		Disclosure: "contains_rendered_content", ContainsContent: true,
		Page: request.Page, PageBounds: Rect{Width: page.Size.Width, Height: page.Size.Height}, CaptureBounds: capture,
		PixelWidth: width, PixelHeight: height, PixelTransform: DisplayRasterTransform{OriginX: capture.X, OriginY: capture.Y, XNumerator: width, XDenominator: capture.Width, YNumerator: height, YDenominator: capture.Height},
		Profile: request.Profile, Limits: request.Limits, Resources: resources, PNGByteLength: uint64(len(pngBytes)), PNGSHA256: hex.EncodeToString(pngHash[:]),
	}
	if _, err := manifest.CanonicalJSON(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	return DisplayRasterArtifact{manifest: manifest, png: append([]byte(nil), pngBytes...)}, nil
}

type rasterPaintState struct {
	transform Transform
	clip      image.Rectangle
}

func preflightDisplayRaster(ctx context.Context, plan LayoutPlan, sources DisplayRasterSources, request DisplayRasterRequest, page PlannedPage) (map[FontResourceID]*rasterSizedFace, map[ImageResourceID]image.Image, []DisplayRasterResourceDigest, error) {
	end, _ := page.Commands.end(len(plan.commands))
	neededFonts := map[FontResourceID]bool{}
	neededImages := map[ImageResourceID]bool{}
	for index := int(page.Commands.Start); index < end; index++ {
		command := plan.commands[index]
		switch command.Kind {
		case CommandClip:
			clip := plan.clips[command.Payload]
			if clip.Rule != FillNonZero || !rasterRectangularPath(plan.paths[clip.Path]) {
				return nil, nil, nil, fmt.Errorf("%w: non-rectangular clip", ErrDisplayRasterUnsupported)
			}
		case CommandStrokePath:
			path := plan.paths[plan.strokes[command.Payload].Path]
			for _, segment := range path.Segments {
				if segment.Kind == PathCubicTo {
					return nil, nil, nil, fmt.Errorf("%w: curved stroke", ErrDisplayRasterUnsupported)
				}
			}
		case CommandFillPath:
			if plan.fills[command.Payload].Rule != FillNonZero {
				return nil, nil, nil, fmt.Errorf("%w: even-odd fill", ErrDisplayRasterUnsupported)
			}
		case CommandTransform:
			transform := plan.transforms[command.Payload]
			if transform.A != Fixed(FixedScale) || transform.B != 0 || transform.C != 0 || transform.D != Fixed(FixedScale) {
				return nil, nil, nil, fmt.Errorf("%w: non-translation transform", ErrDisplayRasterUnsupported)
			}
		case CommandGlyphRun:
			neededFonts[plan.glyphRuns[command.Payload].Font] = true
		case CommandImage:
			neededImages[plan.images[command.Payload].Resource] = true
		}
	}
	fontFaces := map[FontResourceID]*rasterSizedFace{}
	decodedImages := map[ImageResourceID]image.Image{}
	records := make([]DisplayRasterResourceDigest, 0, len(neededFonts)+len(neededImages))
	var total uint64
	for _, resource := range plan.fonts {
		if !neededFonts[resource.ID] {
			continue
		}
		if resource.Face == CoreFontSymbol || resource.Face == CoreFontZapfDingbats {
			return nil, nil, nil, fmt.Errorf("%w: core-font code mapping for %s", ErrDisplayRasterUnsupported, resource.Face)
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		data := sources.FontPrograms[resource.MetricsDigest]
		if len(data) == 0 || uint64(len(data)) > request.Limits.MaxSourceBytes-total {
			return nil, nil, nil, fmt.Errorf("%w: missing or over-budget font %s", ErrDisplayRasterResource, resource.MetricsDigest)
		}
		total += uint64(len(data))
		parsed, err := opentype.Parse(data)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w: parse font %s: %v", ErrDisplayRasterResource, resource.MetricsDigest, err)
		}
		fontFaces[resource.ID] = &rasterSizedFace{font: parsed}
		digest := sha256.Sum256(data)
		if resource.EmbeddedUTF8 != nil && (hex.EncodeToString(digest[:]) != string(resource.EmbeddedUTF8.Digest) || uint32(len(data)) != resource.EmbeddedUTF8.ByteLength) {
			return nil, nil, nil, fmt.Errorf("%w: embedded font digest or length mismatch", ErrDisplayRasterResource)
		}
		records = append(records, DisplayRasterResourceDigest{Kind: "font_program", Identity: string(resource.MetricsDigest), SHA256: hex.EncodeToString(digest[:]), ByteLength: uint64(len(data))})
	}
	for _, resource := range plan.imageResources {
		if !neededImages[resource.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		data := sources.Images[resource.Digest]
		if len(data) == 0 || uint64(len(data)) > request.Limits.MaxSourceBytes-total {
			return nil, nil, nil, fmt.Errorf("%w: missing or over-budget image %s", ErrDisplayRasterResource, resource.Digest)
		}
		total += uint64(len(data))
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != string(resource.Digest) {
			return nil, nil, nil, fmt.Errorf("%w: image digest mismatch", ErrDisplayRasterResource)
		}
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil || decoded.Bounds().Dx() != int(resource.PixelWidth) || decoded.Bounds().Dy() != int(resource.PixelHeight) {
			return nil, nil, nil, fmt.Errorf("%w: image dimensions or encoding", ErrDisplayRasterResource)
		}
		decodedImages[resource.ID] = decoded
		records = append(records, DisplayRasterResourceDigest{Kind: "image", Identity: string(resource.Digest), SHA256: string(resource.Digest), ByteLength: uint64(len(data))})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].Identity < records[j].Identity
	})
	return fontFaces, decodedImages, records, nil
}

func rasterGlyphRun(dst *image.RGBA, face *rasterSizedFace, resource CoreFontResource, run CoreGlyphRun, transform Transform, crop Rect, dpi uint32, clip image.Rectangle) error {
	if face == nil {
		return fmt.Errorf("%w: missing font face", ErrDisplayRasterResource)
	}
	return face.draw(dst, resource, run, transform, crop, dpi, clip)
}

type rasterSizedFace struct{ font *opentype.Font }

func (face *rasterSizedFace) draw(dst *image.RGBA, resource CoreFontResource, run CoreGlyphRun, transform Transform, crop Rect, dpi uint32, clip image.Rectangle) error {
	actual, err := opentype.NewFace(face.font, &opentype.FaceOptions{Size: float64(run.FontSize) / float64(FixedScale), DPI: float64(dpi), Hinting: xfont.HintingNone})
	if err != nil {
		return err
	}
	defer actual.Close()
	origin, _ := transform.Apply(run.Origin)
	alpha := rasterOpacityAlpha(255, run.Opacity)
	colorValue := color.NRGBA{A: alpha}
	if run.Color.Set {
		colorValue.R, colorValue.G, colorValue.B = run.Color.R, run.Color.G, run.Color.B
	}
	x := origin.X
	var target draw.Image = dst
	if clip != dst.Bounds() {
		target = dst.SubImage(clip).(*image.RGBA)
	}
	index := 0
	for _, code := range run.Codes {
		px := pageToRaster26_6(x, crop.X, uint32(dst.Bounds().Dx()), crop.Width)
		py := pageToRaster26_6(origin.Y, crop.Y, uint32(dst.Bounds().Dy()), crop.Height)
		drawer := xfont.Drawer{Dst: target, Src: image.NewUniform(colorValue), Face: actual, Dot: fixed.Point26_6{X: px, Y: py}}
		if resource.IsEmbeddedUTF8() {
			drawer.DrawString(string(code))
		} else {
			drawer.DrawString(string(charmap.Windows1252.DecodeByte(byte(code))))
		}
		x, _ = x.Add(run.Advances[index])
		index++
	}
	return nil
}

func rasterFillPath(dst *image.RGBA, path PlannedPath, fill PlannedFill, transform Transform, crop Rect, clip image.Rectangle) error {
	r := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	for _, segment := range path.Segments {
		point, _ := transform.Apply(segment.Point)
		c1, _ := transform.Apply(segment.Control1)
		c2, _ := transform.Apply(segment.Control2)
		x, y := pageToRasterFloat(point.X, point.Y, crop, dst.Bounds())
		switch segment.Kind {
		case PathMoveTo:
			r.MoveTo(x, y)
		case PathLineTo:
			r.LineTo(x, y)
		case PathCubicTo:
			x1, y1 := pageToRasterFloat(c1.X, c1.Y, crop, dst.Bounds())
			x2, y2 := pageToRasterFloat(c2.X, c2.Y, crop, dst.Bounds())
			r.CubeTo(x1, y1, x2, y2, x, y)
		case PathClose:
			r.ClosePath()
		}
	}
	alpha := uint8(255)
	if fill.Opacity != 0 {
		alpha = uint8((int64(fill.Opacity)*255 + FixedScale/2) / FixedScale)
	}
	c := color.NRGBA{R: fill.Color.R, G: fill.Color.G, B: fill.Color.B, A: alpha}
	r.Draw(dst, clip, image.NewUniform(c), image.Point{})
	return nil
}

func rasterStrokePath(dst *image.RGBA, path PlannedPath, stroke PlannedStroke, transform Transform, crop Rect, clip image.Rectangle) error {
	var current, start Point
	haveCurrent := false
	alpha := uint8(255)
	if stroke.Opacity != 0 {
		alpha = uint8((int64(stroke.Opacity)*255 + FixedScale/2) / FixedScale)
	}
	colorValue := color.NRGBA{R: stroke.Color.R, G: stroke.Color.G, B: stroke.Color.B, A: alpha}
	width := float64(stroke.Width) * float64(dst.Bounds().Dx()) / float64(crop.Width)
	drawDisc := func(x, y, radius float64) {
		const k = 0.5522847498307936
		r := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
		r.MoveTo(float32(x+radius), float32(y))
		r.CubeTo(float32(x+radius), float32(y+k*radius), float32(x+k*radius), float32(y+radius), float32(x), float32(y+radius))
		r.CubeTo(float32(x-k*radius), float32(y+radius), float32(x-radius), float32(y+k*radius), float32(x-radius), float32(y))
		r.CubeTo(float32(x-radius), float32(y-k*radius), float32(x-k*radius), float32(y-radius), float32(x), float32(y-radius))
		r.CubeTo(float32(x+k*radius), float32(y-radius), float32(x+radius), float32(y-k*radius), float32(x+radius), float32(y))
		r.ClosePath()
		r.Draw(dst, clip, image.NewUniform(colorValue), image.Point{})
	}
	drawSegment := func(from, to Point, caps bool) {
		from, _ = transform.Apply(from)
		to, _ = transform.Apply(to)
		x0, y0 := pageToRasterFloat(from.X, from.Y, crop, dst.Bounds())
		x1, y1 := pageToRasterFloat(to.X, to.Y, crop, dst.Bounds())
		dx, dy := float64(x1-x0), float64(y1-y0)
		length := math.Hypot(dx, dy)
		if length == 0 {
			return
		}
		half := width / 2
		if caps && stroke.LineCap == StrokeCapSquare {
			ex, ey := dx/length*half, dy/length*half
			x0, y0, x1, y1 = float32(float64(x0)-ex), float32(float64(y0)-ey), float32(float64(x1)+ex), float32(float64(y1)+ey)
			dx, dy = float64(x1-x0), float64(y1-y0)
			length = math.Hypot(dx, dy)
		}
		nx, ny := -dy/length*half, dx/length*half
		r := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
		r.MoveTo(float32(float64(x0)+nx), float32(float64(y0)+ny))
		r.LineTo(float32(float64(x1)+nx), float32(float64(y1)+ny))
		r.LineTo(float32(float64(x1)-nx), float32(float64(y1)-ny))
		r.LineTo(float32(float64(x0)-nx), float32(float64(y0)-ny))
		r.ClosePath()
		r.Draw(dst, clip, image.NewUniform(colorValue), image.Point{})
		if caps && stroke.LineCap == StrokeCapRound {
			drawDisc(float64(x0), float64(y0), half)
			drawDisc(float64(x1), float64(y1), half)
		}
	}
	dashIndex, dashRemaining, dashOn := 0, float64(0), true
	if len(stroke.Dash) != 0 {
		total := float64(0)
		for _, dash := range stroke.Dash {
			total += dash.Points()
		}
		phase := math.Mod(stroke.DashOffset.Points(), total)
		if phase < 0 {
			phase += total
		}
		dashRemaining = stroke.Dash[0].Points()
		for phase >= dashRemaining && dashRemaining >= 0 {
			phase -= dashRemaining
			dashIndex = (dashIndex + 1) % len(stroke.Dash)
			dashOn = dashIndex%2 == 0
			dashRemaining = stroke.Dash[dashIndex].Points()
		}
		dashRemaining -= phase
	}
	drawAuthoredSegment := func(from, to Point) {
		if len(stroke.Dash) == 0 {
			drawSegment(from, to, stroke.LineCap != "" && stroke.LineCap != StrokeCapButt)
			return
		}
		dx, dy := (to.X - from.X).Points(), (to.Y - from.Y).Points()
		length := math.Hypot(dx, dy)
		used := float64(0)
		for used < length {
			if dashRemaining <= 0 {
				dashIndex = (dashIndex + 1) % len(stroke.Dash)
				dashOn = dashIndex%2 == 0
				dashRemaining = stroke.Dash[dashIndex].Points()
				continue
			}
			step := math.Min(dashRemaining, length-used)
			if dashOn && step > 0 {
				a, _ := FixedFromPoints(used / length)
				b, _ := FixedFromPoints((used + step) / length)
				pieceFrom := Point{X: from.X + Fixed(float64(to.X-from.X)*a.Points()), Y: from.Y + Fixed(float64(to.Y-from.Y)*a.Points())}
				pieceTo := Point{X: from.X + Fixed(float64(to.X-from.X)*b.Points()), Y: from.Y + Fixed(float64(to.Y-from.Y)*b.Points())}
				drawSegment(pieceFrom, pieceTo, true)
			}
			used += step
			dashRemaining -= step
		}
	}
	for _, segment := range path.Segments {
		switch segment.Kind {
		case PathMoveTo:
			current, start, haveCurrent = segment.Point, segment.Point, true
		case PathLineTo:
			if haveCurrent {
				drawAuthoredSegment(current, segment.Point)
				if len(stroke.Dash) == 0 && stroke.LineJoin == StrokeJoinRound {
					p, _ := transform.Apply(segment.Point)
					x, y := pageToRasterFloat(p.X, p.Y, crop, dst.Bounds())
					drawDisc(float64(x), float64(y), width/2)
				}
			}
			current, haveCurrent = segment.Point, true
		case PathClose:
			if haveCurrent {
				drawAuthoredSegment(current, start)
				current = start
			}
		}
	}
	return nil
}

func rasterImage(dst *image.RGBA, source image.Image, placement PlannedImage, transform Transform, crop Rect, clip image.Rectangle) error {
	if source == nil {
		return fmt.Errorf("%w: missing image", ErrDisplayRasterResource)
	}
	bounds := placement.Bounds
	origin, _ := transform.Apply(Point{X: bounds.X, Y: bounds.Y})
	bounds.X, bounds.Y = origin.X, origin.Y
	x0, y0 := pageToRasterFloat(bounds.X, bounds.Y, crop, dst.Bounds())
	right, _ := bounds.Right()
	bottom, _ := bounds.Bottom()
	x1, y1 := pageToRasterFloat(right, bottom, crop, dst.Bounds())
	target := image.Rect(int(x0+0.5), int(y0+0.5), int(x1+0.5), int(y1+0.5)).Intersect(dst.Bounds()).Intersect(clip)
	if target.Empty() {
		return nil
	}
	sourceBounds := source.Bounds()
	if placement.Crop != nil {
		c := placement.Crop
		sx0 := int(float64(c.Source.X)*float64(sourceBounds.Dx())/float64(c.Intrinsic.Width) + 0.5)
		sy0 := int(float64(c.Source.Y)*float64(sourceBounds.Dy())/float64(c.Intrinsic.Height) + 0.5)
		sx1 := int(float64(c.Source.X+c.Source.Width)*float64(sourceBounds.Dx())/float64(c.Intrinsic.Width) + 0.5)
		sy1 := int(float64(c.Source.Y+c.Source.Height)*float64(sourceBounds.Dy())/float64(c.Intrinsic.Height) + 0.5)
		sourceBounds = image.Rect(sx0, sy0, sx1, sy1).Intersect(source.Bounds())
	}
	// Deterministic nearest-neighbor mapping.
	for y := target.Min.Y; y < target.Max.Y; y++ {
		sy := sourceBounds.Min.Y + (y-target.Min.Y)*sourceBounds.Dy()/target.Dy()
		for x := target.Min.X; x < target.Max.X; x++ {
			sx := sourceBounds.Min.X + (x-target.Min.X)*sourceBounds.Dx()/target.Dx()
			value := color.NRGBAModel.Convert(source.At(sx, sy)).(color.NRGBA)
			alpha := uint32(rasterOpacityAlpha(value.A, placement.Opacity))
			background := dst.RGBAAt(x, y)
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((uint32(value.R)*alpha + uint32(background.R)*(255-alpha) + 127) / 255),
				G: uint8((uint32(value.G)*alpha + uint32(background.G)*(255-alpha) + 127) / 255),
				B: uint8((uint32(value.B)*alpha + uint32(background.B)*(255-alpha) + 127) / 255), A: 255,
			})
		}
	}
	return nil
}

func rasterOpacityAlpha(source uint8, opacity Fixed) uint8 {
	if opacity == 0 {
		return source
	}
	return uint8((uint32(source)*uint32(opacity) + uint32(FixedScale)/2) / uint32(FixedScale))
}

func rasterRectangularPath(path PlannedPath) bool {
	if len(path.Segments) != 5 || path.Segments[0].Kind != PathMoveTo || path.Segments[1].Kind != PathLineTo ||
		path.Segments[2].Kind != PathLineTo || path.Segments[3].Kind != PathLineTo || path.Segments[4].Kind != PathClose {
		return false
	}
	p := []Point{path.Segments[0].Point, path.Segments[1].Point, path.Segments[2].Point, path.Segments[3].Point}
	return p[0] == (Point{X: path.Bounds.X, Y: path.Bounds.Y}) &&
		p[1] == (Point{X: path.Bounds.X + path.Bounds.Width, Y: path.Bounds.Y}) &&
		p[2] == (Point{X: path.Bounds.X + path.Bounds.Width, Y: path.Bounds.Y + path.Bounds.Height}) &&
		p[3] == (Point{X: path.Bounds.X, Y: path.Bounds.Y + path.Bounds.Height})
}

func pageToRasterFloat(x, y Fixed, crop Rect, bounds image.Rectangle) (float32, float32) {
	return float32(float64(x-crop.X) * float64(bounds.Dx()) / float64(crop.Width)), float32(float64(y-crop.Y) * float64(bounds.Dy()) / float64(crop.Height))
}
func pageToRaster26_6(value, origin Fixed, pixels uint32, extent Fixed) fixed.Int26_6 {
	numerator := int64(value-origin) * int64(pixels) * 64
	denominator := int64(extent)
	if numerator < 0 {
		return fixed.Int26_6(-((-numerator + denominator/2) / denominator))
	}
	return fixed.Int26_6((numerator + denominator/2) / denominator)
}
func rasterPixelExtent(extent Fixed, dpi uint32) (uint32, error) {
	value := (int64(extent)*int64(dpi) + 36*FixedScale) / (72 * FixedScale)
	if value <= 0 || value > int64(^uint32(0)) {
		return 0, fmt.Errorf("%w: dimension", ErrDisplayRasterLimit)
	}
	return uint32(value), nil
}
func rectContainsRect(outer, inner Rect) bool {
	or, _ := outer.Right()
	ob, _ := outer.Bottom()
	ir, _ := inner.Right()
	ib, _ := inner.Bottom()
	return inner.X >= outer.X && inner.Y >= outer.Y && ir <= or && ib <= ob
}
func validateDigestString(value string) error {
	if len(value) != 64 {
		return errors.New("digest")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return errors.New("digest")
	}
	return nil
}

type rasterLimitWriter struct {
	writer    *bytes.Buffer
	remaining uint64
}

func (w *rasterLimitWriter) Write(p []byte) (int, error) {
	if uint64(len(p)) > w.remaining {
		return 0, ErrDisplayRasterLimit
	}
	n, err := w.writer.Write(p)
	w.remaining -= uint64(n)
	return n, err
}
