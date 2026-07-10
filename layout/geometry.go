// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

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
	return contentHeight > availableHeight
}
