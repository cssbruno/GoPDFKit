// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

const CorePlanSVGFormatVersion uint16 = 1

// CorePlanSVGCapture is a deterministic command preview. Unlike the geometry
// debug capture, it intentionally contains user-authored core-font codes and
// must remain inside the caller's disclosure domain. SVG font substitution is
// diagnostic only; the positions and advances are the authoritative plan.
type CorePlanSVGCapture struct {
	FormatVersion    uint16
	Page             uint32
	PageBounds       Rect
	FixedScale       int64
	ContainsUserText bool
	SVG              []byte
}

// CaptureCorePlanSVG replays one paint-ready page into a bounded SVG without
// measuring, wrapping, paginating, or consulting a live document.
func CaptureCorePlanSVG(plan LayoutPlan, pageNumber uint32) (CorePlanSVGCapture, error) {
	if pageNumber == 0 {
		return CorePlanSVGCapture{}, ErrDebugGeometryInvalidPage
	}
	if err := plan.ValidatePaintReady(); err != nil {
		return CorePlanSVGCapture{}, fmt.Errorf("layoutengine: capture core plan preview: %w", err)
	}
	if uint64(pageNumber) > uint64(len(plan.pages)) {
		return CorePlanSVGCapture{}, fmt.Errorf("%w: %d", ErrDebugGeometryPageNotFound, pageNumber)
	}
	page := plan.pages[pageNumber-1]
	commandEnd, ok := page.Commands.end(len(plan.commands))
	if !ok {
		return CorePlanSVGCapture{}, errors.New("layoutengine: core plan preview found an invalid command range")
	}
	commands := plan.commands[page.Commands.Start:commandEnd]
	glyphCount := 0
	for _, command := range commands {
		glyphCount += len(plan.glyphRuns[command.Payload].Codes)
		if glyphCount > debugGeometrySVGMaxItems {
			return CorePlanSVGCapture{}, fmt.Errorf("%w: page %d has more than %d preview glyphs", ErrDebugGeometryCaptureLimit, page.Number, debugGeometrySVGMaxItems)
		}
	}

	writer := debugGeometrySVGWriter{limit: debugGeometrySVGMaxBytes}
	writer.write("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	writer.write("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 ")
	writer.write(fixedSVGDecimal(page.Size.Width))
	writer.write(" ")
	writer.write(fixedSVGDecimal(page.Size.Height))
	writer.write("\" data-format=\"core-plan-preview\" data-format-version=\"")
	writer.write(fmt.Sprint(CorePlanSVGFormatVersion))
	writer.write("\" data-coordinate-space=\"pdf-fixed\" data-fixed-scale=\"")
	writer.write(fmt.Sprint(FixedScale))
	writer.write("\" data-page=\"")
	writer.write(fmt.Sprint(page.Number))
	writer.write("\" data-disclosure=\"contains-user-text\">")
	writer.write("<title>Core plan command preview</title>")
	writer.write("<rect class=\"page\" x=\"0\" y=\"0\" width=\"")
	writer.write(fixedSVGDecimal(page.Size.Width))
	writer.write("\" height=\"")
	writer.write(fixedSVGDecimal(page.Size.Height))
	writer.write("\" fill=\"white\"/>")
	for commandIndex := int(page.Commands.Start); commandIndex < commandEnd; commandIndex++ {
		command := plan.commands[commandIndex]
		run := plan.glyphRuns[command.Payload]
		font := plan.fonts[run.Font-1]
		writer.write("<g class=\"glyph-run\" data-command-index=\"")
		writer.write(fmt.Sprint(commandIndex))
		writer.write("\" data-run-index=\"")
		writer.write(fmt.Sprint(command.Payload))
		writer.write("\" data-font-face=\"")
		writer.attribute(displayFontFamily(font))
		writer.write("\">")
		writer.rect("line-bounds", command.Bounds)
		cursor := run.Origin.X
		glyphIndex := 0
		for _, code := range run.Codes {
			writer.write("<text class=\"glyph\" x=\"")
			writer.write(fixedSVGDecimal(cursor))
			writer.write("\" y=\"")
			writer.write(fixedSVGDecimal(run.Origin.Y))
			writer.write("\" font-size=\"")
			writer.write(fixedSVGDecimal(run.FontSize))
			writer.write("\" font-family=\"")
			writer.attribute(displayFontFamily(font))
			writeCoreRGBSVGAttribute(&writer, run.Color)
			writer.write("\" data-glyph-index=\"")
			writer.write(fmt.Sprint(glyphIndex))
			writer.write("\" data-advance=\"")
			writer.write(fixedSVGDecimal(run.Advances[glyphIndex]))
			writer.write("\">")
			writer.attribute(string(code))
			writer.write("</text>")
			var err error
			cursor, err = cursor.Add(run.Advances[glyphIndex])
			if err != nil {
				return CorePlanSVGCapture{}, fmt.Errorf("layoutengine: core preview glyph cursor: %w", err)
			}
			glyphIndex++
		}
		writer.write("</g>")
	}
	writer.write("</svg>")
	if writer.err != nil {
		return CorePlanSVGCapture{}, writer.err
	}
	return CorePlanSVGCapture{
		FormatVersion: CorePlanSVGFormatVersion, Page: page.Number,
		PageBounds: Rect{Width: page.Size.Width, Height: page.Size.Height},
		FixedScale: FixedScale, ContainsUserText: true, SVG: []byte(writer.builder.String()),
	}, nil
}
