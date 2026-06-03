// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import "github.com/cssbruno/gopdfkit/document"

// Document is the high-level PDF document API exposed by the document package.
type Document = document.Document

// InitType customizes a new Document.
type InitType = document.InitType

// New returns a new PDF document. With no arguments it uses the document
// package defaults. The four-argument form preserves the legacy convenience
// facade while the implementation lives in document.New.
func New(args ...string) *Document {
	if len(args) == 0 {
		return document.New("", "", "", "")
	}
	orientation, unit, size, fontDir := "", "", "", ""
	if len(args) > 0 {
		orientation = args[0]
	}
	if len(args) > 1 {
		unit = args[1]
	}
	if len(args) > 2 {
		size = args[2]
	}
	if len(args) > 3 {
		fontDir = args[3]
	}
	return document.New(orientation, unit, size, fontDir)
}

// NewCustom returns a new PDF document using the document package initializer.
func NewCustom(init *InitType) *Document {
	return document.NewCustom(init)
}
