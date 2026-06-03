// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// NewReportDocument builds a structured report document model.
func NewReportDocument(title string, blocks ...Block) *LayoutDocument {
	return newKindDocument(DocumentKindReport, title, blocks...)
}

// NewTransactionalDocument builds a transactional document model.
func NewTransactionalDocument(title string, blocks ...Block) *LayoutDocument {
	return newKindDocument(DocumentKindTransactional, title, blocks...)
}

// NewAttestationDocument builds an attestation-style document model.
func NewAttestationDocument(title string, blocks ...Block) *LayoutDocument {
	return newKindDocument(DocumentKindAttestation, title, blocks...)
}

// NewStatementDocument builds a statement-style document model.
func NewStatementDocument(title string, blocks ...Block) *LayoutDocument {
	return newKindDocument(DocumentKindStatement, title, blocks...)
}

// NewGenericDocument builds a generic/free-text document model.
func NewGenericDocument(title string, blocks ...Block) *LayoutDocument {
	return newKindDocument(DocumentKindGeneric, title, blocks...)
}

func newKindDocument(kind DocumentKind, title string, blocks ...Block) *LayoutDocument {
	doc := NewLayoutDocument(kind)
	doc.Title = title
	if title != "" {
		doc.Body = append(doc.Body, HeadingBlock{Level: 1, Segments: []TextSegment{{Text: title}}})
	}
	doc.Body = append(doc.Body, blocks...)
	return doc
}
