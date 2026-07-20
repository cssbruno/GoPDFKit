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

func TestCaptureDebugGeometrySVGIsDeterministicAndCarriesPlanGeometry(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	first, err := plan.CaptureDebugGeometrySVGPage(1)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVGPage(first) = %v", err)
	}
	second, err := plan.CaptureDebugGeometrySVGPage(1)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVGPage(second) = %v", err)
	}
	if first.FormatVersion != second.FormatVersion || first.Page != second.Page ||
		first.PageBounds != second.PageBounds || first.CanvasBounds != second.CanvasBounds ||
		first.FragmentCount != second.FragmentCount || first.BreakMarkerCount != second.BreakMarkerCount ||
		first.FixedScale != second.FixedScale {
		t.Fatalf("capture metadata differs:\n%#v\n%#v", first, second)
	}
	if !bytes.Equal(first.SVG, second.SVG) {
		t.Fatalf("captures differ:\n%s\n%s", first.SVG, second.SVG)
	}
	if first.FormatVersion != DebugGeometrySVGFormatVersion || first.Page != 1 ||
		first.FragmentCount != 1 || first.BreakMarkerCount != 1 || first.FixedScale != FixedScale {
		t.Fatalf("capture metadata = %#v, want format/page/fixed scale", first)
	}
	if first.PageBounds != (Rect{Width: 626688, Height: 811008}) {
		t.Fatalf("page bounds = %#v", first.PageBounds)
	}
	if first.CanvasBounds != (Rect{X: -10230, Y: -13212, Width: 645110, Height: 832412}) {
		t.Fatalf("canvas bounds = %#v", first.CanvasBounds)
	}

	svg := string(first.SVG)
	for _, want := range []string{
		`viewBox="-10230 -13212 645110 832412"`,
		`data-format-version="2"`,
		`data-coordinate-space="pdf-fixed"`,
		`data-fixed-scale="1024"`,
		`data-page="1" data-fragment-count="1" data-break-marker-count="1"`,
		`Layout geometry debug capture`,
		`<g id="page"><rect class="page" x="0" y="0" width="626688" height="811008"/></g>`,
		`class="fragment-border" x="10" y="20" width="300" height="80"`,
		`class="fragment-content" x="14" y="24" width="292" height="72"`,
		`data-fragment-id="1" data-node-id="7" data-region="body" data-continuation="start"`,
		`class="command" x="14" y="24" width="100" height="12"`,
		`data-diagnostic-code="TRACK_MIN_OVERFLOW" data-severity="warning" data-stage="layout"`,
	} {
		if !strings.Contains(svg, want) {
			t.Fatalf("capture does not contain %q:\n%s", want, svg)
		}
	}
	for _, mustNotContain := range []string{
		"@lines",
		"minimum tracks exceed the page width",
		"<script",
		"foreignObject",
		"href=",
	} {
		if strings.Contains(svg, mustNotContain) {
			t.Fatalf("capture leaked forbidden content %q:\n%s", mustNotContain, svg)
		}
	}
	if err := xml.Unmarshal(first.SVG, new(struct{})); err != nil {
		t.Fatalf("capture XML is not well-formed: %v", err)
	}

	wrapper, err := CaptureDebugGeometrySVG(plan, 1)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVG() = %v", err)
	}
	if !bytes.Equal(wrapper, first.SVG) {
		t.Fatal("function wrapper did not return capture SVG")
	}
}

func TestCaptureDebugGeometrySVGCanvasIncludesOverflowAndOffPageGeometry(t *testing.T) {
	input := testPlanInput()
	input.Breaks = nil
	input.Fragments[0].BorderBox = Rect{X: -100, Y: -200, Width: 20, Height: 30}
	input.Fragments[0].ContentBox = input.Fragments[0].BorderBox
	input.Commands[1].Bounds = Rect{X: 700000, Y: 900000, Width: 10, Height: 20}
	input.Diagnostics[0].Location.Bounds = Rect{X: 800000, Y: 1000000, Width: 5, Height: 6}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	capture, err := plan.CaptureDebugGeometrySVGPage(1)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVGPage() = %v", err)
	}
	if got, want := capture.CanvasBounds, (Rect{X: -8292, Y: -8392, Width: 816489, Height: 1016590}); got != want {
		t.Fatalf("canvas bounds = %#v, want %#v", got, want)
	}
	if !strings.Contains(string(capture.SVG), `viewBox="-8292 -8392 816489 1016590"`) {
		t.Fatalf("overflow geometry is outside SVG canvas:\n%s", capture.SVG)
	}
}

func TestCaptureDebugGeometrySVGScopesCommittedBreaksAndDiagnosticsToSelectedPage(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	pageOne, err := CaptureDebugGeometrySVG(plan, 1)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVG(page 1) = %v", err)
	}
	pageOneOutput := string(pageOne)
	if !strings.Contains(pageOneOutput, `data-direction="outgoing" data-break-reason="insufficient_remaining_body_space"`) {
		t.Fatalf("outgoing break is missing:\n%s", pageOneOutput)
	}
	if strings.Contains(pageOneOutput, `data-direction="incoming"`) {
		t.Fatalf("incoming break appeared on its source page:\n%s", pageOneOutput)
	}
	if !strings.Contains(pageOneOutput, `<circle cx="10" cy="100" r="2048"/>`) {
		t.Fatalf("outgoing marker is not anchored to preceding geometry:\n%s", pageOneOutput)
	}
	if !strings.Contains(pageOneOutput, `data-break-label="out:space"`) ||
		!strings.Contains(pageOneOutput, `class="break-label-box" x="4106" y="-5020" width="50176" height="10240"`) ||
		!strings.Contains(pageOneOutput, `class="break-label" x="6154" y="3172">out:space</text>`) {
		t.Fatalf("outgoing break label is missing or not fixed-coordinate geometry:\n%s", pageOneOutput)
	}

	pageTwo, err := CaptureDebugGeometrySVG(plan, 2)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVG(page 2) = %v", err)
	}
	pageTwoOutput := string(pageTwo)
	if !strings.Contains(pageTwoOutput, `data-direction="incoming" data-break-reason="insufficient_remaining_body_space"`) {
		t.Fatalf("incoming break is missing:\n%s", pageTwoOutput)
	}
	if strings.Contains(pageTwoOutput, `data-direction="outgoing"`) {
		t.Fatalf("outgoing break appeared on its destination page:\n%s", pageTwoOutput)
	}
	if !strings.Contains(pageTwoOutput, `<circle cx="10" cy="20" r="2048"/>`) {
		t.Fatalf("incoming marker is not anchored to triggering geometry:\n%s", pageTwoOutput)
	}
	if !strings.Contains(pageTwoOutput, `data-break-label="in:space"`) ||
		!strings.Contains(pageTwoOutput, `class="break-label" x="6154" y="3092">in:space</text>`) {
		t.Fatalf("incoming break label is missing:\n%s", pageTwoOutput)
	}
	if strings.Contains(pageTwoOutput, `data-diagnostic-code="TRACK_MIN_OVERFLOW"`) {
		t.Fatalf("page 1 diagnostic appeared on page 2:\n%s", pageTwoOutput)
	}
	if strings.Contains(pageTwoOutput, `data-fragment-id="1"`) {
		t.Fatalf("page 1 fragment appeared on page 2:\n%s", pageTwoOutput)
	}
}

func TestDebugGeometrySVGBreakLabelsAreConciseClosedMetadata(t *testing.T) {
	tests := []struct {
		reason BreakReason
		want   string
	}{
		{BreakInsufficientRemainingBodySpace, "out:space"},
		{BreakPreviousFragmentOverflow, "out:overflow"},
		{BreakPaginationConstraint, "out:constraint"},
		{BreakExplicitPageBreak, "out:explicit"},
	}
	for _, test := range tests {
		if got := debugGeometrySVGBreakLabel("outgoing", test.reason); got != test.want || len(got) > 16 {
			t.Fatalf("label for %q = %q, want concise %q", test.reason, got, test.want)
		}
		incoming := debugGeometrySVGBreakLabel("incoming", test.reason)
		if !strings.HasPrefix(incoming, "in:") || len(incoming) > 16 {
			t.Fatalf("incoming label for %q = %q", test.reason, incoming)
		}
	}
}

func TestCaptureDebugGeometrySVGDoesNotLeakUserControlledText(t *testing.T) {
	const key = `@<&>"'`
	input := LayoutPlanInput{
		Pages: []PlannedPage{{
			Number:    1,
			Size:      Size{Width: 17, Height: 19},
			Fragments: IndexRange{Count: 1},
		}},
		Fragments: []Fragment{{
			ID:           1,
			Node:         1,
			Key:          NodeKey(key),
			Instance:     InstanceID(key),
			Page:         1,
			Region:       RegionBody,
			BorderBox:    Rect{X: -11, Y: 3, Width: 7, Height: 5},
			ContentBox:   Rect{X: -10, Y: 4, Width: 5, Height: 3},
			Continuation: ContinuationWhole,
		}},
	}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	svg, err := CaptureDebugGeometrySVG(plan, 1)
	if err != nil {
		t.Fatalf("CaptureDebugGeometrySVG() = %v", err)
	}
	output := string(svg)
	for _, forbidden := range []string{key, "@", "&lt;", "&amp;", "&gt;"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("capture leaked user-controlled text %q:\n%s", forbidden, output)
		}
	}
	for _, want := range []string{
		`class="fragment-border" x="-11" y="3" width="7" height="5"`,
		`class="fragment-content" x="-10" y="4" width="5" height="3"`,
		`data-fragment-id="1" data-node-id="1" data-region="body"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("capture does not retain exact raw Fixed coordinates %q:\n%s", want, output)
		}
	}
	if err := xml.Unmarshal(svg, new(struct{})); err != nil {
		t.Fatalf("capture XML is not well-formed: %v", err)
	}
}

func TestCaptureDebugGeometrySVGRejectsInvalidPageSelectors(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if _, err := CaptureDebugGeometrySVG(plan, 0); !errors.Is(err, ErrDebugGeometryInvalidPage) {
		t.Fatalf("CaptureDebugGeometrySVG(page 0) error = %v, want ErrDebugGeometryInvalidPage", err)
	}
	if _, err := CaptureDebugGeometrySVG(plan, 3); !errors.Is(err, ErrDebugGeometryPageNotFound) {
		t.Fatalf("CaptureDebugGeometrySVG(page 3) error = %v, want ErrDebugGeometryPageNotFound", err)
	}
}

func TestCaptureDebugGeometrySVGEnforcesGeometryRecordLimit(t *testing.T) {
	fragments := make([]Fragment, debugGeometrySVGMaxItems+1)
	for index := range fragments {
		id := uint32(index + 1)
		fragments[index] = Fragment{
			ID:           FragmentID(id),
			Node:         NodeID(id),
			Key:          NodeKey(fmt.Sprintf("@node-%d", id)),
			Instance:     "@instance",
			Page:         1,
			Region:       RegionBody,
			Continuation: ContinuationWhole,
		}
	}
	plan, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{
			Number:    1,
			Size:      Size{Width: 1, Height: 1},
			Fragments: IndexRange{Count: uint32(len(fragments))},
		}},
		Fragments: fragments,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if _, err := CaptureDebugGeometrySVG(plan, 1); !errors.Is(err, ErrDebugGeometryCaptureLimit) {
		t.Fatalf("CaptureDebugGeometrySVG() error = %v, want ErrDebugGeometryCaptureLimit", err)
	}
}
