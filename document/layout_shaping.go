// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

// documentCoreASCIIAdvanceProvider adapts the exact embedded core-font width
// tables to the layout engine's characterized ASCII shaping path. Cumulative
// rounding matches the existing typed glyph planner and avoids per-glyph
// rounding drift.
type documentCoreASCIIAdvanceProvider struct{ font fontDefinition }

func (provider documentCoreASCIIAdvanceProvider) CoreASCIIAdvances(face layoutengine.CoreFontFace, text string, fontSize layoutengine.Fixed) ([]layoutengine.Fixed, error) {
	resource, err := typedCoreFontResource(provider.font)
	if err != nil || resource.Face != face || len(provider.font.Cw) != 256 || fontSize <= 0 {
		return nil, errors.New("document: core ASCII shaping font metrics mismatch")
	}
	advances := make([]layoutengine.Fixed, len(text))
	var cumulativeWidth int64
	var previous layoutengine.Fixed
	for index := range []byte(text) {
		width := provider.font.Cw[text[index]]
		if width < 0 {
			return nil, fmt.Errorf("document: core ASCII glyph %d has a negative width", index)
		}
		cumulativeWidth += int64(width)
		current, err := layoutengine.FixedFromPoints(float64(cumulativeWidth) * fontSize.Points() / 1000)
		if err != nil {
			return nil, fmt.Errorf("document: core ASCII glyph %d advance: %w", index, err)
		}
		advance, err := current.Sub(previous)
		if err != nil || advance < 0 {
			return nil, fmt.Errorf("document: core ASCII glyph %d advance is invalid", index)
		}
		advances[index], previous = advance, current
	}
	return advances, nil
}

// shapeDocumentCoreASCII is the bounded adapter used by document planning
// experiments. It refuses a font identity mismatch before consulting cache or
// shaping and returns immutable glyph positions suitable for exact replay.
func shapeDocumentCoreASCII(ctx context.Context, font fontDefinition, input layoutengine.ShapingInput, limits layoutengine.ShapingLimits, cache *layoutengine.ShapeCache) (layoutengine.ShapedText, error) {
	resource, err := typedCoreFontResource(font)
	if err != nil {
		return layoutengine.ShapedText{}, err
	}
	if input.Font.CoreFace != resource.Face || input.Font.Digest != layoutengine.ShapeFontDigest(resource.MetricsDigest) {
		return layoutengine.ShapedText{}, errors.New("document: shaping input font identity does not match resolved core font")
	}
	shaper := layoutengine.CoreASCIIShaper{Metrics: documentCoreASCIIAdvanceProvider{font: font}}
	return layoutengine.ShapeText(ctx, shaper, input, limits, cache)
}
