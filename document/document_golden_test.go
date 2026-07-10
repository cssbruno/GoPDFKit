// SPDX-License-Identifier: MIT
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
