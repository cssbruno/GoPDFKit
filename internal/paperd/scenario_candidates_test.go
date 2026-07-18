// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

func TestScenarioCandidateApplyIsImmutableIdempotentAndRedacted(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	base := mustScenarioRevision(t, workspace)
	candidate, err := workspace.NewScenarioCandidate(base.Handle)
	if err != nil {
		t.Fatal(err)
	}
	request := ScenarioApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "scenario-edit-1",
		Operations: []ScenarioOperation{
			{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer.name", Value: scenarioString("Grace Hopper")},
			{Kind: ScenarioOperationDeleteValue, Scenario: "typical", Path: "customer.secret"},
			{Kind: ScenarioOperationReplaceListItem, Scenario: "typical", Path: "lines", Key: "line-a", Value: scenarioString("replacement-sensitive")},
		},
	}
	result, err := workspace.ApplyScenario(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 3 || result.Revision.Handle == base.Handle || result.Candidate.Head != result.Revision.Handle {
		t.Fatalf("ApplyScenario() = %+v", result)
	}
	if result.Revision.Digest == base.Digest || result.Candidate.HeadDigest != result.Revision.Digest || result.Candidate.FixtureCount != 1 {
		t.Fatalf("candidate identity = %+v", result.Candidate)
	}
	assertScenarioResultRedacted(t, result)

	baseFixture := retainedScenarioFixture(t, workspace, base.Handle)
	if scenarioNestedString(t, baseFixture.Values, "customer", "name") != "Ada Lovelace" || scenarioNestedString(t, baseFixture.Values, "customer", "secret") != "fixture-password" {
		t.Fatal("base scenario revision was mutated")
	}
	edited := retainedScenarioFixture(t, workspace, result.Revision.Handle)
	if scenarioNestedString(t, edited.Values, "customer", "name") != "Grace Hopper" {
		t.Fatal("set-value operation was not retained")
	}
	if scenarioFieldForTest(scenarioFieldForTest(edited.Values, "customer").Object, "secret").Kind != "" {
		t.Fatal("delete-value operation was not retained")
	}
	line := scenarioFieldForTest(edited.Values, "lines")
	if len(line.List) != 2 || line.List[0].Key != "line-a" || line.List[0].Value.String != "replacement-sensitive" || line.List[1].Key != "line-b" {
		t.Fatalf("stable keyed list = %+v", line.List)
	}

	retry, err := workspace.ApplyScenario(request)
	if err != nil || retry.Revision.Handle != result.Revision.Handle {
		t.Fatalf("idempotent retry = %+v, %v", retry, err)
	}
	retry.Revision.Fixtures[0].Name = "mutated-snapshot"
	opened, err := workspace.OpenScenarioRevision(result.Revision.Handle)
	if err != nil || opened.Fixtures[0].Name != "typical" {
		t.Fatal("cached result aliases retained state")
	}
	request.Operations[0].Value = scenarioString("different-secret")
	if _, err := workspace.ApplyScenario(request); !errors.Is(err, ErrRevisionConflict) || errorCode(err) != "SCENARIO_IDEMPOTENCY_CONFLICT" {
		t.Fatalf("idempotency conflict = %v", err)
	}
}

func TestScenarioCandidateWrongWorkspaceAndExactHeadConflicts(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	other := mustWorkspace(t, Limits{})
	base := mustScenarioRevision(t, workspace)
	foreign := mustScenarioRevision(t, other)
	candidate, _ := workspace.NewScenarioCandidate(base.Handle)
	operation := []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer.name", Value: scenarioString("changed")}}

	if _, err := other.ScenarioCandidate(candidate.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("foreign candidate error = %v", err)
	}
	if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: foreign.Handle, ExpectedDigest: foreign.Digest,
		IdempotencyKey: "wrong-workspace", Operations: operation,
	}); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("foreign expected head error = %v", err)
	}
	first, err := workspace.ApplyScenario(ScenarioApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest,
		IdempotencyKey: "first", Operations: operation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest,
		IdempotencyKey: "stale", Operations: operation,
	}); !errors.Is(err, ErrRevisionConflict) || errorCode(err) != "SCENARIO_REVISION_CONFLICT" {
		t.Fatalf("stale head error = %v", err)
	}
	if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: first.Revision.Handle, ExpectedDigest: base.Digest,
		IdempotencyKey: "bad-digest", Operations: operation,
	}); !errors.Is(err, ErrRevisionConflict) || errorCode(err) != "SCENARIO_REVISION_CONFLICT" {
		t.Fatalf("digest conflict error = %v", err)
	}
}

func TestScenarioCandidateConcurrentCASAndIdempotentRetry(t *testing.T) {
	t.Run("different requests", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wait sync.WaitGroup
		for i, value := range []string{"first", "second"} {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, err := workspace.ApplyScenario(ScenarioApplyRequest{
					Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest,
					IdempotencyKey: "concurrent-" + string(rune('a'+i)),
					Operations:     []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer.name", Value: scenarioString(value)}},
				})
				results <- err
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		succeeded, conflicted := 0, 0
		for err := range results {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrRevisionConflict):
				conflicted++
			default:
				t.Fatalf("unexpected error = %v", err)
			}
		}
		if succeeded != 1 || conflicted != 1 {
			t.Fatalf("success/conflict = %d/%d", succeeded, conflicted)
		}
	})

	t.Run("same request", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		request := ScenarioApplyRequest{
			Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "same-request",
			Operations: []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer.name", Value: scenarioString("same")}},
		}
		start := make(chan struct{})
		results := make(chan ScenarioApplyResult, 2)
		errorsSeen := make(chan error, 2)
		var wait sync.WaitGroup
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				result, err := workspace.ApplyScenario(request)
				results <- result
				errorsSeen <- err
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		close(errorsSeen)
		for err := range errorsSeen {
			if err != nil {
				t.Fatalf("same request error = %v", err)
			}
		}
		var handle ScenarioRevisionHandle
		for result := range results {
			if handle.value.serial == 0 {
				handle = result.Revision.Handle
			} else if result.Revision.Handle != handle {
				t.Fatalf("idempotent handles differ: %+v %+v", handle, result.Revision.Handle)
			}
		}
		workspace.mu.RLock()
		revisions := len(workspace.scenarioRevisions)
		workspace.mu.RUnlock()
		if revisions != 2 {
			t.Fatalf("retained revisions = %d, want base plus one edit", revisions)
		}
	})
}

func TestScenarioCandidateLimitsAreHard(t *testing.T) {
	t.Run("candidate", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{MaxScenarioCandidates: 1})
		base := mustScenarioRevision(t, workspace)
		if _, err := workspace.NewScenarioCandidate(base.Handle); err != nil {
			t.Fatal(err)
		}
		if _, err := workspace.NewScenarioCandidate(base.Handle); !errors.Is(err, ErrLimit) {
			t.Fatalf("candidate limit error = %v", err)
		}
	})
	t.Run("operations", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{MaxScenarioOperations: 1})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		op := ScenarioOperation{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer.name", Value: scenarioString("x")}
		if _, err := workspace.ApplyScenario(ScenarioApplyRequest{Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "ops", Operations: []ScenarioOperation{op, op}}); !errors.Is(err, ErrLimit) {
			t.Fatalf("operation limit error = %v", err)
		}
	})
	t.Run("path", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{MaxScenarioPathBytes: 3})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
			Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "path",
			Operations: []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "long", Value: scenarioString("x")}},
		}); !errors.Is(err, ErrLimit) {
			t.Fatalf("path limit error = %v", err)
		}
	})
	t.Run("value nodes", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{MaxScenarioValueNodes: 1})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		value := paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{{Name: "child", Value: scenarioString("x")}}}
		if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
			Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "nodes",
			Operations: []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer", Value: value}},
		}); !errors.Is(err, ErrLimit) {
			t.Fatalf("node limit error = %v", err)
		}
	})
	t.Run("work", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{MaxScenarioWork: 4})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
			Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "work",
			Operations: []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "x", Value: scenarioString("x")}},
		}); !errors.Is(err, ErrLimit) {
			t.Fatalf("work limit error = %v", err)
		}
	})
	t.Run("revisions", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{MaxScenarioRevisions: 1})
		base := mustScenarioRevision(t, workspace)
		candidate, _ := workspace.NewScenarioCandidate(base.Handle)
		if _, err := workspace.ApplyScenario(ScenarioApplyRequest{
			Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "revision",
			Operations: []ScenarioOperation{{Kind: ScenarioOperationSetValue, Scenario: "typical", Path: "customer.name", Value: scenarioString("x")}},
		}); !errors.Is(err, ErrLimit) {
			t.Fatalf("revision limit error = %v", err)
		}
	})
}

func mustScenarioRevision(t *testing.T, workspace *Workspace) ScenarioRevisionSnapshot {
	t.Helper()
	revision, err := workspace.CreateScenarioRevision([]paperscenario.Scenario{{
		Name: "typical", Locale: "en-US",
		Values: []paperscenario.Field{
			{Name: "customer", Value: paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{
				{Name: "name", Value: scenarioString("Ada Lovelace")},
				{Name: "secret", Value: scenarioString("fixture-password")},
			}}},
			{Name: "lines", Value: paperscenario.Value{Kind: paperscenario.List, List: []paperscenario.Item{
				{Key: "line-a", Value: scenarioString("alpha")},
				{Key: "line-b", Value: scenarioString("beta")},
			}}},
		},
	}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func retainedScenarioFixture(t *testing.T, workspace *Workspace, handle ScenarioRevisionHandle) paperscenario.Fixture {
	t.Helper()
	workspace.mu.RLock()
	record, err := workspace.scenarioRevisionLocked(handle)
	if err != nil {
		workspace.mu.RUnlock()
		t.Fatal(err)
	}
	fixture := cloneScenarioFixtures(record.fixtures)[0]
	workspace.mu.RUnlock()
	return fixture
}

func scenarioString(value string) paperscenario.Value {
	return paperscenario.Value{Kind: paperscenario.String, String: value}
}

func scenarioNestedString(t *testing.T, fields []paperscenario.Field, parent, child string) string {
	t.Helper()
	value := scenarioFieldForTest(fields, parent)
	childValue := scenarioFieldForTest(value.Object, child)
	if childValue.Kind != paperscenario.String {
		t.Fatalf("%s.%s = %+v", parent, child, childValue)
	}
	return childValue.String
}

func scenarioFieldForTest(fields []paperscenario.Field, name string) paperscenario.Value {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return paperscenario.Value{}
}

func assertScenarioResultRedacted(t *testing.T, result ScenarioApplyResult) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{"Grace Hopper", "replacement-sensitive", "fixture-password", `"value"`, `"object"`, `"list"`} {
		if strings.Contains(text, secret) {
			t.Fatalf("public result leaked fixture data %q: %s", secret, text)
		}
	}
}

func errorCode(err error) string {
	var workspaceErr *Error
	if errors.As(err, &workspaceErr) {
		return workspaceErr.Code
	}
	return ""
}
