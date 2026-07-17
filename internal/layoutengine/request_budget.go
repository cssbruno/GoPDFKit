// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"strconv"
	"sync"
)

const (
	DefaultRequestPlanningWork uint64 = 32_000_000
	HardRequestPlanningWork    uint64 = 256_000_000
)

var (
	ErrPlanningBudgetInvalid   = errors.New("layoutengine: request planning budget is invalid")
	ErrPlanningBudgetExhausted = errors.New("layoutengine: request planning work budget exhausted")
)

type planningBudgetContextKey struct{}

// PlanningBudget is one request-owned, concurrency-safe cumulative work
// meter. Nested planners receive it through context and cannot replenish it.
type PlanningBudget struct {
	mu    sync.Mutex
	limit uint64
	used  uint64
}

// WithPlanningBudget installs a request meter unless ctx already carries one.
// Repeated use therefore propagates the original meter instead of resetting it.
func WithPlanningBudget(ctx context.Context, limit uint64) (context.Context, *PlanningBudget, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if current := PlanningBudgetFromContext(ctx); current != nil {
		return ctx, current, nil
	}
	if limit == 0 || limit > HardRequestPlanningWork {
		return nil, nil, ErrPlanningBudgetInvalid
	}
	budget := &PlanningBudget{limit: limit}
	return context.WithValue(ctx, planningBudgetContextKey{}, budget), budget, nil
}

// EnsurePlanningBudget preserves an existing request meter or installs the
// conservative document-planning default at the outermost entry point.
func EnsurePlanningBudget(ctx context.Context) (context.Context, *PlanningBudget, error) {
	return WithPlanningBudget(ctx, DefaultRequestPlanningWork)
}

func PlanningBudgetFromContext(ctx context.Context) *PlanningBudget {
	if ctx == nil {
		return nil
	}
	budget, _ := ctx.Value(planningBudgetContextKey{}).(*PlanningBudget)
	return budget
}

// Snapshot returns a coherent cumulative used/limit pair.
func (budget *PlanningBudget) Snapshot() (used, limit uint64) {
	if budget == nil {
		return 0, 0
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.used, budget.limit
}

// ChargePlanningWork charges the request meter carried by ctx. Contexts with
// no meter retain compatibility for isolated low-level planner calls.
func ChargePlanningWork(ctx context.Context, planner string, amount uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
			Stage: StageLayout, Message: planner + " was canceled"})
	}
	budget := PlanningBudgetFromContext(ctx)
	if budget == nil || amount == 0 {
		return nil
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	// Cancellation may race with another goroutine holding the meter. Recheck
	// after acquiring the lock so canceled waiters never consume work.
	if err := ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
			Stage: StageLayout, Message: planner + " was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrPlanningBudgetExhausted, Diagnostic{
			Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout,
			Message: planner + " exhausted the cumulative request planning work budget",
			Evidence: []DiagnosticEvidence{
				{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(budget.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)},
			},
		})
	}
	budget.used += amount
	return nil
}
