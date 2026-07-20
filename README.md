# PaperRune

[![CI][badge-ci]][ci]
[![Custom license][badge-license]][license]
[![GoDoc][badge-doc]][godoc]

<img src="https://raw.githubusercontent.com/cssbruno/paperrune/main/assets/static/image/gopher_pdf.png" alt="PaperRune gopher" width="160">

PaperRune is a pure-Go PDF toolkit for Go applications that need to generate,
inspect, sign, import, or sanitize PDF documents without depending on a
browser. It keeps an FPDF-style API for familiar page, text, and drawing
workflows, while also providing bounded HTML/CSS rendering and typed document
models.

## Highlights

- Generate PDFs with text, tables, images, SVG, forms, templates, and metadata.
- Render controlled HTML/CSS fragments or compile reusable HTML templates.
- Import, merge, split, reorder, rotate, overlay, and watermark PDF pages.
- Sign and verify PDFs with CMS, inspect PDF structure, and sanitize uploads
  through a PDF Content Disarm and Reconstruction boundary.
- Use optional typed Go document models for applications that prefer structured
  data over HTML strings.

The main API is `github.com/cssbruno/paperrune/document`. The optional typed
model and measurement primitives live in
`github.com/cssbruno/paperrune/layout`. Ownership rules and public-surface
policy are documented in [`ARCHITECTURE.md`](ARCHITECTURE.md).

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Choosing a document path](#choosing-a-document-path)
- [Features](#features)
- [Limitations](#limitations)
- [Examples](#examples)
- [HTML support](#html-support)
- [Advanced workflows](#advanced-workflows)
- [Packages and development](#packages-and-development)
- [License](#license)

## Installation

PaperRune requires Go 1.26.5 or newer within the supported Go toolchain. The
library is pure Go; CI runs on Linux and cross-builds for Windows. Other
Go-supported platforms may compile but are not part of the compatibility
guarantee.

```shell
go get github.com/cssbruno/paperrune@latest
```

## Quick start

```go
package main

import "github.com/cssbruno/paperrune/document"

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

Construction can be configured with typed options:

```go
pdf := document.MustNew(
	document.WithOrientation(document.OrientationPortrait),
	document.WithUnit(document.UnitMillimeter),
	document.WithPageSize(document.PageSizeA4),
	document.WithBestCompression(),
)
```

`WithBestCompression` selects stronger zlib compression for generated page and
template streams. It is not a general-purpose optimizer for images, fonts,
object streams, or existing PDFs.

## Choosing a document path

| Need | Recommended API |
| --- | --- |
| Precise drawing, custom pagination, or FPDF-style control | `Document` drawing, text, and table methods |
| Report-like documents with changing values | `document.CompileHTMLTemplate` and `HTML.WriteTemplate` |
| One-off HTML fragments or rich text sections | `pdf.HTMLNew().Write` |
| Typed Go blocks without HTML strings | `layout.NewDocumentModel` and `Document.WriteDocument` |
| Large unsigned output with lower peak memory | `OutputFileStream` or `OutputOptions{StreamFinal: true}` |

## Features

- Pages, margins, headers, footers, page boxes, aliases, links, bookmarks, and
  page breaks.
- Standard PDF fonts and UTF-8 TrueType fonts, including OpenType files with
  TrueType outlines.
- Text, cells, multicells, aligned writing, styled paragraphs, tables, and
  static filled-form documents.
- Lines, rectangles, rounded rectangles, arcs, Bezier curves, polygons, paths,
  clipping, transforms, transparency, gradients, spot colors, and layers.
- JPEG, PNG, GIF, WebP, SVG, data images, QR-code PNG generation, and
  thumbnails.
- Reusable PDF templates and imported-page workflows for merging, splitting,
  reordering, rotating, 4-up layout, overlays, and watermarks.
- Attachments, document metadata, XMP metadata, legacy PDF standard-security
  protection, PDF signing, CMS verification, and lightweight inspection.
- PDF Content Disarm and Reconstruction through the `pdfcdr` package.

## Limitations

The following are intentionally not general-purpose features:

- Full browser-compatible HTML/CSS layout or JavaScript page rendering.
- PDF JavaScript actions. `SetJavascriptError` returns
  `ErrJavaScriptUnsupported`, and `javascript:` links are rejected.
- AES-based PDF document encryption. `SetAESProtection` returns
  `ErrAESProtectionUnsupported` instead of emitting partial encryption syntax.
- DOCX conversion or interactive AcroForm field creation.
- Filling, flattening, or FDF-merging existing interactive forms.
- Unlocking, decrypting, or removing passwords from existing PDFs.
- Arbitrary PDF editing, OCR, or universal semantic text extraction.

Imported pages support classic xref-table PDFs, unencrypted documents, and
pages whose content streams are unfiltered or FlateDecode-compressed. Xref
streams, object streams, unsupported filter chains, and ambiguous stream
lengths fail closed.

Password protection applies to newly generated output. Permission flags are
advisory because PDF readers decide how strictly to enforce them.

## Paper Engine preview

The repository includes an experimental human-readable `.paper` compiler and a
read-first Paper Studio workspace. Studio displays SVG pages captured from the
immutable display plan; it is not a browser-layout replacement and is not yet
the default document frontend.

```shell
go run ./cmd/paper check testdata/paper/studio-demo.paper
make paper-studio PAPER_STUDIO_FILE=testdata/paper/studio-demo.paper
```

Open `http://127.0.0.1:7331`. Studio intentionally accepts only an explicit
loopback listen host and refuses wildcard, LAN, and public bind addresses.
Run `make test-paper-studio-js` for the dependency-free page-rail model tests.
See [`PAPER_ENGINE_PLAN.md`](PAPER_ENGINE_PLAN.md) and the
[`.paper` asset guide](docs/paper-assets.md) for the current design.

### Responsive `.paper` rows and columns

Rows and columns use a deterministic, fixed-point auto-layout solver. Main-axis
sizes can be intrinsic (`"auto"`), container-relative percentages, or flex
bases with grow/shrink factors. Percentages are resolved from the containing
layout region at plan time, so rendering at 50%, 100%, HiDPI, or print DPI does
not change document geometry.

```text
row @cards:
  gap: 3mm
  cross-size: 45mm
  cross-gap: 3mm
  wrap: "wrap"
  main-align: "space-between"
  cross-align: "stretch"
  align-content: "space-around"
  paragraph @primary:
    track: "flex"
    track-size: 50%
    track-grow: 1
    track-shrink: 1
    track-min: 30%
    text: "Primary"
  paragraph @secondary:
    track: "flex"
    track-size: "auto"
    track-grow: 1.5
    track-shrink: 1
    cross-align: "center"
    text: "Secondary"
```

`wrap` accepts `nowrap`, `wrap`, and `wrap-reverse`. Main alignment accepts
`start`, `center`, `end`, `space-between`, `space-around`, and `space-evenly`;
`align-content` also accepts `stretch`. Physical units such as `pt`, `mm`,
`cm`, and `in` remain appropriate for page and print constraints because they
are device-independent rather than screen pixels.

### Render application JSON and check generated edge cases

A `.paper` document can declare a typed schema and receive ordinary JSON at
render time. Types lead each declaration; required fields are the default and
`optional` is explicit. Reusable custom objects live at document scope and can
be used directly by fields and bounded lists:

```paper
object Patient:
  string name

object Medication:
  string name
  string directions

schema:
  string clinic
  optional string phone
  Patient patient
  optional Patient responsible
  list Medication medications:
    max-items: 10
```

Use inline `object fieldName:` or `list object fieldName:` when the shape is
local to one field. Custom object names must resolve to a document-scoped
`object Name:` declaration; unknown names and recursive object graphs are
compile errors.

The JSON adapter rejects unknown fields, missing required fields, wrong types,
duplicate keys, and lists beyond their declared bounds before any PDF state is
changed. A document with one schema uses root-relative paths such as
`bind: "patient.name"` and `source: "medications"`.

When several schemas are intentionally declared, give them bare names and use
that name only to disambiguate a path:

```paper
schema invoice:
  number total

schema receipt:
  string reference

paragraph:
  bind: "invoice.total"
```

The removed `field @name`, `type:`, `required:` and `@schema.field` forms are
not accepted by the grammar.

```shell
go run ./cmd/paper check --data report.json report.paper
go run ./cmd/paper render --data report.json -o report.pdf report.paper
```

`check --edge-cases` generates fixed boundaries for empty text, whitespace,
multiline text, excessive wrapping, 256-character unbroken strings, dense lists,
Portuguese Unicode, punctuation/escaping, and numeric extremes before seeded
random cases. Every case is schema-validated and runs through planning,
painting, structural PDF parsing, page-count comparison, and text extraction.
A seed makes failures replayable.

`--edge-output` keeps every input and PDF plus `edge-report.json`. The report
records empty/whitespace/multiline string counts, the JSON Pointer and size of
the longest string and largest list, input/PDF hashes, per-page extracted-text
hashes, exact page summaries, and positioned layout issues. Add `--edge-visual`
to create SVG contact sheets and a portable `edge-gallery.html` for side-by-side
human review.

```shell
go run ./cmd/paper check --edge-cases 16 --seed 42 report.paper
go run ./cmd/paper check --edge-cases 16 --seed 42 --edge-output ./edge-artifacts report.paper
go run ./cmd/paper check --edge-cases 16 --seed 42 --edge-output ./edge-artifacts --edge-visual report.paper
```

Use `--schema NAME` when the document declares multiple schemas and
`--locale pt-BR` when presentation does not already inherit an explicit
locale. See the [Brazilian lab example](examples/paper-lab-report/README.md).

## Examples

Runnable examples live under [`examples/`][examples]. They write generated PDFs
to `assets/generated/pdf/examples`.

| Workflow | Command | Output |
| --- | --- | --- |
| Hello world | `go run ./examples/hello-world` | `hello-world.pdf` |
| Drawing primitives | `go run ./examples/drawing` | `drawing.pdf` |
| Report | `go run ./examples/report` | `paperrune-report.pdf` |
| Structured report | `go run ./examples/structured-report` | `generated PDF` |
| Table report | `go run ./examples/table-report` | `paperrune-tables.pdf` |
| HTML fragment | `go run ./examples/html-fragment` | generated PDF |
| HTML template | `go run ./examples/html-template` | generated PDF |
| Images and SVG | `go run ./examples/html-images` | generated PDF |
| Import pages | `go run ./examples/import-page` | generated PDF |
| Merge and reorder | `go run ./examples/merge-pdf-pages` | generated PDF |
| Password protection | `go run ./examples/protect-pdf` | generated PDF |
| Signing | `go run ./examples/sign-pdf` | `signed.pdf` |

Use `Document.RegisterQRCodePNG` for QR-code verification blocks.

## HTML support

`HTMLNew()` renders a bounded HTML/CSS subset, not browser layout. `Write` uses
a shared compiled-plan cache under the default resource policy; applications
that need explicit plan ownership can use `CompileHTML` and `WriteCompiled`.

Public compilation and tokenization accept at most 4 MiB and 100,000 HTML
tokens. Rendering a compiled plan still applies the destination
`HTML.MaxHTMLBytes`; compiled template values are also size-checked before
replacement.

Choose the API by what changes:

| Use case | API |
| --- | --- |
| Normal fragment | `pdf.HTMLNew().Write` |
| Explicit plan lifetime or diagnostics | `document.CompileHTML` and `html.WriteCompiled` |
| One-off template with trusted raw HTML | `document.RenderHTMLTemplate` |
| Same HTML shape with changing values | `document.CompileHTMLTemplate` and `html.WriteTemplate` |

Compiled template slots are allowed in text and safe attributes such as
`href`, `src`, `alt`, `width`, and `height`. They are rejected in tags, CSS,
raw HTML, event attributes, and `class`/`style`/`id`.

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

Supported content includes text styling, links, paragraphs, headings, lists,
tables, images, inline SVG, boxes, borders, spacing, colors, page breaks, and
a bounded flexbox subset for direct child blocks.

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

`Stats` reports reusable parse-product counts. `RecoveryIssues` reports
unclosed, misnested, or unexpected closing tags. `DebugDump` is for human
diagnostics and is not a stable wire format.

## Advanced workflows

### Typed document models

Use `layout.NewDocumentModel` when an application wants typed Go blocks instead
of HTML templates. The `document` package renders the model with
`WriteDocument` without duplicating the layout API.

```go
model := layout.NewDocumentModel(
	"Receipt A-100",
	layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "Thank you for your purchase."}},
	},
)

pdf.WriteDocument(model)
```

Keep product-specific document names and builders in the application rather
than adding business-specific concepts to the shared library.

### Large PDF output

Normal `Output` and `OutputFile` calls keep final PDF bytes in the document
buffer, which allows repeated output from one `Document`. For very large
unsigned PDFs where peak memory matters more than repeatability, use:

```go
err := pdf.OutputFileStream("large.pdf")
```

Streaming final output is opt-in, consumes the document for final output, and
is disabled for signed output because signing needs the complete byte range.

### Fonts

Standard PDF fonts require no setup:

```go
pdf.SetFont("Helvetica", "", 12)
```

Use `AddUTF8Font` or `AddUTF8FontFromBytes` for UTF-8 TrueType fonts. Use
`RTL()` and `LTR()` to switch right-to-left rendering mode.

For non-UTF-8 TrueType, OpenType/CFF, or Type1 fonts, generate a JSON font
definition with `font.Make` or `cmd/fontmaker`:

```shell
cd cmd/fontmaker
go build
./fontmaker --embed --enc=../../assets/static/font/cp1252.map --dst=../../assets/static/font ../../assets/static/font/calligra.ttf
```

### Signing and verification

Documents can be signed while writing:

```go
err := pdf.OutputSignedFile("signed.pdf", sign.Options{
	Signer:      signer,
	Certificate: cert,
})
```

The `sign` package uses CMS terminology and supports detached CMS creation,
PDF signature extraction and embedding, signer metadata inspection, and
trusted verification. Trusted verification requires a non-nil root
certificate pool; use the explicitly named integrity-only APIs only when signer
trust is not part of the decision.

### Inspection and sanitization

Use `inspect` for lightweight PDF checks and text extraction from literal text
operators:

```go
count, err := inspect.PageCount(pdfBytes)
text, err := inspect.Text(pdfBytes)
streams, err := inspect.DecodedStreams(pdfBytes)
```

Use `pdfcdr` when an uploaded PDF must cross a reconstruction boundary before
being opened or stored:

```go
clean, err := pdfcdr.Sanitize(input)
if err != nil {
	return err
}
```

CDR removes document actions, annotations, forms, JavaScript, embedded files,
external references, metadata, and unreachable objects while preserving page
painting commands and reachable rendering resources. It is a structural
reconstruction boundary, not malware detection or a substitute for isolating a
downstream PDF viewer.

### Compliance metadata

`document.SetComplianceMetadata` provides PDF/A-4, PDF/UA-2, Arlington, and XMP
metadata foundations. PDF/A mode enforces known generation blockers such as
encryption and JavaScript restrictions, output intents, and embedded fonts.
PDF/UA-2 mode emits tagged PDF structures and semantic content metadata.

This is not a validator replacement. Use `make compliance-fixtures` and
`make compliance-validate` with the external validators required by your
workflow.

## Packages and development

| Package | Purpose |
| --- | --- |
| `document` | Main PDF generation and rendering API |
| `layout` | Typed block model, geometry, pagination, and measurement primitives |
| `font` | Font parsing and JSON font definition generation |
| `importpdf` | Bounded classic-xref parser and imported-page resolver |
| `inspect` | Lightweight PDF structure, stream, page, and text inspection |
| `pdfcdr` | PDF Content Disarm and Reconstruction |
| `sign` | CMS-first PDF signing and signature verification |

Useful repository directories:

```text
cmd/                    command-line tools and Paper Studio
document/               main PDF API
layout/                 renderer-independent document model
examples/               runnable examples
assets/static/          checked-in fonts, images, and fixtures
assets/generated/pdf/   generated example PDFs
tools/                  tool-only module and validation helpers
```

Common development commands:

```shell
go test ./...
go vet ./...
go list ./...
make check
make test-paper-studio-js
```

Test examples generate PDFs in unique temporary directories, so running the
test suite does not remove or overwrite repository assets. See
[`CHANGELOG.md`](CHANGELOG.md) for historical API changes.

## Errors

Most `Document` methods record errors on the document instead of returning
errors directly. Once an error is set, later methods usually return without
changing the PDF. Check `Ok()`, `Err()`, or `Error()` after generation,
especially before trusting output.

Applications can transfer their own failures into the PDF object with
`SetError()` or `SetErrorf()`.

## License

PaperRune is released under the PaperRune Health-Sector Restricted License 1.0.
Use, modification, and distribution are free for Non-Health-Sector Use under
the terms of [`LICENSE`][license]. Health-Sector Organizations and vendors
acting for them must obtain a separate written commercial license before
making Health-Sector Use. This is a source-available license, not an
OSI-approved open-source license. For licensing requests, contact the project
maintainers through the repository's normal project contact channel.


[badge-ci]: https://github.com/cssbruno/paperrune/actions/workflows/ci.yml/badge.svg
[badge-doc]: https://img.shields.io/badge/godoc-PaperRune-blue.svg
[badge-license]: https://img.shields.io/badge/license-custom-orange.svg
[ci]: https://github.com/cssbruno/paperrune/actions/workflows/ci.yml
[examples]: examples
[fpdf-site]: http://www.fpdf.org/
[godoc]: https://pkg.go.dev/github.com/cssbruno/paperrune
[license]: https://raw.githubusercontent.com/cssbruno/paperrune/main/LICENSE
