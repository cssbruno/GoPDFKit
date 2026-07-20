// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"reflect"
	"testing"
)

func TestRecordCorePlanReplaysCommandsWithoutLayout(t *testing.T) {
	plan, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	first, err := RecordCorePlan(plan)
	if err != nil {
		t.Fatalf("RecordCorePlan() = %v", err)
	}
	second, err := RecordCorePlan(plan)
	if err != nil {
		t.Fatalf("second RecordCorePlan() = %v", err)
	}
	if !reflect.DeepEqual(first.Events(), second.Events()) {
		t.Fatalf("recordings differ:\n%+v\n%+v", first.Events(), second.Events())
	}
	events := first.Events()
	if len(events) != 3 || events[0].Kind != PaintPageBegin || events[1].Kind != PaintGlyphRun || events[2].Kind != PaintPageEnd {
		t.Fatalf("events = %+v", events)
	}
	if events[1].Run.Codes != "AB" || events[1].Command.Payload != 0 || events[1].Font.Face != CoreFontCourier {
		t.Fatalf("glyph event = %+v", events[1])
	}
	if !first.ContainsUserText || !events[1].ContainsUserText {
		t.Fatal("recording did not declare its user-text disclosure")
	}
	events[1].Run.Advances[0] = 99
	if got := first.Events()[1].Run.Advances[0]; got != 4 {
		t.Fatalf("event mutation reached recording: got %d", got)
	}
}

func TestPaintCorePlanPreflightsBeforeWritingToSink(t *testing.T) {
	input := coreGlyphPlanInput()
	input.GlyphRuns = nil
	input.Fonts = nil
	input.Commands = nil
	input.Pages[0].Commands = IndexRange{}
	geometry, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("geometry NewLayoutPlan() = %v", err)
	}
	sink := &countingPaintSink{}
	if err := PaintCorePlan(geometry, sink); err == nil {
		t.Fatal("geometry-only plan unexpectedly painted")
	}
	if sink.calls != 0 {
		t.Fatalf("sink received %d calls before failed preflight", sink.calls)
	}
}

func TestPaintReadyAllowsAnExplicitEmptyLineWithoutGlyphs(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Lines[0].Bounds.Width = 0
	input.Fragments[0].BorderBox.Width = 0
	input.Fragments[0].ContentBox.Width = 0
	input.Fonts = nil
	input.GlyphRuns = nil
	input.Commands = nil
	input.Pages[0].Commands = IndexRange{}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	recording, err := RecordCorePlan(plan)
	if err != nil {
		t.Fatalf("RecordCorePlan() = %v", err)
	}
	events := recording.Events()
	if len(events) != 2 || events[0].Kind != PaintPageBegin || events[1].Kind != PaintPageEnd {
		t.Fatalf("empty-line paint events = %+v", events)
	}
}

func TestPaintReadyRejectsVisibleGlyphsOnAZeroWidthLine(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Lines[0].Bounds.Width = 0
	input.Commands[0].Bounds.Width = 0
	input.GlyphRuns[0].Advances = []Fixed{0, 0}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if err := plan.ValidatePaintReady(); err == nil {
		t.Fatal("visible zero-width glyph run unexpectedly became paint-ready")
	}
}

func TestPaintCorePlanEnforcesBudgetBeforeSinkCallbacks(t *testing.T) {
	plan, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	sink := &countingPaintSink{}
	limits := DefaultCorePaintLimits()
	limits.MaxGlyphs = 1
	if err := PaintCorePlanWithLimits(plan, sink, limits); !errors.Is(err, ErrCorePaintLimit) {
		t.Fatalf("PaintCorePlanWithLimits() = %v, want ErrCorePaintLimit", err)
	}
	if sink.calls != 0 {
		t.Fatalf("over-budget paint made %d sink callbacks", sink.calls)
	}
}

func TestPaintReadyRejectsNoncanonicalCommandOrder(t *testing.T) {
	input := twoLineCoreGlyphPlanInput()
	input.Commands[0], input.Commands[1] = input.Commands[1], input.Commands[0]
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if err := plan.ValidatePaintReady(); err == nil {
		t.Fatal("reordered glyph commands unexpectedly became paint-ready")
	}
}

type countingPaintSink struct{ calls int }

func (s *countingPaintSink) BeginPlannedPage(PlannedPage) error {
	s.calls++
	return nil
}

func (s *countingPaintSink) PaintCoreGlyphRun(CoreFontResource, CoreGlyphRun, DisplayCommand) error {
	s.calls++
	return nil
}

func (s *countingPaintSink) EndPlannedPage(PlannedPage) error {
	s.calls++
	return nil
}
