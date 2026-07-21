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
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

const svgDisplayGradientBands = 64
const svgDisplayPatternPaintLimit = 256

var (
	// ErrSVGDisplayPlanUnsupported reports parsed SVG content that the
	// canonical display list cannot replay exactly.
	ErrSVGDisplayPlanUnsupported = errors.New("document: SVG display-plan feature is unsupported")
	// ErrSVGDisplayPlanLimit reports SVG graphics that exceed an explicit
	// lowering resource bound.
	ErrSVGDisplayPlanLimit = errors.New("document: SVG display-plan limit exceeded")
)

// SVGDisplayPlanPlacement maps parsed SVG user coordinates into page-space
// PDF points. Scale must be finite and positive.
type SVGDisplayPlanPlacement struct {
	Page     uint32
	Fragment layoutengine.FragmentID
	X        float64
	Y        float64
	Scale    float64
	LinkURI  string
}

// SVGDisplayPlanLimits bound work before any resources are attached to a
// plan. Values must be positive.
type SVGDisplayPlanLimits struct {
	MaxPaths      int
	MaxSegments   int
	MaxPaintItems int
}

// DefaultSVGDisplayPlanLimits returns conservative bounds below the layout
// engine's hard resource ceilings.
func DefaultSVGDisplayPlanLimits() SVGDisplayPlanLimits {
	return SVGDisplayPlanLimits{MaxPaths: 1 << 16, MaxSegments: 1 << 20, MaxPaintItems: 1 << 17}
}

// AttachSVGDisplayPlan lowers the safe parsed SVG subset into immutable,
// page-space display resources. Supported content includes transformed paths,
// core-font text, embedded PNG/JPEG images, and path clips. Paint must remain
// explicit, opaque, and deterministic; unsupported content rejects the whole
// fragment atomically.
func AttachSVGDisplayPlan(plan layoutengine.LayoutPlan, svg *SVG, placement SVGDisplayPlanPlacement) (layoutengine.LayoutPlan, error) {
	return AttachSVGDisplayPlanContext(context.Background(), plan, svg, placement, DefaultSVGDisplayPlanLimits())
}

// AttachSVGDisplayPlanContext is AttachSVGDisplayPlan with cancellation and
// caller-selected work bounds.
func AttachSVGDisplayPlanContext(ctx context.Context, plan layoutengine.LayoutPlan, svg *SVG, placement SVGDisplayPlanPlacement, limits SVGDisplayPlanLimits) (layoutengine.LayoutPlan, error) {
	if err := ctx.Err(); err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	if svg == nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("%w: nil SVG", ErrSVGDisplayPlanUnsupported)
	}
	if placement.Page == 0 || !svgFinite(placement.X) || !svgFinite(placement.Y) || !svgFinite(placement.Scale) || placement.Scale <= 0 {
		return layoutengine.LayoutPlan{}, fmt.Errorf("%w: placement requires a page and finite positive scale", ErrSVGDisplayPlanUnsupported)
	}
	if limits.MaxPaths <= 0 || limits.MaxSegments <= 0 || limits.MaxPaintItems <= 0 {
		return layoutengine.LayoutPlan{}, fmt.Errorf("%w: limits must be positive", ErrSVGDisplayPlanLimit)
	}
	if len(svg.Elements) > limits.MaxPaths {
		return layoutengine.LayoutPlan{}, fmt.Errorf("%w: element count", ErrSVGDisplayPlanLimit)
	}
	var encodedImageBytes uint64
	for _, element := range svg.Elements {
		if element.Kind == "image" && !element.Image.Style.Hidden {
			encodedImageBytes += uint64(len(element.Image.Data))
			if encodedImageBytes > 64<<20 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%w: aggregate encoded image bytes", ErrSVGDisplayPlanLimit)
			}
		}
	}

	plan, textFonts, textRuns, textRunByElement, err := svgDisplayTextGeometry(plan, svg, placement)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	input := layoutengine.DisplayListInput{
		Fonts: textFonts, GlyphRuns: textRuns,
		Paths:   make([]layoutengine.PlannedPath, 0, len(svg.Elements)),
		Fills:   make([]layoutengine.PlannedFill, 0, len(svg.Elements)),
		Strokes: make([]layoutengine.PlannedStroke, 0, len(svg.Elements)),
		Items:   make([]layoutengine.DisplayItem, 0, len(svg.Elements)*2),
	}
	imageIDs := make(map[layoutengine.ImageContentDigest]layoutengine.ImageResourceID)
	if placement.LinkURI != "" {
		projection := plan.Projection()
		var owner layoutengine.Fragment
		for _, fragment := range projection.Fragments {
			if fragment.ID == placement.Fragment {
				owner = fragment
				break
			}
		}
		if !owner.ID.Valid() || owner.Page != placement.Page {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%w: linked placement requires a fragment on its page", ErrSVGDisplayPlanUnsupported)
		}
		input.Links = []layoutengine.PlannedLink{{Fragment: owner.ID, Bounds: owner.BorderBox, URI: placement.LinkURI, Source: owner.Source}}
	}
	segments := 0
	for index := range svg.Elements {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return layoutengine.LayoutPlan{}, err
			}
		}
		element := svg.Elements[index]
		style := element.Path.Style
		if element.Kind == "text" {
			style = element.Text.Style
		}
		if element.Kind == "image" {
			style = element.Image.Style
		}
		if style.Hidden {
			continue
		}
		clipped := len(style.ClipPath) != 0
		if clipped {
			if len(input.Paths) == limits.MaxPaths || len(input.Items) > limits.MaxPaintItems-3 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%w: clip path or paint item count", ErrSVGDisplayPlanLimit)
			}
			clipPath, clipErr := svgDisplayPath(style.ClipPath, placement)
			if clipErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d clip: %w", index, clipErr)
			}
			if len(clipPath.Segments) > limits.MaxSegments-segments {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%w: clip path or segment count", ErrSVGDisplayPlanLimit)
			}
			segments += len(clipPath.Segments)
			clipRule := layoutengine.FillNonZero
			if style.ClipRule == "evenodd" {
				clipRule = layoutengine.FillEvenOdd
			} else if style.ClipRule != "" && style.ClipRule != "nonzero" {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w: clip rule", index, ErrSVGDisplayPlanUnsupported)
			}
			input.Paths = append(input.Paths, clipPath)
			input.Clips = append(input.Clips, layoutengine.PlannedClip{Path: uint32(len(input.Paths) - 1), Rule: clipRule, Fragment: placement.Fragment})
			input.Items = append(input.Items,
				layoutengine.DisplayItem{Kind: layoutengine.CommandSaveState, Page: placement.Page},
				layoutengine.DisplayItem{Kind: layoutengine.CommandClip, Payload: uint32(len(input.Clips) - 1), Page: placement.Page})
		}
		if element.Kind == "text" {
			if len(input.Items) == limits.MaxPaintItems {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%w: paint item count", ErrSVGDisplayPlanLimit)
			}
			run, ok := textRunByElement[index]
			if !ok {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%w: element %d text geometry is absent", ErrSVGDisplayPlanUnsupported, index)
			}
			input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: run, Page: placement.Page})
			if clipped {
				input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandRestoreState, Page: placement.Page})
			}
			continue
		}
		if element.Kind == "image" {
			if len(input.Items) == limits.MaxPaintItems {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%w: paint item count", ErrSVGDisplayPlanLimit)
			}
			resource, imagePlacement, imageErr := svgDisplayImage(element.Image, placement, placement.Fragment)
			if imageErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, imageErr)
			}
			resourceID := imageIDs[resource.Digest]
			if !resourceID.Valid() {
				resourceID = layoutengine.ImageResourceID(len(input.ImageResources) + 1)
				resource.ID = resourceID
				input.ImageResources = append(input.ImageResources, resource)
				imageIDs[resource.Digest] = resourceID
			}
			imagePlacement.Resource = resourceID
			input.Images = append(input.Images, imagePlacement)
			input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandImage, Payload: uint32(len(input.Images) - 1), Page: placement.Page})
			if clipped {
				input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandRestoreState, Page: placement.Page})
			}
			continue
		}
		if element.Kind != "path" {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%w: element %d kind %q", ErrSVGDisplayPlanUnsupported, index, element.Kind)
		}
		gradient := element.Path.Style.FillGradient.Set
		pattern := element.Path.Style.FillPattern.Set
		pathResources := 1
		if gradient {
			pathResources += svgDisplayGradientBands
		}
		if pathResources > limits.MaxPaths-len(input.Paths) || len(element.Path.Segments) > limits.MaxSegments-segments {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%w: path or segment count", ErrSVGDisplayPlanLimit)
		}
		paintStyle := element.Path.Style
		if gradient || pattern {
			paintStyle.FillGradient = SVGGradient{}
			paintStyle.FillPattern = SVGPattern{}
			paintStyle.FillRef = ""
			paintStyle.Fill = CSSColorType{Set: true}
		}
		fill, stroke, err := svgDisplayPaint(paintStyle)
		if err != nil {
			return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, err)
		}
		paintCount := 0
		if gradient {
			paintCount += svgDisplayGradientBands + 3
		} else if pattern {
			// The exact bounded tile count is checked by the pattern lowerer.
		} else if fill.Set && !fill.None {
			paintCount++
		}
		if stroke.Set && !stroke.None {
			paintCount++
		}
		if paintCount > limits.MaxPaintItems-len(input.Items) {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%w: paint item count", ErrSVGDisplayPlanLimit)
		}
		path, err := svgDisplayPath(element.Path.Segments, placement)
		if err != nil {
			return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, err)
		}
		segments += len(path.Segments)
		pathIndex := uint32(len(input.Paths))
		input.Paths = append(input.Paths, path)
		if gradient {
			if err := svgDisplayAppendGradient(&input, element.Path, pathIndex, placement, limits, &segments); err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, err)
			}
		} else if pattern {
			if err := svgDisplayAppendPattern(&input, element.Path, pathIndex, placement, limits, &segments); err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, err)
			}
		} else if fill.Set && !fill.None {
			rule := layoutengine.FillNonZero
			if element.Path.Style.FillRule == "evenodd" {
				rule = layoutengine.FillEvenOdd
			}
			opacity, opacityErr := svgDisplayOpacity(element.Path.Style, false, true)
			if opacityErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, opacityErr)
			}
			input.Fills = append(input.Fills, layoutengine.PlannedFill{Path: pathIndex, Rule: rule, Color: svgDisplayColor(fill), Opacity: opacity, Fragment: placement.Fragment})
			input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(input.Fills) - 1), Page: placement.Page})
		}
		if stroke.Set && !stroke.None {
			width, widthErr := layoutengine.FixedFromPoints(element.Path.Style.StrokeWidth * placement.Scale)
			if widthErr != nil || width <= 0 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w: stroke width", index, ErrSVGDisplayPlanUnsupported)
			}
			capStyle, joinStyle, dash, dashOffset, strokeErr := svgDisplayStrokeStyle(element.Path.Style, placement.Scale)
			if strokeErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, strokeErr)
			}
			if (capStyle != layoutengine.StrokeCapButt || joinStyle != layoutengine.StrokeJoinMiter || len(dash) != 0) && !svgDisplayStraightStroke(element.Path.Segments) {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w: styled strokes require straight segments", index, ErrSVGDisplayPlanUnsupported)
			}
			opacity, opacityErr := svgDisplayOpacity(element.Path.Style, true, false)
			if opacityErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("element %d: %w", index, opacityErr)
			}
			input.Strokes = append(input.Strokes, layoutengine.PlannedStroke{Path: pathIndex, Color: svgDisplayColor(stroke), Width: width,
				LineCap: capStyle, LineJoin: joinStyle, Dash: dash, DashOffset: dashOffset, Opacity: opacity, Fragment: placement.Fragment})
			input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(input.Strokes) - 1), Page: placement.Page})
		}
		if clipped {
			input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandRestoreState, Page: placement.Page})
		}
	}
	if err := ctx.Err(); err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	if len(input.Links) != 0 {
		input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandLink, Payload: 0, Page: placement.Page})
	}
	return layoutengine.AttachDisplayList(plan, input)
}

func svgDisplayAppendGradient(input *layoutengine.DisplayListInput, source SVGPath, clipPath uint32, placement SVGDisplayPlanPlacement, limits SVGDisplayPlanLimits, segments *int) error {
	switch source.Style.FillGradient.Kind {
	case "linear":
		return svgDisplayAppendLinearGradient(input, source, clipPath, placement, limits, segments)
	case "radial":
		return svgDisplayAppendRadialGradient(input, source, clipPath, placement, limits, segments)
	default:
		return fmt.Errorf("%w: bounded gradients require a linear or centered radial gradient", ErrSVGDisplayPlanUnsupported)
	}
}

func svgDisplayStraightStroke(segments []SVGSegment) bool {
	for _, segment := range segments {
		switch segment.Cmd {
		case 'M', 'L', 'H', 'V', 'Z':
		default:
			return false
		}
	}
	return true
}

func svgDisplayAppendLinearGradient(input *layoutengine.DisplayListInput, source SVGPath, clipPath uint32, placement SVGDisplayPlanPlacement, limits SVGDisplayPlanLimits, segments *int) error {
	gradient := source.Style.FillGradient
	if gradient.Kind != "linear" || !source.HasBounds || len(gradient.Stops) < 2 || len(gradient.Stops) > 16 {
		return fmt.Errorf("%w: bounded gradients require a linear gradient, cached bounds, and 2-16 stops", ErrSVGDisplayPlanUnsupported)
	}
	for _, stop := range gradient.Stops {
		if !svgDisplayRGBValid(stop.Color) || !svgFinite(stop.Offset) || stop.Offset < 0 || stop.Offset > 1 || !svgFinite(stop.Opacity) || stop.Opacity < 0 || stop.Opacity > 1 {
			return fmt.Errorf("%w: gradient stop color, offset, or opacity", ErrSVGDisplayPlanUnsupported)
		}
	}
	minX, minY, maxX, maxY := source.Bounds[0], source.Bounds[1], source.Bounds[2], source.Bounds[3]
	if !svgFinite(minX) || !svgFinite(minY) || !svgFinite(maxX) || !svgFinite(maxY) || maxX <= minX || maxY <= minY {
		return fmt.Errorf("%w: gradient bounds", ErrSVGDisplayPlanUnsupported)
	}
	gx1, gy1, gx2, gy2 := gradient.X1, gradient.Y1, gradient.X2, gradient.Y2
	if gradient.Units != "userSpaceOnUse" {
		gx1, gy1 = minX+gradient.X1*(maxX-minX), minY+gradient.Y1*(maxY-minY)
		gx2, gy2 = minX+gradient.X2*(maxX-minX), minY+gradient.Y2*(maxY-minY)
	}
	dx, dy := gx2-gx1, gy2-gy1
	lengthSquared := dx*dx + dy*dy
	if !svgFinite(gx1) || !svgFinite(gy1) || !svgFinite(gx2) || !svgFinite(gy2) || lengthSquared <= 1e-18 {
		return fmt.Errorf("%w: bounded gradients require a non-zero finite vector", ErrSVGDisplayPlanUnsupported)
	}
	if len(input.Paths)+svgDisplayGradientBands > limits.MaxPaths || *segments > limits.MaxSegments-5*svgDisplayGradientBands {
		return fmt.Errorf("%w: gradient path or segment count", ErrSVGDisplayPlanLimit)
	}
	opacity, err := svgDisplayOpacity(source.Style, false, true)
	if err != nil {
		return err
	}
	input.Clips = append(input.Clips, layoutengine.PlannedClip{Path: clipPath, Rule: svgDisplayFillRule(source.Style.FillRule), Fragment: placement.Fragment})
	input.Items = append(input.Items,
		layoutengine.DisplayItem{Kind: layoutengine.CommandSaveState, Page: placement.Page},
		layoutengine.DisplayItem{Kind: layoutengine.CommandClip, Payload: uint32(len(input.Clips) - 1), Page: placement.Page})
	corners := [][2]float64{{minX, minY}, {maxX, minY}, {maxX, maxY}, {minX, maxY}}
	minT, maxT := math.Inf(1), math.Inf(-1)
	for _, corner := range corners {
		t := ((corner[0]-gx1)*dx + (corner[1]-gy1)*dy) / lengthSquared
		minT, maxT = math.Min(minT, t), math.Max(maxT, t)
	}
	if !svgFinite(minT) || !svgFinite(maxT) || maxT <= minT {
		return fmt.Errorf("%w: gradient projection", ErrSVGDisplayPlanUnsupported)
	}
	extent := math.Hypot(maxX-minX, maxY-minY) + math.Sqrt(lengthSquared)
	for band := 0; band < svgDisplayGradientBands; band++ {
		start := minT + (maxT-minT)*float64(band)/svgDisplayGradientBands
		end := minT + (maxT-minT)*float64(band+1)/svgDisplayGradientBands
		t := (start + end) / 2
		path, pathErr := svgDisplayPath(svgGradientBandSegments(gx1, gy1, dx, dy, start, end, extent), placement)
		if pathErr != nil {
			return pathErr
		}
		bandOpacity, visible, opacityErr := svgDisplayGradientOpacity(gradient.Stops, t, opacity)
		if opacityErr != nil {
			return opacityErr
		}
		if !visible {
			continue
		}
		input.Paths = append(input.Paths, path)
		input.Fills = append(input.Fills, layoutengine.PlannedFill{Path: uint32(len(input.Paths) - 1), Rule: layoutengine.FillNonZero,
			Color: svgDisplayGradientColor(gradient.Stops, t), Opacity: bandOpacity, Fragment: placement.Fragment})
		input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(input.Fills) - 1), Page: placement.Page})
	}
	*segments += 5 * svgDisplayGradientBands
	input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandRestoreState, Page: placement.Page})
	return nil
}

func svgDisplayAppendRadialGradient(input *layoutengine.DisplayListInput, source SVGPath, clipPath uint32, placement SVGDisplayPlanPlacement, limits SVGDisplayPlanLimits, segments *int) error {
	gradient := source.Style.FillGradient
	if gradient.Kind != "radial" || !source.HasBounds || len(gradient.Stops) < 2 || len(gradient.Stops) > 16 {
		return fmt.Errorf("%w: bounded radial gradients require cached bounds and 2-16 stops", ErrSVGDisplayPlanUnsupported)
	}
	for _, stop := range gradient.Stops {
		if !svgDisplayRGBValid(stop.Color) || !svgFinite(stop.Offset) || stop.Offset < 0 || stop.Offset > 1 || !svgFinite(stop.Opacity) || stop.Opacity < 0 || stop.Opacity > 1 {
			return fmt.Errorf("%w: gradient stop color, offset, or opacity", ErrSVGDisplayPlanUnsupported)
		}
	}
	minX, minY, maxX, maxY := source.Bounds[0], source.Bounds[1], source.Bounds[2], source.Bounds[3]
	if !svgFinite(minX) || !svgFinite(minY) || !svgFinite(maxX) || !svgFinite(maxY) || maxX <= minX || maxY <= minY {
		return fmt.Errorf("%w: gradient bounds", ErrSVGDisplayPlanUnsupported)
	}
	cx, cy, fx, fy, rx, ry := gradient.CX, gradient.CY, gradient.FX, gradient.FY, gradient.R, gradient.R
	if gradient.Units != "userSpaceOnUse" {
		cx, cy = minX+gradient.CX*(maxX-minX), minY+gradient.CY*(maxY-minY)
		fx, fy = minX+gradient.FX*(maxX-minX), minY+gradient.FY*(maxY-minY)
		rx, ry = gradient.R*(maxX-minX), gradient.R*(maxY-minY)
	}
	if !svgFinite(cx) || !svgFinite(cy) || !svgFinite(fx) || !svgFinite(fy) || !svgFinite(rx) || !svgFinite(ry) || rx <= 0 || ry <= 0 || math.Abs(fx-cx) > 1e-9 || math.Abs(fy-cy) > 1e-9 {
		return fmt.Errorf("%w: bounded radial gradients require a positive radius and centered focus", ErrSVGDisplayPlanUnsupported)
	}
	const radialSegmentsPerBand = 12
	if len(input.Paths)+svgDisplayGradientBands > limits.MaxPaths || *segments > limits.MaxSegments-radialSegmentsPerBand*svgDisplayGradientBands {
		return fmt.Errorf("%w: radial gradient path or segment count", ErrSVGDisplayPlanLimit)
	}
	opacity, err := svgDisplayOpacity(source.Style, false, true)
	if err != nil {
		return err
	}
	maxT := 1.0
	for _, corner := range [][2]float64{{minX, minY}, {maxX, minY}, {maxX, maxY}, {minX, maxY}} {
		distance := math.Hypot((corner[0]-cx)/rx, (corner[1]-cy)/ry)
		maxT = math.Max(maxT, distance)
	}
	if !svgFinite(maxT) || maxT <= 0 {
		return fmt.Errorf("%w: radial gradient projection", ErrSVGDisplayPlanUnsupported)
	}
	input.Clips = append(input.Clips, layoutengine.PlannedClip{Path: clipPath, Rule: svgDisplayFillRule(source.Style.FillRule), Fragment: placement.Fragment})
	input.Items = append(input.Items,
		layoutengine.DisplayItem{Kind: layoutengine.CommandSaveState, Page: placement.Page},
		layoutengine.DisplayItem{Kind: layoutengine.CommandClip, Payload: uint32(len(input.Clips) - 1), Page: placement.Page})
	for band := svgDisplayGradientBands - 1; band >= 0; band-- {
		inner := maxT * float64(band) / svgDisplayGradientBands
		outer := maxT * float64(band+1) / svgDisplayGradientBands
		t := (inner + outer) / 2
		bandSegments := svgEllipseSegments(cx, cy, rx*outer, ry*outer)
		if inner > 0 {
			bandSegments = append(bandSegments, svgDisplayReverseEllipseSegments(cx, cy, rx*inner, ry*inner)...)
		}
		path, pathErr := svgDisplayPath(bandSegments, placement)
		if pathErr != nil {
			return pathErr
		}
		bandOpacity, visible, opacityErr := svgDisplayGradientOpacity(gradient.Stops, t, opacity)
		if opacityErr != nil {
			return opacityErr
		}
		if !visible {
			continue
		}
		input.Paths = append(input.Paths, path)
		input.Fills = append(input.Fills, layoutengine.PlannedFill{Path: uint32(len(input.Paths) - 1), Rule: layoutengine.FillNonZero,
			Color: svgDisplayGradientColor(gradient.Stops, t), Opacity: bandOpacity, Fragment: placement.Fragment})
		input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(input.Fills) - 1), Page: placement.Page})
	}
	*segments += radialSegmentsPerBand * svgDisplayGradientBands
	input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandRestoreState, Page: placement.Page})
	return nil
}

func svgDisplayReverseEllipseSegments(cx, cy, rx, ry float64) []SVGSegment {
	if rx <= 0 || ry <= 0 {
		return nil
	}
	const kappa = 0.5522847498307936
	kx, ky := rx*kappa, ry*kappa
	return []SVGSegment{
		{Cmd: 'M', Arg: [6]float64{cx + rx, cy}},
		{Cmd: 'C', Arg: [6]float64{cx + rx, cy - ky, cx + kx, cy - ry, cx, cy - ry}},
		{Cmd: 'C', Arg: [6]float64{cx - kx, cy - ry, cx - rx, cy - ky, cx - rx, cy}},
		{Cmd: 'C', Arg: [6]float64{cx - rx, cy + ky, cx - kx, cy + ry, cx, cy + ry}},
		{Cmd: 'C', Arg: [6]float64{cx + kx, cy + ry, cx + rx, cy + ky, cx + rx, cy}},
		{Cmd: 'Z'},
	}
}

func svgGradientBandSegments(x, y, dx, dy, start, end, extent float64) []SVGSegment {
	length := math.Hypot(dx, dy)
	nx, ny := -dy/length*extent, dx/length*extent
	x0, y0 := x+start*dx, y+start*dy
	x1, y1 := x+end*dx, y+end*dy
	return []SVGSegment{
		{Cmd: 'M', Arg: [6]float64{x0 - nx, y0 - ny}},
		{Cmd: 'L', Arg: [6]float64{x0 + nx, y0 + ny}},
		{Cmd: 'L', Arg: [6]float64{x1 + nx, y1 + ny}},
		{Cmd: 'L', Arg: [6]float64{x1 - nx, y1 - ny}},
		{Cmd: 'Z'},
	}
}

func svgDisplayGradientOpacity(stops []SVGGradientStop, t float64, base layoutengine.Fixed) (layoutengine.Fixed, bool, error) {
	stopOpacity := stops[len(stops)-1].Opacity
	if t <= stops[0].Offset {
		stopOpacity = stops[0].Opacity
	} else if t < stops[len(stops)-1].Offset {
		for index := 1; index < len(stops); index++ {
			right := stops[index]
			if t > right.Offset {
				continue
			}
			left := stops[index-1]
			span := right.Offset - left.Offset
			if span <= 0 {
				stopOpacity = right.Opacity
			} else {
				stopOpacity = left.Opacity + (right.Opacity-left.Opacity)*(t-left.Offset)/span
			}
			break
		}
	}
	overall := 1.0
	if base != 0 {
		overall = base.Points()
	}
	value := overall * stopOpacity
	if !svgFinite(value) || value < 0 || value > 1 {
		return 0, false, fmt.Errorf("%w: gradient opacity precision", ErrSVGDisplayPlanUnsupported)
	}
	if value <= 0 {
		return 0, false, nil
	}
	if value >= 1 {
		return 0, true, nil
	}
	fixed, err := layoutengine.FixedFromPoints(value)
	if err != nil || fixed <= 0 || fixed >= layoutengine.Fixed(layoutengine.FixedScale) {
		return 0, false, fmt.Errorf("%w: gradient opacity precision", ErrSVGDisplayPlanUnsupported)
	}
	return fixed, true, nil
}

func svgDisplayAppendPattern(input *layoutengine.DisplayListInput, source SVGPath, clipPath uint32, placement SVGDisplayPlanPlacement, limits SVGDisplayPlanLimits, segments *int) error {
	minX, minY, maxX, maxY, ok := svgPathCachedBounds(source)
	pattern := source.Style.FillPattern
	if !ok || maxX <= minX || maxY <= minY || len(pattern.Elements) == 0 || len(pattern.Elements) > 16 {
		return fmt.Errorf("%w: bounded pattern geometry or element count", ErrSVGDisplayPlanUnsupported)
	}
	tileX, tileY, tileWidth, tileHeight := svgPatternTile(pattern, minX, minY, maxX, maxY)
	if !svgFinite(tileX) || !svgFinite(tileY) || !svgFinite(tileWidth) || !svgFinite(tileHeight) || tileWidth <= 0 || tileHeight <= 0 {
		return fmt.Errorf("%w: pattern tile geometry", ErrSVGDisplayPlanUnsupported)
	}
	for _, element := range pattern.Elements {
		if element.Kind != "path" || !element.Path.HasBounds || element.Path.Style.Hidden || len(element.Path.Style.ClipPath) != 0 ||
			element.Path.Bounds[0] < 0 || element.Path.Bounds[1] < 0 || element.Path.Bounds[2] > tileWidth || element.Path.Bounds[3] > tileHeight {
			return fmt.Errorf("%w: bounded patterns require visible in-tile path elements", ErrSVGDisplayPlanUnsupported)
		}
		fill, stroke, err := svgDisplayPaint(element.Path.Style)
		if err != nil || !fill.Set || fill.None || stroke.Set && !stroke.None {
			return fmt.Errorf("%w: pattern elements require one opaque solid fill", ErrSVGDisplayPlanUnsupported)
		}
		if opacity, opacityErr := svgDisplayOpacity(element.Path.Style, false, true); opacityErr != nil || opacity != 0 {
			return fmt.Errorf("%w: pattern element opacity", ErrSVGDisplayPlanUnsupported)
		}
	}
	minColumn := int(math.Floor((minX - tileX) / tileWidth))
	maxColumn := int(math.Ceil((maxX-tileX)/tileWidth)) - 1
	minRow := int(math.Floor((minY - tileY) / tileHeight))
	maxRow := int(math.Ceil((maxY-tileY)/tileHeight)) - 1
	columns, rows := maxColumn-minColumn+1, maxRow-minRow+1
	if columns <= 0 || rows <= 0 || columns > svgDisplayPatternPaintLimit || rows > svgDisplayPatternPaintLimit || columns*rows*len(pattern.Elements) > svgDisplayPatternPaintLimit {
		return fmt.Errorf("%w: pattern tile count", ErrSVGDisplayPlanLimit)
	}
	paintCount := columns * rows * len(pattern.Elements)
	segmentCount := 0
	for _, element := range pattern.Elements {
		segmentCount += len(element.Path.Segments) * columns * rows
	}
	if len(input.Paths)+paintCount > limits.MaxPaths || segmentCount > limits.MaxSegments-*segments || len(input.Items)+paintCount+3 > limits.MaxPaintItems {
		return fmt.Errorf("%w: pattern paths, segments, or paint items", ErrSVGDisplayPlanLimit)
	}
	opacity, err := svgDisplayOpacity(source.Style, false, true)
	if err != nil {
		return err
	}
	input.Clips = append(input.Clips, layoutengine.PlannedClip{Path: clipPath, Rule: svgDisplayFillRule(source.Style.FillRule), Fragment: placement.Fragment})
	input.Items = append(input.Items,
		layoutengine.DisplayItem{Kind: layoutengine.CommandSaveState, Page: placement.Page},
		layoutengine.DisplayItem{Kind: layoutengine.CommandClip, Payload: uint32(len(input.Clips) - 1), Page: placement.Page})
	for row := minRow; row <= maxRow; row++ {
		for column := minColumn; column <= maxColumn; column++ {
			tilePlacement := placement
			tilePlacement.X += placement.Scale * (tileX + float64(column)*tileWidth)
			tilePlacement.Y += placement.Scale * (tileY + float64(row)*tileHeight)
			for _, element := range pattern.Elements {
				path, pathErr := svgDisplayPath(element.Path.Segments, tilePlacement)
				if pathErr != nil {
					return pathErr
				}
				fill, _, _ := svgDisplayPaint(element.Path.Style)
				input.Paths = append(input.Paths, path)
				input.Fills = append(input.Fills, layoutengine.PlannedFill{Path: uint32(len(input.Paths) - 1), Rule: svgDisplayFillRule(element.Path.Style.FillRule),
					Color: svgDisplayColor(fill), Opacity: opacity, Fragment: placement.Fragment})
				input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(input.Fills) - 1), Page: placement.Page})
			}
		}
	}
	*segments += segmentCount
	input.Items = append(input.Items, layoutengine.DisplayItem{Kind: layoutengine.CommandRestoreState, Page: placement.Page})
	return nil
}

func svgDisplayFillRule(rule string) layoutengine.FillRule {
	if rule == "evenodd" {
		return layoutengine.FillEvenOdd
	}
	return layoutengine.FillNonZero
}

func svgDisplayGradientColor(stops []SVGGradientStop, t float64) layoutengine.CoreRGBColor {
	if t <= stops[0].Offset {
		return svgDisplayColor(stops[0].Color)
	}
	last := stops[len(stops)-1]
	if t >= last.Offset {
		return svgDisplayColor(last.Color)
	}
	for index := 1; index < len(stops); index++ {
		right := stops[index]
		if t > right.Offset {
			continue
		}
		left := stops[index-1]
		span := right.Offset - left.Offset
		if span <= 0 {
			return svgDisplayColor(right.Color)
		}
		mix := (t - left.Offset) / span
		channel := func(a, b int) uint8 { return uint8(math.Floor(float64(a) + (float64(b-a) * mix) + 0.5)) }
		return layoutengine.CoreRGBColor{R: channel(left.Color.R, right.Color.R), G: channel(left.Color.G, right.Color.G), B: channel(left.Color.B, right.Color.B), Set: true}
	}
	return svgDisplayColor(last.Color)
}

func svgDisplayPaint(style SVGStyle) (CSSColorType, CSSColorType, error) {
	if style.FillGradient.Set || style.FillPattern.Set || (style.ClipRef != "" && len(style.ClipPath) == 0) || style.FillRef != "" {
		return CSSColorType{}, CSSColorType{}, fmt.Errorf("%w: complex paint", ErrSVGDisplayPlanUnsupported)
	}
	if style.FillRule != "" && style.FillRule != "nonzero" && style.FillRule != "evenodd" {
		return CSSColorType{}, CSSColorType{}, fmt.Errorf("%w: fill rule", ErrSVGDisplayPlanUnsupported)
	}
	if !style.Fill.Set && !style.Stroke.Set {
		return CSSColorType{}, CSSColorType{}, fmt.Errorf("%w: paint must be explicit", ErrSVGDisplayPlanUnsupported)
	}
	if (!style.Fill.Set || style.Fill.None) && (!style.Stroke.Set || style.Stroke.None) {
		return CSSColorType{}, CSSColorType{}, fmt.Errorf("%w: path has no visible paint", ErrSVGDisplayPlanUnsupported)
	}
	if style.Stroke.Set && !style.Stroke.None && style.StrokeWidth <= 0 {
		return CSSColorType{}, CSSColorType{}, fmt.Errorf("%w: stroke width must be explicit and positive", ErrSVGDisplayPlanUnsupported)
	}
	if (style.Fill.Set && !style.Fill.None && !svgDisplayRGBValid(style.Fill)) || (style.Stroke.Set && !style.Stroke.None && !svgDisplayRGBValid(style.Stroke)) {
		return CSSColorType{}, CSSColorType{}, fmt.Errorf("%w: RGB paint", ErrSVGDisplayPlanUnsupported)
	}
	return style.Fill, style.Stroke, nil
}

func svgDisplayOpacity(style SVGStyle, stroke, fill bool) (layoutengine.Fixed, error) {
	value := svgStyleOpacity(style, stroke, fill)
	if !svgFinite(value) || value <= 0 || value > 1 {
		return 0, fmt.Errorf("%w: opacity must be greater than zero and at most one", ErrSVGDisplayPlanUnsupported)
	}
	if value == 1 {
		return 0, nil
	}
	fixed, err := layoutengine.FixedFromPoints(value)
	if err != nil || fixed <= 0 || fixed >= layoutengine.Fixed(layoutengine.FixedScale) {
		return 0, fmt.Errorf("%w: opacity precision", ErrSVGDisplayPlanUnsupported)
	}
	return fixed, nil
}

func svgDisplayStrokeStyle(style SVGStyle, scale float64) (layoutengine.StrokeLineCap, layoutengine.StrokeLineJoin, []layoutengine.Fixed, layoutengine.Fixed, error) {
	capStyle := layoutengine.StrokeCapButt
	switch style.StrokeLineCap {
	case "", "butt":
	case "round":
		capStyle = layoutengine.StrokeCapRound
	case "square":
		capStyle = layoutengine.StrokeCapSquare
	default:
		return "", "", nil, 0, fmt.Errorf("%w: stroke line cap", ErrSVGDisplayPlanUnsupported)
	}
	joinStyle := layoutengine.StrokeJoinMiter
	switch style.StrokeLineJoin {
	case "", "miter":
	case "round":
		joinStyle = layoutengine.StrokeJoinRound
	case "bevel":
		joinStyle = layoutengine.StrokeJoinBevel
	default:
		return "", "", nil, 0, fmt.Errorf("%w: stroke line join", ErrSVGDisplayPlanUnsupported)
	}
	if len(style.StrokeDashArray) > 128 {
		return "", "", nil, 0, fmt.Errorf("%w: stroke dash count", ErrSVGDisplayPlanLimit)
	}
	dashValues := style.StrokeDashArray
	if len(dashValues)%2 == 1 {
		dashValues = append(append([]float64(nil), dashValues...), dashValues...)
	}
	dash := make([]layoutengine.Fixed, len(dashValues))
	positive := false
	for index, value := range dashValues {
		fixed, err := layoutengine.FixedFromPoints(value * scale)
		if err != nil || fixed < 0 {
			return "", "", nil, 0, fmt.Errorf("%w: stroke dash length", ErrSVGDisplayPlanUnsupported)
		}
		dash[index] = fixed
		positive = positive || fixed > 0
	}
	if len(dash) != 0 && !positive {
		return "", "", nil, 0, fmt.Errorf("%w: all-zero stroke dash", ErrSVGDisplayPlanUnsupported)
	}
	offset, err := layoutengine.FixedFromPoints(style.StrokeDashOffset * scale)
	if err != nil {
		return "", "", nil, 0, fmt.Errorf("%w: stroke dash offset", ErrSVGDisplayPlanUnsupported)
	}
	return capStyle, joinStyle, dash, offset, nil
}

func svgDisplayTextGeometry(plan layoutengine.LayoutPlan, svg *SVG, placement SVGDisplayPlanPlacement) (layoutengine.LayoutPlan, []layoutengine.CoreFontResource, []layoutengine.CoreGlyphRun, map[int]uint32, error) {
	textCount := 0
	textBytes := 0
	for _, element := range svg.Elements {
		if element.Kind == "text" && !element.Text.Style.Hidden {
			textCount++
			textBytes += len(element.Text.Text)
		}
	}
	if textCount == 0 {
		return plan, nil, nil, map[int]uint32{}, nil
	}
	if textBytes > 4<<20 {
		return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("%w: SVG text bytes", ErrSVGDisplayPlanLimit)
	}
	projection := plan.Projection()
	var owner layoutengine.Fragment
	for _, fragment := range projection.Fragments {
		if fragment.ID == placement.Fragment {
			owner = fragment
			break
		}
	}
	if !owner.ID.Valid() || owner.Page != placement.Page {
		return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("%w: text requires a fragment on its page", ErrSVGDisplayPlanUnsupported)
	}

	scratch := documentNew("P", "point", "A4", "", Size{})
	scratch.SetFont("Helvetica", "", 12)
	if scratch.err != nil {
		return layoutengine.LayoutPlan{}, nil, nil, nil, scratch.err
	}
	font, err := typedCoreFontResource(scratch.currentFont)
	if err != nil {
		return layoutengine.LayoutPlan{}, nil, nil, nil, err
	}
	lines := make([]layoutengine.PlannedLine, 0, textCount)
	runs := make([]layoutengine.CoreGlyphRun, 0, textCount)
	byElement := make(map[int]uint32, textCount)
	for index, element := range svg.Elements {
		if element.Kind != "text" || element.Text.Style.Hidden {
			continue
		}
		text := element.Text
		style := text.Style
		if text.Text == "" || !isPlannerCoreText(text.Text) || strings.ContainsAny(text.Text, "\r\n") ||
			!svgFinite(text.X) || !svgFinite(text.Y) || !svgFinite(style.FontSize) || style.FontSize <= 0 {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text geometry or encoding", index, ErrSVGDisplayPlanUnsupported)
		}
		fill, stroke, paintErr := svgDisplayPaint(style)
		opacity, opacityErr := svgDisplayOpacity(style, false, true)
		if paintErr != nil || opacityErr != nil || !fill.Set || fill.None || stroke.Set && !stroke.None {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text requires one solid fill", index, ErrSVGDisplayPlanUnsupported)
		}
		fontSizePoints := style.FontSize * placement.Scale
		fontSize, sizeErr := layoutengine.FixedFromPoints(fontSizePoints)
		if sizeErr != nil || fontSize <= 0 {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text font size", index, ErrSVGDisplayPlanUnsupported)
		}
		scratch.SetFontSize(fontSizePoints)
		width, widthErr := layoutengine.FixedFromPoints(scratch.GetStringWidth(text.Text))
		if widthErr != nil || width <= 0 {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text width", index, ErrSVGDisplayPlanUnsupported)
		}
		x, xErr := layoutengine.FixedFromPoints(placement.X + placement.Scale*text.X)
		baseline, yErr := layoutengine.FixedFromPoints(placement.Y + placement.Scale*text.Y)
		if xErr != nil || yErr != nil {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text origin", index, ErrSVGDisplayPlanUnsupported)
		}
		switch style.TextAnchor {
		case "", "start":
		case "middle":
			x -= width / 2
		case "end":
			x -= width
		default:
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text anchor", index, ErrSVGDisplayPlanUnsupported)
		}
		top, topErr := baseline.Sub(fontSize)
		bounds, boundsErr := layoutengine.NewRect(x, top, width, fontSize)
		if topErr != nil || boundsErr != nil {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w: text bounds", index, ErrSVGDisplayPlanUnsupported)
		}
		advances, advanceErr := typedCoreGlyphAdvances(scratch, text.Text, width)
		if advanceErr != nil {
			return layoutengine.LayoutPlan{}, nil, nil, nil, fmt.Errorf("element %d: %w", index, advanceErr)
		}
		lines = append(lines, layoutengine.PlannedLine{Fragment: owner.ID, Index: uint32(len(lines)), Bounds: bounds, Baseline: baseline, Source: owner.Source})
		runs = append(runs, layoutengine.CoreGlyphRun{Font: font.ID, FontSize: fontSize, Color: svgDisplayColor(fill), Opacity: opacity, Origin: layoutengine.Point{X: x, Y: baseline}, Codes: text.Text, Advances: advances, Source: owner.Source})
		byElement[index] = uint32(len(runs) - 1)
	}
	withLines, start, err := layoutengine.AttachOverlayLines(plan, placement.Page, lines)
	if err != nil {
		return layoutengine.LayoutPlan{}, nil, nil, nil, err
	}
	for index := range runs {
		runs[index].Line = start + uint32(index)
	}
	return withLines, []layoutengine.CoreFontResource{font}, runs, byElement, nil
}

func svgDisplayImage(source SVGImage, placement SVGDisplayPlanPlacement, fragment layoutengine.FragmentID) (layoutengine.ImageResource, layoutengine.PlannedImage, error) {
	style := source.Style
	if style.FillGradient.Set || style.FillPattern.Set || style.FillRef != "" || (style.ClipRef != "" && len(style.ClipPath) == 0) ||
		style.Stroke.Set || style.Fill.Set || style.StrokeDashSet {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: complex image paint", ErrSVGDisplayPlanUnsupported)
	}
	if len(source.Data) == 0 || len(source.Data) > maxImageSourceBytes || !svgFinite(source.X) || !svgFinite(source.Y) ||
		!svgFinite(source.Wd) || !svgFinite(source.Ht) || source.Wd <= 0 || source.Ht <= 0 {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: image geometry or encoded bytes", ErrSVGDisplayPlanUnsupported)
	}
	var format layoutengine.ImageFormat
	switch strings.ToLower(source.ImageType) {
	case "png":
		format = layoutengine.ImagePNG
	case "jpg", "jpeg":
		format = layoutengine.ImageJPEG
	default:
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: image format", ErrSVGDisplayPlanUnsupported)
	}
	config, decodedFormat, err := image.DecodeConfig(bytes.NewReader(source.Data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || uint64(config.Width)*uint64(config.Height) > 64<<20 {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: image decode limits", ErrSVGDisplayPlanLimit)
	}
	if decodedFormat != string(format) && !(format == layoutengine.ImageJPEG && decodedFormat == "jpeg") {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: image bytes do not match declared format", ErrSVGDisplayPlanUnsupported)
	}
	if _, verifiedFormat, decodeErr := image.Decode(bytes.NewReader(source.Data)); decodeErr != nil || verifiedFormat != decodedFormat {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: invalid encoded image", ErrSVGDisplayPlanUnsupported)
	}
	x, errX := layoutengine.FixedFromPoints(placement.X + placement.Scale*source.X)
	y, errY := layoutengine.FixedFromPoints(placement.Y + placement.Scale*source.Y)
	width, errW := layoutengine.FixedFromPoints(placement.Scale * source.Wd)
	height, errH := layoutengine.FixedFromPoints(placement.Scale * source.Ht)
	bounds, errBounds := layoutengine.NewRect(x, y, width, height)
	if errX != nil || errY != nil || errW != nil || errH != nil || errBounds != nil || width <= 0 || height <= 0 {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, fmt.Errorf("%w: image bounds", ErrSVGDisplayPlanUnsupported)
	}
	digest := sha256.Sum256(source.Data)
	resource := layoutengine.ImageResource{Digest: layoutengine.ImageContentDigest(hex.EncodeToString(digest[:])), Format: format, PixelWidth: uint32(config.Width), PixelHeight: uint32(config.Height)}
	opacity, opacityErr := svgDisplayOpacity(style, false, false)
	if opacityErr != nil {
		return layoutengine.ImageResource{}, layoutengine.PlannedImage{}, opacityErr
	}
	return resource, layoutengine.PlannedImage{Fragment: fragment, Bounds: bounds, Opacity: opacity}, nil
}

func svgDisplayImageSources(svg *SVG) plannedImageSources {
	result := make(plannedImageSources)
	if svg == nil {
		return result
	}
	for _, element := range svg.Elements {
		if element.Kind != "image" || element.Image.Style.Hidden || len(element.Image.Data) == 0 {
			continue
		}
		digest := sha256.Sum256(element.Image.Data)
		key := layoutengine.ImageContentDigest(hex.EncodeToString(digest[:]))
		if _, exists := result[key]; !exists {
			result[key] = append([]byte(nil), element.Image.Data...)
		}
	}
	return result
}

func svgDisplayRGBValid(color CSSColorType) bool {
	return color.R >= 0 && color.R <= 255 && color.G >= 0 && color.G <= 255 && color.B >= 0 && color.B <= 255
}

func svgDisplayColor(color CSSColorType) layoutengine.CoreRGBColor {
	return layoutengine.CoreRGBColor{R: uint8(color.R), G: uint8(color.G), B: uint8(color.B), Set: true}
}

func svgDisplayPath(source []SVGSegment, placement SVGDisplayPlanPlacement) (layoutengine.PlannedPath, error) {
	if len(source) < 2 {
		return layoutengine.PlannedPath{}, fmt.Errorf("%w: path has no drawing geometry", ErrSVGDisplayPlanUnsupported)
	}
	point := func(x, y float64) (layoutengine.Point, error) {
		fx, err := layoutengine.FixedFromPoints(placement.X + placement.Scale*x)
		if err != nil {
			return layoutengine.Point{}, err
		}
		fy, err := layoutengine.FixedFromPoints(placement.Y + placement.Scale*y)
		return layoutengine.Point{X: fx, Y: fy}, err
	}
	result := make([]layoutengine.PathSegment, 0, len(source))
	var x, y, startX, startY float64
	active := false
	for index, segment := range source {
		if !svgSegmentFinite(segment) {
			return layoutengine.PlannedPath{}, fmt.Errorf("%w: non-finite path coordinate", ErrSVGDisplayPlanUnsupported)
		}
		switch segment.Cmd {
		case 'M':
			x, y = segment.Arg[0], segment.Arg[1]
			startX, startY, active = x, y, true
			p, err := point(x, y)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathMoveTo, Point: p})
		case 'L':
			if !active {
				return layoutengine.PlannedPath{}, fmt.Errorf("%w: segment %d has no active subpath", ErrSVGDisplayPlanUnsupported, index)
			}
			x, y = segment.Arg[0], segment.Arg[1]
			p, err := point(x, y)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathLineTo, Point: p})
		case 'H':
			if !active {
				return layoutengine.PlannedPath{}, fmt.Errorf("%w: segment %d has no active subpath", ErrSVGDisplayPlanUnsupported, index)
			}
			x = segment.Arg[0]
			p, err := point(x, y)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathLineTo, Point: p})
		case 'V':
			if !active {
				return layoutengine.PlannedPath{}, fmt.Errorf("%w: segment %d has no active subpath", ErrSVGDisplayPlanUnsupported, index)
			}
			y = segment.Arg[0]
			p, err := point(x, y)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathLineTo, Point: p})
		case 'C':
			if !active {
				return layoutengine.PlannedPath{}, fmt.Errorf("%w: segment %d has no active subpath", ErrSVGDisplayPlanUnsupported, index)
			}
			c1, err := point(segment.Arg[0], segment.Arg[1])
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			c2, err := point(segment.Arg[2], segment.Arg[3])
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			x, y = segment.Arg[4], segment.Arg[5]
			p, err := point(x, y)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathCubicTo, Control1: c1, Control2: c2, Point: p})
		case 'Q':
			if !active {
				return layoutengine.PlannedPath{}, fmt.Errorf("%w: segment %d has no active subpath", ErrSVGDisplayPlanUnsupported, index)
			}
			qx, qy, endX, endY := segment.Arg[0], segment.Arg[1], segment.Arg[2], segment.Arg[3]
			c1, err := point(x+2*(qx-x)/3, y+2*(qy-y)/3)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			c2, err := point(endX+2*(qx-endX)/3, endY+2*(qy-endY)/3)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			x, y = endX, endY
			p, err := point(x, y)
			if err != nil {
				return layoutengine.PlannedPath{}, err
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathCubicTo, Control1: c1, Control2: c2, Point: p})
		case 'Z':
			if !active {
				return layoutengine.PlannedPath{}, fmt.Errorf("%w: segment %d has no active subpath", ErrSVGDisplayPlanUnsupported, index)
			}
			result = append(result, layoutengine.PathSegment{Kind: layoutengine.PathClose})
			x, y, active = startX, startY, false
		default:
			return layoutengine.PlannedPath{}, fmt.Errorf("%w: path command %q", ErrSVGDisplayPlanUnsupported, segment.Cmd)
		}
	}
	minimum, maximum, ok := svgDisplayPathBounds(result)
	if !ok {
		return layoutengine.PlannedPath{}, fmt.Errorf("%w: path has no points", ErrSVGDisplayPlanUnsupported)
	}
	bounds, err := layoutengine.RectFromPoints(minimum, maximum)
	if err != nil {
		return layoutengine.PlannedPath{}, err
	}
	return layoutengine.PlannedPath{Segments: result, Bounds: bounds}, nil
}

func svgDisplayPathBounds(segments []layoutengine.PathSegment) (layoutengine.Point, layoutengine.Point, bool) {
	var minimum, maximum layoutengine.Point
	set := false
	include := func(point layoutengine.Point) {
		if !set {
			minimum, maximum, set = point, point, true
			return
		}
		if point.X < minimum.X {
			minimum.X = point.X
		}
		if point.Y < minimum.Y {
			minimum.Y = point.Y
		}
		if point.X > maximum.X {
			maximum.X = point.X
		}
		if point.Y > maximum.Y {
			maximum.Y = point.Y
		}
	}
	for _, segment := range segments {
		switch segment.Kind {
		case layoutengine.PathMoveTo, layoutengine.PathLineTo:
			include(segment.Point)
		case layoutengine.PathCubicTo:
			include(segment.Control1)
			include(segment.Control2)
			include(segment.Point)
		}
	}
	return minimum, maximum, set
}
