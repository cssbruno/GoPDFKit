# GoPDFKit document generator

[![CI][badge-ci]][ci]
[![MIT licensed][badge-mit]][license]
[![GoDoc][badge-doc]][godoc]

![][logo]

Package `gopdfkit` is a Go-native PDF document generator. The module includes
the high-level `document` API plus focused packages for fonts, imported pages,
and signatures.

This repository is the `github.com/cssbruno/gopdfkit` module. The `0.1` line is a
breaking cleanup focused on a clearer package layout and shorter, feature-based
source files.

## Features

* UTF-8 TrueType font support
* Standard PDF fonts: Courier, Helvetica, Times, Symbol, and ZapfDingbats
* Page sizing, margins, headers, footers, page boxes, and page breaks
* Cells, multicells, aligned writing, links, bookmarks, and aliases
* JPEG, PNG, GIF, WebP, and SVG image support
* Lines, rectangles, rounded rectangles, arcs, Bézier curves, polygons, and paths
* Clipping, transforms, transparency, gradients, spot colors, and layers
* Templates and imported template objects
* Document protection, attachments, metadata, JavaScript, and XMP metadata
* Controlled HTML/CSS fragment rendering
* Shared document model and renderer for structured reports/forms
* Integrated thumbnail helpers
* Integrated PDF signing and verification helpers
* CLI tools under `cmd/`

The root `gopdfkit` package provides the default constructor and public document
type aliases. Applications that need explicit constructor options, document
model types, measurement helpers, or lower-level drawing APIs can import
`document` directly.

## Go Project Comparison

GoPDFKit is performance-first: it generates PDFs directly in Go, avoids browser
or office runtimes, and keeps HTML support intentionally bounded so rendering
costs and failure modes stay predictable. It will not try to become fully
browser-compatible HTML-to-PDF, and DOCX conversion is out of scope. Typst
support may be considered later as an optional authoring or import path, but it
is not part of the current API.

This comparison only covers Go projects and is based on public project
documentation plus this repository's API. It is a positioning guide, not a
benchmark.

| Project | What GoPDFKit has | What it does not try to replace |
| --- | --- | --- |
| GoPDFKit | FPDF-style generation, UTF-8 TrueType fonts, standard fonts, drawing, controlled HTML fragments, tables, SVG and WebP images, imported pages, templates, protection, attachments, metadata, thumbnails, signing, and verification helpers. | Full browser HTML/CSS layout, JavaScript page rendering, DOCX conversion, arbitrary PDF editing, and enterprise document SDK coverage. |
| [go-pdf/fpdf][go-pdf-fpdf] | Similar FPDF-style workflow, plus GoPDFKit's package split, controlled HTML validation and limits, WebP handling, expanded SVG handling, report helpers, thumbnails, and integrated signing and verification. | `go-pdf/fpdf` remains the closest mature open-source peer and already covers many classic FPDF features, including drawing, fonts, images, templates, layers, protection, attachments, barcodes, charts, and PDF imports. |
| [signintech/gopdf][signintech-gopdf] | A broader document-generation surface: cells, multicells, headers, footers, HTML/table helpers, SVG and WebP handling, metadata, protection, attachments, thumbnails, and signing helpers. | `signintech/gopdf` is a good fit for lower-level coordinate drawing with Unicode fonts, JPG and PNG images, transforms, and imported pages. |
| [Maroto v2][maroto] | Lower-level PDF primitives plus imports, templates, SVG and WebP, attachments, metadata, protection, thumbnails, and signing helpers. | Maroto is better when the main need is grid-based report layout with rows, columns, components, headers, and a testable component tree. |
| [pdfcpu][pdfcpu] | Direct content generation from Go code, including text, graphics, images, HTML fragments, templates, imported pages, and signed output. | pdfcpu is better for processing existing PDFs: validation, encryption, merge, split, rotate, crop, attachments, bookmarks, forms, stamps, watermarks, signature validation, and JSON-driven creation. |

## Package Layout

The active implementation packages are:

* `gopdfkit`: default constructor and root package types.
* `document`: high-level PDF document API and current feature implementation.
* `font`: font parsing and JSON font definition generation.
* `sign`: PDF signing and signature verification.
* `importpdf`: small helpers for imported PDF pages.

Test-only deterministic PDF helpers live under `internal/`.

PDF signing is available from `sign`, and documents can be saved signed:

```go
err := pdf.OutputSignedFile("signed.pdf", sign.Options{
    Signer:      signer,
    Certificate: cert,
})
```

## Installation

```shell
go get github.com/cssbruno/gopdfkit@latest
```

## Quick Start

```go
pdf := gopdfkit.New()
pdf.AddPage()
pdf.SetFont("Helvetica", "B", 16)
pdf.Cell(40, 10, "Hello, world")
err := pdf.OutputFileAndClose("hello.pdf")
```

Advanced users can import `document` directly:

```go
pdf := document.New("P", "mm", "A4", "")
```

Use `NewWithOptions` when generation defaults should be configured up front:

```go
pdf := document.NewWithOptions(document.Options{Optimize: true})
```

Runnable examples live under [`examples/`][examples] and write PDFs under
`assets/generated/pdf/examples`. Additional generated-PDF examples live as Go
tests, especially in [`document/document_test.go`][document-test]. Running the
tests writes generated PDFs under `assets/generated/pdf`.

```shell
go run ./examples/hello-world
go run ./examples/drawing
go run ./examples/headers-footers
go run ./examples/html-fragment
go run ./examples/image-from-memory
go run ./examples/import-page
go run ./examples/protection-attachments
go run ./examples/rendering-gallery
go run ./examples/structured-report
go run ./examples/sign-pdf
go run ./examples/templates
go run ./examples/thumbnail
go run ./examples/utf8-font
```

The QR-code example is a separate module so barcode dependencies stay out of
GoPDFKit:

```shell
cd examples/external-qr-code
go run .
```

## Repository Layout

The repository has runnable command examples plus test examples that stay close
to the behavior they validate.

```text
document/           high-level PDF document API and current implementation
font/               font parsing and font definition generation
importpdf/          imported PDF page helpers
sign/               PDF signing and verification

cmd/
  list/              generated-reference listing utility
  fontmaker/         font definition generator

examples/            runnable usage examples

assets/
  static/            checked-in fonts, images, and text fixtures
  generated/pdf/     generated PDFs and reference PDFs

internal/
  testpdf/              deterministic PDF comparison helpers for tests
  testsupport/example/  test/example support helpers

doc/                 Markdown and generated documentation inputs/templates
tools/               tool-only module for quality/security commands
```

The `document` package is still split by feature-oriented files while the
dedicated packages are extracted incrementally.

## HTML and CSS Support

`HTMLNew()` renders a controlled subset of HTML fragments into PDF drawing
operations. It is useful for rich text, generated document sections, reports,
letters, and forms. It is not a browser-compatible HTML/CSS renderer.

See [`doc/pdf-html-subset.md`][pdf-html-subset] for the full renderer contract.

Supported HTML includes:

* inline tags: `b`, `strong`, `i`, `em`, `u`, `s`, `code`, `sub`, `sup`
* links, paragraphs, headings, `div`, `section`, blockquotes, and preformatted text
* ordered, unordered, and definition lists
* tables with captions, `thead`, `tbody`, `tfoot`, `colspan`, and `rowspan`
* horizontal rules
* images, figures, captions, data URLs, and opt-in local images
* inline SVG

Supported CSS is deliberately small: text styling, line height, alignment,
vertical alignment, whitespace handling, simple colors, backgrounds, borders,
padding, margins, table/image dimensions, image fit modes, list marker style,
and basic page-break controls.

Selectors are limited to tag, class, ID, tag-qualified class or ID, descendant,
and direct-child selectors. Attribute selectors, pseudo selectors, media rules,
floats, flexbox, grid, absolute positioning, shadows, browser table layout, and
full browser font shaping are not implemented.

Use `HTML.DebugLog` while rendering or `HTML.ValidateHTML` before rendering to
collect best-effort diagnostics for unsupported tags, attributes, CSS selectors,
and CSS properties.

HTML input limits are configurable through fields such as `MaxHTMLBytes`,
`MaxElementDepth`, `MaxTableRows`, `MaxGeneratedPages`, and
`MaxDataImageBytes`.

## Fonts

Nothing special is required for standard PDF fonts:

```go
pdf.SetFont("Helvetica", "", 12)
```

Use `AddUTF8Font()` or `AddUTF8FontFromBytes()` for UTF-8 TrueType fonts,
including OpenType files with TrueType outlines. Use `RTL()` and `LTR()` to
switch right-to-left rendering mode.

For non-UTF-8 TrueType, OpenType/CFF, or Type1 fonts, generate a JSON font
definition file with `font.Make` or the `cmd/fontmaker` command.

```shell
cd cmd/fontmaker
go build
./fontmaker --embed \
  --enc=../../assets/static/font/cp1252.map \
  --dst=../../assets/static/font \
  ../../assets/static/font/calligra.ttf
```

Then call `AddFont()` and `SetFont()` from your PDF generation code.

## Generated PDFs and References

Running the runnable examples writes PDFs in `assets/generated/pdf/examples`.
Running `go test ./...` generates PDFs in `assets/generated/pdf`. Reference
PDFs are stored in `assets/generated/pdf/reference`.

`internal/testsupport/example` contains helpers used by tests to name generated
files and compare generated PDFs against reference copies. Comparisons need
deterministic object ordering and timestamps; tests use `SetCatalogSort()` and
`SetCreationDate()` for that.

## Testing and Quality

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

Tooling commands are defined in the `Makefile`. The `tools/` module keeps
tool-only dependencies separate from the main library module.

## Errors

Most `Document` methods record errors on the document instead of returning errors
directly. Once an error is set, later methods usually return without changing
the PDF. Check `Ok()`, `Err()`, or `Error()` after generation, especially after
calling `Output()`.

Applications can transfer their own failures into the PDF object with
`SetError()` or `SetErrorf()`.

## Conversion Notes

This package is derived from the original [FPDF][fpdf-site] PHP library. The API
keeps many FPDF-style method names for compatibility and familiarity, even where
shorter Go names would be more idiomatic.

Internally, this fork uses Go buffers and `io.Writer`/`io.WriteCloser` output
instead of PHP string concatenation and dynamic argument behavior. Font
definition files are JSON rather than PHP.

The `0.1` cleanup removes the old `Type` suffix convention from public type
names. No compatibility aliases are provided for those breaking renames.

## Contributing

Pull requests are welcome for focused fixes, tests, documentation, and feature
work. Please keep changes scoped and keep tests beside the code they validate.

Before submitting a change, run:

```shell
go test ./...
go vet ./...
gofmt -w .
```

## License

GoPDFKit is released under the MIT License. See [`LICENSE`][license].

## Acknowledgments

This package's code and documentation are closely derived from the
[FPDF][fpdf-site] library created by Olivier Plathey. Many font and image
resources are copied directly from FPDF.

The project also incorporates work or ideas from Bruno Michel, David Hernández
Sanz, Martin Hall-May, Andreas Würmser, Manuel Cornes, Moritz Wagner, Klemen
Vodopivec, Lawrence Kesteloot, Stefan Schroeder, Ivan Daniluk, Anthony Starks,
Robert Lillack, Claudio Felber, Stani Michiels, Marcus Downing, Jan Slabon,
Setasign, Jelmer Snoeck, Guillermo Pascual, Kent Quirk, Paulo Coutinho, Dan
Meyers, David Fish, Andy Bakun, Paul Montag, Wojciech Matusiak, Artem Korotkiy,
Dave Barnes, Brigham Thompson, Joe Westcott, and Benoit KUGLER.

[badge-ci]: https://github.com/cssbruno/gopdfkit/actions/workflows/ci.yml/badge.svg
[badge-doc]: https://img.shields.io/badge/godoc-GoPDFKit-blue.svg
[badge-mit]: https://img.shields.io/badge/license-MIT-blue.svg
[ci]: https://github.com/cssbruno/gopdfkit/actions/workflows/ci.yml
[dfont]: http://dejavu-fonts.org/
[effective-go]: https://golang.org/doc/effective_go.html
[examples]: examples
[fpdf-site]: http://www.fpdf.org/
[document-test]: https://github.com/cssbruno/gopdfkit/blob/master/document/document_test.go
[github]: https://github.com/cssbruno/gopdfkit
[godoc]: https://pkg.go.dev/github.com/cssbruno/gopdfkit
[go-pdf-fpdf]: https://pkg.go.dev/codeberg.org/go-pdf/fpdf
[license]: https://raw.githubusercontent.com/cssbruno/gopdfkit/master/LICENSE
[logo]: https://raw.githubusercontent.com/cssbruno/gopdfkit/master/assets/static/image/gopher_pdf.png
[maroto]: https://pkg.go.dev/github.com/johnfercher/maroto/v2
[noto]: https://github.com/jsntn/webfonts/blob/master/NotoSansSC-Regular.ttf
[pdf-html-subset]: doc/pdf-html-subset.md
[pdfcpu]: https://pkg.go.dev/github.com/pdfcpu/pdfcpu/pkg/pdfcpu
[signintech-gopdf]: https://pkg.go.dev/github.com/signintech/gopdf
