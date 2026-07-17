// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"strings"
	"testing"
)

func TestDerivePlanIDPinsCanonicalInputs(t *testing.T) {
	input := testPlanIdentityInputs(t)
	first, err := DerivePlanID(input)
	if err != nil || !first.Valid() || len(first.String()) != 64 {
		t.Fatalf("DerivePlanID() = %q, %v", first.String(), err)
	}
	reordered := input
	reordered.CompatibilityFlags = []string{"strict-breaks", "ascii-fast-path"}
	second, err := DerivePlanID(reordered)
	if err != nil || second != first {
		t.Fatalf("flag order changed PlanID: %q != %q (%v)", second.String(), first.String(), err)
	}
	changed := input
	changed.Timezone = "America/Fortaleza"
	third, err := DerivePlanID(changed)
	if err != nil || third == first {
		t.Fatalf("timezone did not change PlanID: %q (%v)", third.String(), err)
	}
	parsed, err := ParsePlanID(first.String())
	if err != nil || parsed != first {
		t.Fatalf("ParsePlanID() = %q, %v", parsed.String(), err)
	}
}

func TestDerivePlanIDRejectsAbsentAmbiguousInputs(t *testing.T) {
	input := testPlanIdentityInputs(t)
	input.Template = SemanticTemplateID{}
	if _, err := DerivePlanID(input); !errors.Is(err, ErrPlanIdentityInputs) {
		t.Fatalf("absent template error = %v", err)
	}
	input = testPlanIdentityInputs(t)
	input.Locale = " en-US"
	if _, err := DerivePlanID(input); !errors.Is(err, ErrPlanIdentityInputs) {
		t.Fatalf("noncanonical locale error = %v", err)
	}
	input = testPlanIdentityInputs(t)
	input.CompatibilityFlags = []string{"strict", "strict"}
	if _, err := DerivePlanID(input); !errors.Is(err, ErrPlanIdentityInputs) {
		t.Fatalf("duplicate flag error = %v", err)
	}
	input = testPlanIdentityInputs(t)
	input.CompatibilityFlags = make([]string, MaxPlanIdentityCompatibilityFlags+1)
	if _, err := DerivePlanID(input); !errors.Is(err, ErrPlanIdentityInputs) {
		t.Fatalf("flag count error = %v", err)
	}
}

func TestDerivePlanIDRequiresCanonicalExplicitLocaleAndTimezone(t *testing.T) {
	for _, locale := range []string{"und", "en", "pt-BR", "zh-Hant-TW", "en-US-u-nu-latn"} {
		input := testPlanIdentityInputs(t)
		input.Locale = locale
		if _, err := DerivePlanID(input); err != nil {
			t.Fatalf("canonical locale %q: %v", locale, err)
		}
	}
	for _, timezone := range []string{"UTC", "UTC+03:00", "America/Fortaleza", "Etc/GMT+3"} {
		input := testPlanIdentityInputs(t)
		input.Timezone = timezone
		if _, err := DerivePlanID(input); err != nil {
			t.Fatalf("canonical timezone %q: %v", timezone, err)
		}
	}
	for _, invalid := range []struct{ locale, timezone string }{
		{locale: "", timezone: "UTC"}, {locale: "en-us", timezone: "UTC"}, {locale: "EN-US", timezone: "UTC"},
		{locale: "en-US", timezone: "Local"}, {locale: "en-US", timezone: "UTC-00:00"}, {locale: "en-US", timezone: "../zone"},
	} {
		input := testPlanIdentityInputs(t)
		input.Locale, input.Timezone = invalid.locale, invalid.timezone
		if _, err := DerivePlanID(input); !errors.Is(err, ErrPlanIdentityInputs) {
			t.Fatalf("invalid locale/timezone %#v = %v", invalid, err)
		}
	}
}

func TestPlanIdentityEveryDeclaredInputIsCausalAndRepeatedInputsAreStable(t *testing.T) {
	base := testPlanIdentityInputs(t)
	want, err := DerivePlanID(base)
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 32; iteration++ {
		got, err := DerivePlanID(base)
		if err != nil || got != want {
			t.Fatalf("identical iteration %d = %s, %v; want %s", iteration, got.String(), err, want.String())
		}
	}
	template, _ := ParseSemanticTemplateID(strings.Repeat("4", 64))
	scenario, _ := ParseScenarioRevisionID(strings.Repeat("5", 64))
	resources, _ := ParseResourceCatalogID(strings.Repeat("6", 64))
	mutations := []func(*PlanIdentityInputs){
		func(value *PlanIdentityInputs) { value.Template = template },
		func(value *PlanIdentityInputs) { value.Scenario = scenario },
		func(value *PlanIdentityInputs) { value.Resources = resources },
		func(value *PlanIdentityInputs) { value.Locale = "pt-BR" },
		func(value *PlanIdentityInputs) { value.Timezone = "UTC+03:00" },
		func(value *PlanIdentityInputs) { value.UnicodeVersion = "16.0" },
		func(value *PlanIdentityInputs) { value.CLDRVersion = "46" },
		func(value *PlanIdentityInputs) { value.HyphenationVersion = "en-2026" },
		func(value *PlanIdentityInputs) { value.CompatibilityProfile = "paper-0.2" },
		func(value *PlanIdentityInputs) { value.PageProfile = "letter" },
		func(value *PlanIdentityInputs) { value.PlannerVersion = "layoutengine-2" },
		func(value *PlanIdentityInputs) {
			value.CompatibilityFlags = append(value.CompatibilityFlags, "new-rule")
		},
	}
	for index, mutate := range mutations {
		changed := base
		changed.CompatibilityFlags = append([]string(nil), base.CompatibilityFlags...)
		mutate(&changed)
		got, err := DerivePlanID(changed)
		if err != nil || got == want {
			t.Fatalf("input mutation %d = %s, %v; base %s", index, got.String(), err, want.String())
		}
	}
}

func TestDeriveRenderIDPinsRendererAndDisclosureInputs(t *testing.T) {
	plan, err := DerivePlanID(testPlanIdentityInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	input := RenderIdentityInputs{Plan: plan, RendererVersion: "pdf-1",
		ColorProfile: "sRGB", DPI: 144, CropProfile: "full-page",
		DisclosureDomain: "fixture"}
	first, err := DeriveRenderID(input)
	if err != nil || !first.Valid() {
		t.Fatalf("DeriveRenderID() = %q, %v", first.String(), err)
	}
	input.DisclosureDomain = "production"
	second, err := DeriveRenderID(input)
	if err != nil || second == first {
		t.Fatalf("disclosure did not change RenderID: %q (%v)", second.String(), err)
	}
	parsed, err := ParseRenderID(first.String())
	if err != nil || parsed != first {
		t.Fatalf("ParseRenderID() = %q, %v", parsed.String(), err)
	}
	input.DPI = 0
	if _, err := DeriveRenderID(input); !errors.Is(err, ErrRenderIdentityInputs) {
		t.Fatalf("zero DPI error = %v", err)
	}
}

func testPlanIdentityInputs(t *testing.T) PlanIdentityInputs {
	t.Helper()
	template, err := ParseSemanticTemplateID(strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := ParseScenarioRevisionID(strings.Repeat("2", 64))
	if err != nil {
		t.Fatal(err)
	}
	resources, err := ParseResourceCatalogID(strings.Repeat("3", 64))
	if err != nil {
		t.Fatal(err)
	}
	return PlanIdentityInputs{
		Template: template, Scenario: scenario, Resources: resources,
		Locale: "en-US", Timezone: "UTC", UnicodeVersion: "15.1",
		CLDRVersion: "45", HyphenationVersion: "none",
		CompatibilityProfile: "paper-0.1", PageProfile: "A4",
		PlannerVersion:     "layoutengine-1",
		CompatibilityFlags: []string{"ascii-fast-path", "strict-breaks"},
	}
}
