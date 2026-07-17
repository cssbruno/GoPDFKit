// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/layout"
)

func TestWriteDocumentGoldenPDFs(t *testing.T) {
	cases := []struct {
		name string
		doc  *layout.LayoutDocument
		want string
	}{
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "9e08fd0a467a7fab1658e1df34833c220788df9811b5fa84d835151e9ac4c780"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "af0d2bb5891e6521151bb45bed4cd92fab359bad7fa23a8090e41839d314c142"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "d9ca94a76c06a2bde1bf0475b7b3a856636d3e8f3a625b0dce707c8a064647be"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "6676f027e0652101d443c66ded3e395631b43e849cb5ac6bc0c23db7e120f86d"},
		{name: "statement", doc: goldenStatementDocument(), want: "7643877fffe620edd7ecaf6c02d7d445990ddd8d5f754a5821494280cd310aec"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "f6095689238721c21ca0624833ac44e1fd37e0041a0dac4bbf48acbaf4cc4e8d"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "ce9872880bca539a3ac7ee2ddc96af58b79ad52bc6bff1a234490fb3a3827c05"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "b124b2d03eb2968efcaa797de152f9a57d85dc8d53b000c14e7653c00c075875"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "f1b172763a2dcabaafb89a6a430911e44dc5a02a77f95d5a59900e15f57b8d15"},
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
