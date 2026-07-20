// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func canonicalTreeFixture() CanonicalTreeInput {
	auto := TreeLength{Kind: TreeLengthAuto}
	style := &TreeStyleInput{FontFamily: "Inter", Align: "start", FontSize: Fixed(12 * FixedScale), LineHeight: Fixed(15 * FixedScale),
		Margin: [4]TreeLength{auto, auto, auto, auto}}
	track := &TreeTrackInput{Name: "main", Min: auto, Max: TreeLength{Kind: TreeLengthFraction, Value: Fixed(FixedScale)}}
	resource := &TreeResourceInput{Kind: "font", Key: "inter-regular", Digest: "sha256:abc"}
	semantic := &TreeSemanticInput{Role: SemanticRoleParagraph, Label: "body-copy"}
	return CanonicalTreeInput{Nodes: []TreeNodeInput{
		{ID: 41, Key: "@root", Kind: "document", Parent: -1},
		{ID: 77, Key: "@first", Kind: "paragraph", Parent: 0, Text: "same", Style: style, Track: track, Resource: resource, Semantic: semantic},
		{ID: 92, Key: "@second", Kind: "paragraph", Parent: 0, Text: "same", Style: style, Track: track, Resource: resource, Semantic: semantic},
	}}
}

func TestCanonicalTreeDenseArenaInterningHashAndRoundTrip(t *testing.T) {
	tree, err := NewCanonicalTree(context.Background(), canonicalTreeFixture(), CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	p := tree.Projection()
	if len(p.Nodes) != 3 || !reflect.DeepEqual(p.Children, []TreeNodeIndex{1, 2}) || p.Nodes[0].ChildStart != 0 || p.Nodes[0].ChildCount != 2 {
		t.Fatalf("dense arena = %+v / %v", p.Nodes, p.Children)
	}
	if p.Nodes[1].ID != 77 || p.Nodes[1].Key != "@first" || p.Nodes[1].Style != p.Nodes[2].Style ||
		p.Nodes[1].Track != p.Nodes[2].Track || p.Nodes[1].Resource != p.Nodes[2].Resource || p.Nodes[1].Semantic != p.Nodes[2].Semantic || p.Nodes[1].Text != p.Nodes[2].Text {
		t.Fatalf("identity/intern refs = %+v / %+v", p.Nodes[1], p.Nodes[2])
	}
	if len(p.Styles) != 1 || len(p.Tracks) != 1 || len(p.Resources) != 1 || len(p.Semantics) != 1 {
		t.Fatalf("intern tables = %+v", p)
	}
	encoded, err := tree.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeCanonicalTree(context.Background(), encoded, CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := restored.CanonicalJSON()
	if !bytes.Equal(encoded, again) {
		t.Fatalf("round trip:\n%s\n%s", encoded, again)
	}
	h1, _ := tree.Hash()
	h2, _ := restored.Hash()
	if h1 == "" || h1 != h2 {
		t.Fatalf("hashes = %q/%q", h1, h2)
	}
	p.Nodes[0].Key = "@mutated"
	p.Strings[0] = "mutated"
	if current := tree.Projection(); current.Nodes[0].Key != "@root" || current.Strings[0] == "mutated" {
		t.Fatal("projection aliases arena")
	}
}

func TestCanonicalTreeSemanticHashExcludesOnlySourceLocations(t *testing.T) {
	input := canonicalTreeFixture()
	input.Nodes[1].Source = SourceSpan{File: "first.paper", Start: SourcePosition{Line: 2, Column: 1}, End: SourcePosition{Line: 2, Column: 10}}
	first, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	input.Nodes[1].Source = SourceSpan{File: "formatted.paper", Start: SourcePosition{Line: 20, Column: 4}, End: SourcePosition{Line: 20, Column: 13}}
	formatted, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	firstSemantic, _ := first.SemanticHash()
	formattedSemantic, _ := formatted.SemanticHash()
	firstExact, _ := first.Hash()
	formattedExact, _ := formatted.Hash()
	if firstSemantic == "" || firstSemantic != formattedSemantic || firstExact == formattedExact {
		t.Fatalf("semantic/exact hashes = %q/%q, %q/%q", firstSemantic, formattedSemantic, firstExact, formattedExact)
	}
	input.Nodes[1].Text = "changed"
	changed, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	changedSemantic, _ := changed.SemanticHash()
	if changedSemantic == firstSemantic {
		t.Fatal("semantic content change did not change semantic template identity")
	}
}

func TestCanonicalTreeRejectsCollisionsLimitsOverflowAndCancellation(t *testing.T) {
	input := canonicalTreeFixture()
	input.Nodes[2].ID = input.Nodes[1].ID
	if _, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{}); !errors.Is(err, ErrCanonicalTreeCollision) {
		t.Fatalf("ID collision = %v", err)
	}
	input = canonicalTreeFixture()
	input.Nodes[2].Key = input.Nodes[1].Key
	if _, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{}); !errors.Is(err, ErrCanonicalTreeCollision) {
		t.Fatalf("key collision = %v", err)
	}
	input = canonicalTreeFixture()
	changed := *input.Nodes[2].Resource
	changed.Digest = "sha256:def"
	input.Nodes[2].Resource = &changed
	if _, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{}); !errors.Is(err, ErrCanonicalTreeCollision) {
		t.Fatalf("resource collision = %v", err)
	}
	limits := DefaultCanonicalTreeLimits()
	limits.MaxNodes = 2
	if _, err := NewCanonicalTree(context.Background(), canonicalTreeFixture(), limits); !errors.Is(err, ErrCanonicalTreeLimit) {
		t.Fatalf("node limit = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewCanonicalTree(canceled, canonicalTreeFixture(), CanonicalTreeLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	input = canonicalTreeFixture()
	input.Nodes[1].Parent = int64(^uint32(0)) + 1
	if _, err := NewCanonicalTree(context.Background(), input, CanonicalTreeLimits{}); !errors.Is(err, ErrCanonicalTreeInvalid) {
		t.Fatalf("parent overflow = %v", err)
	}
}

func TestCanonicalTreeConcurrentReadersAreStable(t *testing.T) {
	tree, err := NewCanonicalTree(context.Background(), canonicalTreeFixture(), CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := tree.Hash()
	var wg sync.WaitGroup
	failures := make(chan string, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := tree.Hash()
			if err != nil || got != want {
				failures <- got
			}
			p := tree.Projection()
			p.Nodes[0].Key = "@local"
		}()
	}
	wg.Wait()
	close(failures)
	for got := range failures {
		t.Fatalf("concurrent hash = %q, want %q", got, want)
	}
}
