// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

const authorizationComponentFixture = "document @report:\n" +
	"  component @card:\n" +
	"    paragraph @template-paragraph:\n" +
	"      text @template-copy: \"Template\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @instance:\n" +
	"        component: \"@card\"\n"

func authorizationWorkspace(t *testing.T, options WorkspaceOptions) *Workspace {
	t.Helper()
	workspace, err := NewWorkspaceWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func grantMutationAuthority(t *testing.T, workspace *Workspace, opened PaperOpenSnapshot, actor string, operations []MutationOperation, scopes, protected []string) MutationAuthorityHandle {
	t.Helper()
	granted, err := workspace.GrantMutationAuthority(MutationAuthorityGrant{
		Open: opened.Handle, Actor: actor, Operations: operations,
		NodeScopes: scopes, ProtectedNodes: protected,
	})
	if err != nil {
		t.Fatal(err)
	}
	return granted.Handle
}

func TestSlotValidityIsEvaluatedBeforeActorAuthority(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	guard, created, opened := mutationGuard(t, workspace, slotMutationFixture, "@instance", "slot-authority", CapabilityEdit)
	wrongType := PaperFillSlotRequest{
		Guard: guard, Slot: "@content",
		Content: []paperedit.NodeSpec{{Kind: paperlang.NodeList, Children: []paperedit.NodeSpec{{Kind: paperlang.NodeItem}}}},
	}
	if _, err := workspace.PaperFillSlot(wrongType); errorCode(err) != "SLOT_TYPE" {
		t.Fatalf("structural error was hidden by authority error: %v", err)
	}
	if audit, err := workspace.AuthorizationAudit(8); err != nil || len(audit) != 0 {
		t.Fatalf("structurally invalid request reached authorization: %#v, %v", audit, err)
	}

	valid := PaperFillSlotRequest{
		Guard: guard, Slot: "@content",
		Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("authorized"))}},
	}
	if _, err := workspace.PaperFillSlot(valid); !errors.Is(err, ErrMutationAuthorityDenied) || errorCode(err) != "AUTHORITY_REQUIRED" {
		t.Fatalf("valid unauthorised slot fill = %v", err)
	}
	candidate, _ := workspace.Candidate(created.Candidate.Handle)
	if candidate.Head != created.Revision.Handle {
		t.Fatal("denied slot fill advanced candidate")
	}
	valid.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:slot", []MutationOperation{MutationFillSlot}, []string{"@instance"}, nil)
	result, err := workspace.PaperFillSlot(valid)
	if err != nil || !result.Authorization.Explicit || !result.Authorization.Allowed || result.Authorization.Operation != MutationFillSlot {
		t.Fatalf("authorized slot fill = %#v, %v", result.Authorization, err)
	}
}

func TestMutationAuthorityEnforcesOperationProtectedAndTransitiveScopes(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{ProtectedNodeIDs: []string{"@card"}})
	guard, created, opened := mutationGuard(t, workspace, authorizationComponentFixture, "@template-copy", "protected-edit", CapabilityEdit)
	request := PaperSetLiteralRequest{Guard: guard, Text: "Changed"}

	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:protected", []MutationOperation{MutationSetLiteral}, []string{"@card", "@instance"}, nil)
	if _, err := workspace.PaperSetLiteral(request); errorCode(err) != "PROTECTED_NODE_DENIED" {
		t.Fatalf("missing protected grant = %v", err)
	}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:wrong-operation", []MutationOperation{MutationSetBinding}, []string{"@card", "@instance"}, []string{"@card"})
	if _, err := workspace.PaperSetLiteral(request); errorCode(err) != "AUTHORITY_OPERATION_DENIED" {
		t.Fatalf("wrong operation = %v", err)
	}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:narrow", []MutationOperation{MutationSetLiteral}, []string{"@card"}, []string{"@card"})
	if _, err := workspace.PaperSetLiteral(request); errorCode(err) != "AUTHORITY_SCOPE_DENIED" {
		t.Fatalf("transitive instance escaped scope check: %v", err)
	}
	candidate, _ := workspace.Candidate(created.Candidate.Handle)
	if candidate.Head != created.Revision.Handle {
		t.Fatal("denied protected edits advanced candidate")
	}

	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:allowed", []MutationOperation{MutationSetLiteral}, []string{"@card", "@instance"}, []string{"@card"})
	result, err := workspace.PaperSetLiteral(request)
	if err != nil {
		t.Fatal(err)
	}
	wantEffects := []AuthorizationEffect{{Node: "@card", Reason: "governing_component"}, {Node: "@instance", Reason: "component_instance"}, {Node: "@template-copy", Reason: "direct"}}
	if !reflect.DeepEqual(result.Authorization.Effects, wantEffects) || !reflect.DeepEqual(result.Authorization.ProtectedEffects, []string{"@card"}) {
		t.Fatalf("authorization effects = %#v protected %#v", result.Authorization.Effects, result.Authorization.ProtectedEffects)
	}
	audit, err := workspace.AuthorizationAudit(8)
	if err != nil || len(audit) != 4 || audit[0].Allowed || audit[1].Allowed || audit[2].Allowed || !audit[3].Allowed {
		t.Fatalf("bounded decision audit = %#v, %v", audit, err)
	}
}

func TestDiagnosticFixRequiresApplyFixAuthority(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	request, _, opened := diagnosticFixRequest(t, workspace, invalidBindingFixFixture, "@amount", "authorized-fix", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityEdit)
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:fix", []MutationOperation{MutationSetBinding}, []string{"@amount"}, nil)
	if _, err := workspace.PaperApplyDiagnosticFix(request); errorCode(err) != "AUTHORITY_OPERATION_DENIED" {
		t.Fatalf("diagnostic fix accepted binding authority: %v", err)
	}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:fix", []MutationOperation{MutationApplyFix}, []string{"@amount"}, nil)
	result, err := workspace.PaperApplyDiagnosticFix(request)
	if err != nil || result.Authorization.Operation != MutationApplyFix || !result.Authorization.Allowed {
		t.Fatalf("authorized diagnostic fix = %#v, %v", result.Authorization, err)
	}
}

func TestMutationAuthorityRevocationExpiryReplayAndConcurrency(t *testing.T) {
	t.Run("revocation blocks idempotent replay", func(t *testing.T) {
		workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
		guard, created, opened := mutationGuard(t, workspace, workspaceFixture, "@intro", "revoked-replay", CapabilityEdit)
		guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:revoked", []MutationOperation{MutationSetLiteral}, nil, nil)
		if err := workspace.RevokeMutationAuthority(guard.Authority); err != nil {
			t.Fatal(err)
		}
		if _, err := workspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: guard, Text: "denied"}); errorCode(err) != "AUTHORITY_REQUIRED" {
			t.Fatalf("revoked replay = %v", err)
		}
		candidate, _ := workspace.Candidate(created.Candidate.Handle)
		if candidate.Head != created.Revision.Handle {
			t.Fatal("revoked request advanced candidate")
		}
	})

	t.Run("expiry uses injected clock", func(t *testing.T) {
		now := time.Unix(1_900_000_000, 0).UTC()
		workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true, HandleTTL: time.Minute, Now: func() time.Time { return now }})
		_, _, opened := mutationGuard(t, workspace, workspaceFixture, "@intro", "expiry", CapabilityEdit)
		handle := grantMutationAuthority(t, workspace, opened, "agent:expiry", []MutationOperation{MutationSetLiteral}, nil, nil)
		now = now.Add(time.Minute)
		workspace.PruneExpiredHandles()
		if _, err := workspace.OpenMutationAuthority(handle); !errors.Is(err, ErrHandleExpired) {
			t.Fatalf("expired authority = %v", err)
		}
	})

	t.Run("idempotent concurrent requests", func(t *testing.T) {
		workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
		guard, _, opened := mutationGuard(t, workspace, workspaceFixture, "@intro", "concurrent-authorized", CapabilityEdit)
		guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:race", []MutationOperation{MutationSetLiteral}, nil, nil)
		request := PaperSetLiteralRequest{Guard: guard, Text: "concurrent"}
		results := make([]PaperMutationResult, 2)
		errs := make([]error, 2)
		var wait sync.WaitGroup
		for i := range results {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				results[index], errs[index] = workspace.PaperSetLiteral(request)
			}(i)
		}
		wait.Wait()
		if errs[0] != nil || errs[1] != nil || !reflect.DeepEqual(results[0], results[1]) {
			t.Fatalf("concurrent idempotent mutation = %#v %#v, %v %v", results[0], results[1], errs[0], errs[1])
		}
	})
}
