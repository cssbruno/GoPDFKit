// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layout

import "github.com/cssbruno/paperrune/internal/layoutgeom"

// ImageFitResult describes an image fitted inside a target box. Offset values
// locate the fitted image relative to the target box; callers decide whether a
// cover result should be clipped.
type ImageFitResult struct {
	OffsetX float64
	OffsetY float64
	Width   float64
	Height  float64
}

// FitImage calculates contain or cover geometry without depending on a PDF
// renderer. Invalid dimensions return the zero result.
func FitImage(naturalWidth, naturalHeight, boxWidth, boxHeight float64, mode ImageFitMode) ImageFitResult {
	if naturalWidth <= 0 || naturalHeight <= 0 || boxWidth <= 0 || boxHeight <= 0 {
		return ImageFitResult{}
	}
	scaleX := boxWidth / naturalWidth
	scaleY := boxHeight / naturalHeight
	scale := scaleX
	if mode == ImageFitContain {
		if scaleY < scale {
			scale = scaleY
		}
	} else if scaleY > scale {
		scale = scaleY
	}
	width := naturalWidth * scale
	height := naturalHeight * scale
	return ImageFitResult{
		OffsetX: (boxWidth - width) / 2,
		OffsetY: (boxHeight - height) / 2,
		Width:   width,
		Height:  height,
	}
}

// ExceedsAvailableHeight reports whether content needs more vertical space
// than remains on the current page. Equality fits, matching PDF pagination
// semantics across typed-layout and HTML renderers.
func ExceedsAvailableHeight(contentHeight, availableHeight float64) bool {
	return layoutgeom.ExceedsAvailableHeight(contentHeight, availableHeight)
}

// TrackOffsets returns cumulative offsets for row or column sizes. The result
// always has len(sizes)+1 entries and starts at zero.
func TrackOffsets(sizes []float64) []float64 {
	return layoutgeom.TrackOffsets(sizes)
}

// SpanSize returns the extent of span tracks starting at start. Invalid starts
// and non-positive spans return zero; spans extending past the end are clipped.
func SpanSize(offsets []float64, start, span int) float64 {
	return layoutgeom.SpanSize(offsets, start, span)
}

// SumSpan returns the sum of span values starting at start. It is useful when
// offsets are not already available, such as row-span height calculation.
func SumSpan(values []float64, start, span int) float64 {
	return layoutgeom.SumSpan(values, start, span)
}
