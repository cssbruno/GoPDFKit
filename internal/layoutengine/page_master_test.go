// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestPageMasterSelectionPrecedence(t *testing.T) {
	set := PageMasterSet{
		Default: testPageMaster("default", 100),
		First:   pageMasterPointer(testPageMaster("first", 101)),
		Even:    pageMasterPointer(testPageMaster("even", 102)),
	}
	wants := []PageMasterID{"first", "even", "default", "even"}
	for index, want := range wants {
		got, err := set.Select(uint32(index + 1))
		if err != nil {
			t.Fatalf("Select(%d): %v", index+1, err)
		}
		if got.ID != want {
			t.Errorf("Select(%d).ID = %q, want %q", index+1, got.ID, want)
		}
	}
	odd := testPageMaster("odd", 103)
	set.Odd = &odd
	selected, err := set.Select(3)
	if err != nil || selected.ID != "odd" {
		t.Fatalf("odd selection = (%q, %v), want odd", selected.ID, err)
	}
	if _, err := set.Select(0); !errors.Is(err, ErrPageMasterInvalid) {
		t.Fatalf("Select(0) error = %v, want ErrPageMasterInvalid", err)
	}
}

func TestPlanPageMasterFlowPaginatesIndependentRegionsCanonically(t *testing.T) {
	input := PageMasterFlowInput{
		Masters: PageMasterSet{Default: testPageMaster("default", 100)},
		Header:  []VerticalFlowBlock{masterBlock(1, 15), masterBlock(2, 10)},
		Body:    []VerticalFlowBlock{masterBlock(3, 60), masterBlock(4, 30)},
		Footer:  []VerticalFlowBlock{masterBlock(5, 25), masterBlock(6, 5)},
	}
	plan, err := PlanPageMasterFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(projection.Pages))
	}
	if len(projection.Fragments) != 6 {
		t.Fatalf("fragments = %d, want 6", len(projection.Fragments))
	}
	wantRegions := []RegionID{RegionHeader, RegionBody, RegionFooter, RegionHeader, RegionBody, RegionFooter}
	wantNodes := []NodeID{1, 3, 5, 2, 4, 6}
	for index, fragment := range projection.Fragments {
		if fragment.Region != wantRegions[index] || fragment.Node != wantNodes[index] {
			t.Errorf("fragment %d = region %q node %d, want region %q node %d", index, fragment.Region, fragment.Node, wantRegions[index], wantNodes[index])
		}
		if fragment.Source != masterBlock(uint32(fragment.Node), 1).Source {
			t.Errorf("fragment %d lost source provenance", index)
		}
	}
	if got := projection.Pages[0].Fragments; got != (IndexRange{Start: 0, Count: 3}) {
		t.Errorf("page 1 fragment range = %+v", got)
	}
	if got := projection.Pages[1].Fragments; got != (IndexRange{Start: 3, Count: 3}) {
		t.Errorf("page 2 fragment range = %+v", got)
	}
	if len(projection.Breaks) != 3 {
		t.Fatalf("breaks = %d, want 3", len(projection.Breaks))
	}
	for index, region := range []RegionID{RegionHeader, RegionBody, RegionFooter} {
		decision := projection.Breaks[index]
		if decision.Region != region || decision.FromPage != 1 || decision.ToPage != 2 {
			t.Errorf("break %d = %+v", index, decision)
		}
	}
	if projection.Breaks[2].Reason != BreakPreviousFragmentOverflow || projection.Breaks[2].Available != 0 {
		t.Errorf("footer break = %+v, want overflow evidence", projection.Breaks[2])
	}
	if len(projection.Diagnostics) != 1 || projection.Diagnostics[0].Code != DiagnosticUnbreakableTooTall || projection.Diagnostics[0].Location.Region != RegionFooter {
		t.Fatalf("diagnostics = %+v, want one footer overflow warning", projection.Diagnostics)
	}
	if len(projection.Commands) != 0 || len(projection.Lines) != 0 {
		t.Fatal("page-master kernel emitted painter or shaping output")
	}
}

func TestPlanPageMasterFlowUsesSelectedGeometryPerPage(t *testing.T) {
	first := testPageMaster("first", 80)
	first.Body = Rect{X: 7, Y: 20, Width: 66, Height: 80}
	even := testPageMaster("even", 90)
	even.Body = Rect{X: 9, Y: 20, Width: 72, Height: 80}
	plan, err := PlanPageMasterFlow(PageMasterFlowInput{
		Masters: PageMasterSet{Default: testPageMaster("default", 100), First: &first, Even: &even},
		Body:    []VerticalFlowBlock{masterBlock(1, 70), masterBlock(2, 70), masterBlock(3, 70)},
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if got := []Fixed{projection.Pages[0].Size.Width, projection.Pages[1].Size.Width, projection.Pages[2].Size.Width}; !reflect.DeepEqual(got, []Fixed{80, 90, 100}) {
		t.Fatalf("page widths = %v", got)
	}
	if got := []Fixed{projection.Fragments[0].BorderBox.X, projection.Fragments[1].BorderBox.X, projection.Fragments[2].BorderBox.X}; !reflect.DeepEqual(got, []Fixed{7, 9, 0}) {
		t.Fatalf("body x positions = %v", got)
	}
}

func TestPageMasterValidationReturnsStructuredDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		master PageMaster
		cause  error
		code   DiagnosticCode
	}{
		{
			name: "empty body", master: PageMaster{ID: "empty", PageSize: Size{Width: 100, Height: 120}},
			cause: ErrPageMasterRegionEmpty, code: DiagnosticPageMasterRegionEmpty,
		},
		{
			name: "outside", master: PageMaster{ID: "outside", PageSize: Size{Width: 100, Height: 120}, Body: Rect{X: 0, Y: 20, Width: 101, Height: 80}},
			cause: ErrPageMasterRegionOutside, code: DiagnosticPageMasterRegionInvalid,
		},
		{
			name: "overlap", master: PageMaster{ID: "overlap", PageSize: Size{Width: 100, Height: 120}, Header: Rect{X: 0, Y: 0, Width: 100, Height: 30}, Body: Rect{X: 0, Y: 20, Width: 100, Height: 80}},
			cause: ErrPageMasterRegionOverlap, code: DiagnosticPageMasterRegionOverlap,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := PlanPageMasterFlow(PageMasterFlowInput{Masters: PageMasterSet{Default: test.master}})
			if !errors.Is(err, test.cause) {
				t.Fatalf("error = %v, want %v", err, test.cause)
			}
			var planning *PlanningError
			if !errors.As(err, &planning) || planning.Diagnostic.Code != test.code {
				t.Fatalf("planning diagnostic = %+v, want %s", planning, test.code)
			}
		})
	}
}

func TestPlanPageMasterFlowRejectsContentInDisabledRegion(t *testing.T) {
	master := testPageMaster("body-only", 100)
	master.Header = Rect{}
	_, err := PlanPageMasterFlow(PageMasterFlowInput{
		Masters: PageMasterSet{Default: master},
		Header:  []VerticalFlowBlock{masterBlock(1, 1)},
	})
	if !errors.Is(err, ErrPageMasterRegionEmpty) {
		t.Fatalf("error = %v, want ErrPageMasterRegionEmpty", err)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) || planning.Diagnostic.Location.Region != RegionHeader || planning.Diagnostic.Location.Page != 1 {
		t.Fatalf("diagnostic = %+v", planning)
	}
}

func TestPlanPageMasterFlowAllowsUnusedDisabledOptionalRegions(t *testing.T) {
	master := testPageMaster("body-only", 100)
	master.Header = Rect{}
	master.Footer = Rect{}
	plan, err := PlanPageMasterFlow(PageMasterFlowInput{
		Masters: PageMasterSet{Default: master},
		Body:    []VerticalFlowBlock{masterBlock(1, 1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Fragments) != 1 || projection.Fragments[0].Region != RegionBody {
		t.Fatalf("fragments = %+v", projection.Fragments)
	}
}

func TestPlanPageMasterFlowEnforcesWorkLimits(t *testing.T) {
	_, err := PlanPageMasterFlow(PageMasterFlowInput{
		Masters: PageMasterSet{Default: testPageMaster("default", 100)},
		Body:    []VerticalFlowBlock{masterBlock(1, 60), masterBlock(2, 60)},
		Limits:  PageMasterLimits{MaxBlocks: 2, MaxPages: 1},
	})
	if !errors.Is(err, ErrPageMasterWorkLimit) {
		t.Fatalf("page limit error = %v", err)
	}
	_, err = PlanPageMasterFlow(PageMasterFlowInput{
		Masters: PageMasterSet{Default: testPageMaster("default", 100)},
		Body:    []VerticalFlowBlock{masterBlock(1, 1), masterBlock(2, 1)},
		Limits:  PageMasterLimits{MaxBlocks: 1, MaxPages: 2},
	})
	if !errors.Is(err, ErrPageMasterWorkLimit) {
		t.Fatalf("block limit error = %v", err)
	}
}

func TestPlanPageMasterFlowIsDeterministicAndDetached(t *testing.T) {
	blocks := []VerticalFlowBlock{masterBlock(1, 30), masterBlock(2, 80)}
	input := PageMasterFlowInput{Masters: PageMasterSet{Default: testPageMaster("default", 100)}, Body: blocks}
	first, err := PlanPageMasterFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	want, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	blocks[0].Height = 1
	got, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("mutating input changed an existing plan")
	}
	second, err := PlanPageMasterFlow(PageMasterFlowInput{Masters: PageMasterSet{Default: testPageMaster("default", 100)}, Body: []VerticalFlowBlock{masterBlock(1, 30), masterBlock(2, 80)}})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := second.CanonicalJSON()
	if !reflect.DeepEqual(again, want) {
		t.Fatal("same semantic input did not produce canonical output")
	}
}

func testPageMaster(id PageMasterID, width Fixed) PageMaster {
	return PageMaster{
		ID: id, PageSize: Size{Width: width, Height: 120},
		Header: Rect{X: 0, Y: 0, Width: width, Height: 20},
		Body:   Rect{X: 0, Y: 20, Width: width, Height: 80},
		Footer: Rect{X: 0, Y: 100, Width: width, Height: 20},
	}
}

func pageMasterPointer(master PageMaster) *PageMaster { return &master }

func masterBlock(id uint32, height Fixed) VerticalFlowBlock {
	return VerticalFlowBlock{
		Node: NodeID(id), Key: NodeKey("master-node-" + strconvForTest(id)),
		Instance: InstanceID("root/master-node-" + strconvForTest(id)), Height: height,
		Source: SourceSpan{File: "master.paper", Start: SourcePosition{Offset: uint64(id), Line: id, Column: 1}, End: SourcePosition{Offset: uint64(id + 1), Line: id, Column: 2}},
	}
}

func strconvForTest(value uint32) string {
	if value < 10 {
		return string(rune('0' + value))
	}
	return fmt.Sprintf("%d", value)
}
