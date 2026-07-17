// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperscenario

import (
	"context"
	"reflect"
	"testing"
)

func TestGenerateStressMatrixCoversSchemaDatePageAndResourceAxes(t *testing.T) {
	base := stressTestFixture(t)
	values := cloneFields(base.Values)
	values = append(values, Field{Name: "issued", Value: Value{Kind: String, String: "2026-07-17T12:00:00Z"}})
	base, _ = normalizeStressFixture(base.Name, base.Locale, values)
	request := StressMatrixRequest{
		Base: base, Strategies: []StressStrategy{StressNegativeNumbers, StressDecimalPrecision}, Seed: 7,
		OptionalPaths: []string{"customer.name"}, DatePaths: []string{"issued"},
		PageProfiles:   []PageProfileVariant{{Name: "narrow", WidthMilliPoints: 200_000, HeightMilliPoints: 500_000, MarginMilliPoints: 10_000}},
		ResourceFaults: []ResourceFault{{ResourceKind: "font", ResourceID: "body", Fault: ResourceMissing}, {ResourceKind: "asset", ResourceID: "logo", Fault: ResourceMalformed}},
	}
	first, err := GenerateStressMatrix(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateStressMatrix(context.Background(), request)
	if err != nil || !reflect.DeepEqual(first, second) || len(first) != 9 {
		t.Fatalf("matrix deterministic cases=%d second=%d err=%v", len(first), len(second), err)
	}
	customer, _ := fixtureValueAtPath(first[2].Fixture.Values, "customer")
	if _, exists := fixtureValueAtPath(customer.Object, "name"); exists {
		t.Fatalf("optional nested field was not omitted: %#v", customer)
	}
	if first[6].PageProfile == nil || first[7].ResourceFault == nil || first[8].ResourceFault == nil {
		t.Fatalf("environment axes missing: %#v", first)
	}
	base.Values[0].Name = "mutated"
	if first[0].Fixture.Values[0].Name == "mutated" {
		t.Fatal("matrix aliases base fixture")
	}
}

func TestGenerateStressMatrixRejectsInvalidAndBoundsCandidates(t *testing.T) {
	base := stressTestFixture(t)
	if _, err := GenerateStressMatrix(context.Background(), StressMatrixRequest{Base: base, OptionalPaths: []string{"missing"}}); err == nil {
		t.Fatal("missing optional path accepted")
	}
	if _, err := GenerateStressMatrix(context.Background(), StressMatrixRequest{Base: base, ResourceFaults: []ResourceFault{{ResourceKind: "network", ResourceID: "x", Fault: ResourceMissing}}}); err == nil {
		t.Fatal("ambient resource kind accepted")
	}
	limits := DefaultStressLimits()
	limits.MaxCandidates = 1
	_, err := GenerateStressMatrix(context.Background(), StressMatrixRequest{Base: base, ResourceFaults: []ResourceFault{{ResourceKind: "font", ResourceID: "a", Fault: ResourceMissing}, {ResourceKind: "font", ResourceID: "b", Fault: ResourceMissing}}, Limits: limits})
	if err == nil {
		t.Fatal("candidate bound was not enforced")
	}
}
