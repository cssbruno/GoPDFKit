// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

/*
Package gopdfkit is a small facade over GoPDFKit's PDF generation API.

GoPDFKit generates PDFs directly from Go code. The root package exposes the
default constructor and public aliases. Import github.com/cssbruno/gopdfkit/document
for the full API.

# Quick Start

	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(40, 10, "Hello, world")
	err := pdf.OutputFileAndClose("hello.pdf")

Use document.NewWithOptions when generation defaults should be configured at
construction time:

	pdf := document.NewWithOptions(document.Options{
	    OrientationStr: "P",
	    UnitStr:        "mm",
	    SizeStr:        "A4",
	    Optimize:       true,
	})

Optimize selects best zlib compression for generated page and template streams.
It is not a full PDF optimizer for images, fonts, object streams, or arbitrary
existing PDFs.

# Current Capabilities

The document package supports page generation, text, cells, multicells, tables,
headers, footers, drawing primitives, clipping, transforms, transparency,
gradients, spot colors, layers, templates, imported PDF pages, images, SVG,
WebP, controlled HTML fragments, thumbnails, attachments, metadata, XMP
metadata, JavaScript actions, password protection, PDF signing, and signature
verification.

Imported-page examples cover merge, split, reorder, rotate, 4-up layout,
template overlay, and watermark overlay by creating a new PDF from imported
pages.

# Current Limits

GoPDFKit does not implement full browser HTML/CSS layout, JavaScript page
rendering, DOCX conversion, interactive AcroForm field creation, filling or
flattening existing interactive forms, FDF merging, unlocking/decrypting
existing password-protected PDFs, OCR, text extraction, or arbitrary PDF
content rewriting.

Imported page support is intentionally narrow: classic xref-table PDFs,
unencrypted documents, and pages whose content streams are unfiltered or
FlateDecode-compressed. PDFs that use xref streams or object streams are
reported as unsupported.

# Packages

The main packages are:

  - gopdfkit: root facade with the default constructor and public aliases.
  - document: main PDF generation API.
  - font: font parsing and JSON font definition generation.
  - importpdf: small wrappers around imported-page APIs.
  - sign: PDF signing and signature verification.

# Examples

Runnable examples live under examples/ and write PDFs under
assets/generated/pdf/examples. Focused examples include reports, table reports,
invoices, styled paragraphs, HTML/CSS styling, manual pagination,
document-model pagination, images, compression, watermarks, merge/split/
reorder/rotate page workflows, 4-up pages, template overlays, static form
documents, password protection, templates, thumbnails, UTF-8 fonts, and signing.

# HTML Support

HTMLNew renders a controlled subset of HTML fragments into PDF drawing
operations. It is useful for rich text, generated sections, reports, letters,
and static forms. Supported CSS maps to PDF operations for text, colors,
spacing, borders, border radius, backgrounds, simple box shadows, line height,
page breaks, and table layout. It is not a browser engine.

Use document.RenderHTMLTemplate for simple {{key}} substitution before
rendering HTML. Plain values are escaped, HTMLTemplateRaw inserts trusted HTML,
and HTMLTemplateImage inserts an img tag that can be sized and spaced with
supported HTML/CSS.

See doc/pdf-html-subset.md for the full renderer contract.

# Errors

Most Document methods record errors on the document instead of returning errors
directly. Once an error is set, later methods usually return without changing
the PDF. Check Ok, Err, or Error after generation, especially before trusting
output.

# Background

GoPDFKit is derived from the original FPDF PHP library and keeps many
FPDF-style method names for compatibility and familiarity. Internally, this Go
version uses buffers, io.Writer/io.WriteCloser output, and JSON font definition
files.
*/
package gopdfkit
