// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestLowerLayoutDocumentTreeCoversSupportedTypedPrimitiveFamiliesDeterministically(t *testing.T) {
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}}
	}
	table := layout.TableBlock{
		Caption: "Ledger", Columns: []layout.TableColumn{{Width: 80}, {Width: 80}}, Style: layout.TableStyle{RepeatHeader: true},
		Header: []layout.TableRow{{Cells: []layout.TableCell{{Header: true, Blocks: []layout.Block{paragraph("H1")}}, {Header: true, Blocks: []layout.Block{paragraph("H2")}}}}},
		Body:   []layout.TableRow{{KeepTogether: true, Cells: []layout.TableCell{{Blocks: []layout.Block{paragraph("A")}}, {Blocks: []layout.Block{paragraph("B")}}}}},
	}
	doc := &layout.LayoutDocument{
		Title: "Canonical typed", PageTemplate: layout.PageTemplate{
			Header:          &layout.HeaderBlock{Blocks: []layout.Block{paragraph("header")}},
			FirstPageHeader: &layout.HeaderBlock{Blocks: []layout.Block{paragraph("first header")}},
			Footer:          &layout.FooterBlock{Blocks: []layout.Block{paragraph("footer")}},
			FirstPageFooter: &layout.FooterBlock{Blocks: []layout.Block{paragraph("first footer")}},
			EvenPageFooter:  &layout.FooterBlock{Blocks: []layout.Block{paragraph("even footer")}},
			PageNumbers:     layout.PageNumberOptions{Enabled: true, Format: "Page %d/{total}"},
		},
		Body: []layout.Block{
			paragraph("paragraph"),
			layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: "heading"}}},
			layout.ListBlock{Ordered: true, Items: []layout.ListItem{{Blocks: []layout.Block{paragraph("item")}}}},
			layout.SectionBlock{Title: "section", Blocks: []layout.Block{layout.ClauseBlock{Number: "1", Title: "clause", Blocks: []layout.Block{layout.NoteBoxBlock{Title: "note", Body: []layout.Block{paragraph("inside")}}}}}},
			layout.MetadataGridBlock{Columns: 2, Fields: []layout.MetadataField{{Label: "ID", Value: "7"}}},
			layout.ImageBlock{Data: []byte("image"), Format: "png", Alt: "mark", Width: 10, Height: 10},
			layout.QRVerificationBlock{QR: layout.QRBlock{Value: "verify", Label: "QR"}, Text: []layout.TextSegment{{Text: "verify text"}}},
			table,
			layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{Block: paragraph("row item"), Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: 1}}}},
			layout.PageBreakBlock{After: true},
		},
		Signature:   &layout.SignatureBlock{PlaceholderReference: "Sig", Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Label: "Signer", Width: 80}}}}},
		QR:          &layout.QRBlock{Value: "document qr", Label: "Document QR"},
		Attachments: []layout.AttachmentBlock{{Name: "proof.txt", MIMEType: "text/plain", Description: "proof", Data: []byte("proof")}},
	}
	first, err := LowerLayoutDocumentTreeContext(context.Background(), doc, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := LowerLayoutDocumentTreeContext(context.Background(), doc, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := first.CanonicalJSON()
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("typed canonical tree is not deterministic")
	}
	projection := first.Projection()
	kinds := make(map[string]int)
	for _, node := range projection.Nodes {
		kinds[projection.Strings[node.Kind-1]]++
		if node.ID == 0 || node.Key == "" {
			t.Fatalf("unstable node identity: %#v", node)
		}
	}
	for _, kind := range []string{"document", "page", "region-body", "region-header", "region-header-first", "region-footer", "region-footer-first", "region-footer-even", "page-counter", "paragraph", "heading", "list", "list-item", "section", "clause", "note", "grid", "grid-cell", "image", "qr-verification", "qr", "table", "column-track", "table-row", "table-cell", "row", "track", "page-break", "signature", "signature-row", "signature-cell", "attachment"} {
		if kinds[kind] == 0 {
			t.Fatalf("typed tree lacks %q: %#v", kind, kinds)
		}
	}
	if len(projection.Styles) == 0 || len(projection.Tracks) == 0 || len(projection.Resources) < 3 || len(projection.Semantics) == 0 || len(projection.Children) == 0 {
		t.Fatalf("typed tree tables = styles %d tracks %d resources %d semantics %d edges %d", len(projection.Styles), len(projection.Tracks), len(projection.Resources), len(projection.Semantics), len(projection.Children))
	}
}

func TestLowerLayoutDocumentTreeCancellationLimitsAndCallerMutationIsolation(t *testing.T) {
	paragraph := layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "stable"}}}
	doc := &layout.LayoutDocument{Body: []layout.Block{layout.SectionBlock{Title: "root", Blocks: []layout.Block{paragraph}}}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if tree, err := LowerLayoutDocumentTreeContext(canceled, doc, layoutengine.CanonicalTreeLimits{}); !errors.Is(err, context.Canceled) || len(tree.Projection().Nodes) != 0 {
		t.Fatalf("canceled typed tree = %d nodes, %v", len(tree.Projection().Nodes), err)
	}
	limits := layoutengine.DefaultCanonicalTreeLimits()
	limits.MaxNodes = 3
	if tree, err := LowerLayoutDocumentTreeContext(context.Background(), doc, limits); !errors.Is(err, layoutengine.ErrCanonicalTreeLimit) || len(tree.Projection().Nodes) != 0 {
		t.Fatalf("limited typed tree = %d nodes, %v", len(tree.Projection().Nodes), err)
	}
	tree, err := LowerLayoutDocumentTreeContext(context.Background(), doc, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := tree.CanonicalJSON()
	doc.Body[0] = layout.SectionBlock{Title: "mutated", Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "changed"}}}}}
	after, _ := tree.CanonicalJSON()
	if !bytes.Equal(before, after) {
		t.Fatal("typed canonical tree aliases caller-owned model state")
	}
}
