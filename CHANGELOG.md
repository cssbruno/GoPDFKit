# Changelog

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
