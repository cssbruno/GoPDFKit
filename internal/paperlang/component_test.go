// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestComponentSlotUseGrammarFormatsStably(t *testing.T) {
	const source = "document:\n" +
		"  component @card:\n" +
		"    slot @content:\n" +
		"      type: \"text\"\n" +
		"      paragraph @fallback:\n" +
		"        text: \"Fallback\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      use @card-one:\n" +
		"        component: \"@card\"\n" +
		"        fill @content:\n" +
		"          paragraph @provided:\n" +
		"            text: \"Provided\"\n"
	parsed := Parse("component.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	component := parsed.AST.Root.Members[0].Node
	use := parsed.AST.Root.Members[1].Node.Members[0].Node.Members[0].Node
	if component.Kind != NodeComponent || component.ID != "@card" ||
		component.Members[0].Node.Kind != NodeSlot || use.Kind != NodeUse || use.Members[1].Node.Kind != NodeFill {
		t.Fatalf("component/use AST = %#v / %#v", component, use)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	reparsed := Parse("component.paper", string(formatted))
	second, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(formatted, second) {
		t.Fatalf("parse/format/parse stability failed: %+v / %v\n%s\n%s", reparsed.Diagnostics, secondErr, formatted, second)
	}
}

func TestComponentHierarchyAndScopedSlotNames(t *testing.T) {
	valid := Parse("scoped.paper", "document:\n  component @one:\n    slot @body:\n      type: \"blocks\"\n  component @two:\n    slot @body:\n      type: \"blocks\"\n  page:\n    body:\n      use @instance:\n        component: \"@one\"\n")
	if !valid.OK() {
		t.Fatalf("scoped slots diagnostics = %+v", valid.Diagnostics)
	}
	invalid := Parse("hierarchy.paper", "document:\n  page:\n    fill @body:\n      paragraph:\n        text: \"no\"\n")
	if invalid.OK() {
		t.Fatal("fill outside use unexpectedly parsed as valid")
	}
}

func TestTypedComponentContractsAreReadableLosslessAndFormattable(t *testing.T) {
	const source = "document:\n  component @title-card:\n    prop @title:\n      type: \"string\"\n      required: true\n    slot @content:\n      type: \"blocks\"\n      cardinality: \"one\"\n      layout-affecting: true\n      scenarios: \"@compact, @expanded\"\n  page:\n    body:\n      use @card-one:\n        component: \"@title-card\"\n        arg @title: \"Hello\"\n        fill @content:\n          scenario: \"@compact\"\n          text: \"Short\"\n"
	parsed := Parse("typed-component.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse diagnostics = %+v", parsed.Diagnostics)
	}
	component := parsed.AST.Root.Members[0].Node
	use := parsed.AST.Root.Members[1].Node.Members[0].Node.Members[0].Node
	if component.Members[0].Node.Kind != NodeProp || component.Members[0].Node.ID != "@title" ||
		use.Members[1].Node.Kind != NodeArg || use.Members[1].Node.Value == nil {
		t.Fatalf("typed contract AST = %#v / %#v", component, use)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	reparsed := Parse("typed-component.paper", string(formatted))
	second, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(formatted, second) {
		t.Fatalf("typed contract round trip failed: %+v / %v\n%s", reparsed.Diagnostics, secondErr, formatted)
	}
}
