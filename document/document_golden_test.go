// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/layout"
)

func TestWriteDocumentGoldenPDFs(t *testing.T) {
	cases := []struct {
		name string
		doc  *layout.LayoutDocument
		want string
	}{
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "9e08fd0a467a7fab1658e1df34833c220788df9811b5fa84d835151e9ac4c780"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "af0d2bb5891e6521151bb45bed4cd92fab359bad7fa23a8090e41839d314c142"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "555d9422fd17d913c981a020957349c141e06082daf245c3932d7385bc8131b2"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "5f9076f92a5d4dbd5361cefedb4f4d8d5426d9f33f407bc263c54d9e8eb17ea0"},
		{name: "statement", doc: goldenStatementDocument(), want: "7356c9217ebb1734347cd6853c3dd6feb8c0cca76986b1bfbf7e784815e0b293"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "8ca4af399d06f0297dc54d9c5c47ef4d515fd49f651e7df354c916bf085e1977"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "9d456d67212fe02ef3a9965d31668603dec818180b3a64caf0a326a2f18330d7"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "b124b2d03eb2968efcaa797de152f9a57d85dc8d53b000c14e7653c00c075875"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "f39ecb17bcd75388c9b85d507c2b14429183eb50f1ab5024fac1bfb410994aac"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := goldenDocumentPDFSHA(t, tc.doc)
			if got != tc.want {
				t.Fatalf("golden SHA = %s, want %s", got, tc.want)
			}
		})
	}
}

func goldenDocumentPDFSHA(t *testing.T, doc *layout.LayoutDocument) string {
	t.Helper()
	pdf := MustNew()
	pdf.SetCompression(false)
	pdf.SetCatalogSort(true)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pdf.SetCreationDate(fixed)
	pdf.SetModificationDate(fixed)
	pdf.SetProducer("Document golden", false)
	pdf.SetCreator("Document golden test", false)
	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(out.Bytes()))
}

func goldenStructuredReportDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "Structured Report"
	doc.PageTemplate.Header = &layout.HeaderBlock{Height: 8, Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Structured Header"}}, Style: layout.TextStyle{FontSize: 9}}}}
	doc.PageTemplate.Footer = &layout.FooterBlock{Height: 8, ReservePageArea: true}
	doc.PageTemplate.PageNumbers = layout.PageNumberOptions{Enabled: true, TotalPageAlias: "{total}"}
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Structured Report"}}},
		layout.MetadataGridBlock{Fields: []layout.MetadataField{{Label: "ID", Value: "SR-001"}, {Label: "Status", Value: "Final"}}},
		layout.SectionBlock{Title: "Summary", Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "A deterministic structured report."}}}}},
	}
	return doc
}

func goldenTabularReportDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "Tabular Report"
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Tabular Report"}}},
		layout.TableBlock{
			Caption: "Metrics",
			Header:  []layout.TableRow{{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Metric"}}}}}, {Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Value"}}}}}}}},
			Body: []layout.TableRow{
				{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Alpha"}}}}}, {Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "10"}}}}}}},
				{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Beta"}}}}}, {Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "20"}}}}}}},
			},
		},
	}
	return doc
}

func goldenTransactionalDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "Transaction"
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Transaction Receipt"}}},
		layout.MetadataGridBlock{Fields: []layout.MetadataField{{Label: "Reference", Value: "TX-001"}, {Label: "Amount", Value: "100.00"}}},
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Transaction completed."}}},
	}
	return doc
}

func goldenAttestationDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "Attestation"
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Attestation"}}},
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "This attests that the described facts are recorded."}}},
	}
	doc.Signature = &layout.SignatureBlock{Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Label: "Responsible", Name: "A. Example"}}}}}
	return doc
}

func goldenStatementDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "Statement"
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Statement"}}},
		layout.NoteBoxBlock{Title: "Notice", Body: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "This is a deterministic statement."}}}}},
	}
	return doc
}

func goldenGenericDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "Generic"
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Generic Document"}}},
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Free text content for generic rendering."}}},
	}
	return doc
}

func goldenLongFormDocument() *layout.LayoutDocument {
	doc, messages := LongFormHTMLDocumentModel("Long Form", `<h2>Clause</h2><p>Long-form text.</p><footer>Long footer</footer>`)
	if len(messages) != 0 {
		panic(fmt.Sprintf("unexpected long-form diagnostics: %#v", messages))
	}
	return doc
}

func goldenQRSignatureDocument() *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = "QR Signature"
	doc.Body = []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "QR Signature"}}},
		layout.QRVerificationBlock{QR: layout.QRBlock{Label: "Verify", URL: "https://example.test/verify/1", Size: 22}},
	}
	doc.Signature = &layout.SignatureBlock{Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{
		{Label: "Primary", Name: "A. Example", Role: "Signer"},
		{Label: "Secondary", Name: "B. Example", Role: "Reviewer"},
	}}}}
	return doc
}
