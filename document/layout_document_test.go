// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "testing"

func TestDocumentModelDefaultsAndBlocks(t *testing.T) {
	doc := NewLayoutDocument()

	doc.AddBlock(nil)
	if len(doc.Body) != 0 {
		t.Fatalf("body length after nil block = %d, want 0", len(doc.Body))
	}

	doc.AddBlock(ParagraphBlock{Segments: []TextSegment{{Text: "hello"}}})
	doc.AddBlock(HeadingBlock{Level: 2, Segments: []TextSegment{{Text: "section"}}})
	doc.AddBlock(PageBreakBlock{After: true})

	if len(doc.Body) != 3 {
		t.Fatalf("body length = %d, want 3", len(doc.Body))
	}

	wantKinds := []BlockKind{
		BlockKindParagraph,
		BlockKindHeading,
		BlockKindPageBreak,
	}
	for i, want := range wantKinds {
		if got := doc.Body[i].DocumentBlockKind(); got != want {
			t.Fatalf("body[%d] kind = %q, want %q", i, got, want)
		}
	}
}

func TestPageTemplateFooterHelpers(t *testing.T) {
	template := PageTemplate{
		Footer: &FooterBlock{
			Height:          12,
			ReservePageArea: true,
		},
		PageNumbers: PageNumberOptions{Enabled: true, TotalPageAlias: "{total}"},
	}

	if got := template.FooterReservedHeight(); got != 12 {
		t.Fatalf("FooterReservedHeight() = %.2f, want 12", got)
	}
	if got := template.PageNumberText(3); got != "Page 3 / {total}" {
		t.Fatalf("PageNumberText(3) = %q, want %q", got, "Page 3 / {total}")
	}

	template = PageTemplate{
		Footer: &FooterBlock{
			Height:          12,
			ReservePageArea: true,
		},
		ReserveFooterHeight: 18,
	}
	if got := template.FooterReservedHeight(); got != 18 {
		t.Fatalf("explicit FooterReservedHeight() = %.2f, want 18", got)
	}
}

func TestPageTemplateEvenFooterHelpers(t *testing.T) {
	normal := &FooterBlock{Height: 10, ReservePageArea: true}
	first := &FooterBlock{Height: 6, ReservePageArea: true}
	even := &FooterBlock{Height: 14, ReservePageArea: true}
	template := PageTemplate{
		Footer:               normal,
		FirstPageFooter:      first,
		EvenPageFooter:       even,
		EvenPageFooterHeight: 16,
	}

	if got := template.FooterForPage(1); got != first {
		t.Fatalf("FooterForPage(1) = %#v, want first footer", got)
	}
	if got := template.FooterForPage(2); got != even {
		t.Fatalf("FooterForPage(2) = %#v, want even footer", got)
	}
	if got := template.FooterForPage(3); got != normal {
		t.Fatalf("FooterForPage(3) = %#v, want normal footer", got)
	}
	if got := template.FooterReservedHeightForPage(2); got != 16 {
		t.Fatalf("FooterReservedHeightForPage(2) = %.2f, want 16", got)
	}
	if got := template.FooterReservedHeightForPage(3); got != 10 {
		t.Fatalf("FooterReservedHeightForPage(3) = %.2f, want 10", got)
	}
}

func TestAllSharedBlockKinds(t *testing.T) {
	blocks := []struct {
		block Block
		want  BlockKind
	}{
		{ParagraphBlock{}, BlockKindParagraph},
		{HeadingBlock{}, BlockKindHeading},
		{ListBlock{}, BlockKindList},
		{TableBlock{}, BlockKindTable},
		{ImageBlock{}, BlockKindImage},
		{SignatureRowBlock{}, BlockKindSignatureRow},
		{MetadataGridBlock{}, BlockKindMetadataGrid},
		{QRVerificationBlock{}, BlockKindQRVerification},
		{NoteBoxBlock{}, BlockKindNoteBox},
		{SectionBlock{}, BlockKindSection},
		{ClauseBlock{}, BlockKindClause},
		{PageBreakBlock{}, BlockKindPageBreak},
	}

	for _, tt := range blocks {
		if got := tt.block.DocumentBlockKind(); got != tt.want {
			t.Fatalf("%T kind = %q, want %q", tt.block, got, tt.want)
		}
	}
}

func TestSignatureBlockPAdESFieldName(t *testing.T) {
	if got := (SignatureBlock{}).PAdESFieldName(); got != "Signature1" {
		t.Fatalf("default PAdESFieldName() = %q, want Signature1", got)
	}
	block := SignatureBlock{PlaceholderReference: " ApprovalSignature "}
	if got := block.PAdESFieldName(); got != "ApprovalSignature" {
		t.Fatalf("PAdESFieldName() = %q, want ApprovalSignature", got)
	}
}

func TestDocumentPageTemplateModel(t *testing.T) {
	doc := NewLayoutDocument()
	doc.PageTemplate = PageTemplate{
		Header: &HeaderBlock{Height: 12},
		Footer: &FooterBlock{
			Height:          12,
			ReservePageArea: true,
		},
		PageNumbers:         PageNumberOptions{Enabled: true, Format: "Page %d of {total}", TotalPageAlias: "{total}"},
		ReserveFooterHeight: 18,
	}

	template := doc.PageTemplate
	if template.Header.Height != 12 {
		t.Fatalf("header height = %.2f, want 12", template.Header.Height)
	}
	if !template.PageNumbers.Enabled {
		t.Fatal("template should carry page number option")
	}
	if template.PageNumbers.TotalPageAlias != "{total}" {
		t.Fatalf("total page alias = %q, want {total}", template.PageNumbers.TotalPageAlias)
	}
	if got := template.FooterReservedHeight(); got != 18 {
		t.Fatalf("FooterReservedHeight() = %.2f, want 18", got)
	}
}
