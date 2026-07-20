// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"testing"
)

func TestPlanStackPlacesFixedChildrenInCanonicalPaintOrder(t *testing.T) {
	container, _ := NewRect(10, 20, 100, 80)
	front := stackTestChild(3, "@front", Size{Width: 20, Height: 10})
	front.Horizontal, front.Vertical = StackAlignStretch, StackAlignEnd
	front.Offset, front.ZIndex, front.PaintOrder = Point{X: 1}, 5, 2
	back := stackTestChild(1, "@back", Size{Width: 30, Height: 20})
	back.Horizontal, back.Vertical = StackAlignStart, StackAlignCenter
	back.Offset, back.ZIndex, back.PaintOrder = Point{X: 2, Y: -3}, -2, 9
	middle := stackTestChild(2, "@middle", Size{Width: 40, Height: 40})
	middle.Horizontal, middle.Vertical = StackAlignCenter, StackAlignStart
	middle.ZIndex, middle.PaintOrder = 5, 1

	input := StackPlanInput{PageSize: Size{Width: 200, Height: 200}, Container: container,
		Children: []StackChild{front, back, middle}}
	plan, err := PlanStack(context.Background(), input, StackPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 || len(projection.Fragments) != 3 || len(projection.Diagnostics) != 1 {
		t.Fatalf("projection counts = pages %d fragments %d diagnostics %d", len(projection.Pages), len(projection.Fragments), len(projection.Diagnostics))
	}
	if got := []NodeKey{projection.Fragments[0].Key, projection.Fragments[1].Key, projection.Fragments[2].Key}; got[0] != "@back" || got[1] != "@middle" || got[2] != "@front" {
		t.Fatalf("paint order = %v", got)
	}
	if got := projection.Fragments[0].BorderBox; got != (Rect{X: 12, Y: 47, Width: 30, Height: 20}) {
		t.Fatalf("back bounds = %+v", got)
	}
	if got := projection.Fragments[1].BorderBox; got != (Rect{X: 40, Y: 20, Width: 40, Height: 40}) {
		t.Fatalf("middle bounds = %+v", got)
	}
	if got := projection.Fragments[2].BorderBox; got != (Rect{X: 11, Y: 90, Width: 100, Height: 10}) {
		t.Fatalf("front bounds = %+v", got)
	}
	// The stretched child is intentionally shifted outside the container.
	if projection.Diagnostics[0].Code != DiagnosticStackChildOverflow || projection.Diagnostics[0].Location.Fragment != 3 {
		t.Fatalf("overflow diagnostic = %+v", projection.Diagnostics[0])
	}

	input.Children = []StackChild{middle, front, back}
	reordered, err := PlanStack(context.Background(), input, StackPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	firstHash, _ := plan.Hash()
	secondHash, _ := reordered.Hash()
	if firstHash != secondHash {
		t.Fatalf("unique explicit paint keys changed hash: %s != %s", firstHash, secondHash)
	}
}

func TestPlanStackAllowsIntentionalOverlapAndRetainsStableProvenance(t *testing.T) {
	container, _ := NewRect(0, 0, 100, 100)
	first := stackTestChild(1, "@first", Size{Width: 80, Height: 80})
	second := stackTestChild(2, "@second", Size{Width: 80, Height: 80})
	second.Horizontal, second.Vertical = StackAlignEnd, StackAlignEnd
	plan, err := PlanStack(context.Background(), StackPlanInput{PageSize: Size{Width: 100, Height: 100}, Container: container,
		Children: []StackChild{first, second}}, StackPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Diagnostics) != 0 {
		t.Fatalf("intentional overlap diagnostics = %+v", projection.Diagnostics)
	}
	for index, fragment := range projection.Fragments {
		want := []NodeKey{"@first", "@second"}[index]
		if fragment.Key != want || fragment.Source.File != "stack.paper" || fragment.Continuation != ContinuationWhole {
			t.Fatalf("fragment %d provenance = %+v", index, fragment)
		}
	}
}

func TestPlanStackCancellationWorkStateAndHardLimits(t *testing.T) {
	container, _ := NewRect(0, 0, 100, 100)
	input := StackPlanInput{PageSize: Size{Width: 100, Height: 100}, Container: container,
		Children: []StackChild{stackTestChild(1, "@one", Size{Width: 10, Height: 10}), stackTestChild(2, "@two", Size{Width: 10, Height: 10})}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanStack(ctx, input, StackPlanLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}

	limits := DefaultStackPlanLimits()
	limits.MaxWork = 1
	if _, err := PlanStack(context.Background(), input, limits); !errors.Is(err, ErrStackWorkLimit) {
		t.Fatalf("work limit error = %v", err)
	}
	limits = DefaultStackPlanLimits()
	limits.MaxStateBytes = stackChildStateBase
	if _, err := PlanStack(context.Background(), input, limits); !errors.Is(err, ErrStackStateLimit) {
		t.Fatalf("state limit error = %v", err)
	}
	limits = DefaultStackPlanLimits()
	limits.MaxChildren = 1
	if _, err := PlanStack(context.Background(), input, limits); !errors.Is(err, ErrStackStateLimit) {
		t.Fatalf("child limit error = %v", err)
	}
	if _, err := PlanStack(context.Background(), input, StackPlanLimits{MaxChildren: 1}); !errors.Is(err, ErrStackLimitsInvalid) {
		t.Fatalf("partial limit error = %v", err)
	}
	limits = DefaultStackPlanLimits()
	limits.MaxChildren = hardMaxStackChildren + 1
	if _, err := PlanStack(context.Background(), input, limits); !errors.Is(err, ErrStackLimitsInvalid) {
		t.Fatalf("hard-cap error = %v", err)
	}
}

func TestPlanStackRejectsInvalidSurfaceAndChild(t *testing.T) {
	container, _ := NewRect(0, 0, 101, 100)
	if _, err := PlanStack(context.Background(), StackPlanInput{PageSize: Size{Width: 100, Height: 100}, Container: container}, StackPlanLimits{}); !errors.Is(err, ErrStackSurfaceInvalid) {
		t.Fatalf("surface error = %v", err)
	}
	container, _ = NewRect(0, 0, 100, 100)
	child := stackTestChild(1, "@bad", Size{Width: 10, Height: 10})
	child.Horizontal = "automatic"
	if _, err := PlanStack(context.Background(), StackPlanInput{PageSize: Size{Width: 100, Height: 100}, Container: container,
		Children: []StackChild{child}}, StackPlanLimits{}); !errors.Is(err, ErrStackChildInvalid) {
		t.Fatalf("child error = %v", err)
	}
}

func stackTestChild(node NodeID, key string, size Size) StackChild {
	return StackChild{Node: node, Key: NodeKey(key), Instance: InstanceID(key),
		Source: SourceSpan{File: "stack.paper", Start: SourcePosition{Line: uint32(node), Column: 1},
			End: SourcePosition{Line: uint32(node), Column: 2}},
		Size: size, Horizontal: StackAlignStart, Vertical: StackAlignStart}
}
