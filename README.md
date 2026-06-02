# GoPDFKit document generator

[![CI][badge-ci]][ci]
[![MIT licensed][badge-mit]][license]
[![GoDoc][badge-doc]][godoc]

![][logo]

Package `gopdfkit` implements a Go-native PDF document generator. It supports
text, drawing, images, templates, SVG, HTML fragments, integrated barcodes,
PDF signing helpers, and deterministic test output.

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
* Integrated barcode helpers
* Integrated thumbnail helpers
* Integrated PDF signing and verification helpers
* CLI tools under `cmd/`

The root `gopdfkit` package contains the primary document, image, barcode, and
signing APIs.

## Support Matrix

This table compares the features this codebase explicitly supports with common
alternatives. Other projects can differ by version and configuration, so their
columns describe typical support rather than a guarantee for every library.

| Capability | GoPDFKit support | Browser HTML-to-PDF support | Other FPDF-style code support |
| --- | --- | --- | --- |
| Go-native PDF generation | Yes | No, usually requires a browser/runtime bridge | Varies |
| Standard PDF fonts | Yes | Varies | Usually yes |
| UTF-8 TrueType fonts | Yes | Yes | Varies |
| Text cells, multicells, links, bookmarks, and aliases | Yes | Varies | Usually yes |
| Drawing primitives and paths | Yes | Varies | Usually yes |
| Clipping, transforms, transparency, gradients, spot colors, and layers | Yes | Varies | Varies |
| JPEG, PNG, GIF, WebP, and SVG images | Yes | Usually yes | Varies |
| Controlled HTML/CSS fragment rendering | Yes | Yes, with broader browser layout | Varies |
| Browser-grade HTML/CSS layout | No | Usually yes | Usually no |
| Tables with `thead`, `tbody`, `tfoot`, `colspan`, and `rowspan` | Yes | Usually yes | Varies |
| Configurable HTML validation and render limits | Yes | Varies | Varies |
| Templates and imported template objects | Yes | Varies | Varies |
| Document protection, attachments, metadata, JavaScript, and XMP metadata | Yes | Varies | Varies |
| PDF signing and verification helpers | Yes, integrated in the root API | Varies | Varies |
| Barcode helpers | Yes, integrated in the root API | Varies | Varies |

## Integrated Barcode and Signing

Barcode generation is available directly from `*Fpdf`:

```go
key := pdf.RegisterQRBarcode("https://example.test/verify", gopdfkit.QRBarcodeHigh, gopdfkit.QRBarcodeUnicode)
pdf.Barcode(key, 10, 10, 24, 24, false)
```

PDF signing is also available from the root package:

```go
err := pdf.OutputSignedFile("signed.pdf", gopdfkit.SignOptions{
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
pdf := gopdfkit.New("P", "mm", "A4", "")
pdf.AddPage()
pdf.SetFont("Helvetica", "B", 16)
pdf.Cell(40, 10, "Hello, world")
err := pdf.OutputFileAndClose("hello.pdf")
```

Most runnable usage examples live as Go tests, especially in
[`fpdf_test.go`][fpdf-test]. Running the tests writes generated PDFs under
`assets/generated/pdf`.

## Repository Layout

The repository intentionally does not have a top-level `examples/` directory.
Examples are tests and shared test helpers, so they stay close to the behavior
they validate.

```text
cmd/
  list/              font/map listing utility
  fontmaker/          font definition generator

assets/
  static/            checked-in fonts, images, and text fixtures
  generated/pdf/     generated PDFs and reference PDFs

testsupport/example/    test/example support helpers

doc/                 Markdown and generated documentation inputs/templates
tools/               tool-only module for quality/security commands
```

The root package is split by feature area:

* `types_*.go` and `fpdf_state.go` contain shared public/internal data types.
* `font_*.go`, `image_*.go`, `html_*.go`, `svg_*.go`, `text_*.go`,
  `drawing_*.go`, `output_*.go`, and `document_*.go` group behavior by domain.
* `util_*.go` contains small compression, encoding, I/O, and math helpers.

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
definition file with `MakeFont` or the `cmd/fontmaker` command.

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

Running `go test ./...` generates PDFs in `assets/generated/pdf`. Reference PDFs
are stored in `assets/generated/pdf/reference`.

`testsupport/example` contains helpers used by tests to name generated files and,
when enabled, compare generated PDFs against reference copies. Comparisons need
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

Most `Fpdf` methods record errors on the document instead of returning errors
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
[fpdf-site]: http://www.fpdf.org/
[fpdf-test]: https://github.com/cssbruno/gopdfkit/blob/master/fpdf_test.go
[github]: https://github.com/cssbruno/gopdfkit
[godoc]: https://pkg.go.dev/github.com/cssbruno/gopdfkit
[license]: https://raw.githubusercontent.com/cssbruno/gopdfkit/master/LICENSE
[logo]: https://raw.githubusercontent.com/cssbruno/gopdfkit/master/assets/static/image/gopher_pdf.png
[noto]: https://github.com/jsntn/webfonts/blob/master/NotoSansSC-Regular.ttf
[pdf-html-subset]: doc/pdf-html-subset.md
