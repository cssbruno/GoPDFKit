// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestWriteDocumentGoldenPDFs(t *testing.T) {
	cases := []struct {
		name string
		doc  *LayoutDocument
		want string
	}{
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "aa21ba0157fce52acbad7b5804da62ccbda4efa2135b5ff27f238e0448ebb9d0"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "cac30204cc5bb416f5bed5c00d9bb6c6570df845e86135575ca6711d11582af5"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "41eab88324d00f9d738522d3b172e8a6464218290bad3352c264da42ea6945f3"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "0c782dbd66f58e11d385a620fd695e1aa6a2e724649395314240704ab0449e8e"},
		{name: "statement", doc: goldenStatementDocument(), want: "96dd689d8998b320c79a73c8e95763cbf03ff691ba68750421f62273abd63d55"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "7ec99cb7cf2a73ec2c56eb103f69a86587f4a9880c2340a5e860b401114fe8f3"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "5574646134671082be0e30a09c7b4755edc7118a3d4dd114d0995df3810a7076"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "eeadec653874d11a96d76f66ba075cfce68f8e3ec67faa05887e2ff051c329ee"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "570a92edc56f4691f8f6bd9b3d3060039ddd22ce842c2c63bbc243731c7d1d89"},
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

func goldenDocumentPDFSHA(t *testing.T, doc *LayoutDocument) string {
	t.Helper()
	pdf := New("P", "mm", "A4", "")
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

func goldenStructuredReportDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindReport)
	doc.Title = "Structured Report"
	doc.Header = &HeaderBlock{Height: 8, Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Structured Header"}}, Style: TextStyle{FontSize: 9}}}}
	doc.Footer = &FooterBlock{Height: 8, ShowPageNumber: true, TotalPageAlias: "{total}", ReservePageArea: true}
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Structured Report"}}},
		MetadataGridBlock{Fields: []MetadataField{{Label: "ID", Value: "SR-001"}, {Label: "Status", Value: "Final"}}},
		SectionBlock{Title: "Summary", Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "A deterministic structured report."}}}}},
	}
	return doc
}

func goldenTabularReportDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindReport)
	doc.Title = "Tabular Report"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Tabular Report"}}},
		TableBlock{
			Caption: "Metrics",
			Header:  []TableRow{{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Metric"}}}}}, {Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Value"}}}}}}}},
			Body: []TableRow{
				{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Alpha"}}}}}, {Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "10"}}}}}}},
				{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Beta"}}}}}, {Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "20"}}}}}}},
			},
		},
	}
	return doc
}

func goldenTransactionalDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindTransactional)
	doc.Title = "Transaction"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Transaction Receipt"}}},
		MetadataGridBlock{Fields: []MetadataField{{Label: "Reference", Value: "TX-001"}, {Label: "Amount", Value: "100.00"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "Transaction completed."}}},
	}
	return doc
}

func goldenAttestationDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindAttestation)
	doc.Title = "Attestation"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Attestation"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "This attests that the described facts are recorded."}}},
	}
	doc.Signature = &SignatureBlock{Rows: []SignatureRowBlock{{Columns: []SignatureColumn{{Label: "Responsible", Name: "A. Example"}}}}}
	return doc
}

func goldenStatementDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindStatement)
	doc.Title = "Statement"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Statement"}}},
		NoteBoxBlock{Title: "Notice", Body: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "This is a deterministic statement."}}}}},
	}
	return doc
}

func goldenGenericDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.Title = "Generic"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Generic Document"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "Free text content for generic rendering."}}},
	}
	return doc
}

func goldenLongFormDocument() *LayoutDocument {
	doc, messages := LongFormHTMLDocumentModel("Long Form", `<h2>Clause</h2><p>Long-form text.</p><footer>Long footer</footer>`)
	if len(messages) != 0 {
		panic(fmt.Sprintf("unexpected long-form diagnostics: %#v", messages))
	}
	return doc
}

func goldenQRSignatureDocument() *LayoutDocument {
	doc := NewLayoutDocument(DocumentKindStatement)
	doc.Title = "QR Signature"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "QR Signature"}}},
		QRVerificationBlock{QR: QRBlock{Label: "Verify", URL: "https://example.test/verify/1", Size: 22}},
	}
	doc.Signature = &SignatureBlock{Rows: []SignatureRowBlock{{Columns: []SignatureColumn{
		{Label: "Primary", Name: "A. Example", Role: "Signer"},
		{Label: "Secondary", Name: "B. Example", Role: "Reviewer"},
	}}}}
	return doc
}
