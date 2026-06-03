// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"io"

	"github.com/cssbruno/gopdfkit/document"
)

// Page imports a page from sourceFile into pdf and returns its page ID.
func Page(pdf *document.Document, sourceFile string, pageNo int, box string) int {
	return pdf.ImportPage(sourceFile, pageNo, box)
}

// PageStream imports a page from source into pdf and returns its page ID.
func PageStream(pdf *document.Document, source io.Reader, pageNo int, box string) int {
	return pdf.ImportPageStream(source, pageNo, box)
}

// UsePage draws an imported page on pdf.
func UsePage(pdf *document.Document, pageID int, x, y, w, h float64) {
	pdf.UseImportedPage(pageID, x, y, w, h)
}
