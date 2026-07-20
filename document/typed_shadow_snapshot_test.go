// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

type typedShadowSnapshot struct {
	page         int
	state        documentState
	x            float64
	y            float64
	left         float64
	top          float64
	right        float64
	bottom       float64
	fontFamily   string
	fontStyle    string
	fontSizePt   float64
	fontSizeUnit float64
	fontCount    int
	errorText    string
}

func typedShadowSnapshotOf(pdf *Document) typedShadowSnapshot {
	left, top, right, bottom := pdf.GetMargins()
	errorText := ""
	if pdf.err != nil {
		errorText = pdf.err.Error()
	}
	return typedShadowSnapshot{
		page:         pdf.page,
		state:        pdf.state,
		x:            pdf.x,
		y:            pdf.y,
		left:         left,
		top:          top,
		right:        right,
		bottom:       bottom,
		fontFamily:   pdf.fontFamily,
		fontStyle:    pdf.fontStyle,
		fontSizePt:   pdf.fontSizePt,
		fontSizeUnit: pdf.fontSize,
		fontCount:    len(pdf.ensureResourceStore().fonts),
		errorText:    errorText,
	}
}
