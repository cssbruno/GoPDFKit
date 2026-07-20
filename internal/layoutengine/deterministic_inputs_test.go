// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"strings"
	"testing"
)

func TestResourceCatalogIsContentAddressedCanonicalAndDetached(t *testing.T) {
	input := []ContentAddressedResource{
		{Kind: "image-png", Name: "hero", Digest: strings.Repeat("2", 64)},
		{Kind: "core-font-metrics", Name: "helvetica", Digest: strings.Repeat("1", 64)},
	}
	first, err := NewResourceCatalogManifest(input)
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := NewResourceCatalogManifest([]ContentAddressedResource{input[1], input[0]})
	if err != nil || reordered.ID != first.ID {
		t.Fatalf("reordered catalog = %+v, %v; want ID %s", reordered, err, first.ID)
	}
	input[0].Digest = strings.Repeat("3", 64)
	if first.Resources[1].Digest != strings.Repeat("2", 64) {
		t.Fatal("catalog retained caller storage")
	}
	changed, _ := NewResourceCatalogManifest([]ContentAddressedResource{
		{Kind: "image-png", Name: "hero", Digest: strings.Repeat("3", 64)},
		{Kind: "core-font-metrics", Name: "helvetica", Digest: strings.Repeat("1", 64)},
	})
	if changed.ID == first.ID {
		t.Fatal("resource content change did not change catalog identity")
	}
	if _, err := NewResourceCatalogManifest([]ContentAddressedResource{input[1], input[1]}); err == nil {
		t.Fatal("duplicate logical resource identity unexpectedly accepted")
	}
}

func TestDeterministicInputsParticipateInCanonicalPlanIdentity(t *testing.T) {
	base, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	manifest := testDeterministicManifest(t)
	bound, err := base.WithDeterministicInputs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	baseHash, _ := base.Hash()
	boundHash, _ := bound.Hash()
	if baseHash == boundHash || bound.Projection().DeterministicInputs == nil {
		t.Fatal("input manifest did not participate in canonical plan hash")
	}
	detached, ok := bound.DeterministicInputs()
	if !ok || detached.PlanID != manifest.PlanID {
		t.Fatalf("DeterministicInputs() = %+v, %v", detached, ok)
	}
	detached.CompatibilityFlags[0] = "mutated"
	again, _ := bound.DeterministicInputs()
	if again.CompatibilityFlags[0] == "mutated" {
		t.Fatal("deterministic input projection aliases plan storage")
	}

	variants := []func(*DeterministicInputManifest){
		func(value *DeterministicInputManifest) { value.Locale = "pt-BR" },
		func(value *DeterministicInputManifest) { value.Timezone = "America/Fortaleza" },
		func(value *DeterministicInputManifest) { value.TextData.Unicode = "future" },
		func(value *DeterministicInputManifest) { value.TextData.CLDR = "46" },
		func(value *DeterministicInputManifest) { value.TextData.Hyphenation = "en-2026" },
	}
	for index, mutate := range variants {
		changed := manifest
		changed.ResourceCatalog = cloneResourceCatalog(manifest.ResourceCatalog)
		changed.CompatibilityFlags = append([]string(nil), manifest.CompatibilityFlags...)
		mutate(&changed)
		changed = rebuildManifest(t, changed)
		changedPlan, bindErr := base.WithDeterministicInputs(changed)
		if bindErr != nil {
			t.Fatalf("variant %d: %v", index, bindErr)
		}
		changedHash, _ := changedPlan.Hash()
		if changed.PlanID == manifest.PlanID || changedHash == boundHash {
			t.Fatalf("variant %d did not change PlanID and plan hash", index)
		}
	}
}

func TestPageProfileIdentityPinsExactPhysicalSize(t *testing.T) {
	first, err := NewPageProfileManifest("a4", Fixed(595*FixedScale), Fixed(842*FixedScale))
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewPageProfileManifest("a4", Fixed(596*FixedScale), Fixed(842*FixedScale))
	alias, _ := NewPageProfileManifest("iso-a4", first.Width, first.Height)
	if first.ID == second.ID || first.ID == alias.ID {
		t.Fatal("page profile identity did not cover dimensions and canonical name")
	}
}

func TestBuiltinTextVersionsAreExplicitAndDoNotClaimUnsupportedData(t *testing.T) {
	versions := BuiltinTextDataVersions()
	if versions.Unicode == "" || versions.CLDR != "none" || versions.Hyphenation != "none" {
		t.Fatalf("BuiltinTextDataVersions() = %+v", versions)
	}
}

func TestDeterministicManifestRejectsResourcePageAndPlannerSubstitution(t *testing.T) {
	base, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	exact := testDeterministicManifest(t)
	if _, err := base.WithDeterministicInputs(exact); err != nil {
		t.Fatal(err)
	}

	ids := testPlanIdentityInputs(t)
	wrongResources, _ := NewResourceCatalogManifest([]ContentAddressedResource{{Kind: "core-font-metrics", Name: "courier", Digest: strings.Repeat("4", 64)}})
	wrongCatalog, _ := NewDeterministicInputManifest(ids.Template, ids.Scenario, wrongResources, exact.Locale, exact.Timezone, exact.TextData,
		exact.CompatibilityProfile, exact.CompatibilityFlags, exact.PageProfile, PlannerVersion)
	if _, err := base.WithDeterministicInputs(wrongCatalog); err == nil || !strings.Contains(err.Error(), "exact plan resources") {
		t.Fatalf("resource substitution = %v", err)
	}

	wrongPage, _ := NewPageProfileManifest("a4", Fixed(595*FixedScale), Fixed(842*FixedScale))
	wrongDimensions, _ := NewDeterministicInputManifest(ids.Template, ids.Scenario, exact.ResourceCatalog, exact.Locale, exact.Timezone, exact.TextData,
		exact.CompatibilityProfile, exact.CompatibilityFlags, wrongPage, PlannerVersion)
	if _, err := base.WithDeterministicInputs(wrongDimensions); err == nil || !strings.Contains(err.Error(), "page profile") {
		t.Fatalf("page substitution = %v", err)
	}

	wrongPlanner := exact
	wrongPlanner.PlannerVersion = "layoutengine/future"
	wrongPlanner = rebuildManifestWithPlanner(t, wrongPlanner)
	if _, err := base.WithDeterministicInputs(wrongPlanner); err == nil || !strings.Contains(err.Error(), "planner version") {
		t.Fatalf("planner substitution = %v", err)
	}
}

func TestResourceAttachmentRebindsExactDeterministicCatalog(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Pages[0].Commands = IndexRange{}
	fonts, runs := input.Fonts, input.GlyphRuns
	input.Fonts, input.GlyphRuns, input.Commands = nil, nil, nil
	geometry, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	manifest := deterministicManifestForPlan(t, geometry)
	geometry, err = geometry.WithDeterministicInputs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := AttachCoreGlyphRuns(geometry, fonts, runs)
	if err != nil {
		t.Fatal(err)
	}
	rebound, ok := planned.DeterministicInputs()
	if !ok || rebound.PlanID == manifest.PlanID || len(rebound.ResourceCatalog.Resources) != 1 || rebound.ResourceCatalog.Resources[0].Digest != string(fonts[0].MetricsDigest) {
		t.Fatalf("rebound manifest = %#v, %v", rebound, ok)
	}
	if err := planned.Validate(); err != nil {
		t.Fatal(err)
	}
}

func testDeterministicManifest(t *testing.T) DeterministicInputManifest {
	t.Helper()
	ids := testPlanIdentityInputs(t)
	resources, err := NewResourceCatalogManifest(nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := NewPageProfileManifest("letter", Fixed(612*FixedScale), Fixed(792*FixedScale))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewDeterministicInputManifest(ids.Template, ids.Scenario, resources, "en-US", "UTC",
		BuiltinTextDataVersions(), "paper-0.1", []string{"strict-breaks", "ascii-fast-path"}, page, PlannerVersion)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func rebuildManifest(t *testing.T, input DeterministicInputManifest) DeterministicInputManifest {
	t.Helper()
	template, _ := ParseSemanticTemplateID(input.SemanticTemplateID)
	scenario, _ := ParseScenarioRevisionID(input.ScenarioRevisionID)
	result, err := NewDeterministicInputManifest(template, scenario, input.ResourceCatalog, input.Locale, input.Timezone,
		input.TextData, input.CompatibilityProfile, input.CompatibilityFlags, input.PageProfile, input.PlannerVersion)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func rebuildManifestWithPlanner(t *testing.T, input DeterministicInputManifest) DeterministicInputManifest {
	t.Helper()
	template, _ := ParseSemanticTemplateID(input.SemanticTemplateID)
	scenario, _ := ParseScenarioRevisionID(input.ScenarioRevisionID)
	result, err := NewDeterministicInputManifest(template, scenario, input.ResourceCatalog, input.Locale, input.Timezone,
		input.TextData, input.CompatibilityProfile, input.CompatibilityFlags, input.PageProfile, input.PlannerVersion)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func deterministicManifestForPlan(t *testing.T, plan LayoutPlan) DeterministicInputManifest {
	t.Helper()
	ids := testPlanIdentityInputs(t)
	resources, err := ResourceCatalogFromPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	page, err := NewPageProfileManifest("fixture", projection.Pages[0].Size.Width, projection.Pages[0].Size.Height)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewDeterministicInputManifest(ids.Template, ids.Scenario, resources, "en-US", "UTC", BuiltinTextDataVersions(),
		"test/1", []string{"strict"}, page, PlannerVersion)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
