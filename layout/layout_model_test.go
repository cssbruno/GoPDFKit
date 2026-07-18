// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layout

import (
	"reflect"
	"testing"
)

type externalBlockForTest struct{}

func (externalBlockForTest) DocumentBlockKind() BlockKind { return "external" }

func TestLayoutDocumentConstructorsAndNormalization(t *testing.T) {
	paragraph := &ParagraphBlock{Segments: []TextSegment{{Text: "body"}}}
	doc := NewDocumentModel("Title", paragraph, RowColumnBlock{}, CanvasBlock{})
	if doc.Title != "Title" || len(doc.Body) != 4 {
		t.Fatalf("NewDocumentModel() = title %q, %d blocks; want title heading plus 3 blocks", doc.Title, len(doc.Body))
	}
	if _, ok := doc.Body[0].(HeadingBlock); !ok {
		t.Fatalf("title block = %T, want HeadingBlock", doc.Body[0])
	}

	var nilCanvas *CanvasBlock
	blocks := NormalizeBlocks([]Block{
		nilCanvas,
		paragraph,
		&RowColumnBlock{},
		&CanvasBlock{},
		externalBlockForTest{},
	})
	if len(blocks) != 4 {
		t.Fatalf("NormalizeBlocks() returned %d blocks, want 4", len(blocks))
	}
	for _, block := range blocks[1:3] {
		if reflect.ValueOf(block).Kind() == reflect.Pointer {
			t.Fatalf("built-in pointer normalized as %T", block)
		}
	}
	if blocks[3].DocumentBlockKind() != "external" {
		t.Fatalf("external block kind = %q, want external", blocks[3].DocumentBlockKind())
	}
}

func TestLayoutEffectiveReferencesAndImageData(t *testing.T) {
	style := TextStyle{FontFamily: "Helvetica", FontSize: 14, Bold: true}
	box := BoxStyle{Width: 42, Padding: Spacing{Left: 2}}
	segment := TextSegment{Style: TextStyle{FontSize: 8}, StyleRef: &style}
	if got := segment.EffectiveStyle(); got != style {
		t.Fatalf("segment EffectiveStyle() = %#v, want reference", got)
	}

	paragraph := ParagraphBlock{Style: TextStyle{FontSize: 8}, StyleRef: &style, Box: BoxStyle{}, BoxRef: &box}
	heading := HeadingBlock{StyleRef: &style, BoxRef: &box}
	list := ListBlock{StyleRef: &style, BoxRef: &box}
	table := TableBlock{BoxRef: &box}
	cell := TableCell{StyleRef: &style, BoxRef: &box}
	imageBytes := []byte{1, 2, 3}
	image := ImageBlock{Data: []byte{9}, DataRef: &imageBytes, BoxRef: &box}
	signatureRow := SignatureRowBlock{BoxRef: &box}
	metadata := MetadataGridBlock{StyleRef: &style, BoxRef: &box}
	qr := QRVerificationBlock{StyleRef: &style, BoxRef: &box}
	note := NoteBoxBlock{StyleRef: &style, BoxRef: &box}
	section := SectionBlock{BoxRef: &box}
	clause := ClauseBlock{BoxRef: &box}
	header := HeaderBlock{BoxRef: &box}
	footer := FooterBlock{BoxRef: &box}

	for name, got := range map[string]TextStyle{
		"paragraph": paragraph.EffectiveStyle(),
		"heading":   heading.EffectiveStyle(),
		"list":      list.EffectiveStyle(),
		"cell":      cell.EffectiveStyle(),
		"metadata":  metadata.EffectiveStyle(),
		"qr":        qr.EffectiveStyle(),
		"note":      note.EffectiveStyle(),
	} {
		if got != style {
			t.Errorf("%s EffectiveStyle() = %#v, want reference", name, got)
		}
	}
	for name, got := range map[string]BoxStyle{
		"paragraph": paragraph.EffectiveBox(),
		"heading":   heading.EffectiveBox(),
		"list":      list.EffectiveBox(),
		"table":     table.EffectiveBox(),
		"cell":      cell.EffectiveBox(),
		"image":     image.EffectiveBox(),
		"signature": signatureRow.EffectiveBox(),
		"metadata":  metadata.EffectiveBox(),
		"qr":        qr.EffectiveBox(),
		"note":      note.EffectiveBox(),
		"section":   section.EffectiveBox(),
		"clause":    clause.EffectiveBox(),
		"header":    header.EffectiveBox(),
		"footer":    footer.EffectiveBox(),
	} {
		if got != box {
			t.Errorf("%s EffectiveBox() = %#v, want reference", name, got)
		}
	}
	if got := image.ImageData(); !reflect.DeepEqual(got, imageBytes) {
		t.Fatalf("ImageData() = %v, want referenced bytes", got)
	}
	if got := (ImageBlock{Data: imageBytes}).ImageData(); !reflect.DeepEqual(got, imageBytes) {
		t.Fatalf("ImageData() without reference = %v, want inline bytes", got)
	}
}

func TestSignatureAndPageTemplateContracts(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", want: "Signature1"},
		{name: "space", in: " \t", want: "Signature1"},
		{name: "trimmed", in: " \tApproval\n", want: "Approval"},
		{name: "interior", in: "Approval Field", want: "Approval Field"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := (SignatureBlock{PlaceholderReference: test.in}).PAdESFieldName(); got != test.want {
				t.Fatalf("PAdESFieldName() = %q, want %q", got, test.want)
			}
		})
	}

	defaultHeader := &HeaderBlock{}
	firstHeader := &HeaderBlock{Height: 10}
	defaultFooter := &FooterBlock{Height: 12, ReservePageArea: true}
	firstFooter := &FooterBlock{Height: 14, ReservePageArea: true}
	evenFooter := &FooterBlock{Height: 16, ReservePageArea: true}
	pt := PageTemplate{
		Header:          defaultHeader,
		FirstPageHeader: firstHeader,
		Footer:          defaultFooter,
		FirstPageFooter: firstFooter,
		EvenPageFooter:  evenFooter,
		PageNumbers:     PageNumberOptions{Enabled: true, TotalPageAlias: "{total}"},
	}
	if pt.HeaderForPage(1) != firstHeader || pt.HeaderForPage(2) != defaultHeader {
		t.Fatal("HeaderForPage() did not select first/default headers")
	}
	if pt.FooterForPage(1) != firstFooter || pt.FooterForPage(2) != evenFooter || pt.FooterForPage(3) != defaultFooter {
		t.Fatal("FooterForPage() did not select first/even/default footers")
	}
	if got := pt.FooterReservedHeightForPage(1); got != 14 {
		t.Fatalf("FooterReservedHeightForPage(1) = %.1f, want 14", got)
	}
	if got := pt.FooterReservedHeightForPage(2); got != 16 {
		t.Fatalf("FooterReservedHeightForPage(2) = %.1f, want 16", got)
	}
	pt.ReserveFooterHeight = 20
	if got := pt.FooterReservedHeight(); got != 20 {
		t.Fatalf("FooterReservedHeight() = %.1f, want explicit reservation", got)
	}
	if got := pt.PageNumberText(2); got != "Page 2 / {total}" {
		t.Fatalf("default PageNumberText() = %q", got)
	}
	if got := (PageTemplate{}).PageNumberText(0); got != "" {
		t.Fatalf("disabled PageNumberText(0) = %q, want empty", got)
	}
	pt.PageNumbers.Format = "p=%02d"
	if got := pt.PageNumberText(3); got != "p=03" {
		t.Fatalf("custom PageNumberText() = %q", got)
	}
	if got := pt.PageTotalAlias(); got != "{total}" {
		t.Fatalf("PageTotalAlias() = %q", got)
	}
}

func TestTextStyleHelpers(t *testing.T) {
	base := TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12, Color: DocumentColor{R: 1, Set: true}}
	override := TextStyle{FontFamily: "Courier", FontSize: 15, Bold: true, Underline: true, Align: "C", TabSize: 4, WhiteSpace: "pre", Color: DocumentColor{G: 2, Set: true}}
	merged := MergedTextStyle(base, override)
	if merged.FontFamily != "Courier" || merged.FontSize != 15 || merged.LineHeight != 18 || !merged.Bold || !merged.Underline || merged.Align != "C" || merged.TabSize != 4 || merged.WhiteSpace != "pre" || merged.Color != override.Color {
		t.Fatalf("MergedTextStyle() = %#v", merged)
	}
	if got := MergedTextStyle(TextStyle{FontSize: 10, LineHeight: 20}, TextStyle{FontSize: 15}).LineHeight; got != 30 {
		t.Fatalf("scaled line height = %.1f, want 30", got)
	}
	if got := MergedTextStyle(TextStyle{FontSize: 10, LineHeight: 20}, TextStyle{FontSize: 15, LineHeight: 9}).LineHeight; got != 9 {
		t.Fatalf("explicit line height = %.1f, want 9", got)
	}
	for _, test := range []struct {
		style TextStyle
		want  float64
	}{
		{TextStyle{LineHeight: 9}, 9},
		{TextStyle{FontSize: 10}, 12},
		{TextStyle{}, 5},
	} {
		if got := ResolvedLineHeight(test.style); got != test.want {
			t.Errorf("ResolvedLineHeight(%#v) = %.1f, want %.1f", test.style, got, test.want)
		}
	}
	if got := HeadingFontSize(0, 1); got != 21.6 {
		t.Fatalf("HeadingFontSize() default = %.1f, want 21.6", got)
	}
	for level, factor := range map[int]float64{1: 1.8, 2: 1.5, 3: 1.25, 4: 1.1} {
		if got := HeadingFontSize(10, level); got != 10*factor {
			t.Errorf("HeadingFontSize(level %d) = %.2f, want %.2f", level, got, 10*factor)
		}
	}

	paragraph := ParagraphBox(BoxStyle{})
	if paragraph.Margin.Bottom != 2 {
		t.Fatalf("ParagraphBox() bottom = %.1f, want 2", paragraph.Margin.Bottom)
	}
	heading := HeadingBox(BoxStyle{})
	if heading.Margin.Top != 2.5 || heading.Margin.Bottom != 1.5 {
		t.Fatalf("HeadingBox() margins = %#v", heading.Margin)
	}
	box := BoxStyle{Padding: Spacing{Left: 2, Right: 3}, Border: BorderStyle{Left: BorderSide{Width: 1}, Right: BorderSide{Width: 2}}}
	if got := InnerWidth(10, box); got != 2 {
		t.Fatalf("InnerWidth() = %.1f, want 2", got)
	}
	if got := InnerWidth(2, box); got != 0 {
		t.Fatalf("InnerWidth() negative result = %.1f, want 0", got)
	}
	if VerticalSpacing(Spacing{Top: 2, Bottom: 3}) != 5 || BorderVertical(BorderStyle{Top: BorderSide{Width: 1}, Bottom: BorderSide{Width: 2}}) != 3 {
		t.Fatal("vertical spacing helpers returned unexpected values")
	}
	if got := FirstPositive(0, -1, 4, 5); got != 4 || FirstPositive(0, -1) != 0 {
		t.Fatal("FirstPositive() returned unexpected value")
	}
}
