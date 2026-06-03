// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

import "github.com/cssbruno/gopdfkit/document"

// Document is the structured report/form model rendered by document.Document.
type Document = document.LayoutDocument

// Kind identifies the high-level purpose of a generated document.
type Kind = document.DocumentKind

const (
	Generic       Kind = document.DocumentKindGeneric
	Report        Kind = document.DocumentKindReport
	Form          Kind = document.DocumentKindForm
	Letter        Kind = document.DocumentKindLetter
	Transactional Kind = document.DocumentKindTransactional
	Attestation   Kind = document.DocumentKindAttestation
	Statement     Kind = document.DocumentKindStatement
	LongForm      Kind = document.DocumentKindLongForm
)

// New creates a structured layout document.
func New(kind Kind) *Document {
	return document.NewLayoutDocument(kind)
}
