// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCaptureCorePlanSVGReplaysExactGlyphPositions(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Lines[0].Bounds.Width = 8
	input.GlyphRuns[0].Codes = "<&"
	input.GlyphRuns[0].Advances = []Fixed{3, 5}
	input.GlyphRuns[0].Color = CoreRGBColor{R: 17, G: 34, B: 51, Set: true}
	input.Commands[0].Bounds = input.Lines[0].Bounds
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	first, err := CaptureCorePlanSVG(plan, 1)
	if err != nil {
		t.Fatalf("CaptureCorePlanSVG() = %v", err)
	}
	second, _ := CaptureCorePlanSVG(plan, 1)
	if !bytes.Equal(first.SVG, second.SVG) {
		t.Fatal("core plan previews are not deterministic")
	}
	if first.FormatVersion != CorePlanSVGFormatVersion || first.Page != 1 || !first.ContainsUserText {
		t.Fatalf("capture metadata = %+v", first)
	}
	output := string(first.SVG)
	for _, want := range []string{
		`data-format="core-plan-preview"`,
		fmt.Sprintf(`data-format-version="%d"`, first.FormatVersion),
		`data-disclosure="contains-user-text"`,
		`data-font-face="courier"`,
		`class="line-bounds" x="10" y="20" width="8" height="12"`,
		`class="glyph" x="10" y="29" font-size="10240"`,
		`fill="#112233"`,
		`data-glyph-index="1" data-advance="5"`,
		`x="13" y="29"`,
		`&lt;`,
		`&amp;`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("preview does not contain %q:\n%s", want, output)
		}
	}
	if err := xml.Unmarshal(first.SVG, new(struct{})); err != nil {
		t.Fatalf("preview XML is not well formed: %v", err)
	}
}

func TestCaptureCorePlanSVGEnforcesGlyphLimit(t *testing.T) {
	input := coreGlyphPlanInput()
	input.GlyphRuns[0].Codes = strings.Repeat("A", debugGeometrySVGMaxItems+1)
	input.GlyphRuns[0].Advances = make([]Fixed, debugGeometrySVGMaxItems+1)
	for index := range input.GlyphRuns[0].Advances {
		input.GlyphRuns[0].Advances[index] = 1
	}
	input.Lines[0].Bounds.Width = Fixed(debugGeometrySVGMaxItems + 1)
	input.Commands[0].Bounds = input.Lines[0].Bounds
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if _, err := CaptureCorePlanSVG(plan, 1); !errors.Is(err, ErrDebugGeometryCaptureLimit) {
		t.Fatalf("CaptureCorePlanSVG() = %v, want ErrDebugGeometryCaptureLimit", err)
	}
}

func TestCaptureCorePlanSVGRejectsGeometryOnlyPlan(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Fonts = nil
	input.GlyphRuns = nil
	input.Commands = nil
	input.Pages[0].Commands = IndexRange{}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if _, err := CaptureCorePlanSVG(plan, 1); err == nil {
		t.Fatal("geometry-only plan unexpectedly produced a core preview")
	}
}
