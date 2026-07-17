// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

func retainedExplainFixture(t *testing.T, source string) (*Workspace, PaperCreateResult, PaperOpenSnapshot, PlanSnapshot) {
	t.Helper()
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 1 << 20, MaxSearchResults: 32, MaxScenarioWork: 1_000_000})
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "explain.paper", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead})
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := workspace.CreatePlan(created.Revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	return workspace, created, opened, plan
}

func exactLayoutExplainRequest(created PaperCreateResult, opened PaperOpenSnapshot, plan PlanSnapshot) LayoutIssueExplainRequest {
	return LayoutIssueExplainRequest{Open: opened.Handle, Plan: plan.Handle, ExpectedRevision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Selector: LayoutIssueSelector{Key: "@intro"},
		MaxItems: 16, MaxBytes: 512 << 10, MaxWork: 500_000}
}

func TestExplainLayoutIssueReturnsExactRedactedDeterministicCausalChain(t *testing.T) {
	source := strings.Replace(workspaceFixture, "Hello agent", "TOP SECRET CUSTOMER VALUE", 1)
	workspace, created, opened, plan := retainedExplainFixture(t, source)
	request := exactLayoutExplainRequest(created, opened, plan)
	first, err := workspace.ExplainLayoutIssue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspace.ExplainLayoutIssue(context.Background(), request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic result: %v", err)
	}
	if first.Resolution != LayoutIssueExact || !first.CandidateBound || first.Plan.Hash != plan.Hash ||
		first.Revision.Digest != created.Revision.Revision || len(first.Candidates) != 1 || first.Layout == nil {
		t.Fatalf("identity/resolution = %#v", first)
	}
	if len(first.Layout.Fragments) == 0 || len(first.Layout.Commands) == 0 || len(first.Layout.Semantics) == 0 ||
		first.Layout.Fragments[0].Source.Key != "@intro" || len(first.Source) == 0 {
		t.Fatalf("causal chain incomplete = %#v", first)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != first.EncodedBytes || len(encoded) > int(request.MaxBytes) ||
		strings.Contains(string(encoded), "TOP SECRET") || strings.Contains(string(encoded), "Hello agent") {
		t.Fatalf("response bound/redaction failed: bytes=%d field=%d payload=%s", len(encoded), first.EncodedBytes, encoded)
	}
	first.Candidates[0].Pages[0] = 999
	again, err := workspace.ExplainLayoutIssue(context.Background(), request)
	if err != nil || again.Candidates[0].Pages[0] == 999 {
		t.Fatal("result was not detached")
	}
}

func TestExplainLayoutIssueRejectsStaleCandidateWrongPlanAndInvalidBounds(t *testing.T) {
	workspace, created, opened, plan := retainedExplainFixture(t, workspaceFixture)
	request := exactLayoutExplainRequest(created, opened, plan)
	other, err := workspace.CreateRevision("other.paper", strings.Replace(workspaceFixture, "Hello agent", "Other", 1))
	if err != nil {
		t.Fatal(err)
	}
	otherPlan, _, err := workspace.CreatePlan(other.Handle)
	if err != nil {
		t.Fatal(err)
	}
	wrong := request
	wrong.Plan = otherPlan.Handle
	if _, err := workspace.ExplainLayoutIssue(context.Background(), wrong); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("wrong-plan error = %v", err)
	}
	tiny := request
	tiny.MaxWork = 1
	if _, err := workspace.ExplainLayoutIssue(context.Background(), tiny); !errors.Is(err, ErrLimit) {
		t.Fatalf("work error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := workspace.ExplainLayoutIssue(canceled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	_, err = workspace.Apply(ApplyRequest{Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
		ExpectedRevision: created.Revision.Revision, IdempotencyKey: "advance-explain",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "explain.paper", workspaceFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "advanced"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.ExplainLayoutIssue(context.Background(), request); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale candidate error = %v", err)
	}
}

func TestExplainLayoutIssueConcurrentReadersAndSelectorOutcomes(t *testing.T) {
	workspace, created, opened, plan := retainedExplainFixture(t, workspaceFixture)
	base := exactLayoutExplainRequest(created, opened, plan)
	notFound := base
	notFound.Selector = LayoutIssueSelector{Key: "@missing"}
	result, err := workspace.ExplainLayoutIssue(context.Background(), notFound)
	if err != nil || result.Resolution != LayoutIssueNotFound || result.Layout != nil {
		t.Fatalf("not found = %#v, %v", result, err)
	}
	invalid := base
	invalid.Selector = LayoutIssueSelector{DiagnosticCode: "not canonical"}
	if _, err := workspace.ExplainLayoutIssue(context.Background(), invalid); err == nil {
		t.Fatal("invalid issue code accepted")
	}

	const readers = 16
	results := make([]string, readers)
	errs := make([]error, readers)
	var wg sync.WaitGroup
	for i := range readers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			got, callErr := workspace.ExplainLayoutIssue(context.Background(), base)
			errs[index] = callErr
			if callErr == nil {
				encoded, _ := json.Marshal(got)
				results[index] = string(encoded)
			}
		}(i)
	}
	wg.Wait()
	for i := range readers {
		if errs[i] != nil || results[i] != results[0] {
			t.Fatalf("reader %d = %v deterministic=%v", i, errs[i], results[i] == results[0])
		}
	}
}

func TestExplainLayoutIssueReturnsExactCandidatesInsteadOfGuessingRepeatedInstance(t *testing.T) {
	const source = "document:\n" +
		"  component @card:\n" +
		"    paragraph @copy:\n" +
		"      text: \"private repeated value\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      use @first:\n" +
		"        component: \"@card\"\n" +
		"      use @second:\n" +
		"        component: \"@card\"\n"
	workspace, created, opened, plan := retainedExplainFixture(t, source)
	request := exactLayoutExplainRequest(created, opened, plan)
	request.Selector = LayoutIssueSelector{Page: 1}
	result, err := workspace.ExplainLayoutIssue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Resolution != LayoutIssueAmbiguous || len(result.Candidates) != 2 || result.Layout != nil ||
		result.Candidates[0].Instance != "@first/@first--copy" || result.Candidates[1].Instance != "@second/@second--copy" {
		t.Fatalf("ambiguous candidates = %#v", result)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "private repeated value") {
		t.Fatalf("candidate response leaked data: %s", encoded)
	}
	request.Selector.Instance = string(result.Candidates[1].Instance)
	exact, err := workspace.ExplainLayoutIssue(context.Background(), request)
	if err != nil || exact.Resolution != LayoutIssueExact || exact.Layout == nil || len(exact.Candidates) != 1 ||
		exact.Candidates[0].Instance != "@second/@second--copy" {
		t.Fatalf("exact candidate explanation = %#v, %v", exact, err)
	}
}
