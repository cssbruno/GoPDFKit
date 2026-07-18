// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

const boxMutationFixture = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      paragraph @box:\n" +
	"        padding: 4pt\n" +
	"        text @copy: \"Box\"\n"

const invalidFontMutationFixture = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      paragraph @copy:\n" +
	"        font: \"Unavailable Sans\"\n" +
	"        text: \"Strict font\"\n"

const gridMutationFixture = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      row @grid:\n" +
	"        gap: 4pt\n" +
	"        paragraph @left:\n" +
	"          track: \"fixed\"\n" +
	"          track-size: 40pt\n" +
	"          text @left-copy: \"Left\"\n" +
	"        paragraph @right:\n" +
	"          track: \"fraction\"\n" +
	"          track-weight: 1\n" +
	"          text @right-copy: \"Right\"\n"

const pageMarginMutationFixture = "document @report:\n  page @sheet:\n    width: 200pt\n    height: 120pt\n    margin: 8pt\n    body @body:\n      paragraph @copy:\n        text: \"Page\"\n"

const canvasMutationFixture = "document @report:\n  page @sheet:\n    body @body:\n      canvas @diagram:\n        width: 160pt\n        height: 80pt\n        anchor @base:\n          width: 40pt\n          height: 20pt\n          left: \"canvas.left\"\n          top: \"canvas.top\"\n        anchor @badge:\n          width: 24pt\n          height: 12pt\n          left: \"@base.right\"\n          top: \"@base.top\"\n"

func TestPaperSetCanvasAnchorIsReadableTransitiveAndAuthorized(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	guard, _, opened := mutationGuard(t, workspace, canvasMutationFixture, "@badge", "canvas-anchor", CapabilityEdit)
	fingerprint, err := paperedit.FingerprintNode("test.paper", canvasMutationFixture, "@diagram")
	if err != nil {
		t.Fatal(err)
	}
	instance, err := paperedit.SourceInstance("test.paper", canvasMutationFixture, "@diagram")
	if err != nil {
		t.Fatal(err)
	}
	guard.TargetPreconditions = []paperedit.TargetPrecondition{{Target: "@diagram", ExpectedFingerprint: fingerprint, ExpectedInstance: instance}}
	guard.Authority = grantMutationAuthority(t, workspace, opened, "studio:canvas", []MutationOperation{MutationSetCanvasAnchor}, []string{"@badge", "@diagram"}, nil)
	result, err := workspace.PaperSetCanvasAnchor(PaperSetCanvasAnchorRequest{Guard: guard, Property: PaperCanvasLeft, Reference: "@base", TargetAnchor: PaperCanvasRight, Offset: 8})
	if err != nil || !result.Authorization.Allowed || result.Authorization.Operation != MutationSetCanvasAnchor ||
		result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, `left: "@base.right + 8pt"`) {
		t.Fatalf("canvas anchor = %#v, %v", result, err)
	}
}

func TestPaperSetPageMarginIsReadableMinimalAndAuthorized(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	guard, _, opened := mutationGuard(t, workspace, pageMarginMutationFixture, "@sheet", "page-margin", CapabilityEdit)
	guard.Authority = grantMutationAuthority(t, workspace, opened, "studio:page-master", []MutationOperation{MutationSetPageMargin}, []string{"@sheet"}, nil)
	result, err := workspace.PaperSetPageMargin(PaperSetPageMarginRequest{Guard: guard, Property: PaperPageMarginLeft, Points: 16})
	if err != nil || !result.Authorization.Allowed || result.Authorization.Operation != MutationSetPageMargin ||
		result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, "margin-left: 16pt") {
		t.Fatalf("page margin = %#v, %v", result, err)
	}
}

func TestPaperSetPageSizeWritesTwoExactDimensions(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	guard, _, opened := mutationGuard(t, workspace, pageMarginMutationFixture, "@sheet", "page-size", CapabilityEdit)
	guard.Authority = grantMutationAuthority(t, workspace, opened, "studio:page-size", []MutationOperation{MutationSetPageSize}, []string{"@sheet"}, nil)
	result, err := workspace.PaperSetPageSize(PaperSetPageSizeRequest{Guard: guard, WidthPoints: 595.275590551, HeightPoints: 841.88976378})
	if err != nil || !result.Authorization.Allowed || result.Authorization.Operation != MutationSetPageSize ||
		result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 2 || !strings.Contains(result.Revision.Source, "width: 595.275590551pt") || !strings.Contains(result.Revision.Source, "height: 841.88976378pt") {
		t.Fatalf("page size = %#v, %v", result, err)
	}
}

func TestPaperSetPageRegionGuardsGoverningPage(t *testing.T) {
	source := "document @report:\n  page @sheet:\n    header @head:\n      paragraph @copy:\n        text: \"Header\"\n    body @body:\n      paragraph @main:\n        text: \"Body\"\n"
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	guard, _, opened := mutationGuard(t, workspace, source, "@head", "page-region", CapabilityEdit)
	fingerprint, _ := paperedit.FingerprintNode("test.paper", source, "@sheet")
	instance, _ := paperedit.SourceInstance("test.paper", source, "@sheet")
	guard.TargetPreconditions = []paperedit.TargetPrecondition{{Target: "@sheet", ExpectedFingerprint: fingerprint, ExpectedInstance: instance}}
	guard.Authority = grantMutationAuthority(t, workspace, opened, "studio:region", []MutationOperation{MutationSetPageRegion}, []string{"@head", "@sheet"}, nil)
	result, err := workspace.PaperSetPageRegion(PaperSetPageRegionRequest{Guard: guard, Property: "background", Color: "#AABBCC"})
	if err != nil || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, `background: "#aabbcc"`) {
		t.Fatalf("page region = %#v, %v", result, err)
	}
}

func TestPaperSetBoxPropertyIsTypedMinimalAndAuthorized(t *testing.T) {
	tests := []struct {
		name     string
		property PaperBoxProperty
		points   float64
		color    string
		want     string
	}{
		{"padding", PaperBoxPadding, 8.5, "", "padding: 8.5pt"},
		{"border", PaperBoxBorderWidth, 1.25, "", "border-width: 1.25pt"},
		{"radius", PaperBoxRadius, 4, "", "border-radius: 4pt"},
		{"background", PaperBoxBackground, 0, "#AABBCC", `background: "#aabbcc"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
			guard, _, opened := mutationGuard(t, workspace, boxMutationFixture, "@box", "box-"+test.name, CapabilityEdit)
			guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:box", []MutationOperation{MutationSetBoxProperty}, []string{"@box"}, nil)
			result, err := workspace.PaperSetBoxProperty(PaperSetBoxPropertyRequest{Guard: guard, Property: test.property, Points: test.points, Color: test.color})
			if err != nil {
				t.Fatalf("PaperSetBoxProperty() error = %v", err)
			}
			if !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, test.want) {
				t.Fatalf("box mutation = %#v", result)
			}
			if !result.Authorization.Explicit || !result.Authorization.Allowed || result.Authorization.Operation != MutationSetBoxProperty {
				t.Fatalf("authorization = %#v", result.Authorization)
			}
		})
	}
}

func TestPaperSetTextPropertyExplicitlyRepairsUnavailableFont(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	guard, created, opened := mutationGuard(t, workspace, invalidFontMutationFixture, "@copy", "font-replacement", CapabilityEdit)
	if created.Revision.CompileOK {
		t.Fatal("unavailable font unexpectedly compiled")
	}
	guard.Authority = grantMutationAuthority(t, workspace, opened, "studio:font-replacement", []MutationOperation{MutationSetTextProperty}, []string{"@copy"}, nil)
	result, err := workspace.PaperSetTextProperty(PaperSetTextPropertyRequest{Guard: guard, Property: PaperTextFont, Text: "Helvetica"})
	if err != nil || !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 ||
		!strings.Contains(result.Revision.Source, `font: "Helvetica"`) || result.Authorization.Operation != MutationSetTextProperty {
		t.Fatalf("font replacement = %#v, %v", result, err)
	}

	workspace = mustWorkspace(t, Limits{})
	invalidGuard, invalidCreated, _ := mutationGuard(t, workspace, invalidFontMutationFixture, "@copy", "font-invalid", CapabilityEdit)
	if _, err := workspace.PaperSetTextProperty(PaperSetTextPropertyRequest{Guard: invalidGuard, Property: PaperTextFont, Text: "Another Missing Font"}); err == nil {
		t.Fatal("unsupported replacement unexpectedly succeeded")
	}
	if candidate, _ := workspace.Candidate(invalidCreated.Candidate.Handle); candidate.Head != invalidCreated.Revision.Handle {
		t.Fatal("invalid replacement advanced candidate")
	}
}

func TestPaperSetBoxPropertyRejectsAdversarialPayloadsBeforePublication(t *testing.T) {
	tests := []PaperSetBoxPropertyRequest{
		{Property: PaperBoxProperty("padding\nowned"), Points: 1},
		{Property: PaperBoxPadding, Points: math.NaN()},
		{Property: PaperBoxPadding, Points: -1},
		{Property: PaperBoxPadding, Points: 1_000_001},
		{Property: PaperBoxBackground, Color: "red"},
		{Property: PaperBoxBackground, Color: "#112233\nowned"},
	}
	for index, request := range tests {
		workspace := mustWorkspace(t, Limits{})
		guard, created, _ := mutationGuard(t, workspace, boxMutationFixture, "@box", "box-invalid-"+string(rune('a'+index)), CapabilityEdit)
		request.Guard = guard
		if _, err := workspace.PaperSetBoxProperty(request); err == nil {
			t.Fatalf("request %d unexpectedly succeeded", index)
		}
		candidate, err := workspace.Candidate(created.Candidate.Handle)
		if err != nil || candidate.Head != created.Revision.Handle {
			t.Fatalf("invalid request advanced candidate: %#v, %v", candidate, err)
		}
	}
}

func gridRequest(t *testing.T, workspace *Workspace, key string) (PaperSetGridTrackRequest, PaperCreateResult, PaperOpenSnapshot) {
	t.Helper()
	guard, created, opened := mutationGuard(t, workspace, gridMutationFixture, "@left", key, CapabilityEdit)
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", gridMutationFixture, "@grid")}
	return PaperSetGridTrackRequest{Guard: guard, Property: PaperGridTrackSize, Points: 48}, created, opened
}

func TestPaperSetGridTrackRequiresExactTransitiveGuardAndCompilesBeforeCommit(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
	request, created, opened := gridRequest(t, workspace, "grid-normal")
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:grid", []MutationOperation{MutationSetGridTrack}, []string{"@grid"}, nil)
	result, err := workspace.PaperSetGridTrack(request)
	if err != nil {
		t.Fatalf("PaperSetGridTrack() error = %v", err)
	}
	if !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, "track-size: 48pt") {
		t.Fatalf("grid mutation = %#v", result)
	}
	if result.Authorization.Operation != MutationSetGridTrack || len(result.Authorization.DirectTargets) != 2 {
		t.Fatalf("authorization = %#v", result.Authorization)
	}

	workspace = mustWorkspace(t, Limits{})
	missing, missingCreated, _ := gridRequest(t, workspace, "grid-missing-parent")
	missing.Guard.TargetPreconditions = nil
	if _, err := workspace.PaperSetGridTrack(missing); errorCode(err) != "TRANSITIVE_PRECONDITION_REQUIRED" {
		t.Fatalf("missing parent guard = %v", err)
	}
	if candidate, _ := workspace.Candidate(missingCreated.Candidate.Handle); candidate.Head != missingCreated.Revision.Handle {
		t.Fatal("missing parent guard advanced candidate")
	}

	workspace = mustWorkspace(t, Limits{})
	stale, staleCreated, _ := gridRequest(t, workspace, "grid-stale-parent")
	stale.Guard.TargetPreconditions[0].ExpectedFingerprint = paperedit.NodeFingerprint(strings.Repeat("0", 64))
	if _, err := workspace.PaperSetGridTrack(stale); errorCode(err) != "TRANSITIVE_PRECONDITION_CONFLICT" {
		t.Fatalf("stale parent guard = %v", err)
	}
	if candidate, _ := workspace.Candidate(staleCreated.Candidate.Handle); candidate.Head != staleCreated.Revision.Handle {
		t.Fatal("stale parent guard advanced candidate")
	}

	workspace = mustWorkspace(t, Limits{})
	invalid, invalidCreated, _ := gridRequest(t, workspace, "grid-invalid-candidate")
	invalid.Property, invalid.Kind, invalid.Points = PaperGridTrackKind, "fraction", 0
	if _, err := workspace.PaperSetGridTrack(invalid); errorCode(err) != "INVALID_GRID_TRACK" {
		t.Fatalf("invalid compiled candidate = %v", err)
	}
	if candidate, _ := workspace.Candidate(invalidCreated.Candidate.Handle); candidate.Head != invalidCreated.Revision.Handle {
		t.Fatal("invalid compiled candidate advanced head")
	}
	_ = created
}

func TestPaperSetGridTrackAcceptsResponsiveAndAutomaticSizes(t *testing.T) {
	for _, test := range []struct {
		name   string
		length string
		want   string
	}{{"percentage", "50%", "track-size: 50%"}, {"automatic", "auto", `track-size: "auto"`}} {
		t.Run(test.name, func(t *testing.T) {
			workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
			request, _, opened := gridRequest(t, workspace, "grid-responsive-"+test.name)
			request.Points, request.Length = 0, test.length
			request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:grid", []MutationOperation{MutationSetGridTrack}, []string{"@grid"}, nil)
			result, err := workspace.PaperSetGridTrack(request)
			if err != nil || !result.Revision.CompileOK || !strings.Contains(result.Revision.Source, test.want) {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestPaperSetGridTrackAuthorsFlexFactorsAndCrossAxisConstraints(t *testing.T) {
	tests := []struct {
		name     string
		property PaperGridTrackProperty
		length   string
		kind     string
		factor   float64
		want     string
	}{
		{name: "grow", property: PaperGridTrackGrow, factor: 1.5, want: "track-grow: 1.5"},
		{name: "shrink-zero", property: PaperGridTrackShrink, factor: 0, want: "track-shrink: 0"},
		{name: "cross-size", property: PaperGridCrossSize, length: "50%", want: "cross-size: 50%"},
		{name: "cross-min", property: PaperGridCrossMin, length: "20%", want: "cross-min: 20%"},
		{name: "cross-max", property: PaperGridCrossMax, length: "80%", want: "cross-max: 80%"},
		{name: "cross-align", property: PaperGridCrossAlign, kind: "stretch", want: `cross-align: "stretch"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
			request, _, opened := gridRequest(t, workspace, "grid-flex-"+test.name)
			request.Property, request.Points, request.Length, request.Kind, request.Factor = test.property, 0, test.length, test.kind, test.factor
			request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:grid", []MutationOperation{MutationSetGridTrack}, []string{"@grid"}, nil)
			result, err := workspace.PaperSetGridTrack(request)
			if err != nil || !result.Revision.CompileOK || !strings.Contains(result.Revision.Source, test.want) {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}

	workspace := mustWorkspace(t, Limits{})
	invalid, created, _ := gridRequest(t, workspace, "grid-flex-invalid")
	invalid.Property, invalid.Points, invalid.Factor = PaperGridTrackGrow, 0, 0.1234567
	if _, err := workspace.PaperSetGridTrack(invalid); errorCode(err) != "INVALID_GRID_TRACK_VALUE" {
		t.Fatalf("invalid factor = %v", err)
	}
	if candidate, _ := workspace.Candidate(created.Candidate.Handle); candidate.Head != created.Revision.Handle {
		t.Fatal("invalid flex factor advanced candidate")
	}
}

func TestPaperSetGridTrackIdempotentRace(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	request, _, _ := gridRequest(t, workspace, "grid-race")
	const workers = 8
	results := make(chan PaperMutationResult, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := workspace.PaperSetGridTrack(request)
			results <- result
			errors <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent mutation error = %v", err)
		}
	}
	var revision paperedit.Revision
	for result := range results {
		if revision == "" {
			revision = result.Revision.Revision
		} else if result.Revision.Revision != revision {
			t.Fatalf("concurrent result revision = %q, want %q", result.Revision.Revision, revision)
		}
	}
}
