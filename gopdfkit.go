// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import "github.com/cssbruno/gopdfkit/document"

// Document is the high-level PDF document API exposed by the document package.
type Document = document.Document

// Options customizes a new Document.
type Options = document.Options

// New returns a new PDF document using the document package defaults.
func New() *Document {
	return document.New("", "", "", "")
}

// NewWithOptions returns a new PDF document using explicit construction options.
func NewWithOptions(options Options) *Document {
	return document.NewWithOptions(options)
}
