// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperscenario

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func stressTestFixture(t *testing.T) Fixture {
	t.Helper()
	resolved, err := Resolve([]Scenario{{Name: "typical", Locale: "en-US", Values: []Field{
		{Name: "customer", Value: Value{Kind: Object, Object: []Field{{Name: "name", Value: Value{Kind: String, String: "Ada"}}}}},
		{Name: "amount", Value: Value{Kind: Number, Number: "12.5"}},
		{Name: "rows", Value: Value{Kind: List, List: []Item{{Key: "one", Value: Value{Kind: Object, Object: []Field{{Name: "label", Value: Value{Kind: String, String: "row"}}}}}}}},
	}}}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return resolved[0]
}

func TestGenerateStressFixturesIsDeterministicDetachedAndBounded(t *testing.T) {
	base := stressTestFixture(t)
	limits := DefaultStressLimits()
	limits.MaxStringBytes = 128
	limits.MaxListItems = 8
	strategies := []StressStrategy{StressEmptyCollections, StressMaximumCollections, StressLongLocalizedText, StressUnbreakableText, StressComplexUnicode, StressExtremeNumbers}
	first, err := GenerateStressFixtures(context.Background(), base, strategies, 42, limits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateStressFixtures(context.Background(), base, strategies, 42, limits)
	if err != nil || !reflect.DeepEqual(first, second) || len(first) != len(strategies) {
		t.Fatalf("generated fixtures differ: %v\n%#v\n%#v", err, first, second)
	}
	if got := first[0].Fixture.Values[fieldIndex(first[0].Fixture.Values, "rows")].Value.List; len(got) != 0 {
		t.Fatalf("empty list = %#v", got)
	}
	maxRows := first[1].Fixture.Values[fieldIndex(first[1].Fixture.Values, "rows")].Value.List
	if len(maxRows) != int(limits.MaxListItems) || maxRows[1].Key != "stress-42-1" {
		t.Fatalf("maximum rows = %#v", maxRows)
	}
	name, _ := fixtureStringAtPath(first[2].Fixture.Values, "customer.name")
	if len(name) > int(limits.MaxStringBytes) || len(name) <= len("Ada") {
		t.Fatalf("long localized name bytes = %d", len(name))
	}
	amount := first[5].Fixture.Values[fieldIndex(first[5].Fixture.Values, "amount")].Value.Number
	if amount != "999999999999999999999999.99" {
		t.Fatalf("extreme amount = %q", amount)
	}
	first[1].Fixture.Values[0].Name = "mutated"
	if second[1].Fixture.Values[0].Name == "mutated" || base.Values[0].Name == "mutated" {
		t.Fatal("generated fixtures alias inputs or each other")
	}
}

func TestGenerateStressFixturesRejectsDuplicateUnsupportedAndCancellation(t *testing.T) {
	base := stressTestFixture(t)
	if _, err := GenerateStressFixtures(context.Background(), base, []StressStrategy{StressLongLocalizedText, StressLongLocalizedText}, 1, StressLimits{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate strategy = %v", err)
	}
	if _, err := GenerateStressFixtures(context.Background(), base, []StressStrategy{"assets"}, 1, StressLimits{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported strategy = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GenerateStressFixtures(ctx, base, []StressStrategy{StressLongLocalizedText}, 1, StressLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestFindStringRepeatBoundaryReturnsAdjacentReplayableFixtures(t *testing.T) {
	base := stressTestFixture(t)
	evaluate := func(_ context.Context, fixture Fixture) (LayoutObservation, error) {
		name, _ := fixtureStringAtPath(fixture.Values, "customer.name")
		pages := uint32(1)
		if len(name) >= 15 {
			pages = 2
		}
		return LayoutObservation{PageCount: pages, BreakDigest: "pages-" + string(rune('0'+pages))}, nil
	}
	result, err := FindStringRepeatBoundary(context.Background(), BoundaryRequest{Base: base, Path: "customer.name", Minimum: 1, Maximum: 16, Evaluate: evaluate})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := fixtureStringAtPath(result.Before.Values, result.Path)
	after, _ := fixtureStringAtPath(result.After.Values, result.Path)
	if result.Boundary != 5 || len(before) != 12 || len(after) != 15 || result.BeforeObservation.PageCount != 1 || result.AfterObservation.PageCount != 2 || result.Evaluations > 10 {
		t.Fatalf("boundary = %#v before=%q after=%q", result, before, after)
	}
	second, err := FindStringRepeatBoundary(context.Background(), BoundaryRequest{Base: base, Path: "customer.name", Minimum: 1, Maximum: 16, Evaluate: evaluate})
	if err != nil || !reflect.DeepEqual(result, second) {
		t.Fatalf("boundary is nondeterministic: %v %#v", err, second)
	}
}

func TestFindStringRepeatBoundaryLimitsAndNoCrossing(t *testing.T) {
	base := stressTestFixture(t)
	constant := func(context.Context, Fixture) (LayoutObservation, error) { return LayoutObservation{PageCount: 1}, nil }
	if _, err := FindStringRepeatBoundary(context.Background(), BoundaryRequest{Base: base, Path: "customer.name", Minimum: 1, Maximum: 4, Evaluate: constant}); err == nil {
		t.Fatal("no-crossing boundary succeeded")
	}
	limits := DefaultStressLimits()
	limits.MaxEvaluations = 2
	change := func(_ context.Context, fixture Fixture) (LayoutObservation, error) {
		name, _ := fixtureStringAtPath(fixture.Values, "customer.name")
		return LayoutObservation{PageCount: uint32(len(name))}, nil
	}
	if _, err := FindStringRepeatBoundary(context.Background(), BoundaryRequest{Base: base, Path: "customer.name", Minimum: 1, Maximum: 16, Evaluate: change, Limits: limits}); !errors.Is(err, ErrLimit) {
		t.Fatalf("evaluation limit = %v", err)
	}
}

func TestListAndIntegerBoundaryAxesAreStableKeyedAndAdjacent(t *testing.T) {
	base := stressTestFixture(t)
	listResult, err := FindListLengthBoundary(context.Background(), BoundaryRequest{
		Base: base, Path: "rows", Minimum: 0, Maximum: 16,
		Evaluate: func(_ context.Context, fixture Fixture) (LayoutObservation, error) {
			value, _ := fixtureValueAtPath(fixture.Values, "rows")
			pages := uint32(1)
			if len(value.List) >= 7 {
				pages = 2
			}
			return LayoutObservation{PageCount: pages}, nil
		},
	})
	if err != nil || listResult.Boundary != 7 {
		t.Fatalf("list boundary = %#v, %v", listResult, err)
	}
	beforeRows, _ := fixtureValueAtPath(listResult.Before.Values, "rows")
	afterRows, _ := fixtureValueAtPath(listResult.After.Values, "rows")
	if len(beforeRows.List) != 6 || len(afterRows.List) != 7 || afterRows.List[6].Key != "boundary-6" {
		t.Fatalf("list boundary rows = %d/%d %#v", len(beforeRows.List), len(afterRows.List), afterRows.List)
	}

	integerResult, err := FindIntegerBoundary(context.Background(), BoundaryRequest{
		Base: base, Path: "amount", Minimum: 0, Maximum: 100,
		Evaluate: func(_ context.Context, fixture Fixture) (LayoutObservation, error) {
			value, _ := fixtureValueAtPath(fixture.Values, "amount")
			pages := uint32(1)
			if len(value.Number) >= 2 {
				pages = 2
			}
			return LayoutObservation{PageCount: pages}, nil
		},
	})
	if err != nil || integerResult.Boundary != 10 {
		t.Fatalf("integer boundary = %#v, %v", integerResult, err)
	}
	beforeAmount, _ := fixtureValueAtPath(integerResult.Before.Values, "amount")
	afterAmount, _ := fixtureValueAtPath(integerResult.After.Values, "amount")
	if beforeAmount.Number != "9" || afterAmount.Number != "10" {
		t.Fatalf("integer boundary values = %q/%q", beforeAmount.Number, afterAmount.Number)
	}
}

func TestMinimizeFixtureDeltaDebugsFieldsAndStrings(t *testing.T) {
	base := stressTestFixture(t)
	values := cloneFields(base.Values)
	setFixtureStringAtPath(values, "customer.name", strings.Repeat("X", 64))
	input, err := normalizeStressFixture(base.Name, base.Locale, values)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(_ context.Context, fixture Fixture) (bool, error) {
		name, ok := fixtureStringAtPath(fixture.Values, "customer.name")
		return ok && len(name) >= 8, nil
	}
	result, err := MinimizeFixture(context.Background(), input, issue, StressLimits{})
	if err != nil {
		t.Fatal(err)
	}
	name, _ := fixtureStringAtPath(result.Fixture.Values, "customer.name")
	if len(result.Fixture.Values) != 1 || result.Fixture.Values[0].Name != "customer" || len(name) != 8 || result.Evaluations == 0 {
		t.Fatalf("minimized = %#v name=%q", result, name)
	}
	result.Fixture.Values[0].Name = "changed"
	if input.Values[0].Name == "changed" {
		t.Fatal("minimized fixture aliases input")
	}
}
