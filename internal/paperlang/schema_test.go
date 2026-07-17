// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestSchemaFieldAndBindingSyntaxFormatsStably(t *testing.T) {
	const source = "document:\n" +
		"  schema @invoice:\n" +
		"    field @customer:\n" +
		"      type: \"object\"\n" +
		"      required: false\n" +
		"      field @name:\n" +
		"        type: \"string\"\n" +
		"    field @items:\n" +
		"      type: \"list\"\n" +
		"      item-type: \"object\"\n" +
		"      max-items: 100\n" +
		"      field @price:\n" +
		"        type: \"number\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      paragraph @price:\n" +
		"        bind: \"@invoice.items[].price\"\n" +
		"        text: \"Price\"\n"
	parsed := Parse("schema.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	schema := parsed.AST.Root.Members[0].Node
	if schema.Kind != NodeSchema || schema.ID != "@invoice" || schema.Members[0].Node.Kind != NodeField {
		t.Fatalf("schema AST = %#v", schema)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	reparsed := Parse("schema.paper", string(formatted))
	second, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(formatted, second) {
		t.Fatalf("schema format stability failed: %+v / %v\n%s\n%s", reparsed.Diagnostics, secondErr, formatted, second)
	}
}
