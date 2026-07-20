// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperscenario"
)

func TestScenarioRevisionIsSeparateScopedAndImmutable(t *testing.T) {
	first := mustWorkspace(t, Limits{})
	second := mustWorkspace(t, Limits{})
	created, err := first.CreateScenarioRevision([]paperscenario.Scenario{{Name: "typical", Locale: "en-US", Values: []paperscenario.Field{{Name: "name", Value: paperscenario.Value{Kind: paperscenario.String, String: "Ada"}}}}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Fixtures) != 1 || created.Fixtures[0].Digest == "" {
		t.Fatalf("snapshot = %+v", created)
	}
	created.Fixtures[0].Name = "changed"
	opened, err := first.OpenScenarioRevision(created.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Fixtures[0].Name != "typical" {
		t.Fatal("snapshot aliases retained state")
	}
	if _, err := second.OpenScenarioRevision(created.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("cross-workspace error = %v", err)
	}
	if _, err := first.OpenScenarioRevision(ScenarioRevisionHandle{}); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("zero handle error = %v", err)
	}
}

func TestScenarioRevisionCapacityAndInvalidInput(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxScenarioRevisions: 1})
	valid := []paperscenario.Scenario{{Name: "empty"}}
	if _, err := workspace.CreateScenarioRevision(valid, paperscenario.Limits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.CreateScenarioRevision(valid, paperscenario.Limits{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("capacity error = %v", err)
	}
	other := mustWorkspace(t, Limits{})
	if _, err := other.CreateScenarioRevision([]paperscenario.Scenario{{Name: "a", Parent: "missing"}}, paperscenario.Limits{}); !errors.Is(err, paperscenario.ErrInvalid) {
		t.Fatalf("invalid scenario error = %v", err)
	}
}
