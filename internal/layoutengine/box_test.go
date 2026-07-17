// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"testing"
)

func TestPlanBoxFlowContextPropagatesCancellationAndFlowLimits(t *testing.T) {
	input := BoxFlowInput{
		PageSize: Size{Width: fixedPoints(120), Height: fixedPoints(140)},
		Body:     Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(100), Height: fixedPoints(100)},
		Blocks: []BoxFlowBlock{
			testBoxFlowBlock(1, "@first", fixedPoints(60), BoxFlowStyle{}),
			testBoxFlowBlock(2, "@second", fixedPoints(60), BoxFlowStyle{}),
		},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PlanBoxFlowContext(canceled, input, VerticalFlowLimits{})
	var planning *PlanningError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticCanceled {
		t.Fatalf("canceled box flow = %v", err)
	}
	limits := DefaultVerticalFlowLimits()
	limits.MaxBlocks = 1
	_, err = PlanBoxFlowContext(context.Background(), input, limits)
	if !errors.Is(err, ErrFlowStateLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticResourceLimit {
		t.Fatalf("block-limited box flow = %v", err)
	}
	limits = DefaultVerticalFlowLimits()
	limits.MaxPages = 1
	_, err = PlanBoxFlowContext(context.Background(), input, limits)
	if !errors.Is(err, ErrFlowStateLimit) {
		t.Fatalf("page-limited box flow = %v", err)
	}
}

func TestPlanBoxFlowResolvesExactBoxesAndMarginPagination(t *testing.T) {
	first := testBoxFlowBlock(1, "@first", fixedPoints(20), BoxFlowStyle{
		Margin:  Insets{Top: fixedPoints(2), Right: fixedPoints(3), Bottom: fixedPoints(5), Left: fixedPoints(5)},
		Border:  Insets{Top: fixedPoints(1), Right: fixedPoints(2), Bottom: fixedPoints(2), Left: fixedPoints(1)},
		Padding: Insets{Top: fixedPoints(3), Right: fixedPoints(6), Bottom: fixedPoints(4), Left: fixedPoints(4)},
	})
	second := testBoxFlowBlock(2, "@second", fixedPoints(64), BoxFlowStyle{})
	plan, err := PlanBoxFlow(BoxFlowInput{
		PageSize: Size{Width: fixedPoints(120), Height: fixedPoints(140)},
		Body:     Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(100), Height: fixedPoints(100)},
		Blocks:   []BoxFlowBlock{first, second},
	})
	if err != nil {
		t.Fatalf("PlanBoxFlow() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 2 || len(projection.Fragments) != 2 {
		t.Fatalf("pages/fragments = %d/%d, want 2/2", len(projection.Pages), len(projection.Fragments))
	}
	if got, want := projection.Fragments[0].BorderBox, (Rect{
		X: fixedPoints(15), Y: fixedPoints(22), Width: fixedPoints(92), Height: fixedPoints(30),
	}); got != want {
		t.Fatalf("border box = %+v, want %+v", got, want)
	}
	if got, want := projection.Fragments[0].MarginBox, (Rect{
		X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(100), Height: fixedPoints(37),
	}); got != want {
		t.Fatalf("margin box = %+v, want %+v", got, want)
	}
	if got, want := projection.Fragments[0].PaddingBox, (Rect{
		X: fixedPoints(16), Y: fixedPoints(23), Width: fixedPoints(89), Height: fixedPoints(27),
	}); got != want {
		t.Fatalf("padding box = %+v, want %+v", got, want)
	}
	if got, want := projection.Fragments[0].ContentBox, (Rect{
		X: fixedPoints(20), Y: fixedPoints(26), Width: fixedPoints(79), Height: fixedPoints(20),
	}); got != want {
		t.Fatalf("content box = %+v, want %+v", got, want)
	}
	if projection.Fragments[1].Page != 2 || projection.Fragments[1].BorderBox.Y != fixedPoints(20) {
		t.Fatalf("second fragment = %+v, want fresh page body origin", projection.Fragments[1])
	}
	requireBreaks(t, projection.Breaks, []BreakDecision{{
		Reason: BreakInsufficientRemainingBodySpace, FromPage: 1, ToPage: 2, Region: RegionBody,
		Preceding: 1, Triggering: 2, Required: fixedPoints(64), Available: fixedPoints(63),
	}})
}

func TestPlanBoxFlowKeepsOversizeDiagnosticOnResolvedBorderBox(t *testing.T) {
	block := testBoxFlowBlock(1, "@oversize", fixedPoints(101), BoxFlowStyle{
		Margin:  Insets{Top: fixedPoints(2), Bottom: fixedPoints(3)},
		Padding: Insets{Top: fixedPoints(1), Bottom: fixedPoints(1)},
	})
	plan, err := PlanBoxFlow(BoxFlowInput{
		PageSize: Size{Width: fixedPoints(120), Height: fixedPoints(140)},
		Body:     Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(100), Height: fixedPoints(100)},
		Blocks:   []BoxFlowBlock{block},
	})
	if err != nil {
		t.Fatalf("PlanBoxFlow() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Diagnostics) != 1 || projection.Diagnostics[0].Location.Bounds != projection.Fragments[0].BorderBox {
		t.Fatalf("diagnostic/fragment = %+v / %+v", projection.Diagnostics, projection.Fragments[0])
	}
	if projection.Fragments[0].BorderBox.Y != fixedPoints(22) || projection.Fragments[0].ContentBox.Y != fixedPoints(23) {
		t.Fatalf("resolved oversize boxes = %+v", projection.Fragments[0])
	}
}

func TestPlanBoxFlowRejectsInvalidEdgesAndHorizontalOverflow(t *testing.T) {
	input := BoxFlowInput{
		PageSize: Size{Width: fixedPoints(120), Height: fixedPoints(140)},
		Body:     Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(100), Height: fixedPoints(100)},
		Blocks:   []BoxFlowBlock{testBoxFlowBlock(1, "@box", fixedPoints(10), BoxFlowStyle{})},
	}
	input.Blocks[0].Style.Padding.Left = -1
	if _, err := PlanBoxFlow(input); !errors.Is(err, ErrBoxNegativeEdge) {
		t.Fatalf("negative edge = %v, want ErrBoxNegativeEdge", err)
	}
	input.Blocks[0].Style.Padding.Left = fixedPoints(101)
	if _, err := PlanBoxFlow(input); !errors.Is(err, ErrBoxHorizontalOverflow) {
		t.Fatalf("horizontal overflow = %v, want ErrBoxHorizontalOverflow", err)
	}
	input.Blocks[0].Style.Padding.Left = 0
	input.Blocks[0].ContentHeight = -1
	if _, err := PlanBoxFlow(input); !errors.Is(err, ErrBoxNegativeContent) {
		t.Fatalf("negative content = %v, want ErrBoxNegativeContent", err)
	}
}

func testBoxFlowBlock(node NodeID, key NodeKey, height Fixed, style BoxFlowStyle) BoxFlowBlock {
	return BoxFlowBlock{
		Node: node, Key: key, Instance: InstanceID(key),
		Source: SourceSpan{
			File:  "boxes.paper",
			Start: SourcePosition{Offset: uint64(node * 10), Line: uint32(node), Column: 1},
			End:   SourcePosition{Offset: uint64(node*10 + 5), Line: uint32(node), Column: 6},
		},
		ContentHeight: height, Style: style,
	}
}
