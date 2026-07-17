// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

const (
	textWatermarkAlpha    = 0.16
	textWatermarkAngle    = 35.0
	textWatermarkFont     = "Helvetica"
	textWatermarkFontSize = 54.0
	textWatermarkFontType = "B"
)

// AddTextWatermark draws a centered, translucent text watermark on the current
// page.
func (f *Document) AddTextWatermark(label string) {
	if f.err != nil || label == "" {
		return
	}

	oldTextR, oldTextG, oldTextB := f.GetTextColor()
	oldAlpha, oldBlendMode := f.GetAlpha()
	oldFontFamily := f.fontFamily
	oldFontStyle := f.fontStyle
	oldFontSize := f.fontSizePt
	hadFont := oldFontFamily != ""

	f.SetAlpha(textWatermarkAlpha, "Normal")
	f.SetTextColor(170, 35, 35)
	f.SetFont(textWatermarkFont, textWatermarkFontType, textWatermarkFontSize)
	if f.err != nil {
		return
	}

	width, height := f.GetPageSize()
	labelWidth := f.GetStringWidth(label)
	f.TransformBegin()
	f.TransformRotate(textWatermarkAngle, width/2, height/2)
	f.Text(width/2-labelWidth/2, height/2, label)
	f.TransformEnd()

	f.SetAlpha(oldAlpha, oldBlendMode)
	f.SetTextColor(oldTextR, oldTextG, oldTextB)
	if hadFont {
		f.SetFont(oldFontFamily, oldFontStyle, oldFontSize)
	}
}
