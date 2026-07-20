// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"os"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

func TestPaperReadableBoxModelRetainsFourExactLayers(t *testing.T) {
	source, err := os.ReadFile("../testdata/paper/studio-box-model.paper")
	if err != nil {
		t.Fatal(err)
	}
	plan, result, err := PlanPaper("studio-box-model.paper", string(source))
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 1 {
		t.Fatalf("fragments = %+v", fragments)
	}
	fragment := fragments[0]
	if fragment.MarginBox != (layoutengine.Rect{X: 20 * 1024, Y: 20 * 1024, Width: 200 * 1024, Height: 76 * 1024}) ||
		fragment.BorderBox != (layoutengine.Rect{X: 30 * 1024, Y: 30 * 1024, Width: 176 * 1024, Height: 56 * 1024}) ||
		fragment.PaddingBox != (layoutengine.Rect{X: 33 * 1024, Y: 33 * 1024, Width: 170 * 1024, Height: 48 * 1024}) ||
		fragment.ContentBox != (layoutengine.Rect{X: 45 * 1024, Y: 41 * 1024, Width: 150 * 1024, Height: 32 * 1024}) {
		t.Fatalf("box layers = %+v", fragment)
	}
}
