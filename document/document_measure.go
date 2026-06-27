// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "github.com/cssbruno/gopdfkit/layout"

// MeasureContext contains the renderer state needed to estimate block layout.
type MeasureContext = layout.MeasureContext

// BlockMeasurement is the estimated layout footprint of a block.
type BlockMeasurement = layout.BlockMeasurement

// NewMeasureContext creates a measurement context for the given content width.
func NewMeasureContext(pdf *Document, width float64) MeasureContext {
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
	ctx := layout.NewMeasureContext(width, TextStyle{
		FontFamily: fontFamily,
		FontSize:   fontSize,
		LineHeight: lineHeight,
	})
	if pdf != nil {
		ctx.TextMeasurer = documentTextMeasurer{pdf: pdf}
	}
	return ctx
}

// MeasureBlocks estimates a sequence of blocks.
func MeasureBlocks(ctx MeasureContext, blocks []Block) []BlockMeasurement {
	return layout.MeasureBlocks(ctx, blocks)
}

// MeasureBlock estimates the layout footprint for one block.
func MeasureBlock(ctx MeasureContext, block Block) BlockMeasurement {
	return layout.MeasureBlock(ctx, block)
}

type documentTextMeasurer struct {
	pdf *Document
}

func (m documentTextMeasurer) TextLineCount(text string, style TextStyle, width float64) int {
	if m.pdf == nil {
		return 0
	}
	state := applyPDFTextStyle(m.pdf, style)
	defer restorePDFTextStyle(m.pdf, state)
	return len(m.pdf.SplitText(text, width))
}

type pdfTextStyleState struct {
	family    string
	style     string
	sizePt    float64
	underline bool
	strikeout bool
}

func applyPDFTextStyle(pdf *Document, style TextStyle) pdfTextStyleState {
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
	pdf.SetFont(family, fontStyle, size)
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
	pdf.SetFont(family, state.style, size)
	pdf.underline = state.underline
	pdf.strikeout = state.strikeout
}

func textSegmentsPlainText(segments []TextSegment) string {
	return layout.TextSegmentsPlainText(segments)
}

func mergedTextStyle(base, override TextStyle) TextStyle {
	return layout.MergedTextStyle(base, override)
}

func resolvedLineHeight(style TextStyle) float64 {
	return layout.ResolvedLineHeight(style)
}

func documentHeadingFontSize(base float64, level int) float64 {
	return layout.HeadingFontSize(base, level)
}

func innerWidth(width float64, box BoxStyle) float64 {
	return layout.InnerWidth(width, box)
}

func verticalSpacing(spacing Spacing) float64 {
	return layout.VerticalSpacing(spacing)
}

func borderVertical(border BorderStyle) float64 {
	return layout.BorderVertical(border)
}

func firstPositive(values ...float64) float64 {
	return layout.FirstPositive(values...)
}
