// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"strconv"
	"testing"
)

func TestHitTestPageOrdersCommandsAndFragmentsLastPlannedFirst(t *testing.T) {
	plan, err := NewLayoutPlan(overlappingHitTestPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	hit, err := plan.HitTestPage(1, Point{X: 35, Y: 35})
	if err != nil {
		t.Fatalf("HitTestPage() = %v", err)
	}
	if !hit.InsidePage || hit.Page != 1 || hit.Point != (Point{X: 35, Y: 35}) {
		t.Fatalf("hit metadata = %#v", hit)
	}
	if got, want := hit.PageBounds, (Rect{Width: 200, Height: 300}); got != want {
		t.Fatalf("page bounds = %#v, want %#v", got, want)
	}
	if len(hit.Commands) != 2 || hit.Commands[0].Index != 1 || hit.Commands[1].Index != 0 {
		t.Fatalf("command hits = %#v, want indexes [1 0]", hit.Commands)
	}
	if hit.Commands[0].Kind != CommandLink || hit.Commands[0].Fragment != 2 ||
		hit.Commands[0].PageIndex != 1 || !hit.Commands[0].BoundsOnly ||
		!hit.Commands[0].HasFragmentProvenance || hit.Commands[0].Key != "@child" {
		t.Fatalf("top command hit = %#v", hit.Commands[0])
	}
	if len(hit.Fragments) != 2 || hit.Fragments[0].Fragment != 2 || hit.Fragments[1].Fragment != 1 {
		t.Fatalf("fragment hits = %#v, want fragment IDs [2 1]", hit.Fragments)
	}
	if hit.Fragments[0].Area != HitFragmentContent || hit.Fragments[0].Key != "@child" ||
		hit.Fragments[0].Instance != "@child-instance" || hit.Fragments[0].Region != RegionBody ||
		hit.Fragments[0].PageIndex != 1 {
		t.Fatalf("top fragment hit = %#v", hit.Fragments[0])
	}
	if hit.Fragments[0].Source.File != "example.paper" {
		t.Fatalf("fragment source = %#v", hit.Fragments[0].Source)
	}
}

func TestHitTestPageExcludesStructuralCommandsAndBoundsResults(t *testing.T) {
	input := overlappingHitTestPlanInput()
	input.Pages[0].Commands.Count = 3
	input.Commands = append(input.Commands, DisplayCommand{
		Kind:   CommandClip,
		Bounds: Rect{X: 30, Y: 30, Width: 40, Height: 40},
	})
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	hit, err := plan.HitTestPage(1, Point{X: 35, Y: 35})
	if err != nil {
		t.Fatalf("HitTestPage() = %v", err)
	}
	if len(hit.Commands) != 2 || hit.CommandMatchCount != 2 || hit.CommandsTruncated {
		t.Fatalf("command hits = %#v, structural clip should be excluded", hit)
	}
}

func TestHitTestPageTruncatesPathologicalOverlapDeterministically(t *testing.T) {
	const count = int(HitTestResultLimit) + 3
	fragments := make([]Fragment, count)
	commands := make([]DisplayCommand, count)
	for index := range count {
		id := FragmentID(index + 1)
		fragments[index] = Fragment{
			ID:           id,
			Node:         NodeID(index + 1),
			Key:          NodeKey("@node-" + strconv.Itoa(index+1)),
			Instance:     "@instance",
			Page:         1,
			Region:       RegionBody,
			BorderBox:    Rect{Width: 10, Height: 10},
			ContentBox:   Rect{Width: 10, Height: 10},
			Continuation: ContinuationWhole,
		}
		commands[index] = DisplayCommand{Kind: CommandFillPath, Fragment: id, Bounds: Rect{Width: 10, Height: 10}}
	}
	plan, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{
			Number:    1,
			Size:      Size{Width: 10, Height: 10},
			Fragments: IndexRange{Count: uint32(count)},
			Commands:  IndexRange{Count: uint32(count)},
		}},
		Fragments: fragments,
		Commands:  commands,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	hit, err := plan.HitTestPage(1, Point{X: 1, Y: 1})
	if err != nil {
		t.Fatalf("HitTestPage() = %v", err)
	}
	if got := len(hit.Commands); got != int(HitTestResultLimit) || !hit.CommandsTruncated || hit.CommandMatchCount != uint32(count) {
		t.Fatalf("bounded command hits = %#v", hit)
	}
	if got := len(hit.Fragments); got != int(HitTestResultLimit) || !hit.FragmentsTruncated || hit.FragmentMatchCount != uint32(count) {
		t.Fatalf("bounded fragment hits = %#v", hit)
	}
	if hit.Commands[0].Index != uint64(count-1) || hit.Fragments[0].Index != uint64(count-1) {
		t.Fatalf("truncated frontmost hits = %#v/%#v", hit.Commands[0], hit.Fragments[0])
	}
}

func TestHitTestPageUsesHalfOpenEdgesAndDistinguishesBorder(t *testing.T) {
	plan, err := NewLayoutPlan(overlappingHitTestPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	leftTop, err := plan.HitTestPage(1, Point{X: 10, Y: 10})
	if err != nil {
		t.Fatalf("HitTestPage(left/top) = %v", err)
	}
	if len(leftTop.Fragments) != 1 || leftTop.Fragments[0].Fragment != 1 || leftTop.Fragments[0].Area != HitFragmentBorder {
		t.Fatalf("left/top hits = %#v, want fragment 1 border", leftTop.Fragments)
	}

	right, err := plan.HitTestPage(1, Point{X: 110, Y: 30})
	if err != nil {
		t.Fatalf("HitTestPage(right) = %v", err)
	}
	if len(right.Fragments) != 0 {
		t.Fatalf("exclusive right edge hits = %#v, want none", right.Fragments)
	}

	bottom, err := plan.HitTestPage(1, Point{X: 30, Y: 110})
	if err != nil {
		t.Fatalf("HitTestPage(bottom) = %v", err)
	}
	if len(bottom.Fragments) != 0 {
		t.Fatalf("exclusive bottom edge hits = %#v, want none", bottom.Fragments)
	}
}

func TestHitTestPageCanInspectOverflowOutsidePage(t *testing.T) {
	input := overlappingHitTestPlanInput()
	input.Fragments[1].BorderBox = Rect{X: 210, Y: 10, Width: 20, Height: 20}
	input.Fragments[1].ContentBox = input.Fragments[1].BorderBox
	input.Commands[1].Bounds = Rect{X: 240, Y: 10, Width: 20, Height: 20}
	input.Links[0].Bounds = input.Commands[1].Bounds
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	hit, err := plan.HitTestPage(1, Point{X: 245, Y: 15})
	if err != nil {
		t.Fatalf("HitTestPage() = %v", err)
	}
	if hit.InsidePage {
		t.Fatal("overflow query unexpectedly reported inside page")
	}
	if len(hit.Fragments) != 0 || len(hit.Commands) != 1 {
		t.Fatalf("overflow hit = %#v", hit)
	}
	if command := hit.Commands[0]; !command.HasFragmentProvenance || command.Fragment != 2 ||
		command.Node != 2 || command.Key != "@child" || command.Instance != "@child-instance" ||
		command.Source.File != "example.paper" {
		t.Fatalf("overflow command provenance = %#v", command)
	}
}

func TestHitTestPageRejectsInvalidPageSelectors(t *testing.T) {
	plan, err := NewLayoutPlan(overlappingHitTestPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	if _, err := plan.HitTestPage(0, Point{}); !errors.Is(err, ErrHitTestInvalidPage) {
		t.Fatalf("HitTestPage(page 0) error = %v, want ErrHitTestInvalidPage", err)
	}
	if _, err := plan.HitTestPage(2, Point{}); !errors.Is(err, ErrHitTestPageNotFound) {
		t.Fatalf("HitTestPage(page 2) error = %v, want ErrHitTestPageNotFound", err)
	}
}

func overlappingHitTestPlanInput() LayoutPlanInput {
	source := SourceSpan{
		File:  "example.paper",
		Start: SourcePosition{Line: 1, Column: 1},
		End:   SourcePosition{Offset: 1, Line: 1, Column: 2},
	}
	return LayoutPlanInput{
		Pages: []PlannedPage{{
			Number:    1,
			Size:      Size{Width: 200, Height: 300},
			Fragments: IndexRange{Count: 2},
			Commands:  IndexRange{Count: 2},
		}},
		Fragments: []Fragment{
			{
				ID:           1,
				Node:         1,
				Key:          "@parent",
				Instance:     "@parent-instance",
				Page:         1,
				Region:       RegionBody,
				BorderBox:    Rect{X: 10, Y: 10, Width: 100, Height: 100},
				ContentBox:   Rect{X: 20, Y: 20, Width: 80, Height: 80},
				Source:       source,
				Continuation: ContinuationWhole,
			},
			{
				ID:           2,
				Node:         2,
				Key:          "@child",
				Instance:     "@child-instance",
				Page:         1,
				Region:       RegionBody,
				BorderBox:    Rect{X: 30, Y: 30, Width: 40, Height: 40},
				ContentBox:   Rect{X: 32, Y: 32, Width: 36, Height: 36},
				Source:       source,
				Continuation: ContinuationWhole,
			},
		},
		Commands: []DisplayCommand{
			{Kind: CommandFillPath, Fragment: 1, Bounds: Rect{X: 10, Y: 10, Width: 100, Height: 100}, Payload: 3},
			{Kind: CommandLink, Fragment: 2, Bounds: Rect{X: 30, Y: 30, Width: 40, Height: 40}, Payload: 0},
		},
		Links: []PlannedLink{{Fragment: 2, Bounds: Rect{X: 30, Y: 30, Width: 40, Height: 40},
			URI: "https://example.test", Source: source}},
	}
}
