// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"os"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

func TestPaperInspectionFixtureRetainsOverflowCollisionAndClipEvidence(t *testing.T) {
	source, err := os.ReadFile("../testdata/paper/studio-issues.paper")
	if err != nil {
		t.Fatal(err)
	}
	plan, result, err := PlanPaper("studio-issues.paper", string(source))
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	var overflow bool
	for _, diagnostic := range projection.Diagnostics {
		overflow = overflow || diagnostic.Code == layoutengine.DiagnosticCanvasNodeOverflow && diagnostic.Location.HasBounds
	}
	if !overflow {
		t.Fatalf("overflow diagnostics = %+v", projection.Diagnostics)
	}
	if len(projection.Fragments) < 4 || len(projection.Images) != 1 || projection.Images[0].Crop == nil || projection.Images[0].Crop.Clip.IsEmpty() {
		t.Fatalf("fragments/images = %+v / %+v", projection.Fragments, projection.Images)
	}
	left, right := projection.Fragments[0].BorderBox, projection.Fragments[1].BorderBox
	if left.X+left.Width <= right.X || left.Y+left.Height <= right.Y {
		t.Fatalf("fixture panels do not collide: %+v / %+v", left, right)
	}
}
