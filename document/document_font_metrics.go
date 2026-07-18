// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "github.com/cssbruno/gopdfkit/layout"

// plannerDefaultTextStyle snapshots the receiver's current text defaults for
// immutable lowering. It does not estimate a block or choose page geometry.
func plannerDefaultTextStyle(pdf *Document) layout.TextStyle {
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
	return layout.TextStyle{FontFamily: fontFamily, FontSize: fontSize, LineHeight: lineHeight}
}

type pdfTextStyleState struct {
	family    string
	style     string
	sizePt    float64
	underline bool
	strikeout bool
}

// applyPlannerTextStyle selects the exact font metrics needed while lowering
// a text run. It never writes a font-selection operator into page content.
func applyPlannerTextStyle(pdf *Document, style layout.TextStyle) pdfTextStyleState {
	state := pdfTextStyleState{
		family: pdf.fontFamily, style: pdf.fontStyle, sizePt: pdf.fontSizePt,
		underline: pdf.underline, strikeout: pdf.strikeout,
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
	setFontForPlannerMetrics(pdf, family, fontStyle, size)
	pdf.strikeout = style.StrikeThrough
	return state
}

func restorePlannerTextStyle(pdf *Document, state pdfTextStyleState) {
	family := state.family
	if family == "" {
		family = "Helvetica"
	}
	size := state.sizePt
	if size <= 0 {
		size = 12
	}
	setFontForPlannerMetrics(pdf, family, state.style, size)
	pdf.underline = state.underline
	pdf.strikeout = state.strikeout
}

func setFontForPlannerMetrics(pdf *Document, family, style string, size float64) {
	page := pdf.page
	pdf.page = 0
	defer func() { pdf.page = page }()
	pdf.SetFont(family, style, size)
}
