// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layout

import "strings"

const (
	paragraphSpacing = 2.0
	headingTopSpace  = 2.5
	headingBotSpace  = 1.5
)

// TextSegmentsPlainText joins styled text segments without performing layout.
func TextSegmentsPlainText(segments []TextSegment) string {
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(segment.Text)
	}
	return builder.String()
}

// MergedTextStyle overlays override onto base without measuring content.
func MergedTextStyle(base, override TextStyle) TextStyle {
	if override.FontFamily != "" {
		base.FontFamily = override.FontFamily
	}
	if override.FontSize > 0 {
		if override.LineHeight <= 0 {
			base.LineHeight = scaledLineHeight(base, override.FontSize)
		}
		base.FontSize = override.FontSize
	}
	if override.LineHeight > 0 {
		base.LineHeight = override.LineHeight
	}
	base.Bold = base.Bold || override.Bold
	base.Italic = base.Italic || override.Italic
	base.Underline = base.Underline || override.Underline
	base.StrikeThrough = base.StrikeThrough || override.StrikeThrough
	if override.Color.Set {
		base.Color = override.Color
	}
	if override.Align != "" {
		base.Align = override.Align
	}
	if override.WhiteSpace != "" {
		base.WhiteSpace = override.WhiteSpace
	}
	if override.TabSize != 0 {
		base.TabSize = override.TabSize
	}
	return base
}

// ResolvedLineHeight returns the concrete line height for style.
func ResolvedLineHeight(style TextStyle) float64 {
	if style.LineHeight > 0 {
		return style.LineHeight
	}
	if style.FontSize > 0 {
		return style.FontSize * 1.2
	}
	return 5
}

func scaledLineHeight(base TextStyle, fontSize float64) float64 {
	if fontSize <= 0 {
		return ResolvedLineHeight(base)
	}
	if base.LineHeight > 0 && base.FontSize > 0 {
		return base.LineHeight * fontSize / base.FontSize
	}
	return fontSize * 1.2
}

// HeadingFontSize returns the default heading size for level.
func HeadingFontSize(base float64, level int) float64 {
	if base <= 0 {
		base = 12
	}
	switch level {
	case 1:
		return base * 1.8
	case 2:
		return base * 1.5
	case 3:
		return base * 1.25
	default:
		return base * 1.1
	}
}

// ParagraphBox returns paragraph box defaults applied to box.
func ParagraphBox(box BoxStyle) BoxStyle {
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = paragraphSpacing
	}
	return box
}

// HeadingBox returns heading box defaults applied to box.
func HeadingBox(box BoxStyle) BoxStyle {
	if box.Margin.Top == 0 {
		box.Margin.Top = headingTopSpace
	}
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = headingBotSpace
	}
	return box
}

// InnerWidth returns the content width inside padding and borders.
func InnerWidth(width float64, box BoxStyle) float64 {
	inner := width - horizontalSpacing(box.Padding) - borderHorizontal(box.Border)
	if inner < 0 {
		return 0
	}
	return inner
}

func VerticalSpacing(spacing Spacing) float64 { return spacing.Top + spacing.Bottom }

func horizontalSpacing(spacing Spacing) float64 { return spacing.Left + spacing.Right }

func BorderVertical(border BorderStyle) float64 { return border.Top.Width + border.Bottom.Width }

func borderHorizontal(border BorderStyle) float64 { return border.Left.Width + border.Right.Width }

// FirstPositive returns the first positive value.
func FirstPositive(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
