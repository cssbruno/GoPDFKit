# GoPDFKit

[![CI][badge-ci]][ci]
[![MIT licensed][badge-mit]][license]
[![GoDoc][badge-doc]][godoc]

## Benchmark Snapshot

Local benchmark run on `Apple M2` with 8 logical CPUs. Results below are from:

```shell
make bench-generation-core
```

For a generation-only suite without HTML examples:

```shell
make bench-generation-core-ci
```

| Workload | Mode | ns/PDF | PDF/sec | Memory/PDF | Allocs/PDF | Output size | Total allocated |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Text table | 40 workers | 85,868 | 11,646 | 369,284 B | 1,479 | 43,543 B | 5,033 MB |
| Long text | 40 workers | 35,729 | 27,989 | 75,519 B | 235 | 8,233 B | 2,284 MB |
| Baseline no-compliance, uncached image | 40 workers | 151,452 | 6,603 | 632,074 B | 1,391 | 59,498 B | 4,619 MB |
| Baseline no-compliance, no image | 40 workers | 87,335 | 11,450 | 436,097 B | 1,207 | 50,260 B | 7,090 MB |
| Baseline no-compliance, cached image | 40 workers | 69,819 | 14,323 | 455,848 B | 1,268 | 59,330 B | 7,497 MB |
| Baseline no-compliance, signed uncached image | 40 workers | 422,288 | 2,368 | 1,023,959 B | 1,581 | 93,290 B | 3,161 MB |
| Baseline no-compliance, signed cached image | 40 workers | 339,404 | 2,946 | 848,000 B | 1,457 | 93,066 B | 2,911 MB |
| UTF-8 text | 40 workers | 699,661 | 1,429 | 5,646,683 B | 12,464 | 44,322 B | 8,385 MB |
| UTF-8 text, cached font | 40 workers | 200,822 | 4,980 | 636,136 B | 2,052 | 44,322 B | 3,568 MB |
| Text compression, best speed | 40 workers | 79,007 | 12,657 | 360,635 B | 959 | 8,139 B | 4,990 MB |
| Text compression, best compression | 40 workers | 153,613 | 6,510 | 369,508 B | 959 | 7,678 B | 2,608 MB |
| Four uncached images | 40 workers | 202,576 | 4,936 | 1,346,954 B | 1,293 | 15,025 B | 8,028 MB |
| Four cached images | 40 workers | 5,399 | 185,214 | 63,357 B | 175 | 14,969 B | 13,958 MB |
| SVG | 40 workers | 8,579 | 116,558 | 45,922 B | 118 | 7,613 B | 6,282 MB |
| Templates | 40 workers | 66,614 | 15,012 | 229,002 B | 402 | 9,862 B | 3,840 MB |
| Imported PDF pages | 40 workers | 5,423 | 184,402 | 40,338 B | 267 | 1,890 B | 8,296 MB |
| Protection | 40 workers | 8,198 | 121,980 | 54,476 B | 328 | 4,999 B | 7,727 MB |
| Attachments | 40 workers | 67,375 | 14,842 | 118,438 B | 173 | 13,684 B | 2,088 MB |

The 40-worker rows use a fixed explicit worker count, so they measure concurrent
PDF generation throughput with the same workload pressure across machines.
Signed rows include PDF output plus detached CMS signing; the benchmark
certificate and key are prepared outside the timed loop. Compliance rows measure
generation only; external veraPDF and Arlington validation are separate CI
steps. The raw Go benchmark output also includes `pdf/s`, `pdf_bytes`, and
`total_MB` metrics from the timed loop.

Additional compiled HTML/parser medians from:

```shell
go test ./document -run '^$' -bench 'BenchmarkHTMLTokenize|BenchmarkCompileHTMLLargeFragment|BenchmarkGenerationHTML(SelectorHeavyCompiled|DataImageHeavy|DataImageHeavyCompiled|LargeTableCompiled|WideTableCompiled|SVGHeavyCompiled)$' -benchmem -count=5
```

| Workload | ns/op | Ops/sec | Memory/op | Allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Tokenizer, small attributes | 313 | 3,190,810 | 432 B | 3 |
| Tokenizer, large fragment | 459,518 | 2,176 | 1,130,706 B | 3,125 |
| CompileHTML, large fragment | 1,995,077 | 501 | 3,105,631 B | 11,384 |
| Selector-heavy compiled HTML render | 626,999 | 1,595 | 193,069 B | 1,629 |
| Data-image-heavy compiled HTML render | 209,107 | 4,782 | 137,117 B | 1,027 |
| Data-image-heavy non-compiled HTML render | 310,302 | 3,223 | 233,747 B | 1,439 |
| Large table compiled HTML render | 5,127,521 | 195 | 5,227,244 B | 11,515 |
| Wide table compiled HTML render | 1,461,651 | 684 | 1,584,045 B | 3,538 |
| SVG-heavy compiled HTML render | 452,602 | 2,209 | 391,676 B | 1,762 |

The latest local profiles used for the tokenizer and compiled HTML work are
`/tmp/gopdfkit-tokenizer.cpu`, `/tmp/gopdfkit-selector-heavy.cpu`, and
`/tmp/gopdfkit-data-image-heavy.mem`.

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
* JPEG, PNG, GIF, WebP, SVG, data-image, QR-code PNG generation, and thumbnail
  workflows
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

## Compliance Metadata

`document.SetComplianceMetadata` can generate PDF/A-4, PDF/UA-2, Arlington, and
XMP metadata foundations. PDF/UA-2 mode enables tagged PDF output with
`/StructTreeRoot`, `/ParentTree`, page and annotation `/StructParent` entries,
MCIDs, text/link/image marked content, link annotation `/OBJR` references,
image alternate text, artifact spans for drawing/raw decorative content, and
semantic structure containers for HTML and shared document renderer lists and
tables. Templates and imported pages are treated as artifacts unless their
content is rendered through semantic APIs before placement. PDF/A mode also
enforces the high-risk generation blockers currently known to the library: no
encryption, no JavaScript, an ICC output intent, embedded UTF-8 fonts with
Unicode maps, and PDF/A-4f or PDF/A-4e when attachments are present.

This is not a full validator replacement. Use `make compliance-fixtures` and
`make compliance-validate` with external validators for standards checks. See
[`compliance-validation.md`][compliance-validation].

Strict validation can be run with Dockerized veraPDF plus the Arlington REST
service:

```shell
REQUIRE_COMPLIANCE_TOOLS=1 \
SRGB_ICC=/usr/share/color/icc/colord/sRGB.icc \
VERAPDF_DOCKER_IMAGE=verapdf/cli:v1.30.2 \
VERAPDF='tools/verapdf-docker.sh 0' \
PDFUA_CHECKER='tools/verapdf-docker.sh ua2' \
ARLINGTON_CHECKER='tools/arlington-validate.sh' \
ARLINGTON_URL='http://localhost:8080' \
make compliance-validate
```

## Examples

Runnable examples live under [`examples/`][examples]. They write PDFs to
`assets/generated/pdf/examples`. For compact code snippets grouped by workflow,
see [`generation-examples.md`][generation-examples].

| Workflow | Command | Output |
| --- | --- | --- |
| Hello world | `go run ./examples/hello-world` | `hello-world.pdf` |
| Drawing primitives | `go run ./examples/drawing` | `drawing.pdf` |
| Headers and footers | `go run ./examples/headers-footers` | `headers-footers.pdf` |
| Report | `go run ./examples/report` | `gopdfkit-report.pdf` |
| Structured report | `go run ./examples/structured-report` | `structured-report.pdf` |
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
| Image from memory | `go run ./examples/image-from-memory` | `image-from-memory.pdf` |
| Compression | `go run ./examples/compress-optimize-pdf` | `compressed-optimized.pdf`, `uncompressed-debug.pdf` |
| Import page | `go run ./examples/import-page` | `import-page.pdf` |
| Watermark | `go run ./examples/watermark-pdf` | `watermarked.pdf` |
| Merge pages | `go run ./examples/merge-pdf-pages` | `merged-pages.pdf` |
| Split and reorder pages | `go run ./examples/split-reorder-pages` | `split-page-2.pdf`, `reordered-pages.pdf` |
| Rotate pages | `go run ./examples/rotate-pages` | `rotated-pages.pdf` |
| 4-up pages | `go run ./examples/four-up-pages` | `four-up-pages.pdf` |
| Template overlay | `go run ./examples/template-overlay` | `template-overlay.pdf` |
| Static form document | `go run ./examples/form-creation` | `form-creation.pdf` |
| Password protection | `go run ./examples/protect-pdf` | `protected-password.pdf` |
| Password and attachments | `go run ./examples/protection-attachments` | `protection-attachments.pdf` |
| Templates | `go run ./examples/templates` | `templates.pdf` |
| Thumbnail | `go run ./examples/thumbnail` | `thumbnail.pdf` |
| UTF-8 font | `go run ./examples/utf8-font` | `utf8-font.pdf` |
| Signing | `go run ./examples/sign-pdf` | `signed.pdf` |
| Rendering gallery | `go run ./examples/rendering-gallery` | many generated PDFs |
| External QR code module | `cd examples/external-qr-code && go run .` | `qr-code.pdf` |

Use `Document.RegisterQRCodePNG` for QR-code verification blocks.

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

Use `document.CompileHTML` with `HTML.WriteCompiled` when the same fragment is
rendered repeatedly. The compiled plan reuses tokenization, CSS selector
matching, table parsing, inline SVG parsing, data URI image decoding, and cached
block text. A `CompiledHTML` value is safe to reuse across documents and
goroutines.

Compiled fragments expose lightweight diagnostics:

```go
compiled, err := document.CompileHTML(fragment)
if err != nil {
    return err
}
stats := compiled.Stats()
issues := compiled.RecoveryIssues()
dump := compiled.DebugDump()
_, _, _ = stats, issues, dump
```

`Stats` reports reusable parse-product counts such as nodes, tables, images,
inline SVGs, CSS rules, cached text, cached styles, and malformed-fragment
recoveries. `RecoveryIssues` reports unclosed, misnested, or unexpected closing
tags observed while building the private node model. `DebugDump` is intended for
human diagnostics and should not be parsed as a stable wire format.

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
make bench-generation-core
make bench-generation-core-ci
```

`make bench-generation-core` runs non-HTML generation benchmarks only: baseline,
text, UTF-8 text, images, SVG, templates, imported pages, protection, and
attachments.

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
[compliance-validation]: compliance-validation.md
[examples]: ../examples
[fpdf-site]: http://www.fpdf.org/
[generation-examples]: generation-examples.md
[godoc]: https://pkg.go.dev/github.com/cssbruno/gopdfkit
[license]: https://raw.githubusercontent.com/cssbruno/gopdfkit/master/LICENSE
[pdf-html-subset]: pdf-html-subset.md
