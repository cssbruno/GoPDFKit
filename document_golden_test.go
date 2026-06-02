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
		{name: "structured-report", doc: goldenStructuredReportDocument(), want: "2f28c21b820942ee819169fd4f3b22de5fc5515e3e0719d9e268e49fddc455b1"},
		{name: "tabular-report", doc: goldenTabularReportDocument(), want: "bfbb0e5b1b72ed3b9041a4a530acff9290b27e2cdc2be93de461374b3ed7d842"},
		{name: "transactional", doc: goldenTransactionalDocument(), want: "8d3702d0c66b92e14a972aed5f66fc72484cddda2f7b206092540bea8fe2e63b"},
		{name: "attestation", doc: goldenAttestationDocument(), want: "bb8be6b3ce95f0e25436575eaad1a3081a4ef56cece7d07d5908e26e22e5d3f9"},
		{name: "statement", doc: goldenStatementDocument(), want: "1fba557ba7d8b462a2ff1245a38e75e92bb03a691a9d6e07126b88567803d0dc"},
		{name: "generic-free-text", doc: goldenGenericDocument(), want: "5a423415375e0230c3b011b0df65320a15ab16f151d0ca97748828df3b58b7df"},
		{name: "long-form", doc: goldenLongFormDocument(), want: "538aa869b1c418452301895a0ebd64f452e234991fc69d2cf98c8f115ff05ac5"},
		{name: "form", doc: FormDocumentModel(testFormDocument()), want: "c8c960befebe048286eb202e1f21dd4a6517ab607a585f528f465e2ae0a4910d"},
		{name: "qr-signature", doc: goldenQRSignatureDocument(), want: "ce09836366e8f91eb99b7a73d49832c3ceba4b97c1695bb19e978c93d7ce0826"},
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
	pdf.SetProducer("gopdfkit golden", false)
	pdf.SetCreator("gopdfkit golden test", false)
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
