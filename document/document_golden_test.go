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
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "930141a3ef5eac55889db442d0f6a61523e5a2c431ab80f98828aeb3bbc38754"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "fedb7af8aef8bf85a71a19f40eb7fcc2ce3b10b2c9e1120e7c4367f22be57cae"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "d0441843abf1f0408572d11f108633058dd083cf2b02a807fdff992efbf476fc"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "3d9c363a116b2c8bf6a7da70362fc4567909ac35eebb0afd03a45d186610ea3e"},
		{name: "statement", doc: goldenStatementDocument(), want: "a5d587d53d55eb3bf83c2085839a4090865cdc60d87d0dc3f003eadf1ed72d75"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "95accc322c7bb51ac69b6c21f614feb1ec834b93aae4e6a1fdc02250ef1252fe"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "94a6ebbc5e0dd076706eb0f65a606f98e5e2dfdd5732887a21629a8ed4dc460d"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "9809b8d113f7638ffc7467b2a5c69ae0d941f302a7a54ce0db8d4e61bb1886b3"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "a23f8a5ffda13573e9b9e00b8f767fb28253f7deb444a1caa6b54f1decf9d872"},
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
