// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import "testing"

func TestExplicitPageBreakRequiresZeroSpaceEvidence(t *testing.T) {
	input := testPlanInput()
	input.Breaks[0].Reason = BreakExplicitPageBreak
	input.Breaks[0].Required = 0
	input.Breaks[0].Available = 0
	if _, err := NewLayoutPlan(input); err != nil {
		t.Fatalf("NewLayoutPlan(explicit break) = %v", err)
	}
	input.Breaks[0].Required = 1
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("NewLayoutPlan accepted false space evidence for explicit break")
	}
}
