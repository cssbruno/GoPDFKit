# Changelog

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
