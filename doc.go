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

Use NewDocument when generation defaults should be configured at construction
time and constructor errors should be returned directly:

	pdf, err := gopdfkit.NewDocument(
	    gopdfkit.WithOrientation(gopdfkit.OrientationPortrait),
	    gopdfkit.WithUnit(gopdfkit.UnitMillimeter),
	    gopdfkit.WithPageSize(gopdfkit.PageSizeA4),
	    gopdfkit.WithBestCompression(),
	)

WithBestCompression selects best zlib compression for generated page and
template streams. It is not a full PDF optimizer for images, fonts, object
streams, or arbitrary existing PDFs.

Use document.NewWithDefaults when compression, catalog ordering, or fixed
metadata dates should be explicit for one document instead of inherited from
package-wide defaults:

	defaults := document.DefaultSettings()
	defaults.Compression = false
	pdf := document.NewWithDefaults(document.Options{SizeStr: "Letter"}, defaults)

# Current Capabilities

The document package supports page generation, text, cells, multicells, tables,
headers, footers, drawing primitives, clipping, transforms, transparency,
gradients, spot colors, layers, templates, imported PDF pages, images, SVG,
WebP, controlled HTML fragments, thumbnails, attachments, metadata, XMP
metadata, JavaScript actions, password protection, PDF signing, and signature
verification through CMS SignedData. The inspect package provides lightweight
PDF structure, stream, page, and literal text inspection helpers.
Shared layout document models are domain-neutral; application-specific
document categories should be modeled by callers using layout blocks and
document.WriteDocument.

Imported-page examples cover merge, split, reorder, rotate, 4-up layout,
template overlay, and watermark overlay by creating a new PDF from imported
pages.

# Current Limits

GoPDFKit does not implement full browser HTML/CSS layout, JavaScript page
rendering, DOCX conversion, interactive AcroForm field creation, filling or
flattening existing interactive forms, FDF merging, unlocking/decrypting
existing password-protected PDFs, OCR, arbitrary PDF content rewriting, or
general-purpose semantic text extraction from every possible PDF encoding.

Imported page support is intentionally narrow: classic xref-table PDFs,
unencrypted documents, and pages whose content streams are unfiltered or
FlateDecode-compressed. PDFs that use xref streams or object streams are
reported as unsupported.

# Packages

The main packages are:

  - gopdfkit: root facade with the default constructor and public aliases.
  - document: main PDF generation API.
  - layout: renderer-independent structured document model types.
  - font: font parsing and JSON font definition generation.
  - importpdf: small wrappers around imported-page APIs.
  - inspect: lightweight PDF structure, stream, page, and text inspection.
  - sign: CMS-first PDF signing and signature verification.
  - sign/pkcs7: legacy PKCS #7 terminology wrappers around CMS APIs.

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
