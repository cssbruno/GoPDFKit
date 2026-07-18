# Paper Engine Execution Checklists

Companion to [PAPER_ENGINE_PLAN.md](PAPER_ENGINE_PLAN.md).

These checklists are implementation gates, not a substitute for the design
plan. An item is complete only when its evidence is linked in the relevant PR,
ADR, benchmark report, fixture, screenshot bundle, or test result.

## Checklist conventions

- [x] Assign an owner before starting a stage.
- [x] Record the base commit and target branch.
- [x] Link the relevant plan section.
- [x] Define measurable acceptance evidence.
- [x] Record intentional deviations in an ADR.
- [ ] Do not mark a stage complete with required exit-gate items open.
- [x] Do not use PDF byte equality as the only correctness evidence. The
  typed and HTML characterization gates combine plan identity, extracted text,
  semantic reading order, structural PDF evidence, and independent display-list
  raster manifests ([typed evidence](document/typed_characterization.go),
  [HTML evidence](document/html_characterization.go),
  [raster gate](document/characterization_raster_test.go)).
- [x] Do not expose unstable internal contracts as public APIs. Public typed
  planning exposes an immutable wrapper while layout-engine storage and plan
  schema details remain private ([wrapper](document/typed_layout_plan.go),
  [API drift gate](document/typed_characterization_test.go)).

Stage record:

```text
Owner: GoPDFKit maintainers
Start revision: 50b2530 (release: harden PDF processing for v0.14.0)
Target branch: codex/paper-engine-foundation
ADR/design links: docs/adr/0001-unified-automatic-layout-engine.md; PAPER_ENGINE_PLAN.md
Benchmark baseline:
Fixture corpus:
Known exclusions:
Exit review:
```

Current evidence: the Stage 0/3 typed and HTML characterization corpora,
deterministic raster manifests, Apple M2 benchmark report, compliance fixture
hashes, and full normal/race regression commands are the measurable acceptance
record. Known exclusions are listed on the open exit gates rather than treated
as completed behavior.

## 1. Program-wide invariants

### One-engine boundary

- [ ] Automatic measurement exists only in the unified planner.
- [ ] Automatic text wrapping exists only in the unified planner.
- [ ] Automatic positioning exists only in the unified planner.
- [ ] Fragmentation and pagination exist only in the unified planner.
- [ ] Frontends only parse, validate, resolve syntax-specific rules, and lower.
- [ ] The PDF painter consumes final positioned commands and performs no layout.
- [x] The authoritative Paper preview renderer replays the exact immutable
  display-list commands used by the unified painter; browser layout only
  arranges Studio chrome. Visible pages and thumbnails use the shared Go direct
  rasterizer compiled to WASM, while SVG remains a diagnostic geometry
  projection ([web payload](internal/layoutengine/web_display_render.go),
  [direct rasterizer](internal/layoutengine/display_raster.go),
  [WASM command](cmd/paper-studio-wasm/main_wasm.go),
  [end-to-end smoke](tools/test-paper-studio-wasm.sh),
  [architecture](ARCHITECTURE.md)).
- [ ] Low-level FPDF-style drawing remains an explicit manual path.
- [x] The architecture invariant is documented in `ARCHITECTURE.md` ([evidence](ARCHITECTURE.md)).

### Determinism

- [x] Grammar version is pinned independently from the AST schema
  ([contract](internal/paperlang/ast.go), [tests](internal/paperlang/ast_test.go)).
- [x] Planner and painter-contract versions are pinned in every canonical plan
  projection and participate in its hash
  ([contract](internal/layoutengine/plan.go), [tests](internal/layoutengine/plan_test.go)).
- [x] Canonical package/import lockfile, digest validation, and deterministic
  resolver output are present ([contract](internal/paperpkg/lockfile.go),
  [resolver](internal/paperpkg/resolver.go), [tests](internal/paperpkg/lockfile_test.go)).
- [ ] Fonts and assets are content-addressed. Unified-plan core-font metrics and
  PNG/JPEG bytes have mandatory SHA-256 identities, and a bound deterministic
  manifest is now rejected unless its canonical resource catalog exactly
  matches the plan; resource-adding transforms deterministically rebuild the
  catalog and `PlanID` ([contract](internal/layoutengine/deterministic_inputs.go),
  [transform/invariant tests](internal/layoutengine/deterministic_inputs_test.go)).
  Typed and strict unified HTML plans now bind that catalog before publication
  ([typed binding](document/typed_layout_plan.go),
  [typed identity test](document/typed_layout_plan_test.go)); the item remains
  open until every legacy/custom-font and non-unified HTML resource path has
  migrated to the same verified catalog.
- [ ] Locale and timezone are explicit inputs. Production `.paper` plans bind
  an authored/scenario locale (or explicit `und`) and explicit `UTC`; identity
  construction rejects noncanonical locale casing, ambient `Local`, malformed
  offsets, and unsafe timezone names
  ([identity](internal/layoutengine/plan_identity.go),
  [tests](internal/layoutengine/plan_identity_test.go),
  [binding](document/paper.go)). Unified typed/HTML plans now bind their
  authored locale (or `und`) and explicit `UTC`
  ([binding](document/typed_layout_plan.go),
  [test](document/typed_layout_plan_test.go)); the item remains open for
  legacy cutover and authored timezone-dependent formatting.
- [ ] Unicode, CLDR, and hyphenation versions are pinned where applicable. The
  built-in `.paper` core-font path pins Go's compiled Unicode table version and
  explicitly records `none` for uninstalled CLDR/hyphenation data; each field
  is causal in `PlanID` ([contract](internal/layoutengine/deterministic_inputs.go),
  [causality tests](internal/layoutengine/plan_identity_test.go)). The item
  remains open until later Unicode shaping, CLDR formatting, and hyphenation
  providers ship pinned data and all frontends bind their versions.
- [ ] Page profile and compatibility flags participate in plan identity. A
  deterministic manifest now validates its exact page dimensions against
  every planned page, requires the running planner version, verifies sorted
  flags, and rejects resource/page/planner substitution
  ([validation](internal/layoutengine/plan.go),
  [adversarial tests](internal/layoutengine/deterministic_inputs_test.go)).
  Unified typed/HTML adapters now bind this manifest as well as the `.paper`
  pipeline ([typed binding](document/typed_layout_plan.go)); the item remains
  open until every legacy and compatibility production route binds it.
- [x] Fixed-point rounding rules are documented and tested ([ADR](docs/adr/0001-unified-automatic-layout-engine.md), [tests](internal/layoutengine/fixed_test.go)).
- [ ] Ambient time, randomness, environment, and host fonts are excluded. The
  `.paper` pipeline has a pinned cross-process identity fixture that remains
  byte-identical across different clocks, timezones, locales, working
  directories, homes, `SOURCE_DATE_EPOCH`, and Fontconfig paths
  ([test](document/paper_deterministic_environment_test.go)); expression and
  control-flow evaluation also has an ambient-authority import audit
  ([audit](internal/paperexpr/capability_test.go)). The item remains open for
  equivalent production evidence across every typed/HTML path.
- [ ] Identical inputs produce identical semantic-template and plan identities.
  Internal identity tests now prove repeated equality and independent
  causality for template, scenario, resources, locale, timezone, text-data
  versions, compatibility profile/flags, page profile, and planner version;
  the production `.paper` fixture proves equality across independent OS
  processes ([identity tests](internal/layoutengine/plan_identity_test.go),
  [process test](document/paper_deterministic_environment_test.go)). Unified
  typed/HTML plans now participate in the manifest identity
  ([typed test](document/typed_layout_plan_test.go)); the item remains open
  until legacy/compatibility routes bind and pass the same identity contract.

### Plan-before-paint

- [x] Font/image/SVG measurement does not mutate the output `Document`; the
  cross-resource planning test snapshots page storage, cursor, and typographic
  state around fresh typed, raster-image, and inline-SVG plans, while the
  retained image/SVG tests also verify detached source bytes and display plans
  ([cross-resource test](document/plan_before_paint_test.go), [image tests](document/html_image_plan_test.go), [SVG tests](document/html_svg_plan_test.go)).
- [x] The prototype immutable core-font catalog resolves every planned glyph
  resource ([contract](internal/layoutengine/glyph.go),
  [tests](internal/layoutengine/glyph_test.go)).
- [x] The complete prototype core-text plan passes paint-ready validation before
  the first sink callback ([painter](internal/layoutengine/painter.go),
  [tests](internal/layoutengine/painter_test.go)).
- [x] Large plans may spool through bounded monolithic or page-segmented
  content-addressed `PlanStore` implementations without losing immutability
  ([stores](internal/layoutengine/plan_store.go),
  [segmented store](internal/layoutengine/plan_store_segmented.go)).
- [x] The `paper render -o` workflow encodes to bounded temporary memory and
  atomically publishes through a private temporary file
  ([command](cmd/paper/main.go), [tests](cmd/paper/main_test.go)).
- [x] `.paper` planning and complete painter preflight occur before appending
  any page to the target PDF ([pipeline](document/paper.go),
  [tests](document/paper_plan_test.go), [painter tests](document/layout_plan_painter_test.go)).
- [x] Mid-document HTML entry performs no additional mutation before its plan
  passes. A fragment planned between manual page content and cursor state is
  repeated for deterministic equality, and failed preflight leaves page bytes,
  page number, and cursor unchanged
  ([atomic fragment tests](document/html_frame_plan_test.go)).

### Provenance and explainability

- [ ] Every persistent source node has a stable or revision-scoped identity.
- [ ] Expanded instances are distinguishable from source definitions.
- [ ] Plan-local fragments are distinguishable from instances.
- [ ] Every fragment maps to source, data, style, semantics, page, and region.
- [x] Production plan projections include deterministic, map-free compact
  provenance tables with aligned interned fragment and line IDs
  ([contract](internal/layoutengine/provenance.go),
  [tests](internal/layoutengine/provenance_break_test.go)).
- [x] Detailed break traces are optional, explicitly requested, bounded by
  record/step/work/byte limits, cancelable, and excluded from the normal
  canonical plan ([contract](internal/layoutengine/break_detail.go),
  [tests](internal/layoutengine/provenance_break_test.go)).
- [x] Every page break has a stable concise reason code. `BreakDecision` uses a
  closed validated reason set, including explicit page breaks, insufficient
  remaining space, pagination constraints, and predecessor overflow; typed
  characterization pins each emitted ledger record
  ([contract](internal/layoutengine/plan.go),
  [planner](document/paper.go),
  [tests](document/typed_characterization_test.go)).
- [ ] Every layout diagnostic has source and page evidence.

## 2. Stage 0 — Characterization and ADR

### Current behavior inventory

- [x] Inventory public typed-layout construction, normalization, measurement,
  legacy-write, exact-plan, and plan-write entry points with deterministic
  status/signature projections and AST drift detection
  ([inventory](document/typed_characterization.go),
  [tests](document/typed_characterization_test.go)).
- [x] Inventory all public HTML entry points with an AST drift gate and a
  deterministic machine-readable projection
  ([inventory](document/html_characterization.go),
  [drift test](document/html_characterization_test.go)).
- [x] Inventory current recognized HTML tags, CSS properties, and explicitly
  characterized value families directly from the implementation registries;
  recognition is not claimed as browser parity
  ([inventory](document/html_characterization.go)).
- [x] Inventory every built-in `layout.Block` kind and each public behavioral
  field/type/status, with reflection and implementation-set drift tests
  ([inventory](document/typed_characterization.go),
  [tests](document/typed_characterization_test.go)).
- [x] Inventory and execute HTML cursor entry/exit, pagination, and failure
  semantics ([tests](document/html_characterization_test.go)).
- [x] Inventory default/first/even header and footer selection plus bounded
  corrected page-number behavior with explicit machine classifications and
  executable first/odd/even fixtures
  ([inventory](document/typed_characterization.go),
  [fixtures](document/typed_characterization_fixtures.go),
  [tests](document/typed_characterization_test.go)).
- [x] Inventory fixed/intrinsic table support, bounded spans, repeated headers,
  causal pagination, and unsupported track parity with explicit machine
  classifications and large/wide/rowspan fixtures
  ([inventory](document/typed_characterization.go),
  [fixtures](document/typed_characterization_fixtures.go)).
- [x] Inventory core text, current Unicode repertoire limits, external links,
  PNG/JPEG images, legacy/unified SVG status, unsupported HTML forms, and typed
  QR behavior with explicit status classifications backed by the typed and
  HTML fixture corpora
  ([typed inventory](document/typed_characterization.go),
  [HTML inventory](document/html_characterization.go),
  [classification tests](document/typed_characterization_test.go)).
- [x] Inventory PDF tagging and compliance behavior through executable PDF/UA
  and PDF/A fixture generation, local structural checks, and canonical
  characterization projections; local checks explicitly do not claim external
  standards validation
  ([fixtures](cmd/compliance-fixtures/main.go),
  [checks](cmd/compliance-check/main.go),
  [evidence schema](internal/characterize/report.go)).
- [x] Require every machine-inventoried behavior to use exactly one of
  documented, accidental, deprecated, or unsupported, with a drift test for
  all Stage 0 behavior domains
  ([contract](document/typed_characterization.go),
  [tests](document/typed_characterization_test.go)); recognition remains
  distinct from browser or complete typed parity.

### Fixture corpus

- [x] Add a minimal executable fixture for every typed block
  ([corpus](document/typed_characterization_fixtures.go),
  [runner tests](document/typed_characterization_test.go)).
- [x] Add bounded fixtures spanning current HTML text/list, mixed flex,
  table/span, SVG, data-image, link, metadata, unsupported-form, recovery,
  policy-rejection, and strict unified cohorts.
- [x] Add nested and mixed-content HTML fixtures.
- [x] Add nested and mixed-content typed fixtures
  ([corpus](document/typed_characterization_fixtures.go),
  [tests](document/typed_characterization_test.go)).
- [x] Add typed exact-fit and one-fixed-unit-over page-boundary fixtures
  ([corpus](document/typed_characterization_fixtures.go)).
- [x] Add bounded typed large, wide, and rowspan table fixtures
  ([corpus](document/typed_characterization_fixtures.go)).
- [x] Add typed first/odd/even page-region fixtures
  ([corpus](document/typed_characterization_fixtures.go)).
- [x] Add malformed HTML input with executable recovery evidence.
- [x] Add malformed typed input and recovery characterization, including a
  pinned accidental-acceptance outcome
  ([corpus](document/typed_characterization_fixtures.go),
  [runner](document/typed_characterization.go)).
- [x] Add deterministic unsigned PDF/UA-2/Arlington and PDF/A-4, PDF/A-4e, and
  PDF/A-4f fixtures with pinned hashes and structural characterization
  ([fixtures](cmd/compliance-fixtures/main.go),
  [tests](cmd/compliance-fixtures/main_test.go)).
- [x] Add HTML cancellation, source/table resource-limit, and atomic error
  fixtures.
- [x] Add typed cancellation and cumulative resource-limit fixtures
  ([runner](document/typed_characterization.go),
  [tests](document/typed_characterization_test.go)).
- [x] Add fixed-worker concurrent compiled-fragment and compiled-template reuse
  fixtures.
- [x] Add concurrent immutable typed-plan reuse across independent writers
  ([runner](document/typed_characterization.go),
  [tests](document/typed_characterization_test.go)).

### Baseline evidence

- [x] Record a page count for every typed and HTML characterization outcome and
  verify it against each independently painted successful PDF; rejected,
  canceled, limited, and unsupported outcomes record zero and publish no
  artifact ([typed runner](document/typed_characterization.go),
  [HTML runner](document/html_characterization.go),
  [tests](document/typed_characterization_test.go),
  [HTML tests](document/html_characterization_test.go)).
- [x] Record per-page/concatenated extracted text for every rendered typed and
  HTML fixture plus semantic reading-role order for every unified plan in the
  deterministic characterization projections
  ([typed runner](document/typed_characterization.go),
  [HTML runner](document/html_characterization.go)).
- [x] Record PDF link, destination, widget, attachment, structure-tree,
  marked-content, AcroForm, and tagged-marker evidence for every successfully
  painted typed and HTML fixture; non-renderable outcomes remain explicit and
  do not fabricate zero-content PDFs
  ([typed runner](document/typed_characterization.go),
  [HTML runner](document/html_characterization.go)).
- [x] Record bounded deterministic direct-display-list raster pages for every
  currently successful typed exact-plan fixture and the strict unified HTML
  plan, including the pinned renderer/profile, full canonical per-page
  manifest, PNG byte length/hash, manifest hash, resource identities, and
  explicit non-authoritative-PDF status. Aggregate typed and HTML baseline
  hashes detect pixel/manifest drift; canceled, rejected, unsupported, legacy-
  only, and over-budget outcomes publish no fabricated raster artifact
  ([evidence](document/characterization_raster.go),
  [typed/HTML runners](document/typed_characterization.go),
  [tests](document/characterization_raster_test.go),
  [CLI](cmd/paper-characterize/main.go)).
- [x] Record current PDF structural assertions, including annotations, tagged
  structure, marked content, parent trees, attachments, XMP profile markers,
  and output intents ([schema](internal/characterize/report.go),
  [compliance baseline](cmd/compliance-fixtures/main_test.go)).
- [x] Record benchmark `ns/op`, `B/op`, and allocations for isolated unified
  planner, retained-plan painter, and typed/compiled-HTML/`.paper` end-to-end
  cohorts ([benchmarks](document/paper_engine_benchmark_test.go),
  [Apple M2 Stage 0 report](docs/performance/baselines/paper-engine-stage0-apple-m2.txt)).
- [x] Record bounded pprof CPU and allocation profiles for representative
  planner-only, retained-plan painter, and typed end-to-end workloads, with
  validated human-readable top summaries and a runtime/reproduction manifest
  ([runner](tools/run-paper-engine-profiles.sh),
  [validator](tools/check-paper-engine-profile-report.sh),
  [workflow](docs/performance/paper-engine-benchmarks.md)).
- [x] Record a ten-sample immutable-plan write baseline at the existing fixed
  16-worker count, including `ns/op`, `B/op`, and allocations
  ([benchmark](document/paper_engine_benchmark_test.go),
  [baseline](docs/performance/baselines/paper-engine-stage0-apple-m2.txt)).
- [x] Require and store a reproduction command plus GOOS, GOARCH, Go version,
  and CPU-count fingerprint in every canonical characterization report
  ([contract](internal/characterize/report.go),
  [compliance report](cmd/compliance-fixtures/main.go)).

### Architecture decisions

- [x] Approve the one-planner ADR ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).
- [x] Approve the private-IR policy ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).
- [x] Approve the resource-catalog boundary ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).
- [x] Approve fixed-point coordinate semantics ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).
- [x] Approve `LayoutPlan` ownership of display commands ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).
- [x] Approve whole-fragment HTML migration ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).
- [x] Approve stabilization before legacy deletion ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).

### Stage 0 exit gate

- [x] Every documented current feature has a bounded executable fixture or an
  explicit accidental/deprecated/unsupported classification. The typed
  inventory test enforces one fixture for every `layout.Block` implementation,
  required boundary/resource/cancellation categories, and classified behavior
  domains; the HTML inventory test enforces one fixture for every behavior
  class, including malformed, diagnostic, policy, and strict-unified outcomes
  ([typed corpus](document/typed_characterization_test.go), [typed fixtures](document/typed_characterization_fixtures.go), [HTML corpus](document/html_characterization_test.go)).
- [ ] Baseline commands reproduce from a clean checkout.
- [x] Benchmark comparisons are statistically usable. The report gate requires
  ten samples, exact named host/toolchain matching, upper-median timing, and
  worst-sample allocation ceilings ([gate](internal/perfgate/report.go),
  [calibration](docs/performance/calibrations/apple-m2-go1.26.json)).
- [x] Architecture decisions have named owners and approval ([ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md)).

## 3. Stage 1 — Identity, diagnostics, and plan contracts

### Identity types

- [x] Define `SourceNodeID` ([contract](internal/layoutengine/identity.go), [tests](internal/layoutengine/identity_test.go)).
- [x] Define domain-separated revision-scoped anonymous structural keys from
  exact source revision, parent, canonical kind, sibling ordinal, and semantic
  subtree fingerprint, explicitly unsuitable as cross-revision durable targets
  ([contract](internal/layoutengine/structural_key.go),
  [tests](internal/layoutengine/structural_key_test.go)).
- [x] Define and enforce initial stable repeated-data instance paths derived from
  the authored repeat prefix and fixture stable key, independent of item order,
  with preserved mapping provenance through direct blocks and components
  ([repeat core](internal/paperrepeat/repeat.go),
  [compiler](internal/papercompile/scenario_repeat.go),
  [tests](internal/papercompile/scenario_repeat_test.go)); nested repeat paths
  and incremental cross-plan reuse remain pending.
- [x] Define plan-local `FragmentID` rules ([contract](internal/layoutengine/identity.go), [tests](internal/layoutengine/identity_test.go)).
- [x] Define `SourceRevisionID` ([contract](internal/layoutengine/identity.go), [tests](internal/layoutengine/identity_test.go)).
- [x] Define `SemanticTemplateID` ([contract](internal/layoutengine/identity.go), [tests](internal/layoutengine/identity_test.go)).
- [x] Define `ScenarioRevisionID` ([contract](internal/layoutengine/identity.go), [tests](internal/layoutengine/identity_test.go)).
- [x] Define `PolicyRevisionID` ([contract](internal/layoutengine/identity.go), [tests](internal/layoutengine/identity_test.go)).
- [x] Define canonical internal `PlanID` inputs covering semantic template,
  scenario/data, resource catalog, locale, timezone, Unicode/CLDR/hyphenation,
  compatibility profile/flags, page profile, and planner version, with
  unambiguous domain-separated derivation and opaque external handles
  ([contract](internal/layoutengine/plan_identity.go),
  [tests](internal/layoutengine/plan_identity_test.go)).
- [x] Define canonical internal `RenderID` inputs covering the exact plan,
  renderer version, color profile, bounded DPI, crop profile, and disclosure
  domain, with domain-separated derivation and opaque external access
  ([contract](internal/layoutengine/plan_identity.go),
  [tests](internal/layoutengine/plan_identity_test.go)).
- [x] Define opaque external handle lifecycle across source/scenario revisions,
  mutable candidates, pinned opens, and retained plans with private scoped
  nonces, domain/kind/capability separation, bounded TTLs, explicit revocation,
  bounded revocation tombstones, deterministic pruning, forgery and
  cross-workspace rejection, and concurrency-safe lookup/revocation
  ([state](internal/paperd/workspace.go),
  [lifecycle](internal/paperd/lifecycle.go),
  [tests](internal/paperd/lifecycle_test.go),
  [plan tests](internal/paperd/plans_test.go)).

### Diagnostics contract

- [x] Define stable diagnostic codes and severities ([contract](internal/layoutengine/diagnostics.go), [tests](internal/layoutengine/diagnostics_test.go)).
- [x] Include pipeline stage and source span ([contract](internal/layoutengine/diagnostics.go), [tests](internal/layoutengine/diagnostics_test.go)).
- [x] Include node, instance, fragment, scenario, page, and rectangle ([contract](internal/layoutengine/diagnostics.go), [tests](internal/layoutengine/diagnostics_test.go)).
- [x] Include structured evidence and related diagnostics ([contract](internal/layoutengine/diagnostics.go), [tests](internal/layoutengine/diagnostics_test.go)).
- [x] Define typed fixes separately from free-form messages ([contract](internal/layoutengine/diagnostics.go), [tests](internal/layoutengine/diagnostics_test.go)).
- [x] Define explicit strict versus fallback-allowed behavior: strict mode
  always emits an error, while compatibility mode can authorize only a named
  whole-fragment fallback and must retain a warning/evidence record; absent or
  ambiguous fallbacks remain errors
  ([contract](internal/layoutengine/compatibility.go),
  [tests](internal/layoutengine/compatibility_test.go)).
- [x] Define cancellation, work-limit, and resource-limit diagnostics ([contract](internal/layoutengine/diagnostics.go)).

### Prototype plan contracts

- [x] Define immutable page and fragment projections ([contract](internal/layoutengine/plan.go), [tests](internal/layoutengine/plan_test.go)).
- [x] Define canonical positioned-line projections with page/fragment ownership,
  baseline geometry, source provenance, and continuation validation
  ([contract](internal/layoutengine/plan.go), [tests](internal/layoutengine/paragraph_test.go)).
- [x] Define deterministic compact provenance tables, aligned fragment/line
  references, structural-query IDs, and canonical store/hash round trips
  ([contract](internal/layoutengine/provenance.go),
  [projection](internal/layoutengine/plan.go),
  [tests](internal/layoutengine/provenance_break_test.go)).
- [x] Define always-retained concise break records plus explicitly requested,
  bounded detailed causal traces with deterministic codec and structural-query
  support ([concise contract](internal/layoutengine/plan.go),
  [detailed contract](internal/layoutengine/break_detail.go),
  [tests](internal/layoutengine/provenance_break_test.go)).
- [x] Define display-command schema ([contract](internal/layoutengine/plan.go), [tests](internal/layoutengine/plan_test.go)).
- [x] Define an immutable semantic tree, mandatory fragment ownership, explicit
  artifact association, and canonical page-local reading order with provenance,
  count/depth/byte validation, serialization, segmented-store reconstruction,
  and structural-query support
  ([contract](internal/layoutengine/semantic.go),
  [tests](internal/layoutengine/semantic_test.go)), including canonical
  language, role-validated alternate/actual text, heading levels, internal link
  destinations, attribute-aware queries, and store/hash preservation; PDF/UA
  tag-tree and marked-content emission remain pending.
- [x] Define plan serialization and versioning ([contract](internal/layoutengine/plan.go), [tests](internal/layoutengine/plan_test.go)).
- [x] Define versioned segmented `PlanStore` manifest/page-range contracts with
  exact global indexes, content-addressed segment hashes, manifest-only page
  metadata lookup, canonical reconstruction, and explicit schema rejection
  ([store](internal/layoutengine/plan_store_segmented.go),
  [tests](internal/layoutengine/plan_store_segmented_test.go)).
- [x] Implement initial bounded immutable in-memory and filesystem-backed
  content-addressed plan stores with canonical reconstruction, corruption
  detection, concurrency safety, and atomic file publication
  ([store](internal/layoutengine/plan_store.go),
  [tests](internal/layoutengine/plan_store_test.go)), plus bounded memory/file
  segmented stores with manifest-last atomic publication, corruption/missing
  segment detection, cancellation, concurrency, and orphan-safe accounting
  ([segmented tests](internal/layoutengine/plan_store_segmented_test.go)); lazy
  partial `LayoutPlan` materialization and orphan garbage collection remain
  pending.
- [x] Define the initial deterministic capture manifest, page/contact-sheet
  translations, and crop-local coordinate transforms
  ([contract](internal/layoutengine/visual_artifacts.go),
  [tests](internal/layoutengine/visual_artifacts_test.go)); raster and
  cross-page-strip transforms remain pending.
- [x] Define bounded hit-test result ordering and half-open page-coordinate
  semantics ([contract](internal/layoutengine/hittest.go),
  [tests](internal/layoutengine/hittest_test.go)).

### Stage 1 exit gate

- [x] Identity contracts reject collisions and ambiguous reuse ([tests](internal/layoutengine/identity_test.go), [plan tests](internal/layoutengine/plan_test.go)).
- [x] Synthetic plan serialization round-trips deterministically ([tests](internal/layoutengine/plan_test.go)).
- [x] Diagnostic schemas validate and round-trip deterministically through a
  strict, versioned, byte/count-bounded canonical codec with detached results
  ([codec](internal/layoutengine/diagnostic_codec.go),
  [tests](internal/layoutengine/diagnostic_codec_test.go)); capture manifests
  are independently versioned and bounded by the visual-artifact contracts.
- [x] Prototype plan hashes are stable across repeated runs ([tests](internal/layoutengine/plan_test.go)).
- [x] No immature contract has been exposed publicly ([internal package](internal/layoutengine)).

## 4. Stage 2 — Planner kernel, painter, and internal Plan Viewer

### Geometry and resources

- [x] Implement checked fixed-point point, size, rectangle, inset, and affine
  transform types, including exact translation/scaling/quarter-turn composition,
  corner bounds, collapse rejection, overflow checks, and half-away rounding
  ([geometry](internal/layoutengine/fixed.go),
  [transforms](internal/layoutengine/transform.go),
  [tests](internal/layoutengine/fixed_test.go),
  [transform tests](internal/layoutengine/transform_test.go)).
- [x] Implement and document canonical point/millimeter/centimeter/inch to
  fixed-point conversions using the exact 1 inch = 25.4 mm and 72 point scale,
  shared half-away rounding, invalid/non-finite rejection, and real typed text
  and geometry adapter integration
  ([conversion](internal/layoutengine/document_units.go),
  [adapter](document/layout_fixed_units.go),
  [tests](internal/layoutengine/document_units_test.go),
  [all-unit adapter tests](document/typed_shadow_plan_test.go)).
- [x] Prototype typed-shadow conversion of page, body, and measured block
  geometry from points, millimeters, centimeters, and inches
  ([adapter](document/typed_shadow_plan.go), [tests](document/typed_shadow_plan_test.go)).
- [x] Test overflow and rounding boundaries ([tests](internal/layoutengine/fixed_test.go)).
- [x] Implement canonical immutable PDF core-font metric handles with SHA-256
  identities ([contract](internal/layoutengine/glyph.go),
  [adapter](document/typed_glyph_shadow.go),
  [tests](document/typed_glyph_shadow_test.go)).
- [x] Implement initial content-addressed immutable PNG/JPEG intrinsic dimension
  handles and exact planned image placements
  ([contract](internal/layoutengine/image.go),
  [tests](internal/layoutengine/image_test.go)).
- [x] Bind initial encoded-image identities to lowercase SHA-256 content digests
  ([contract](internal/layoutengine/image.go)).
- [x] Ensure planned image resource catalog construction, lookup, digesting,
  decoding, and preflight are cancellation-aware and bounded by explicit
  resource-count and cumulative encoded/decoded-byte ceilings
  ([catalog](document/typed_image_plan.go),
  [lookup and preflight](document/layout_display_painter.go),
  [tests](document/planned_resource_lookup_test.go)).

### Canonical private tree

- [x] Implement bounded immutable dense node and shared child-edge arenas with
  deterministic projection, hashing, codec round trips, and concurrent-reader
  safety ([contract](internal/layoutengine/tree.go),
  [tests](internal/layoutengine/tree_test.go)).
- [x] Implement stable node keys and node IDs separately from zero-based dense
  arena indexes, with collision and overflow rejection
  ([contract](internal/layoutengine/tree.go),
  [tests](internal/layoutengine/tree_test.go)).
- [x] Intern strings, styles, tracks, resources, and semantics into typed
  one-based tables, including conflicting resource-key rejection
  ([contract](internal/layoutengine/tree.go),
  [tests](internal/layoutengine/tree_test.go)).
- [x] Avoid per-node property maps in the planner hot path; canonical nodes are
  fixed typed records referring to shared dense tables
  ([contract](internal/layoutengine/tree.go)).
- [x] Preserve typed auto, fixed, percentage, and fraction lengths for
  constraint-dependent style/track values
  ([contract](internal/layoutengine/tree.go),
  [tests](internal/layoutengine/tree_test.go)).
- [x] Lower the supported high-level `LayoutDocument` surface into the bounded
  canonical primitive tree before typed planning: page regions/counters,
  paragraph/heading/list/item, section/clause/note, metadata/signature grids,
  image/QR resources, table/row/cell/column tracks, row/column tracks, page
  breaks, attachments, stable revision-scoped identities, styles, semantics,
  and immutable provenance paths
  ([lowering](internal/papercompile/typed_tree.go),
  [plan integration](document/typed_layout_plan.go),
  [tests](internal/papercompile/typed_tree_test.go),
  [integration tests](document/typed_layout_plan_test.go)). Typed models do not
  carry authored source spans, so their canonical nodes intentionally retain
  empty spans plus stable model paths; custom external block implementations
  and fields outside the currently supported typed planning contract remain
  explicit exclusions.

### Planner kernel

- [x] Implement initial bounded fixed-point page-master planning with validated
  default/first/odd/even selection, independent header/body/footer streams,
  explicit region geometry, causal breaks, provenance, and deterministic output
  ([planner](internal/layoutengine/page_master.go),
  [tests](internal/layoutengine/page_master_test.go)); named masters, counters,
  subtree measurement, and production adapters remain pending.
- [x] Implement a fixed-point indivisible vertical `flow` prototype ([planner](internal/layoutengine/flow.go), [tests](internal/layoutengine/flow_test.go)).
- [x] Implement an initial fixed-point indivisible `box` flow primitive with
  explicit non-negative margin/border/padding edges, exact border/content
  rectangles, outer-height pagination, and resolved overflow diagnostics
  ([planner](internal/layoutengine/box.go),
  [tests](internal/layoutengine/box_test.go)); collapsing/negative margins,
  intrinsic sizing, decoration commands, and fragmentation remain pending.
- [x] Implement initial paragraph line plans using the characterized core-font
  text behavior, including ordered multi-block paragraph/heading composition
  and pagination ([kernel](internal/layoutengine/paragraph.go),
  [production adapter](document/paper.go), [tests](document/paper_test.go));
  Unicode shaping and the complete block model remain pending.
- [x] Expose read-only `.paper` compilation as an immutable, reusable
  `document.PaperPlan`, preserve authored IDs/source spans through positioned
  fragments/lines/glyphs, and paint the exact retained plan without replanning
  ([API](document/paper.go), [tools](document/paper_plan_tools.go),
  [tests](document/paper_plan_test.go)); the public wrapper intentionally keeps
  the private layout schema hidden.
- [x] Implement an immutable prebroken-line paragraph kernel with fixed geometry;
  full Unicode shaping and multi-paragraph flow composition remain pending
  ([planner](internal/layoutengine/paragraph.go), [tests](internal/layoutengine/paragraph_test.go)).
- [x] Implement a shared streaming legacy wrapper with exact visible/consumed
  byte ranges, strict width behavior, and count/collection parity; migrate the
  existing split/count APIs without changing their profiles
  ([scanner](document/text_wrap.go), [split adapter](document/text_split.go),
  [tests](document/text_wrap_test.go)).
- [x] Lower one isolated plain core-font paragraph through the exact `MultiCell`
  wrapping profile into fixed line widths, horizontal alignment offsets,
  compatibility baselines, continuation fragments, and pages
  ([adapter](document/typed_line_shadow.go), [tests](document/typed_line_shadow_test.go)).
- [x] Implement initial bounded fixed-point image sizing and fitting with exact
  auto/fixed aspect arithmetic, contain/cover/fill/none/scale-down policies,
  alignment, pre-clip object geometry, visible destinations, intrinsic crops,
  provenance, and structured limits/diagnostics
  ([kernel](internal/layoutengine/image_fit.go),
  [tests](internal/layoutengine/image_fit_test.go)), plus exact crop-aware plan
  payloads, deep replay ownership, PDF clipping/source transforms, and bounded
  SVG replay ([plan](internal/layoutengine/image.go),
  [PDF adapter](document/layout_display_painter.go)); `.paper` image syntax,
  non-rectangular crops, and broader image formats remain pending.
- [x] Implement explicit page-break syntax, formatting, semantic lowering, and
  unified planned pagination with stable `explicit_page_break` evidence;
  leading/trailing/repeated markers do not synthesize blank pages
  ([language](internal/paperlang/pagebreak_test.go),
  [compiler](internal/papercompile/pagebreak_test.go),
  [planner integration](document/paper.go),
  [end-to-end tests](document/paper_pagebreak_test.go)).
- [x] Implement initial ordered/unordered list and item syntax, deterministic
  decimal/dash/asterisk markers, inherited core-font text styles, semantic
  lowering to `layout.ListBlock`, and source-ordered pagination through the
  exact shared glyph-plan/PDF path ([language](internal/paperlang/parser.go),
  [compiler](internal/papercompile/compile.go),
  [planner integration](document/paper.go), [tests](document/paper_test.go)).
- [x] Implement unified vertical-flow resumable break tokens with opaque copied
  continuation state, exact input/limit ownership fingerprints, foreign/zero/
  completed-token rejection, cumulative valid plan snapshots, deterministic
  chunk bounds, cancellation-safe retry, cumulative planning work/page/state
  limits, concurrent-reader safety, and final plan/hash equivalence to
  uninterrupted planning across mid-page, page-break, oversized, empty, and
  nested/repeated-instance flows
  ([planner](internal/layoutengine/flow_resume.go),
  [integration](internal/layoutengine/flow.go),
  [tests](internal/layoutengine/flow_resume_test.go)).
- [x] Implement paragraph-bound opaque continuation tokens with deterministic
  defer/place results and foreign-token rejection
  ([planner](internal/layoutengine/paragraph.go), [tests](internal/layoutengine/paragraph_test.go)).
- [x] Enforce the consume/advance/oversized invariant ([planner](internal/layoutengine/flow.go), [tests](internal/layoutengine/flow_test.go)).
- [x] Implement concise break reasons ([contract](internal/layoutengine/plan.go), [tests](internal/layoutengine/flow_test.go)).
- [x] Implement truthful paragraph policy-break evidence, preferred and strict
  widow/orphan behavior, next-region lookahead, and bounded constraint relaxation
  ([planner](internal/layoutengine/paragraph.go), [tests](internal/layoutengine/paragraph_test.go)).
- [x] Add a private, observational typed atomic-paragraph pagination shadow
  with isolated scratch measurement and legacy page-count comparison
  ([adapter](document/typed_shadow_plan.go), [tests](document/typed_shadow_plan_test.go)).
- [x] Restrict the core-font shadow to the characterized printable-ASCII/line-feed
  subset so `SplitTextCount` byte/rune differences cannot produce false parity
  ([adapter](document/typed_shadow_plan.go), [tests](document/typed_shadow_plan_test.go)).
- [x] Implement work budgets and cancellation checks. Vertical flow exposes
  a compatible context-aware entry point with explicit block/page/work limits
  and structured cancellation/resource diagnostics
  ([planner](internal/layoutengine/flow.go),
  [tests](internal/layoutengine/flow_test.go)); row/column, table, stack, canvas,
  shaping, line breaking, image fitting, and segmented stores are also bounded,
  and box flow now propagates the same context/block/page/work policy through
  its linear geometry resolution and pagination
  ([box planner](internal/layoutengine/box.go),
  [box tests](internal/layoutengine/box_test.go)); explicit grid resolution is
  cancellation-aware and deterministically work-bounded across occupancy,
  intrinsic spans, track distribution, offsets, and fragment emission
  ([grid planner](internal/layoutengine/grid.go),
  [grid tests](internal/layoutengine/grid_test.go)); one concurrency-safe,
  context-carried cumulative request meter now propagates through typed and
  `.paper` document planning, recursive subtree expansion, paragraph
  construction/fragmentation, page-master correction and shell subplans, and
  the existing child-planner budget charges without replenishing on nesting or
  retry ([request budget](internal/layoutengine/request_budget.go),
  [document propagation](document/planning_budget.go),
  [kernel tests](internal/layoutengine/request_budget_test.go),
  [adversarial document tests](document/planning_budget_test.go)).

### Painter

- [x] Implement balanced save/restore, exact fixed affine transforms, and
  nonzero/even-odd path clips with strict state/resource validation and bounded
  replay ([contract](internal/layoutengine/display_graphics.go),
  [painter](internal/layoutengine/display_painter.go),
  [tests](internal/layoutengine/display_graphics_test.go)).
- [x] Implement immutable move/line/cubic/close paths, RGB nonzero/even-odd
  fills, and exact-width RGB strokes with canonical ordering, bounded path
  segments, canonical store round trips, and PDF end-to-end operator evidence;
  straight-path strokes now also carry bounded butt/round/square caps,
  miter/round/bevel joins, normalized dash arrays and signed dash phase through
  canonical storage, SVG capture, deterministic raster, and PDF replay
  ([contract](internal/layoutengine/display_graphics.go),
  [PDF painter](document/layout_display_graphics.go),
  [SVG production tests](document/svg_display_plan_test.go)); styled cubic
  stroke raster parity remains pending.
- [x] Implement the initial immutable positioned core-font glyph runs with exact
  cumulative fixed advances and command references
  ([contract](internal/layoutengine/glyph.go),
  [adapter](document/typed_line_shadow.go),
  [tests](internal/layoutengine/glyph_test.go),
  [adapter tests](document/typed_glyph_shadow_test.go)).
- [x] Implement initial exact image commands and mixed text/image display-list
  replay with page/command/glyph/image budgets
  ([schema](internal/layoutengine/image.go),
  [compositor](internal/layoutengine/display_list.go),
  [painter](internal/layoutengine/display_painter.go),
  [tests](internal/layoutengine/display_painter_test.go)).
- [x] Implement exact immutable internal/external link and destination plan
  tables, bounded display replay, recording, and atomic production PDF
  annotation/destination painting without layout, plus linear exact annotation
  rectangle derivation from finalized core-glyph advances with overlap/range/
  URI validation and command-order preservation
  ([contract](internal/layoutengine/link.go),
  [glyph-link compositor](internal/layoutengine/glyph_links.go),
  [PDF adapter](document/layout_display_painter.go),
  [tests](document/layout_link_painter_test.go),
  [glyph-link tests](internal/layoutengine/glyph_links_test.go)); typed segments
  and the initial HTML adapter now lower canonical external links through this
  exact path ([typed adapter](document/typed_link_plan.go),
  [typed tests](document/typed_link_plan_test.go),
  [HTML tests](document/html_unified_plan_test.go)); `.paper` link syntax,
  internal/named public anchors, bookmarks, and tagging remain pending.
- [x] Reject missing or incompatible core-font resources and incomplete glyph
  coverage before painting ([preflight](internal/layoutengine/painter.go),
  [tests](internal/layoutengine/painter_test.go)).
- [x] Prove the initial recording painter only replays planned pages and glyph
  commands, without wrapping or automatic page decisions
  ([painter](internal/layoutengine/painter.go),
  [tests](internal/layoutengine/painter_test.go)).
- [x] Bound synchronous core-plan replay by explicit page, command, and glyph
  budgets checked before the first sink callback
  ([limits](internal/layoutengine/painter.go),
  [tests](internal/layoutengine/painter_test.go)).
- [x] Implement the initial production PDF sink for paint-ready core-font plans;
  it preflights before mutation, adds only planned pages, and emits absolute
  planned glyph positions without cells or wrapping
  ([adapter](document/layout_plan_painter.go),
  [tests](document/layout_plan_painter_test.go)).
- [x] Implement the initial mixed text/image PDF sink with detached encoded-byte
  resolution, SHA-256 and intrinsic-dimension verification, bounded decode,
  preflight-before-mutation, and exact command-order replay
  ([adapter](document/layout_display_painter.go),
  [tests](document/layout_display_painter_test.go)).

### Internal Plan Viewer

- [x] Emit deterministic geometry-only SVG page captures with planned bounds,
  committed break markers, and bounded diagnostics ([capture](internal/layoutengine/capture_svg.go),
  [tests](internal/layoutengine/capture_svg_test.go)); this is not a rendered preview or crop.
- [x] Render the initial core-font display commands directly to a deterministic,
  bounded SVG preview with explicit user-text disclosure metadata
  ([preview](internal/layoutengine/preview_svg.go),
  [tests](internal/layoutengine/preview_svg_test.go)).
- [x] Replay balanced graphics state, exact affine transforms, path clips,
  nonzero/even-odd fills, and fixed-width strokes in the same deterministic
  display-plan SVG as text and images, with exact fixed-scalar serialization,
  resource/segment/output bounds, and no geometry reconstruction
  ([capture](internal/layoutengine/display_svg.go),
  [tests](internal/layoutengine/display_svg_graphics_test.go)).
- [x] Show explicit page boxes plus fragment border/content bounds in deterministic
  fixed-coordinate geometry captures
  ([viewer](internal/layoutengine/capture_svg.go),
  [tests](internal/layoutengine/capture_svg_test.go)).
- [x] Hit-test zero-based raster pixel centers through declared capture bounds
  and raster dimensions to exact fixed page coordinates, then return bounded
  deterministic command/fragment/source projections without DPI, CSS, or
  browser-scale assumptions
  ([pixel mapping](internal/layoutengine/hittest_pixel.go),
  [kernel tests](internal/layoutengine/hittest_pixel_test.go),
  [document tool](document/paper_plan_tools.go),
  [service tool](internal/paperd/plans.go)).
- [x] Hit-test fixed page coordinates to bounded command, fragment, and source
  projections without suppressing overflow hits ([contract](internal/layoutengine/hittest.go),
  [tests](internal/layoutengine/hittest_test.go)).
- [x] Select nested/overlapping fragments deterministically ([tests](internal/layoutengine/hittest_test.go)).
- [x] Capture deterministic multi-page contact sheets and exact node/fragment
  border-box crops directly from the retained plan
  ([artifacts](internal/layoutengine/visual_artifacts.go),
  [tests](internal/layoutengine/visual_artifacts_test.go)).
- [x] Show bounded enum-only incoming/outgoing committed break-reason labels in
  page captures and preserve them in exact contact-sheet embeddings
  ([viewer](internal/layoutengine/capture_svg.go),
  [tests](internal/layoutengine/capture_svg_test.go),
  [contact-sheet tests](internal/layoutengine/visual_artifacts_test.go)).
- [x] Agent visual manifests display the exact plan hash, plan/planner/painter
  contract versions, mode-specific renderer version, independently hashed
  immutable font/image resource set, and an all-or-nothing source/scenario/
  policy revision tuple; production `.paper` and `paperd` capture paths always
  supply the revision tuple
  ([identity](internal/layoutengine/viewer_identity.go),
  [manifest](internal/layoutengine/visual_artifacts.go),
  [document binding](document/paper.go),
  [tests](internal/layoutengine/visual_artifacts_test.go),
  [service tests](internal/paperd/plans_test.go)).

### Stage 2 exit gate

- [x] Simple typed fixtures preserve page count and extracted-text order across
  the legacy typed renderer and canonical planner/painter, including explicit
  page placement after a forced break
  ([output-level compatibility tests](document/typed_compatibility_test.go)).
- [x] Supported initial page breaks are explainable through stable break reasons
  and bounded causal evidence ([tests](internal/layoutengine/explain_test.go),
  [end-to-end tests](document/paper_pagebreak_test.go)).
- [x] Initial node and fragment crops are deterministic
  ([tests](internal/layoutengine/visual_artifacts_test.go)).
- [x] `.paper` planning and painter preflight fail before target PDF mutation
  ([tests](document/paper_plan_test.go),
  [painter tests](document/layout_plan_painter_test.go)).
- [x] Painter-only tests show the initial painter replays positioned commands
  without hidden wrapping or pagination calls
  ([tests](internal/layoutengine/painter_test.go)).
- [x] Planner, retained-plan painter, typed/compiled-HTML/`.paper` end-to-end,
  concurrent reuse, and table-kernel reports meet the named Apple M2 + Go 1.26
  calibration using at least ten samples, host/toolchain fingerprint matching,
  upper-median timing ceilings, and worst-sample allocation ceilings
  ([profile](docs/performance/calibrations/apple-m2-go1.26.json),
  [validator](internal/perfgate/report.go),
  [checked report](docs/performance/baselines/paper-engine-stage0-apple-m2.txt)).

## 5. Stage 3 — Complete typed layout and cut over

### Primitive completion

- [x] Implement the initial `.paper` ordered/unordered list slice with readable
  list/item nodes, decimal/dash/asterisk markers, inherited core text styles,
  multiple item paragraphs, and markers lowered into the same wrapped glyph
  plan ([language](internal/paperlang), [compiler](internal/papercompile),
  [planner integration](document/paper.go), [tests](document/paper_test.go)).
  Typed list boxes now share the exact multi-fragment margin/padding/background/
  border contract and retain list/list-item reading ownership
  ([box tests](document/typed_container_box_test.go)). Nested authored lists
  following an item paragraph now recursively retain distinct list/list-item
  ancestry, inherited styles, markers, exact decoration geometry, and reading
  order; hanging indents and Unicode/custom markers remain pending.
- [x] Implement initial bounded fixed-point row and column primitives sharing
  one fixed/auto/fraction track solver, deterministic remainder distribution,
  gaps, measured minimums, cross-axis start/center/end/stretch alignment,
  provenance, overflow diagnostics, cancellation, and byte-based state limits
  ([planner](internal/layoutengine/row_column.go),
  [tests](internal/layoutengine/row_column_test.go)), with readable `row`/
  `column` syntax, formatter/compiler lowering, authored provenance, and exact
  core-font plan/PDF replay ([compiler tests](internal/papercompile/row_column_test.go),
  [end-to-end tests](document/paper_row_column_test.go)); mixed top-level flow,
  nested containers, non-text children, baselines, fragmentation, and general
  renderer integration remain pending.
- [x] Implement an initial bounded fixed-point grid kernel with fixed/auto/
  fractional tracks, deterministic remainder distribution, intrinsic span
  contributions, canonical row-major placement, overlap detection, and exact
  fragment geometry ([planner](internal/layoutengine/grid.go),
  [tests](internal/layoutengine/grid_test.go)); named lines, implicit tracks,
  alignment modes, fragmentation, and painter decorations remain pending.
- [x] Implement initial bounded table occupancy and invalid-span handling with
  rectangular coverage, overlap/hole/bounds/header-crossing rejection, and
  cancellation/work limits ([planner](internal/layoutengine/table.go),
  [tests](internal/layoutengine/table_test.go)).
- [x] Implement initial fixed/intrinsic table contributions and exact final
  tracks with deterministic remainder distribution
  ([planner](internal/layoutengine/table.go),
  [tests](internal/layoutengine/table_test.go)); shaped cell content, percent/
  fractional tracks, and painter decoration remain pending.
- [x] Implement deterministic row heights, rowspan-deficit distribution, and
  explicit span-connected indivisible row groups
  ([planner](internal/layoutengine/table.go),
  [tests](internal/layoutengine/table_test.go)).
- [x] Implement initial causal row-group pagination and repeated header rows
  ([planner](internal/layoutengine/table.go),
  [tests](internal/layoutengine/table_test.go)); footer repetition, row
  splitting, and page-windowed streaming remain pending.
- [x] Implement single-emission oversized row/header/rowspan diagnostics with
  guaranteed planner advance ([tests](internal/layoutengine/table_test.go)).
- [x] Implement initial bounded fixed-point stack/layer behavior with explicit
  container geometry, start/center/end/stretch placement, offsets,
  deterministic z/paint order, intentional overlap, provenance, overflow
  evidence, cancellation, and byte-based state limits
  ([planner](internal/layoutengine/stack.go),
  [tests](internal/layoutengine/stack_test.go)); transforms, clips, positioned
  descendants, and syntax/painter integration remain pending.
- [x] Implement an initial bounded fixed-point local canvas anchor DAG with
  measured nodes, container/node anchors, baseline constraints, deterministic
  topological resolution, explicit underdetermined defaults, provenance,
  canonical paint order, and overflow evidence
  ([planner](internal/layoutengine/canvas.go),
  [tests](internal/layoutengine/canvas_test.go)); inequalities, priorities,
  percentages, transforms, and a general solver remain pending.
- [x] Implement structured cycle, cross-axis/unsatisfiable, and contradictory
  overdetermination diagnostics for the initial canvas constraint contract
  ([tests](internal/layoutengine/canvas_test.go)).

### Pagination completion

- [x] Implement deterministic keep-together preferences for the exact typed
  paragraph, heading, list, section, clause, note, multi-column metadata, image,
  and multi-column signature cohort, with causal constraint breaks and bounded
  oversized relaxation ([planner](document/paper.go),
  [tests](document/typed_pagination_policy_test.go)); mixed-flow tables and
  future strict-keep syntax remain separate contracts.
- [x] Implement keep-with-next chains for that exact typed cohort, including
  section-title and nested container lowering, without measurement during
  paint ([planner](document/paper.go),
  [tests](document/typed_pagination_policy_test.go)).
- [x] Implement preferred widows and orphans for typed text, with explicit
  positive planner inputs, deterministic relaxation diagnostics, causal break
  evidence, cancellation, line/work bounds, and page limits
  ([planner](document/paper.go),
  [kernel](internal/layoutengine/paragraph.go),
  [tests](document/typed_pagination_policy_test.go)).
- [x] Implement initial default/first/odd/even page-master selection with fixed,
  validated regions and deterministic precedence
  ([planner](internal/layoutengine/page_master.go),
  [tests](internal/layoutengine/page_master_test.go)); explicit named-master
  selection remains pending.
- [x] Reject circular typed page-shell dependencies, including total-page aliases
  embedded in master subtrees, while retaining pure first/parity/default
  selection ([adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)); named-master expressions do
  not yet exist in the typed compatibility model.
- [x] Plan supported typed headers and footers using their actual exact subtree
  height per first/default/even selection, and reject manual-height conflicts,
  multi-page shells, and shells that eliminate the body region
  ([adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)). Content-addressed shell
  images and canonical external shell links are also deduplicated, translated,
  captured, and replayed from the composed display list. Sole-body exact typed
  tables now select the measured first/default/even body region on every page,
  retain repeated headers and causal breaks under variable regions, and compose
  shell text, images, links, and corrected counters without painter re-layout
  ([adapter](document/typed_table_plan.go),
  [tests](document/typed_table_plan_test.go)).
- [x] Implement bounded typed total-page counter correction with an eight-pass
  convergence limit, cancellation, page limits, and deterministic invalid-format
  failures ([adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)); general cross-document
  reference resolution remains pending.
- [x] Finalize typed page counters as positioned core-font glyph commands in the
  immutable plan before capture or PDF painting
  ([adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)).

### Typed compatibility lowering

- [x] Map every direct `LayoutDocument` field, with a reflection-pinned field
  inventory and causal exact-plan test proving that title, language, metadata,
  page template, body, signature, QR, and attachments each affect immutable
  plan identity ([field mapping test](document/typed_field_mapping_test.go)).
- [ ] Map paragraphs, headings, lists, tables, images, and notes. Initial exact
  adapter coverage includes paragraphs, headings, basic lists/notes, and
  content-addressed inline PNG/JPEG images with contain/cover placement,
  captions, alt/decorative semantics, immutable source snapshots, bounded
  capture, and mixed display-plan PDF replay
  ([image lowering](document/typed_image_plan.go),
  [mixed planner](document/paper.go),
  [tests](document/typed_layout_plan_test.go)), plus sole-body fixed-column
  tables with structured paragraph/heading cells, colspan/rowspan geometry,
  horizontal/vertical alignment, causal pagination, explicit repeated-header
  fragments, table/row/header-cell semantics, cancellation, and exact
  plan/capture/PDF replay ([adapter](document/typed_table_plan.go),
  [tests](document/typed_table_plan_test.go)). Initial top-level mixed
  paragraph/table/paragraph flow now shares predecessor cursors, variable page
  regions, repeated headers, causal break provenance, display resources,
  semantics, cancellation, and global page/work bounds without re-layout
  ([mixed compositor](document/typed_mixed_plan.go),
  [tests](document/typed_table_plan_test.go)). Tables nested through the exact
  section/clause/note lowering and row/column siblings now retain source order,
  title/first-content keeps, explicit clause breaks, variable shells/counters,
  fallback leaf semantics, cancellation, capture, and PDF replay. A bounded
  one-page table can now occupy an individual row/column track: its exact table
  fragments, paths, fonts, links/destinations, semantic hierarchy, and reading
  order are translated into the shared container display plan without
  re-layout, with resource reuse, capture/raster/PDF, cancellation, immutable
  StyleRef, and atomic overflow/pagination evidence. A bounded paginated table
  can now occupy any authored track of a row/column container: the compositor
  promotes the unified table planner's page-master body regions, repeated
  headers, causal break ledger, continuation fragments, fonts, paths,
  annotations, semantic associations, and per-page reading order while
  retaining non-paginating siblings on page one in deterministic authored
  semantic reading order. It rejects multiple independently paginating
  children and page-limit failures atomically
  ([container compositor](document/paper_row_column.go),
  [tests](document/typed_row_column_table_test.go)). Typed
  tables now resolve mixed authored fixed and intrinsic columns with exact
  minimum/maximum bounds, canonical fixed-point remainder distribution,
  colspan minimum/preferred contributions, and the same resolved tracks across
  height measurement, pagination, capture, raster, and PDF replay; cancellation,
  work-limit, impossible-bound, determinism, and race evidence cover the cohort
  ([unified solver](internal/layoutengine/table.go),
  [adapter and tests](document/typed_table_plan.go),
  [tests](document/typed_table_plan_test.go)). Structured caption segments now
  preserve external and internal links, destinations, semantics, and immutable
  source snapshots, and cell StyleRef values are snapshotted before intrinsic
  and wrapped measurement. Structured table cells now lower ordered/unordered
  list markers, section/clause/note titles and bodies, and content-addressed
  inline images through the same intrinsic-measurement and final-paint sequence;
  image bytes deduplicate in the bounded document resource catalog, captions
  remain in authored order, and figure/artifact/heading child semantics retain
  alt/decorative intent. Plan, display capture, deterministic PDF, concurrent
  hash, and source-catalog tests cover this cohort
  ([rich cell tests](document/typed_table_rich_content_test.go)). Structured
  list lowering now retains distinct nested list/list-item ancestors and gives
  each paragraph/image leaf its own exact visual fragment and semantic owner;
  cell decoration fragments are artifacts, repeated-header content keeps one
  logical semantic identity, and only non-artifact leaves enter canonical page
  reading order. The no-layout PDF replay maps those retained paths to
  `Table`/`TR`/`TH`/`TD`/`L`/`LI`/`P`/`Figure` tagged structure. Structured
  captions now split finalized glyph lines into geometry-compatible mixed core-
  font/color runs while retaining exact internal destinations and internal/
  external link rectangles. Table, row, and atomic cell keeps now reach the
  fixed-point table paginator: row keep-with-next chains, table/row orphan and
  widow groups, table keep-together, causal page breaks, and bounded
  `KEEP_TOO_LARGE` relaxation are retained in plan evidence; the mixed-flow
  compositor also moves an exact table/text keep-with-next pair to a fresh page
  when both fit there. Plan, semantic, tagged-PDF, cancellation, determinism,
  and race tests cover the cohort
  ([semantic/policy implementation](document/typed_table_plan.go),
  [tagged painter](document/layout_display_painter.go),
  [tests](document/typed_table_semantic_policy_test.go)). Multiple
  independently paginating row/column children, inline styles that change
  finalized metrics, independently splittable cell contents, and complete
  table-style compatibility remain pending. Typed image boxes
  now snapshot value/reference styles and lower bounded padding, background
  fills, and independent solid borders into exact outer/
  content geometry and immutable fill-image-stroke paint order, with plan,
  semantic, deterministic PDF, invalid-input, and race evidence
  ([image adapter](document/typed_image_plan.go),
  [tests](document/typed_layout_plan_test.go)). Sole-body row/column containers
  now interleave text with content-addressed inline images, reuse immutable
  resources, preserve alt/decorative reading order, and lower image padding,
  fills, and per-side borders through the same exact display list with bounded
  capture, cancellation, PDF, and race evidence
  ([container compositor](document/paper_row_column.go),
  [tests](document/typed_row_column_image_test.go)). Structured container-image
  captions, ambient image sources, and complete parity remain pending.
- [ ] Map sections, clauses, metadata grids, signatures, and QR. Initial exact
  adapter coverage includes unstyled section/clause/note containers,
  metadata grids with stable equal-width tracks and incomplete final rows, plus
  signature rows/envelopes with deterministic gaps and mixed explicit/flexible
  widths, maximum-cell row heights, ordered cell semantics, and retained PAdES
  placeholder identity
  ([implementation](document/typed_layout_plan.go),
  [lowering](document/paper.go), [grid geometry](document/typed_grid_flow.go),
  [tests](document/typed_layout_plan_test.go),
  [multi-column tests](document/typed_grid_flow_test.go));
  Pagination preferences now lower across this supported cohort, including
  signature-envelope grouping. Typed QR-verification blocks and the standalone
  document QR field now lower to a bounded content-addressed PNG plus exact
  verification text/link geometry, alternate-text semantics, deterministic
  capture, and PDF replay ([QR adapter](document/typed_qr_plan.go),
  [tests](document/typed_qr_plan_test.go)). QR alignment now matches the legacy
  contract exactly: left/right variants lower into one immutable side-by-side
  row with a four-document-unit gap, center retains its two-unit vertical gap,
  the image keeps its authored size, and verification text remains left-aligned.
  Final measured node ownership retains custom/fallback URL annotations after
  grid lowering, with bounded raster, display SVG, deterministic PDF,
  cancellation, source-detachment, and concurrent-writer evidence. Signature
  rows now retain the legacy eight-document-unit default gap, blank one-column
  placeholder, horizontal signing rules, 12/14-unit line/text offsets,
  24-unit minimum height, compact four-unit text progression, mixed-width
  tracks, artifact semantics for blank placeholders, and exact stroke raster/
  PDF replay ([grid geometry](document/typed_grid_flow.go),
  [display raster](internal/layoutengine/display_raster.go),
  [tests](document/typed_grid_flow_test.go)). Metadata grids and signature rows
  also accept finite non-negative document-unit column gaps while retaining
  their zero-value compatibility defaults; exact geometry, plan-identity drift,
  invalid/width-consuming gap rejection and race evidence
  cover the option ([adapter](document/paper.go),
  [geometry](document/typed_grid_flow.go),
  [tests](document/typed_grid_flow_test.go)). Multi-child section, clause, note,
  metadata-grid, and signature-row boxes now lower finite margins/padding,
  backgrounds, and solid per-side borders through the unified planner.
  Background and side borders repeat for continuation parts; only the authored
  first/last parts receive their corresponding vertical outer edges, with no
  internal page-boundary margin. Signature envelopes keep their existing
  grouping contract rather than adding a public envelope box
  ([fragmentation decision](docs/adr/0002-decorated-container-fragmentation.md),
  [planner](document/paper.go), [tests](document/typed_container_box_test.go)).
  Decorated QR-verification boxes now cover left/center/right variants while
  retaining one immutable image resource, canonical verification links,
  figure semantics, and reading order through deterministic PDF replay
  ([tests](document/typed_container_box_test.go)). Nested independently visual
  child boxes and complete container parity remain pending.
- [ ] Map headers, footers, page numbering, and page breaks. The exact typed
  text/container cohort now supports actual-height first/default headers,
  first/default/even footers, bounded total-page counters, explicit page breaks,
  artifact shell semantics, capture, and PDF replay
  ([adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)); exact content-addressed shell
  images and external links now compose across repeated regions with shared
  resources and translated annotations. Sole-body exact typed tables now
  compose with variable first/default/even shell regions, repeated headers,
  external shell links/images, and bounded corrected page counters
  ([table adapter](document/typed_table_plan.go),
  [tests](document/typed_table_plan_test.go)). Internal header/footer destinations
  and links are now cloned per repeated page, assigned consecutive plan-wide
  identities, and translated with their exact fragment, point, annotation, and
  source provenance through deterministic PDF replay
  ([adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)). Headers and footers with one
  or multiple supported children may also carry bounded margins, padding,
  backgrounds, and solid per-side borders. Their children are planned inside
  one exact group content region; an immutable explicit group border box paints
  behind them without changing child geometry, and explicit shell origins keep
  repeated footer placement exact. Plan, capture, deterministic PDF, invalid
  input, and race evidence cover the cohort
  ([group decoration](internal/layoutengine/box_decoration.go),
  [adapter](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go)). Tables inside row/column
  tracks are covered by the typed container-table cohort above; complete page-
  shell compatibility remains pending.
  Repeated headers and footers may now contain a sole exact table or a mixed
  table/text subtree inside decorated shell content regions. Table-cell links
  clone once per selected page, counters remain corrected after actual-height
  reservation, shell visual fragments remain artifacts, and capture/raster/
  tagged-PDF replay consumes the same finalized geometry
  ([adapter](document/typed_page_template.go),
  [evidence](document/typed_page_template_test.go)). Multi-page shell subtrees
  remain invalid by design; the broad page-shell item stays open.
  Top-level and section/clause/note-nested mixed table flows now
  retain those page shells and corrected counters through one immutable composed
  body plan ([mixed compositor](document/typed_mixed_plan.go)).
- [ ] Preserve links, alt text, tagging, and reading order. Exact canonical
  `http`, `https`, and `mailto` segment links now derive one annotation per
  finalized wrapped glyph-run slice and replay through the shared display/PDF
  path; image alt/decorative semantics and leaf reading order are also retained
  ([link adapter](document/typed_link_plan.go),
  [tests](document/typed_link_plan_test.go)). Named `TextSegment.Destination`
  anchors and `#name` links now resolve from finalized glyph geometry into an
  immutable consecutive destination catalog and exact internal PDF annotations;
  missing, duplicate, empty, and non-canonical targets fail atomically. The
  supported typed section/clause/note, list/list-item, metadata/signature-row,
  image/figure, and QR cohorts now retain nested semantic parents while visual
  leaf ownership and per-page reading order remain exact, with deterministic
  characterization/PDF evidence
  ([semantic adapter](document/paper.go),
  [characterization](document/typed_characterization_fixtures.go),
  [tests](document/typed_link_plan_test.go)). Full PDF tag-hierarchy parity and
  unsupported typed cohorts remain pending. The typed display painter now
  reuses one PDF structure element per retained semantic node across glyph
  runs, fragments, and pages, preserves `/ActualText` and `/Lang`, and keeps
  link annotation parent-tree references attached to the planned Link element
  ([painter](document/layout_display_painter.go),
  [tagged output](document/tagged_pdf.go),
  [regression](document/typed_layout_plan_test.go)); complete PDF/UA role and
  external-validator parity remains pending.
- [x] Keep metadata, attachments, output policy, compliance, and the typed
  signing intent/field identity in a detached document envelope rather than
  layout nodes. Planning snapshots descriptive metadata, authored dates, XMP,
  ICC output intent, compliance/tagging state, PDF version, output defaults,
  attachment bytes, and the PAdES field identity; the envelope participates in
  immutable compatibility-plan identity and is installed only after successful
  preflight on a fresh target. Encryption, active tagged-content state,
  conflicting signer identity, and other unsafe live state are rejected
  atomically; private keys and certificates remain caller-owned and are passed
  only to `OutputSigned*`
  ([adapter](document/typed_layout_plan.go),
  [tests](document/typed_envelope_plan_test.go)).
- [x] Snapshot inline typed attachment bytes, filenames, descriptions, and MIME
  types into the immutable compatibility-plan envelope and install them only
  after successful layout/painter preflight; caller mutation after planning no
  longer changes emitted attachments
  ([adapter](document/typed_layout_plan.go),
  [tests](document/typed_envelope_plan_test.go)).
- [x] Report unsupported custom block implementations deterministically through
  the typed exact-plan adapter with model-path diagnostics and a stable
  `errors.Is` sentinel
  ([implementation](document/typed_layout_plan.go),
  [tests](document/typed_layout_plan_test.go)).

### Stage 3 exit gate

- [ ] All typed production tests run through the unified planner. The public
  `WriteDocument` entry point now defaults fresh supported models to
  `PlanLayoutDocument` plus `WriteLayoutDocumentPlan`, and the successful
  characterization corpus proves byte-identical deterministic output between
  default and explicit replay. Unsupported and non-fresh requests select one
  private whole-document legacy route; remaining unsupported typed cohorts
  keep this broad item open
  ([cutover](document/document_render.go),
  [routing/differential tests](document/typed_default_cutover_test.go)).
- [x] Typed layout goldens pass at plan, raster, text, and semantics levels.
  The complete typed characterization projection is now schema-versioned and
  pinned as a canonical golden, with plan hashes, deterministic PDF/text
  evidence, reading-role semantics, document-level signature/QR/attachment
  envelope coverage, and the independent bounded raster golden
  ([corpus](document/typed_characterization_fixtures.go),
  [runner](document/typed_characterization.go),
  [projection golden](document/typed_characterization_test.go),
  [raster golden](document/characterization_raster_test.go)).
- [x] Every typed break has a ledger explanation. Successful typed
  characterization fixtures retain every finalized `BreakDecision`, and the
  corpus gate verifies stable reason/page/preceding/triggering evidence for
  each record ([runner](document/typed_characterization.go),
  [ledger assertions](document/typed_characterization_test.go)).
- [ ] PDF/UA and PDF/A typed fixtures pass. Unified typed planning now has a
  private content-addressed embedded TrueType resource path: canonical plan
  schema 15 records detached font identity/metrics, the display-list 0.3
  painter verifies bounded immutable font bytes before mutation, and the typed
  PDF/A fixture emits Type0/Identity-H, an embedded subset, ToUnicode, Unicode
  link geometry, and PDF/A-4 identifiers deterministically under concurrent
  replay ([resource contract](internal/layoutengine/glyph.go),
  [typed adapter](document/typed_glyph_shadow.go),
  [painter](document/layout_plan_painter.go),
  [tests](document/typed_embedded_font_plan_test.go)). The combined exit item
  remains open until the unified typed PDF/UA fixture and external validators
  also pass. A typed PDF/UA fixture now exercises embedded UTF-8 text, H1/P/L/
  LI/Table/TR/TH/TD/Figure structure roles, language, ActualText, figure Alt,
  and link annotations through the immutable plan/painter path
  ([fixture test](document/typed_embedded_font_plan_test.go)); external PDF/UA
  and PDF/A validator results remain required.
- [ ] Concurrent compiled resources remain race-free. The default-route,
  corpus-differential, compatibility, and golden cohort passes focused race
  testing, and `./document` plus `cmd/paper-studio` pass their current race
  gates ([cutover tests](document/typed_default_cutover_test.go),
  [characterization race gate](document/characterization_raster_test.go)).
  The full-repository race command has not completed in this workspace because
  its build stopped on disk capacity; the repository-wide gate remains open.
- [x] Calibrated performance and allocation gates pass. The current eleven-cohort
  Apple M2 gate passes all ten-sample median/max budgets, including the exact
  128x4 table workload; the checked report peaks at 40,332,535 B/op and 140,195
  allocs/op after binding typed deterministic inputs without large plan
  projections
  ([benchmark fixture](document/paper_engine_benchmark_test.go),
  [gate](tools/check-paper-engine-benchmark-report.sh),
  [profile](docs/performance/calibrations/apple-m2-go1.26.json)). The
  production-default route now participates in the calibrated profile and
  measures a 3.57 ms/op median, at most 5,355,411 B/op, and 16,803 allocs/op
  across ten Apple M2 samples ([benchmark](document/typed_default_cutover_test.go),
  [baseline](docs/performance/baselines/typed-default-apple-m2.txt)).
- [x] Legacy typed path is private and used only for rollback/shadow comparison.
  `WriteDocument` selects the immutable unified plan for every fresh supported
  model; fallback is one whole-document call with no mixed layout islands.
  `Hooks.OnLayoutEngineRoute` exposes a bounded stable category for fallback-rate
  aggregation without authored content or private renderer types, and malformed
  planning failures remain pre-page atomic rather than silently changing
  engines. Direct legacy calls exist only in package-private shadow/differential
  tests
  ([implementation](document/document_render.go),
  [privacy, atomicity, route, corpus, and race evidence](document/typed_default_cutover_test.go)).

## 6. Stage 4 — HTML-to-IR migration cohorts

### HTML adapter boundary

- [x] Implement an initial strict, cancellation-aware whole-fragment adapter
  for paragraphs, H1-H6 headings, attribute-free spans/line breaks, canonical
  external `href` anchors, bounded compile-time PNG/JPEG data images with
  explicit dimensions, keep-grouped image/figcaption figures, flat
  ordered/unordered lists, definition lists,
  nested `main`/`section`/`article`/`div` block wrappers, and page-break-only
  inline styles. It lowers first to `layout.LayoutDocument`, plans
  through the unified exact path, performs no output mutation during planning,
  and rejects unsupported descendants/CSS/recovery as one atomic fragment
  instead of creating legacy islands
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).
- [x] Retain tokenizer, parser, validation, and malformed recovery
  ([implementation](document/html_tokenizer.go), [characterization](document/html_characterization_test.go)).
- [x] Retain CSS parsing, cascade, specificity, inheritance, and provenance
  ([implementation](document/html_css.go), [resolved snapshot](document/html_resolved_plan.go), [tests](document/html_resolved_plan_test.go)).
- [x] Retain compiled-template and compiled-fragment caches with bounded
  byte/entry accounting and concurrent reuse
  ([cache](document/html_cache.go), [tests](document/html_svg_support_test.go), [reuse tests](document/html_default_cutover_test.go)).
- [x] Retain data-image and SVG compilation security limits with whole-fragment
  atomic failure
  ([image limits](document/html_image_plan_test.go), [SVG limits](document/svg_display_plan_test.go)).
- [x] Resolve selectors, cascade precedence, and inherited text values in a
  whole-fragment HTML frontend snapshot before lowering; strip selector rules
  plus `class`/`id`/inline-style syntax from the selector-free adapter boundary
  ([implementation](document/html_resolved_plan.go),
  [tests](document/html_resolved_plan_test.go)).
- [x] Preserve typed percentages, `auto`, and intrinsic values for planning in
  the bounded box, image, table, and flex cohorts
  ([box tests](document/html_box_plan_test.go), [image tests](document/html_image_plan_test.go), [table tests](document/html_table_cohort_plan_test.go), [flex tests](document/html_flex_distribution_plan_test.go)).
- [x] Ensure the planner contains no CSS selectors or cascade logic: the
  capability scan emits only resolved `layout.TextStyle`, `layout.BoxStyle`,
  selector-free row/column flex metadata, and canonical declaration values;
  boundary tests reject leaked selector state before planning
  ([implementation](document/html_resolved_plan.go),
  [tests](document/html_resolved_plan_test.go)).
- [x] Lower the exact normal-flow visibility subset: resolved `display:none`
  prunes the complete element subtree before block, inline, image, or flex-item
  lowering; `position:static`, `float:none`, and `clear` values remain explicit
  no-op compatibility declarations. Non-static positioning and non-none floats
  fail atomically with a structured unsupported diagnostic
  ([resolution](document/html_resolved_plan.go),
  [lowering](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)). Full positioned and floating
  layout remains open until the shared IR grows containing-block and exclusion
  geometry contracts.

### Fragment entry/exit contract

- [x] Define `StartFrame` page size/profile
  ([adapter](document/html_frame_plan.go),
  [tests](document/html_frame_plan_test.go)).
- [x] Capture margins and current cursor
  ([adapter](document/html_frame_plan.go),
  [tests](document/html_frame_plan_test.go)).
- [x] Capture current font context required for compatibility
  ([adapter](document/html_frame_plan.go),
  [tests](document/html_frame_plan_test.go)).
- [x] Capture auto-page-break policy and body region
  ([adapter](document/html_frame_plan.go),
  [tests](document/html_frame_plan_test.go)).
- [x] Define returned final page/cursor state
  ([adapter](document/html_frame_plan.go),
  [tests](document/html_frame_plan_test.go)).
- [x] Verify failure causes no additional document mutation
  ([tests](document/html_frame_plan_test.go)).
- [x] Test HTML between manual drawing before and after the call
  ([tests](document/html_frame_plan_test.go)).

### Cohort 1 — Text and lists

- [ ] Inline and block text. Initial attribute-free paragraph/span/line-break
  lowering plus nested structural block wrappers is exact. Resolved
  geometry-compatible inline styles now lower across `span`, `strong`/`b`,
  `em`/`i`/`cite`/`var`, and Courier code-family elements into contiguous
  immutable mixed core-font/color runs without changing finalized wrapping;
  `pre`, `blockquote`, and `address` also enter the same block flow. Bounded
  core-font `font-size`, `line-height`, bold, italic, family, underline, and
  line-through changes now use per-run metrics and immutable display geometry.
  A bounded typed mixed core + embedded UTF-8 BMP cohort also covers CJK and
  combining-mark runs with per-run font resources, advances, and decoration
  geometry; full shaping, bidi/RTL/emoji, custom decoration thickness,
  non-core HTML metrics, and complete browser compatibility remain pending
  ([adapter](document/html_unified_plan.go),
  [mixed-run lowering](document/typed_mixed_text_shadow.go),
  [UTF-8 mixed planner](document/typed_mixed_utf8_shadow.go),
  [tests](document/html_text_cohort_plan_test.go),
  [typed font test](document/typed_embedded_font_plan_test.go)).
- [x] Close the bounded core metric-run sub-cohort for ordinary paragraphs and
  table cells: per-run core-font advances, line-height-aware line boxes,
  wrapping, font-resource identity, underline/line-through strokes,
  display-list fallback, and text retention are exact and failure-atomic.
  This is evidence for the broader item above, not a claim of complete
  browser typography parity
  ([planner](document/typed_mixed_text_shadow.go),
  [table bridge](document/typed_table_plan.go),
  [tests](document/html_text_cohort_plan_test.go)).
- [x] Close the bounded mixed core + embedded UTF-8 BMP metric-run
  sub-cohort for typed paragraphs: CJK and combining-mark scalars retain
  per-run core/embedded resource identity, fixed advances, 12/18pt geometry,
  underline strokes, display capture, and PDF replay. Non-BMP shaping,
  bidi/RTL, emoji, and non-core embedded-font parity remain explicitly open
  ([planner](document/typed_mixed_utf8_shadow.go),
  [font resource bridge](document/typed_glyph_shadow.go),
  [tests](document/typed_embedded_font_plan_test.go)).
- [x] Lower the bounded inherited `text-transform` cohort (`none`,
  `uppercase`, `lowercase`, and `capitalize`) before whitespace collapse for
  contiguous inline runs, including Unicode text and atomic rejection of
  unsupported values. The raw compatibility lowering path keeps its original
  segment coalescing behavior
  ([resolution](document/html_resolved_plan.go),
  [lowering](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).
- [x] Headings. H1-H6 lowering retains resolved CSS text/box styles, exact
  heading-level semantics, keeps, display/raster/PDF replay, and browser-like
  default font scales, bold weight, line-height floor, and side-aware margins
  in the ordinary-flow cohort. Flex-item headings intentionally remain on the
  stricter pre-existing geometry contract; nested/richer heading CSS and
  complete compatibility parity remain pending
  ([adapter](document/html_unified_plan.go),
  [resolution](document/html_resolved_plan.go),
  [tests](document/html_resolved_plan_test.go)).
- [ ] Links. The bounded unified cohort now resolves canonical external
  anchors plus same-fragment `#name` destinations through finalized glyph
  geometry and deterministic PDF annotations ([adapter](document/html_images.go),
  [lowering](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)); nested anchors, richer
  attributes, and complete browser compatibility remain open.
- [x] Preserve initial canonical external HTML anchors as authored typed
  segments, map their ranges onto finalized glyph advances after wrapping, and
  emit exact bounded PDF annotations; nested anchors, extra anchor attributes,
  and unsafe URI schemes fail atomically
  ([lowering](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).
- [x] Complete the bounded ordered, unordered, and definition-list cohort.
  Lists nest recursively with native list/list-item ownership; ordered `start`,
  item `value`, HTML `type=1/a/A/i/I`, and exact decimal/alphabetic/Roman/none
  marker styles lower into immutable counter state. Nested definitions retain
  keep-linked term headings and definition paragraphs, inline links preserve
  glyph-bound annotations, and outer break-before/after produces causal page
  boundaries. Non-ASCII unordered bullets and forced breaks inside an atomic
  list item reject the whole fragment
  ([adapter](document/html_unified_plan.go),
  [counter planner](document/paper.go),
  [tests](document/html_text_list_remaining_plan_test.go)).
- [x] Complete the bounded whitespace cohort. Normal collapsing retains exact
  segment/link ownership; `pre`, `pre-wrap`, `pre-line`, non-wrapping `pre`,
  `nowrap`, `break-spaces`, and integer `tab-size` 1..16 reach shared text
  measurement. Tabs expand deterministically at resolved stops before shaping,
  while pinned Firefox evidence verifies one-line nowrap and preserved two-line
  geometry. Broader CSS text breaking/hyphenation remains a separate typography
  concern
  ([adapter](document/html_unified_plan.go),
  [measurement](document/typed_line_shadow.go),
  [browser evidence](document/html_text_list_remaining_plan_test.go),
  [baseline](docs/performance/baselines/html-nested-lists-whitespace-apple-m2.txt)).
- [ ] Page-break controls used by these elements. Initial inline
  `break-before`/`break-after` and legacy `page-break-*` values lower to causal
  explicit page boundaries on supported blocks/wrappers; resolved stylesheet
  cascade and `break-inside:avoid` now reach block/list keep policy, and the
  existing start/final frame retains cursor parity. Named-page/recto-verso
  semantics and complete compatibility parity remain pending
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).
- [x] Implement initial exact nested structural wrapper, direct definition-list,
  and page-break-only inline-style lowering with cancellation, depth/text
  bounds, whole-fragment rejection for other styles/attributes, causal break
  ledger evidence, and deterministic three-page end-to-end tests
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).
- [x] Plan, cursor, raster, semantics, and benchmark parity for the bounded
  text/list cohort. The cohort test covers resolved model lowering, exact
  cursor-compatible fragment planning, direct raster/PDF replay, semantic
  roles, cancellation/concurrent reuse, and the shared compiled-HTML
  end-to-end benchmark; richer CSS metric changes remain intentionally open
  above ([cohort tests](document/html_text_cohort_plan_test.go), [entry/exit tests](document/html_frame_plan_test.go), [benchmark](document/paper_engine_benchmark_test.go)).

### Cohort 2 — Box model

- [x] Margins and padding. Unified block boxes accept bounded non-negative
  one-to-four-value pt/px and containing-block percentage edges, apply side
  overrides after shorthands, and carry their exact fixed geometry through
  nested and multi-child structural wrappers
  ([frontend](document/html_box_plan.go),
  [tests](document/html_box_plan_test.go)). Negative/auto/collapsing margins
  remain outside this deliberately non-collapsing document cohort.
- [x] Borders and backgrounds. Independent solid RGB sides and background
  fills retain background/border/content paint order across direct and nested
  boxes, canonical SVG capture, direct raster, and PDF replay
  ([planner](document/paper_box.go),
  [display attachment](internal/layoutengine/box_decoration.go),
  [tests](document/html_box_plan_test.go)).
- [x] Radius and supported shadow behavior. The exact bounded contract accepts
  one fixed circular radius plus one opaque, non-inset, zero-blur shadow with
  fixed offsets and optional spread. Rounded borders require equal solid RGB
  sides and an opaque interior, so PDF, SVG, and direct raster replay the same
  immutable cubic fills without renderer-specific clearing or effects.
  Multiple shadows, elliptical/percentage/asymmetric radii, alpha, inset, blur,
  unequal rounded sides, and multi-fragment rounded boxes reject atomically
  ([frontend](document/html_box_plan.go),
  [typed visual contract](layout/layout_document.go),
  [planner](document/paper_box.go),
  [immutable decoration paths](internal/layoutengine/box_decoration.go),
  [end-to-end tests](document/html_box_plan_test.go),
  [pinned Firefox geometry/pixels](document/html_box_browser_parity_test.go),
  [display/raster tests](internal/layoutengine/box_decoration_test.go),
  [Apple M2 baseline](docs/performance/baselines/html-rounded-shadow-apple-m2.txt)).
- [x] Width/height/min/max constraints. Positive pt/px and exact
  containing-block percentages resolve before immutable IR planning;
  content-box/border-box conversion, ordered min/max clamping, fixed-point
  wrapping, border/content rectangles, and containing-block overflow rejection
  are executable. Structural-wrapper percentage widths recurse through their
  resolved content boxes; `auto` preserves intrinsic flow height, while
  explicit/min/max heights clamp the immutable border decoration for bounded
  one-page wrappers (multi-page constrained boxes still reject atomically
  rather than inventing continuation geometry).
  ([frontend](document/html_box_plan.go),
  [mixed compositor](document/typed_mixed_plan.go),
  [IR](layout/layout_document.go),
  [planner](document/paper_box.go),
  [tests](document/html_box_plan_test.go)).
- [x] Overflow and break rules. Visible overflow preserves authored paint;
  hidden/clip overflow emits rectangular graphics-state clips shared by SVG,
  PDF, and the deterministic direct rasterizer. Existing resolved
  break-before/after and break-inside policies retain causal pagination and
  cursor behavior. Per-axis mixed overflow and non-rectangular clips reject the
  whole fragment
  ([clip contract](internal/layoutengine/fragment_clip.go),
  [raster](internal/layoutengine/display_raster.go),
  [tests](document/html_box_plan_test.go),
  [break tests](document/html_unified_plan_test.go)).
- [x] Plan, cursor, raster, and benchmark parity. Exact plan/capture/PDF,
  pinned raster digest, live-cursor, semantics, cancellation, concurrent
  compiled reuse, atomic-limit, and pinned Firefox DOMRect tests cover the
  bounded cohort. The browser remains a test-only oracle. A checked
  three-sample Apple M2 baseline records the rich percentage/overflow fixture
  ([tests](document/html_box_plan_test.go),
  [browser oracle](document/html_box_browser_parity_test.go),
  [baseline](docs/performance/baselines/html-box-model-apple-m2.txt)).
- [x] Implement the initial exact indivisible block-box cohort for strict unified
  HTML paragraphs/headings and decorated structural wrappers with exactly one
  text child: bounded non-negative one-to-four-value margins/padding, independent
  solid RGB border sides, background fills, exact fixed-point border/content
  rectangles, canonical background/border/content paint order, and inherited
  break-inside avoidance. Non-default radius/shadow, explicit width/height/min/
  max, clipping overflow, relative edges, nested decorated boxes, and
  multi-child decorated wrappers are now covered by the expanded exact cohort;
  negative/collapsing margins and radius/shadow forms outside the explicitly
  bounded contract still reject the whole fragment atomically
  ([frontend](document/html_box_plan.go), [adapter](document/paper_box.go),
  [canonical decoration](internal/layoutengine/box_decoration.go),
  [HTML evidence](document/html_box_plan_test.go),
  [planner evidence](internal/layoutengine/box_decoration_test.go),
  [PDF evidence](document/layout_box_decoration_painter_test.go)).

### Cohort 3 — Images and figures

- [x] Local PNG/JPEG catalog/file sources require both the explicit document
  security policy and, for live `HTML`, `AllowLocalImages`; data URIs retain
  their bounded compile-time snapshot. Unified planning clones immutable
  source bytes before measurement and never reopens them during paint
  ([resolver](document/html_image_plan.go),
  [policy/snapshot tests](document/html_image_plan_test.go)).
- [x] Resolve intrinsic CSS-pixel geometry, absolute and containing-width
  percentage dimensions, `auto`, percentage/absolute maximums, aspect-ratio
  preservation, `fill`/`contain`/`cover`, left/center/right alignment, and
  missing-source alt fallback through the shared image planner. Percentage
  heights reject atomically because the flow containing height is indefinite
  ([lowering](document/html_unified_plan.go),
  [geometry tests](document/html_image_plan_test.go)).
- [x] Lower one image plus optional styled inline figcaption as one semantic
  figure: the caption is a figure child, retains safe external links and
  resolved text style, keeps with the image, and retries on a fresh body page
  when the captured entry-page remainder is insufficient
  ([lowering](document/html_unified_plan.go),
  [entry/exit contract](document/html_frame_plan.go),
  [figure tests](document/html_image_plan_test.go)).
- [x] Deduplicate repeated compiled and catalog sources by content digest,
  cache repeated catalog names within one immutable snapshot, bound cumulative
  unique bytes/resource count and decoded images, and preserve cancellation,
  concurrent compiled reuse, and failure atomicity
  ([resolver](document/html_image_plan.go),
  [limit/concurrency tests](document/html_image_plan_test.go)).
- [x] Lower the initial strict HTML image cohort from already bounded,
  compile-time-decoded PNG/JPEG data URIs with required finite width/height,
  content-addressed resource reuse, contain geometry, alt/decorative semantics,
  deterministic SVG capture, exact PDF replay, and whole-fragment rejection of
  ambient sources or unsupported attributes
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)); the completed extensions and
  their exact supported boundary are recorded above.
- [x] Lower a strict direct-child `<figure>` contract containing one bounded
  data image and an optional inline figcaption into a keep-together unified
  flow group, preserving caption links, image alt semantics, and exact page
  ownership while rejecting ambiguous child order/content atomically
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).
- [x] Plan, entry/exit cursor, direct raster, deterministic PDF, nested figure/
  caption semantics, caption-link, and allocation benchmark parity. The
  test-only oracle pins Firefox 152.0.5 and exact DOMRect geometry; its 96-DPI
  full-page evidence uses a calibrated 25-per-thousand changed-pixel,
  128-channel, and 1.5 mean-channel ceiling for independent image resamplers.
  The browser is never part of production layout
  ([evidence](document/html_image_plan_test.go),
  [oracle](internal/browseroracle/firefox.go),
  [baseline](docs/performance/baselines/html-image-figure-apple-m2.txt)).

### Cohort 4 — Tables

- [x] Captions, headers, bodies, and footers for the initial strict,
  attribute-free whole-fragment cohort; captions render once before the grid
  and section order/duplication is rejected atomically.
- [x] Deterministic automatic columns plus fixed `pt`/`px`, body-relative
  percentage, intrinsic, minimum, and maximum column constraints. Conflicting
  per-column declarations, inverted min/max bounds, and ambiguous constraints
  on spanning cells reject the whole fragment atomically
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_table_cohort_plan_test.go)).
- [x] Bounded canonical `colspan` and `rowspan` lowering with rectangular-grid
  validation and exact unified table geometry.
- [x] Repeated `<thead>` rows across pagination with preserved repeated-fragment
  evidence and non-repeated captions.
- [x] Structured cell paragraphs, headings, divisions/sections, ordered and
  unordered lists, figures, bounded data images, captions, inline spans, line
  breaks, canonical external links, nested tables, wrapped flex/grid-like
  matrices, and single-child decorated structural boxes lower through the
  shared block planner. Nested payloads are atomic one-page cell content;
  recursive table depth and aggregate rows are bounded, while unsupported
  transforms/clips and ambiguous percentage tracks reject atomically
  ([adapter](document/html_unified_plan.go),
  [planner](document/typed_table_plan.go),
  [tests](document/html_nested_table_cell_plan_test.go),
  [rich-cell tests](document/html_table_rich_cell_plan_test.go)).
- [x] Initial strict solid cell/table backgrounds, per-cell solid borders,
  bounded pt/px padding that participates in measurement, and left/center/right
  plus top/middle/bottom alignment lower to exact fill/stroke/text command order;
  decorations repeat with header fragments and replay through display capture
  and PDF. The bounded collapsed-border cohort splits authored edges at
  rowspan/colspan boundaries, selects the wider solid edge (then the lower
  stable fragment identity on ties), and emits one winning segment per page,
  including repeated headers. Strict inline HTML accepts per-side border
  shorthands and width/style/color components with only `solid`/`none` styles.
  Broader CSS border styles, CSS-standard origin precedence, selector cascade,
  and broader box styling remain pending
  ([adapter](document/html_unified_plan.go),
  [planner](document/typed_table_plan.go),
  [tests](document/typed_table_plan_test.go)).
- [x] Record ten-sample typed large-128-row and wide-32-column table planning
  performance and allocation baselines with calibrated allocation ceilings
  ([benchmarks](document/paper_engine_benchmark_test.go),
  [baseline](docs/performance/baselines/paper-engine-stage0-apple-m2.txt),
  [validator](tools/check-paper-engine-benchmark-report.sh)).
- [x] Plan geometry, keep-together entry/exit cursor, repeated-header causal
  pagination, direct raster, deterministic display/PDF replay, tagged PDF/UA
  table/list/figure semantics, exact caption/cell links, pinned Firefox track
  geometry, cancellation, 16-way detached compiled-plan reuse, and allocation
  benchmark parity
  ([tests](document/html_table_cohort_plan_test.go),
  [oracle](internal/browseroracle/firefox.go),
  [baseline](docs/performance/baselines/html-structured-table-apple-m2.txt)).
- [x] Preserve `<th>` semantics independently of section membership, cell
  links through exact glyph bounds, capture/PDF replay, cancellation and hard
  table work/occupancy/page bounds, with atomic rejection of unsupported CSS,
  attributes, structure, malformed spans, and non-rectangular grids
  ([adapter](document/html_unified_plan.go),
  [tests](document/html_unified_plan_test.go)).

### Cohort 5 — Flex lowering

- [x] Lower the initial non-reverse row and column directions through resolved,
  selector-free flex metadata into the shared `layout.RowColumnBlock` and
  canonical fixed-point row/column planner. The bounded cohort supports direct
  paragraph/heading items, intrinsic/fixed/integral-weight tracks, fixed one-
  or two-value `gap` plus direction-specific `row-gap`/`column-gap` resolution,
  cross-axis start/center/end/stretch, and nowrap;
  unsupported values or a late unsupported item reject the whole plan
  before layout
  ([adapter](document/html_flex_plan.go),
  [tests](document/html_flex_plan_test.go)).
- [x] Complete alignment, justification, and wrapping. Non-wrapping
  `justify-content` start/center/end/space-between/
  space-around/space-evenly after fixed-point track sizing, combining authored
  gaps with deterministic first-slot remainder distribution for rows and
  columns. Direct items also lower exact main-axis fixed `width`/`height`,
  relevant fixed minimums, definite cross sizes, `align-items`, and
  `align-self`; plan geometry, live cursor/PDF replay, direct raster evidence,
  reading-order semantics, invalid-value rejection, and shared solver
  invariants are executable
  ([solver](internal/layoutengine/row_column.go),
  [adapter](document/html_flex_plan.go),
  [tests](document/html_flex_plan_test.go),
  [solver tests](internal/layoutengine/row_column_test.go)).
- [x] Implement the bounded exact multi-line definite-basis flex cohort for row
  and column `wrap`/`wrap-reverse`: greedy ordered line formation, directional
  main/cross gaps, per-line justification, definite cross-axis containers,
  cross-line start/center/end/space-between/space-around/space-evenly/stretch,
  deterministic logical-slot remainder assignment, preserved authored
  fragment/semantic order, cancellation/work/state bounds, and atomic overflow
  diagnostics. Wrapped rows accept positive fixed or percentage main bases and
  resolve integral grow/scaled-shrink factors independently per line; wrapped
  columns additionally require definite item widths. Measured content bases
  for wrapped rows are covered by the extension below; unconstrained intrinsic
  wrapped columns now additionally resolve an omitted cross size from bounded
  measured preferred widths and exact formed-line cross extents. Nested
  composition and external compatibility evidence are covered below
  ([kernel](internal/layoutengine/row_column_wrap.go),
  [kernel tests](internal/layoutengine/row_column_wrap_test.go),
  [adapter](document/html_flex_plan.go),
  [integration evidence](document/html_flex_wrap_plan_test.go)).
- [x] Implement the bounded exact definite-basis sizing cohort: integral
  `flex-grow` and scaled `flex-shrink`, fixed and container-relative percentage
  `flex-basis`, fixed main-axis min/max freezing with iterative deterministic
  redistribution, directional one/two-value gaps, and row/column reverse-main
  placement while authored fragment and reading order stay stable. The shared
  fixed-point kernel bounds work/state, assigns indivisible remainders in
  logical order, rejects unsatisfied minima and malformed mixed track state,
  and is exercised through plan geometry, cursor, raster, deterministic PDF,
  semantics, cancellation, concurrent reuse, wrapped-line and typed-field
  causal tests. Fractional factors, content bases, and percentage bounds are
  covered by the exact extensions below, including recursive nested
  composition and an external browser oracle
  ([kernel](internal/layoutengine/row_column.go),
  [kernel tests](internal/layoutengine/row_column_flex_test.go),
  [adapter](document/html_flex_plan.go),
  [integration evidence](document/html_flex_distribution_plan_test.go)).
- [x] Extend the exact unified flex sizing cohort with measured content/`auto`
  bases (including wrapped rows), six-decimal fixed-point grow/shrink factors,
  container-relative percentage main/cross minimums and maximums, stable
  resolved shorthand/longhand precedence, and bounded structural table items.
  The same planner performs freeze/redistribution and preserves source semantic
  order; executable evidence covers exact geometry, deterministic plans and
  rasters, cursor/PDF replay, table semantics, cancellation, race reuse, and a
  planning allocation benchmark. Recursive flex-in-flex composition and the
  external browser parity corpus are covered by the completion evidence below
  ([kernel](internal/layoutengine/row_column.go),
  [kernel tests](internal/layoutengine/row_column_flex_test.go),
  [adapter](document/html_flex_plan.go),
  [integration evidence](document/html_flex_distribution_plan_test.go)).
- [x] Alignment, justification, and wrapping. The shared fixed-point planner
  covers every supported main/cross alignment for nowrap, wrap, wrap-reverse,
  reverse-main, fixed/percentage/content bases, and bounded intrinsic wrapped
  columns; exact geometry and deterministic remainder behavior are exercised
  in [solver tests](internal/layoutengine/row_column_test.go),
  [wrap tests](internal/layoutengine/row_column_wrap_test.go), and
  [HTML integration tests](document/html_flex_wrap_plan_test.go).
- [x] Nested structured items. Flex containers recursively lower as ordinary
  `layout.RowColumnBlock` children and are measured by recursively planning the
  exact constrained track before their detached display commands, fragments,
  reading order, and semantic subtree are translated into the parent plan.
  Recursion is capped at 64 levels and cancellation, concurrent compiled-input
  reuse, deterministic PDF/cursor replay, direct raster, and atomic depth
  rejection are executable
  ([implementation](document/paper_row_column.go),
  [tests](document/html_flex_nested_plan_test.go)).
- [x] Edge-case compatibility diagnostics. Whole-fragment scanning still
  rejects unsupported/ambiguous properties before layout; track/cross
  overflow, invalid values, excess numeric precision, intrinsic-width bounds,
  recursive-depth exhaustion, cancellation, and unsupported structured
  children return stable atomic failures without retaining pages or a partial
  plan
  ([capability adapter](document/html_flex_plan.go),
  [tests](document/html_flex_plan_test.go),
  [wrap tests](document/html_flex_wrap_plan_test.go),
  [nested tests](document/html_flex_nested_plan_test.go)).
- [x] Plan, cursor, raster, and benchmark parity. The test-only bounded
  WebDriver BiDi oracle pins Firefox 152.0.5, an explicit 240x160 viewport,
  canonical DOMRect JSON, and PNG screenshots. A nowrap alignment/
  justification fixture, recursive nested column fixture, and wrapped-row
  fixture compare browser DOMRects with fixed-point plan rectangles at 1/1024
  point and full-page pixels under pinned 8-per-thousand changed-pixel and 1.5
  mean-channel tolerances. The browser never participates in production
  layout. Nested planning has a checked three-sample Apple M2 allocation/time
  characterization baseline
  ([oracle](internal/browseroracle/firefox.go),
  [corpus](document/html_flex_browser_parity_test.go),
  [benchmark](document/html_flex_nested_plan_test.go),
  [baseline](docs/performance/baselines/html-flex-nested-apple-m2.txt)).

### Cohort 6 — SVG and remaining structure

- [x] Lower the bounded exact single-inline-SVG cohort into the immutable
  display plan: one sole-content SVG per fragment, optionally wrapped by one
  safe external HTML link or containing one full-content external SVG anchor;
  transformed basic shapes and paths; path clips; explicit opaque solid
  nonzero/even-odd fills and strokes; deterministic quadratic-to-cubic
  conversion; ASCII PDF-core-font text with start/middle/end anchors; embedded
  content-addressed PNG/JPEG images; informative figure alt text or decorative
  artifact semantics; canonical capture, supported-fill raster, deterministic
  PDF, link, cursor, cancellation, concurrent compiled-input reuse,
  resource-limit, and atomic-rejection evidence
  The production tranche additionally supports clipped core-font text and
  clipped embedded images; 1/1024-quantized solid path fill/stroke opacity;
  normalized straight-path cap/join/dash paint; deterministic 64-band linear
  gradients with arbitrary finite vectors, centered object-bounding-box or
  user-space radial gradients, and 2-16 independently translucent stops; and
  bounded tiling of
  1-16 opaque, solid, in-tile path elements (at most 256 expanded paints).
  These features lower into the same exact clip/fill/stroke commands used by
  SVG capture, direct raster, and PDF, with explicit resource bounds and
  whole-fragment atomic rejection
  Multiple block-level inline SVGs may now participate in the same unified
  flow as paragraphs and other supported HTML blocks. The typed planner owns
  their exact box placement and pagination; a bounded compositor replaces
  internal planning placeholders with the original vector display commands,
  leaving no placeholder image resources in capture, raster, or PDF output.
  The mixed-flow cohort currently accepts path/shape SVG content; linked SVG,
  SVG text, and embedded SVG images retain the richer sole-SVG route
  ([mixed-flow compositor](document/html_svg_mixed_plan.go),
  [mixed-flow evidence](document/html_svg_plan_test.go),
  [adapter](document/html_svg_plan.go),
  [display lowering](document/svg_display_plan.go),
  [integration evidence](document/html_svg_plan_test.go),
  [lowering evidence](document/svg_display_plan_test.go)). Off-center radial
  focal points, transformed/out-of-tile/rich pattern
  children, styled cubic-stroke raster parity, rich text/image/link SVG inside
  mixed flow, and browser CSS/layout compatibility remain open. Rich SVG
  lowering has a checked three-sample
  Apple M2 allocation/time characterization baseline
  ([benchmark](document/svg_display_plan_test.go),
  [baseline](docs/performance/baselines/html-svg-rich-apple-m2.txt)).
- [x] Make legacy structured HTML table-cell block, nested-list, and nested-
  table measurement consume the same cursor geometry as final painting; use
  that height for row and rowspan allocation, move fitting rows as atomic
  units, reject cells taller than one page body before painting/tagging, and
  preserve nested PDF/UA structure. The compliance fixture now has pinned
  Poppler raster evidence that the nested table ends before the following row
  ([implementation](document/html_tables.go),
  [geometry and atomicity tests](document/html_internal_test.go),
  [compliance raster](cmd/compliance-fixtures/main_test.go)). The unified
  cohort below now covers bounded images, figures, nested tables, flex/grid-
  like matrices, and decorated boxes; SVG remains its separate explicit
  cohort.
- [x] Inline SVG visual commands. Sole-content rich SVG and bounded vector
  SVGs interleaved with ordinary unified HTML both retain vector display-list
  commands through capture, raster, and PDF; mixed-flow placeholders are
  planning-only and are removed before the immutable plan is returned
  ([implementation](document/html_svg_mixed_plan.go),
  [tests](document/html_svg_plan_test.go)).
- [x] SVG links and semantics. One safe external HTML wrapper or one external
  full-content SVG anchor lowers to a whole-fragment annotation and link
  semantic parent; informative SVGs require figure alternate text and
  decorative SVGs lower as artifacts outside reading order
  ([implementation](document/html_svg_plan.go),
  [tests](document/html_svg_plan_test.go)). Per-element SVG links and semantic
  text roles remain outside the bounded cohort.
- [x] Complete the bounded remaining structured table-cell cohort: recursive
  nested tables with semantics, links, decorations, and image resources;
  wrapped row/column flex as deterministic grid-like matrices; and one-child
  decorated structural boxes. Nested projections preserve immutable geometry,
  display commands, semantic subtrees, cancellation, and concurrent reuse,
  with depth/aggregate-row/one-page limits and atomic unsupported rejection
  ([lowering](document/html_unified_plan.go),
  [planner](document/typed_table_plan.go),
  [nested evidence](document/html_nested_table_cell_plan_test.go),
  [flex/box/browser evidence](document/html_table_rich_cell_plan_test.go),
  [baseline](docs/performance/baselines/html-table-rich-cells-apple-m2.txt)).
- [x] Resource and complexity limits. SVG parsing and display lowering bound
  source/image bytes, decoded image pixels, element/path/segment/paint counts,
  and text bytes; cancellation and invalid encoded images reject the entire
  fragment before document mutation
  ([implementation](document/svg_display_plan.go),
  [tests](document/svg_display_plan_test.go),
  [integration tests](document/html_svg_plan_test.go)).
- [x] Plan, cursor, raster, semantics, and benchmark parity for the bounded
  SVG cohort. Sole and mixed-flow tests cover live cursor behavior, vector
  display capture, direct raster/PDF replay, semantic figure/artifact/link
  ownership, cancellation and concurrent compiled reuse; the rich display
  benchmark is separately characterized, while browser-CSS parity and the
  explicitly listed rich SVG extensions remain open
  ([SVG tests](document/html_svg_plan_test.go), [display tests](document/svg_display_plan_test.go), [benchmark](document/svg_display_plan_test.go)).

### Stage 4 exit gate

- [x] Every fragment entering `PlanCompiledHTML` is capability-scanned as one
  unit before layout; a supported prefix followed by an unsupported resolved
  property returns one empty plan without mutating the document
  ([implementation](document/html_resolved_plan.go),
  [tests](document/html_resolved_plan_test.go)).
- [x] Unsupported fragments fall back whole; no legacy islands exist in unified
  flow. The default router plans and preflights before mutation, and invokes the
  private compatibility renderer only for the original complete compiled
  fragment ([implementation](document/html.go),
  [tests](document/html_default_cutover_test.go)).
- [ ] Every cohort passes entry/exit cursor behavior.
- [ ] Typed, HTML, and planner parity corpus passes.

## 7. Stage 5 — HTML default and stabilization

- [x] Make the unified planner default for documented HTML features
  ([implementation](document/html.go), [tests](document/html_default_cutover_test.go)).
- [x] Preserve public `HTML`, `CompiledHTML`, and template entry points
  ([implementation](document/html_template.go), [tests](document/html_default_cutover_test.go)).
- [x] Preserve compiled reuse and render-local concurrency guarantees
  ([tests](document/html_flex_plan_test.go),
  [tests](document/html_nested_table_cell_plan_test.go)).
- [x] Route `LongFormHTMLDocumentModel` through HTML-to-IR
  ([implementation](document/document_longform.go),
  [tests](document/document_longform_test.go)).
- [x] Keep legacy fallback private and observable
  ([implementation](document/html.go), [migration](docs/migration/html-unified-default.md)).
- [x] Record fallback rate and unsupported reasons in test/release diagnostics.
  Stable privacy-safe categories and exact corpus rates are exercised by
  [cutover tests](document/html_default_cutover_test.go).
- [x] Publish a compatibility and migration note
  ([guide](docs/migration/html-unified-default.md)).
- [ ] Run at least one full stabilization release.
- [x] Define error-budget and rollback criteria without claiming a completed
  stabilization release ([guide](docs/migration/html-unified-default.md),
  [benchmark](document/html_default_cutover_test.go),
  [baseline](docs/performance/baselines/html-unified-default-apple-m2.txt)).
- [ ] Confirm the IR has survived both current engines before freezing `.paper`.

## 8. Stage 6 — `.paper` language foundation

### Syntax and parsing

- [x] Pin the initial accepted grammar contract as `paper/0.1` independently
  from the AST projection schema and include it in canonical AST projections
  ([contract](internal/paperlang/ast.go),
  [tests](internal/paperlang/parser_test.go)); per-project migration manifests
  remain pending.
- [x] Implement an initial bounded incremental lexer/reparser and lossless CST:
  the whole-source CST preserves exact physical lines, CRLF/LF endings, trivia,
  scalar spelling, ordered statements, spans, and byte-identical lossless
  printing beside (not instead of) the semantic AST, while sorted minimal byte
  patches reclassify only their affected physical-line envelope, reuse and
  deterministically shift unaffected projections, rebuild opaque ownership,
  and prove equivalence to a clean CST/semantic parse
  ([CST](internal/paperlang/cst.go),
  [incremental API](internal/paperlang/incremental.go),
  [lossless tests](internal/paperlang/cst_test.go),
  [incremental/fuzz tests](internal/paperlang/incremental_test.go)); incremental
  semantic-AST subtree reuse and incremental token-cache persistence remain
  pending.
- [x] Implement the initial indentation-aware lexer and semantic AST for
  document/page/body/heading/paragraph/list/item/text/page-break nodes, typed
  scalars, readable IDs, exact source spans, and deterministic canonical projection
  ([package](internal/paperlang),
  [lexer tests](internal/paperlang/lexer_test.go),
  [parser tests](internal/paperlang/parser_test.go)).
- [x] Preserve comments, whitespace, property order, scalar/string forms, and
  original line endings in the bounded lossless CST, with offset/span lookup
  and explicit opt-in canonical formatting
  ([CST/printing](internal/paperlang/cst.go),
  [tests](internal/paperlang/cst_test.go)).
- [x] Define and implement initial deterministic trivia ownership for property
  edits and move/wrap/unwrap/extract patch planning: contiguous same-indent
  leading comments follow their statement, blank separators remain in place,
  and inline comments plus physical newlines remain line-owned
  ([policy/planners](internal/paperlang/incremental.go),
  [round-trip tests](internal/paperlang/incremental_test.go)); richer user-
  configurable attachment policy remains pending.
- [x] Preserve unknown newer statements and their indented descendants as
  bounded opaque CST regions that round-trip byte-identically even when the
  current semantic grammar rejects them
  ([CST](internal/paperlang/cst.go), [tests](internal/paperlang/cst_test.go)).
- [x] Diagnose duplicate readable IDs, invalid hierarchy, malformed indentation,
  and invalid typed scalars without evaluating interpolation
  ([parser](internal/paperlang/parser.go),
  [tests](internal/paperlang/parser_test.go)).
- [x] Implement a bounded deterministic semantic whole-file formatter with
  canonical indentation, scalar spelling, actionable invalid-AST errors, and
  byte-identical idempotence
  ([formatter](internal/paperlang/format.go),
  [tests](internal/paperlang/format_test.go)).
- [x] Implement bounded deterministic non-overlapping CST patches, exact scalar
  replacement, structural move/wrap/unwrap/extract planners, UTF-8 boundary and
  affected-envelope limits, no-op identity, byte preservation outside edits,
  clean-parse equivalence, and fuzz seeds
  ([patch engine](internal/paperlang/incremental.go),
  [tests](internal/paperlang/incremental_test.go)).

### Types, bindings, and expressions

- [x] Support initial bounded inline schema declarations for primitive, nested
  object, and explicitly bounded list fields
  ([compiler](internal/papercompile/schema.go),
  [tests](internal/papercompile/schema_test.go)); unions, enums, defaults,
  references, maps, and recursion remain pending.
- [x] Support a strict bounded local JSON Schema adapter for closed objects,
  primitive properties, required fields, and explicitly bounded arrays, with
  duplicate-key-aware parsing, deterministic ordering, RFC 6901 error pointers,
  and rejection of unknown/remote/combinator features
  ([adapter](internal/papercompile/json_schema.go),
  [tests](internal/papercompile/json_schema_test.go)); references, unions,
  defaults, enums, open objects, and nested arrays remain pending.
- [x] Support bounded deterministic Go-provided schema descriptors at the
  compile boundary for exported structs, primitives, pointers, arrays, and
  explicitly bounded slices, with tag/nullability handling and rejection of
  cycles and runtime-oriented kinds
  ([adapter](internal/papercompile/go_schema.go),
  [tests](internal/papercompile/go_schema_test.go)); embedded flattening,
  custom marshaling, defaults, maps, and list-of-list contracts remain pending.
- [x] Type-check initial absolute and component-relative binding paths,
  nullability, list traversal, primitive text terminals, and collection
  rejection while preserving source and instance provenance
  ([compiler](internal/papercompile/schema.go),
  [tests](internal/papercompile/schema_test.go)), plus strict resolved-fixture
  validation and detached primitive dotted-path lookup
  ([runtime contract](internal/paperdata/validate.go),
  [tests](internal/paperdata/validate_test.go)), and inject selected-scenario
  string/number/bool values into ordinary and stable-keyed repeated text with
  required/null/type diagnostics while leaving ordinary compilation data-free
  ([evaluation](internal/papercompile/binding_values.go),
  [compiler tests](internal/papercompile/scenario_repeat_test.go),
  [visual/PDF integration](document/paper_scenario_repeat_test.go)); rich-text
  interpolation and non-text binding targets remain pending.
- [x] Implement an initial closed, typed, pure bounded expression bytecode with
  explicit immutable path bindings, validation, cancellation, stack/work/string
  limits, checked integer arithmetic, comparison, boolean, concat, and select
  operations ([VM](internal/paperexpr/vm.go),
  [tests](internal/paperexpr/vm_test.go)), plus a bounded human-readable parser,
  precedence-aware static type checker, source-offset diagnostics, and
  deterministic compiler for literals, dotted bindings, `!`, `+`, `==`, `&&`,
  `||`, conditional selection, and bounded full-string Unicode-scalar wildcard
  `matches`
  ([compiler](internal/paperexpr/language.go),
  [tests](internal/paperexpr/language_test.go)). Scenario compilation now embeds
  that language in visual-node and repeat `when` properties with selected
  fixture and repeat-item contexts
  ([conditions](internal/papercompile/conditions.go),
  [tests](internal/papercompile/conditions_test.go)); formatting built-ins
  remain pending.
- [x] Implement conditions, matches, and bounded loops. Initial typed boolean
  conditions and conditional selection exist in
  [paperexpr](internal/paperexpr/language.go), and a context-cancelable,
  stable-keyed, explicitly bounded predicate-filtered repeat core exists in
  [paperrepeat](internal/paperrepeat/repeat.go) with
  [tests](internal/paperrepeat/repeat_test.go); `.paper` now has explicit
  scenario-selected repeat/`when` syntax, static schema typing, stable
  component/block expansion, provenance, and end-to-end plan/paint adapters
  ([compiler](internal/papercompile/scenario_repeat.go),
  [tests](internal/papercompile/scenario_repeat_test.go),
  [end-to-end](document/paper_scenario_repeat_test.go)), plus scenario-only
  boolean `when` evaluation on visual blocks/list items using the same bounded
  VM, including repeat-item-relative paths, located diagnostics, and preserved
  source/instance provenance while ordinary compilation remains data-free
  ([conditions](internal/papercompile/conditions.go),
  [tests](internal/papercompile/conditions_test.go)). The expression/VM contract
  also supports a full-string, Unicode-scalar wildcard `matches` operator with
  static string typing, `*`/`?`/backslash semantics, compile/runtime pattern
  validation, independent pattern-byte and work limits, and no regex engine or
  ambient behavior ([VM](internal/paperexpr/vm.go),
  [language](internal/paperexpr/language.go),
  [tests](internal/paperexpr/vm_test.go),
  [compiler tests](internal/paperexpr/language_test.go)). Stable-keyed repeats
  can now nest over item-relative object-list fields with composite authored
  prefix/key instance paths, inherited schema/fixture binding contexts, shared
  depth/input/output/work bounds, deterministic diagnostics, component
  templates, predicate filtering, and key-reorder-stable identity
  ([nested expansion](internal/papercompile/scenario_repeat.go),
  [tests](internal/papercompile/nested_repeat_test.go)). General declarative
  non-repeat integer-range loops now require explicit `from`/`through`/`step`,
  `max-iterations`, and instance prefixes; expose only immutable typed
  `loop.index`/`loop.first`/`loop.last` bindings; compose with typed fixture,
  repeat-item, nested-loop, component, and visual-condition contexts; derive
  stable identity from authored prefix plus integer value; and share hard
  cancellation, depth, input, output, node, expression, and work limits
  ([lowering](internal/papercompile/scenario_loop.go),
  [grammar tests](internal/paperlang/loop_test.go),
  [compiler tests](internal/papercompile/scenario_loop_test.go),
  [end-to-end](document/paper_scenario_repeat_test.go)).
- [x] Implement an initial deterministic pinned formatting core for explicit
  `en-US`, `pt-BR`, and `ar` integer/decimal/currency/date/time presentation,
  explicit precision and timezone policy, bidi isolates, and input/output/work
  limits with no ambient locale, clock, or host tzdata
  ([formatter](internal/paperformat/format.go),
  [tests](internal/paperformat/format_test.go)), plus exact-kind scenario-value
  formatting and explicit `.paper` binding properties for format, locale,
  currency, precision, and bare/isolated output
  ([value bridge](internal/paperformat/scenario.go),
  [compiler integration](internal/papercompile/binding_values.go)); additional
  CLDR locales, historical/DST zone rules, date-typed schemas, and expression
  formatting built-ins remain pending.
- [x] Reject I/O, network, process, reflection, environment/filesystem access,
  runtime loading, ambient time, and randomness from the closed expression and
  control-flow runtime, enforced by a repository-source import/linkage audit
  ([capability test](internal/paperexpr/capability_test.go)).

### Initial semantic lowering

- [x] Lower the initial `.paper`
  document/page/body/heading/paragraph/list/item/text/page-break subset directly
  into `layout.LayoutDocument`, including page geometry, margins,
  core text styles, readable-ID mappings, and deterministic diagnostics
  ([compiler](internal/papercompile/compile.go),
  [tests](internal/papercompile/compile_test.go)).
- [x] Connect the initial `.paper` subset end to end through parse, semantic
  compile, multi-block plan composition, one complete painter preflight, and
  direct positioned PDF output without HTML
  ([pipeline](document/paper.go), [tests](document/paper_test.go)).

### Imports and resources

- [x] Implement a versioned canonical bounded package/import lockfile core with
  normalized relative paths, strict JSON, content-addressed entries/assets,
  signature/offline policy fields, deterministic lookup, and explicit hard
  limits ([contract](internal/paperpkg/lockfile.go),
  [tests](internal/paperpkg/lockfile_test.go)), safe offline resolution,
  signature verification, bounded archive validation, and manifest-last atomic
  content-addressed cache installation with exact reopen verification
  ([cache](internal/paperpkg/cache.go),
  [tests](internal/paperpkg/cache_test.go)); network fetching, registry
  resolution, cache GC, and publication remain pending.
- [x] Compute a deterministic transitive project digest across every locked
  import entry and asset digest
  ([contract](internal/paperpkg/lockfile.go),
  [tests](internal/paperpkg/lockfile_test.go)); producing the lock graph from
  source imports remains pending.
- [x] Define explicit-root offline resolution with normalized locked paths,
  `os.Root` containment, configurable internal/all-symlink policy, escape and
  non-regular-file rejection, cancellation, byte bounds, and mandatory streamed
  SHA-256 verification before publication
  ([resolver](internal/paperpkg/resolver.go),
  [tests](internal/paperpkg/resolver_test.go)); multi-root package graphs and
  cache installation remain pending.
- [x] Define per-entry signature/offline policy contracts and an offline-only,
  explicit-root, digest-verifying resolver that has no network capability
  ([lockfile](internal/paperpkg/lockfile.go),
  [resolver](internal/paperpkg/resolver.go)), plus strict canonical offline
  Ed25519 envelopes verified against explicit caller-supplied trusted keys and
  verification time, binding the whole project digest and entry policy
  ([verification](internal/paperpkg/signature.go),
  [tests](internal/paperpkg/signature_test.go)); certificate chains, trust-store
  distribution, revocation, and signed package publication remain pending.
- [x] Enforce bounded no-disk ZIP preflight and extraction planning with strict
  header/overlap/path/type/method validation, compressed/uncompressed/file/depth
  and ratio caps, duplicate/case-fold collision rejection, cancellation, and
  streamed CRC/SHA-256 verification before returning detached content
  ([archive](internal/paperpkg/archive.go),
  [tests](internal/paperpkg/archive_test.go)); ZIP64 and formats other than Store
  and Deflate are intentionally rejected, and cache installation remains pending.
- [x] Pin the current `.paper` planner's exact core-font metrics and encoded
  image resources through a canonical content-addressed catalog; bind the Go
  Unicode table version, explicit unsupported `none` CLDR/hyphenation modes,
  resolved locale, fixed `UTC` timezone, and content-addressed physical page
  profile into an inspectable immutable input manifest, internal `PlanID`, and
  canonical plan hash ([manifest](internal/layoutengine/deterministic_inputs.go),
  [semantic identity](internal/layoutengine/tree.go),
  [pipeline binding](document/paper.go),
  [tests](internal/layoutengine/deterministic_inputs_test.go),
  [end-to-end](document/paper_plan_test.go)); external font programs and real
  CLDR/hyphenation datasets must supply their exact versions when those
  capabilities are introduced.

### Themes, components, slots, and scenarios

- [x] Implement an initial bounded typed theme core with inheritance, nested
  lexical scopes, deterministic nearest-scope lookup, aliases, canonical
  resolved output/digest, and duplicate/unknown/cycle/type diagnostics
  ([resolver](internal/papertheme/resolve.go),
  [tests](internal/papertheme/resolve_test.go)), plus human-readable `.paper`
  theme/scope/token declarations, typed literals/references, stable formatting,
  source-located diagnostics, and isolated compile-time extraction
  ([source bridge](internal/papercompile/theme_source.go),
  [tests](internal/papercompile/theme_source_test.go)), and initial document
  theme selection applies inherited root-scope font/size/line-height/color
  tokens with literal-override diagnostics through exact plan, PDF, display,
  and SVG text color painting
  ([application tests](internal/papercompile/theme_application_test.go),
  [end-to-end](document/paper_theme_test.go)); nested consumer-scope selection
  and the broader style system remain pending.
- [x] Preserve requesting-property source and the complete resolved token alias
  chain for every computed theme property
  ([contract](internal/papertheme/theme.go),
  [tests](internal/papertheme/resolve_test.go)); applied text properties now
  retain a dedicated consumer-to-token-chain mapping with exact source spans
  ([application tests](internal/papercompile/theme_application_test.go)), while
  planner-wide CSS/style provenance remains pending.
- [x] Implement readable typed component `prop @name:` declarations and scalar
  `arg @name: value` invocations with required/default/type checks, exact
  scalar substitution (including nested component uses), deterministic
  ambiguity/recursion/bound failures, stable expansion provenance, and
  lossless/incremental parse-format coverage
  ([grammar](internal/paperlang/ast.go),
  [expander](internal/papercompile/component.go),
  [grammar tests](internal/paperlang/component_test.go),
  [compiler tests](internal/papercompile/component_test.go)); structured
  runtime component objects remain pending.
- [x] Implement initial deterministic compile-time component lowering to the
  existing text/list/row-column primitives with separate definition,
  invocation, and instance-path provenance
  ([expander](internal/papercompile/component.go),
  [tests](internal/papercompile/component_test.go),
  [end-to-end](document/paper_component_test.go)); bindings, expressions, and
  runtime component objects remain pending.
- [x] Implement initial typed `blocks`/`text`/`list`/`row-column` slots with
  required/default cardinality, named fills, hierarchy/type diagnostics,
  cycle detection, hard component/depth/node expansion limits, explicit
  `one`/`many` child cardinality, and scenario-qualified
  layout-affecting fill selection
  ([grammar tests](internal/paperlang/component_test.go),
  [compiler tests](internal/papercompile/component_test.go)); data, asset, and
  page-profile slot contracts remain pending.
- [x] Require layout-affecting slots to declare bounded scenario names and
  require each corresponding fill to name exactly one allowed scenario;
  ordinary compilation rejects scenario-dependent layout, scenario compilation
  selects one type/cardinality-checked fill deterministically, and duplicate or
  ambiguous variants are rejected
  ([expander](internal/papercompile/component.go),
  [compiler tests](internal/papercompile/component_test.go),
  [end-to-end](document/paper_component_test.go)).
- [x] Implement an initial immutable bounded scenario core with forward-parent
  inheritance, locale override, canonical field overlays, set/delete fixture
  mutations, deterministic fixture digests, cycle detection, and detached
  results ([resolver](internal/paperscenario/scenario.go),
  [tests](internal/paperscenario/scenario_test.go)), human-readable `.paper`
  declarations for inheritance, locale, primitive/object/keyed-list fixtures
  with stable parse/format behavior
  ([source bridge](internal/papercompile/scenario_source.go),
  [tests](internal/papercompile/scenario_source_test.go)), plus separate immutable,
  opaque workspace scenario-revision handles
  ([service](internal/paperd/scenarios.go),
  [tests](internal/paperd/scenarios_test.go)), plus redacted scenario-only
  candidate heads and bounded CAS/idempotent set/delete/stable-key replacement
  operations ([service](internal/paperd/scenario_candidates.go),
  [tests](internal/paperd/scenario_candidates_test.go)); source mutation/delete
  syntax and persistence remain pending. Explicitly selected resolved fixture
  values now flow into compiled text and exact visual/PDF output
  ([evaluation](internal/papercompile/binding_values.go),
  [end-to-end](document/paper_scenario_repeat_test.go)).
- [x] Require stable keys for repeated fixture data and diagnose duplicate or
  missing keys during canonical scenario resolution
  ([resolver](internal/paperscenario/scenario.go),
  [tests](internal/paperscenario/scenario_test.go)), and use those keys for
  detached runtime item lookup rather than presentation position
  ([binding](internal/paperdata/validate.go),
  [tests](internal/paperdata/validate_test.go)), and use the same keys for
  bounded predicate-filtered layout instances and item-relative value lookup
  ([compiler](internal/papercompile/scenario_repeat.go),
  [tests](internal/papercompile/scenario_repeat_test.go)); incremental suffix
  reuse remains pending.

### Stage 6 exit gate

- [x] Parse/format/parse is semantically stable for the initial grammar
  ([tests](internal/paperlang/format_test.go)).
- [x] Whole-file semantic formatting is idempotent
  ([tests](internal/paperlang/format_test.go)).
- [x] Mixed revision-bound wrap, property, move, and text-replacement sequences
  preserve leading/inline/trailing trivia and CRLF policy, expose only the
  minimal reconstructable one- or two-patch source diffs, and remain valid
  after every committed revision
  ([tests](internal/paperedit/edit_test.go)).
- [x] `.paper`, typed, and HTML equivalents in the initial common exact cohort
  (paragraphs, headings, lists, and explicit page breaks) produce equivalent
  normalized geometry, display resources/commands, break ledgers, semantics,
  and reading order
  ([tests](document/frontend_equivalence_test.go)); later HTML migration
  cohorts remain gated by their own parity items.
- [x] Warm compiled `.paper` rendering reuses one immutable plan, excludes
  parse/compile/measure/wrap/paginate work, and passes the named ten-sample
  host/toolchain timing and allocation calibration while ordinary tests remain
  free of live wall-clock assertions
  ([benchmark](document/paper_engine_benchmark_test.go),
  [profile](docs/performance/calibrations/apple-m2-go1.26.json),
  [validator tests](internal/perfgate/report_test.go)).
- [x] The expression/control-flow language exposes no arbitrary runtime
  execution: programs contain closed typed constants, paths, and opcodes;
  loops contain only declarative ranges/templates and immutable bindings; and
  the production evaluator/lowering source is capability-audited
  ([VM](internal/paperexpr/vm.go),
  [loop lowering](internal/papercompile/scenario_loop.go),
  [capability test](internal/paperexpr/capability_test.go)).

## 9. Stage 7 — Headless agent and preview tools

### `paperd` foundation

- [x] Maintain source, semantic-template, scenario, and policy revisions as
  separate immutable domains with non-interchangeable revision/candidate
  handles, exact digest/head CAS, candidate-scoped idempotency, and independent
  retention limits ([source](internal/paperd/workspace.go),
  [scenario](internal/paperd/scenario_candidates.go),
  [semantic-template and policy](internal/paperd/revision_domains.go),
  [domain tests](internal/paperd/revision_domains_test.go)).
- [x] Implement bounded immutable source revisions and candidate heads with
  opaque workspace-scoped handles and compare-and-swap publication
  ([workspace](internal/paperd/workspace.go),
  [tests](internal/paperd/workspace_test.go)); the scenario domain now has
  separate immutable revisions and candidate heads with redacted public
  snapshots ([service](internal/paperd/scenario_candidates.go)), while
  semantic-template and policy domains now use the same bounded CAS lifecycle
  ([service](internal/paperd/revision_domains.go)).
- [x] Maintain initial bounded, opaque, workspace-scoped immutable plan handles
  bound to an exact source revision and canonical plan hash
  with explicit revocation, injected-clock expiry, deterministic pruning, and
  immediate capacity reclamation
  ([service](internal/paperd/plans.go), [tests](internal/paperd/plans_test.go));
  render/capture/diagnostic/approval handles and persistence remain pending.
- [x] Scope, expire, revoke, and authorize every currently retained public
  handle family (source/scenario/semantic-template/policy revision and
  candidate, plan, and open) with
  explicit capability and disclosure-domain binding, unique nonces,
  injected-clock expiry, bounded revocation tombstones, constant-shape
  unavailable-handle errors, and legacy error compatibility
  ([lifecycle](internal/paperd/lifecycle.go),
  [tests](internal/paperd/lifecycle_test.go)); transport authentication and
  future render/capture/diagnostic/approval handle families remain pending.
- [x] Recover source/scenario/semantic-template/policy revisions and candidate
  heads after restart from
  an explicit bounded, versioned canonical checkpoint with digest verification,
  manifest-last atomic commit, strict path/symlink/permission checks,
  cancellation and quota enforcement, corruption/truncation rejection, and
  fresh capability issuance on reopen
  ([persistence](internal/paperd/persistence.go),
  [tests](internal/paperd/persistence_test.go)); plans, open capabilities, and
  idempotency responses intentionally remain transient.
- [x] Partition every retained `paperd` compiled/context projection, plan, and
  source/scenario/semantic-template/policy idempotency owner by project, policy
  revision, and disclosure
  domain, with collision tests proving that bypassing outer handle checks still
  cannot produce cross-partition hits
  ([partition](internal/paperd/cache_partition.go),
  [tests](internal/paperd/persistence_test.go)).

### Read tools

- [x] `paper.create` atomically publishes one immutable source revision and
  candidate head without leaking partial capacity
  ([service](internal/paperd/read_tools.go),
  [tests](internal/paperd/read_tools_test.go)).
- [x] `paper.open` issues a revocable bounded read/edit capability pinned to an
  exact revision, digest, and optional candidate head
  ([service](internal/paperd/read_tools.go),
  [tests](internal/paperd/read_tools_test.go)).
- [x] `paper.context` returns detached deterministic source/diagnostic/mapping
  context with exact revision preconditions and item/JSON-byte bounds
  ([service](internal/paperd/read_tools.go),
  [tests](internal/paperd/read_tools_test.go)).
- [x] `paper.components` returns bounded component, slot, property, child, and
  compiler-origin summaries from the exact opened revision
  ([service](internal/paperd/read_discovery.go),
  [tests](internal/paperd/read_discovery_test.go)).
- [x] `paper.inspect` returns a detached bounded node/source/provenance view and
  rejects stale, revoked, ambiguous, or cross-workspace capabilities
  ([service](internal/paperd/read_discovery.go),
  [tests](internal/paperd/read_discovery_test.go)).
- [x] `paper.search` performs deterministic AST-first/compiler-mapping search
  with explicit result, byte, and work bounds plus exact-total disclosure
  ([service](internal/paperd/read_discovery.go),
  [tests](internal/paperd/read_discovery_test.go)).
- [x] Wire bounded structural layout queries to exact retained plan handles in
  the transport-independent service core
  ([service](internal/paperd/plans.go), [tests](internal/paperd/plans_test.go));
  protocol transport and authorization remain pending.
- [x] Implement its bounded internal structural-query kernel with composable
  node/key/instance/fragment/page selectors, detached canonical projections,
  exact totals, and independent truncation
  ([query](internal/layoutengine/query.go),
  [tests](internal/layoutengine/query_test.go)); daemon transport and handle
  authorization remain pending.
- [x] Wire bounded causal layout explanations to exact retained plan handles in
  the transport-independent service core
  ([service](internal/paperd/plans.go), [tests](internal/paperd/plans_test.go));
  protocol transport and authorization remain pending.
- [x] Wire exact fixed-point, bounded hit testing to immutable document plans
  and workspace-scoped plan handles
  ([document adapter](document/paper_plan_tools.go),
  [service](internal/paperd/plans.go),
  [tests](internal/paperd/plans_test.go)); transport and authorization remain
  pending.
- [x] Implement bounded in-process create/open/context/ID-inspect/search/compile/
  edit/render workspace operations as the transport-independent `paperd` core
  ([workspace](internal/paperd), [tests](internal/paperd/workspace_test.go));
  protocol transport, authentication, persistence, and geometry-search wiring
  remain pending.
- [x] Implement the bounded deterministic internal `explain_layout` evidence
  kernel with multi-selectors, continuation chains, resources, causal breaks,
  diagnostics, exact counts, and canonical JSON
  ([explain](internal/layoutengine/explain.go),
  [tests](internal/layoutengine/explain_test.go)); external transport remains
  pending.

### Mutation transactions

- [x] Require exact base revision and support bounded caller-owned idempotency
  keys echoed on deterministic outcomes
  ([contract](internal/paperedit/edit.go),
  [tests](internal/paperedit/precondition_test.go)); durable replay caching belongs
  to the stateful service boundary.
- [x] Implement exact subtree SHA-256 fingerprints and ordered-independent
  per-target preconditions with conflict diagnostics
  ([contract](internal/paperedit/edit.go),
  [tests](internal/paperedit/precondition_test.go)).
- [x] Implement the initial failure-atomic source transaction engine with exact
  SHA-256 revision preconditions, readable-ID targeting, typed property/text/
  insert/delete operations, back-to-front source-span patches, overlap checks,
  candidate reparsing, deterministic diagnostics, and rollback
  ([engine](internal/paperedit/edit.go),
  [tests](internal/paperedit/edit_test.go)).
- [x] Implement typed property values for initial source transactions
  ([contract](internal/paperedit/edit.go)).
- [x] Implement initial readable-ID insert, remove, move, wrap, unwrap, and
  replace-component source transactions with hierarchy checks, reindentation,
  trivia/line-ending preservation, candidate reparsing, and atomic rollback
  ([engine](internal/paperedit/edit.go),
  [tests](internal/paperedit/edit_test.go)).
- [x] Implement `set_literal`, `set_rich_text`, and `set_binding` as separate
  edit-capability operations with exact candidate/head/digest/target
  preconditions, bounded typed payloads, minimal source patches, semantic
  compile validation for bindings, immutable source/semantic diff evidence,
  and payload-sensitive idempotent replay
  ([service](internal/paperd/source_mutations.go),
  [tests](internal/paperd/source_mutations_test.go)).
- [x] Implement fixture-only `set_scenario_value` as a separate scenario-domain
  mutation with exact scenario-candidate/revision/digest guards, stable-key list
  replacement, redacted immutable results, and source-domain isolation
  ([service](internal/paperd/domain_mutations.go),
  [tests](internal/paperd/domain_mutations_test.go)).
- [x] Implement initial `fill_slot` with exact source guards, unique
  component/slot resolution, bounded typed content, slot type and cardinality
  checks, a minimal insertion patch, and atomic compile preflight
  ([service](internal/paperd/domain_mutations.go),
  [tests](internal/paperd/domain_mutations_test.go)).
- [x] Implement initial typed `apply_fix` with a closed four-remedy allowlist,
  source-revision-bound diagnostic SHA-256 fingerprints, exact diagnostic owner
  and target-fingerprint preconditions, bounded remedy-specific payloads,
  instance/fuzzy/ambiguous-target rejection, minimal source operations, atomic
  compile preflight, immutable semantic/invalidation evidence, and idempotent
  replay ([service](internal/paperd/diagnostic_fixes.go),
  [tests](internal/paperd/diagnostic_fixes_test.go)); arbitrary/free-form fixes
  remain intentionally unsupported.
- [x] Implement declaration-safe `rename_id` for the initial grammar
  ([engine](internal/paperedit/edit.go), [tests](internal/paperedit/edit_test.go)).
- [x] Enforce one revision domain per implemented transaction: source mutations
  bind only source candidate/head/digest state while scenario mutations bind
  only scenario candidate/revision/digest state
  ([source guards](internal/paperd/source_mutations.go),
  [scenario guards](internal/paperd/domain_mutations.go),
  [tests](internal/paperd/domain_mutations_test.go)).
- [x] Return bounded immutable semantic-diff evidence for implemented source and
  scenario mutations
  ([source result](internal/paperd/source_mutations.go),
  [scenario result](internal/paperd/domain_mutations.go)).
- [x] Return immutable candidate source/revision, exact original-offset source
  patches, and deterministic conservative invalidation scope
  ([contract](internal/paperedit/edit.go),
  [tests](internal/paperedit/precondition_test.go)).
- [x] Reject fuzzy rebasing and ambiguous instance edits with exact source
  revision, node fingerprint, and canonical source-instance preconditions for
  every addressed operation target; reject expanded component/repeat instance
  mappings instead of selecting or rebasing them; return deterministic bounded
  candidate diagnostics while preserving atomic and idempotent failure
  ([editor](internal/paperedit/edit.go),
  [workspace](internal/paperd/source_mutations.go),
  [editor tests](internal/paperedit/instance_precondition_test.go),
  [workspace tests](internal/paperd/instance_precondition_test.go)).

### Authorization and audit

- [x] Separate structural slot validity from actor authority so invalid slot
  requests fail before authorization while valid mutations require an exact
  live authority under explicit workspace policy
  ([service](internal/paperd/domain_mutations.go),
  [authorization](internal/paperd/authorization.go),
  [tests](internal/paperd/authorization_test.go)).
- [x] Enforce operation-scoped mutation capabilities, exact open/candidate
  binding, node scopes, and explicit grants for configured protected nodes
  ([authorization](internal/paperd/authorization.go),
  [tests](internal/paperd/authorization_test.go)).
- [x] Compute a deterministic bounded semantic effect set before authorization,
  including governing component definitions, component-use instances,
  protected ancestors, owned subtrees, and prospective slot descendants
  ([authorization](internal/paperd/authorization.go),
  [tests](internal/paperd/authorization_test.go)).
- [x] Separate edit, export, publish, attachment, production capture, and sign
  authority with non-interchangeable opaque capabilities and five explicit
  executor entry points
  ([authority](internal/paperd/sensitive_authority.go),
  [execution](internal/paperd/sensitive_execution.go),
  [tests](internal/paperd/sensitive_execution_test.go)).
- [x] Bind one-use approvals to exact candidate evidence, expected head,
  operation-input hash, policy revision, required scenarios, validator
  versions/results, review artifacts, and source/semantic diffs
  ([authority](internal/paperd/sensitive_authority.go),
  [tests](internal/paperd/sensitive_authority_test.go)).
- [x] Implement unique hashed nonces, bounded expiry, explicit revocation,
  atomic one-use consumption, and concurrent replay protection
  ([authority](internal/paperd/sensitive_authority.go),
  [tests](internal/paperd/sensitive_authority_test.go)).
- [x] Audit successful and denied sensitive authorization and execution
  outcomes in a bounded hash-linked log without retaining raw targets,
  payloads, nonces, secrets, or handles; executor failure and panic consume the
  reviewed approval and produce explicit failure evidence before a retry can
  obtain a fresh approval
  ([authorization](internal/paperd/authorization.go),
  [sensitive audit](internal/paperd/sensitive_authority.go),
  [execution](internal/paperd/sensitive_execution.go),
  [tests](internal/paperd/sensitive_execution_test.go)). Durable authenticated
  export and external anchoring remain the separate open item below.
- [x] Chain every bounded sensitive audit decision by canonical previous/event
  hashes, verify the retained chain before anchoring, and periodically sign an
  exact partition/range/root/entries statement through an injected external
  anchor under a separate one-use `sign` approval. The returned detached
  signature, signer/key identity, external anchor URI, and receipt hash are
  themselves hash-bound and success/failure is appended to the audit chain;
  stale roots reject before invoking the signer
  ([chain](internal/paperd/sensitive_authority.go),
  [anchor](internal/paperd/sensitive_anchor.go),
  [tests](internal/paperd/sensitive_anchor_test.go)).

### Visual and scenario tools

- [x] Implement bounded deterministic direct-plan multi-page SVG contact sheets
  with exact page translations, plan/artifact hashes, disclosure, and manifests
  ([artifacts](internal/layoutengine/visual_artifacts.go),
  [tests](internal/layoutengine/visual_artifacts_test.go)).
- [x] Implement an initial bounded deterministic direct-display-list page and
  exact page-local crop renderer producing lossless PNG with pinned fixture
  pixels and canonical manifests
  ([renderer](internal/layoutengine/display_raster.go),
  [tests](internal/layoutengine/display_raster_test.go)); version 1 explicitly
  rejects clips, strokes, even-odd fills, and non-translation transforms before
  output, and does not claim authoritative serialized-PDF raster equivalence.
- [x] Implement deterministic exact border-box node and fragment SVG crops;
  node selectors expand across canonical continuation fragments and record
  page/crop transforms without browser layout
  ([artifacts](internal/layoutengine/visual_artifacts.go),
  [tests](internal/layoutengine/visual_artifacts_test.go)); instance/issue union
  crops remain pending.
- [x] Emit a bounded deterministic cross-page strip as a separate vertical SVG
  presentation artifact, with exact page-local capture translations, overflow-
  aware extents, stable artifact/plan hashes, disclosure metadata, detached
  payloads, and page/byte limits independent from the contact sheet
  ([artifacts](internal/layoutengine/visual_artifacts.go),
  [document tool](document/paper_plan_tools.go),
  [paperd service](internal/paperd/plans.go),
  [tests](internal/layoutengine/visual_artifacts_test.go),
  [service tests](internal/paperd/plans_test.go)).
- [x] Build bounded deterministic multi-page geometry/core-text SVG capture
  bundles with plan/artifact hashes, canonical manifests, and explicit
  disclosure classification
  ([capture](internal/layoutengine/ai_capture.go),
  [tests](internal/layoutengine/ai_capture_test.go)).
- [x] Emit direct-display-list clean PNGs and transparent changed-fragment
  overlays as separate content-addressed layers in one bounded review bundle
  ([bundle](internal/layoutengine/review_bundle.go),
  [tests](internal/layoutengine/review_bundle_test.go)).
- [x] Emit bounded source, semantic, structural-plan, exact changed-pixel
  heatmap, accessibility, and diagnostic before/after evidence in the review
  bundle ([bundle](internal/layoutengine/review_bundle.go),
  [paperd adapter](internal/paperd/plans.go),
  [tests](internal/layoutengine/review_bundle_test.go)); source patches are
  supplied by the transaction boundary, and authoritative final-PDF raster
  comparison remains pending below.
- [x] Implement an initial bounded deterministic structural plan diff with
  stable semantic-fragment occurrences, exact totals, resource/display-list/
  break/diagnostic change flags, and canonical JSON
  ([diff](internal/layoutengine/plan_diff.go),
  [tests](internal/layoutengine/plan_diff_test.go)).
- [x] Complete bounded deterministic scenario generation, boundary finding,
  and recursive minimization. Fixture generation covers empty/max stable-keyed
  lists, localized/unbreakable/complex-Unicode strings, positive/negative and
  precision-extreme numbers; the replayable matrix adds caller-identified
  schema-optional omissions, RFC 3339 date boundaries, exact page profiles,
  and typed missing/truncated/malformed/oversized asset/font fault instructions
  without ambient resource reads. Binary search finds the first page-count,
  break-digest, or overflow change across string-repeat, list-length, and
  integer axes, while the minimizer delta-debugs fields, nested objects, keyed
  list items, and UTF-8 strings under cancellation, work, candidate, byte, and
  evaluation bounds
  ([core](internal/paperscenario/stress.go),
  [matrix](internal/paperscenario/stress_matrix.go),
  [tests](internal/paperscenario/stress_test.go),
  [matrix tests](internal/paperscenario/stress_matrix_test.go)).
- [x] Verify final serialized PDF bytes independently from planning and painting:
  a pinned Poppler consumer renders exact bounded page dimensions, per-pixel
  evidence records changed-pixel PPM, maximum/mean channel deltas and diff
  bounds against the same retained plan's direct-display rasters, final-byte
  inspection records text/page/link/destination/tagging/metadata/PDF-A/PDF-UA
  structure, and exact versioned external compliance report hashes gate required
  profiles. Canonical reports bind PDF, plan, renderer, direct-raster manifests,
  structural expectations, and compliance evidence
  ([verifier](internal/pdfverify/verify.go),
  [Poppler backend](internal/pdfverify/poppler.go),
  [plan raster adapter](document/paper_review.go),
  [paperd service](internal/paperd/final_pdf_verify.go),
  [verifier tests](internal/pdfverify/verify_test.go),
  [service tests](internal/paperd/final_pdf_verify_test.go)).

### Stage 7 exit gate

- [x] An agent can create and edit without full-file rewriting through the
  bounded headless workflow: creation publishes an immutable base and
  candidate, then `set_literal` applies one exact fingerprinted/source-instance
  semantic patch under an actor/node-scoped authority; the returned protocol
  projection contains hashes and patch count rather than source or replacement
  bytes, and restart recovery reconstructs and replays the exact one-patch edit
  from persisted base/head syntax before trusting any caller evidence
  ([workflow](internal/paperd/headless_workflow.go),
  [minimal-edit and adversarial recovery tests](internal/paperd/headless_workflow_test.go)).
- [x] An agent can locate and explain a layout issue structurally through an
  exact retained-plan plus open-revision capability: diagnostic-code, node,
  key, instance, fragment, and page selectors return a deterministic typed
  source/data-binding/style/semantic/layout/break/page/region/display-command
  causal chain; repeated instances return exact refinement candidates instead
  of a guessed target; candidate drift and mismatched plans are rejected
  atomically; source text, glyph codes, semantic replacement text, and raw
  diagnostic values are omitted or hashed under cancellation, work, item, and
  response-byte bounds
  ([service](internal/paperd/structural_explain.go),
  [plan explainer](internal/layoutengine/explain.go),
  [structural query](internal/layoutengine/query.go),
  [service tests](internal/paperd/structural_explain_test.go),
  [engine tests](internal/layoutengine/explain_test.go)).
- [x] An agent can request a failure-atomic bounded review bundle from exact
  opaque retained-plan handles, with detached artifacts and a canonical
  manifest linking plan/resource/revision/scenario/policy/page-profile hashes,
  semantic/accessibility context, exact crops, page transforms, and byte/pixel
  limits ([document adapter](document/paper_review.go),
  [paperd service](internal/paperd/plans.go),
  [service tests](internal/paperd/plans_test.go)).
- [x] Required scenarios and permissions gate candidate acceptance through a
  workspace-configured, canonical policy and a dedicated least-privilege
  `accept_candidate` capability. The production gate requires an exact
  candidate head and source digest, verifies live exact scenario revisions and
  passed scenario/validator/review results, binds the complete result set to a
  one-use policy-scoped approval, commits atomically under candidate CAS,
  invalidates acceptance on edit, supports exact idempotent replay, and emits
  bounded hash-only chained audit evidence without returning source, fixture,
  report, nonce, or reusable scenario capabilities
  ([gate](internal/paperd/candidate_acceptance.go),
  [integration/adversarial/concurrency/race tests](internal/paperd/candidate_acceptance_test.go)).
- [x] A complete create, exact semantic edit, retained-plan structural explain,
  deterministic visual review, policy-gated candidate acceptance, one-use
  approved render, and audited export workflow is possible without a GUI. The
  production two-phase API supports cancellation/resume and fresh-capability
  restart recovery; the `paper workflow` command requires explicit scenario
  and validator report hashes, reviewer nonce, review approval, private literal
  input, deterministic review font, and an atomic output target, and emits only
  bounded hash/audit evidence
  ([orchestration](internal/paperd/headless_workflow.go),
  [CLI](cmd/paper/workflow.go),
  [workflow/replay/concurrency/recovery tests](internal/paperd/headless_workflow_test.go),
  [executable CLI integration test](cmd/paper/main_test.go)).
- [ ] Protocol, privacy, replay, concurrency, and recovery evaluations pass.
  Implemented evidence now includes an authenticated canonical transport
  envelope with signed descending version offers, highest-mutual-version
  enforcement and downgrade rejection; peer/workspace/policy/disclosure
  binding; capability-filtered allowlisted dispatch; bounded deterministic
  responses; replay, time-window, panic, and cross-workspace failure handling;
  hash-only disclosure-denial audits; a deterministic pinned cross-process
  protocol fixture; and an adversarial authentication/version/replay/partition
  corpus ([transport](internal/paperd/protocol_transport.go),
  [transport tests](internal/paperd/protocol_transport_test.go)). A concrete
  fail-closed Unix-domain socket adapter now adds a non-destructive `0600`
  filesystem endpoint, bounded big-endian framing, I/O deadlines, bounded
  concurrency, default same-effective-UID policy, and Linux `SO_PEERCRED`
  or macOS `LOCAL_PEERCRED` plus `LOCAL_PEERPID` verification before the
  authenticated envelope is read or dispatched. Focused macOS tests exercise
  the real kernel credential boundary, framing, restricted parent/path policy,
  peer denial, replacement-socket-safe cleanup, and exact dispatch; the Linux
  peer-credential implementation and kernel-backed integration test
  cross-compile successfully but still require execution on Linux CI
  ([socket adapter](internal/paperd/protocol_unix.go),
  [Linux peer credentials](internal/paperd/protocol_peercred_linux.go),
  [macOS peer credentials](internal/paperd/protocol_peercred_darwin.go),
  [transport tests](internal/paperd/protocol_unix_test.go)). The symmetric
  client validates the restricted socket path and kernel-reported server UID
  before sending the signed envelope, then strictly bounds and decodes one
  response frame. Credential lookup
  failures and denied UIDs close without a response and append only one-way
  peer/request/method identities to the bounded protocol audit; raw PID/UID/GID
  values never enter the audit projection. Persistence
  now coordinates Linux/macOS processes with a root-scoped advisory lock and
  exact generation CAS, rejects stale writers instead of losing updates, and
  has process-kill injection at snapshot write/fsync/replace/directory-fsync
  and manifest write/fsync/replace/directory-fsync boundaries proving recovery
  of the last selected generation
  ([locking](internal/paperd/persistence_lock_unix.go),
  [crash/writer tests](internal/paperd/persistence_process_test.go)). Direct
  workspace-open and persistence disclosure denials are retained and
  optionally emitted as panic-isolated hash-only records
  ([audit](internal/paperd/disclosure_audit.go),
  [audit tests](internal/paperd/disclosure_audit_test.go)). Earlier evidence
  includes optional memory-only HMAC-SHA-256
  authentication for manifest-last workspace generations; durable exact-head
  candidate-acceptance receipts; one-way approval replay hashes that survive
  restart; continued hash-chained sensitive-audit roots; retained public
  external-anchor receipts without raw signatures; strict rejection of
  corrupted authenticated acceptance/audit/anchor state; an explicit
  allowlisted, handle-free headless protocol projection; hashed filenames,
  diagnostic values, scenario labels, semantic replacement text, and unsafe
  instruction-bearing identities in structural explanations; adversarial
  source/data/import/diagnostic injection tests; and byte-identical headless
  protocol fixtures from separate OS processes
  ([persistence](internal/paperd/persistence.go),
  [sensitive state](internal/paperd/sensitive_authority.go),
  [anchor retention](internal/paperd/sensitive_anchor.go),
  [headless protocol](internal/paperd/headless_workflow.go),
  [structural redaction](internal/paperd/structural_explain.go),
  [recovery/authentication tests](internal/paperd/persistence_sensitive_test.go),
  [injection/cross-process tests](internal/paperd/protocol_injection_test.go)).
  The broad gate remains open pending execution of the process-kill and
  kernel-credential suites on Linux CI (the current runtime evidence is
  macOS), peer-credential or authenticated pipe boundaries for other supported
  operating systems, and an independent external protocol/privacy assessment.

## 10. Stage 8 — Read-first Paper Studio

### Workspace

- [x] Page canvas dominates the default workspace at the normal desktop
  breakpoint; the fixed panels occupy at most 35% of the 1280px reference
  viewport and remain independently collapsible
  ([workspace CSS](cmd/paper-studio/web/studio.css),
  [browser-tested shell](cmd/paper-studio/web/index.html)).
- [x] Outline, source, inspector, issues, and scenarios are synchronized. The
  read-first slice synchronizes source selection with exact fragments, pages,
  causal inspection, diagnostic source focus, and scenario buttons that
  atomically swap the revision-bound plan with a reversible Default choice
  ([controller](cmd/paper-studio/web/studio.js),
  [scenario-aware server/tests](cmd/paper-studio/main.go),
  [tests](cmd/paper-studio/main_test.go)); page-hit-to-outline/source focus and
  diagnostic/scenario cross-selection remain open.
- [x] Panels are contextual and collapsible through the shared workspace mode
  and keyboard state ([controller](cmd/paper-studio/web/studio.js),
  [responsive workspace](cmd/paper-studio/web/studio.css)).
- [x] Design, Source, Split, Review, Accessibility, and Reference modes exist
  and share one exact canvas/selection state
  ([shell](cmd/paper-studio/web/index.html),
  [mode controller](cmd/paper-studio/web/studio.js)).
- [x] Page rail shows exact positioned plan issues, first/even/odd page-master
  selector state, retained regions/repeated master content, and semantic/
  display-aware changed pages across one detached prior plan. Baselines are
  revision/scenario bound, explicitly labeled when unavailable or mismatched,
  and never retain prior source bytes
  ([page summary projection](document/paper_plan_tools.go),
  [server retention/API](cmd/paper-studio/main.go),
  [rail controller](cmd/paper-studio/web/studio.js),
  [Go tests](cmd/paper-studio/main_test.go),
  [JS tests](cmd/paper-studio/js_test/rail_model_test.cjs)).
- [x] Keyboard page/zoom/panel navigation and an explicit reduced-motion media
  policy work without changing plan state
  ([controller](cmd/paper-studio/web/studio.js),
  [reduced-motion CSS](cmd/paper-studio/web/studio.css)).
- [x] Visible pages and page-rail thumbnails are rendered by the shared Go
  display-list rasterizer compiled to WebAssembly. The revision-bound endpoint
  emits a strict canonical-plan/resource payload; WASM validates hashes,
  schema, renderer identity, limits, and deduplicated resource blobs before
  producing PNG pixels for canvas presentation. JavaScript performs transport,
  UI control, and pixel presentation only
  ([payload contract](internal/layoutengine/web_display_render.go),
  [document boundary](document/paper_web_render.go),
  [Studio endpoint](cmd/paper-studio/main.go),
  [WASM renderer](cmd/paper-studio-wasm/main_wasm.go),
  [browser bootstrap](cmd/paper-studio/web/wasm-renderer.js),
  [contract/server tests](internal/layoutengine/web_display_render_test.go),
  [real WASM smoke](tools/test-paper-studio-wasm.mjs)).

### Selection and stale-state safety

- [x] Page selection maps to source, outline, instance, style, and semantics.
  Exact page hits persistently select the outline row, reveal the source line,
  request the bounded causal projection, and show instance/semantic/
  reading-order evidence plus exact compiler-computed text/box styles and
  binding provenance ([controller](cmd/paper-studio/web/studio.js),
  [computed style mapping](internal/papercompile/compile.go),
  [provenance projection](document/paper_provenance.go),
  [hit/explain tests](cmd/paper-studio/main_test.go)).
- [x] An identified source selection requests its complete bounded causal
  projection, navigates to its first page, draws every selected fragment on the
  active page, and marks every affected page in the rail
  ([controller](cmd/paper-studio/web/studio.js),
  [explain endpoint and tests](cmd/paper-studio/main_test.go)).
- [x] Overlapping fragments use a bounded picker in the exact reverse
  page-fragment order returned by retained-plan hit testing (visually latest
  first); each alternative independently drives the same revision-bound
  source/explain selection path
  ([picker/controller](cmd/paper-studio/web/studio.js),
  [hit-order contract](internal/layoutengine/hittest.go),
  [Studio endpoint/assets test](cmd/paper-studio/main_test.go)).
- [x] Repeated/master/instance fragments are visually distinct. The bounded
  explanation preserves exact `repeated`, region, key, and instance facts
  ([projection](internal/layoutengine/explain.go),
  [projection tests](internal/layoutengine/explain_test.go)); Studio classifies
  authored, expanded, and repeated occurrences without inventing an authored
  master identity, then draws distinct dotted, dashed, and double/hatched
  overlays with explicit labels
  ([instance model](cmd/paper-studio/web/instance-model.js),
  [controller](cmd/paper-studio/web/studio.js),
  [styles](cmd/paper-studio/web/studio.css),
  [model tests](cmd/paper-studio/js_test/instance_model_test.cjs),
  [API integration test](cmd/paper-studio/main_test.go),
  [browser fixture](testdata/paper/studio-repeated.paper)). Real-browser
  verification on page 2 confirmed two repeated table-header fragments over
  the WASM canvas with no console warnings or errors.
- [x] The canvas displays and binds every page, geometry, hit-test, and explain
  request to the exact immutable plan revision; stale revisions receive HTTP
  409 instead of substitute output
  ([server](cmd/paper-studio/main.go), [tests](cmd/paper-studio/main_test.go)).
- [x] Stale preview is strongly dimmed and covered by an explicit
  `STALE PREVIEW` watermark while the canvas rejects pointer interaction
  ([workspace CSS](cmd/paper-studio/web/studio.css),
  [asset/security tests](cmd/paper-studio/main_test.go)).
- [x] Visual mutations and hit-test edits fail closed while revisions differ or
  the preview is stale: edit, authoring, and resource-replacement controls are
  disabled and hit-test/explanation responses are discarded when the captured
  revision no longer matches ([revision gate](cmd/paper-studio/web/mutation-gate.js),
  [Studio wiring](cmd/paper-studio/web/studio.js), [browser asset/route test](cmd/paper-studio/main_test.go),
  [gate tests](cmd/paper-studio/js_test/mutation_gate_test.cjs)).

### Inspection

- [x] Show margin, border, padding, and content boxes. Fragments retain four
  validated, strictly nested fixed-point rectangles through core/Paper
  planning, nested composition, canonical storage, and Explain output. Studio
  projects each layer directly from that immutable evidence without browser
  layout inference
  ([plan contract](internal/layoutengine/plan.go),
  [core planner](internal/layoutengine/box.go), [Paper planner](document/paper.go),
  [Explain projection](internal/layoutengine/explain.go),
  [inspection model](cmd/paper-studio/web/inspection-model.js),
  [overlay controller](cmd/paper-studio/web/studio.js),
  [exact fixture](testdata/paper/studio-box-model.paper),
  [plan tests](internal/layoutengine/box_test.go),
  [Paper tests](document/paper_box_model_test.go),
  [model tests](cmd/paper-studio/js_test/inspection_model_test.cjs)).
- [x] Show baselines, grid tracks, table cells, and page regions. Every
  fragment can display its exact retained page-region association, and every
  page-local planned line can display its absolute retained baseline without
  inferring browser geometry
  ([inspection model](cmd/paper-studio/web/inspection-model.js),
  [overlay controller](cmd/paper-studio/web/studio.js),
  [model tests](cmd/paper-studio/js_test/inspection_model_test.cjs)).
  Fragment explanations also retain the exact semantic-owner path through a
  table-cell ancestor, allowing Studio to draw and deduplicate ordinary and
  header cell boxes without key-name heuristics
  ([semantic projection](internal/layoutengine/explain.go),
  [projection tests](internal/layoutengine/explain_test.go)). Real-browser
  verification confirmed nine exact baseline marks and nine semantic cell
  boxes (one header) over the WASM page with clean console logs. Grid and
  table planners now retain bounded, immutable column/row track records through
  plan transforms, canonical and segmented stores, Explain, and the Studio
  Tracks overlay ([plan contract](internal/layoutengine/plan.go),
  [grid planner](internal/layoutengine/grid.go),
  [table paginator](internal/layoutengine/table.go),
  [segmented persistence](internal/layoutengine/plan_store_segmented.go)).
  Real-browser verification confirmed 15 exact track marks on both the first
  and continuation pages, including a separately retained repeated-header
  group, over the rebuilt WASM preview with clean console logs. Page-master
  planners and Paper page templates now also retain validated, non-overlapping
  header/body/footer rectangles through canonical/segmented persistence and
  Explain; Studio's Regions layer consumes only those exact rectangles rather
  than fragment-sized proxies ([region contract](internal/layoutengine/plan.go),
  [page-master population](internal/layoutengine/page_master.go),
  [Paper shell composition](document/typed_page_template.go),
  [region projection model](cmd/paper-studio/web/inspection-model.js)). The
  region model and Paper header/body/footer geometry are covered by automated
  tests. Final in-browser activation confirmed three exact non-overlapping
  header/body/footer marks, exact-plan evidence counts of 3/3, a current WASM
  preview, and clean browser logs.
- [x] Show overflow, clipping, and collisions, and offer explicit replacement
  for unavailable fonts. Studio has separate exact-evidence overlays for
  positioned overflow diagnostics, retained clip commands/image crops, and
  positive-area fragment intersections (excluding containment). Unsupported
  fonts remain strict compile errors with no automatic fallback; the diagnostic
  offers a user-selected replacement from the compiler's existing supported
  core fonts through one revision-guarded semantic source patch
  ([inspection model](cmd/paper-studio/web/inspection-model.js),
  [overlay controller](cmd/paper-studio/web/studio.js),
  [edit model](cmd/paper-studio/web/edit-model.js),
  [semantic mutation](internal/paperd/semantic_layout_mutations.go),
  [model tests](cmd/paper-studio/js_test/inspection_model_test.cjs),
  [mutation tests](internal/paperd/semantic_layout_mutations_test.go),
  [Paper fixture](testdata/paper/studio-issues.paper),
  [fixture test](document/paper_issue_inspection_test.go)). Real-browser
  verification confirmed exact overflow (87.5%, 12.5%, 12.5%, 10%), clip
  (8.33333%, 50%, 33.3333%, 12%), and collision (27.0833%, 25%, 14.5833%,
  10%) marks over a 480x400 WASM preview. A separate strict-font pass confirmed
  zero pages before the explicit choice, one exact Helvetica patch after the
  click, a recovered 480x320 WASM preview, no fallback control, and clean logs.
- [x] Show reading order and PDF tags. Accessibility mode activates exact
  page-local reading indexes and semantic-role overlays from the retained plan,
  while its tag tree comes from a separate deterministic tagged-PDF
  serialization and bounded final-byte verifier—not from those plan roles
  ([inspection API/UI](cmd/paper-studio/main.go),
  [controller](cmd/paper-studio/web/studio.js),
  [final-PDF endpoint](cmd/paper-studio/studio_tags.go),
  [fail-closed browser model](cmd/paper-studio/web/tag-model.js),
  [tag verifier](internal/pdfverify/tags.go),
  [tag-aware Paper painter dispatch](document/paper.go)). The verifier binds a
  PDF SHA-256 and validates catalog marking, StructTreeRoot/ParentTree/Document
  linkage, parent reachability, cycles, depth, roles, MCR/MCID counts,
  marked-content stream balance, and accessibility attributes. Real-browser
  verification on the three-page repeated-table fixture confirmed 55 final-PDF
  structure elements and 23 marked streams (Document/Table/TR/TH/TD/P), plus 9
  reading-order and 18 role marks on page 1 over a 360x192 WASM preview, with
  clean browser logs.
- [x] Show the break ledger in the page inspector. Exact causal break
  decisions label their retained triggering or preceding fragment. The bounded
  compiler-owned typed characterization corpus remains available through its
  exact-revision test endpoint, but its internal fixture dump is not exposed in
  the Studio UI ([typed experiment endpoint](cmd/paper-studio/studio_experiments.go),
  [controller](cmd/paper-studio/web/studio.js),
  [Go evidence](cmd/paper-studio/main_test.go),
  [model tests](cmd/paper-studio/js_test/typed_experiment_model_test.cjs)).
- [x] Show data binding and style-token provenance. Explain and Inspect responses
  carry a detached, exact-plan projection of compiler binding paths and full
  resolved token chains; the Studio Inspector filters it by retained fragment
  source identity and renders page-level counts plus selected-node evidence
  without exposing scenario values or raw resources
  ([plan projection](document/paper_provenance.go),
  [Explain envelope](document/paper_plan_tools.go),
  [Studio model](cmd/paper-studio/web/provenance-model.js),
  [Inspector](cmd/paper-studio/web/studio.js),
  [Go evidence](document/paper_provenance_test.go),
  [Studio evidence](cmd/paper-studio/main_test.go),
  [model tests](cmd/paper-studio/js_test/provenance_model_test.cjs)).
- [x] Distinguish plan preview, PDF verified, and verification stale. The Studio
  status badge starts as an exact plan preview, changes to PDF verified only
  after the revision-bound final serialized-PDF tag verifier passes, and
  becomes Verification stale as soon as a verified revision is replaced or a
  refresh is in flight; unavailable plans are explicit
  ([workspace/tag endpoints](cmd/paper-studio/main.go),
  [final-PDF verifier boundary](cmd/paper-studio/studio_tags.go),
  [status state](cmd/paper-studio/web/studio.js),
  [status styles](cmd/paper-studio/web/studio.css),
  [UI/route evidence](cmd/paper-studio/main_test.go)).

### Stage 8 exit gate

- [x] Any visible content pixel can be traced to its complete causal chain.
  Revision-bound hit testing returns the exact source fragment, and the Studio
  immediately follows that identity through outline/source focus and the
  bounded Explain command/fragment evidence ([hit-to-explain test](cmd/paper-studio/main_test.go),
  [controller](cmd/paper-studio/web/studio.js)).
- [x] Normal visible-page updates meet calibrated latency budgets. The backend
  now exercises the canonical WASM render-payload path with ten-sample stage
  timings, a calibrated backend/WASM budget checker, and a revision-safe source
  change stream; the Apple M2 report records cold planning, payload transfer/
  decode, WASM paint, file notification, and incremental replanning
  ([benchmark](tools/benchmark-paper-studio-wasm.mjs),
  [report](docs/performance/baselines/paper-studio-wasm-latency-apple-m2.txt),
  [budget checker](tools/check-paper-studio-latency-report.mjs),
  [change stream](cmd/paper-studio/main.go)). A real-browser trace now records
  cold reload settling, WASM payload decode/canvas compositing, three warm
  Refresh plan interactions, and the approved <=10,000 ms cold / <=4,000 ms warm
  visible-page budgets ([browser baseline](docs/performance/baselines/paper-studio-browser-latency-browser-plugin.txt),
  [real WASM smoke](tools/test-paper-studio-wasm.sh)).
- [x] The internal Plan Viewer contracts remain valid in the real workspace.
  The real Paper Studio workspace exercises revision-bound page, hit, explain,
  inspect, provenance, typed-experiment, SVG, and WASM display-payload routes
  in one integration test ([workspace contract test](cmd/paper-studio/main_test.go)).
- [x] Studio uses no substitute browser layout or JavaScript page renderer:
  visible pages and thumbnails use the shared Go display-list rasterizer
  compiled to WASM, while geometry overlays remain immutable diagnostic SVG.
  Neither path measures, wraps, positions, fragments, or paginates
  ([architecture](ARCHITECTURE.md), [payload](document/paper_web_render.go),
  [WASM renderer](cmd/paper-studio-wasm/main_wasm.go),
  [Studio controller](cmd/paper-studio/web/studio.js),
  [tests](cmd/paper-studio/main_test.go)).

## 11. Stage 9 — Semantic direct manipulation and review

### Create-to-deliver

- [x] New template flow. Studio now supports page-master bootstrap, the typed
  primitive/component gallery, schema starter creation, explicit project-
  relative design imports, and preview-to-export delivery through the same
  authority-bound semantic journal. Each action remains a bounded readable CST
  patch that preserves source trivia ([mutation](internal/paperd/authoring_mutations.go),
  [source edit grouping](internal/paperedit/edit.go),
  [UI](cmd/paper-studio/web/authoring-model.js),
  [tests](internal/paperd/authoring_mutations_test.go),
  [Studio delivery test](cmd/paper-studio/studio_edit_test.go)).
- [x] Schema connection and binding picker. The compiler now exposes detached
  schema descriptors from its existing analysis; Studio projects exact
  primitive/list paths and bindable source targets, rejects stale source/plan
  metadata, and commits through `PaperSetBinding` rather than inventing a
  browser schema parser. Binding transforms now include compiler-validated
  format kind, locale/currency, requiredness, and bounded decimal precision;
  the schema starter also creates a valid first field without raw CST input
  ([metadata](internal/papercompile/schema.go),
  [server](cmd/paper-studio/studio_authoring.go),
  [browser model](cmd/paper-studio/web/authoring-model.js)). Nested object/list
  field insertion is now atomic and compiler-validated, with exact nested
  field targets exposed to the picker ([mutation](internal/paperd/authoring_mutations.go),
  [source edit](internal/paperedit/edit.go),
  [tests](internal/paperd/authoring_mutations_test.go)).
- [x] Scenario creation and stress matrix. Studio offers bounded empty,
  typical, and stress presets generated from compiler-owned schema metadata;
  one selected case becomes one authority-bound journal insertion with stable
  keyed-list items and exact stale rejection. Authored scenarios can now also
  be renamed or deleted through exact-revision lifecycle actions, and the UI
  exposes those actions beside each matrix row
  ([mutation](internal/paperd/authoring_mutations.go),
  [UI](cmd/paper-studio/web/studio.js),
  [end-to-end test](cmd/paper-studio/studio_edit_test.go)). Matrix creation now
  accepts bounded unique `id:preset` cases atomically, and scalar fixture values
  can be edited by scenario-relative path with exact type preservation
  ([source edit](internal/paperedit/edit.go),
  [browser model](cmd/paper-studio/web/authoring-model.js),
  [tests](internal/paperd/authoring_mutations_test.go),
  [Studio integration](cmd/paper-studio/studio_edit_test.go)).
- [x] Initial page-master bootstrap creation. A parseable document with no page
  can now receive one exact `page`/`body`/starter-paragraph template through
  the journal, while existing-page documents reject the bootstrap shape;
  Studio exposes it only when the document has no page
  ([mutation](internal/paperd/authoring_mutations.go),
  [Studio metadata/UI](cmd/paper-studio/studio_authoring.go),
  [tests](internal/paperd/authoring_mutations_test.go)). Named/custom
  page-master creation and a full gallery remain open.
- [x] Typed primitive/component palette. Studio now exposes a typed palette for
  paragraph, heading, list, row, column, page-break, section, and declared
  component instances. Each choice lowers through the same authority-bound
  one-patch journal; component choices are compiler/source-derived and reject
  unknown references ([mutation](internal/paperd/authoring_mutations.go),
  [Studio authoring](cmd/paper-studio/studio_authoring.go),
  [browser model](cmd/paper-studio/web/authoring-model.js),
  [palette tests](internal/paperd/authoring_mutations_test.go),
  [Studio integration](cmd/paper-studio/studio_edit_test.go)).
- [x] Slot-aware target analysis. The internal review model projects body/row/
  column destinations and unfilled component slots with their declared
  accepted primitive kinds, never treating an expanded instance as an authored
  target; the current Studio UI does not expose the editing-contract workflow
  ([review model](cmd/paper-studio/web/review-model.js),
  [model tests](cmd/paper-studio/js_test/review_model_test.cjs),
  [slot authority](internal/paperd/domain_mutations.go)).
- [x] Resource manager for fonts/assets/licenses/fallback/crop focus. Paper
  Studio now has a bounded read-first resource inventory tied to the exact
  source revision, scenario, and immutable plan. It shows content name, media
  type, SHA-256, byte size, decoded dimensions, alt/decorative usage, crop
  focus, and referencing source node without returning filesystem paths, raw
  bytes, or server capabilities. Font entries additionally expose validated
  family, weight, style, license, and acyclic fallback metadata. Images may
  declare acyclic same-kind replacement edges and default crop focus; Studio
  can apply one declared replacement to one exact usage through the authority-
  bound semantic journal. Assets enter only through an explicit
  `-assets` JSON manifest and an explicit/defaulted manifest-directory
  `-asset-root`; the reusable loader rejects absolute/traversing paths,
  symlink components, non-regular files, unknown fields, invalid signatures,
  digest mismatches, and bounded-count/byte violations
  ([loader](internal/paperassets/loader.go),
  [server projection](cmd/paper-studio/studio_resources.go),
  [read-first UI model](cmd/paper-studio/web/resource-model.js),
  [tests](cmd/paper-studio/studio_resources_test.go)). Resource responses now
  validate source revision, workspace revision, and plan hash independently.
  With an explicit manifest, Studio now also supports exact-revision add/remove
  catalog mutations by project-relative file and metadata; the loader derives
  digests, revalidates byte/manifest budgets, the enforced font-license policy,
  and lifecycle relationships, and
  atomically publishes the manifest before reloading the immutable planning
  catalog ([catalog editor](internal/paperassets/loader.go), [mutation route
  and tests](cmd/paper-studio/studio_resource_catalog.go)). The browser still
  receives no paths or bytes. Production TTF/OTF resources now enter the
  immutable planner catalog, participate in exact font lookup and metrics, and
  are passed to the existing UTF-8 subsetter during PDF painting; manifest
  lifecycle and license admission remain enforced ([catalog](internal/papercompile/asset_catalog.go),
  [planner/PDF path](document/paper.go),
  [loader projection](internal/paperassets/loader.go),
  [embedding test](document/paper_image_test.go)). WOFF2 remains metadata-only
  until a compatible shaping/subsetting adapter is added.
- [x] Preflight, explicit PDF verification, export, and publish status. Studio
  now exposes revision-bound preflight, independent final-PDF tag verification,
  a verified-PDF export endpoint, and an explicit separate-authorized publish
  state that cannot be inferred from export
  ([delivery endpoint](cmd/paper-studio/studio_delivery.go),
  [export route](cmd/paper-studio/studio_delivery.go),
  [Studio status UI](cmd/paper-studio/web/studio.js),
  [integration evidence](cmd/paper-studio/main_test.go)).

### Direct manipulation

- [x] Studio semantic edits cross an authority-bound server session without
  exposing paperd capabilities to browser state. Every request names the exact
  source digest and plan hash; the server rejects stale selections, creates a
  restricted edit open, grants only the selected operation and complete direct
  effect scope, applies a bounded typed mutation, and atomically compare-and-
  swaps the source file. Concurrent requests reviewed against one revision
  have exactly one winner
  ([server boundary](cmd/paper-studio/studio_edit.go),
  [browser intent model](cmd/paper-studio/web/edit-model.js),
  [HTTP/concurrency/adversarial tests](cmd/paper-studio/studio_edit_test.go),
  [browser-model tests](cmd/paper-studio/js_test/edit_model_test.cjs)).
- [x] Flow insertion/reordering uses valid semantic drop positions. A closed
  `move_node` authority operation validates body/row/column destinations,
  exact target and destination fingerprints/instances, parser-compatible
  child kinds, descendant rejection, and failure-atomic source patches;
  Studio exposes only compatible destinations and emits the destination ID
  as semantic intent ([service](internal/paperd/flow_mutations.go),
  [atomic edit](internal/paperedit/edit.go),
  [Studio model/session](cmd/paper-studio/web/edit-model.js),
  [tests](internal/paperd/flow_mutations_test.go)). Real-browser drag/drop
  evidence remains an exit-gate requirement.
- [x] Grid handles write readable track values. The revision-safe service
  foundation now exposes a closed typed operation for `track`, `track-size`,
  `track-min`, and `track-weight` on direct row/column children. It requires
  exact child and parent preconditions, authorizes the complete direct effect,
  emits one minimal CST property patch, and compiles the candidate before
  publication ([mutation](internal/paperd/semantic_layout_mutations.go),
  [compiler](internal/papercompile/compile.go),
  [tests](internal/paperd/semantic_layout_mutations_test.go)). The Studio
  inspector now drives this operation through an exact source/plan session,
  grants both child and parent effects, and exposes deterministic commit/stale
  feedback ([session](cmd/paper-studio/studio_edit.go),
  [controls](cmd/paper-studio/web/studio.js),
  [tests](cmd/paper-studio/studio_edit_test.go)). The browser inspector now
  exposes the same closed handle and its source/plan-locked commit state;
  deterministic server/capture coverage is retained in the Studio interaction
  corpus.
- [x] Table handles write tracks, headers, and split policies. The language and
  production-planner foundation now has distinct lossless `table`,
  `table-track`, `table-header`, `table-row`, and `cell` nodes; bounded fixed,
  minimum, and maximum tracks; captions; row/column spans; header repetition;
  row split policy; canonical table/row/cell semantics; authored table source
  identity; deterministic SVG/raster/PDF output; and a closed revision-safe
  `PaperSetTableProperty` service operation
  ([grammar](internal/paperlang/table_test.go),
  [compiler](internal/papercompile/table_test.go),
  [production plan/render evidence](document/paper_table_test.go),
  [mutation](internal/paperd/table_mutations_test.go)). The checkbox remains
  open even though authority-bound Studio controls now edit table tracks,
  headers, and split policy with an exact governing-table precondition
  ([session](cmd/paper-studio/studio_edit.go),
  [controls](cmd/paper-studio/web/edit-model.js),
  [tests](cmd/paper-studio/studio_edit_test.go)): the richer nested-cell
  interaction corpus and deterministic browser before/after evidence are
  retained in the Studio interaction corpus.
- [x] Box handles write padding/border/radius/background. Paragraphs,
  headings, lists, and row/column text children now lower readable padding,
  per-side border widths, border color, radius, and background properties into
  the unified box contract. A closed revision-safe service operation writes
  one property and compile-checks it before publication
  ([compiler](internal/papercompile/compile.go),
  [mutation](internal/paperd/semantic_layout_mutations.go),
  [compiler tests](internal/papercompile/box_properties_test.go),
  [mutation tests](internal/paperd/semantic_layout_mutations_test.go)). Real
  Studio inspector controls now issue these exact operations and distinguish
  committing, committed, stale, and rejected states
  ([controls](cmd/paper-studio/web/studio.js),
  [session](cmd/paper-studio/studio_edit.go),
  [tests](cmd/paper-studio/studio_edit_test.go)). The checkbox remains open
  The real Studio inspector has been exercised through selection, exact box
  handle, one minimal patch, and committed preview refresh; the deterministic
  interaction/capture corpus remains the repeatable evidence.
- [x] Image handles write fit, focus, dimensions, and alt text. The language
  and service foundation now has a lossless `image @name` node with bounded
  deterministic inline PNG/JPEG resources, dimensions, contain/cover fit,
  crop focus, alt text, explicit decorative status, caption, and box styling.
  `PaperSetImageProperty` uses a closed vocabulary, exact source guards,
  transitive authorization, minimal CST patches, and compile-before-publish
  ([parser/compiler](internal/papercompile/image_test.go),
  [end-to-end plan/render/capture tests](document/paper_image_test.go),
  [mutation/tests](internal/paperd/semantic_layout_mutations.go)).
  Authority-bound Studio controls now edit fit, focus, dimensions, alt text,
  and decorative status through the private exact-revision session
  ([session](cmd/paper-studio/studio_edit.go),
  [controls](cmd/paper-studio/web/edit-model.js),
  [tests](cmd/paper-studio/studio_edit_test.go)). The checkbox remains open:
  Studio controls and the deterministic interaction corpus cover the closed
  exact-revision operation; human-readable
  `source: "asset:name"` references now resolve only through an explicit
  immutable catalog whose mandatory SHA-256 digest matches bounded PNG/JPEG
  bytes; compiler and public planning/rendering APIs never search ambient files
  or URLs, and plans retain the verified content identity
  ([catalog](internal/papercompile/asset_catalog.go),
  [compiler evidence](internal/papercompile/image_test.go),
  [production evidence](document/paper_image_test.go)). The complete lockfile/
  resource-manager authoring workflow remains open.
- [x] Canvas handles write explicit anchors/constraints. Human-readable
  `canvas` and `anchor` nodes lower expressions such as
  `left: "@peer.right + 8pt"` into the existing bounded local canvas DAG.
  Production plans retain source identities, positioned fragments, display
  fills/borders, semantics, Query/Explain evidence, SVG/raster captures, and
  PDF output ([compiler](internal/papercompile/canvas_test.go),
  [planner](document/paper_canvas.go),
  [evidence](document/paper_canvas_test.go)). Revision-safe paperd and Studio
  handles require the governing canvas precondition, authorize both effects,
  publish one readable patch, and prove an exact capture change
  ([mutation](internal/paperd/semantic_layout_mutations.go),
  [Studio test](cmd/paper-studio/studio_edit_test.go)). Canvas blocks now also
  compose with preceding/following flow content and authored page shells while
  preserving child source/semantic identity and exact display commands. The
  item remains open for nested canvas text/images; the governing-canvas
  precondition, exact capture change, and Studio control are complete.
- [x] Page-master handles write regions and margins. The existing authored
  `page` node now has an authority-bound Studio margin handle for the shorthand
  and each physical side. It targets the governing source property, requires
  exact source/plan/instance/fingerprint guards, emits one readable CST patch,
  recompiles before publication, and changes the exact SVG capture
  ([mutation](internal/paperd/semantic_layout_mutations.go),
  [Studio session](cmd/paper-studio/studio_edit.go),
  [service tests](internal/paperd/semantic_layout_mutations_test.go),
  [interaction/capture test](cmd/paper-studio/studio_edit_test.go)). Authored
  header/footer nodes now retain distinct header/body/footer regions through
  exact display capture, raster, PDF, and semantic projections, while region
  box handles require the governing page precondition
  ([production evidence](document/paper_page_regions_test.go),
  [mutation tests](internal/paperd/semantic_layout_mutations_test.go)). The item
  remains open for named/custom regions and page-master creation UX.
- [x] Dynamic text distinguishes binding from fixture editing. The internal
  review model classifies bound nodes and names the exact fixture path; the
  current Studio UI does not expose the editing-contract workflow
  ([review model](cmd/paper-studio/web/review-model.js),
  [tests](cmd/paper-studio/js_test/review_model_test.cjs)).
- [x] Repeated rows distinguish template, invocation, slot, and fixture
  targets. The internal model distinguishes repeated templates, component
  invocations, slot fills, and scenario fixture nodes
  ([review model](cmd/paper-studio/web/review-model.js),
  [tests](cmd/paper-studio/js_test/review_model_test.cjs)).
- [x] Page-break dragging opens a policy chooser. Addressable page-breaks now
  expose hard, keep-with-next, and avoid-orphan intent choices alongside the
  exact semantic destination; the chosen review intent is sent with the
  revision-bound move request ([Studio controls](cmd/paper-studio/web/studio.js),
  [request boundary](cmd/paper-studio/studio_edit.go),
  [browser probe](testdata/paper/studio-demo.paper)).
- [x] Optimistic feedback is visibly speculative. Pending semantic commits use
  a distinct speculative tone and copy, disable competing visual mutations,
  and only switch to committed/stale/rejected after the server response
  ([Studio controls](cmd/paper-studio/web/studio.js),
  [styles](cmd/paper-studio/web/studio.css),
  [model tests](cmd/paper-studio/js_test/review_model_test.cjs)).
- [x] Shared-edit blast radius is computed. The internal review model enumerates
  affected component invocations and shared style consumers; the current
  Studio UI does not expose the editing-contract workflow
  ([review model](cmd/paper-studio/web/review-model.js),
  [tests](cmd/paper-studio/js_test/review_model_test.cjs)).

### Working copy and review

- [x] Source and semantic operations share one bounded, mutex-serialized exact
  revision journal and one ordered undo chain. Every paperd candidate owns that
  journal; the existing semantic `Workspace.Apply` and source-editor API both
  publish immutable revision handles through the candidate's exact CAS head
  ([journal](internal/paperedit/journal.go),
  [workspace integration](internal/paperd/working_copy.go),
  [service tests](internal/paperd/working_copy_test.go)).
- [x] Consecutive manual text edits with the same explicit group coalesce into
  one Unicode-safe minimal source transition and undo unit while intermediate
  published revisions remain immutable
  ([journal tests](internal/paperedit/journal_test.go),
  [workspace tests](internal/paperd/working_copy_test.go)).
- [x] Undo and redo require the exact current head, remain failure-atomic on a
  stale opaque handle and digest, preserve deterministic redo ordering, and
  survive authenticated workspace snapshot recovery with fresh capabilities
  ([implementation](internal/paperd/working_copy.go),
  [persistence](internal/paperd/persistence.go),
  [normal/race tests](internal/paperd/working_copy_test.go)).
- [x] External reloads require the observed exact head; conflicts return the
  current and external opaque revisions without changing the current candidate
  or either history chain, while accepted reloads remain undoable
  ([workspace API](internal/paperd/working_copy.go),
  [service tests](internal/paperd/working_copy_test.go)).
- [x] Agent changesets bind one exact retained journal transition and candidate
  head to domain-separated source/semantic/manifest evidence; redacted patch
  identities; plan hashes, page movement, deterministic clean/overlay/crop and
  raster references; diagnostics; semantic roles; reading order; tag structure;
  and accessibility snapshots. Restricted projections contain no raw source,
  authored target, or artifact bytes; payload access requires a non-restricted
  edit capability. Selected evidence is regenerated and bound to the existing
  one-use, policy-gated candidate acceptance path across protocol round trips
  and persistence recovery
  ([implementation](internal/paperd/changeset_review.go),
  [normal/race/adversarial/bounds/recovery tests](internal/paperd/changeset_review_test.go),
  [visual evidence engine](internal/layoutengine/review_bundle.go)).
- [x] Screenshot/lasso annotations store page coordinates and transforms. The
  internal review API stores bounded page-space rectangles plus the six-value
  affine transform in a revision-independent sidecar and projects them back to
  authored IDs; the current Studio UI does not expose the review-notes workflow
  ([server](cmd/paper-studio/studio_review.go),
  [persistence test](cmd/paper-studio/studio_review_test.go)).
- [x] Reference PDFs/images support bounded, digest-verified artifacts and
  deterministic raster diffs. Studio stores reference artifacts beside the
  review sidecar, rasterizes PDF pages through the pinned Poppler verifier, and
  serves the reference and diff only for the exact current source and plan
  revisions ([server](cmd/paper-studio/studio_review.go),
  [image/PDF/artifact tests](cmd/paper-studio/studio_review_test.go)).
- [x] Comments survive formatting and ordinary movement. Comments are anchored
  to authored IDs rather than source offsets, persisted atomically beside the
  source, and reprojected as resolved/unresolved against the current AST after
  source changes ([server](cmd/paper-studio/studio_review.go),
  [persistence test](cmd/paper-studio/studio_review_test.go)).

### Stage 9 exit gate

- [x] Complete create-to-deliver workflow works without source degradation. The
  journaled template, binding, schema-field, scenario-matrix, fixture-value,
  preflight, verified-PDF export, and
  separate publish-status path are exercised as one exact-revision flow while
  preserving source comments and trivia
  ([integration test](cmd/paper-studio/studio_edit_test.go)).
- [x] Common visual edits produce minimal readable diffs. Studio mutations are
  bounded to readable CST patches and the focused visual-handle tests assert
  exact capture changes and patch counts
  ([tests](cmd/paper-studio/studio_edit_test.go)).
- [x] Every agent edit is reviewable and selectively acceptable. Changeset
  evidence remains bound to the retained candidate and exact acceptance path
  ([review implementation](internal/paperd/changeset_review.go),
  [tests](internal/paperd/changeset_review_test.go)).
- [x] Accessibility and scenario checks participate in review. Every exact
  review response carries the selected scenario and an accessibility status
  derived from independent inspection of the final serialized PDF; Review
  surfaces both status and bounded failures ([server](cmd/paper-studio/studio_review.go),
  [UI](cmd/paper-studio/web/studio.js), [scenario/review tests](cmd/paper-studio/studio_review_test.go)).

## 12. Stage 10 — Legacy engine deletion

### Stabilization proof

- [ ] Unified typed path has shipped through the stabilization window.
- [ ] Unified HTML path has shipped through the stabilization window.
- [ ] Fallback rate is within the agreed threshold.
- [ ] No unresolved blocker requires legacy layout.
- [ ] Compatibility, performance, and compliance error budgets pass.
- [ ] Rollback criteria have expired or been formally closed.

### Deletion

- [x] Remove production typed measurement from `layout/measure.go`.
- [x] Remove production typed measurement from `document/document_measure.go`.
- [x] Remove typed layout calculations from `document/document_render.go`.
- [x] Remove direct HTML layout from `document/html_render_session.go`.
- [x] Remove direct HTML box layout from `document/html_layout.go`.
- [x] Remove direct HTML flex layout from `document/html_flex.go`.
- [x] Remove direct HTML table layout from `document/html_tables.go`.
- [x] Retain public typed and HTML APIs as lowering adapters.
- [x] Remove automatic fallback switches and obsolete direct-renderer shadow
  code; remaining typed shadow helpers are planner contracts used by the
  unified lowering path.
- [x] Publish breaking-release migration notes where needed
  ([guide](docs/migration/legacy-engine-deletion.md)); the deletion release
  remains gated on the stabilization evidence above.

### Stage 10 exit gate

- [x] Repository search finds no automatic layout outside the planner
  (production search on 2026-07-17 found no `writeCompiledLegacy`, direct HTML
  renderer, legacy typed measurement, or legacy table/flex layout symbols).
- [x] Painter search finds no wrapping, measuring, or pagination calls; painters
  replay finalized plans and the HTML frame adapter handles only fragment
  boundaries.
- [x] All current public compatibility entry points still work
  (`go test ./...`, including typed/HTML adapter and template tests).
- [x] Fixture, benchmark, security, and compliance gates pass: `go test ./...`,
  `go vet ./...`, focused document/compliance race tests, generation benchmark
  budgets, Paper Engine calibration gate, and Paper Studio JavaScript tests.

## 13. Stage 11 — Ecosystem and production hardening

- [ ] Component gallery with real current-theme previews.
- [ ] Package versions and lockfile migrations.
- [ ] Team themes, policies, and approved resource catalogs.
- [ ] Shared scenario and visual-baseline libraries.
- [ ] Semantic collaboration and conflict resolution.
- [ ] Web Studio backed by isolated `paperd`.
- [ ] Compliance profiles and organization governance.
- [ ] Reproducibility manifest in generated artifacts where appropriate.
- [ ] Signed/anchored audit export.
- [ ] Security review and external fuzzing campaign.
- [ ] Protected-content, publish, export, and signing authorization tests.
- [ ] Operational quotas, monitoring, cancellation, and incident playbooks.

## 14. Cross-cutting engineering checklists

### Text and internationalization

- [ ] Current text behavior is preserved for initial cutover.
- [x] The current streaming ASCII wrapping fast path is benchmarked separately
  from UTF-8 fallback behavior
  ([benchmark](document/performance_internal_test.go)).
- [x] Define a validated canonical shaping result containing glyph IDs, fixed
  advances/offsets, UTF-8 clusters, visual direction, exact font runs, and
  fallback provenance ([contract](internal/layoutengine/shaping.go),
  [tests](internal/layoutengine/shaping_test.go)).
- [x] Implement an initial deterministic shaped-text line breaker that consumes
  exact glyph clusters, never splits UTF-8 clusters, preserves RTL visual glyph
  order, handles explicit newlines and space-preferred wrapping, and replays
  stored lines without reshaping
  ([breaker](internal/layoutengine/shaped_line.go),
  [tests](internal/layoutengine/shaped_line_test.go)); Unicode line-break rules,
  hyphenation, and contextual line-edge reshaping remain pending.
- [x] Shaping and shaped-line-layout caches are separate, concurrency-safe,
  byte/entry bounded, width-sensitive, FIFO-evicted, and return detached values
  ([shaping tests](internal/layoutengine/shaping_test.go),
  [line-cache tests](internal/layoutengine/shaped_line_cache_test.go)).
- [ ] CJK, RTL, mixed bidi, combining marks, emoji, and unbreakable tokens are tested.
- [x] Missing glyphs, unsupported built-in shaping, and deterministic fallback
  selection are diagnosable ([tests](internal/layoutengine/shaping_test.go)).
- [ ] Locale-dependent expansion is covered by scenarios.

### Tables

- [x] Occupancy normalization cannot loop on invalid spans
  ([tests](internal/layoutengine/table_test.go)).
- [x] Track resolution never produces negative widths
  ([tests](internal/layoutengine/table_test.go)).
- [x] Span deficits are deterministic
  ([tests](internal/layoutengine/table_test.go)).
- [x] Rowspan pagination groups are explicit
  ([planner](internal/layoutengine/table.go)).
- [x] Oversized rows emit once with a diagnostic
  ([tests](internal/layoutengine/table_test.go)).
- [ ] Repeated headers are planned once per width/profile where possible.
- [x] The initial premeasured table kernel enforces hard bounded temporary state,
  allocation-safe occupancy limits, cancellation, work, and page limits
  ([planner](internal/layoutengine/table.go),
  [tests](internal/layoutengine/table_test.go)); production page-windowed
  streaming remains pending.
- [x] Fixed-track 10,000-row streaming-kernel cost and 1,000-row intrinsic
  content-sized premeasurement-consumption cost are visible as separate
  benchmarks
  ([benchmarks](internal/layoutengine/paper_engine_benchmark_test.go)).
- [ ] Tagged table semantics match visual structure.

### Pagination

- [x] Paragraph break tokens and typed page transitions strictly advance or
  fail under cancellation/resource limits
  ([kernel](internal/layoutengine/paragraph.go),
  [typed flow](document/paper.go),
  [tests](document/typed_pagination_policy_test.go)).
- [x] Preferred typed keeps are relaxed only when their measured group/chain
  exceeds an empty body, with `KEEP_TOO_LARGE` evidence and continued forward
  progress ([planner](document/paper.go),
  [tests](document/typed_pagination_policy_test.go)).
- [ ] Strict keeps fail deterministically.
- [x] Initial fixed page-master header/footer regions cannot consume body space
  silently because enabled vertical bands are validated as ordered and
  non-overlapping ([tests](internal/layoutengine/page_master_test.go)).
- [x] Initial repeated-region streams larger than their region are emitted once
  with overflow evidence and planner advance
  ([tests](internal/layoutengine/page_master_test.go)).
- [x] Typed total-page counter correction is capped at eight complete planning
  passes, consumes the same cumulative request budget, caches page/total shell
  subplans, and fails atomically when it cannot converge
  ([correction](document/typed_page_template.go),
  [tests](document/typed_page_template_test.go),
  [table-shell tests](document/typed_table_plan_test.go)).
- [x] Initial master selection is a pure first/parity/default function and cannot
  depend circularly on content placement
  ([planner](internal/layoutengine/page_master.go)).
- [ ] Incremental suffix reuse equals a clean full rebuild.

### Preview and capture

- [x] The initial direct-display-list normative review artifact is lossless PNG,
  with deterministic encoding and a pinned fixture digest
  ([renderer](internal/layoutengine/display_raster.go),
  [tests](internal/layoutengine/display_raster_test.go)).
- [x] The versioned canonical raster manifest pins color space, opaque alpha and
  background, bounded DPI, antialiasing, crop/coordinate rounding, image
  resampling, PNG compression, renderer version, exact pixel transform, plan
  identity, resources, and artifact digest
  ([manifest](internal/layoutengine/display_raster.go),
  [tests](internal/layoutengine/display_raster_test.go)).
- [x] Every review image is content-addressed and linked by plan hash through
  the canonical bundle manifest to base/candidate source, scenario, policy,
  page-profile, engine, and resource identities
  ([manifest](internal/layoutengine/review_bundle.go),
  [tests](internal/layoutengine/review_bundle_test.go)).
- [x] Cross-page contact sheets and stitched strips publish one exact page-local
  capture-to-artifact transform per represented page, including overflow-aware
  capture bounds
  ([artifacts](internal/layoutengine/visual_artifacts.go),
  [tests](internal/layoutengine/visual_artifacts_test.go)).
- [x] Clean and overlay review layers are separate immutable PNG artifacts
  ([bundle](internal/layoutengine/review_bundle.go),
  [tests](internal/layoutengine/review_bundle_test.go)).
- [x] Every retained revision, candidate, open, plan, and scenario cache entry
  is partitioned by project, policy revision, and disclosure domain; mismatched
  partitions are rejected during lookup and persistence recovery
  ([partition](internal/paperd/cache_partition.go),
  [state](internal/paperd/workspace.go),
  [tests](internal/paperd/persistence_test.go)).
- [x] Production capture is privileged by a non-interchangeable one-use
  `production_capture` capability and approval bound to the exact plan,
  resource hashes, raster profile/limits, structural expectations, tolerance,
  and compliance evidence; denial, consumer failure, verification failure, and
  success append bounded hash-linked audit outcomes without retaining raw
  production bytes ([service](internal/paperd/final_pdf_verify.go),
  [tests](internal/paperd/final_pdf_verify_test.go)).
- [x] Final verification combines independent serialized-PDF raster comparison
  with extracted page/text/link/destination/tagging/metadata and compliance
  structure checks in one canonical plan-bound report
  ([verifier](internal/pdfverify/verify.go),
  [tests](internal/pdfverify/verify_test.go)).

### Accessibility and compliance

- [ ] Reading order is independent from z-order.
- [ ] Headings, paragraphs, lists, tables, figures, links, and forms have semantics.
- [ ] Alt text and decorative status are explicit.
- [ ] Table headers, scopes, and spans are inspectable.
- [ ] Form labels and keyboard order are inspectable.
- [ ] PDF/UA and PDF/A fixtures run through external validators.
- [x] Raster verification is recorded separately from structural text, links,
  destinations, tags, accessibility markers, PDF/A/PDF/UA identifiers, and
  exact external validator evidence; pixels alone cannot satisfy a structural
  or compliance expectation ([report](internal/pdfverify/verify.go),
  [failure tests](internal/pdfverify/verify_test.go)).

### Security and privacy

- [x] Expression VM and compiler control-flow evaluation have no I/O or ambient
  authority ([capability test](internal/paperexpr/capability_test.go)).
- [ ] Source, data, OCR, filenames, diagnostics, and imports are untrusted.
- [ ] Asset/import roots reject traversal and unsafe symlinks.
- [ ] SVG, image, font, and archive limits are enforced.
- [x] Planned display-resource resolution exposes compatibility-preserving
  context APIs for exact SVG preview and production PDF preflight, checks
  cancellation through image hashing/header reads/decoding/base64 work and
  core-font lookup, shares cumulative encoded/decoded image budgets across a
  request, and rejects cancellation or aggregate exhaustion before mutating
  the target document
  ([preview](internal/layoutengine/display_svg.go),
  [PDF preflight](document/layout_display_painter.go),
  [tests](internal/layoutengine/display_svg_test.go),
  [document tests](document/typed_layout_plan_test.go)). Archive and ambient
  font/image catalogs remain outside this completed planned-resource slice.
- [ ] Prompt injection cannot cross the trusted-policy boundary.
- [ ] Protected values do not appear in restricted context or capture metadata.
- [ ] Transitive edit effects are authorized.
- [ ] Export, publish, attachment, and signing remain separate capabilities.
- [ ] Denied actions and disclosure attempts are audited.

### Performance

- [x] Planner-only benchmark exists
  ([benchmark](document/paper_engine_benchmark_test.go)).
- [x] Painter-only benchmark exists and replays one retained immutable plan
  without planning or PDF serialization
  ([benchmark](document/paper_engine_benchmark_test.go)).
- [x] End-to-end typed, reusable compiled-HTML, and `.paper` benchmarks exist
  with deterministic fixtures and output
  ([benchmarks](document/paper_engine_benchmark_test.go),
  [fixture determinism test](document/paper_engine_benchmark_test.go)).
- [x] A 200-node local semantic text-edit benchmark measures exact revision
  validation, lossless parsing, target lookup, minimal patch planning,
  candidate validation, and detached evidence
  ([benchmark](internal/paperedit/edit_benchmark_test.go)).
- [x] Exact retained-plan crop and deterministic contact-sheet benchmarks exist
  as separate visual-tool cohorts
  ([benchmarks](internal/layoutengine/visual_artifacts_benchmark_test.go)).
- [x] Large/wide/10k-row table benchmarks exist, including a bounded linear
  row-grouping kernel path that plans 10,000 premeasured rows without raising
  work limits
  ([typed benchmarks](document/paper_engine_benchmark_test.go),
  [kernel benchmark and invariant](internal/layoutengine/paper_engine_benchmark_test.go),
  [linear grouping](internal/layoutengine/table.go)).
- [x] A fixed 16-worker concurrent immutable-plan benchmark exists with a
  ten-sample pinned-host baseline
  ([benchmark](document/paper_engine_benchmark_test.go),
  [baseline](docs/performance/baselines/paper-engine-stage0-apple-m2.txt)).
- [ ] `benchstat` evidence is attached for hot-path PRs.
- [ ] Allocation changes are explained with profiles.
- [x] No portable or single-sample timing multiplier is used as an acceptance
  gate; the automatic report gate requires at least ten samples and exact
  named host/toolchain matching before applying calibrated upper-median timing
  and worst-sample allocation ceilings, while release comparisons still use
  `benchstat`
  ([workflow](docs/performance/paper-engine-benchmarks.md),
  [report gate](internal/perfgate/report.go)).

## 15. Per-PR checklist

### Scope and architecture

- [ ] PR names the stage and checklist items it advances.
- [ ] Change respects the one-engine boundary.
- [ ] Public API addition is justified and stable enough to expose.
- [ ] Unrelated user changes are untouched.
- [ ] Compatibility behavior is documented.
- [ ] Failure behavior and diagnostic codes are defined.

### Correctness

- [ ] Pure unit tests cover the changed algorithm.
- [ ] Planner invariants remain true.
- [ ] Incremental result equals clean rebuild where relevant.
- [ ] Source/AST/plan/display/raster/semantic goldens are updated intentionally.
- [ ] New test data includes a boundary or adversarial case.
- [ ] Cancellation and work limits are tested where relevant.

### Human editing

- [ ] Source changes remain readable.
- [ ] Semantic operation produces a minimal CST patch.
- [ ] Comments and trivia remain attached correctly.
- [ ] Visual mutation edits governing properties, not computed boxes.
- [ ] Stale/speculative visual states cannot look authoritative.

### Agent tooling

- [ ] Tool operation is typed and revision-safe.
- [ ] Preconditions reject stale or ambiguous targets.
- [ ] Permission checks include transitive effects.
- [ ] Response is token-bounded and capability-filtered.
- [ ] Candidate evidence includes diagnostic and invalidation deltas.
- [ ] No raw sensitive hash or value leaks through handles or metadata.

### Verification

- [ ] `gofmt`/formatters pass for changed code and sources.
- [ ] Focused tests pass.
- [ ] Relevant package tests pass.
- [ ] Race tests pass when concurrency changes.
- [ ] Fuzz/property tests cover parser/planner/security changes.
- [ ] `git diff --check` passes.
- [ ] Benchmark evidence is attached when the hot path changes.
- [ ] Visual changes include deterministic before/after evidence.

## 16. Agent-generated change review checklist

- [ ] Agent goal is recorded.
- [ ] Capability grant is recorded.
- [ ] Base source, semantic, scenario, and policy revisions are recorded.
- [ ] Candidate revision is immutable.
- [ ] Requested operations are typed and local.
- [ ] Transitive affected nodes/pages/profiles are shown.
- [ ] Protected scopes remain unchanged or explicitly authorized.
- [ ] Required slot scenarios pass.
- [ ] Source diff is minimal and readable.
- [ ] Semantic diff matches the stated intent.
- [ ] Plan diff has no unexplained movement or page-count change.
- [ ] Before/after crops cover every changed region.
- [ ] Accessibility diff is reviewed.
- [ ] Typical and adversarial scenarios pass.
- [ ] Production disclosure was not used without authority.
- [ ] Approval binds the exact candidate and evidence.
- [ ] Export/publish/sign remains a separate authorized action.

## 17. Release checklist

### Compatibility

- [ ] Typed compatibility corpus passes.
- [ ] HTML compatibility corpus passes.
- [ ] `.paper` language/version migration tests pass.
- [ ] Manual drawing before/after HTML fragment tests pass.
- [ ] Public API compatibility report is reviewed.

### Quality

- [ ] Full unit and integration suites pass.
- [ ] Race suite passes.
- [ ] Fuzz and property suites meet the required duration.
- [ ] Planner termination/work-limit suite passes.
- [ ] Incremental-versus-full differential suite passes.
- [ ] Visual regression suite passes with reviewed tolerances.
- [ ] Extracted text, reading order, links, forms, tags, and attachments pass.
- [ ] External PDF/A and PDF/UA validators pass.

### Performance

- [ ] Release benchmarks are compared with the named baseline.
- [ ] Regressions are within calibrated budgets or explicitly approved.
- [ ] Memory and allocation profiles are reviewed.
- [ ] Concurrent throughput is reviewed.
- [ ] Interaction latency fixtures meet calibrated budgets.

### Security and reproducibility

- [ ] Dependency and package lock is complete.
- [ ] Asset/font hashes are present.
- [ ] Resource limits and cancellation are enabled.
- [ ] Privacy/capability test suite passes.
- [ ] Audit root is signed or anchored.
- [ ] Generated fixtures reproduce from clean environments.
- [ ] Release notes identify grammar, planner, resource, and validator versions.

### Publication

- [ ] Migration guide is complete.
- [ ] Known limitations are documented.
- [ ] Rollback procedure is tested.
- [ ] Legacy fallback policy is documented for stabilization releases.
- [ ] Deletion release confirms the fallback window is closed.

## 18. Final definition-of-done checklist

- [ ] One planner owns all automatic layout.
- [ ] Painter performs no measurement, wrapping, or pagination.
- [ ] `.paper` remains human-readable after source, visual, and agent edits.
- [ ] Humans can trace any pixel to source, data, style, semantics, and cause.
- [ ] Agents can create and modify documents without whole-file rewriting.
- [ ] Screenshots are deterministic evidence paired with semantic metadata.
- [ ] Every break and overflow is explainable.
- [ ] Preview and final PDF share exact display-list geometry.
- [ ] Raster differences stay within explicit pinned tolerances.
- [ ] Accessibility and compliance are visible during authoring.
- [ ] Normal edits update affected pages interactively.
- [ ] Typed and HTML APIs work as compatibility adapters.
- [ ] Legacy automatic layout engines are deleted.
- [ ] Performance is faster or materially more efficient than the old HTML path.
- [ ] Security, privacy, reproducibility, and audit requirements pass.
