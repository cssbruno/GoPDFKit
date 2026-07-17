// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"errors"
	"testing"
)

func TestCaptureDisplayPlanSVGReplaysExactGraphicsStatePathsAndStyles(t *testing.T) {
	plan := graphicsDisplayPlan(t)
	first, err := CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil || !bytes.Equal(first.SVG, second.SVG) {
		t.Fatalf("graphics SVG determinism = %v, equal=%t", err, bytes.Equal(first.SVG, second.SVG))
	}
	if got, want := first.FormatVersion, uint16(3); got != want {
		t.Fatalf("graphics SVG format = %d, want %d", got, want)
	}
	checks := [][]byte{
		[]byte(`transform="matrix(1 0 0 1 5120 7168)"`),
		[]byte(`<clipPath id="display-clip-2" clipPathUnits="userSpaceOnUse">`),
		[]byte(`clip-rule="evenodd"`),
		[]byte(`fill="#112233" fill-rule="nonzero"`),
		[]byte(`stroke="#c86432" stroke-width="2048"`),
		[]byte(`M 10240 10240 L 40960 10240 L 40960 30720 L 10240 30720 Z`),
		[]byte(`C 61440 0 71680 40960 81920 30720`),
	}
	for _, check := range checks {
		if !bytes.Contains(first.SVG, check) {
			t.Fatalf("graphics SVG lacks %q:\n%s", check, first.SVG)
		}
	}
	if bytes.Count(first.SVG, []byte("<g ")) != bytes.Count(first.SVG, []byte("</g>")) {
		t.Fatalf("graphics SVG groups are unbalanced:\n%s", first.SVG)
	}
}

func TestCaptureDisplayPlanSVGRejectsGraphicsBeforeOutputWhenWorkBoundIsExceeded(t *testing.T) {
	limits := DefaultDisplaySVGLimits()
	limits.MaxPathSegments = 4
	capture, err := CaptureDisplayPlanSVGWithLimits(graphicsDisplayPlan(t), 1, nil, limits)
	if !errors.Is(err, ErrDisplaySVGLimit) || len(capture.SVG) != 0 {
		t.Fatalf("bounded graphics SVG = %d bytes, %v", len(capture.SVG), err)
	}
}

func TestFixedSVGScalarDecimalIsExactForBinaryFixedValues(t *testing.T) {
	for _, test := range []struct {
		value Fixed
		want  string
	}{
		{Fixed(FixedScale), "1"}, {Fixed(FixedScale / 2), "0.5"}, {Fixed(-FixedScale / 8), "-0.125"},
		{Fixed(FixedScale + 1), "1.0009765625"},
	} {
		if got := fixedSVGScalarDecimal(test.value); got != test.want {
			t.Fatalf("fixedSVGScalarDecimal(%d) = %q, want %q", test.value, got, test.want)
		}
	}
}
