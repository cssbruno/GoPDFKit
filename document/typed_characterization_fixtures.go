// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/base64"
	"math"

	"github.com/cssbruno/paperrune/layout"
)

type typedCharacterizationFixture struct {
	inventory  TypedFixtureInventory
	doc        *layout.LayoutDocument
	pageHeight float64
	mode       string
}

func characterizationParagraph(text string) layout.ParagraphBlock {
	return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}}
}

func characterizationDocument(blocks ...layout.Block) *layout.LayoutDocument {
	return &layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: 10, Right: 10, Bottom: 10, Left: 10}}, Body: blocks}
}

func typedCharacterizationFixtures() []typedCharacterizationFixture {
	pixel, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	p := characterizationParagraph("body")
	envelope := characterizationDocument(p)
	envelope.Signature = &layout.SignatureBlock{
		PlaceholderReference: "CharacterizationSignature",
		Rows:                 []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Label: "Approved", Name: "Ada Example"}}}},
	}
	envelope.QR = &layout.QRBlock{Value: "characterization-envelope", Label: "Verify", Size: 24}
	envelope.Attachments = []layout.AttachmentBlock{{Name: "evidence.txt", MIMEType: "text/plain", Description: "characterization evidence", Data: []byte("typed envelope")}}
	fixtures := []typedCharacterizationFixture{
		{TypedFixtureInventory{Name: "block-paragraph", Coverage: []string{"every-block"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph}}, characterizationDocument(p), 200, ""},
		{TypedFixtureInventory{Name: "block-heading", Coverage: []string{"every-block"}, Blocks: []layout.BlockKind{layout.BlockKindHeading}}, characterizationDocument(layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Heading"}}, Style: p.Style}), 200, ""},
		{TypedFixtureInventory{Name: "block-list", Coverage: []string{"every-block", "nested"}, Blocks: []layout.BlockKind{layout.BlockKindList, layout.BlockKindParagraph}}, characterizationDocument(layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{p}}}}), 200, ""},
		{TypedFixtureInventory{Name: "block-table", Coverage: []string{"every-block", "table"}, Blocks: []layout.BlockKind{layout.BlockKindTable, layout.BlockKindParagraph}}, characterizationDocument(characterizationTable(false, false)), 200, ""},
		{TypedFixtureInventory{Name: "block-image", Coverage: []string{"every-block"}, Blocks: []layout.BlockKind{layout.BlockKindImage}}, characterizationDocument(layout.ImageBlock{Data: pixel, Format: "png", Alt: "pixel", Width: 10, Height: 10}), 200, ""},
		{TypedFixtureInventory{Name: "block-signature-row", Coverage: []string{"every-block"}, Blocks: []layout.BlockKind{layout.BlockKindSignatureRow}}, characterizationDocument(layout.SignatureRowBlock{Columns: []layout.SignatureColumn{{Label: "Sign", Name: "A", Width: 80}}}), 200, ""},
		{TypedFixtureInventory{Name: "block-metadata-grid", Coverage: []string{"every-block"}, Blocks: []layout.BlockKind{layout.BlockKindMetadataGrid}}, characterizationDocument(layout.MetadataGridBlock{Columns: 2, Fields: []layout.MetadataField{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}}), 200, ""},
		{TypedFixtureInventory{Name: "block-qr-verification", Coverage: []string{"every-block"}, Blocks: []layout.BlockKind{layout.BlockKindQRVerification}}, characterizationDocument(layout.QRVerificationBlock{QR: layout.QRBlock{Value: "verify", Size: 24}, Text: []layout.TextSegment{{Text: "Verify"}}}), 200, ""},
		{TypedFixtureInventory{Name: "block-note-box", Coverage: []string{"every-block", "nested"}, Blocks: []layout.BlockKind{layout.BlockKindNoteBox, layout.BlockKindParagraph}}, characterizationDocument(layout.NoteBoxBlock{Title: "Note", Body: []layout.Block{p}}), 200, ""},
		{TypedFixtureInventory{Name: "block-section", Coverage: []string{"every-block", "nested"}, Blocks: []layout.BlockKind{layout.BlockKindSection, layout.BlockKindParagraph}}, characterizationDocument(layout.SectionBlock{Title: "Section", Blocks: []layout.Block{p}}), 200, ""},
		{TypedFixtureInventory{Name: "block-clause", Coverage: []string{"every-block", "nested"}, Blocks: []layout.BlockKind{layout.BlockKindClause, layout.BlockKindParagraph}}, characterizationDocument(layout.ClauseBlock{Number: "1", Title: "Clause", Blocks: []layout.Block{p}}), 200, ""},
		{TypedFixtureInventory{Name: "block-page-break", Coverage: []string{"every-block", "mixed"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph, layout.BlockKindPageBreak}}, characterizationDocument(p, layout.PageBreakBlock{After: true}, p), 200, ""},
		{TypedFixtureInventory{Name: "block-row-column", Coverage: []string{"every-block", "nested"}, Blocks: []layout.BlockKind{layout.BlockKindRowColumn, layout.BlockKindParagraph}}, characterizationDocument(layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{Block: p, Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: 1}}, {Block: p, Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: 1}}}}), 200, ""},
		{TypedFixtureInventory{Name: "block-canvas", Coverage: []string{"every-block", "local-constraints"}, Blocks: []layout.BlockKind{layout.BlockKindCanvas}}, characterizationDocument(layout.CanvasBlock{Width: 120, Height: 60, DefaultHorizontal: "left", DefaultVertical: "top", Items: []layout.CanvasItem{{ID: "@box", Width: 30, Height: 20, Alt: "positioned box", Box: layout.BoxStyle{BackgroundColor: layout.DocumentColor{R: 51, G: 102, B: 153, Set: true}}, Constraints: []layout.CanvasConstraint{{Anchor: "left", Target: "canvas", TargetAnchor: "left", Offset: 8}, {Anchor: "top", Target: "canvas", TargetAnchor: "top", Offset: 8}}}}}), 200, ""},
		{TypedFixtureInventory{Name: "document-envelope-fields", Coverage: []string{"envelope", "attachments", "signature", "qr", "mixed"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph, layout.BlockKindSignatureRow, layout.BlockKindQRVerification}}, envelope, 240, ""},
		{TypedFixtureInventory{Name: "mixed-nested", Coverage: []string{"nested", "mixed"}, Blocks: []layout.BlockKind{layout.BlockKindHeading, layout.BlockKindSection, layout.BlockKindList, layout.BlockKindParagraph}}, characterizationDocument(layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "H"}}, Style: p.Style}, layout.SectionBlock{Blocks: []layout.Block{layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{p}}}}}}), 200, ""},
		{TypedFixtureInventory{Name: "internal-links-hierarchy", Coverage: []string{"internal-links", "semantic-hierarchy", "nested"}, Blocks: []layout.BlockKind{layout.BlockKindSection, layout.BlockKindHeading, layout.BlockKindList, layout.BlockKindParagraph}}, characterizationDocument(
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Jump", Link: "#details"}}, Style: p.Style},
			layout.SectionBlock{Title: "Details", Blocks: []layout.Block{
				layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: "Target", Destination: "details"}}, Style: p.Style},
				layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{p}}}},
			}},
		), 200, ""},
		{TypedFixtureInventory{Name: "page-exact-fit", Coverage: []string{"exact-fit"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph}}, characterizationDocument(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "fit"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 180}}), 200, ""},
		{TypedFixtureInventory{Name: "page-one-unit-over", Coverage: []string{"one-unit-over"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph}}, characterizationDocument(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "over"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 180 + 1.0/1024.0}}), 200, ""},
		{TypedFixtureInventory{Name: "table-wide-rowspan", Coverage: []string{"table", "wide", "rowspan"}, Blocks: []layout.BlockKind{layout.BlockKindTable}}, characterizationDocument(characterizationTable(true, true)), 120, ""},
		{TypedFixtureInventory{Name: "table-large", Coverage: []string{"table", "large"}, Blocks: []layout.BlockKind{layout.BlockKindTable}}, characterizationDocument(characterizationLargeTable()), 120, ""},
		{TypedFixtureInventory{Name: "page-regions-first-even", Coverage: []string{"page-regions", "first", "odd", "even"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph, layout.BlockKindPageBreak}}, characterizationPageRegionDocument(), 100, ""},
		{TypedFixtureInventory{Name: "malformed-heading", Coverage: []string{"malformed", "recovery"}, Blocks: []layout.BlockKind{layout.BlockKindHeading}}, characterizationDocument(layout.HeadingBlock{Level: 99, Segments: []layout.TextSegment{{Text: "bad"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: math.NaN(), LineHeight: 12}}), 200, "malformed"},
		{TypedFixtureInventory{Name: "cancellation", Coverage: []string{"cancellation"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph}}, characterizationDocument(p), 200, "cancel"},
		{TypedFixtureInventory{Name: "resource-limit", Coverage: []string{"limits"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph}}, characterizationDocument(p, p, p), 200, "limit"},
		{TypedFixtureInventory{Name: "concurrent-plan-reuse", Coverage: []string{"concurrent-reuse"}, Blocks: []layout.BlockKind{layout.BlockKindParagraph}}, characterizationDocument(p), 200, "concurrent"},
	}
	return fixtures
}

func characterizationTable(wide, rowspan bool) layout.TableBlock {
	columns := []layout.TableColumn{{Width: 70}, {Width: 70}}
	if wide {
		columns = append(columns, layout.TableColumn{Width: 70})
	}
	span := 1
	if rowspan {
		span = 2
	}
	return layout.TableBlock{Columns: columns, Style: layout.TableStyle{RepeatHeader: true},
		Header: []layout.TableRow{{Cells: []layout.TableCell{{Header: true, ColSpan: len(columns), Blocks: []layout.Block{characterizationParagraph("Header")}}}}},
		Body: []layout.TableRow{{Cells: []layout.TableCell{{RowSpan: span, Blocks: []layout.Block{characterizationParagraph("A")}}, {Blocks: []layout.Block{characterizationParagraph("B")}}}},
			{Cells: []layout.TableCell{{Blocks: []layout.Block{characterizationParagraph("C")}}}}}}
}

func characterizationLargeTable() layout.TableBlock {
	table := characterizationTable(false, false)
	table.Body = make([]layout.TableRow, 128)
	for index := range table.Body {
		table.Body[index] = layout.TableRow{Cells: []layout.TableCell{
			{Blocks: []layout.Block{characterizationParagraph("A")}}, {Blocks: []layout.Block{characterizationParagraph("B")}},
		}}
	}
	return table
}

func typedCharacterizationDocumentWork(doc *layout.LayoutDocument) uint64 {
	if doc == nil {
		return 1
	}
	var visit func([]layout.Block) uint64
	visit = func(blocks []layout.Block) uint64 {
		work := uint64(1)
		for _, candidate := range layout.NormalizeBlocks(blocks) {
			work++
			switch block := candidate.(type) {
			case layout.ListBlock:
				for _, item := range block.Items {
					work += visit(item.Blocks)
				}
			case layout.TableBlock:
				for _, rows := range [][]layout.TableRow{block.Header, block.Body, block.Footer} {
					for _, row := range rows {
						work++
						for _, cell := range row.Cells {
							work += visit(cell.Blocks)
						}
					}
				}
			case layout.NoteBoxBlock:
				work += visit(block.Body)
			case layout.SectionBlock:
				work += visit(block.Blocks)
			case layout.ClauseBlock:
				work += visit(block.Blocks)
			case layout.RowColumnBlock:
				for _, item := range block.Items {
					work += visit([]layout.Block{item.Block})
				}
			}
		}
		return work
	}
	return visit(doc.Body)
}

func characterizationPageRegionDocument() *layout.LayoutDocument {
	p := characterizationParagraph("body")
	return &layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: 5, Right: 5, Bottom: 5, Left: 5},
		Header: &layout.HeaderBlock{Blocks: []layout.Block{characterizationParagraph("odd")}}, FirstPageHeader: &layout.HeaderBlock{Blocks: []layout.Block{characterizationParagraph("first")}},
		Footer: &layout.FooterBlock{Blocks: []layout.Block{characterizationParagraph("footer")}}, EvenPageFooter: &layout.FooterBlock{Blocks: []layout.Block{characterizationParagraph("even")}},
		PageNumbers: layout.PageNumberOptions{Enabled: true}}, Body: []layout.Block{p, layout.PageBreakBlock{After: true}, p, layout.PageBreakBlock{After: true}, p}}
}

func typedFixtureInventory() []TypedFixtureInventory {
	fixtures := typedCharacterizationFixtures()
	result := make([]TypedFixtureInventory, len(fixtures))
	for index := range fixtures {
		result[index] = fixtures[index].inventory
	}
	return result
}
