// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestPlanningBudgetIsCumulativeNestedAtomicAndDeterministic(t *testing.T) {
	ctx, budget, err := WithPlanningBudget(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	nested, same, err := WithPlanningBudget(ctx, HardRequestPlanningWork)
	if err != nil || nested != ctx || same != budget {
		t.Fatalf("nested budget reset: same=%t err=%v", same == budget, err)
	}
	if err := ChargePlanningWork(ctx, "parent planning", 3); err != nil {
		t.Fatal(err)
	}
	err = ChargePlanningWork(nested, "child planning", 3)
	var planning *PlanningError
	if !errors.Is(err, ErrPlanningBudgetExhausted) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticWorkLimit {
		t.Fatalf("exhaustion = %v", err)
	}
	want := []DiagnosticEvidence{{Key: "work_limit", Value: "5"}, {Key: "work_used", Value: "3"}, {Key: "work_requested", Value: "3"}}
	if len(planning.Diagnostic.Evidence) != len(want) {
		t.Fatalf("evidence = %+v", planning.Diagnostic.Evidence)
	}
	for index := range want {
		if planning.Diagnostic.Evidence[index] != want[index] {
			t.Fatalf("evidence = %+v, want %+v", planning.Diagnostic.Evidence, want)
		}
	}
	if used, limit := budget.Snapshot(); used != 3 || limit != 5 {
		t.Fatalf("failed charge mutated budget = %d/%d", used, limit)
	}
	if err := ChargePlanningWork(ctx, "retry planning", 2); err != nil {
		t.Fatal(err)
	}
	if err := ChargePlanningWork(ctx, "retry planning", 1); !errors.Is(err, ErrPlanningBudgetExhausted) {
		t.Fatalf("retry replenished budget: %v", err)
	}
}

func TestPlanningBudgetConcurrentChargesNeverOverspend(t *testing.T) {
	ctx, budget, err := WithPlanningBudget(context.Background(), 64)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for worker := 0; worker < 128; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_ = ChargePlanningWork(ctx, "concurrent planning", 1)
		}()
	}
	wait.Wait()
	if used, limit := budget.Snapshot(); used != 64 || limit != 64 {
		t.Fatalf("concurrent budget = %d/%d", used, limit)
	}
}

func TestParagraphAndPageMasterShareRequestBudgetAndCancellation(t *testing.T) {
	ctx, _, err := WithPlanningBudget(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	paragraph := testParagraphFlowInput(2, 2, 1, 1, ParagraphBreakPrefer)
	if _, err := PlanParagraphFlowContext(ctx, paragraph); err != nil {
		t.Fatalf("paragraph = %v", err)
	}
	masterInput := PageMasterFlowInput{Masters: PageMasterSet{Default: testPageMaster("default", 100)}, Body: []VerticalFlowBlock{masterBlock(1, 10)}}
	if plan, err := PlanPageMasterFlowContext(ctx, masterInput); !errors.Is(err, ErrPlanningBudgetExhausted) || len(plan.Projection().Pages) != 0 {
		t.Fatalf("nested page master = pages %d, %v", len(plan.Projection().Pages), err)
	}
	if _, err := PlanPageMasterFlowContext(ctx, masterInput); !errors.Is(err, ErrPlanningBudgetExhausted) {
		t.Fatalf("retry reset request budget: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanParagraphFlowContext(canceled, paragraph); !errors.Is(err, context.Canceled) {
		t.Fatalf("paragraph cancellation = %v", err)
	}
}
