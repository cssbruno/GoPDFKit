// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// DebugGeometrySVGFormatVersion is the stable geometry-only capture
	// envelope. It intentionally does not version the display-list or PDF
	// renderer because this capture cannot reproduce either.
	DebugGeometrySVGFormatVersion uint16 = 2

	// debugGeometrySVGMaxItems bounds one debug capture to a useful,
	// inspectable page rather than turning it into an unbounded export
	// surface.
	debugGeometrySVGMaxItems = 4096
	debugGeometrySVGMaxBytes = 1 << 20

	// Canvas and marker values are raw Fixed units, just like every SVG
	// coordinate emitted by this file.
	debugGeometrySVGCanvasPadding Fixed = Fixed(8 * FixedScale)
	debugGeometrySVGMarkerRadius  Fixed = Fixed(2 * FixedScale)
	debugGeometrySVGLabelGap      Fixed = Fixed(2 * FixedScale)
	debugGeometrySVGLabelPaddingX Fixed = Fixed(2 * FixedScale)
	debugGeometrySVGLabelWidth    Fixed = Fixed(5 * FixedScale)
	debugGeometrySVGLabelHeight   Fixed = Fixed(10 * FixedScale)
	debugGeometrySVGLabelBaseline Fixed = Fixed(8 * FixedScale)
	debugGeometrySVGLabelFontSize Fixed = Fixed(8 * FixedScale)
)

var (
	// ErrDebugGeometryInvalidPage reports an absent one-based page selector.
	ErrDebugGeometryInvalidPage = errors.New("layoutengine: debug geometry page number is zero")
	// ErrDebugGeometryPageNotFound reports a page selector outside a plan.
	ErrDebugGeometryPageNotFound = errors.New("layoutengine: debug geometry page was not found")
	// ErrDebugGeometryCaptureLimit reports a capture that exceeds its bounded
	// debug-only output budget.
	ErrDebugGeometryCaptureLimit = errors.New("layoutengine: debug geometry capture limit exceeded")
)

// DebugGeometrySVGCapture is a deterministic, one-page, geometry-only debug
// artifact. PageBounds is the planned page surface; CanvasBounds also includes
// emitted off-page geometry and a fixed padding so overflow remains visible.
//
// SVG coordinates are raw Fixed integers: one SVG unit is 1/FixedScale PDF
// point. This is not a display-list preview or a PDF/raster equivalence claim.
type DebugGeometrySVGCapture struct {
	FormatVersion    uint16
	Page             uint32
	PageBounds       Rect
	CanvasBounds     Rect
	FragmentCount    uint32
	BreakMarkerCount uint32
	FixedScale       int64
	SVG              []byte
}

// CaptureDebugGeometrySVG returns only the deterministic SVG bytes for one
// LayoutPlan page. Call LayoutPlan.CaptureDebugGeometrySVGPage when callers
// also need the page and canvas bounds for tool-coordinate mapping.
func CaptureDebugGeometrySVG(plan LayoutPlan, pageNumber uint32) ([]byte, error) {
	capture, err := plan.CaptureDebugGeometrySVGPage(pageNumber)
	if err != nil {
		return nil, err
	}
	return capture.SVG, nil
}

// CaptureDebugGeometrySVGPage returns an in-memory layout geometry debug
// capture for one plan page. It serializes raw Fixed units as exact base-10
// integers and declares that contract in the root SVG metadata, so it never
// rounds planner geometry through float64.
//
// The capture draws only recorded page, fragment, command, diagnostic, and
// committed-break geometry. It deliberately excludes glyph shaping, images,
// paint styles, clips, transforms, annotations, links, tags, and all
// user-authored text. It has no file, browser, renderer, or production-capture
// side effects.
func (p LayoutPlan) CaptureDebugGeometrySVGPage(pageNumber uint32) (DebugGeometrySVGCapture, error) {
	if pageNumber == 0 {
		return DebugGeometrySVGCapture{}, ErrDebugGeometryInvalidPage
	}
	if err := p.Validate(); err != nil {
		return DebugGeometrySVGCapture{}, fmt.Errorf("layoutengine: capture debug geometry from invalid plan: %w", err)
	}
	if uint64(pageNumber) > uint64(len(p.pages)) {
		return DebugGeometrySVGCapture{}, fmt.Errorf("%w: %d", ErrDebugGeometryPageNotFound, pageNumber)
	}

	page := p.pages[int(pageNumber-1)]
	fragmentEnd, fragmentsOK := page.Fragments.end(len(p.fragments))
	commandEnd, commandsOK := page.Commands.end(len(p.commands))
	if !fragmentsOK || !commandsOK {
		return DebugGeometrySVGCapture{}, errors.New("layoutengine: capture debug geometry found an invalid plan page range")
	}
	fragments := p.fragments[int(page.Fragments.Start):fragmentEnd]
	commands := p.commands[int(page.Commands.Start):commandEnd]
	if !debugGeometrySVGWithinItemLimit(len(fragments), len(commands)) {
		return DebugGeometrySVGCapture{}, fmt.Errorf("%w: page %d has more than %d geometry records", ErrDebugGeometryCaptureLimit, page.Number, debugGeometrySVGMaxItems)
	}

	fragmentsByID := make(map[FragmentID]Fragment, len(fragments))
	for _, fragment := range fragments {
		fragmentsByID[fragment.ID] = fragment
	}
	diagnostics, err := capturePageDiagnostics(p.diagnostics, page.Number, fragmentsByID)
	if err != nil {
		return DebugGeometrySVGCapture{}, err
	}
	breaks, err := capturePageBreakMarkers(p.breaks, page.Number, fragmentsByID)
	if err != nil {
		return DebugGeometrySVGCapture{}, err
	}
	if !debugGeometrySVGWithinItemLimit(len(fragments), len(commands), len(diagnostics), len(breaks)) {
		return DebugGeometrySVGCapture{}, fmt.Errorf("%w: page %d has more than %d geometry records", ErrDebugGeometryCaptureLimit, page.Number, debugGeometrySVGMaxItems)
	}

	pageBounds := Rect{Width: page.Size.Width, Height: page.Size.Height}
	canvasBounds, err := captureDebugGeometrySVGCanvas(pageBounds, fragments, commands, diagnostics, breaks)
	if err != nil {
		return DebugGeometrySVGCapture{}, fmt.Errorf("layoutengine: capture debug geometry canvas: %w", err)
	}
	svg, err := writeDebugGeometrySVG(page, pageBounds, canvasBounds, fragments, commands, diagnostics, breaks)
	if err != nil {
		return DebugGeometrySVGCapture{}, err
	}
	return DebugGeometrySVGCapture{
		FormatVersion:    DebugGeometrySVGFormatVersion,
		Page:             page.Number,
		PageBounds:       pageBounds,
		CanvasBounds:     canvasBounds,
		FragmentCount:    uint32(len(fragments)), // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		BreakMarkerCount: uint32(len(breaks)),    // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		FixedScale:       FixedScale,
		SVG:              svg,
	}, nil
}

func capturePageDiagnostics(diagnostics []Diagnostic, pageNumber uint32, fragments map[FragmentID]Fragment) ([]Diagnostic, error) {
	result := make([]Diagnostic, 0)
	for _, diagnostic := range diagnostics {
		if !diagnostic.Location.HasBounds {
			continue
		}
		_, fragmentOnPage := fragments[diagnostic.Location.Fragment]
		if diagnostic.Location.Page != pageNumber &&
			(diagnostic.Location.Page != 0 || !fragmentOnPage) {
			continue
		}
		if !debugGeometrySVGWithinItemLimit(len(result), 1) {
			return nil, fmt.Errorf("%w: page %d has more than %d diagnostics", ErrDebugGeometryCaptureLimit, pageNumber, debugGeometrySVGMaxItems)
		}
		result = append(result, diagnostic)
	}
	return result, nil
}

type debugGeometrySVGBreakMarker struct {
	Decision    BreakDecision
	Direction   string
	Label       string
	X           Fixed
	Y           Fixed
	Bounds      Rect
	LabelBounds Rect
	LabelX      Fixed
	LabelY      Fixed
}

func capturePageBreakMarkers(decisions []BreakDecision, pageNumber uint32, fragments map[FragmentID]Fragment) ([]debugGeometrySVGBreakMarker, error) {
	result := make([]debugGeometrySVGBreakMarker, 0)
	for _, decision := range decisions {
		var (
			direction string
			fragment  Fragment
			x         Fixed
			y         Fixed
			ok        bool
		)
		switch {
		case decision.FromPage == pageNumber:
			direction = "outgoing"
			fragment, ok = fragments[decision.Preceding]
			if !ok {
				return nil, errors.New("layoutengine: outgoing break has no selected-page preceding fragment")
			}
			x = fragment.BorderBox.X
			var err error
			y, err = fragment.BorderBox.Bottom()
			if err != nil {
				return nil, fmt.Errorf("layoutengine: outgoing break marker: %w", err)
			}
		case decision.ToPage == pageNumber:
			direction = "incoming"
			fragment, ok = fragments[decision.Triggering]
			if !ok {
				return nil, errors.New("layoutengine: incoming break has no selected-page triggering fragment")
			}
			x, y = fragment.BorderBox.X, fragment.BorderBox.Y
		default:
			continue
		}
		bounds, err := debugGeometrySVGMarkerBounds(x, y)
		if err != nil {
			return nil, fmt.Errorf("layoutengine: %s break marker bounds: %w", direction, err)
		}
		if !debugGeometrySVGWithinItemLimit(len(result), 1) {
			return nil, fmt.Errorf("%w: page %d has more than %d break markers", ErrDebugGeometryCaptureLimit, pageNumber, debugGeometrySVGMaxItems)
		}
		label := debugGeometrySVGBreakLabel(direction, decision.Reason)
		labelBounds, labelX, labelY, err := debugGeometrySVGLabelGeometry(x, y, label)
		if err != nil {
			return nil, fmt.Errorf("layoutengine: %s break label bounds: %w", direction, err)
		}
		result = append(result, debugGeometrySVGBreakMarker{
			Decision:    decision,
			Direction:   direction,
			Label:       label,
			X:           x,
			Y:           y,
			Bounds:      bounds,
			LabelBounds: labelBounds,
			LabelX:      labelX,
			LabelY:      labelY,
		})
	}
	return result, nil
}

func debugGeometrySVGBreakLabel(direction string, reason BreakReason) string {
	prefix := "in:"
	if direction == "outgoing" {
		prefix = "out:"
	}
	switch reason {
	case BreakInsufficientRemainingBodySpace:
		return prefix + "space"
	case BreakPreviousFragmentOverflow:
		return prefix + "overflow"
	case BreakPaginationConstraint:
		return prefix + "constraint"
	case BreakExplicitPageBreak:
		return prefix + "explicit"
	default:
		// LayoutPlan.Validate rejects unknown reasons. Keep this defensive value
		// fixed and non-disclosing if a future internal caller bypasses it.
		return prefix + "break"
	}
}

func debugGeometrySVGLabelGeometry(x, y Fixed, label string) (Rect, Fixed, Fixed, error) {
	textWidth, err := debugGeometrySVGLabelWidth.MulInt(int64(len(label)))
	if err != nil {
		return Rect{}, 0, 0, err
	}
	paddingWidth, err := debugGeometrySVGLabelPaddingX.MulInt(2)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	width, err := textWidth.Add(paddingWidth)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	markerOffset, err := debugGeometrySVGMarkerRadius.Add(debugGeometrySVGLabelGap)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	left, err := x.Add(markerOffset)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	halfHeight, err := debugGeometrySVGLabelHeight.DivInt(2)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	top, err := y.Sub(halfHeight)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	bounds, err := NewRect(left, top, width, debugGeometrySVGLabelHeight)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	textX, err := left.Add(debugGeometrySVGLabelPaddingX)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	textY, err := top.Add(debugGeometrySVGLabelBaseline)
	if err != nil {
		return Rect{}, 0, 0, err
	}
	return bounds, textX, textY, nil
}

func debugGeometrySVGMarkerBounds(x, y Fixed) (Rect, error) {
	minimumX, err := x.Sub(debugGeometrySVGMarkerRadius)
	if err != nil {
		return Rect{}, err
	}
	minimumY, err := y.Sub(debugGeometrySVGMarkerRadius)
	if err != nil {
		return Rect{}, err
	}
	diameter, err := debugGeometrySVGMarkerRadius.MulInt(2)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(minimumX, minimumY, diameter, diameter)
}

func captureDebugGeometrySVGCanvas(pageBounds Rect, fragments []Fragment, commands []DisplayCommand, diagnostics []Diagnostic, breaks []debugGeometrySVGBreakMarker) (Rect, error) {
	canvas := pageBounds
	add := func(bounds Rect) error {
		var err error
		canvas, err = canvas.Union(bounds)
		return err
	}
	for _, fragment := range fragments {
		if err := add(fragment.BorderBox); err != nil {
			return Rect{}, err
		}
		if err := add(fragment.ContentBox); err != nil {
			return Rect{}, err
		}
	}
	for _, command := range commands {
		if err := add(command.Bounds); err != nil {
			return Rect{}, err
		}
	}
	for _, diagnostic := range diagnostics {
		if err := add(diagnostic.Location.Bounds); err != nil {
			return Rect{}, err
		}
	}
	for _, marker := range breaks {
		if err := add(marker.Bounds); err != nil {
			return Rect{}, err
		}
		if err := add(marker.LabelBounds); err != nil {
			return Rect{}, err
		}
	}
	return debugGeometrySVGExpandCanvas(canvas)
}

func debugGeometrySVGExpandCanvas(bounds Rect) (Rect, error) {
	if err := bounds.Validate(); err != nil {
		return Rect{}, err
	}
	right, err := bounds.Right()
	if err != nil {
		return Rect{}, err
	}
	bottom, err := bounds.Bottom()
	if err != nil {
		return Rect{}, err
	}
	minimumX, minimumY, maximumX, maximumY := bounds.X, bounds.Y, right, bottom
	if value, err := minimumX.Sub(debugGeometrySVGCanvasPadding); err == nil {
		minimumX = value
	}
	if value, err := minimumY.Sub(debugGeometrySVGCanvasPadding); err == nil {
		minimumY = value
	}
	if value, err := maximumX.Add(debugGeometrySVGCanvasPadding); err == nil {
		maximumX = value
	}
	if value, err := maximumY.Add(debugGeometrySVGCanvasPadding); err == nil {
		maximumY = value
	}
	return RectFromPoints(Point{X: minimumX, Y: minimumY}, Point{X: maximumX, Y: maximumY})
}

func writeDebugGeometrySVG(page PlannedPage, pageBounds, canvasBounds Rect, fragments []Fragment, commands []DisplayCommand, diagnostics []Diagnostic, breaks []debugGeometrySVGBreakMarker) ([]byte, error) {
	writer := debugGeometrySVGWriter{limit: debugGeometrySVGMaxBytes}
	writer.write("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	writer.write("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"")
	writer.write(fixedSVGDecimal(canvasBounds.X))
	writer.write(" ")
	writer.write(fixedSVGDecimal(canvasBounds.Y))
	writer.write(" ")
	writer.write(fixedSVGDecimal(canvasBounds.Width))
	writer.write(" ")
	writer.write(fixedSVGDecimal(canvasBounds.Height))
	writer.write("\" preserveAspectRatio=\"xMinYMin meet\" role=\"img\" aria-labelledby=\"layout-debug-title layout-debug-description\" data-format-version=\"")
	writer.write(strconv.FormatUint(uint64(DebugGeometrySVGFormatVersion), 10))
	writer.write("\" data-coordinate-space=\"pdf-fixed\" data-fixed-scale=\"")
	writer.write(strconv.FormatInt(FixedScale, 10))
	writer.write("\" data-page=\"")
	writer.write(strconv.FormatUint(uint64(page.Number), 10))
	writer.write("\" data-fragment-count=\"")
	writer.write(strconv.FormatUint(uint64(len(fragments)), 10))
	writer.write("\" data-break-marker-count=\"")
	writer.write(strconv.FormatUint(uint64(len(breaks)), 10))
	writer.write("\">\n")
	writer.write("<title id=\"layout-debug-title\">Layout geometry debug capture</title>\n")
	writer.write("<desc id=\"layout-debug-description\">Recorded plan geometry only; not a display-list preview.</desc>\n")
	writer.write("<style>.page{fill:none;stroke:#667085;stroke-width:512}.fragment-border{fill:#84cc161a;stroke:#65a30d;stroke-width:512}.fragment-content{fill:none;stroke:#65a30d;stroke-width:256;stroke-dasharray:1024 1024}.command{fill:#38bdf51a;stroke:#0284c7;stroke-width:256;stroke-dasharray:512 512}.diagnostic{fill:#f973161a;stroke:#c2410c;stroke-width:512}.diagnostic-error{stroke:#b42318}.diagnostic-info{stroke:#0284c7}.break-outgoing{fill:#dc2626}.break-incoming{fill:#7c3aed}.break-label-box{fill:#fffffff2;stroke:#475467;stroke-width:256}.break-label{fill:#101828;font-family:monospace;font-size:")
	writer.write(fixedSVGDecimal(debugGeometrySVGLabelFontSize))
	writer.write("px;pointer-events:none}</style>\n")

	writer.write("<g id=\"page\"><rect class=\"page\" x=\"0\" y=\"0\" width=\"")
	writer.write(fixedSVGDecimal(pageBounds.Width))
	writer.write("\" height=\"")
	writer.write(fixedSVGDecimal(pageBounds.Height))
	writer.write("\"/></g>\n")

	writer.write("<g id=\"fragments\">\n")
	for _, fragment := range fragments {
		writer.write("<g class=\"fragment\" data-fragment-id=\"")
		writer.write(strconv.FormatUint(uint64(fragment.ID), 10))
		writer.write("\" data-node-id=\"")
		writer.write(strconv.FormatUint(uint64(fragment.Node), 10))
		writer.write("\" data-region=\"")
		writer.attribute(string(fragment.Region))
		writer.write("\" data-continuation=\"")
		writer.attribute(string(fragment.Continuation))
		writer.write("\">")
		writer.rect("fragment-border", fragment.BorderBox)
		writer.rect("fragment-content", fragment.ContentBox)
		writer.write("</g>\n")
	}
	writer.write("</g>\n")

	writer.write("<g id=\"commands\">\n")
	for commandIndex, command := range commands {
		writer.write("<g class=\"command-group\" data-command-index=\"")
		writer.write(strconv.FormatUint(uint64(page.Commands.Start)+uint64(commandIndex), 10))
		writer.write("\" data-command-kind=\"")
		writer.attribute(string(command.Kind))
		writer.write("\" data-fragment-id=\"")
		writer.write(strconv.FormatUint(uint64(command.Fragment), 10))
		writer.write("\" data-payload-index=\"")
		writer.write(strconv.FormatUint(uint64(command.Payload), 10))
		writer.write("\">")
		writer.rect("command", command.Bounds)
		writer.write("</g>\n")
	}
	writer.write("</g>\n")

	writer.write("<g id=\"breaks\">\n")
	for _, marker := range breaks {
		decision := marker.Decision
		writer.write("<g class=\"break-marker break-")
		writer.write(marker.Direction)
		writer.write("\" data-direction=\"")
		writer.write(marker.Direction)
		writer.write("\" data-break-reason=\"")
		writer.attribute(string(decision.Reason))
		writer.write("\" data-break-label=\"")
		writer.attribute(marker.Label)
		writer.write("\" data-from-page=\"")
		writer.write(strconv.FormatUint(uint64(decision.FromPage), 10))
		writer.write("\" data-to-page=\"")
		writer.write(strconv.FormatUint(uint64(decision.ToPage), 10))
		writer.write("\" data-region=\"")
		writer.attribute(string(decision.Region))
		writer.write("\" data-preceding-fragment=\"")
		writer.write(strconv.FormatUint(uint64(decision.Preceding), 10))
		writer.write("\" data-triggering-fragment=\"")
		writer.write(strconv.FormatUint(uint64(decision.Triggering), 10))
		writer.write("\" data-required-fixed=\"")
		writer.write(fixedSVGDecimal(decision.Required))
		writer.write("\" data-available-fixed=\"")
		writer.write(fixedSVGDecimal(decision.Available))
		writer.write("\"><circle cx=\"")
		writer.write(fixedSVGDecimal(marker.X))
		writer.write("\" cy=\"")
		writer.write(fixedSVGDecimal(marker.Y))
		writer.write("\" r=\"")
		writer.write(fixedSVGDecimal(debugGeometrySVGMarkerRadius))
		writer.write("\"/>")
		writer.rect("break-label-box", marker.LabelBounds)
		writer.write("<text class=\"break-label\" x=\"")
		writer.write(fixedSVGDecimal(marker.LabelX))
		writer.write("\" y=\"")
		writer.write(fixedSVGDecimal(marker.LabelY))
		writer.write("\">")
		writer.attribute(marker.Label)
		writer.write("</text></g>\n")
	}
	writer.write("</g>\n")

	writer.write("<g id=\"diagnostics\">\n")
	for _, diagnostic := range diagnostics {
		writer.write("<g class=\"diagnostic diagnostic-")
		writer.attribute(string(diagnostic.Severity))
		writer.write("\" data-diagnostic-code=\"")
		writer.attribute(string(diagnostic.Code))
		writer.write("\" data-severity=\"")
		writer.attribute(string(diagnostic.Severity))
		writer.write("\" data-stage=\"")
		writer.attribute(string(diagnostic.Stage))
		writer.write("\">")
		writer.rect("diagnostic", diagnostic.Location.Bounds)
		writer.write("</g>\n")
	}
	writer.write("</g>\n</svg>\n")
	if writer.err != nil {
		return nil, writer.err
	}
	return []byte(writer.builder.String()), nil
}

func debugGeometrySVGWithinItemLimit(counts ...int) bool {
	total := 0
	for _, count := range counts {
		if count < 0 || count > debugGeometrySVGMaxItems-total {
			return false
		}
		total += count
	}
	return true
}

// fixedSVGDecimal is an exact, locale-independent, base-10 serialization of
// a Fixed unit. SVG coordinates use the same raw fixed-point unit as the plan;
// data-fixed-scale declares the conversion back to PDF points.
func fixedSVGDecimal(value Fixed) string {
	return strconv.FormatInt(int64(value), 10)
}

type debugGeometrySVGWriter struct {
	builder strings.Builder
	limit   int
	err     error
}

func (w *debugGeometrySVGWriter) write(value string) {
	if w.err != nil {
		return
	}
	if len(value) > w.limit-w.builder.Len() {
		w.err = ErrDebugGeometryCaptureLimit
		return
	}
	w.builder.WriteString(value)
}

func (w *debugGeometrySVGWriter) attribute(value string) {
	if w.err != nil {
		return
	}
	escaped := strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"'", "&#39;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(value)
	w.write(escaped)
}

func (w *debugGeometrySVGWriter) rect(class string, rect Rect) {
	w.write("<rect class=\"")
	w.write(class)
	w.write("\" x=\"")
	w.write(fixedSVGDecimal(rect.X))
	w.write("\" y=\"")
	w.write(fixedSVGDecimal(rect.Y))
	w.write("\" width=\"")
	w.write(fixedSVGDecimal(rect.Width))
	w.write("\" height=\"")
	w.write(fixedSVGDecimal(rect.Height))
	w.write("\"/>")
}
