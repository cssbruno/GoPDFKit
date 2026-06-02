// Copyright (c) 2026 cssBruno

package gopdfkit

// NewReportDocument builds a structured report document model.
func NewReportDocument(title string, blocks ...Block) *Document {
	return newKindDocument(DocumentKindReport, title, blocks...)
}

// NewTransactionalDocument builds a transactional document model.
func NewTransactionalDocument(title string, blocks ...Block) *Document {
	return newKindDocument(DocumentKindTransactional, title, blocks...)
}

// NewAttestationDocument builds an attestation-style document model.
func NewAttestationDocument(title string, blocks ...Block) *Document {
	return newKindDocument(DocumentKindAttestation, title, blocks...)
}

// NewStatementDocument builds a statement-style document model.
func NewStatementDocument(title string, blocks ...Block) *Document {
	return newKindDocument(DocumentKindStatement, title, blocks...)
}

// NewGenericDocument builds a generic/free-text document model.
func NewGenericDocument(title string, blocks ...Block) *Document {
	return newKindDocument(DocumentKindGeneric, title, blocks...)
}

func newKindDocument(kind DocumentKind, title string, blocks ...Block) *Document {
	doc := NewDocument(kind)
	doc.Title = title
	if title != "" {
		doc.Body = append(doc.Body, HeadingBlock{Level: 1, Segments: []TextSegment{{Text: title}}})
	}
	doc.Body = append(doc.Body, blocks...)
	return doc
}
