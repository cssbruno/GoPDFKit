// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"strings"
	"testing"
)

func TestDocumentKindBuilders(t *testing.T) {
	cases := []struct {
		name string
		doc  *Document
		kind DocumentKind
	}{
		{name: "report", doc: NewReportDocument("Report", ParagraphBlock{Segments: []TextSegment{{Text: "Report body"}}}), kind: DocumentKindReport},
		{name: "transactional", doc: NewTransactionalDocument("Transaction", ParagraphBlock{Segments: []TextSegment{{Text: "Transaction body"}}}), kind: DocumentKindTransactional},
		{name: "attestation", doc: NewAttestationDocument("Attestation", ParagraphBlock{Segments: []TextSegment{{Text: "Attestation body"}}}), kind: DocumentKindAttestation},
		{name: "statement", doc: NewStatementDocument("Statement", ParagraphBlock{Segments: []TextSegment{{Text: "Statement body"}}}), kind: DocumentKindStatement},
		{name: "generic", doc: NewGenericDocument("Generic", ParagraphBlock{Segments: []TextSegment{{Text: "Generic body"}}}), kind: DocumentKindGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.doc.Kind != tc.kind {
				t.Fatalf("kind = %q, want %q", tc.doc.Kind, tc.kind)
			}
			pdf := New("P", "mm", "A4", "")
			pdf.SetCompression(false)
			pdf.WriteDocument(tc.doc)
			var out bytes.Buffer
			if err := pdf.Output(&out); err != nil {
				t.Fatalf("Output() error = %v", err)
			}
			if !strings.Contains(out.String(), tc.doc.Title) {
				t.Fatalf("PDF output missing title %q", tc.doc.Title)
			}
		})
	}
}
