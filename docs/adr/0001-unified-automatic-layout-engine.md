# ADR 0001: One Automatic Layout Engine

- Status: Accepted for staged implementation
- Date: 2026-07-16
- Decision owners: GoPDFKit maintainers
- Execution plan: [PAPER_ENGINE_PLAN.md](../../PAPER_ENGINE_PLAN.md)
- Checklists: [PAPER_ENGINE_CHECKLISTS.md](../../PAPER_ENGINE_CHECKLISTS.md)

## Context

GoPDFKit currently has two automatic layout implementations:

1. Typed `layout.LayoutDocument` measurement in `layout/measure.go` followed by
   renderer-local positioning and pagination in `document/document_render.go`.
2. HTML/CSS token, box, flex, table, pagination, and direct drawing logic in the
   `document/html_*.go` implementation.

Both paths calculate layout while owning different state and rules. Typed
measurement does not produce exact lines, fragments, coordinates, continuation
state, or paint commands, so rendering must calculate important geometry again.
The HTML implementation independently owns much of the same geometry and
pagination behavior. Adding a third engine for a new document language would
make parity, performance, diagnostics, accessibility, and visual tooling harder
to maintain.

The low-level FPDF-style drawing API serves a different purpose: it is an
explicit manual drawing path and does not need to be removed.

## Decision

GoPDFKit will implement one automatic layout engine with multiple input
frontends:

```text
LayoutDocument adapter ─┐
HTML/CSS adapter ────────┼─> canonical tree ─> planner ─> LayoutPlan
.paper compiler ─────────┘                          (fragments + display commands)
                                                        ├─> PDF painter
                                                        └─> preview renderer
```

The following boundaries are mandatory:

- Frontends may parse, validate, resolve syntax-specific cascade/inheritance,
  and lower content into canonical primitives.
- Only the planner may measure, wrap, position, fragment, or paginate automatic
  content.
- `LayoutPlan` owns final positioned fragments, glyph runs, semantics,
  diagnostics, break decisions, and display commands.
- The PDF painter and preview renderer consume the plan without recalculating
  geometry.
- Planning uses an immutable resource catalog independent of the mutable output
  `document.Document`.
- Internal geometry uses deterministic fixed-point PDF points.
- Typed and HTML public entry points remain compatibility adapters.
- The manual drawing API remains supported.

## Coordinate and arithmetic contract

Engine geometry uses signed `int64` values at 1/1024 PDF point. The coordinate
origin is the page's top-left corner; x increases rightward and y increases
downward. Rectangle widths and heights are non-negative, and containment uses
exclusive right and bottom edges.

Conversions from points round to the nearest fixed unit, with exact half units
rounded away from zero. Non-finite and out-of-range values are errors. Addition,
subtraction, negation, multiplication, division, far-edge calculation, and
insetting are checked operations: overflow, division by zero, and negative
extents must not wrap, saturate, or be silently clamped. Integer division
truncates toward zero; algorithms that distribute a remainder must do so
explicitly and deterministically.

Legacy typed-layout lengths are expressed in the unit configured on
`document.Document`, while `TextStyle.FontSize` is already expressed in points.
Adapters must therefore convert unit-bearing values such as margins, padding,
borders, line heights, tracks, image and QR sizes, and page-region heights by
the document conversion ratio without applying that conversion to font sizes.
This mixed-unit rule requires parity fixtures for points, millimeters,
centimeters, and inches before typed-layout cutover.

## Painter and resource contract

Display commands retain fixed-point geometry. The PDF painter converts the
top-left coordinate system to PDF's bottom-left system and serializes every
fixed coordinate as its exact canonical decimal value, with trailing fractional
zeros removed. At the selected scale this requires at most ten fractional
digits. It must not introduce the legacy operation-dependent two- or
five-decimal coordinate rounding. Raster output maps the same fixed coordinates
to pixels only at the requested DPI; that mapping and its rounding policy are
versioned inputs to `RenderID`.

The painter may allocate pages, register preflighted resources, and serialize
commands. It may not call `MultiCell`, `Write`, HTML rendering, automatic page
breaking, or any other operation that measures, wraps, repositions, fragments,
or paginates. Planned page creation must not execute opaque document callbacks.

Font metrics, image and SVG dimensions, content hashes, and resource identities
come from an immutable catalog created before planning. Measurement and
preflight must not select fonts on, register resources with, or otherwise mutate
the output `document.Document`. Every resource referenced by a plan is
validated before painting begins.

## Identity and observability

The engine distinguishes:

- Source node identity.
- Data/component-expanded instance identity.
- Plan-local fragment identity.
- Exact source, semantic-template, scenario, policy, plan, and render revisions.

Every planned fragment can be traced to source, generated instance, style
provenance, semantic role, page region, and a concise break or overflow reason.
Detailed derivation traces are optional and bounded.

## Migration order

1. Characterize current typed and HTML behavior and performance.
2. Establish private identity, geometry, diagnostic, and plan contracts.
3. Build the planner kernel, exact painter, and internal Plan Viewer.
4. Migrate and stabilize typed layout.
5. Migrate HTML feature cohorts and make the unified path the default.
6. Stabilize the human-readable `.paper` language on the proven engine.
7. Add headless agent tooling and Paper Studio.
8. Delete legacy automatic layout implementations after a release window.

The canonical IR remains private until both existing engines have exercised it.

## Compatibility constraints

- `Document.WriteDocument` remains public while explicitly classified
  compatibility behavior lowers through the planner.
- HTML fragments migrate as complete units; legacy-rendered islands cannot be
  embedded inside unified automatic flow.
- `HTML.Write` requires an explicit start-frame and end-frame contract because
  it may begin at a current cursor after manual drawing.
- Metadata, attachments, encryption, output policy, and signing remain document
  envelope concerns rather than layout nodes.
- Visible forms, signature placeholders, annotations, and semantic associations
  may appear in a plan because they occupy page space.

The temporary legacy fallback described by the original migration design has
been deleted. Capability validation happens deterministically before the output
document is mutated. Unsupported input fails atomically; planner or painter
failure never retries through another renderer, and no public or private legacy
switch remains.

## Unresolved characterization gates

These questions do not block the private foundation slice, but each blocks the
corresponding production cutover:

- `Document.WriteDocument` can currently begin on an existing page at the
  current cursor, change margins, and interact with already drawn content. Its
  entry and exit state must be characterized before deciding whether typed
  layout receives a `StartFrame` contract like HTML or whether this behavior is
  explicitly deprecated.
- User-provided `SetHeaderFunc`, `SetFooterFunc`, `SetFooterFuncLpi`, and
  `SetAcceptPageBreakFunc` callbacks can perform arbitrary drawing and state
  changes during page creation. They cannot be represented in an exact plan or
  preview as opaque closures. Characterization must precede a decision to use
  rejection or replacement with declarative planned page regions.
- Current typed and HTML paths do not always choose the same page profile when
  automatic and explicit breaks interact with custom size, orientation, or
  rotation. Entry, continuation, and exit behavior must be captured before the
  unified planner owns those transitions.

Until these gates are resolved, the unified engine must not be selected for an
affected production call.

## Consequences

### Benefits

- Measurement and painting cannot drift.
- Typed, HTML, and future `.paper` input share pagination and table behavior.
- Exact preview, hit testing, node crops, visual diffs, and break explanations
  become products of the production engine rather than approximations.
- Agent edits can target stable semantic nodes and validate exact affected
  pages.
- Performance work benefits every automatic frontend.
- Legacy layout code can eventually be deleted.

### Costs

- The migration is multi-release and requires temporary shadow/fallback paths.
- Existing implicit HTML cursor behavior must be characterized precisely.
- A complete plan has storage proportional to output; large documents require a
  segmented `PlanStore` rather than pretending plans are free.
- Deterministic resources and fixed-point coordinates require explicit adapter
  work around current float and `Document`-owned measurement code.
- Exact source-to-pixel provenance adds data structures that must be compact in
  production and richer only in debug/Studio modes.

## Rejected alternatives

### Add `.paper` as a third direct renderer

Rejected because it permanently triples pagination, tables, text, semantics,
testing, and visual-debugging work.

### Keep typed and HTML engines and share only geometry helpers

Rejected because helpers do not eliminate duplicated stateful measurement,
fragmentation, and painting decisions.

### Make HTML/CSS the canonical internal format

Rejected because CSS cascade and browser-oriented semantics would leak into the
planner, preserve selector costs, and constrain page-native features.

### Use a global constraint solver for all layout

Rejected because normal flow, tables, and pagination have faster specialized
algorithms. Local canvas anchor constraints are sufficient for precise forms,
labels, and overlays.

## Initial implementation boundary

The first implementation slice creates only private foundation contracts:

- Fixed-point geometry.
- Identity and source-span types.
- Structured diagnostics.
- Minimal immutable-by-convention `LayoutPlan` and deterministic validation.
- Focused tests.

These contracts are internal and may change while both legacy engines exercise
them. This slice adds no production call site, changes no generated PDF, and
exposes no public API.
