// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "github.com/cssbruno/paperrune/layout"

// newMeasureContext creates a measurement context for the given content width.
func newMeasureContext(pdf *Document, width float64) layout.MeasureContext {
	fontSize := 12.0
	lineHeight := 5.0
	fontFamily := "Helvetica"
	if pdf != nil {
		fontFamily = pdf.fontFamily
		if fontFamily == "" {
			fontFamily = "Helvetica"
		}
		ptSize, unitSize := pdf.GetFontSize()
		if ptSize > 0 && isFiniteFloat(ptSize) {
			fontSize = ptSize
		}
		if unitSize > 0 && isFiniteFloat(unitSize) {
			lineHeight = unitSize * 1.2
		}
	}
	ctx := layout.NewMeasureContext(width, layout.TextStyle{
		FontFamily: fontFamily,
		FontSize:   fontSize,
		LineHeight: lineHeight,
	})
	if pdf != nil {
		ctx.TextMeasurer = documentTextMeasurer{pdf: pdf}
	}
	return ctx
}

type documentTextMeasurer struct {
	pdf *Document
}

func (m documentTextMeasurer) TextLineCount(text string, style layout.TextStyle, width float64) int {
	if m.pdf == nil {
		return 0
	}
	state := applyPDFTextStyle(m.pdf, style)
	defer restorePDFTextStyle(m.pdf, state)
	return m.pdf.SplitTextCount(text, width)
}

type pdfTextStyleState struct {
	family    string
	style     string
	sizePt    float64
	underline bool
	strikeout bool
}

func applyPDFTextStyle(pdf *Document, style layout.TextStyle) pdfTextStyleState {
	state := pdfTextStyleState{
		family:    pdf.fontFamily,
		style:     pdf.fontStyle,
		sizePt:    pdf.fontSizePt,
		underline: pdf.underline,
		strikeout: pdf.strikeout,
	}
	if style.FontFamily == "" {
		style.FontFamily = state.family
	}
	size := style.FontSize
	if size <= 0 {
		size = state.sizePt
	}
	if size <= 0 {
		size = 12
	}
	style.FontSize = size
	family := firstNonEmpty(style.FontFamily, "Helvetica")
	fontStyle := ""
	if style.Bold {
		fontStyle += "B"
	}
	if style.Italic {
		fontStyle += "I"
	}
	if style.Underline {
		fontStyle += "U"
	}
	if style.StrikeThrough {
		fontStyle += "S"
	}
	setFontForMeasurement(pdf, family, fontStyle, size)
	pdf.strikeout = style.StrikeThrough
	return state
}

func restorePDFTextStyle(pdf *Document, state pdfTextStyleState) {
	family := state.family
	if family == "" {
		family = "Helvetica"
	}
	size := state.sizePt
	if size <= 0 {
		size = 12
	}
	setFontForMeasurement(pdf, family, state.style, size)
	pdf.underline = state.underline
	pdf.strikeout = state.strikeout
}

// setFontForMeasurement selects font metrics without writing a font-selection
// operator into the current page. Measurement may populate the document's font
// resource map, but it must not mutate rendered page content.
func setFontForMeasurement(pdf *Document, family, style string, size float64) {
	page := pdf.page
	pdf.page = 0
	defer func() { pdf.page = page }()
	pdf.SetFont(family, style, size)
}
