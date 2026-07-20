// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperrepeat

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperexpr"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

func TestExpandPreservesAuthoredOrderButDerivesStableIdentityFromKey(t *testing.T) {
	predicate := compilePredicate(t, "active && score == 2", []paperexpr.PathKind{{Path: "active", Kind: paperexpr.Bool}, {Path: "score", Kind: paperexpr.Integer}})
	items := []paperscenario.Item{
		objectItem("bravo", boolField("active", true), numberField("score", "2"), stringField("name", "B")),
		objectItem("alpha", boolField("active", true), numberField("score", "2"), stringField("name", "A")),
		objectItem("hidden", boolField("active", false), numberField("score", "2")),
	}
	expansion, err := Expand(context.Background(), Input{Items: items, MaxOutput: 3, Predicate: &predicate, InstancePrefix: "invoice/items"}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	instances := expansion.Instances()
	if expansion.Len() != 2 || len(instances) != 2 || instances[0].Key != "bravo" || instances[1].Key != "alpha" {
		t.Fatalf("instances = %#v", instances)
	}
	if instances[0].Identity != "invoice/items/bravo" || instances[0].Path != "invoice/items[bravo]" ||
		instances[1].Identity != "invoice/items/alpha" || instances[1].Path != "invoice/items[alpha]" {
		t.Fatalf("stable identities = %#v", instances)
	}

	reordered, err := Expand(context.Background(), Input{Items: []paperscenario.Item{items[1], items[0]}, MaxOutput: 2, Predicate: &predicate, InstancePrefix: "invoice/items"}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	got := reordered.Instances()
	if got[0].Key != "alpha" || got[0].Identity != instances[1].Identity || got[1].Key != "bravo" || got[1].Identity != instances[0].Identity {
		t.Fatalf("reordered identities = %#v", got)
	}
}

func TestExpansionOwnsDetachedInputAndReturnedInstances(t *testing.T) {
	items := []paperscenario.Item{objectItem("alpha", stringField("name", "original"), paperscenario.Field{Name: "nested", Value: paperscenario.Value{Kind: paperscenario.List,
		List: []paperscenario.Item{{Key: "child", Value: paperscenario.Value{Kind: paperscenario.String, String: "value"}}}}})}
	expansion, err := Expand(context.Background(), Input{Items: items, MaxOutput: 1, InstancePrefix: "items"}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	items[0].Key = "mutated"
	items[0].Value.Object[0].Value.String = "mutated"
	first := expansion.Instances()
	first[0].Key = "changed"
	first[0].Value.Object[1].Value.List[0].Value.String = "changed"
	second := expansion.Instances()
	if second[0].Key != "alpha" || second[0].Value.Object[0].Value.String != "original" ||
		second[0].Value.Object[1].Value.List[0].Value.String != "value" {
		t.Fatalf("detached instances = %#v", second)
	}
}

func TestExpandRejectsStableKeyAndObjectShapeErrorsWithExactPaths(t *testing.T) {
	tests := []struct {
		name  string
		items []paperscenario.Item
		path  string
	}{
		{"missing key", []paperscenario.Item{{Value: objectValue()}}, "items[0].key"},
		{"invalid key", []paperscenario.Item{{Key: "bad key", Value: objectValue()}}, "items[0].key"},
		{"duplicate key", []paperscenario.Item{objectItem("same"), objectItem("same")}, "items[1].key"},
		{"nonobject", []paperscenario.Item{{Key: "value", Value: paperscenario.Value{Kind: paperscenario.String, String: "x"}}}, "items[0].value"},
		{"duplicate field", []paperscenario.Item{objectItem("value", stringField("name", "a"), stringField("name", "b"))}, "items[0].value.name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Expand(context.Background(), Input{Items: test.items, MaxOutput: 10, InstancePrefix: "items"}, DefaultLimits())
			assertRepeatError(t, err, ErrInvalid, test.path)
		})
	}
	_, err := Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a")}, MaxOutput: 1, InstancePrefix: "bad//prefix"}, DefaultLimits())
	assertRepeatError(t, err, ErrInvalid, "instance_prefix")
}

func TestPredicateBindingsRejectMissingCollectionsDecimalsAndNonBoolResults(t *testing.T) {
	active := compilePredicate(t, "active", []paperexpr.PathKind{{Path: "active", Kind: paperexpr.Bool}})
	_, err := Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a")}, MaxOutput: 1, Predicate: &active, InstancePrefix: "items"}, DefaultLimits())
	assertRepeatError(t, err, ErrBinding, "items[0].bindings.active")

	tags := compilePredicate(t, `tags == "x"`, []paperexpr.PathKind{{Path: "tags", Kind: paperexpr.String}})
	_, err = Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a", paperscenario.Field{Name: "tags", Value: paperscenario.Value{Kind: paperscenario.List}})}, MaxOutput: 1, Predicate: &tags, InstancePrefix: "items"}, DefaultLimits())
	assertRepeatError(t, err, ErrBinding, "items[0].bindings.tags")

	score := compilePredicate(t, "score == 2", []paperexpr.PathKind{{Path: "score", Kind: paperexpr.Integer}})
	_, err = Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a", numberField("score", "2.5"))}, MaxOutput: 1, Predicate: &score, InstancePrefix: "items"}, DefaultLimits())
	assertRepeatError(t, err, ErrBinding, "items[0].bindings.score")

	name := compilePredicate(t, "name", []paperexpr.PathKind{{Path: "name", Kind: paperexpr.String}})
	_, err = Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a", stringField("name", "A"))}, MaxOutput: 1, Predicate: &name, InstancePrefix: "items"}, DefaultLimits())
	assertRepeatError(t, err, ErrPredicate, "items[0].predicate")
}

func TestNestedPrimitivePredicateBinding(t *testing.T) {
	predicate := compilePredicate(t, "profile.enabled", []paperexpr.PathKind{{Path: "profile.enabled", Kind: paperexpr.Bool}})
	item := objectItem("a", paperscenario.Field{Name: "profile", Value: paperscenario.Value{Kind: paperscenario.Object,
		Object: []paperscenario.Field{boolField("enabled", true)}}})
	expansion, err := Expand(context.Background(), Input{Items: []paperscenario.Item{item}, MaxOutput: 1, Predicate: &predicate, InstancePrefix: "items"}, DefaultLimits())
	if err != nil || expansion.Len() != 1 {
		t.Fatalf("nested binding expansion = %#v, %v", expansion.Instances(), err)
	}
}

func TestExpandEnforcesOutputWorkDepthByteAndInputLimits(t *testing.T) {
	items := []paperscenario.Item{objectItem("a"), objectItem("b")}
	_, err := Expand(context.Background(), Input{Items: items, MaxOutput: 1, InstancePrefix: "items"}, DefaultLimits())
	assertRepeatError(t, err, ErrLimit, "items[1]")

	limits := DefaultLimits()
	limits.MaxWork = 1
	_, err = Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a", stringField("name", "x"))}, MaxOutput: 1, InstancePrefix: "items"}, limits)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("work-limited Expand() = %v", err)
	}

	deep := paperscenario.Value{Kind: paperscenario.String, String: "leaf"}
	for index := 0; index < 4; index++ {
		deep = paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{{Name: "child", Value: deep}}}
	}
	limits = DefaultLimits()
	limits.MaxDepth = 3
	_, err = Expand(context.Background(), Input{Items: []paperscenario.Item{{Key: "a", Value: deep}}, MaxOutput: 1, InstancePrefix: "items"}, limits)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("depth-limited Expand() = %v", err)
	}

	limits = DefaultLimits()
	limits.MaxStateBytes = 64
	_, err = Expand(context.Background(), Input{Items: []paperscenario.Item{objectItem("a", stringField("name", strings.Repeat("x", 100)))}, MaxOutput: 1, InstancePrefix: "items"}, limits)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("byte-limited Expand() = %v", err)
	}

	limits = DefaultLimits()
	limits.MaxInputItems = 1
	_, err = Expand(context.Background(), Input{Items: items, MaxOutput: 1, InstancePrefix: "items"}, limits)
	assertRepeatError(t, err, ErrLimit, "items")

	_, err = Expand(context.Background(), Input{Items: nil, MaxOutput: 0, InstancePrefix: "items"}, DefaultLimits())
	assertRepeatError(t, err, ErrLimit, "max_output")
}

func TestExpandHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	expansion, err := Expand(ctx, Input{Items: []paperscenario.Item{objectItem("a")}, MaxOutput: 1, InstancePrefix: "items"}, DefaultLimits())
	if !errors.Is(err, context.Canceled) || expansion.Len() != 0 {
		t.Fatalf("canceled Expand() = %#v, %v", expansion.Instances(), err)
	}
}

func assertRepeatError(t *testing.T, err, cause error, path string) {
	t.Helper()
	var diagnostic *ExpansionError
	if !errors.Is(err, cause) || !errors.As(err, &diagnostic) || diagnostic.Path != path {
		t.Fatalf("error = %v (%#v), want cause %v path %q", err, diagnostic, cause, path)
	}
}

func compilePredicate(t *testing.T, source string, environment []paperexpr.PathKind) paperexpr.Program {
	t.Helper()
	program, _, err := paperexpr.Compile(source, environment, paperexpr.LanguageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func objectItem(key string, fields ...paperscenario.Field) paperscenario.Item {
	return paperscenario.Item{Key: key, Value: paperscenario.Value{Kind: paperscenario.Object, Object: fields}}
}

func objectValue(fields ...paperscenario.Field) paperscenario.Value {
	return paperscenario.Value{Kind: paperscenario.Object, Object: fields}
}

func boolField(name string, value bool) paperscenario.Field {
	return paperscenario.Field{Name: name, Value: paperscenario.Value{Kind: paperscenario.Bool, Bool: value}}
}

func numberField(name, value string) paperscenario.Field {
	return paperscenario.Field{Name: name, Value: paperscenario.Value{Kind: paperscenario.Number, Number: value}}
}

func stringField(name, value string) paperscenario.Field {
	return paperscenario.Field{Name: name, Value: paperscenario.Value{Kind: paperscenario.String, String: value}}
}

func TestInstancesAreDeterministic(t *testing.T) {
	input := Input{Items: []paperscenario.Item{objectItem("a", stringField("name", "A"))}, MaxOutput: 1, InstancePrefix: "items"}
	first, err := Expand(context.Background(), input, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Expand(context.Background(), input, DefaultLimits())
	if err != nil || !reflect.DeepEqual(first.Instances(), second.Instances()) {
		t.Fatalf("deterministic expansion = %#v / %#v, %v", first.Instances(), second.Instances(), err)
	}
}
