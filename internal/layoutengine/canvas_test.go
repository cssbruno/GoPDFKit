// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPlanCanvasResolvesAnchorDAGInCanonicalPaintOrder(t *testing.T) {
	input := canvasFixture()
	first, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{})
	if err != nil {
		t.Fatalf("PlanCanvas() = %v", err)
	}
	second, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{})
	if err != nil {
		t.Fatalf("second PlanCanvas() = %v", err)
	}
	firstHash, _ := first.Hash()
	secondHash, _ := second.Hash()
	if firstHash != secondHash {
		t.Fatalf("plan hashes differ: %s != %s", firstHash, secondHash)
	}
	fragments := first.Projection().Fragments
	if len(fragments) != 2 || fragments[0].Node != 2 || fragments[1].Node != 1 {
		t.Fatalf("paint order = %+v", fragments)
	}
	if fragments[0].BorderBox != (Rect{X: 38, Y: 32, Width: 10, Height: 4}) {
		t.Fatalf("dependent bounds = %+v", fragments[0].BorderBox)
	}
	if fragments[1].BorderBox != (Rect{X: 15, Y: 27, Width: 20, Height: 10}) {
		t.Fatalf("target bounds = %+v", fragments[1].BorderBox)
	}
	if fragments[0].Key != "@b" || fragments[0].Instance != "@b" {
		t.Fatalf("dependent provenance = %+v", fragments[0])
	}
}

func TestPlanCanvasUsesExplicitUnderdeterminedDefaults(t *testing.T) {
	input := canvasFixture()
	input.Nodes = input.Nodes[1:]
	input.Nodes[0].Constraints = nil
	input.Defaults = CanvasDefaults{Horizontal: CanvasAnchorCenterX, Vertical: CanvasAnchorBottom}
	plan, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{})
	if err != nil {
		t.Fatalf("PlanCanvas() = %v", err)
	}
	if got := plan.Projection().Fragments[0].BorderBox; got != (Rect{X: 50, Y: 90, Width: 20, Height: 10}) {
		t.Fatalf("defaulted bounds = %+v", got)
	}
}

func TestPlanCanvasReportsCycleAndOverdetermination(t *testing.T) {
	cycle := canvasFixture()
	cycle.Nodes[1].Constraints[0] = CanvasConstraint{Anchor: CanvasAnchorLeft, TargetNode: 2, TargetAnchor: CanvasAnchorLeft}
	_, err := PlanCanvas(context.Background(), cycle, CanvasPlanLimits{})
	var planning *PlanningError
	if !errors.Is(err, ErrCanvasCycle) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticConstraintCycle {
		t.Fatalf("cycle error = %#v", err)
	}

	overdetermined := canvasFixture()
	overdetermined.Nodes = overdetermined.Nodes[1:]
	overdetermined.Nodes[0].Constraints = []CanvasConstraint{
		{Anchor: CanvasAnchorLeft, TargetAnchor: CanvasAnchorLeft},
		{Anchor: CanvasAnchorRight, TargetAnchor: CanvasAnchorRight},
	}
	_, err = PlanCanvas(context.Background(), overdetermined, CanvasPlanLimits{})
	if !errors.Is(err, ErrCanvasOverdetermined) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticConstraintOverdetermined {
		t.Fatalf("overdetermination error = %#v", err)
	}
	if len(planning.Diagnostic.Evidence) != 3 {
		t.Fatalf("overdetermination evidence = %+v", planning.Diagnostic.Evidence)
	}
}

func TestPlanCanvasAcceptsConsistentRedundantConstraints(t *testing.T) {
	input := canvasFixture()
	input.Nodes = input.Nodes[1:]
	input.Nodes[0].Constraints = []CanvasConstraint{
		{Anchor: CanvasAnchorLeft, TargetAnchor: CanvasAnchorLeft, Offset: 5},
		{Anchor: CanvasAnchorRight, TargetAnchor: CanvasAnchorLeft, Offset: 25},
		{Anchor: CanvasAnchorTop, TargetAnchor: CanvasAnchorTop, Offset: 7},
	}
	plan, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{})
	if err != nil {
		t.Fatalf("PlanCanvas() = %v", err)
	}
	if got := plan.Projection().Fragments[0].BorderBox; got.X != 15 || got.Y != 27 {
		t.Fatalf("redundantly constrained bounds = %+v", got)
	}
}

func TestPlanCanvasRetainsOverflowEvidence(t *testing.T) {
	input := canvasFixture()
	input.Nodes = input.Nodes[1:]
	input.Nodes[0].Constraints = []CanvasConstraint{
		{Anchor: CanvasAnchorLeft, TargetAnchor: CanvasAnchorRight, Offset: -5},
		{Anchor: CanvasAnchorTop, TargetAnchor: CanvasAnchorTop},
	}
	plan, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{})
	if err != nil {
		t.Fatalf("PlanCanvas() = %v", err)
	}
	diagnostics := plan.Projection().Diagnostics
	if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticCanvasNodeOverflow || !diagnostics[0].Location.HasBounds {
		t.Fatalf("overflow diagnostics = %+v", diagnostics)
	}
	want := []DiagnosticEvidence{{Key: "bottom", Value: "0"}, {Key: "left", Value: "0"}, {Key: "right", Value: "15"}, {Key: "top", Value: "0"}}
	if !reflect.DeepEqual(diagnostics[0].Evidence, want) {
		t.Fatalf("overflow evidence = %+v, want %+v", diagnostics[0].Evidence, want)
	}
}

func TestPlanCanvasRejectsUnsatisfiableBaselineAndCrossAxis(t *testing.T) {
	input := canvasFixture()
	input.Nodes[0].Baseline = 0
	if _, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{}); !errors.Is(err, ErrCanvasUnsatisfiable) {
		t.Fatalf("missing baseline = %v", err)
	}
	input = canvasFixture()
	input.Nodes[0].Constraints[0].TargetAnchor = CanvasAnchorTop
	if _, err := PlanCanvas(context.Background(), input, CanvasPlanLimits{}); !errors.Is(err, ErrCanvasConstraintInvalid) {
		t.Fatalf("cross-axis constraint = %v", err)
	}
}

func TestPlanCanvasEnforcesCancellationWorkStateAndHardLimits(t *testing.T) {
	input := canvasFixture()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PlanCanvas(canceled, input, CanvasPlanLimits{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled PlanCanvas() = %v", err)
	}

	limits := DefaultCanvasPlanLimits()
	limits.MaxNodes = 1
	_, err = PlanCanvas(context.Background(), input, limits)
	if !errors.Is(err, ErrCanvasResourceLimit) {
		t.Fatalf("node limit = %v", err)
	}
	limits = DefaultCanvasPlanLimits()
	limits.MaxEdges = 1
	_, err = PlanCanvas(context.Background(), input, limits)
	if !errors.Is(err, ErrCanvasResourceLimit) {
		t.Fatalf("edge limit = %v", err)
	}
	limits = DefaultCanvasPlanLimits()
	limits.MaxBytes = 1
	_, err = PlanCanvas(context.Background(), input, limits)
	if !errors.Is(err, ErrCanvasResourceLimit) {
		t.Fatalf("byte limit = %v", err)
	}
	short := canvasFixture()
	short.Nodes = short.Nodes[1:]
	short.Nodes[0].Constraints = nil
	limits = DefaultCanvasPlanLimits()
	limits.MaxBytes = canvasRetainedNodeBytes - 1
	_, err = PlanCanvas(context.Background(), short, limits)
	if !errors.Is(err, ErrCanvasResourceLimit) {
		t.Fatalf("short-identity retained-state limit = %v", err)
	}
	limits = DefaultCanvasPlanLimits()
	limits.MaxWork = 2
	_, err = PlanCanvas(context.Background(), input, limits)
	if !errors.Is(err, ErrCanvasWorkLimit) {
		t.Fatalf("work limit = %v", err)
	}
	limits = DefaultCanvasPlanLimits()
	limits.MaxNodes = hardMaxCanvasNodes + 1
	if _, err := PlanCanvas(context.Background(), input, limits); !errors.Is(err, ErrCanvasLimitsInvalid) {
		t.Fatalf("hard limit = %v", err)
	}
}

func canvasFixture() CanvasPlanInput {
	return CanvasPlanInput{
		PageSize:  Size{Width: 200, Height: 200},
		Container: Rect{X: 10, Y: 20, Width: 100, Height: 80},
		Defaults:  CanvasDefaults{Horizontal: CanvasAnchorLeft, Vertical: CanvasAnchorTop},
		Nodes: []CanvasNode{
			{
				Node: 2, Key: "@b", Instance: "@b", Size: Size{Width: 10, Height: 4}, Baseline: 3,
				Constraints: []CanvasConstraint{
					{Anchor: CanvasAnchorLeft, TargetNode: 1, TargetAnchor: CanvasAnchorRight, Offset: 3},
					{Anchor: CanvasAnchorBaseline, TargetNode: 1, TargetAnchor: CanvasAnchorBaseline},
				},
			},
			{
				Node: 1, Key: "@a", Instance: "@a", Size: Size{Width: 20, Height: 10}, Baseline: 8,
				Constraints: []CanvasConstraint{
					{Anchor: CanvasAnchorLeft, TargetAnchor: CanvasAnchorLeft, Offset: 5},
					{Anchor: CanvasAnchorTop, TargetAnchor: CanvasAnchorTop, Offset: 7},
				},
			},
		},
	}
}
