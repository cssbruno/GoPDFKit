// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package html

import "github.com/cssbruno/gopdfkit/document"

// Renderer writes supported HTML/CSS into a PDF document.
type Renderer = document.HTML

// NewRenderer returns a renderer bound to pdf.
func NewRenderer(pdf *document.Document) Renderer {
	return pdf.HTMLNew()
}
