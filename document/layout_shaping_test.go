// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

func TestShapeDocumentCoreASCIIUsesExactEmbeddedMetrics(t *testing.T) {
	font, err := loadCoreFontDef("helvetica")
	if err != nil {
		t.Fatalf("loadCoreFontDef() = %v", err)
	}
	resource, err := typedCoreFontResource(font)
	if err != nil {
		t.Fatalf("typedCoreFontResource() = %v", err)
	}
	input := layoutengine.ShapingInput{
		Text:     "Wi A",
		Font:     layoutengine.ShapeFont{Name: font.Name, Digest: layoutengine.ShapeFontDigest(resource.MetricsDigest), CoreFace: resource.Face},
		FontSize: layoutengine.Fixed(11 * layoutengine.FixedScale), Language: "en", Direction: layoutengine.DirectionLTR,
	}
	result, err := shapeDocumentCoreASCII(context.Background(), font, input, layoutengine.ShapingLimits{}, nil)
	if err != nil {
		t.Fatalf("shapeDocumentCoreASCII() = %v", err)
	}
	var cumulative int64
	var previous layoutengine.Fixed
	for index := range []byte(input.Text) {
		cumulative += int64(font.Cw[input.Text[index]])
		current, err := layoutengine.FixedFromPoints(float64(cumulative) * input.FontSize.Points() / 1000)
		if err != nil {
			t.Fatalf("expected cumulative advance = %v", err)
		}
		want, _ := current.Sub(previous)
		if result.Glyphs[index].Advance != want {
			t.Fatalf("glyph %d advance = %d, want %d", index, result.Glyphs[index].Advance, want)
		}
		previous = current
	}
}

func TestShapeDocumentCoreASCIIRejectsFontIdentityMismatch(t *testing.T) {
	font, _ := loadCoreFontDef("courier")
	input := layoutengine.ShapingInput{
		Text:     "ASCII",
		Font:     layoutengine.ShapeFont{Name: font.Name, Digest: layoutengine.ShapeFontDigest("1111111111111111111111111111111111111111111111111111111111111111"), CoreFace: layoutengine.CoreFontCourier},
		FontSize: layoutengine.Fixed(10 * layoutengine.FixedScale), Language: "en", Direction: layoutengine.DirectionLTR,
	}
	if _, err := shapeDocumentCoreASCII(context.Background(), font, input, layoutengine.ShapingLimits{}, nil); err == nil {
		t.Fatal("font identity mismatch unexpectedly shaped")
	}
}
