// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

func TestPaperPlanPageSVGCapturesBindExactDisplayAndGeometryToPlan(t *testing.T) {
	plan, result, err := PlanPaper("studio.paper", paperPipelineFixture)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper = %+v, %v", result, err)
	}
	display, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := plan.CaptureGeometryPageSVG(1)
	if err != nil {
		t.Fatal(err)
	}
	if display.PlanHash != plan.Hash() || geometry.PlanHash != plan.Hash() || display.Kind != "display" || geometry.Kind != "geometry" ||
		display.Page != 1 || geometry.Page != 1 || display.PageWidth <= 0 || display.PageHeight <= 0 || display.FixedScale != layoutengine.FixedScale ||
		geometry.CanvasWidth <= 0 || geometry.CanvasHeight <= 0 || len(display.SVG) == 0 || len(geometry.SVG) == 0 {
		t.Fatalf("display=%+v geometry=%+v", display, geometry)
	}
	if bytes.Equal(display.SVG, geometry.SVG) || !bytes.Contains(display.SVG, []byte(`data-format="display-plan-preview"`)) ||
		!bytes.Contains(geometry.SVG, []byte(`aria-labelledby="layout-debug-title layout-debug-description"`)) {
		t.Fatalf("display/geometry formats were not distinct exact artifacts")
	}
	display.SVG[0] = '!'
	again, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || len(again.SVG) == 0 || again.SVG[0] == '!' {
		t.Fatalf("capture aliasing = first %q again %q, %v", display.SVG[:1], again.SVG[:1], err)
	}
}

func TestPaperPlanPageSVGCaptureRejectsInvalidRequestsAtomically(t *testing.T) {
	plan, result, err := PlanPaper("studio.paper", paperPipelineFixture)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper = %+v, %v", result, err)
	}
	if capture, err := plan.CaptureDisplayPageSVG(nil, 1, nil); err == nil || len(capture.SVG) != 0 {
		t.Fatalf("nil context capture = %d, %v", len(capture.SVG), err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if capture, err := plan.CaptureDisplayPageSVG(canceled, 1, nil); !errors.Is(err, context.Canceled) || len(capture.SVG) != 0 {
		t.Fatalf("canceled capture = %d, %v", len(capture.SVG), err)
	}
	if capture, err := plan.CaptureGeometryPageSVG(0); err == nil || len(capture.SVG) != 0 {
		t.Fatalf("page zero geometry = %d, %v", len(capture.SVG), err)
	}
}
