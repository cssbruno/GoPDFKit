# GoPDFKit

[![CI][badge-ci]][ci]
[![MIT licensed][badge-mit]][license]
[![GoDoc][badge-doc]][godoc]
<img src="https://raw.githubusercontent.com/cssbruno/gopdfkit/master/assets/static/image/gopher_pdf.png" alt="GoPDFKit gopher" width="160">
## Benchmark Snapshot

Local benchmark run on `Apple M2` with 8 logical CPUs. Results below are from:

```shell
make bench-generation-core
```

For a focused generation suite that includes compiled HTML table cases:

```shell
make bench-generation-core-ci
```

| Workload | Mode | ns/PDF | PDF/sec | Memory/PDF | Allocs/PDF | Output size |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Text table | 40 workers | 85,868 | 11,646 | 369,284 B | 1,479 | 43,543 B |
| Long text | 40 workers | 35,729 | 27,989 | 75,519 B | 235 | 8,233 B |
| Baseline no-compliance, uncached image | 40 workers | 151,452 | 6,603 | 632,074 B | 1,391 | 59,498 B |
| Baseline no-compliance, no image | 40 workers | 87,335 | 11,450 | 436,097 B | 1,207 | 50,260 B |
| Baseline no-compliance, cached image | 40 workers | 69,819 | 14,323 | 455,848 B | 1,268 | 59,330 B |
| Baseline no-compliance, signed uncached image | 40 workers | 422,288 | 2,368 | 1,023,959 B | 1,581 | 93,290 B |
| Baseline no-compliance, signed cached image | 40 workers | 339,404 | 2,946 | 848,000 B | 1,457 | 93,066 B |
| UTF-8 text | 40 workers | 699,661 | 1,429 | 5,646,683 B | 12,464 | 44,322 B |
| UTF-8 text, cached font | 40 workers | 200,822 | 4,980 | 636,136 B | 2,052 | 44,322 B |
| Text compression, best speed | 40 workers | 79,007 | 12,657 | 360,635 B | 959 | 8,139 B |
| Text compression, best compression | 40 workers | 153,613 | 6,510 | 369,508 B | 959 | 7,678 B |
| Four uncached images | 40 workers | 202,576 | 4,936 | 1,346,954 B | 1,293 | 15,025 B |
| Four cached images | 40 workers | 5,399 | 185,214 | 63,357 B | 175 | 14,969 B |
| SVG | 40 workers | 8,579 | 116,558 | 45,922 B | 118 | 7,613 B |
| Templates | 40 workers | 66,614 | 15,012 | 229,002 B | 402 | 9,862 B |
| Imported PDF pages | 40 workers | 5,423 | 184,402 | 40,338 B | 267 | 1,890 B |
| Protection | 40 workers | 8,198 | 121,980 | 54,476 B | 328 | 4,999 B |
| Attachments | 40 workers | 67,375 | 14,842 | 118,438 B | 173 | 13,684 B |

The 40-worker rows use a fixed explicit worker count, so they measure concurrent
PDF generation throughput with the same workload pressure across machines.
Signed rows include PDF output plus detached CMS signing; the benchmark
certificate and key are prepared outside the timed loop. Compliance rows measure
generation only; external veraPDF and Arlington validation run locally and in
the release workflow, not on every push or pull request. The raw Go benchmark
output also includes the per-operation `pdf/s` and `pdf_bytes` metrics.

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

Generate reproducible profiles under `artifacts/profiles` by selecting a single
benchmark with `BENCH`:

```shell
make profile-cpu BENCH='BenchmarkGenerationHTMLLargeTableCompiled$'
make profile-alloc BENCH='BenchmarkGenerationHTMLLargeTableCompiled$'
make profile-block BENCH='BenchmarkGenerationTextConcurrent40$'
make profile-mutex BENCH='BenchmarkGenerationTextConcurrent40$'
make profile-trace BENCH='BenchmarkGenerationTextConcurrent40$'

go tool pprof -top artifacts/profiles/document.test artifacts/profiles/cpu.pprof
go tool pprof -top -sample_index=alloc_objects artifacts/profiles/document.test artifacts/profiles/alloc.pprof
go tool trace artifacts/profiles/trace.out
```

Allocation profiles use a fixed 20-iteration run with full allocation sampling;
use timing-only benchmark runs for performance comparisons. Benchmarks and
`benchstat` comparisons are intentionally local-only and are not part of CI.



GoPDFKit is an MIT-licensed Go library for generating PDFs directly from Go
code. It keeps an FPDF-style API for familiar page/text/drawing workflows, with
additional helpers for HTML fragments, imported pages, signing, thumbnails, and
optional typed document models.

The canonical API is `github.com/cssbruno/gopdfkit/document`. The module root
no longer contains a facade package as of v0.12.0. Renderer-independent typed
models and measurement primitives live in `github.com/cssbruno/gopdfkit/layout`.
Ownership rules and the public-surface policy are documented in
[`ARCHITECTURE.md`](ARCHITECTURE.md).

## Install

```shell
go get github.com/cssbruno/gopdfkit@latest
```

## Quick Start

```go
package main

import "github.com/cssbruno/gopdfkit/document"

func main() {
	pdf, err := document.NewDocument()
	if err != nil {
		panic(err)
	}
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(40, 10, "Hello, world")
	if err := pdf.OutputFile("hello.pdf"); err != nil {
		panic(err)
	}
}
```

Configure construction with typed functional options:

```go
pdf := document.MustNew(
	document.WithOrientation(document.OrientationPortrait),
	document.WithUnit(document.UnitMillimeter),
	document.WithPageSize(document.PageSizeA4),
	document.WithBestCompression(),
)
```

`WithBestCompression` selects best zlib compression for generated page and template
streams. It is not a full PDF optimizer for images, object streams, fonts, or
arbitrary existing PDFs.

## Choosing a Document Path

| Need | Use |
| --- | --- |
| Precise drawing, custom pagination, or FPDF-style control | `Document` drawing/text/table methods |
| Report-like documents with fixed HTML/CSS and changing values | `document.CompileHTMLTemplate` + `HTML.WriteTemplate` |
| One-off HTML fragments or normal rich text sections | `HTMLNew().Write` |
| Typed Go blocks without HTML strings, or existing model-based code | `layout.NewDocumentModel` + `document.WriteDocument` |

The `layout` package exists for the last case: an optional typed block model. It
is not the default template system. Prefer compiled HTML templates when the
document is naturally described as HTML/CSS with changing values.

## Large PDF Output

Normal `Output` and `OutputFile` calls keep the final PDF bytes in the document
buffer, which makes repeated output from one `Document` possible. For very
large unsigned PDFs where peak memory matters more than repeatability, use the
one-shot streaming path:

```go
err := pdf.OutputFileStream("large.pdf")
```

The same path is available through `OutputStream`,
`OutputOptions{StreamFinal: true}`, or
`WithOutputPolicy(document.OutputPolicy{StreamFinal: true})`.
Streaming final output is opt-in, consumes the document for final output, and is
disabled for signed output because signing needs the complete byte range.

Use `document.NewDocumentWithDefaults` when compression, catalog ordering, or
fixed metadata dates should be explicit for one document. Defaults are
immutable and request-scoped; there are no package-wide setters:

```go
defaults := document.DefaultSettings()
defaults.Compression = false
pdf, err := document.NewDocumentWithDefaults(
	defaults,
	document.WithPageSize(document.PageSizeLetter),
)
```

## Optional Typed Document Models

Use `layout.NewDocumentModel` only when your application wants typed Go blocks
instead of HTML templates. The `document` package renders these models with
`WriteDocument` but does not duplicate the layout API.

Keep product-specific document names in your application:

```go
func receiptModel(number string, rows []layout.TableRow) *layout.LayoutDocument {
	return layout.NewDocumentModel("Receipt "+number,
		layout.MetadataGridBlock{Fields: []layout.MetadataField{
			{Label: "Number", Value: number},
		}},
		layout.TableBlock{Body: rows},
	)
}
```

Migration from the previous document-kind helpers:

* `document.NewLayoutDocument(document.DocumentKindReport)` becomes
  `layout.NewLayoutDocument()`.
* `document.NewGenericDocument("Title", blocks...)` becomes
  `layout.NewDocumentModel("Title", blocks...)`.
* Replace report, transactional, attestation, and statement builders with
  caller-owned functions that return `*layout.LayoutDocument`.

## Current Capabilities

GoPDFKit currently supports:

* PDF generation with pages, margins, headers, footers, page boxes, and page
  breaks
* Standard PDF fonts and UTF-8 TrueType fonts
* Text, cells, multicells, aligned writing, styled paragraphs, links, bookmarks,
  and aliases
* Tables, static filled form documents, and optional typed block rendering
  through `WriteDocument`
* Drawing primitives: lines, rectangles, rounded rectangles, arcs, Bezier
  curves, polygons, paths, clipping, transforms, transparency, gradients, spot
  colors, and layers
* JPEG, PNG, GIF, WebP, SVG, data-image, QR-code PNG generation, and thumbnail
  workflows
* Controlled HTML/CSS fragment rendering through `HTMLNew`, including compiled
  templates, text styles, spacing, borders, backgrounds, and simple box shadows
* Reusable PDF templates and imported PDF pages
* Page workflows built from imported pages: merge, split, reorder, rotate,
  4-up layout, template overlay, and watermark overlay
* Attachments, metadata, XMP metadata, legacy PDF standard-security protection,
  PDF signing, CMS signature verification, and lightweight PDF inspection

## Current Limits

These are not implemented as general-purpose features:

* Full browser-compatible HTML/CSS layout
* JavaScript page rendering
* PDF JavaScript actions; `SetJavascriptError` returns
  `ErrJavaScriptUnsupported`, and `javascript:` URI links are rejected
* AES-based PDF document encryption; `SetAESProtection` returns
  `ErrAESProtectionUnsupported` instead of emitting partial encryption syntax
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
`make compliance-validate` with external validators for standards checks.

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
`assets/generated/pdf/examples`.

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
| Compiled HTML template values | `go run ./examples/html-template` | `html-template.pdf` |
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
document   main PDF generation API
layout     typed block model, geometry, pagination, and measurement primitives
font       font parsing and JSON font definition generation
importpdf  small wrappers around imported-page APIs
inspect    lightweight PDF structure, stream, page, and text inspection
sign       CMS-first PDF signing and signature verification
```

Useful repository directories:

```text
cmd/fontmaker          font definition generator
cmd/list               generated-reference listing utility
examples/              runnable examples
assets/static/         checked-in fonts, images, and text fixtures
assets/generated/pdf/  generated PDFs
tools/                 tool-only module for quality/security commands
```

## HTML Support

`HTMLNew()` renders a bounded HTML/CSS subset, not browser layout. `Write` uses
a shared compiled-plan cache; use `CompileHTML` and `WriteCompiled` when you
want to own the plan.

Choose the HTML API by what changes:

| Use case | API |
| --- | --- |
| Normal fragment; default shared cache is enough | `html.Write(lineHt, fragment)` |
| Explicit plan lifetime, diagnostics, or cross-document reuse | `document.CompileHTML` + `html.WriteCompiled` |
| One-off template that inserts trusted raw HTML or builds an image tag | `document.RenderHTMLTemplate` + `html.Write` |
| Same HTML shape with changing text, links, image sources, or dimensions | `document.CompileHTMLTemplate` + `html.WriteTemplate` |

Compiled template slots are allowed in text and safe attributes such as `href`,
`src`, `alt`, `width`, and `height`; they are rejected in tags, CSS, raw HTML,
event attributes, and `class`/`style`/`id`.

```go
template, err := document.CompileHTMLTemplate(`
    <h1>{{title}}</h1>
    <p>Customer: {{customer}}</p>
    <p><a href="{{url}}">{{url_text}}</a></p>
    <img src="{{logo}}" alt="{{logo_alt}}" width="55mm">
`)
if err != nil {
    return err
}

html := pdf.HTMLNew()
html.AllowLocalImages = true
err = html.WriteTemplateContext(ctx, 6, template, document.HTMLTemplateValues{
    "title":    "Invoice A-100",
    "customer": "Northwind",
    "url":      "https://example.com/invoices/A-100",
    "url_text": "Open invoice",
    "logo":     "/absolute/path/logo.png",
    "logo_alt": "Company logo",
})
if err != nil {
    return err
}
```

Use `RenderHTMLTemplate` instead when a value must change the HTML structure,
for example by inserting `HTMLTemplateRaw` or `HTMLTemplateImage`.

Supported content includes text styling, links, paragraphs, headings, lists,
tables, images, inline SVG, boxes, borders, spacing, colors, page breaks, and a
bounded flexbox subset for direct child blocks.

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

PDF signature helpers can extract `/ByteRange` content, compute digests, embed an
externally produced detached CMS signature, inspect CMS signer metadata, and read
the Adobe revocation-info archival signed attribute used by PAdES workflows:

```go
cms, encoding, err := sign.DecodeCMS(rawSignature)
revInfo, err := sign.ExtractAdobeRevocationInfo(cms)
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

v0.12 removes the legacy `sign/pkcs7` wrappers. Use the CMS names in `sign`.

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

## API Surface

Import `github.com/cssbruno/gopdfkit/document` for PDF generation and
`github.com/cssbruno/gopdfkit/layout` for typed models and pure layout
primitives. See `MIGRATION_v0.12.md` when upgrading from v0.11 or earlier.

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
make bench-generation-core-budget
make profile-cpu BENCH='BenchmarkGenerationHTMLLargeTableCompiled$'
make benchstat
```

`make bench-generation-core` runs representative generation benchmarks:
baseline, text, UTF-8 text, images, SVG, templates, imported pages, protection,
attachments, and compiled HTML tables. The budget target always captures a new
three-sample result before applying its broad regression ceilings. Use
`tools/bin/benchstat baseline.txt current.txt` for local statistical A/B
comparisons after `make benchstat`.

Test examples generate PDFs in a unique temporary directory, so running the
test suite never removes or overwrites repository assets.

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
