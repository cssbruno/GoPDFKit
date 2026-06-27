# Changelog

## v0.6.2 - 2026-06-27

Patch release for compliance baseline CI stability after v0.6.1.

### Fixed

- Ensured generated compliance fixture PDFs are readable by external validator
  containers.
- Preserved readable default permissions for new files written through
  `OutputFileAndClose` while keeping existing destination permissions.
- Normalized volatile Arlington validator metadata in compliance baseline
  comparisons.

## v0.6.1 - 2026-06-27

Patch release for the follow-up non-HTML performance checklist.

### Added

- Added the v0.6.0 pprof target checklist as a completed audit record.

### Changed

- Reduced allocation and CPU overhead in attachment output, SVG parsing and
  rendering, template serialization, font loading, layout rendering, QR image
  generation, signing dictionary updates, and PDF parser utilities.
- Regenerated tracked PDF fixtures after the output-path performance changes.

## v0.6.0 - 2026-06-27

Minor release for non-HTML generation performance work.

### Added

- Added reusable imported PDF source workflows, including immutable byte parsing,
  seekable `ReaderAt` parsing, and parsed source caching for repeated imports.
- Added the v0.5.6 non-HTML performance checklist as a completed audit record.

### Changed

- Reduced allocation and formatting overhead across text/cell rendering,
  document-model rendering, image output, page serialization, UTF-8 font
  subsetting, imported PDF output, signing, metadata, tags, and compliance paths.
- Preserved compatible imported FlateDecode page streams and cached rewritten
  imported object bodies when output mappings are unchanged.
- Regenerated tracked PDF fixtures after the performance changes.

### Security

- Updated `golang.org/x/image` to a WebP decoder release with current
  `govulncheck` fixes.

## v0.5.5 - 2026-06-13

Patch release for CI benchmark workflow cleanup.

### Fixed

- Updated the CI benchmark job to run the current
  `make bench-generation-core-ci` target after the external gopdfsuit comparison
  benchmark target was removed.

## v0.5.4 - 2026-06-13

Patch release for generation benchmark throughput cleanup after the v0.5.3
rollback.

### Changed

- Reverted the experimental PDF hot-path formatting helper extraction while
  keeping the benchmark suite focused on native GoPDFKit generation throughput.
- Expanded fixed 40-worker generation benchmark coverage for text, UTF-8 text,
  compression levels, images, SVG, templates, imported pages, protection, and
  attachments.
- Improved image benchmark throughput measurements with cached and uncached
  image rows.

## v0.5.3 - 2026-06-10

Patch release for native generation throughput and benchmark cleanup.

### Changed

- Replaced high-frequency PDF formatting calls in object output, xref writing,
  image placement, clipping, gradients, transforms, templates, tagged PDF
  references, and attachment output with scratch-buffer append helpers.
- Kept core generation benchmark reporting focused on fixed 40-worker native
  GoPDFKit workloads.

### Removed

- Removed the external PDF-library comparison benchmark module and its Makefile
  targets.
- Removed external comparison benchmark tables and documentation.

## v0.5.2 - 2026-06-09

Patch release for benchmark reporting and fixed 40-worker throughput
comparisons.

### Added

- Fixed-40-worker benchmark reporting for the core generation suite.
- Raw benchmark `pdf/s`, `pdf_bytes`, and `total_MB` metrics for PDF
  throughput, output size, and total timed-loop allocation reporting.
- No-image baseline rows and expanded non-HTML generation workload coverage in
  benchmark documentation.

### Changed

- `make bench-generation-core` now runs only 40-worker generation benchmarks.
- Benchmark snapshots now include 40-worker PDF/sec and total allocated MB
  columns.

## v0.5.1 - 2026-06-09

Patch release for the post-v0.5 compiled HTML performance work and benchmark
repeatability.

### Added

- Deeper reusable compiled HTML parse products: private AST node indexes,
  precomputed element declarations, cached block/table text, decoded data URI
  images, malformed-fragment diagnostics, and public compiled-plan stats.
- Selector-heavy, table-heavy, data-image-heavy, and malformed HTML generation
  benchmarks with single-worker and 40-worker rows.
### Changed

- `v0.5.0` remains on the original compliance release commit; the compiled HTML
  AST/tokenizer performance work is intended for `v0.5.1`.

## v0.5.0 - 2026-06-09

Minor release focused on PDF compliance metadata, tagged PDF output, external
validator fixtures, and generation benchmark coverage.

### Added

- PDF/A-4, PDF/A-4f, PDF/UA-2, Arlington, and XMP metadata support through
  `document.SetComplianceMetadata`.
- Tagged PDF generation foundations for text, links, images, lists, HTML
  structure, tables, artifacts, parent-tree entries, and structure namespaces.
- PDF/A-4 output requirements for embedded UTF-8 fonts, ToUnicode maps,
  output intents, trailer identifiers, and attachment metadata.
- Compliance fixture generation and local structural checks under
  `cmd/compliance-fixtures` and `cmd/compliance-check`.
- Docker-backed veraPDF and Arlington validation wrappers plus CI wiring for
  strict `REQUIRE_COMPLIANCE_TOOLS=1` validation.
- Passing external validator baselines for PDF/A-4, PDF/A-4f, PDF/UA-2, and
  Arlington PDF 2.0 under `testdata/compliance`.
- Compiled HTML rendering support with `document.CompileHTML` and
  `HTML.WriteCompiled`.
- Reusable image caching support with `document.ImageCache`.
- Expanded generation benchmarks for image caching, PDF/A-4f, PDF/UA-2,
  Arlington, XMP metadata, signing, and concurrent throughput.

### Changed

- HTML, SVG, imported page, drawing, link, image, and document-renderer output
  paths now propagate semantic tagging or artifact markers when tagged PDF mode
  is enabled.
- PDF 2.0 Arlington mode omits deprecated Info and ProcSet entries and writes a
  trailer ID.
- Documentation now includes compliance validation workflow, benchmark results,
  CPU model, and strict validator setup.

### Fixed

- Corrected generated UTF-8 ToUnicode CMap ranges so veraPDF accepts the
  embedded font maps.
- Added embedded-file MIME type and AFRelationship output for PDF/A-4f
  attachments.
- Adjusted compliance fixtures to pass real veraPDF PDF/A-4, PDF/A-4f,
  PDF/UA-2, and Arlington PDF 2.0 validation.

## v0.4.0 - 2026-06-04

Minor release focused on reusable PAdES/CMS revocation-info helpers.

### Added

- `sign.RevocationInfo`, `sign.OtherRevocation`,
  `sign.AdobeRevocationInfoArchivalOID`,
  `sign.DecodeAdobeRevocationInfo`, and `sign.ExtractAdobeRevocationInfo`.
- Matching `sign/pkcs7` wrappers for callers that still use PKCS #7 terminology.

## v0.3.0 - 2026-06-04

Minor release focused on reusable QR-code generation for PDF verification
workflows.

### Added

- `document.QRCodePNG`, `document.QRCodeImageName`, and `Document.RegisterQRCodePNG`.

## v0.2.0 - 2026-06-04

Minor release focused on CMS-first signing, PDF inspection, and reusable HTML
document helpers.

### Added

- CMS-first signing and verification APIs: `CreateCMS`, `VerifyCMS`,
  `VerifyDetachedCMS`, CMS decoding, signer-certificate inspection, signed
  attribute access, ByteRange helpers, detached CMS embedding, and digest
  helpers.
- `sign/pkcs7` as a separate legacy-terminology wrapper package around the CMS
  APIs.
- `inspect` package for lightweight PDF page count, page size, decoded stream,
  and literal text inspection.
- `document.ExtractHTMLFooterBlock` support for footer elements,
  `data-pdf-footer`, and common footer marker classes.

### Changed

- PDF signing now writes the CMS/CAdES detached subfilter and uses CMS naming
  in public docs.
- Documentation now separates CMS-first APIs from legacy PKCS #7 terminology.

## v0.1.1 - 2026-06-03

Patch release with performance fixes and internal robustness updates.

### Changed

- Optimized HTML table rendering by reducing repeated border, background, and
  text-measurement work during table layout.
- Reduced HTML table allocation pressure by pre-sizing table row/cell
  structures and avoiding full cell copies in layout placements.
- Updated document internals for image parsing, font limits, PDF import,
  signing, templates, and output helpers.
- Updated examples to match the current document and image APIs.
- Refreshed generated PDF fixtures and removed generated example PDFs that are
  no longer retained in the repository.

### Fixed

- Avoided unnecessary `rowspan` and `colspan` parsing work for normal HTML
  table cells without span attributes.
- Added image and font limit helpers plus security regression coverage for
  oversized or unsafe inputs.

## v0.1.0 - 2026-06-03

Initial cssbruno/gopdfkit release.

### Added

- Release tooling for semver tags, release checks, release notes, and tag publishing.
- GitHub Release workflow for tagged releases.
- Go quality tool commands for `golangci-lint`, `nilaway`, `gosec`, and `govulncheck`.
- `govulncheck` release gate with Go 1.26.3 toolchain baseline.
- `document.RenderHTMLTemplate`, `document.HTMLTemplateValues`,
  `document.HTMLTemplateRaw`, and `document.HTMLTemplateImage` for simple
  `{{key}}` substitution before HTML rendering.
- HTML examples for images, generated tables, and template values.

### Changed

- Trimmed the public package surface to real library packages and moved PDF comparison plus deterministic example helpers under `internal/`.
- Removed empty internal placeholder packages with no implementation.
- Removed barcode generation/rendering APIs and the `github.com/boombuler/barcode` dependency.
- Added runnable examples for text, drawing, headers and footers, HTML fragments, in-memory images, imported pages, protection and attachments, structured reports, signing, templates, thumbnails, UTF-8 fonts, and external QR-code images.
- Replaced `InitType`/`NewCustom` with `Options`/`NewWithOptions`.
- Simplified the root `gopdfkit.New` facade to the default constructor only.
- Removed deprecated image and template compatibility wrappers from the public API.
- Removed the oversized exported `document.Pdf` interface.
- Migrated examples and benchmarks to `ImageOptions`, `RegisterImageOptions`, and `RegisterImageOptionsReader`.
- Documented the pre-1.0 versioning policy: minor releases for new functions
  or breaking API changes, patch releases for bug fixes.
- Reduced the README gopher image size.

### Fixed

- Fixed document-model list and table cell rendering so nested content does not
  leak margins or overlap later form content.
- Replaced decorative barcode-like marks in generated examples with real Code
  39 barcodes.
- Updated the UTF-8 cut-font example to use the glyphs shown in the generated
  PDF and stable relative font labels.

### Known Quality Baseline

- `make check` and `make govulncheck` pass.
- `make quality` is intentionally strict and currently fails on existing lint, nilability, and security findings.
