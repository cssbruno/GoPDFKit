// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSemanticPlanIsCanonicalImmutableAndStoreRoundTrips(t *testing.T) {
	plan := semanticTestPlan(t, false)
	projection := plan.Projection()
	if projection.SchemaVersion != LayoutPlanSchemaVersion || len(projection.SemanticNodes) != 2 ||
		len(projection.SemanticFragments) != 2 || len(projection.ReadingOrder) != 2 {
		t.Fatalf("semantic projection = %#v", projection)
	}
	encoded, err := plan.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded LayoutPlanProjection
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"attributes"`) {
		t.Fatalf("empty semantic attributes were not omitted: %s", encoded)
	}
	if !reflect.DeepEqual(decoded.SemanticNodes, projection.SemanticNodes) ||
		!reflect.DeepEqual(decoded.SemanticFragments, projection.SemanticFragments) ||
		!reflect.DeepEqual(decoded.ReadingOrder, projection.ReadingOrder) {
		t.Fatalf("semantic canonical round trip changed data: %#v", decoded)
	}
	wantHash, err := plan.Hash()
	if err != nil {
		t.Fatal(err)
	}
	projection.SemanticNodes[1].Role = SemanticRoleArtifact
	projection.SemanticFragments[0].Semantic = 1
	projection.ReadingOrder[0].ReadingIndex = 99
	if got, _ := plan.Hash(); got != wantHash {
		t.Fatalf("projection mutation changed plan hash: %s != %s", got, wantHash)
	}

	store, err := NewMemoryPlanStore(DefaultPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.Put(plan)
	if err != nil || hash != wantHash {
		t.Fatalf("Put() = %s, %v; want %s", hash, err, wantHash)
	}
	loaded, err := store.Get(hash)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := loaded.Hash(); got != wantHash || !reflect.DeepEqual(loaded.Projection().SemanticNodes, plan.Projection().SemanticNodes) {
		t.Fatalf("semantic store round trip = %s, %#v", got, loaded.Projection())
	}

	segmented, err := NewMemorySegmentedPlanStore(DefaultSegmentedPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	segmentedHash, err := segmented.Put(context.Background(), plan)
	if err != nil || segmentedHash != wantHash {
		t.Fatalf("segmented Put() = %s, %v; want %s", segmentedHash, err, wantHash)
	}
	segmentedPlan, err := segmented.Get(context.Background(), segmentedHash)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := segmentedPlan.Hash(); got != wantHash ||
		!reflect.DeepEqual(segmentedPlan.Projection().ReadingOrder, plan.Projection().ReadingOrder) {
		t.Fatalf("segmented semantic round trip = %s, %#v", got, segmentedPlan.Projection())
	}
}

func TestReplaceSemanticsPreservesDisplayAndDoesNotMutateOriginal(t *testing.T) {
	original := semanticTestPlan(t, false)
	before := original.Projection()
	nodes := before.SemanticNodes
	nodes[0].Key, nodes[0].Instance = "@report", "@report"
	replaced, err := ReplaceSemantics(original, nodes, before.SemanticFragments, before.ReadingOrder)
	if err != nil {
		t.Fatal(err)
	}
	after := replaced.Projection()
	if after.SemanticNodes[0].Key != "@report" || original.Projection().SemanticNodes[0].Key != "@document" {
		t.Fatalf("semantic replacement aliased original: %#v / %#v", after.SemanticNodes, original.Projection().SemanticNodes)
	}
	before.SemanticNodes, after.SemanticNodes = nil, nil
	before.SemanticFragments, after.SemanticFragments = nil, nil
	before.ReadingOrder, after.ReadingOrder = nil, nil
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("semantic replacement changed non-semantic plan\nbefore=%#v\nafter=%#v", before, after)
	}
	invalid := replaced.Projection()
	invalid.SemanticNodes[0].Role = SemanticRoleSection
	if changed, replaceErr := ReplaceSemantics(replaced, invalid.SemanticNodes, invalid.SemanticFragments, invalid.ReadingOrder); replaceErr == nil || len(changed.Projection().Pages) != 0 {
		t.Fatalf("invalid semantic replacement = %#v, %v", changed.Projection(), replaceErr)
	}
}

func TestSemanticValidationRejectsInvalidTreeOwnershipAndReadingOrder(t *testing.T) {
	valid := semanticTestInput(false)
	tests := []struct {
		name   string
		mutate func(*LayoutPlanInput)
	}{
		{"root role", func(input *LayoutPlanInput) { input.SemanticNodes[0].Role = SemanticRoleSection }},
		{"missing root", func(input *LayoutPlanInput) { input.SemanticNodes[0].Parent = 2 }},
		{"cycle", func(input *LayoutPlanInput) {
			input.SemanticNodes[1].Parent = 3
			input.SemanticNodes = append(input.SemanticNodes, SemanticNode{ID: 3, Parent: 2, Role: SemanticRoleSection, Key: "@cycle", Instance: "@cycle"})
		}},
		{"duplicate ownership", func(input *LayoutPlanInput) {
			input.SemanticFragments = append(input.SemanticFragments, input.SemanticFragments[0])
		}},
		{"missing ownership", func(input *LayoutPlanInput) { input.SemanticFragments = input.SemanticFragments[:1] }},
		{"cross page ownership", func(input *LayoutPlanInput) { input.SemanticFragments[0].Page = 2 }},
		{"ownership provenance", func(input *LayoutPlanInput) { input.SemanticFragments[0].Semantic = 1 }},
		{"cross page reading", func(input *LayoutPlanInput) { input.ReadingOrder[0].Page = 2 }},
		{"wrong reading owner", func(input *LayoutPlanInput) { input.ReadingOrder[0].Semantic = 1 }},
		{"duplicate reading", func(input *LayoutPlanInput) { input.ReadingOrder[1].Fragment = 1 }},
		{"missing reading", func(input *LayoutPlanInput) { input.ReadingOrder = input.ReadingOrder[:1] }},
		{"noncanonical page order", func(input *LayoutPlanInput) {
			input.ReadingOrder[0], input.ReadingOrder[1] = input.ReadingOrder[1], input.ReadingOrder[0]
		}},
		{"nonconsecutive page index", func(input *LayoutPlanInput) { input.ReadingOrder[0].ReadingIndex = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			input.SemanticNodes = cloneSlice(valid.SemanticNodes)
			input.SemanticFragments = cloneSlice(valid.SemanticFragments)
			input.ReadingOrder = cloneSlice(valid.ReadingOrder)
			test.mutate(&input)
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid semantic plan unexpectedly validated")
			}
		})
	}
}

func TestSemanticArtifactsAreOwnedButNeverRead(t *testing.T) {
	plan := semanticTestPlan(t, true)
	projection := plan.Projection()
	if projection.SemanticNodes[1].Role != SemanticRoleArtifact || len(projection.SemanticFragments) != 2 || len(projection.ReadingOrder) != 0 {
		t.Fatalf("artifact semantic contract = %#v", projection)
	}
	input := semanticTestInput(true)
	input.ReadingOrder = []ReadingOccurrence{{Semantic: 2, Page: 1, Fragment: 1}}
	if _, err := NewLayoutPlan(input); err == nil || !strings.Contains(err.Error(), "artifact fragments") {
		t.Fatalf("artifact reading occurrence = %v", err)
	}
}

func TestSemanticStateHasHardByteLimitAndLegacyPlansRemainValid(t *testing.T) {
	if _, err := NewLayoutPlan(testPlanInput()); err != nil {
		t.Fatalf("legacy semantic-free plan = %v", err)
	}
	nodes := []SemanticNode{{ID: 1, Role: SemanticRoleDocument, Key: "@document", Instance: "@document"}}
	for index := 2; index < 300; index++ {
		identity := NodeKey(fmt.Sprintf("@%04d-%s", index, strings.Repeat("x", 3900)))
		nodes = append(nodes, SemanticNode{ID: SemanticNodeID(index), Parent: 1, Role: SemanticRoleSection, Key: identity, Instance: InstanceID(identity)})
	}
	if _, err := NewLayoutPlan(LayoutPlanInput{SemanticNodes: nodes}); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversized semantic state = %v", err)
	}
}

func TestSemanticAccessibilityAttributesAreCanonicalStoredAndQueryable(t *testing.T) {
	plan := semanticAccessibilityTestPlan(t)
	projection := plan.Projection()
	root := projection.SemanticNodes[0]
	link := projection.SemanticNodes[1]
	if root.Attributes.Language != "en-US" || link.Attributes.ActualText != "Open target" ||
		link.Attributes.LinkDestination != 1 {
		t.Fatalf("semantic accessibility projection = %#v", projection.SemanticNodes)
	}
	wantHash, _ := plan.Hash()
	changedInput := planInputFromProjection(plan.Projection())
	changedInput.SemanticNodes[1].Attributes.ActualText = "Open a different target"
	changed, err := NewLayoutPlan(changedInput)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash, _ := changed.Hash(); changedHash == wantHash {
		t.Fatal("semantic accessibility attribute did not affect the canonical hash")
	}
	projection.SemanticNodes[1].Attributes.ActualText = "mutated"
	if got, _ := plan.Hash(); got != wantHash {
		t.Fatalf("attribute projection mutation changed hash: %s != %s", got, wantHash)
	}

	result, err := plan.QueryStructure(StructuralQuery{
		Role: SemanticRoleLink, Language: "pt-BR", LinkDestination: 1, MaxResults: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Semantics.Matches != 1 || result.Summary.Fragments.Matches != 1 ||
		result.Summary.ReadingOrder.Matches != 1 || result.Semantics[0].Node.Attributes.ActualText != "Open target" {
		t.Fatalf("accessibility query = %#v", result)
	}
	if _, err := plan.QueryStructure(StructuralQuery{Language: "pt-br", MaxResults: 8}); err == nil {
		t.Fatal("noncanonical query language unexpectedly validated")
	}

	store, _ := NewMemoryPlanStore(DefaultPlanStoreLimits())
	hash, err := store.Put(plan)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(hash)
	if err != nil || !reflect.DeepEqual(loaded.Projection().SemanticNodes, plan.Projection().SemanticNodes) {
		t.Fatalf("attribute monolithic round trip = %#v, %v", loaded.Projection().SemanticNodes, err)
	}
	segmented, _ := NewMemorySegmentedPlanStore(DefaultSegmentedPlanStoreLimits())
	hash, err = segmented.Put(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = segmented.Get(context.Background(), hash)
	if err != nil || !reflect.DeepEqual(loaded.Projection().SemanticNodes, plan.Projection().SemanticNodes) {
		t.Fatalf("attribute segmented round trip = %#v, %v", loaded.Projection().SemanticNodes, err)
	}
}

func TestSemanticAccessibilityAttributesEnforceRoleAndValueContracts(t *testing.T) {
	valid := semanticTestInput(false)
	tests := []struct {
		name       string
		attributes SemanticAttributes
		role       SemanticRole
	}{
		{"noncanonical language", SemanticAttributes{Language: "en-us"}, SemanticRoleParagraph},
		{"artifact language", SemanticAttributes{Language: "en-US"}, SemanticRoleArtifact},
		{"alternate text role", SemanticAttributes{AlternateText: "A chart"}, SemanticRoleParagraph},
		{"actual text role", SemanticAttributes{ActualText: "replacement"}, SemanticRoleSection},
		{"actual text control", SemanticAttributes{ActualText: "bad\ntext"}, SemanticRoleParagraph},
		{"actual text size", SemanticAttributes{ActualText: strings.Repeat("x", int(SemanticMaxTextBytes)+1)}, SemanticRoleParagraph},
		{"heading level role", SemanticAttributes{HeadingLevel: 2}, SemanticRoleParagraph},
		{"heading level range", SemanticAttributes{HeadingLevel: 7}, SemanticRoleHeading},
		{"link destination role", SemanticAttributes{LinkDestination: 1}, SemanticRoleParagraph},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			input.SemanticNodes = cloneSlice(valid.SemanticNodes)
			input.SemanticNodes[1].Role = test.role
			input.SemanticNodes[1].Attributes = test.attributes
			if test.role == SemanticRoleArtifact {
				input.ReadingOrder = nil
			}
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid semantic accessibility attributes unexpectedly validated")
			}
		})
	}

	for _, validCase := range []struct {
		role       SemanticRole
		attributes SemanticAttributes
	}{
		{SemanticRoleFigure, SemanticAttributes{Language: "pt-BR", AlternateText: "Revenue chart", ActualText: "Revenue increased"}},
		{SemanticRoleHeading, SemanticAttributes{Language: "en-US", ActualText: "Overview", HeadingLevel: 2}},
	} {
		input := semanticTestInput(false)
		input.SemanticNodes[1].Role = validCase.role
		input.SemanticNodes[1].Attributes = validCase.attributes
		if _, err := NewLayoutPlan(input); err != nil {
			t.Fatalf("valid %s attributes = %v", validCase.role, err)
		}
	}
}

func TestSemanticLinkDestinationMustMatchAnOwnedPlannedLink(t *testing.T) {
	plan := semanticAccessibilityTestPlan(t)
	input := planInputFromProjection(plan.Projection())
	input.SemanticNodes[1].Attributes.LinkDestination = 2
	if _, err := NewLayoutPlan(input); err == nil || !strings.Contains(err.Error(), "owned planned link") {
		t.Fatalf("unmatched semantic link destination = %v", err)
	}
	input = planInputFromProjection(plan.Projection())
	input.SemanticNodes[1].Attributes.LinkDestination = 3
	if _, err := NewLayoutPlan(input); err == nil || !strings.Contains(err.Error(), "existing destination") {
		t.Fatalf("missing semantic link destination = %v", err)
	}
}

func TestSemanticAccessibilityAttributesConsumeHardStateBudget(t *testing.T) {
	nodes := []SemanticNode{{ID: 1, Role: SemanticRoleDocument, Key: "@document", Instance: "@document"}}
	text := strings.Repeat("x", int(SemanticMaxTextBytes))
	for index := 2; index < 70; index++ {
		identity := NodeKey(fmt.Sprintf("@paragraph-%d", index))
		nodes = append(nodes, SemanticNode{ID: SemanticNodeID(index), Parent: 1, Role: SemanticRoleParagraph,
			Key: identity, Instance: InstanceID(identity), Attributes: SemanticAttributes{ActualText: text}})
	}
	if _, err := NewLayoutPlan(LayoutPlanInput{SemanticNodes: nodes}); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversized semantic attribute state = %v", err)
	}
}

func semanticAccessibilityTestPlan(t *testing.T) LayoutPlan {
	t.Helper()
	geometry, destinations, links := linkTestInputs(t)
	plan, err := AttachLinks(geometry, destinations, links)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	nodes := []SemanticNode{
		{ID: 1, Role: SemanticRoleDocument, Key: "@document", Instance: "@document", Attributes: SemanticAttributes{Language: "en-US"}},
		{ID: 2, Parent: 1, Role: SemanticRoleLink, Key: projection.Fragments[0].Key, Instance: projection.Fragments[0].Instance,
			Source: projection.Fragments[0].Source, Attributes: SemanticAttributes{Language: "pt-BR", ActualText: "Open target", LinkDestination: 1}},
		{ID: 3, Parent: 1, Role: SemanticRoleParagraph, Key: projection.Fragments[1].Key, Instance: projection.Fragments[1].Instance,
			Source: projection.Fragments[1].Source, Attributes: SemanticAttributes{ActualText: "Target"}},
	}
	associations := []SemanticFragmentAssociation{{Semantic: 2, Page: 1, Fragment: 1}, {Semantic: 3, Page: 2, Fragment: 2}}
	reading := []ReadingOccurrence{{Semantic: 2, Page: 1, Fragment: 1}, {Semantic: 3, Page: 2, Fragment: 2}}
	plan, err = AttachSemantics(plan, nodes, associations, reading)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func planInputFromProjection(projection LayoutPlanProjection) LayoutPlanInput {
	return LayoutPlanInput{Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns, ImageResources: projection.ImageResources,
		Images: projection.Images, Destinations: projection.Destinations, Links: projection.Links,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments, ReadingOrder: projection.ReadingOrder}
}

func semanticTestPlan(t *testing.T, artifact bool) LayoutPlan {
	t.Helper()
	plan, err := NewLayoutPlan(semanticTestInput(artifact))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func semanticTestInput(artifact bool) LayoutPlanInput {
	input := testPlanInput()
	role := SemanticRoleParagraph
	if artifact {
		role = SemanticRoleArtifact
	}
	input.SemanticNodes = []SemanticNode{
		{ID: 1, Role: SemanticRoleDocument, Key: "@document", Instance: "@document"},
		{ID: 2, Parent: 1, Role: role, Key: "@lines", Instance: "@lines"},
	}
	input.SemanticFragments = []SemanticFragmentAssociation{
		{Semantic: 2, Page: 1, Fragment: 1},
		{Semantic: 2, Page: 2, Fragment: 2},
	}
	if !artifact {
		input.ReadingOrder = []ReadingOccurrence{
			{Semantic: 2, Page: 1, Fragment: 1, ReadingIndex: 0},
			{Semantic: 2, Page: 2, Fragment: 2, ReadingIndex: 0},
		}
	}
	return input
}
