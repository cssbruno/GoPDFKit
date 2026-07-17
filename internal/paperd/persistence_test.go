// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

func persistenceOptions(root string) WorkspaceOptions {
	return WorkspaceOptions{PersistenceRoot: root, ProjectID: "project-a", PolicyRevision: "policy-v3", DisclosureDomain: DisclosureRestricted}
}

func populatedPersistentWorkspace(t *testing.T, root string) (*Workspace, RevisionSnapshot, CandidateSnapshot, ScenarioRevisionSnapshot, ScenarioCandidateSnapshot) {
	t.Helper()
	workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	revision, err := workspace.CreateRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := workspace.NewCandidate(revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	scenarioRevision, err := workspace.CreateScenarioRevision([]paperscenario.Scenario{{Name: "typical", Locale: "en-US", Values: []paperscenario.Field{{Name: "name", Value: paperscenario.Value{Kind: paperscenario.String, String: "Ada"}}}}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	scenarioCandidate, err := workspace.NewScenarioCandidate(scenarioRevision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	return workspace, revision, candidate, scenarioRevision, scenarioCandidate
}

func TestPersistentRecoveryIsCanonicalHandleFreeAndReissuesCapabilities(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	_, oldRevision, oldCandidate, oldScenarioRevision, oldScenarioCandidate := populatedPersistentWorkspace(t, root)
	manifestBefore, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest persistenceManifest
	if err := json.Unmarshal(manifestBefore, &manifest); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(filepath.Join(root, manifest.Snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"handle", "nonce", "scope", "idempotency", "expires_at"} {
		if strings.Contains(string(snapshot), forbidden) {
			t.Fatalf("persistent snapshot leaked transient field %q: %s", forbidden, snapshot)
		}
	}

	recovered, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.revisions) != 1 || len(recovered.candidates) != 1 || len(recovered.scenarioRevisions) != 1 || len(recovered.scenarioCandidates) != 1 {
		t.Fatalf("recovered counts = %d/%d/%d/%d", len(recovered.revisions), len(recovered.candidates), len(recovered.scenarioRevisions), len(recovered.scenarioCandidates))
	}
	newRevision := recovered.revisions[1].handle
	newCandidate := recovered.candidates[1].handle
	newScenarioRevision := recovered.scenarioRevisions[1].handle
	newScenarioCandidate := recovered.scenarioCandidates[1].handle
	if newRevision == oldRevision.Handle || newCandidate == oldCandidate.Handle || newScenarioRevision == oldScenarioRevision.Handle || newScenarioCandidate == oldScenarioCandidate.Handle {
		t.Fatal("recovery reused a persisted capability")
	}
	if recovered.candidates[1].head != newRevision || recovered.scenarioCandidates[1].head != newScenarioRevision {
		t.Fatal("candidate heads were not deterministically rebound")
	}
	opened, err := recovered.OpenRevision(newRevision)
	if err != nil || opened.Source != workspaceFixture {
		t.Fatalf("recovered revision = %+v, %v", opened, err)
	}
	if _, err := recovered.OpenRevision(oldRevision.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("old capability after recovery = %v", err)
	}
	if err := recovered.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifestAfter, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
	if string(manifestAfter) != string(manifestBefore) {
		t.Fatalf("canonical reopen changed manifest:\n%s\n%s", manifestBefore, manifestAfter)
	}
}

func TestSemanticTemplateAndPolicyDomainsPersistWithFreshCapabilities(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	semanticBase, err := workspace.CreateSemanticTemplateRevision("document @base")
	if err != nil {
		t.Fatal(err)
	}
	semanticCandidate, _ := workspace.NewSemanticTemplateCandidate(semanticBase.Handle)
	semanticApplied, err := workspace.ApplySemanticTemplate(SemanticTemplateApplyRequest{Candidate: semanticCandidate.Handle, ExpectedHead: semanticBase.Handle, ExpectedDigest: semanticBase.Digest, IdempotencyKey: "semantic-save", Content: "document @candidate"})
	if err != nil {
		t.Fatal(err)
	}
	policyBase, err := workspace.CreatePolicyRevision("deny publish")
	if err != nil {
		t.Fatal(err)
	}
	policyCandidate, _ := workspace.NewPolicyCandidate(policyBase.Handle)
	policyApplied, err := workspace.ApplyPolicy(PolicyApplyRequest{Candidate: policyCandidate.Handle, ExpectedHead: policyBase.Handle, ExpectedDigest: policyBase.Digest, IdempotencyKey: "policy-save", Content: "allow edit\ndeny publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifestBefore, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.semanticTemplateRevisions) != 2 || len(recovered.semanticTemplateCandidates) != 1 || len(recovered.policyRevisions) != 2 || len(recovered.policyCandidates) != 1 {
		t.Fatalf("recovered domain counts=%d/%d/%d/%d", len(recovered.semanticTemplateRevisions), len(recovered.semanticTemplateCandidates), len(recovered.policyRevisions), len(recovered.policyCandidates))
	}
	semanticHead := recovered.semanticTemplateCandidates[1].head
	policyHead := recovered.policyCandidates[1].head
	if semanticHead == semanticApplied.Revision.Handle || policyHead == policyApplied.Revision.Handle {
		t.Fatal("recovery reused transient capabilities")
	}
	semanticOpened, err := recovered.OpenSemanticTemplateRevision(semanticHead)
	if err != nil || semanticOpened.Content != "document @candidate" || semanticOpened.Digest != semanticApplied.Revision.Digest {
		t.Fatalf("semantic recovery=%+v,%v", semanticOpened, err)
	}
	policyOpened, err := recovered.OpenPolicyRevision(policyHead)
	if err != nil || policyOpened.Content != "allow edit\ndeny publish" || policyOpened.Digest != policyApplied.Revision.Digest {
		t.Fatalf("policy recovery=%+v,%v", policyOpened, err)
	}
	if len(recovered.semanticTemplateCandidates[1].idempotency) != 0 || len(recovered.policyCandidates[1].idempotency) != 0 {
		t.Fatal("transient idempotency cache was persisted")
	}
	if err := recovered.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifestAfter, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
	if !bytes.Equal(manifestBefore, manifestAfter) {
		t.Fatalf("canonical domain recovery changed manifest:\n%s\n%s", manifestBefore, manifestAfter)
	}
	if _, err := recovered.OpenSemanticTemplateRevision(semanticApplied.Revision.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("old semantic capability=%v", err)
	}
	if _, err := recovered.OpenPolicyRevision(policyApplied.Revision.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("old policy capability=%v", err)
	}
}

func TestPersistenceManifestLastIgnoresInterruptedGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	populatedPersistentWorkspace(t, root)
	orphan := filepath.Join(root, "snapshot-"+strings.Repeat("0", 64)+".json")
	if err := os.WriteFile(orphan, []byte(`{"truncated":`), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil || len(recovered.revisions) != 1 {
		t.Fatalf("recovery with orphan generation = %v, revisions=%d", err, len(recovered.revisions))
	}
	if err := recovered.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "snapshot-") {
			snapshots++
		}
	}
	if snapshots != 1 {
		t.Fatalf("orphan reclamation retained %d snapshots", snapshots)
	}
}

func TestPersistenceRejectsCorruptionTruncationAndUnsafeFiles(t *testing.T) {
	t.Run("digest corruption", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		populatedPersistentWorkspace(t, root)
		manifestBytes, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
		var manifest persistenceManifest
		_ = json.Unmarshal(manifestBytes, &manifest)
		if err := os.WriteFile(filepath.Join(root, manifest.Snapshot), []byte(`{}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenWorkspace(context.Background(), persistenceOptions(root)); !errors.Is(err, ErrPersistenceCorrupt) {
			t.Fatalf("corrupt snapshot error = %v", err)
		}
	})
	t.Run("truncated manifest", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		populatedPersistentWorkspace(t, root)
		if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"version":1`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenWorkspace(context.Background(), persistenceOptions(root)); !errors.Is(err, ErrPersistenceCorrupt) {
			t.Fatalf("truncated manifest error = %v", err)
		}
	})
	t.Run("manifest permissions", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		populatedPersistentWorkspace(t, root)
		if err := os.Chmod(filepath.Join(root, "manifest.json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenWorkspace(context.Background(), persistenceOptions(root)); !errors.Is(err, ErrPersistenceCorrupt) {
			t.Fatalf("permissive manifest error = %v", err)
		}
	})
	t.Run("symlink root", func(t *testing.T) {
		base := t.TempDir()
		realRoot := filepath.Join(base, "real")
		if err := os.Mkdir(realRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(base, "link")
		if err := os.Symlink(realRoot, link); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenWorkspace(context.Background(), persistenceOptions(link)); !errors.Is(err, ErrPersistence) {
			t.Fatalf("symlink root error = %v", err)
		}
	})
	t.Run("relative root", func(t *testing.T) {
		if _, err := OpenWorkspace(context.Background(), persistenceOptions("relative")); !errors.Is(err, ErrPersistence) {
			t.Fatalf("relative root error = %v", err)
		}
	})
	t.Run("root permissions", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenWorkspace(context.Background(), persistenceOptions(root)); !errors.Is(err, ErrPersistence) {
			t.Fatalf("permissive root error = %v", err)
		}
	})
}

func TestPersistenceCancellationQuotaAndConcurrentRecovery(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := workspace.SaveSnapshot(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled save = %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "manifest.json")); !os.IsNotExist(err) {
			t.Fatalf("cancelled save committed a manifest: %v", err)
		}
	})
	t.Run("quota", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		options := persistenceOptions(root)
		options.Limits.MaxPersistenceBytes = 64
		workspace, err := OpenWorkspace(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := workspace.CreateRevision("report.paper", workspaceFixture); err != nil {
			t.Fatal(err)
		}
		if err := workspace.SaveSnapshot(context.Background()); !errors.Is(err, ErrLimit) {
			t.Fatalf("quota save = %v", err)
		}
	})
	t.Run("concurrent reopen", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "state")
		populatedPersistentWorkspace(t, root)
		var wait sync.WaitGroup
		errorsSeen := make(chan error, 16)
		for index := 0; index < 16; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
				if err == nil && (len(workspace.revisions) != 1 || len(workspace.candidates) != 1) {
					err = errors.New("unexpected recovered counts")
				}
				errorsSeen <- err
			}()
		}
		wait.Wait()
		close(errorsSeen)
		for err := range errorsSeen {
			if err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestRetainedCachesRejectProjectPolicyAndDisclosurePartitionCollisions(t *testing.T) {
	if _, err := NewWorkspaceWithOptions(WorkspaceOptions{ProjectID: " invalid "}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("invalid project partition = %v", err)
	}
	first, err := NewWorkspaceWithOptions(WorkspaceOptions{ProjectID: "a", PolicyRevision: "v1", DisclosureDomain: DisclosureRestricted})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := first.CreateRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := first.NewCandidate(revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := first.CreatePlan(revision.Handle)
	if err != nil {
		t.Fatal(err)
	}

	variants := []WorkspaceOptions{
		{ProjectID: "b", PolicyRevision: "v1", DisclosureDomain: DisclosureRestricted},
		{ProjectID: "a", PolicyRevision: "v2", DisclosureDomain: DisclosureRestricted},
		{ProjectID: "a", PolicyRevision: "v1", DisclosureDomain: DisclosurePublic},
	}
	for _, options := range variants {
		other, err := NewWorkspaceWithOptions(options)
		if err != nil {
			t.Fatal(err)
		}
		// Deliberately defeat the outer handle scope/domain checks to prove the
		// retained compiled/context, plan, and idempotency records still cannot
		// hit across the complete cache partition tuple.
		other.scope = first.scope
		other.disclosureTag = first.disclosureTag
		other.revisions[revision.Handle.value.serial] = first.revisions[revision.Handle.value.serial]
		other.candidates[candidate.Handle.value.serial] = first.candidates[candidate.Handle.value.serial]
		other.plans[plan.Handle.value.serial] = first.plans[plan.Handle.value.serial]
		if _, err := other.Context(revision.Handle); !errors.Is(err, ErrRevisionNotFound) {
			t.Fatalf("context cross-partition hit for %+v: %v", options, err)
		}
		if _, err := other.Compile(revision.Handle); !errors.Is(err, ErrRevisionNotFound) {
			t.Fatalf("compile cross-partition hit for %+v: %v", options, err)
		}
		if _, err := other.Candidate(candidate.Handle); !errors.Is(err, ErrCandidateNotFound) {
			t.Fatalf("idempotency-owner cross-partition hit for %+v: %v", options, err)
		}
		if _, err := other.OpenPlan(plan.Handle); !errors.Is(err, ErrPlanNotFound) {
			t.Fatalf("plan cross-partition hit for %+v: %v", options, err)
		}
	}
}
