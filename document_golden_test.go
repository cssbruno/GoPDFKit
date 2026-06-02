// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

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
		doc  *Document
		want string
	}{
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "04e468743224cb0fc6f4513ff3c7dcd1cea10f9a5c674ecfb0d420dd3bd5dcc3"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "ec19e0e288e14a686f270fba3878a5ff3b6ebf3c376daca6ffe49ea538d02acb"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "3a7e096c9710594cb462da3af26cd587ed685fa76c76d7be668dc6a881ba9349"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "1a203cac11285249217db6c1b2e732a082824269ab287ddce1f239d94afd0934"},
		{name: "statement", doc: goldenStatementDocument(), want: "ed86891efb61d0114d484ad39cebe58af876eaf11ce88ecf5e86ac23a9ddd186"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "1fc3ac087ca3c0aa4df180e1c0151722e708ade960fc3dd9152a39bf0d2c1e79"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "fbe4d2eda38fc8b7c314d4330b1b8257a41fcdb5cbb046c74d6746ad02110c51"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "b2f392032311b9d9d1f0b8bf11464c433661664f5ff4e89aaea87ede4c4dc1ca"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "5f30f6ada0f10f325123e04be7044aad21e7020c15e0c98db32e8efebe211c77"},
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

func goldenDocumentPDFSHA(t *testing.T, doc *Document) string {
	t.Helper()
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetCatalogSort(true)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pdf.SetCreationDate(fixed)
	pdf.SetModificationDate(fixed)
	pdf.SetProducer("GoPDFKit golden", false)
	pdf.SetCreator("GoPDFKit golden test", false)
	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(out.Bytes()))
}

func goldenStructuredReportDocument() *Document {
	doc := NewDocument(DocumentKindReport)
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

func goldenTabularReportDocument() *Document {
	doc := NewDocument(DocumentKindReport)
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

func goldenTransactionalDocument() *Document {
	doc := NewDocument(DocumentKindTransactional)
	doc.Title = "Transaction"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Transaction Receipt"}}},
		MetadataGridBlock{Fields: []MetadataField{{Label: "Reference", Value: "TX-001"}, {Label: "Amount", Value: "100.00"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "Transaction completed."}}},
	}
	return doc
}

func goldenAttestationDocument() *Document {
	doc := NewDocument(DocumentKindAttestation)
	doc.Title = "Attestation"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Attestation"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "This attests that the described facts are recorded."}}},
	}
	doc.Signature = &SignatureBlock{Rows: []SignatureRowBlock{{Columns: []SignatureColumn{{Label: "Responsible", Name: "A. Example"}}}}}
	return doc
}

func goldenStatementDocument() *Document {
	doc := NewDocument(DocumentKindStatement)
	doc.Title = "Statement"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Statement"}}},
		NoteBoxBlock{Title: "Notice", Body: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "This is a deterministic statement."}}}}},
	}
	return doc
}

func goldenGenericDocument() *Document {
	doc := NewDocument(DocumentKindGeneric)
	doc.Title = "Generic"
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Generic Document"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "Free text content for generic rendering."}}},
	}
	return doc
}

func goldenLongFormDocument() *Document {
	doc, messages := LongFormHTMLDocumentModel("Long Form", `<h2>Clause</h2><p>Long-form text.</p><footer>Long footer</footer>`)
	if len(messages) != 0 {
		panic(fmt.Sprintf("unexpected long-form diagnostics: %#v", messages))
	}
	return doc
}

func goldenQRSignatureDocument() *Document {
	doc := NewDocument(DocumentKindStatement)
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
