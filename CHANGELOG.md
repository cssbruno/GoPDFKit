# Changelog

## v0.16.0-rc.1 - 2026-07-20

First release candidate under the independent PaperRune project identity.

### Added

- Added repeatable real-world JSON edge inputs, explicit page-issue, text, and
  page-count thresholds, and deterministic baseline regression comparison to
  `paper check`.
- Added final-PDF visual evidence: Poppler rasterizes every written PDF page to
  a hashed PNG and PaperRune assembles the results into a PDF review book.

### Changed

- Renamed the project, Go module, commands, package imports, documentation, and
  release identity from GoPDFKit to PaperRune.
- Made edge checks fail when generated PDFs contain positioned layout issues
  above the configured threshold instead of merely recording those issues.
- Removed the HTML/SVG edge gallery so visual review cannot be mistaken for
  evidence from the final PDF artifact.

### Verification

- Edge reports now bind input, plan, PDF, extracted text, per-page raster, and
  acceptance-policy evidence in report format 3.
- Automated repository gates and the laboratory PDF visual review are required
  before this candidate is published. Elapsed stabilization and named external
  security acceptance remain external release gates and are not self-certified.

## v0.15.0-rc.1 - 2026-07-18

Release candidate for the breaking pre-1.0 unified automatic-layout engine and
Paper Studio authoring foundation.

### Changed

- Made typed and supported HTML document entry points lower through the unified
  planner and painter while retaining their public compatibility signatures.
- Removed the legacy typed measurement and direct HTML automatic-layout
  production engines. Unsupported contracts now fail atomically instead of
  retrying through a hidden compatibility renderer.
- Added the Stage 9 Paper Studio semantic editing, review, accessibility, and
  exact-revision delivery foundations.
- Added migration guides for typed callers, HTML callers, and legacy-engine
  deletion, including the remaining stabilization-window release contract.

### Verification

- Full Go tests, vet, the full race suite, Paper Studio JavaScript tests,
  generation budgets, and the calibrated Paper Engine benchmark gate pass on
  the release candidate branch.
- The stabilization-window record and formal rollback closure remain final
  release prerequisites; the `v0.15.0-rc.1` prerelease tag starts the
  stabilization window and does not itself close Stage 10.

## v0.14.0 - 2026-07-16

### Security

- Hardened classic-xref parsing against integer overflow, cumulative xref
  exhaustion, stale-generation resurrection, cyclic indirect lengths, missing
  or out-of-bounds stream lengths, malformed filter chains, duplicate page-tree
  nodes, and excessive indirect-object depth.
- Restricted PDF signature discovery to signature values reachable through the
  current catalog's AcroForm field tree, so names in comments, strings, stream
  payloads, unrelated dictionaries, and unreferenced objects cannot be selected
  as real fields; added generation-aware incremental-xref handling and rejected
  underreported trailer sizes before allocating signature objects.
- Made CDR reconstruction fail closed on active resource objects, ambiguous
  resource roles, producer-name context confusion, malformed stream suffixes,
  and cumulative resource/output limits. Shared resources are now retained and
  budgeted once across pages.
- Corrected legacy RC4 object encryption so every string and stream starts from
  a fresh cipher state, and stopped encrypted stream output from mutating
  caller-owned buffers.

### Fixed

- Corrected Symbol core-font metrics and BaseFont output, supplementary-rune
  validation, finite word spacing, UTF-8 spacing arrays, `MultiCell` wrapping
  and state restoration, and nil handling across HTML, image, SVG, font, and
  template boundaries.
- Unified typed-layout measurement and rendering for headers, kept block
  chains, titled clauses, sections, note boxes, tables, and whitespace-only
  titles; removed measurement-only PDF content commands while preserving the
  visually rendered golden output.
- Reworked stream inspection around actual live xref objects and declared
  lengths, preventing stream payloads from being interpreted as indirect PDF
  objects and avoiding repeated whole-file scans.
- Made document, CDR, and signing file output sync the temporary file before
  atomic rename and sync the containing directory on supported platforms.
- Made CDR output byte-idempotent, including source content with LF, CRLF, or
  repeated trailing line endings.

### Changed

- Added malformed-input and end-to-end regressions, whole-document CDR fuzzing,
  signature-parser fuzzing, expanded coverage floors, NilAway in CI, a Windows
  cross-build, strict release-version validation, and Poppler fixture smoke
  tests. Consolidated recurring fuzzing and removed duplicate release and
  platform test runs to reduce GitHub Actions usage.
- Removed the stale generated-reference command and the nested external QR-code
  example module; QR generation remains available through the main package.
- Documented supported platforms, signing limitations, security reporting, and
  the contributor verification workflow.

## v0.13.1 - 2026-07-15

Patch release fixing PDF CDR rendering-resource reconstruction across a wider
range of valid producer output.

### Fixed

- Fixed PDF CDR reconstruction of non-sequential rendering-resource references,
  preventing fonts, images, and form XObjects from becoming invalid after
  sanitization.
- Distinguished real PDF stream markers from `stream` text used in strings or
  names while continuing to support delimiter-adjacent stream markers.
- Preserved producer-defined names inside direct and indirect rendering-resource
  dictionaries without weakening inline action removal.

## v0.13.0 - 2026-07-11

Minor release adding PDF Content Disarm and Reconstruction support and lower
allocation PDF reconstruction paths.

### Added

- Added the `pdfcdr` package with bounded PDF reconstruction that removes
  actions, JavaScript, annotations, attachments, metadata, and unreachable
  objects while preserving page content and rendering resources.
- Added CDR file and context-aware APIs, benchmark coverage, nesting-limit
  tests, and fuzz coverage for the PDF value sanitizer.

### Changed

- Added borrowed page-content and resource accessors to avoid redundant copies
  during reconstruction.
- Reduced CDR reconstruction allocations by reusing sanitized objects,
  avoiding unnecessary reference sorting, and preallocating final output.

## v0.12.3 - 2026-07-11

Compliance-baseline follow-up to the v0.12.2 security hardening release.

### Fixed

- Regenerated the pinned veraPDF PDF/A-4e, PDF/A-4f, and signed PDF/A-4f
  validation counts after confirming that every fixture remains compliant.

## v0.12.2 - 2026-07-11

Security hardening release with bounded untrusted-input parsing, explicit
signature trust semantics, immutable CI dependencies, and preserved PDF output
determinism.

### Security

- Prevented server-safe documents from retaining caller HTML or SVG source in
  process-global caches, and enforced generated-page limits while automatic
  HTML text pagination is occurring.
- Bounded public HTML compilation, tokenization, compiled-plan rendering, and
  compiled-template value expansion; preflighted SVG XML depth and node counts
  before recursive decoding.
- Bounded signing input size, incremental xref traversal, and xref entry scans,
  and split trusted signature verification from explicitly named integrity-only
  verification.
- Added aggregate serialized-template node, image, and page budgets and rejected
  trailing bytes after a decoded template.
- Pinned GitHub Actions and compliance container images to immutable revisions,
  narrowed workflow permissions, and isolated release write permissions to the
  publishing job.
- Replaced blanket static-analysis exclusions with narrow reviewed suppressions,
  and updated the required Go patch release to 1.26.5.

### Fixed

- Updated the veraPDF PDF/A-4 compliance baseline to match the validated
  v0.12.1 fixture output.

## v0.12.1 - 2026-07-11

Patch release for safer text and HTML rendering, tighter internal ownership,
and lower allocation pressure in document generation.

### Fixed

- Made HTML rendering select Helvetica when callers have not explicitly chosen
  a font, preventing the empty-width-table panic reported by an external
  benchmark.
- Made text rendering and measurement APIs report a document error when no font
  is selected, and safely handle incomplete font-width tables instead of
  indexing outside their bounds.
- Made typed table measurement and rendering share column constraints, span
  geometry, pagination boundaries, repeated headers, cell alignment, and box
  styling, with typed-versus-HTML parity coverage.
- Rendered styled and linked text segments, real QR images, document language,
  signature field names, and requested signature-column widths from the typed
  layout model.
- Applied output options atomically so a later validation error cannot leave
  earlier settings partially mutated.

### Changed

- Moved shared geometry into a private internal package while preserving the
  existing `layout` compatibility functions.
- Moved resource initialization and PDF object-number allocation behavior onto
  their concrete private owners and separated resource object numbers from
  attachment caches.
- Routed output orchestration through private buffered, streaming, and signing
  primitives instead of re-entering public wrappers.
- Replaced the PDF inspection MediaBox regex with the real importer parser and
  bounded importer sources with a sentinel `ErrSourceTooLarge` error.
- Made page-compression workers explicitly cancellable and bounded the string
  width cache with constant-time ring eviction.
- Reused bounded page-buffer capacity from the preceding page to reduce
  allocation churn without retaining unusually large buffers.
- Moved compiled HTML traversal stacks into a private render session, interned
  repeated table-cell appearances, and reused bounded content-command buffers.
- Reused one parsed SFNT directory and Unicode cmap when building cached UTF-8
  font definitions and immutable subset tables.

### Removed

- Removed the redundant `importpdf` capability interfaces and forwarding
  helpers; applications call `document.Document` import methods directly.
- Removed benchmark execution from CI. Benchmarks and profiling remain
  available as explicit local Make targets.

## v0.12.0 - 2026-07-09

Intentionally breaking pre-v1 release that makes ownership and package
boundaries explicit. See `MIGRATION_v0.12.md` for exact replacements.

### Changed

- Made `document` the only high-level PDF API and `layout` the direct owner of
  typed models, geometry, pagination, and measurement primitives.
- Replaced all document construction variants with `NewDocument`, `MustNew`,
  and `NewDocumentWithDefaults`, configured through typed functional options.
- Made defaults immutable and document-scoped, eliminating package-global
  configuration and test-order coupling.
- Split `Document` serialization, resource/cache ownership, page geometry,
  metadata/compliance, and security/runtime policy into concrete private state
  owners while preserving the public `Document` facade.
- Collapsed construction normalization and operational policy into one private
  configuration path and centralized every output variant behind one private
  coordinator.
- Shared pure image-fit, measurement, table geometry, and pagination behavior
  between typed layout and HTML rendering with parity coverage.
- Kept benchmarks as explicit local commands while excluding them from CI and
  release workflows.

### Removed

- Removed the root `paperrune` facade package and all mirrored declarations.
- Removed `document` aliases for `layout` types, constructors, and measurement
  helpers.
- Removed legacy string constructors, the exported `document.Options` bridge,
  mutable package-default setters, and deprecated compatibility names listed in
  the migration guide.
- Removed the legacy `sign/pkcs7` wrapper package and nonstandard
  `Document.CurveCubic` alias.

## v0.11.3 - 2026-07-09

Patch release for incremental Document ownership boundaries, centralized output
coordination, and stable release automation.

### Changed

- Moved PDF serialization/object-number and resource-ownership state behind
  private embedded owners while preserving the `Document` facade.
- Routed file, writer, streaming, option, and signing output variants through
  shared private output coordinators.
- Shared renderer-independent image-fit and page-break primitives between
  typed layout and HTML rendering, with parity coverage.
- Documented `document` as the canonical API; root and layout aliases remain
  compatibility-only until the next major release.
- Removed benchmark execution from CI and release automation. Benchmarks remain
  available as explicit local commands.

## v0.11.2 - 2026-07-09

Patch release for safer PDF parsing, deterministic test isolation, and
enforced release-quality checks.

### Fixed

- Parsed imported PDFs from a stable in-memory snapshot and recognized object
  boundaries only outside PDF streams, strings, names, and comments.
- Correctly calculated page dimensions from all `/MediaBox` coordinates and
  bounded decoded stream and DER child processing.
- Prevented generated test PDFs from modifying repository assets.
- Updated the nested external QR-code module dependency set.

### Changed

- Consolidated document output option application and removed unused internal
  forwarding helpers.
- Enforced nested-module, coverage, lint, security, race, fuzz, benchmark,
  and external-compliance checks in CI and release automation.

## v0.11.1 - 2026-06-30

Patch release for HTML table CSS rendering and a Brazilian lab-report example.

### Added

- Added a runnable Brazilian hemograma HTML example using flex layout, spans,
  compact tables, and a clinical-report style header/footer.

### Fixed

- Applied selector-resolved table cell CSS to HTML table padding, background,
  borders, and alignment in the compiled rendering path.
- Allowed CSS table cell padding rules to reduce row height instead of being
  clamped by the default table padding fallback.

## v0.11.0 - 2026-06-30

Minor release for faster HTML rendering, compiled HTML templates, and a bounded
flexbox subset.

### Added

- Added `CompileHTMLTemplate`, `CompileHTMLTemplateContext`,
  `HTML.WriteTemplate`, and `HTML.WriteTemplateContext` for fixed HTML/CSS
  templates with changing text and safe attribute values.
- Added a bounded shared compiled-HTML cache used by `HTML.Write` by default.
- Added a bounded flexbox subset for direct child blocks, including row and
  column direction, wrapping, gaps, main/cross-axis alignment, order, flex
  grow/shrink/basis, min/max sizing, and direct text flex items.
- Added support for `<span>` in the HTML renderer validation path.
- Added compiled-template benchmarks and focused tests for dynamic HTML values.
- Added runnable report examples that exercise compiled HTML, flex layout,
  styled spans, and richer table documents.

### Changed

- Repositioned compiled HTML templates as the preferred path for report-like
  documents with fixed HTML/CSS and changing values.
- Reframed the `layout` package documentation as optional typed block
  infrastructure instead of the default template system.
- Updated the HTML template example to use `CompileHTMLTemplate` and
  `WriteTemplateContext`.
- Kept README as the canonical documentation source and stopped regenerating it
  from `doc/document.md`.

### Fixed

- Preserved leading whitespace correctly when collapsed HTML text begins with a
  newline before visible text.
- Updated HTML validation diagnostics so supported flex `display` values no
  longer report as unsupported while unsupported values such as `grid` still do.

### Removed

- Removed the duplicate `doc/document.md` README source.

## v0.10.1 - 2026-06-29

Patch release for UTF-8 generation benchmark budget stability.

### Fixed

- Reused cached UTF-8 font IDs instead of re-hashing full font definitions for
  every document.
- Reduced UTF-8 CID width-map allocations by emitting compact `/W` arrays
  directly during font output.

## v0.10.0 - 2026-06-29

Minor release for v0.9 production-policy semantics, API polish, and the
breaking pre-v1 layout model cleanup.

### Added

- Added `layout.NewDocumentModel` and `document.NewDocumentModel` as neutral
  helpers for assembling a title plus caller-supplied layout blocks.
- Added tri-state `CompressionMode` values for `CompressionPolicy`, plus
  explicit worker-default and worker-disabled constants.
- Added `OutputWithOptionsContext`, `OutputFileWithOptionsContext`,
  `OutputSignedFileContext`, `OutputSignedWithOptionsContext`, and
  `OutputSignedFileWithOptionsContext`.
- Added `SetLimits`, `SetSecurityPolicy`, `SetHooks`, and
  `SetProductionPolicy` for legacy-constructed documents.
- Added `WithServerSafeDefaults` and `OutputFile` convenience wrappers for the
  v0.9 production entry points.
- Added the `ProtectionLegacyRC4` algorithm marker for the legacy protection
  compatibility path.
- Added `SetAESProtection` and `ErrAESProtectionUnsupported` to make AES-based
  PDF encryption explicitly unsupported instead of partially implemented.
- Added `importpdf.ImportOptions` and `Open*WithOptions` parsing entry points.
- Added `TemplateDecodeOptions`, `DeserializeTemplateWithOptions`, and
  `TemplateFingerprintVersion`.
- Added `TemplateView`, `TemplateChildrenView`, `PagedTemplate`, and
  `SerializableTemplate` as narrow additive template interfaces.
- Added `ErrImageTooLarge` for source and decoded image limit failures.
- Added `Document.Stats`, cache `Stats`/`Clear` methods, shared cache stats,
  and shared cache clearing helpers.
- Added opt-in `OutputStream*` and `OutputFileStream*` methods that stream
  unsigned final PDF serialization directly to the caller or atomic temp file.
- Added `OutputOptions.StreamFinal`, `OutputPolicy.StreamFinal`, and
  `WithOutputPolicy` so memory-constrained callers can route ordinary output
  methods through the streaming final writer explicitly.
- Added the `v0.10` architecture roadmap and the pre-`v1.0` API freeze map.

### Changed

- Fixed partial `CompressionPolicy` structs so setting only `Level` or worker
  counts no longer disables compression.
- Made partial `ProductionPolicy` cache behavior server-safe by defaulting to
  document-local caches unless shared caching is explicitly selected by a
  preset or `CacheSet`.
- Routed document import limits into `importpdf` parser options and aligned
  batch presets with parser hard caps where appropriate.
- Passed output context through attachment loading and attachment compression
  scheduling.
- Moved resource cache hit/miss hooks to real image/font cache boundaries and
  documented hook concurrency requirements.
- Made output-with-options restore output settings when canceled before close.
- Converted DER/CMS encoding panics into signing errors at the CMS construction
  boundary.
- Preserved render-only `TemplateView` child dependencies when creating parent
  templates, while keeping serialized-template output limited to serializable
  child templates.
- Routed final PDF serialization writes through a counted internal output sink
  so object offsets are no longer tied directly to `Document.buffer`.
- Moved central PDF object, dictionary, and stream boundary writes behind
  internal syntax helpers as groundwork for the v0.10 writer cleanup.
- Started internal runtime-policy consolidation so constructors and
  `SetProductionPolicy` apply production/cache/compression/output settings
  through one transition snapshot.
- Started the internal `ResourceStore` transition by binding new documents to a
  store-owned set of resource maps and routing stats/final-size estimates
  through that store.
- Routed initial image, template, imported-object, and imported-page
  registration writes through internal `ResourceStore` methods.
- Routed embedded-attachment stream, filespec, and compressed-stream object
  caches through internal `ResourceStore` methods.
- Routed core, cached UTF-8, byte-backed, reader-backed, and output-time font
  registration updates through internal `ResourceStore` methods.
- Routed resource dictionary, image/template/import output, compliance checks,
  and image lookup reads through internal `ResourceStore` helpers.
- Added internal PDF resource-name helpers for font, image, template, and
  imported-page references so output-local names are less coupled to resource
  identity fields.
- Added generalized `ResourceLoader` APIs and routed image registration,
  file-backed attachments, font resources, and string-based PDF imports through
  the loader when installed.

### Fixed

- Used `ForEachObjectBorrowed` internally when rewriting imported PDF objects to
  avoid unnecessary copies.
- Returned `ErrImageTooLarge` instead of `ErrUnsupportedImageType` for image
  size-limit failures.
- Expanded fuzz seeds and targets for imported PDFs, image formats, serialized
  templates, inspect stream/text extraction, PDF literal escaping, and DER/CMS
  parsing and verification.

### Removed

- Removed PaperRune-owned `DocumentKind` values and the named document model
  builders. This is a breaking pre-v1 API change: PaperRune now exposes layout
  primitives and model assembly tools; application-specific document categories
  should live in caller code.

### Migration

- Replace `document.NewLayoutDocument(document.DocumentKindReport)` and other
  kind-based construction with `document.NewLayoutDocument()`.
- Replace `document.NewGenericDocument("Title", blocks...)` with
  `document.NewDocumentModel("Title", blocks...)`.
- Replace report, transactional, attestation, and statement builders with
  caller-owned functions that return `*document.LayoutDocument`.

## v0.9.0 - 2026-06-28

Production-stability release for the pre-v1.0 API contract.

### Added

- Added `ProductionPolicy`, `Limits`, `SecurityPolicy`, `OutputPolicy`, and
  `Hooks` for server and batch production profiles.
- Added `ServerSafePolicy`, `BatchPolicy`, `DeterministicPolicy`,
  `ServerSafeLimits`, `BatchLimits`, and deterministic defaults.
- Added output-wide options and context-aware output entry points for normal and
  signed PDF output.
- Added parser-level context cancellation for the built-in PDF importer,
  imported-page lazy content, HTML tokenization/compilation, SVG parsing, image
  reader parsing, signing analysis/scanners, and context-aware inspect helpers.
- Added bounded, context-aware attachment loader APIs.
- Added temp-file spooling for large deferred attachment compression to avoid
  retaining duplicate heap copies before final PDF assembly.
- Added automated generation benchmark budget checking.
- Hard-disabled PDF JavaScript action output through
  `ErrJavaScriptUnsupported` and added security gates for local HTML images,
  file-backed attachments, raw writes, legacy RC4 protection, PDF import, and
  PDF signing.
- Added sentinel errors for production error handling and initial fuzz targets
  for HTML, SVG, template deserialization, image parsing, imported PDFs, and
  CMS/DER parsing.
- Added `TemplateSerializationVersion` and external validation integration
  contracts for compliance workflows.

### Changed

- Made image resource identity SHA-256-based and intrinsic to image content
  rather than output object numbers or document unit scale.
- Enforced production limits for attachments, image source bytes, estimated
  decoded image bytes, HTML input/generated pages, page count, imported PDF
  source bytes, and imported page referenced objects.
- Exposed output and production policy helpers through the root `paperrune`
  facade.
- Documented v0.9 production usage, security posture, deterministic output,
  migration guidance, readiness gates, and benchmark budgets.

### Fixed

- Made legacy protection setup panic-free by returning/latching random-source
  errors.
- Fixed embedded attachment name spelling from `Attachement` to `Attachment`.

## v0.8.0 - 2026-06-28

Minor release for cache, output, and error-handling hardening.

### Added

- Added explicit resource cache policies for shared, per-document, and disabled
  file-backed image and UTF-8 font caching.
- Added direct error-returning constructors and registration methods for
  per-document defaults, image registration, and font registration.

### Changed

- Replaced public mutable function aliases with wrapper functions.
- Replaced pointer-based font subset and document rendering cache keys with
  stable content-based keys or simpler local measurement paths.
- Routed signed file output through the same atomic temp-file output path as
  normal PDF file output.
- Split image parsing away from temporary `Document` construction and preserved
  PDF version promotion for alpha images, including cached images.
- Kept explicit per-document defaults independent from package-wide defaults.

## v0.7.0 - 2026-06-28

Minor release for the second-pass generation performance work.

### Added

- Added count-only wrapped text measurement APIs for UTF-8 and single-byte text.
- Added file-backed attachment descriptors and configurable file-output sync
  behavior.

### Changed

- Reduced allocation overhead across UTF-8 wrapping, text measurement,
  PDF-literal escaping, RTL text output, HTML table and block layout, SVG text
  roles, image caching, page compression, imported PDF output, signing xref
  lookup, and UTF-8 font subsetting.
- Cached reusable UTF-8 font file parse state across documents and reduced
  transient allocations during subset generation.
- Regenerated tracked PDF fixtures after the performance changes.

## v0.6.3 - 2026-06-27

Patch release for Arlington baseline timing normalization.

### Fixed

- Ignored volatile Arlington `processingTime` ordering in compliance baseline
  comparisons.

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
  keeping the benchmark suite focused on native PaperRune generation throughput.
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
  PaperRune workloads.

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

Initial cssbruno/paperrune release.

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
- Simplified the root `paperrune.New` facade to the default constructor only.
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
