// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

const DefaultPlanningWorkLimit = layoutengine.DefaultRequestPlanningWork

// WithPlanningWorkLimit starts an explicit cumulative planning request. Every
// supported nested typed/.paper planner shares this meter through ctx.
func WithPlanningWorkLimit(ctx context.Context, maxWork uint64) (context.Context, error) {
	result, _, err := layoutengine.WithPlanningBudget(ctx, maxWork)
	return result, err
}

func ensureDocumentPlanningBudget(ctx context.Context) (context.Context, error) {
	result, _, err := layoutengine.EnsurePlanningBudget(ctx)
	return result, err
}
