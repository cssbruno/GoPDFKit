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
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "615dc1ab3bee7dd52213941e46ea6025f63accdd437da027493dd262c0d56bac"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "f623d6d05d7215ea51c4af070eff6f2837597efe257fc38aac4153a6db6d32cc"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "7470283a444191dac3b142e3cc3ef491f6c6dff5fb1c71cb51062494c761bc70"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "4eb5e2a22d28dc24e8a3729eb564c74f3bc99af57440b1ac9368de06915881fb"},
		{name: "statement", doc: goldenStatementDocument(), want: "0ef4047cd108eea7fb4a0d09d8481e670ff301d3ac59a32414410c088b176b9a"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "5dea1e2e71dcefb6d43af3d7fe40af1064b2320359715029931e61a5aca61e6d"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "e2a299fb4d988378625d6cd7f8a2d3fd8bc8ce18b353801e58a253bd2ee23fb1"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "a84758aa1336b6465602b38a2b60063adb2fa29b01b903b733f616354998968b"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "b778cd93543d0cd78ab101fc72b2051e31bdc38c0d11bae22367164a0b34feba"},
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
	doc := NewLayoutDocument()
	doc.Title = "Structured Report"
	doc.PageTemplate.Header = &HeaderBlock{Height: 8, Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Structured Header"}}, Style: TextStyle{FontSize: 9}}}}
	doc.PageTemplate.Footer = &FooterBlock{Height: 8, ReservePageArea: true}
	doc.PageTemplate.PageNumbers = PageNumberOptions{Enabled: true, TotalPageAlias: "{total}"}
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Structured Report"}}},
		MetadataGridBlock{Fields: []MetadataField{{Label: "ID", Value: "SR-001"}, {Label: "Status", Value: "Final"}}},
		SectionBlock{Title: "Summary", Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "A deterministic structured report."}}}}},
	}
	return doc
}

func goldenTabularReportDocument() *LayoutDocument {
	doc := NewLayoutDocument()
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
	doc := NewLayoutDocument()
	doc.Title = "Transaction"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Transaction Receipt"}}},
		MetadataGridBlock{Fields: []MetadataField{{Label: "Reference", Value: "TX-001"}, {Label: "Amount", Value: "100.00"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "Transaction completed."}}},
	}
	return doc
}

func goldenAttestationDocument() *LayoutDocument {
	doc := NewLayoutDocument()
	doc.Title = "Attestation"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Attestation"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "This attests that the described facts are recorded."}}},
	}
	doc.Signature = &SignatureBlock{Rows: []SignatureRowBlock{{Columns: []SignatureColumn{{Label: "Responsible", Name: "A. Example"}}}}}
	return doc
}

func goldenStatementDocument() *LayoutDocument {
	doc := NewLayoutDocument()
	doc.Title = "Statement"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Statement"}}},
		NoteBoxBlock{Title: "Notice", Body: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "This is a deterministic statement."}}}}},
	}
	return doc
}

func goldenGenericDocument() *LayoutDocument {
	doc := NewLayoutDocument()
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
	doc := NewLayoutDocument()
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
