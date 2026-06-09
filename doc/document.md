# GoPDFKit

[![CI][badge-ci]][ci]
[![MIT licensed][badge-mit]][license]
[![GoDoc][badge-doc]][godoc]

## Benchmark Snapshot

Local benchmark run on `12th Gen Intel(R) Core(TM) i7-12700` with 20 logical
CPUs. Results are medians from:

```shell
go test ./document -run '^$' -bench 'BenchmarkGeneration(BaselineNoCompliance.*|Images.*|PDFA4FCompliance.*|PDFUA2ArlingtonCompiledHTML.*|SignedPDFA4FPDFUA2ArlingtonXMP.*|HTML(SelectorHeavyCompiled|TableHeavyCompiled|DataImageHeavyCompiled|MalformedCompiled).*)$' -benchmem -count=3
```

| Workload | Mode | ns/PDF | PDF/sec | Memory/PDF | Allocs/PDF |
| --- | ---: | ---: | ---: | ---: | ---: |
| Baseline no-compliance, uncached image | Single worker | 932,906 | 1,072 | 1,117,652 B | 1,703 |
| Baseline no-compliance, uncached image | 100% CPU | 89,024 | 11,233 | 874,016 B | 1,690 |
| Baseline no-compliance, cached image | Single worker | 517,280 | 1,933 | 825,700 B | 1,564 |
| Baseline no-compliance, cached image | 100% CPU | 79,543 | 12,572 | 649,346 B | 1,555 |
| Baseline no-compliance, signed uncached image | Single worker | 1,934,896 | 517 | 1,953,249 B | 2,033 |
| Baseline no-compliance, signed uncached image | 100% CPU | 251,348 | 3,979 | 1,706,731 B | 2,014 |
| Baseline no-compliance, signed cached image | Single worker | 1,419,739 | 704 | 1,555,624 B | 1,889 |
| Baseline no-compliance, signed cached image | 100% CPU | 203,488 | 4,914 | 1,466,333 B | 1,879 |
| PDF/A-4f metadata, ICC, UTF-8 fonts, attachment | Single worker | 5,094,580 | 196 | 10,526,864 B | 27,629 |
| PDF/A-4f metadata, ICC, UTF-8 fonts, attachment | 100% CPU | 916,350 | 1,091 | 11,220,702 B | 27,644 |
| PDF/UA-2 + Arlington tagged compiled HTML | Single worker | 5,842,078 | 171 | 11,087,177 B | 34,761 |
| PDF/UA-2 + Arlington tagged compiled HTML | 100% CPU | 764,041 | 1,309 | 12,095,492 B | 34,785 |
| Signed PDF/A-4f + PDF/UA-2 + Arlington + XMP XML metadata | Single worker | 6,824,408 | 147 | 11,877,863 B | 35,144 |
| Signed PDF/A-4f + PDF/UA-2 + Arlington + XMP XML metadata | 100% CPU | 833,561 | 1,200 | 12,621,359 B | 35,159 |
| Selector-heavy compiled HTML | Single worker | 635,650 | 1,573 | 207,433 B | 1,615 |
| Selector-heavy compiled HTML | 40 workers | 127,062 | 7,870 | 211,776 B | 1,628 |
| Table-heavy compiled HTML | Single worker | 968,191 | 1,033 | 826,387 B | 1,899 |
| Table-heavy compiled HTML | 40 workers | 174,264 | 5,738 | 826,360 B | 1,903 |
| Data-image-heavy compiled HTML | Single worker | 257,365 | 3,886 | 263,682 B | 1,059 |
| Data-image-heavy compiled HTML | 40 workers | 36,791 | 27,181 | 168,223 B | 1,056 |
| Malformed compiled HTML recovery | Single worker | 340,485 | 2,937 | 296,380 B | 1,196 |
| Malformed compiled HTML recovery | 40 workers | 101,023 | 9,899 | 299,271 B | 1,206 |
| Four uncached images | Single worker | 1,311,764 | 762 | 1,971,071 B | 1,470 |
| Four uncached images | 100% CPU | 182,279 | 5,486 | 1,838,310 B | 1,460 |
| Four cached images | Single worker | 63,990 | 15,627 | 70,630 B | 275 |
| Four cached images | 100% CPU | 48,924 | 20,440 | 72,979 B | 283 |

The 100% CPU rows use `runtime.GOMAXPROCS(0)` explicit benchmark workers, so
they measure concurrent PDF generation throughput across all logical CPUs
available to the Go process. Signed rows
include PDF output plus detached CMS signing; the benchmark certificate and key
are prepared outside the timed loop. Compliance rows measure generation only;
external veraPDF and Arlington validation are separate CI steps.

## Apples-to-Apples gopdflib Harness

The `benchmarks/gopdfsuit` module compares GoPDFKit against the in-process
`github.com/chinmay-sawant/gopdfsuit/v5/pkg/gopdflib` API with one shared
harness. Both libraries run on the same machine, use the same prepared workload
data, write to the same in-memory PDF target, and use the same explicit worker
counts.

```shell
make bench-gopdfsuit-ci
```

The comparable workloads are `table_180_rows`, `table_900_rows`, and
`png_table_180_rows`, each run in `single` and `workers_40` modes. The harness
validates that every output starts with a PDF header and contains an EOF marker.
HTML and compliance rows are opt-in or excluded because the two libraries do not
expose equivalent behavior for those cases.

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
goroutines as long as callers do not mutate values returned by `Tokens()`.

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
make bench-gopdfsuit
make bench-gopdfsuit-ci
```

`make bench-gopdfsuit` runs the apples-to-apples comparison harness in
`benchmarks/gopdfsuit`. That harness drives GoPDFKit and the in-process
`github.com/chinmay-sawant/gopdfsuit/v5/pkg/gopdflib` API with the same workload
data, in-memory PDF output target, and explicit single-worker and 40-worker
modes. It intentionally does not include GoPdfSuit HTTP service or client
transport overhead. Chrome-backed HTML conversion is opt-in because GoPDFKit's
HTML renderer is an in-process subset while gopdflib uses Chrome/Chromium.

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
