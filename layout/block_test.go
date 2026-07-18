// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layout

import (
	"reflect"
	"testing"
)

func TestNormalizeBlockAcceptsAllBuiltInPointersAsValueSnapshots(t *testing.T) {
	blocks := []Block{
		&ParagraphBlock{},
		&HeadingBlock{},
		&ListBlock{},
		&TableBlock{},
		&ImageBlock{},
		&SignatureRowBlock{},
		&MetadataGridBlock{},
		&QRVerificationBlock{},
		&NoteBoxBlock{},
		&SectionBlock{},
		&ClauseBlock{},
		&PageBreakBlock{},
	}

	for _, block := range blocks {
		normalized, ok := NormalizeBlock(block)
		if !ok {
			t.Fatalf("NormalizeBlock(%T) returned ok=false", block)
		}
		if reflect.TypeOf(normalized).Kind() == reflect.Pointer {
			t.Fatalf("NormalizeBlock(%T) returned pointer %T", block, normalized)
		}
		if normalized.DocumentBlockKind() != block.DocumentBlockKind() {
			t.Fatalf("NormalizeBlock(%T) kind = %q, want %q", block, normalized.DocumentBlockKind(), block.DocumentBlockKind())
		}
	}
}

func TestNormalizeBlockTreatsTypedNilLikeNil(t *testing.T) {
	var paragraph *ParagraphBlock
	if block, ok := NormalizeBlock(paragraph); ok || block != nil {
		t.Fatalf("NormalizeBlock(typed nil) = %#v, %v; want nil, false", block, ok)
	}
	doc := NewLayoutDocument()
	doc.AddBlock(paragraph)
	if len(doc.Body) != 0 {
		t.Fatalf("AddBlock(typed nil) body length = %d, want 0", len(doc.Body))
	}
}

func TestAddBlockSnapshotsBuiltInPointer(t *testing.T) {
	paragraph := &ParagraphBlock{Segments: []TextSegment{{Text: "before"}}}
	doc := NewLayoutDocument()
	doc.AddBlock(paragraph)
	paragraph.Segments = []TextSegment{{Text: "after"}}

	got, ok := doc.Body[0].(ParagraphBlock)
	if !ok {
		t.Fatalf("stored block = %T, want ParagraphBlock", doc.Body[0])
	}
	if text := TextSegmentsPlainText(got.Segments); text != "before" {
		t.Fatalf("stored text = %q, want pointer snapshot before mutation", text)
	}
}
