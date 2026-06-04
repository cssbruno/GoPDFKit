# GoPDFKit

[![CI][badge-ci]][ci]
[![MIT licensed][badge-mit]][license]
[![GoDoc][badge-doc]][godoc]

<img src="https://raw.githubusercontent.com/cssbruno/gopdfkit/master/assets/static/image/gopher_pdf.png" alt="GoPDFKit gopher" width="160">

GoPDFKit is an MIT-licensed Go library for generating PDFs directly from Go
code. It keeps an FPDF-style API for familiar page/text/drawing workflows, with
additional helpers for HTML fragments, imported pages, signing, thumbnails, and
structured document models.

The root `gopdfkit` package is intentionally small. It exposes the default
constructor and public aliases. Import `github.com/cssbruno/gopdfkit/document`
when you need the full API.

## Install

```shell
go get github.com/cssbruno/gopdfkit@latest
```

## Quick Start

```go
package main

import "github.com/cssbruno/gopdfkit"

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(40, 10, "Hello, world")
	if err := pdf.OutputFileAndClose("hello.pdf"); err != nil {
		panic(err)
	}
}
```

Use `document.NewWithOptions` when generation defaults should be configured at
construction time:

```go
pdf := document.NewWithOptions(document.Options{
	OrientationStr: "P",
	UnitStr:        "mm",
	SizeStr:        "A4",
	Optimize:       true,
})
```

`Optimize: true` selects best zlib compression for generated page and template
streams. It is not a full PDF optimizer for images, object streams, fonts, or
arbitrary existing PDFs.

## Current Capabilities

GoPDFKit currently supports:

* PDF generation with pages, margins, headers, footers, page boxes, and page
  breaks
* Standard PDF fonts and UTF-8 TrueType fonts
* Text, cells, multicells, aligned writing, styled paragraphs, links, bookmarks,
  and aliases
* Tables, reports, invoices, static filled form documents, and shared document
  model rendering
* Drawing primitives: lines, rectangles, rounded rectangles, arcs, Bezier
  curves, polygons, paths, clipping, transforms, transparency, gradients, spot
  colors, and layers
* JPEG, PNG, GIF, WebP, SVG, data-image, and thumbnail workflows
* Controlled HTML/CSS fragment rendering through `HTMLNew`, including text
  styles, spacing, borders, border radius, backgrounds, and simple box shadows
* Templates and imported PDF pages
* Page workflows built from imported pages: merge, split, reorder, rotate,
  4-up layout, template overlay, and watermark overlay
* Attachments, metadata, XMP metadata, JavaScript actions, password protection,
  PDF signing, CMS signature verification, and lightweight PDF inspection

## Current Limits

These are not implemented as general-purpose features:

* Full browser-compatible HTML/CSS layout
* JavaScript page rendering
* DOCX conversion
* Interactive AcroForm field creation
* Filling, flattening, or FDF-merging existing interactive forms
* Unlocking, decrypting, or removing passwords from existing PDFs
* Arbitrary PDF editing, OCR, or content rewriting
* General-purpose semantic text extraction from every possible PDF encoding

Imported page support is intentionally narrow: classic xref-table PDFs,
unencrypted documents, and pages whose content streams are unfiltered or
FlateDecode-compressed. PDFs that use xref streams or object streams are
reported as unsupported.

Password protection applies to newly generated output. The permission flags are
advisory because PDF readers decide how strictly to enforce them.

## Examples

Runnable examples live under [`examples/`][examples]. They write PDFs to
`assets/generated/pdf/examples`.

| Workflow | Command | Output |
| --- | --- | --- |
| Hello world | `go run ./examples/hello-world` | `hello-world.pdf` |
| Report | `go run ./examples/report` | `gopdfkit-report.pdf` |
| Table report | `go run ./examples/table-report` | `gopdfkit-tables.pdf` |
| Invoice | `go run ./examples/invoice` | `invoice.pdf` |
| Styled paragraphs | `go run ./examples/styled-paragraphs` | `styled-paragraphs.pdf` |
| HTML fragment | `go run ./examples/html-fragment` | `html-fragment.pdf` |
| HTML CSS styles | `go run ./examples/html-css-styles` | `html-css-styles.pdf` |
| HTML images and SVG | `go run ./examples/html-images` | `html-images.pdf` |
| HTML tables | `go run ./examples/html-tables` | `html-tables.pdf` |
| HTML template values | `go run ./examples/html-template` | `html-template.pdf` |
| Manual pagination | `go run ./examples/pagination-table` | `pagination-table.pdf` |
| Document pagination | `go run ./examples/pagination-document` | `pagination-document.pdf` |
| Images | `go run ./examples/add-images-to-pages` | `images-on-pages.pdf` |
| Compression | `go run ./examples/compress-optimize-pdf` | `compressed-optimized.pdf`, `uncompressed-debug.pdf` |
| Watermark | `go run ./examples/watermark-pdf` | `watermarked.pdf` |
| Merge pages | `go run ./examples/merge-pdf-pages` | `merged-pages.pdf` |
| Split and reorder pages | `go run ./examples/split-reorder-pages` | `split-page-2.pdf`, `reordered-pages.pdf` |
| Rotate pages | `go run ./examples/rotate-pages` | `rotated-pages.pdf` |
| 4-up pages | `go run ./examples/four-up-pages` | `four-up-pages.pdf` |
| Template overlay | `go run ./examples/template-overlay` | `template-overlay.pdf` |
| Static form document | `go run ./examples/form-creation` | `form-creation.pdf` |
| Password protection | `go run ./examples/protect-pdf` | `protected-password.pdf` |
| Templates | `go run ./examples/templates` | `templates.pdf` |
| Thumbnail | `go run ./examples/thumbnail` | `thumbnail.pdf` |
| UTF-8 font | `go run ./examples/utf8-font` | `utf8-font.pdf` |
| Signing | `go run ./examples/sign-pdf` | `signed.pdf` |

The QR-code example is a separate module so barcode dependencies stay out of
the main module:

```shell
cd examples/external-qr-code
go run .
```

## Packages

```text
gopdfkit   root package: default constructor and public aliases
document   main PDF generation API
font       font parsing and JSON font definition generation
importpdf  small wrappers around imported-page APIs
inspect    lightweight PDF structure, stream, page, and text inspection
sign       CMS-first PDF signing and signature verification
sign/pkcs7 legacy PKCS #7 terminology wrappers around CMS APIs
```

Useful repository directories:

```text
cmd/fontmaker          font definition generator
cmd/list               generated-reference listing utility
examples/              runnable examples
assets/static/         checked-in fonts, images, and text fixtures
assets/generated/pdf/  generated PDFs
doc/                   Markdown documentation
tools/                 tool-only module for quality/security commands
```

## HTML Support

`HTMLNew()` renders a controlled subset of HTML fragments into PDF drawing
operations. It is useful for rich text, generated sections, reports, letters,
and static forms. It is not a browser engine.

Supported content includes inline text tags, links, paragraphs, headings,
blocks, lists, tables, horizontal rules, images, figures, captions, opt-in local
images, data URLs, and inline SVG.

Supported CSS is deliberately small: text styling, line height, alignment,
vertical alignment, whitespace handling, simple colors, backgrounds, borders,
border radius, simple box shadows, padding, margins, table/image dimensions,
image fit modes, list marker style, and basic page-break controls.

Use `document.RenderHTMLTemplate` when HTML fragments need `{{key}}`
substitution. Plain values are escaped, `document.HTMLTemplateRaw` inserts
trusted HTML, and `document.HTMLTemplateImage` inserts an `<img>` tag that can
be sized and spaced with supported HTML/CSS.

See [`doc/pdf-html-subset.md`][pdf-html-subset] for the full contract.

## Fonts

Standard PDF fonts require no setup:

```go
pdf.SetFont("Helvetica", "", 12)
```

Use `AddUTF8Font` or `AddUTF8FontFromBytes` for UTF-8 TrueType fonts,
including OpenType files with TrueType outlines. Use `RTL()` and `LTR()` to
switch right-to-left rendering mode.

For non-UTF-8 TrueType, OpenType/CFF, or Type1 fonts, generate a JSON font
definition with `font.Make` or `cmd/fontmaker`:

```shell
cd cmd/fontmaker
go build
./fontmaker --embed \
  --enc=../../assets/static/font/cp1252.map \
  --dst=../../assets/static/font \
  ../../assets/static/font/calligra.ttf
```

## Signing

Documents can be signed while writing:

```go
err := pdf.OutputSignedFile("signed.pdf", sign.Options{
	Signer:      signer,
	Certificate: cert,
})
```

The `sign` package uses CMS terminology and produces CMS SignedData:

```go
cms, err := sign.CreateCMS(content, sign.CMSOptions{
	Signer:      signer,
	Certificate: cert,
	Detached:    true,
})
```

PDF signature helpers can extract `/ByteRange` content, compute digests, embed
an externally produced detached CMS signature, and inspect CMS signer metadata:

```go
cms, encoding, err := sign.DecodeCMS(rawSignature)
prepared, err := sign.ExtractSignature(pdfBytes)
signedPDF, err := sign.EmbedDetachedCMS(pdfBytes, cms)
result, err := sign.VerifyDetachedCMS(cms, prepared.SignedContent, roots)
cert, err := sign.SignerCertificate(cms)
```

For workflows that require exactly one PDF signature, use
`ExtractSingleSignature`. Its ByteRange can be converted to the fixed
`[4]int64` form and reused for digesting or extracting signed content:

```go
prepared, err := sign.ExtractSingleSignature(pdfBytes)
byteRange, err := prepared.ByteRange64()
digestHex, err := sign.DigestHexForByteRange(pdfBytes, byteRange, crypto.SHA256)
content, err := sign.SignedContentForByteRange(pdfBytes, byteRange)
```

Legacy PKCS #7 terminology is kept separate in `sign/pkcs7`. New code should
prefer the CMS names in `sign`.

## Inspection

Use `inspect` for lightweight PDF checks and text extraction from literal text
operators:

```go
count, err := inspect.PageCount(pdfBytes)
text, err := inspect.Text(pdfBytes)
streams, err := inspect.DecodedStreams(pdfBytes)
```

## Errors

Most `Document` methods record errors on the document instead of returning
errors directly. Once an error is set, later methods usually return without
changing the PDF. Check `Ok()`, `Err()`, or `Error()` after generation,
especially before trusting output.

Applications can transfer their own failures into the PDF object with
`SetError()` or `SetErrorf()`.

## Development

Common commands:

```shell
go test ./...
go vet ./...
go list ./...
make check
```

Benchmarks:

```shell
make bench
make bench-ci
```

Some test examples generate or refresh PDFs under `assets/generated/pdf`. The
`document` test package also clears generated PDFs before its example tests run.

## Background

GoPDFKit is derived from the original [FPDF][fpdf-site] PHP library and keeps
many FPDF-style method names for compatibility and familiarity. Internally, this
Go version uses buffers, `io.Writer`/`io.WriteCloser` output, and JSON font
definition files.

## License

GoPDFKit is released under the MIT License. See [`LICENSE`][license].

## Acknowledgments

This package's code and documentation are closely derived from the
[FPDF][fpdf-site] library created by Olivier Plathey. Many font and image
resources are copied directly from FPDF.

The project also incorporates work or ideas from Bruno Michel, David Hernandez
Sanz, Martin Hall-May, Andreas Wurmser, Manuel Cornes, Moritz Wagner, Klemen
Vodopivec, Lawrence Kesteloot, Stefan Schroeder, Ivan Daniluk, Anthony Starks,
Robert Lillack, Claudio Felber, Stani Michiels, Marcus Downing, Jan Slabon,
Setasign, Jelmer Snoeck, Guillermo Pascual, Kent Quirk, Paulo Coutinho, Dan
Meyers, David Fish, Andy Bakun, Paul Montag, Wojciech Matusiak, Artem Korotkiy,
Dave Barnes, Brigham Thompson, Joe Westcott, and Benoit KUGLER.

[badge-ci]: https://github.com/cssbruno/gopdfkit/actions/workflows/ci.yml/badge.svg
[badge-doc]: https://img.shields.io/badge/godoc-GoPDFKit-blue.svg
[badge-mit]: https://img.shields.io/badge/license-MIT-blue.svg
[ci]: https://github.com/cssbruno/gopdfkit/actions/workflows/ci.yml
[examples]: examples
[fpdf-site]: http://www.fpdf.org/
[godoc]: https://pkg.go.dev/github.com/cssbruno/gopdfkit
[license]: https://raw.githubusercontent.com/cssbruno/gopdfkit/master/LICENSE
[pdf-html-subset]: doc/pdf-html-subset.md
