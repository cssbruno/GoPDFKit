// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
)

// This test is intentionally both a drift detector and an executable mapping
// inventory. A new public LayoutDocument field cannot silently bypass the exact
// adapter: it must be named here and supplied with causal plan evidence.
func TestExactTypedAdapterMapsEveryLayoutDocumentField(t *testing.T) {
	type fieldCase struct {
		name     string
		typeName string
		mutate   func(*layout.LayoutDocument)
	}
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}}
	}
	cases := []fieldCase{
		{"Title", "string", func(doc *layout.LayoutDocument) { doc.Title = "Mapped title" }},
		{"Language", "string", func(doc *layout.LayoutDocument) { doc.Language = "en-US" }},
		{"Metadata", "layout.DocumentMetadata", func(doc *layout.LayoutDocument) { doc.Metadata.Subject = "Mapped subject" }},
		{"PageTemplate", "layout.PageTemplate", func(doc *layout.LayoutDocument) { doc.PageTemplate.Margins.Left = 17 }},
		{"Body", "[]layout.Block", func(doc *layout.LayoutDocument) { doc.Body = []layout.Block{paragraph("changed body")} }},
		{"Signature", "*layout.SignatureBlock", func(doc *layout.LayoutDocument) {
			doc.Signature = &layout.SignatureBlock{PlaceholderReference: "Approval", Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Label: "Approve", Name: "Ada"}}}}}
		}},
		{"QR", "*layout.QRBlock", func(doc *layout.LayoutDocument) {
			doc.QR = &layout.QRBlock{Value: "mapped-qr", Label: "Verify", Size: 24}
		}},
		{"Attachments", "[]layout.AttachmentBlock", func(doc *layout.LayoutDocument) {
			doc.Attachments = []layout.AttachmentBlock{{Name: "proof.txt", MIMEType: "text/plain", Description: "mapping proof", Data: []byte("mapped")}}
		}},
	}

	typeOf := reflect.TypeOf(layout.LayoutDocument{})
	if typeOf.NumField() != len(cases) {
		t.Fatalf("LayoutDocument field count drifted: got %d want %d", typeOf.NumField(), len(cases))
	}
	for index, test := range cases {
		field := typeOf.Field(index)
		if field.Name != test.name || field.Type.String() != test.typeName {
			t.Fatalf("LayoutDocument field[%d] drifted: got %s:%s want %s:%s", index, field.Name, field.Type, test.name, test.typeName)
		}
	}

	newModel := func() *layout.LayoutDocument {
		return &layout.LayoutDocument{
			PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: 10, Right: 10, Bottom: 10, Left: 10}},
			Body:         []layout.Block{paragraph("baseline body")},
		}
	}
	newPlanner := func() *Document {
		return MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 220}), WithNoCompression(), WithDeterministicOutput())
	}
	baseline, err := newPlanner().PlanLayoutDocument(newModel())
	if err != nil {
		t.Fatalf("baseline exact plan: %v", err)
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			model := newModel()
			test.mutate(model)
			plan, err := newPlanner().PlanLayoutDocument(model)
			if err != nil || plan.PageCount() == 0 || plan.Hash() == "" {
				t.Fatalf("mapped field plan = pages %d hash %q, %v", plan.PageCount(), plan.Hash(), err)
			}
			if plan.Hash() == baseline.Hash() {
				t.Fatalf("field %s had no causal effect on immutable plan identity", test.name)
			}
		})
	}
}
