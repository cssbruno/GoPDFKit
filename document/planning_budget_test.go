// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestTypedPageShellAndCorrectionShareOneRequestBudgetAtomically(t *testing.T) {
	ctx, err := WithPlanningWorkLimit(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	doc := &layout.LayoutDocument{PageTemplate: layout.PageTemplate{
		Header: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("shell")}},
	}, Body: []layout.Block{pageShellParagraph("body")}}
	planner := paginationTestDocument(t, 100)
	plan, err := planner.PlanLayoutDocumentContext(ctx, doc)
	if !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("bounded shell plan = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	if _, err := planner.PlanLayoutDocumentContext(ctx, doc); !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) {
		t.Fatalf("same-context retry replenished work: %v", err)
	}
	fresh, err := WithPlanningWorkLimit(context.Background(), DefaultPlanningWorkLimit)
	if err != nil {
		t.Fatal(err)
	}
	first, err := planner.PlanLayoutDocumentContext(fresh, doc)
	if err != nil || first.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("fresh retry = hash %q pages %d, %v", first.Hash(), planner.PageCount(), err)
	}
}

func TestPaperContextUsesExplicitCumulativeBudget(t *testing.T) {
	ctx, err := WithPlanningWorkLimit(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	plan, result, err := PlanPaperContext(ctx, "bounded.paper", paperPipelineFixture)
	if !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) || plan.Hash() != "" || result.Pages != 0 {
		t.Fatalf("bounded paper = hash %q result %+v, %v", plan.Hash(), result, err)
	}
	deepCtx, err := WithPlanningWorkLimit(context.Background(), uint64(len(paperPipelineFixture)+2))
	if err != nil {
		t.Fatal(err)
	}
	plan, result, err = PlanPaperContext(deepCtx, "bounded.paper", paperPipelineFixture)
	if !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) || !errors.Is(err, ErrPaperRender) ||
		plan.Hash() != "" || result.Pages != 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("nested bounded paper = hash %q result %+v, %v", plan.Hash(), result, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := PlanPaperContext(canceled, "cancel.paper", paperPipelineFixture); !errors.Is(err, context.Canceled) {
		t.Fatalf("paper cancellation = %v", err)
	}
}

func TestAdversarialNestedTypedSubtreeExhaustsBeforePlanning(t *testing.T) {
	var block layout.Block = pageShellParagraph("leaf")
	for depth := 0; depth < 64; depth++ {
		block = layout.SectionBlock{Blocks: []layout.Block{block}}
	}
	ctx, err := WithPlanningWorkLimit(context.Background(), 12)
	if err != nil {
		t.Fatal(err)
	}
	planner := paginationTestDocument(t, 100)
	plan, err := planner.PlanLayoutDocumentContext(ctx, &layout.LayoutDocument{Body: []layout.Block{block}})
	if !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("nested subtree = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
}
