// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "testing"

func TestDocumentModelDefaultsAndBlocks(t *testing.T) {
	doc := NewLayoutDocument("")
	if doc.Kind != DocumentKindGeneric {
		t.Fatalf("kind = %q, want %q", doc.Kind, DocumentKindGeneric)
	}

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

func TestPageChromeFooterHelpers(t *testing.T) {
	chrome := PageChrome{
		Footer: &FooterBlock{
			Height:          12,
			ShowPageNumber:  true,
			TotalPageAlias:  "{total}",
			ReservePageArea: true,
		},
	}

	if got := chrome.FooterReservedHeight(); got != 12 {
		t.Fatalf("FooterReservedHeight() = %.2f, want 12", got)
	}
	if got := chrome.PageNumberText(3); got != "Page 3 / {total}" {
		t.Fatalf("PageNumberText(3) = %q, want %q", got, "Page 3 / {total}")
	}

	chrome = PageChrome{
		Footer: &FooterBlock{
			Height:          12,
			ReservePageArea: true,
		},
		ReserveFooterHeight: 18,
	}
	if got := chrome.FooterReservedHeight(); got != 18 {
		t.Fatalf("explicit FooterReservedHeight() = %.2f, want 18", got)
	}
}

func TestPageChromeAlternateFooterHelpers(t *testing.T) {
	normal := &FooterBlock{Height: 10, ReservePageArea: true}
	first := &FooterBlock{Height: 6, ReservePageArea: true}
	alternate := &FooterBlock{Height: 14, ReservePageArea: true}
	chrome := PageChrome{
		Footer:                normal,
		FirstPageFooter:       first,
		AlternateFooter:       alternate,
		AlternateFooterHeight: 16,
	}

	if got := chrome.FooterForPage(1); got != first {
		t.Fatalf("FooterForPage(1) = %#v, want first footer", got)
	}
	if got := chrome.FooterForPage(2); got != alternate {
		t.Fatalf("FooterForPage(2) = %#v, want alternate footer", got)
	}
	if got := chrome.FooterForPage(3); got != normal {
		t.Fatalf("FooterForPage(3) = %#v, want normal footer", got)
	}
	if got := chrome.FooterReservedHeightForPage(2); got != 16 {
		t.Fatalf("FooterReservedHeightForPage(2) = %.2f, want 16", got)
	}
	if got := chrome.FooterReservedHeightForPage(3); got != 10 {
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

func TestDocumentPageChromeModel(t *testing.T) {
	doc := NewLayoutDocument(DocumentKindReport)
	doc.Chrome = &PageChrome{
		Header: &HeaderBlock{Height: 12},
		Footer: &FooterBlock{
			Height:          12,
			ShowPageNumber:  true,
			TotalPageAlias:  "{total}",
			ReservePageArea: true,
		},
		PageNumberFormat:    "Page %d of {total}",
		TotalPageAlias:      "{total}",
		ReserveFooterHeight: 18,
	}

	chrome := doc.PageChrome()
	if chrome.Header.Height != 12 {
		t.Fatalf("header height = %.2f, want 12", chrome.Header.Height)
	}
	if !chrome.Footer.ShowPageNumber {
		t.Fatal("footer should carry page number option")
	}
	if chrome.TotalPageAlias != "{total}" {
		t.Fatalf("total page alias = %q, want {total}", chrome.TotalPageAlias)
	}
	if got := chrome.FooterReservedHeight(); got != 18 {
		t.Fatalf("FooterReservedHeight() = %.2f, want 18", got)
	}
}
