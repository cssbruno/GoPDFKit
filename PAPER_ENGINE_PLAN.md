# Paper: Unified Layout Engine, Studio, and Agentic Authoring Plan

- Status: implemented architecture; stabilization and named external acceptance gates remain open
- Scope: replace both automatic layout engines while preserving the low-level PDF API
- Working names: `.paper` source, Paper engine, Paper Studio, `paperd` service

Execution companion: [PAPER_ENGINE_CHECKLISTS.md](PAPER_ENGINE_CHECKLISTS.md)

## 1. Executive decision

PaperRune should have one automatic layout engine and three first-class ways to
use it:

1. Human-readable `.paper` source.
2. A semantic visual editor.
3. Typed, transactional agent tools.

The current typed and HTML automatic layout implementations should converge on
one canonical node tree, one planner, one `LayoutPlan`, and one display list.
The FPDF-style drawing API remains supported as a manual escape hatch.

```text
LayoutDocument adapter ─┐
HTML/CSS adapter ────────┼─> canonical tree ─> planner ─> LayoutPlan
.paper compiler ─────────┘                          (fragments + display commands)
                                                        ├─> PDF painter
                                                        └─> preview renderer
```

This must not become a third engine. Automatic measurement, wrapping,
positioning, fragmentation, and pagination belong only to the unified planner.

The product thesis is:

> Every rendered pixel can be traced to a source node, bound data, resolved
> style, semantic role, page fragment, and layout decision.

The AI thesis is:

> Agents edit the semantic tree through typed transactions. Images are visual
> evidence, not the source of structural truth.

## 2. Why this replacement is necessary

The typed path currently estimates in [layout/measure.go](layout/measure.go)
and then calculates layout again while drawing in
[document/document_render.go](document/document_render.go). Tables also have
renderer-local measurement and pagination. This permits measurement and drawing
to disagree.

The HTML path is a second automatic layout engine. It performs direct layout
and drawing through:

- [document/html_render_session.go](document/html_render_session.go)
- [document/html_layout.go](document/html_layout.go)
- [document/html_flex.go](document/html_flex.go)
- [document/html_tables.go](document/html_tables.go)

HTML table handling alone owns column sizing, row measurement, spans, repeated
headers, pagination, and painting. Maintaining parity between two complete
engines will become harder as either gains features.

The repository already states that `layout` owns renderer-independent models
and geometry, while `document` owns PDF construction. The new design completes
that boundary instead of adding another rendering path.

## 3. Goals

### 3.1 Product goals

- Human-readable, Git-friendly document source.
- Fast warm rendering and fast local preview.
- Exact shared display-list geometry for preview and production, with raster
  parity measured under specified renderer, color, alpha, and pixel tolerances.
- Safe visual editing without degrading source readability.
- AI generation and editing through stable semantic operations.
- First-class pagination, page masters, tables, forms, signatures, tagging,
  attachments, and PDF compliance.
- Deterministic and explainable output.
- Compatibility frontends for current typed layouts and supported HTML/CSS.
- A component ecosystem with typed props, typed slots, accessibility contracts,
  scenarios, and performance expectations.

### 3.2 Engineering goals

- One planner for all automatic layout.
- The painter performs no layout.
- Immutable compiled templates safe for concurrent reuse.
- Single-render mutable sessions with explicit limits and cancellation.
- Near-linear behavior for normal flows and large tables.
- Fixed and bounded reference-resolution behavior.
- Source, AST, plan, display-list, raster, and PDF-level testing.
- Inspectable failure modes instead of blank pages, loops, or silent clipping.

## 4. Non-goals

- Reimplementing a browser or the entire CSS cascade.
- Becoming a general-purpose programming language.
- Arbitrary code execution, network access, or ambient filesystem access.
- A Word clone or unconstrained WYSIWYG editor.
- Free x/y positioning in normal document flow.
- Using browser HTML layout as the Studio preview.
- Making screenshots the agent's only observation channel.
- Supporting every advanced typography or publishing feature in the first
  engine cutover.
- Bidirectional editing of arbitrary existing PDFs.

## 5. Architectural rules

1. **One engine:** frontends parse and lower; only the planner lays out.
2. **Plan before paint:** all resources and pages are preflighted before
   `document.Document` is mutated.
3. **Exact display list:** PDF and preview consume the same positioned glyphs,
   shapes, images, clips, links, and semantic associations.
4. **Stable semantic identity:** editable source nodes, expanded instances, and
   rendered fragments use different identity types.
5. **Bounded computation:** expressions, node expansion, pages, rows, glyphs,
   images, traces, and layout work have explicit limits.
6. **Local effects:** cascade, inheritance, and provenance are resolved before
   planning; constraint-dependent lengths remain typed values for the planner.
   There is no selector matching in the planner.
7. **Determinism:** fonts, assets, locale, timezone, input data, grammar, and
   engine versions are explicit inputs.
8. **Causal diagnostics:** a layout error includes source, instance, page,
   rectangle, evidence, and typed remedies.
9. **Semantic editing:** visual and agent edits generate AST operations, not
   fuzzy text replacement or unreviewable source rewrites.
10. **Compatibility at the edges:** existing public HTML and typed APIs remain
    adapters during migration.

Add this invariant to `ARCHITECTURE.md` when implementation starts:

> Automatic layout exists only in the unified planner. Frontends may parse and
> resolve syntax-specific styles, and `document` may paint and serialize, but
> neither a frontend nor a painter may measure, wrap, position, fragment, or
> paginate content.

## 6. Identity, revision, and provenance model

Use distinct identities:

```text
SourceNodeID   @invoice-lines
InstanceID     @invoice-lines/row[key="SKU-187"]
FragmentID     @invoice-lines/row[key="SKU-187"]/page=3/fragment=1
SourceRevisionID    exact project snapshot, including comments and formatting
SemanticTemplateID canonical typed AST + resolved imports + package lock
ScenarioRevisionID fixture/scenario data revision
PolicyRevisionID   capabilities, protected scopes, and disclosure policy
PlanID              semantic template + canonical data digest + resources + locale
RenderID            plan + renderer + color/DPI/crop settings + disclosure domain
```

`PlanID` also pins timezone, Unicode/CLDR/hyphenation versions, compatibility
flags, page profile, fonts/assets, and planner version. Public tool handles are
opaque session-scoped capabilities, not raw hashes; raw hashes of low-entropy
sensitive datasets must never become identifiers exposed to clients.

### 6.1 Source node rules

- Human-authored IDs are readable `@slug` values.
- IDs are namespace-scoped, case-sensitive, and unique.
- Long-lived comments, automations, slots, and agent operations target explicit
  source IDs.
- Anonymous nodes use revision-scoped structural keys plus fingerprints for
  ordinary same-revision edits. Insert a readable source ID only when creating
  a durable cross-revision handle such as a comment, automation, protected
  scope, agent contract, or explicit user reference.
- Renaming an ID is an atomic refactor that rewrites references.
- Inline spans generally belong to the nearest structural node and do not need
  persistent IDs.

### 6.2 Expanded instances

- Component and loop expansion creates instance paths without polluting source.
- Repeated data must declare a stable key when instances need durable addressing.
- An agent cannot mutate an `InstanceID` as if it were a source definition. It
  must edit the row template, source data scenario, or an explicitly supported
  per-instance override.

### 6.3 Fragment identity

- Fragment IDs are plan-local and never persistent edit targets.
- A fragment records its source node, expanded instance, page, region,
  continuation state, and fragment index.
- Hit testing maps pixels to fragments, then to instances and source nodes.

### 6.4 Provenance on every planned fragment

- Source file and span.
- Component definition and invocation path.
- Bound data paths and scenario.
- Resolved style and token origin.
- Semantic role and reading-order position.
- Layout inputs, selected break reason, and diagnostics.

Fragments store compact interned provenance IDs rather than duplicating full
paths and evidence. Shared plan tables resolve those IDs. Production mode may
retain only source/semantic IDs and concise break codes; detailed rejected
candidates and derivations belong to bounded debug/Studio plans.

## 7. Human-readable `.paper` language

`.paper` is an indentation-based, typed document language. It may resemble a
configuration format, but it is not YAML and not generic serialization. The
grammar knows document nodes, typed properties, units, expressions, children,
roles, components, slots, and source spans.

```text
document Invoice(invoice: Invoice):
  theme: corporate

  page A4:
    margin: 18mm

    header repeat:
      row:
        image: asset(company-logo)
          width: 32mm
        text: ${invoice.company.name}
          align: right

    footer repeat:
      text: Page {page} of {pages}
        align: center
        style: page-number

  body:
    heading 1 @title: Invoice ${invoice.number}

    grid @summary:
      columns: 2fr 1fr
      gap: 8mm

      column @customer:
        label: Customer
        value: ${invoice.customer.name}

      column @due-date:
        label: Due date
        value: ${invoice.due_date | date}

    table @lines:
      source: invoice.lines as line key line.sku
      columns: 1fr 16mm 28mm
      repeat-header: true
      split: rows
      header: Item | Qty | Price
      row: ${line.name} | ${line.qty} | ${line.total | money}

    slot @additional-content:
      accepts: paragraph | list | note
      pages: <= 1
      overflow: continue

    keep:
      signatures: Customer | Issuer
```

### 7.1 Syntax principles

- The project manifest pins a grammar version; migrations are explicit and
  formatter-assisted.
- One canonical syntax for each core feature.
- Indentation expresses nesting; property values remain line-oriented.
- `//` comments do not conflict with headings or data expressions.
- `${...}` expressions are pure, typed, and bounded.
- Rich text supports a deliberately small Markdown-like inline subset.
- Units are explicit and type-checked.
- Source supports imports, themes, components, schemas, scenarios, and slots.
- The formatter is deterministic and idempotent.
- The lossless CST preserves comments, whitespace, property order, and original
  string forms during normal semantic edits.
- Explicit `paper fmt` canonicalizes a complete file; routine visual edits
  generate minimal token-range patches.

Trivia ownership is part of the language contract:

- Leading comments move with their node.
- Trailing comments remain attached to their property.
- Blank-line ownership is deterministic.
- Unknown syntax from a newer grammar is preserved as opaque CST and cannot be
  visually rewritten by an older editor.
- Duplicate or invalid properties are diagnosed and never silently normalized.
- Multiline-text indentation and rewrite rules are specified and golden-tested.

A semantic operation is deterministic relative to an exact `SourceRevisionID`
and produces a deterministic minimal CST patch. It does not format unrelated
source. Whole-file canonical formatting is a separate `paper fmt` operation.

### 7.2 Types and schemas

- Templates declare typed parameters.
- Schemas may originate from an inline declaration, a JSON Schema adapter, or a
  Go-provided descriptor.
- The compiler validates binding paths and filters before rendering.
- Hover and agent inspection can show type, current scenario value, redaction
  state, and schema origin.
- Nullability and missing-data policies are explicit.

### 7.3 Expressions

Allowed operations should include:

- Field access and indexing.
- Pure arithmetic and comparison.
- `if`, `match`, bounded `for`, and collection transforms.
- Formatting functions for dates, numbers, currency, text, and locale.
- Pure user functions compiled to bytecode.

Disallowed operations include:

- File, network, process, reflection, or environment access.
- Ambient time or randomness.
- Runtime module loading.
- Post-layout queries that can change document structure.
- Unbounded recursion or loops.

### 7.4 Themes and styles

- Themes expose named semantic roles and tokens.
- Style cascade, inheritance, and token provenance are resolved before
  planning. Percentages, intrinsic sizes, `auto`, and containing-block-relative
  values remain typed until the planner has concrete constraints.
- There are no selectors or specificity rules in `.paper`.
- A computed property reports whether it came from a component default, theme,
  lexical scope, or local override.
- Direct manipulation should prefer writing token references over magic values.

### 7.5 Components

Components define:

- Typed props.
- Typed slots and allowed children.
- Default semantic roles.
- Theme tokens.
- Pagination behavior.
- Accessibility requirements.
- Empty, typical, and extreme scenarios.
- Performance expectations.

High-level components lower into primitive layout nodes before planning. The
planner does not know concepts such as invoice totals or verification cards.

### 7.6 Slots for controlled generation

Slots are content and layout contracts:

```text
slot @executive-summary:
  accepts: heading | paragraph | list | callout
  cardinality: 1..12
  characters: <= 4000
  height: <= 80mm
  pages: <= 1
  overflow: continue
  styles: summary-heading | summary-body | bullet
  data: report.metrics.* | report.period
  assets: none
```

Slots define validity. Agent capability policies separately define authority.
Filling a slot does not imply permission to resize the slot, alter protected
neighbors, change publishing settings, or sign output.

Layout-dependent slot limits declare a required scenario set. A candidate must
satisfy page, height, cardinality, and accessibility contracts across every
required scenario, not only the scenario currently visible in Studio.

## 8. Compiler architecture

```text
source buffer
  -> incremental lexer
  -> lossless CST
  -> semantic AST
  -> type and binding checks
  -> component expansion
  -> computed style templates and interned provenance
  -> canonical layout tree
  -> compiled Template
```

### 8.1 Compile/runtime separation

```go
template, err := paper.Compile(source, compileOptions)
err = template.Render(ctx, output, data, renderOptions)
```

Compilation performs all work that is independent of a particular dataset:

- Parsing and typing.
- Imports and package resolution.
- Component and property validation.
- Expression bytecode generation.
- Style/token resolution plans.
- Static subtree lowering.
- Asset and font manifest validation.
- Dependency graph construction.

The compiled template is immutable and concurrent-safe. A render session owns
data expansion, resource instances, planning state, limits, diagnostics, and
cancellation.

### 8.2 Resource catalog

Planning must not depend on mutating an output `document.Document`. Introduce an
immutable resource catalog that owns font metrics and identities, image/SVG
dimensions, asset hashes, and stable resource handles. The planner references
catalog handles; the painter maps those handles into one output document.

The project lockfile records transitive imports, component packages, grammar,
fonts, assets, Unicode/CLDR/hyphenation data, and content hashes. Resolution
specifies project-root and symlink behavior, package signature policy, archive
and decoder limits, and offline behavior. Reference PDFs, scans, OCR text,
filenames, diagnostics, and imported component content are untrusted inputs for
both the compiler and any agent context.

During migration, a scratch `Document` may temporarily adapt existing font or
image measurement code for parity, but it cannot become the architectural
resource owner. The target planner must be able to shape, measure, preflight,
and fail without registering resources in the final PDF.

### 8.3 Lossless editing model

Maintain both:

- A lossless CST for minimal source patches.
- A typed semantic AST for compiler and editor operations.

Visual or agent edits apply semantic operations, then a source rewriter patches
only affected token ranges. While source is temporarily invalid, Studio shows
diagnostics and continues previewing the last valid candidate where safe.

If source and plan revisions differ, the canvas must show an unmistakable stale
watermark and the exact `PlanRevision` it represents. Hit-test-driven edits and
visual mutations are disabled until revisions match; Studio must never combine
current-source selection with last-good-plan coordinates.

### 8.4 Canonical compiled layout tree

Do not build the hot path around `map[string]any`, reflection, or one interface
allocation per node. Start with private, dense, slice-backed structures:

```go
type Node struct {
    Kind       NodeKind
    ID         NodeID
    Key        NodeKey
    Style      StyleID
    Semantic   SemanticID
    FirstChild uint32
    ChildCount uint32
    Payload    uint32
    Source     SourceSpan
    Flags      NodeFlags
}
```

- Dense IDs support cache locality.
- Stable keys support humans and tools.
- Styles, tracks, strings, resources, and semantics are interned.
- High-level widgets lower to primitive kinds.
- Keep the IR private until it survives both engine migrations.

## 9. Unified layout engine

### 9.1 Planner protocol

Use compact switch-based algorithms over node kinds:

```text
intrinsic(node, inline constraints) -> min/max metrics
fragment(node, constraint space, break token) -> fragments + next break token
```

A break token stores resumable state such as:

- Child index in a flow.
- Line index in a paragraph.
- List item and nested continuation.
- Table row and rowspan group.
- Column or region continuation.

Every planner iteration must do one of three things:

1. Consume content.
2. Advance a break token.
3. Emit one explicitly oversized fragment and a diagnostic.

It may never loop by repeatedly adding blank pages.

### 9.2 Coordinate model

- Normalize to fixed-point PDF points inside the engine.
- A proposed initial resolution is 1/1024 point in signed `int64`.
- Convert current `Document` units at adapter boundaries.
- Convert to painter values only at the display-list boundary.
- Specify all rounding rules and include them in plan hashes.

### 9.3 Primitive node set

Initial primitives:

- `flow`
- `inline` / `paragraph`
- `list`
- `box`
- `row`
- `column`
- `grid`
- `table`
- `stack`
- `image`
- `canvas`
- `break`
- repeated page regions

Later primitives:

- balanced columns
- page and column floats
- footnotes and sidenotes
- advanced references and indexes
- charts and specialized accessible figures

### 9.4 Box and spacing model

- Containers own `gap`; boxes own `padding`, border, background, radius, and
  optional shadow.
- Margins do not collapse.
- Normal flow cannot use arbitrary x/y offsets.
- Stack and canvas make intentional layering explicit.
- Overflow policy is explicit: visible, clip, error, or continuation where
  supported.

### 9.5 Lengths and tracks

Support typed lengths:

```text
auto
42mm
35%
1fr
min-content
max-content
fit-content(70mm)
between(30mm, 1fr, 80mm)
```

One track resolver should serve grids, rows/columns, HTML flex lowering, and
tables. It must support fixed, percent, fractional, intrinsic, span
contributions, and min/max clamps without negative tracks.

### 9.6 Text

Introduce a stable shaping contract:

```go
type TextShaper interface {
    Shape(TextInput) (ShapedParagraph, error)
}
```

`ShapedParagraph` contains exact glyph data, advances, clusters, bidi order,
break opportunities, and fallback assignments. Line breaking consumes the
shaped paragraph; painting consumes the exact planned lines. Painting never
reshapes or rewraps.

Migration order:

1. Preserve current text behavior behind the new contract.
2. Add an ASCII fast path and stable caches.
3. Add better Unicode line breaking, bidi, shaping, and fallback without
   changing planner APIs.

Cache shaping by font identity, size, features, language, direction, and text
hash. Cache line breaking separately by shaped paragraph and width.

### 9.7 Flow and pagination

Support:

- Named page masters and regions.
- First, odd, even, and explicit named page masters.
- Repeated headers, footers, and backgrounds.
- Break before/after.
- Keep together and keep with next.
- Widow and orphan counts.
- Preferred versus strict breaks.
- Explicit split policies.

Keeps are preferences unless strict. If a kept group is larger than an empty
page, relax it, emit `KEEP_TOO_LARGE`, and fragment it.

Header and footer capacity comes from their actual planned subtrees, not a
separate estimate.

Master selection may depend only on bounded facts known before laying out the
page body: first/odd/even state, explicit named-page transitions, and known
section state. It may not depend on total-page results or on where content later
lands, which would create a circular body-capacity calculation.

### 9.8 Tables

All frontends use one table subsystem:

1. Normalize occupancy and spans.
2. Calculate min/max content contributions.
3. Resolve columns and span deficits.
4. Measure cell content at final widths.
5. Resolve row heights and rowspan deficits.
6. Form pagination groups.
7. Fragment rows with repeated headers/footers.
8. Emit boxes, content fragments, and semantic table metadata.

First-release policy:

- Rows are indivisible.
- Rowspan groups do not split.
- An oversized row is emitted once with an overflow diagnostic.
- Cell-safe row fragmentation is a later explicit feature.

Large tables should use page-windowed planning. Explicit/fixed tracks allow true
streaming. Content-sized columns may require a bounded premeasurement phase;
the language and diagnostics should make that cost visible.

### 9.9 Grid, row, column, and stack

- Row/column: grow, shrink, gap, alignment, justification, optional wrap.
- Grid: explicit tracks, row-major placement, spans, and later controlled auto
  placement.
- HTML flex lowers into row/column primitives after CSS resolution.
- Stack layers children in one resolved box.

### 9.10 Canvas constraints

Do not introduce a document-wide constraint solver. Canvas uses a local,
deterministic anchor graph:

- Left, right, top, bottom, center, and baseline relations.
- Explicit or intrinsic width and height.
- Equality plus offset constraints initially.
- Horizontal and vertical DAGs solved independently.
- Cycles and contradictions are source-located errors.
- Underconstrained nodes default to the canvas start edge.

This supports certificates, labels, overlays, and reference matching without
making every flow layout pay for a global solver.

### 9.11 Counters and references

- Reserve bounded width for total-page counters using configured page limits.
- Resolve counters and references before finalizing the `LayoutPlan`. If reflow
  is forbidden, shape the final value inside its previously reserved box.
- Cross-references receive at most one bounded correction pass.
- If the correction changes geometry again, emit
  `REFERENCE_LAYOUT_UNSTABLE`; do not iterate indefinitely.
- Post-layout inspection is read-only and cannot generate new structure.

The painter always receives final positioned glyph runs. It never replaces,
shapes, measures, or repositions counter text.

## 10. `LayoutPlan` and display list

`LayoutPlan` is the single immutable production artifact and owns its positioned
display commands. There is no second post-plan command-generation stage that
can recalculate geometry.

```go
type LayoutPlan struct {
    Pages       []PlannedPage
    Fragments   []Fragment
    Lines       []TextLine
    GlyphRuns   []GlyphRun
    Commands    []DisplayCommand
    Semantics   []SemanticNode
    Breaks      []BreakDecision
    Diagnostics []Diagnostic
}
```

Each fragment records:

- Node and instance IDs.
- Source span.
- Page and region.
- Margin, border, padding, and content rectangles.
- Fragment and continuation state.
- Semantic role.
- Minimal break reason.
- Overflow and clipping state.
- Optional detailed constraint trace.

The production plan stores concise reason codes. Debug/Studio mode may store
rejected break candidates, intrinsic inputs, constraints, timings, and style
derivations. Full tracing must be optional and bounded.

### 10.1 Plan detail and storage modes

- **Compact production:** retain page commands, resources, semantics, concise
  diagnostics, and the minimum index required for export.
- **Inspectable production:** additionally retain source/instance bounds and
  concise break provenance for audit and support tooling.
- **Studio:** retain spatial indexes, dependency edges, computed properties,
  and bounded detailed traces.

Very large documents must not require every command to remain in memory. The
planner writes immutable finalized page segments into a `PlanStore`, which may
spool display lists to bounded temporary storage while retaining page metadata
and a compact global index. This is O(output) storage even when it is not
O(output) memory. Validation and resource preflight finish before publication;
the painter then streams finalized segments into a temporary PDF output and
publishes it atomically. This preserves the existing large-output use case
without pretending that a complete inspectable plan is free.

### 10.2 Display list contract

Commands include:

- Save/restore state.
- Transform and clip.
- Fill/stroke path.
- Positioned glyph run.
- Image and SVG-derived primitives.
- Link annotation and destination.
- Widget/form instruction.
- Semantic marked-content association.

The PDF painter must not call `MultiCell`, `Write`, HTML rendering, automatic
page addition, or any method that performs wrapping or pagination.

### 10.3 Spatial and dependency indexes

Build:

- A per-page spatial index for hit testing and crops.
- Source node -> instance -> fragments indexes.
- Data path -> generated nodes dependencies.
- Style token -> affected nodes dependencies.
- Node -> display-list command ranges.
- Page continuation hashes for incremental suffix reuse.

### 10.4 Document envelope boundary

PDF metadata, attachments, output policy, encryption, and signing configuration
remain owned by `document`; they are not layout nodes. The plan contains only
visible or spatial concerns such as signature placeholders, form widgets,
annotations, and their semantic associations. Compatibility adapters carry the
nonvisual document envelope alongside the planned body and apply it during
output.

## 11. Preview and screenshot architecture

Use two render tiers:

1. **Fast preview:** rasterize the production display list directly. It supports
   tiles, node crops, overlays, and incremental invalidation without emitting a
   PDF.
2. **Authoritative verification:** serialize the PDF, rasterize it through a
   pinned external renderer, and compare it with the display-list preview.

This catches serialization, font embedding, transparency, annotation, and
resource defects without making every editor interaction pay PDF round-trip
cost.

### 11.0 Foundation geometry capture

Before the display-list preview exists, the planner may emit a deterministic,
one-page **layout geometry debug capture** for tools. The private
`LayoutPlan.CaptureDebugGeometrySVGPage` contract serializes page, fragment,
command, bounded-diagnostic, and committed-break geometry using raw
`Fixed` (1/1024-point) integer coordinates. Its canvas expands to include
recorded off-page geometry, and its metadata pins the capture format and
coordinate space.

It is deliberately not a document preview, raster capture, semantic reading
order, or PDF-equivalence claim. It does not include glyphs, images, payloads,
source text, diagnostic messages, or other user-controlled content. This
provides early AI/tool inspection of real planner geometry without pretending
that the current prototype paints a page.

### 11.1 AI capture targets

```text
paper.render(view="page:1")
paper.render(view="pages:1..4", layout="contact-sheet")
paper.crop(target="@invoice-lines", padding="8mm")
paper.crop(target="@invoice-lines/row[key=SKU-187]")
paper.crop(target="issue:overflow-7")
paper.crop(target="changed-fragments")
```

Every capture returns:

- A lossless PNG image for normative review and diffing. WebP may be offered
  only as a clearly non-normative transport thumbnail.
- Optional debug-overlay image as a separate layer.
- Page and crop bounds.
- Pixel-to-page transform.
- Intersecting fragment/node IDs and rectangles.
- Source ranges and semantic roles.
- Scenario, revision, plan, engine, font, and asset hashes.
- Diagnostics and nearby fragment links.

The capture manifest fixes color space, background/alpha, DPI, antialiasing
mode, renderer version, crop rounding, page profile, base/candidate revisions,
scenario, and device-pixel transform. A cross-page node returns page-local
frames with individual transforms; a stitched strip is a separate presentation
artifact.

Rendering is data disclosure. Support three explicit modes:

1. **Redacted scenario preview:** safe for restricted agents, but not proof that
   unredacted production content fits because redaction changes metrics.
2. **Isolated production validation:** runs real data but returns only authorized
   diagnostics and geometry summaries.
3. **Production capture:** privileged and audited disclosure of rendered values.

Plan queries, hit tests, crops, explanations, overlays, and caches are filtered
and partitioned by `PolicyRevisionID` and disclosure domain. A production render
or metadata cache can never be reused by a redacted session.

### 11.2 Views and overlays

- Low-resolution document contact sheet.
- Clean page.
- High-resolution node crop.
- Parent-context crop.
- Cross-page strip for continued content.
- Before/after.
- Onion skin.
- Pixel-difference heatmap.
- Box model and baselines.
- Grid and table tracks.
- Page regions and margins.
- Break opportunities and rejected breaks.
- Keep chains.
- Overflow, collision, clipping, and fallback fonts.
- Reading order and PDF tags.
- Changed-node-only view.
- Reference overlay and edge comparison.

Do not use operating-system screenshots for document reasoning. OS screen
capture is reserved for testing Studio itself. The offscreen capture API is
deterministic and excludes editor chrome, zoom artifacts, cursor state, and
unrelated windows.

### 11.3 Review bundle

One command can produce a content-addressed bundle:

```text
review/
  contact-sheet.png
  changed-page-02.png
  crop-title-before.png
  crop-title-after.png
  crop-table-break.png
  raster-diff-page-02.png
  source.diff
  semantic-diff.json
  plan-diff.json
  accessibility-diff.json
  diagnostics.json
  capture-manifest.json
```

Automatically recommend crops from changed nodes, diagnostics, new page breaks,
text reflow, large positional changes, and accessibility changes.

## 12. Paper Studio

Paper Studio is a semantic page debugger, not Figma-for-PDF.

### 12.1 Visual and interaction thesis

- The exact page canvas dominates the workspace.
- Chrome is restrained and contextual.
- One accent color communicates selection and action.
- Selection travels instantly among source, outline, page, and inspector.
- Direct manipulation changes semantic constraints, not unexplained pixels.
- Breaks and overflows explain themselves.
- Agent edits arrive as reviewable changesets.

```text
┌ File · Scenario · Page · Zoom · Overlays · Validate · Agent ┐
├─────────────┬─────────────────────────────────┬───────────────┤
│ Outline     │                                 │ Inspector     │
│ Components  │       exact page canvas         │ Layout/Style  │
│ Scenarios   │                                 │ Data/Semantic │
│ Issues      │                                 │ Why/Break     │
├─────────────┴─────────────────────────────────┴───────────────┤
│ Optional source split · diagnostics · agent changeset        │
└───────────────────────────────────────────────────────────────┘
```

The canvas should normally occupy at least 65-70% of available width. Panels
collapse. Source, design, split, review, accessibility, and reference modes
reuse the same engine and selection state.

### 12.2 Synchronized selection

Clicking a fragment should reveal:

- Exact selected rectangle.
- Other fragments of the same instance/source node.
- Source construct and outline path.
- Component definition and invocation.
- Bound scenario data.
- Resolved style with provenance.
- Semantic role and reading-order position.
- Relevant diagnostics and break ledger.

Repeated/master fragments must be visually distinguishable from ordinary
instances. Overlap selection should present an ordered candidate picker instead
of guessing.

### 12.3 WYSIWYM direct manipulation

- Flow dragging uses valid insertion points, not free coordinates.
- Grid handles edit tracks and gaps.
- Table handles edit columns, headers, and split policies.
- Box handles edit padding, border, radius, and background.
- Image handles edit fit, crop focus, and alt text.
- Canvas handles edit explicit anchors and constraints.
- Page handles edit masters, margins, bleed, and regions.
- Dynamic text opens binding or scenario-data editing instead of silently
  replacing an expression.
- Repeated rows distinguish row-template editing from fixture editing.
- Dragging an observed page break opens a policy chooser; it never guesses among
  break, keep, row-splitting, or spacing policies.

A drag may use an optimistic local preview, but releasing it creates one typed
transaction and swaps in an exact plan. Optimistic feedback is visibly ghosted
or watermarked and cannot be mistaken for the current exact plan. Handles edit
governing source inputs, never computed rectangles.

Before applying a component-definition, theme-token, import, or page-master
edit, Studio shows its blast radius: affected instances, nodes, pages, profiles,
required scenarios, and protected scopes.

### 12.4 Break ledger: signature feature

Every fragmentation decision records:

- Available and required dimensions.
- Candidate breakpoints.
- Keep/widow/orphan/row policies.
- Candidate costs.
- Rejected candidates and reasons.
- Repeated-region and footnote reservations.
- Final choice.

Example:

```text
Row 18 moved to page 3

Page 2 remaining:       12.8 mm
Row required:           15.2 mm
Row splitting:          forbidden
Repeated table header:   8.4 mm

Rejected:
- Split row: table.split = rows
- Compress gaps: no flexible gaps
- Keep on page 2: overflow by 2.4 mm
```

The inspector may offer typed experiments such as allowing a split, adjusting a
spacing token, or inserting a preferred break. “Try” creates a candidate;
“Apply” creates a semantic patch.

### 12.5 Reference mode

Import a reference PDF, scan, or screenshot and provide:

- Page calibration and optional perspective correction.
- Side-by-side view.
- Ghost overlay and opacity control.
- Difference and edge overlays.
- Rulers, guides, and region crops.
- Optional OCR/zone suggestions.
- Agent-proposed semantic structure with confidence markers.

The goal is to reconstruct a flow/grid/table/master design, not flatten the
reference into absolute coordinates.

### 12.6 Create-to-deliver workflow

Studio must support complete authoring, not only debugging:

```text
New template -> connect schema -> create scenarios -> define page master
-> insert primitives/components -> bind data -> run stress matrix
-> preflight -> verify PDF -> export/publish
```

The component palette is typed and slot-aware. It searches by capability and
shows previews using the active theme and scenario. A binding picker exposes
schema paths, types, optionality, formatting, and current fixture values.

The resource manager covers font fallback, embedding and license status, asset
hashes, replacement, image crop focus, missing-resource recovery, and which
nodes/scenarios use each resource. Basic component insertion, schema binding,
and resource management ship with semantic direct manipulation; they are not
deferred to the later team ecosystem.

### 12.7 Working copy, undo, and collaboration

Source keystrokes, semantic operations, visual edits, fixture edits, and agent
candidates share one revision journal while remaining distinct transaction
types. The journal defines grouped text edits, semantic transaction boundaries,
candidate branches, redo invalidation, external file reloads, crash recovery,
and conflict behavior.

Undo never applies an inverse operation against a changed head silently. It
either reverts the exact current transaction, rebases by semantic preconditions,
or reports a conflict. Comments anchor to source identity plus page-local
rectangle and `RenderID` fallback. Lasso annotations store page coordinates and
transforms, not exported-image pixels.

Git remains durable source truth. A future collaboration layer may use a CRDT
for text, but structural conflicts resolve by source ID/property and never hide
conflicting semantic effects.

### 12.8 Desktop and web strategy

Start desktop-first for exact fonts, local assets, offline operation, file
watching, low latency, and final-PDF verification.

```text
paper-core       compiler and planner
paperd           incremental compile/preview/inspection RPC
paper-cli        check/render/capture/diff
paper-studio     shared frontend
desktop shell    local paperd process
web shell        isolated remote paperd service, later
agent adapter    the same RPC exposed as tools
```

The frontend never performs substitute CSS layout. A future web shell uses the
same remote engine. WASM may assist parsing or editing but must not become a
second layout implementation.

## 13. Agent tool protocol

The transport may use structured JSON internally; the human-authored document
does not.

### 13.1 Read tools

- `paper.create`: create an uncommitted document workspace from a schema,
  theme, component/template, reference, or blank page profile.
- `paper.open`: open a revision, scenario, and capability mode.
- `paper.context`: create a token-budgeted task context.
- `paper.components`: discover components by capability, props, slots,
  semantics, pagination behavior, and current-theme previews.
- `paper.inspect`: source, AST, editable properties, computed values,
  instances, or provenance for one target.
- `paper.search`: find nodes by kind, role, text, page, or diagnostic.
- `paper.layout_query`: bounded queries over fragments, boxes, constraints,
  font runs, reading order, or dependency edges.
- `paper.explain_layout`: explain page break, position, size, overflow, style,
  font, or reading order.
- `paper.hit_test`: map a page coordinate to candidate fragments and source.

### 13.2 Mutation tool

Use one general transactional operation: `paper.propose_patch`.

Supported typed operations:

- `set_property`
- `set_literal`
- `set_rich_text`
- `set_binding`
- `set_scenario_value` in the separate fixture revision domain
- `insert`
- `remove`
- `move`
- `wrap`
- `unwrap`
- `replace_component`
- `fill_slot`
- `rename_id`
- `apply_fix`

Each proposal includes:

- Base revision.
- Idempotency key.
- Expected node fingerprints.
- Typed property values.
- Requested operations.

It returns an immutable candidate revision, semantic diff, minimal source diff,
diagnostic delta, and invalidated nodes/pages. It never overwrites the head.

Each transaction declares one revision domain: template source, scenario
fixture, or policy. A review changeset may group candidates from multiple
domains, but they retain separate preconditions, permissions, and approvals.

Requirements:

- Atomic application.
- Idempotency.
- Optimistic revision and node preconditions.
- No fuzzy rebasing.
- Semantic conflict reporting by stable ID/property.
- No generic shell, arbitrary source evaluation, or network operation.

Editing a selected component instance must explicitly choose definition edit,
invocation override, slot content, or scenario fixture; the tool never infers
the target. Missing or duplicate repeated-data keys emit deterministic
`INSTANCE_KEY_MISSING` or `INSTANCE_KEY_DUPLICATE` diagnostics. Index-based
instance addresses are read-only and plan-local.

Revision, candidate, plan, render, capture, diagnostic, and approval handles
are opaque, unguessable, project/session-scoped, expiring, revocable, and
authorization-checked on every dereference.

### 13.3 Validation and visual tools

- `paper.validate`: syntax, types, layout, accessibility, PDF/UA, PDF/A,
  security, and performance across scenarios.
- `paper.render`: contact sheet, pages, or cross-page strip.
- `paper.crop`: deterministic node/instance/issue crop.
- `paper.diff`: source, semantic, layout, pixels, reading order, and
  accessibility.
- `paper.scenario`: list, generate, boundary-search, and minimize scenarios.
- `paper.commit`: advance head after approval and validation.

Publishing, exporting, attaching, modifying protected nodes, and signing are
separate capabilities. Edit permission never implies any of them.

An approval token binds the exact candidate, `PolicyRevisionID`, expected head,
semantic/source diff, required scenarios, validation profiles and versions, and
review artifacts. Tokens expire, contain a nonce, and cannot be replayed after
the head or evidence changes.

### 13.4 Agent document-creation loop

1. Create an uncommitted workspace with grammar, page profile, schema, theme,
   policy, and required scenario set.
2. Discover components by capability and slot contract.
3. Propose the semantic outline and page masters before detailed styling.
4. Bind typed data and declare stable repeated-data keys.
5. Fill controlled slots and create local components where authorized.
6. Render typical and boundary scenarios.
7. Iterate through semantic patches and targeted crops.
8. Run layout, accessibility, privacy, and compliance preflight.
9. Present one review bundle and candidate source revision for approval.

Creation uses the same transactions and evidence as editing; an agent never
writes an untracked complete file and claims success.

### 13.5 Agent visual loop

1. Open and receive outline, capabilities, and diagnostic summary.
2. Request task-specific context rather than the whole document.
3. Render low-resolution contact sheets for normal and extreme scenarios.
4. Use diagnostics and hit testing to select suspect regions.
5. Request high-resolution clean and overlay crops.
6. Ask for causal layout explanations.
7. Propose a typed patch against a known revision.
8. Incrementally replan and rerender affected pages.
9. Compare semantic, layout, pixel, and accessibility differences.
10. Run adversarial scenarios.
11. Commit the source candidate when its editing policy gates pass.
12. On idle, explicit verification, export, publish, or signing, verify the
    serialized PDF through raster, structural inspection, and compliance tools.

Studio distinguishes `Plan preview`, `PDF verified`, and `Verification stale`.
Raster verification cannot validate links, annotations, tags, reading order,
forms, attachments, alternative text, or signatures, so final verification
combines raster comparison with structural PDF inspection and applicable
compliance validators.

### 13.6 Token-efficient context

- Return an outline before full source.
- Slice schema and source to the requested anchors.
- Include only relevant ancestors, siblings, components, styles, and data paths.
- Use handles for revisions, plans, renders, diagnostics, and captures.
- Paginate searches and layout queries.
- Return diagnostic deltas.
- Use contact sheets before detailed crops.
- Return changed pages instead of entire documents.
- Keep clean and overlay images separate.
- Redact data at binding time, not by blurring rendered pixels.
- Mark context as trusted policy, template source, untrusted document text,
  untrusted data, or diagnostic output.

## 14. Capability and security model

Example capability set:

```text
source.read
schema.read
data.read.redacted
render.redacted
slot.fill:@executive-summary
content.edit:@appendix
style.tokens:report.*
layout.structure:@summary-grid
asset.use:approved/*
protected.edit:false
publish:false
sign:false
```

Rules:

- Slots describe valid content; policies describe authorized actions.
- The agent cannot edit or elevate its own policy.
- Legal clauses, bank details, signatures, verification URLs, and compliance
  metadata may be protected.
- Production data read and production rendering are separate capabilities.
- Untrusted bound data and document text are never interpreted as agent
  instructions.
- Edits invalidate earlier render, compliance, export, and signing attestations.
- Authorization applies to semantic effects, not merely requested targets. A
  theme, component, import, or page-master edit must compute its transitive
  affected-node set and fail if the effect crosses a protected, slot, file, or
  component boundary without authority.

Template source, scenario fixtures, project policy, and production data are
different revision domains with different typed transactions and capabilities.
`set_scenario_value` changes a fixture candidate; it does not edit a template or
production input. Production data remains externally owned and immutable unless
the caller has separate data-write authority.

### 14.1 Runtime limits

Enforce limits on:

- Source bytes and tokens.
- Nesting and imports.
- Expanded nodes and loop iterations.
- Rows, columns, pages, glyphs, fonts, and display commands.
- Image pixels, SVG complexity, and render DPI.
- Memory, CPU work, and trace size.
- Asset paths and URI schemes.

Cancellation must penetrate parsing, type checking, expansion, shaping, table
planning, page fragmentation, image decode, preview, painting, and export.

Assets are project-root confined, allowlisted, and content-addressed. Remote
fonts/images are disabled during render. SVG, fonts, images, attachments, links,
forms, and signing use existing PaperRune security limits plus new planner work
budgets.

### 14.2 Audit ledger

Record append-only events containing:

- Actor/session/model identity.
- Capability used and approval reference.
- Base and candidate revisions.
- Idempotency key.
- Before/after node hashes.
- Semantic patch and source diff hash.
- Diagnostics before/after.
- Scenario, plan, render, and artifact hashes.
- Commit, publish, export, and sign events.

Chain event hashes for tamper evidence. Keep prompts and sensitive values out of
the normal ledger; store hashes or protected references. Do not pollute `.paper`
source with model metadata.

Periodically sign or externally anchor ledger roots; a local hash chain alone
can be rewritten by an actor controlling its store. Audit denied mutations,
production reads/captures, policy changes, expired or cross-session handle use,
and publish/sign attempts in addition to successful actions. Sensitive patch
payloads use encrypted references under an explicit retention policy.

## 15. Scenarios and stress testing

Paged documents are responsive to content, locale, data volume, page profile,
fonts, and assets rather than screen width alone.

```text
scenario empty:
  data: samples/empty.json

scenario typical:
  data: samples/invoice.json

scenario long-names:
  derive: typical
  mutate:
    invoice.customer.name: repeat(4)

scenario rtl:
  data: samples/arabic.json
  locale: ar

scenario letter:
  derive: typical
  page-size: Letter
```

Default generated strategies:

- Empty and missing optional data.
- Minimum and maximum collections.
- Long localized strings and unbreakable tokens.
- CJK, RTL, bidi controls, combining marks, and emoji.
- Extreme numbers, dates, and currency formats.
- One oversized table row and many normal rows.
- Exact-fit and one-unit-over page boundaries.
- Extreme image aspect ratios and corrupt assets.
- Missing fonts and fallback chains.
- A4, Letter, portrait, and landscape profiles.
- Repeated headers larger than page regions.
- Nested components, tables, lists, and keeps.

A boundary finder varies schema-compatible values until page count, break
selection, or overflow changes, then delta-debugging minimizes the dataset.
Every issue is replayable from seed, revision, data hash, assets, fonts, page
profile, and engine version.

## 16. Performance plan

### 16.1 Baseline first

Record current named baselines before implementation. Existing README medians
include approximately:

- Compiled large HTML table: 5.13 ms, 5.23 MB/op, 11,515 allocs/op.
- Compiled wide HTML table: 1.46 ms, 1.58 MB/op, 3,538 allocs/op.
- Compiled selector-heavy HTML: 0.63 ms, 193 KB/op.

Use `benchstat`, pprof, and traces on pinned local hardware. CI should enforce
algorithmic and allocation invariants; local/release workflows should enforce
timing budgets.

### 16.2 Engine budgets

Initial Stage 0 hypotheses to benchmark and calibrate:

- Typed cutover should remain near the legacy typed path while adding a reusable
  plan; compare planner-only, painter-only, and end-to-end costs separately.
- HTML cutover should materially reduce time and allocations for large and wide
  compiled tables by removing duplicate layout and painting calculations.
- Compiled `.paper` should outperform equivalent compiled HTML because it skips
  HTML/CSS adaptation, but no multiplier is accepted until equivalent retained
  plan detail is measured.
- All normal planner algorithms demonstrate linear or near-linear scaling.
- No hidden whole-document repeated layout.

Stage 0 records comparable fixtures and plan-detail modes, then sets numeric
release gates from repeated `benchstat` samples. Do not reject or approve an
architecture using unsupported 0.5x/0.7x estimates.

### 16.3 Interaction budgets

Measure on pinned hardware and representative fixtures:

- Inspect/context/layout query: p95 under 20 ms.
- Semantic patch plus incremental parse/type check: p95 under 30 ms.
- Cached-plan crop: p95 under 50 ms.
- Affected-page layout and 144-DPI preview: p95 under 200 ms.
- Local handle feedback: one frame where an optimistic preview is valid.
- Full stress matrix: asynchronous with progressive results.

These are target hypotheses to calibrate during Stage 0, not public promises.

### 16.4 Hot-path strategy

- Dense arena slices and interned styles.
- No per-node property maps or interface dispatch in the planner.
- Fixed-point geometry.
- ASCII text fast path.
- Shaping and line-layout caches.
- Prefix offsets for span operations.
- Measure repeated regions once per width/profile.
- Page-windowed table planning.
- Bounded, byte-accounted caches.
- Detailed traces disabled in production.
- Object pooling only after profiles prove a benefit.
- Explicit resource manifests and reused decoded resources.

### 16.5 Incremental layout

Maintain dependencies:

```text
source node
  -> component/style/data expansion
  -> intrinsic metrics
  -> fragments/pages
  -> display-list ranges
  -> raster tiles
```

After a change:

1. Invalidate the changed node and dependents.
2. Start from the earliest affected fragment/page.
3. Replan the suffix.
4. Compare continuation-state hashes at page boundaries.
5. Reuse the old suffix once the state converges.
6. Rerasterize only visible changed tiles immediately.

Global theme or font changes may invalidate the document. Most local edits
should touch one page and a bounded suffix.

## 17. Diagnostics and failure policy

Every diagnostic contains:

- Stable code and severity.
- Pipeline stage.
- Source node and span.
- Instance and fragment where applicable.
- Scenario, page, and rectangle.
- Evidence and related diagnostics.
- Typed fixes when safe.

Required failures include:

- `UNBREAKABLE_TOO_TALL`: emit once, mark overflow, never loop.
- `PAGE_REGION_NO_BODY_SPACE`: header/footer consumed the body.
- `KEEP_TOO_LARGE`: relax a preference; fail if strict.
- `REPEATED_HEADER_TOO_TALL`: disable repeat in compatibility mode or fail in
  strict mode.
- `TABLE_SPAN_INVALID`: clip only in compatibility mode.
- `TABLE_ROWSPAN_CROSSES_PAGE`: keep group or emit oversized diagnostic.
- `TRACK_MIN_OVERFLOW`: preserve minimums and report horizontal overflow.
- `CONSTRAINT_CYCLE` and `CONSTRAINT_OVERDETERMINED`.
- `REFERENCE_LAYOUT_UNSTABLE`.
- `FONT_MISSING`, `GLYPH_MISSING`, and `IMAGE_MISSING`.
- `RESOURCE_LIMIT`, `WORK_LIMIT`, and `CANCELED`.
- `PAINTER_RESOURCE_MISMATCH`.

Plan and resource preflight must complete before adding a PDF page. This avoids
partially mutated documents on planning failure.

## 18. Accessibility and PDF compliance

Semantics are independent from visual style and survive all frontends.

Track:

- Document language.
- Heading levels.
- Paragraphs, lists, figures, captions, and notes.
- Table header/data cells, row/column scopes, and spans.
- Link text and targets.
- Alternative text and decorative status.
- Form labels, descriptions, and keyboard order.
- Reading order distinct from z-order.

Studio accessibility mode provides a tag tree, numbered reading-order overlay,
linearized preview, language changes, table-scope visualization, missing-alt
diagnostics, and contrast checks.

Tests verify extracted text, reading order, tagged structure, annotations,
PDF/UA, PDF/A, forms, signatures, and attachments. Existing compliance tooling
continues to validate release fixtures.

## 19. Testing and evaluation

### 19.1 Compiler and editing

- Lexer/parser fuzzing and termination.
- Parse/format/parse semantic round trips.
- Lossless CST preservation.
- Formatter idempotence.
- Semantic patch idempotence.
- Stable ID and rename/reference tests.
- Stale revision and node fingerprint conflicts.
- Slot and capability enforcement.
- Minimal source patch tests.
- Long mixed sequences of source, move, wrap, unwrap, extract-component,
  visual, fixture, undo, and redo operations with trivia-preservation goldens.

### 19.2 Planner invariants

- No invalid or negative extents.
- Every fragment belongs to one page and region.
- Normal-flow fragments are ordered and non-overlapping.
- Text clusters appear exactly once unless deliberately repeated.
- Break tokens always advance.
- Out-of-bounds content carries overflow state.
- Spans occupy valid tracks.
- Display commands reference valid resources and semantics.
- Identical inputs produce identical plan hashes.
- Incremental planning equals a clean full rebuild.

### 19.3 Layered goldens

1. Canonical `.paper` source.
2. Typed AST.
3. Expanded component tree.
4. Normalized `LayoutPlan` and break reasons.
5. Display list.
6. Direct preview raster.
7. Actual-PDF raster with pinned renderer.
8. Extracted text and reading order.
9. Tagged semantic tree.
10. PDF/A and PDF/UA validator results.
11. Agent tool transcripts.
12. Typed/HTML/`.paper` adapter parity.

Use exact pixels for hermetic simple fixtures. Use perceptual thresholds and
localized masks for antialiasing-sensitive output. Never depend only on PDF
byte hashes or only on raster images.

### 19.4 Differential compatibility

Equivalent typed, HTML, and `.paper` documents should compare:

- Pages and regions.
- Fragment rectangles.
- Text lines and reading order.
- Row and page breaks.
- Links, destinations, and semantic roles.
- Display-list commands where applicable.
- Raster output.

Expand the existing typed-versus-HTML parity coverage beyond page count.

### 19.5 Agent evaluation corpus

Tasks should include:

- Add a field without changing unrelated content.
- Fix overflow without shrinking body text.
- Improve a header while preserving page count.
- Fill only an approved slot.
- Refuse to edit a protected legal clause.
- Explain why a row moved to another page.
- Improve reading order without visual changes.
- Match a reference crop using semantic layout.

Score correctness, permission compliance, patch locality, unrelated source and
pixel churn, diagnostics, accessibility, scenario validation, tool calls,
images, tokens, latency, and false success claims.

Run a semantic-plus-vision versus screenshot-only comparison. The toolchain
should demonstrate that structured causality reduces edits, tokens, and errors.

### 19.6 Protocol, privacy, and recovery evaluation

- Concurrent candidate creation and expected-head races.
- Replay, stale fix, expired handle, revoked handle, cross-session handle, and
  capability-downgrade cases.
- Prompt injection through source text, bound data, alt text, OCR, filenames,
  diagnostics, reference PDFs, and imported packages.
- Privacy proofs that protected values do not enter contexts, logs, caches,
  errors, plan queries, crops, or overlays.
- Transitive authorization for theme, component, import, and page-master edits.
- Property-based semantic patch and capability testing.
- Crash/restart recovery for working copies, candidates, commits, plan stores,
  and audit events.
- Determinism across warm/cold caches, process restarts, goroutine schedules,
  supported operating systems, and CPU architectures.

## 20. Package and process shape

Initial implementation should remain private where contracts are immature:

```text
paper/
  public compile/render/format/edit facade

internal/paperlang/
  lexer, CST, AST, type checker, expression VM, formatter, source patches

internal/layoutengine/
  tree, styles, planner, flow, text, tracks, table, grid, canvas,
  pagination, plan, display list, diagnostics, indexes

document/
  layout resource adapter, display-list PDF painter, typed compatibility adapter,
  HTML-to-IR adapter

cmd/paper/
  fmt, check, render, capture, explain, diff, scenario

cmd/paperd/
  incremental compiler, LSP/RPC, preview and inspection service

studio/
  page canvas and editor client, introduced after headless tooling
```

Do not expose the canonical IR publicly until both typed and HTML migrations
prove it. Later, expose stable inspection types through a focused package rather
than expanding `document.Document`.

## 21. CLI and service surface

Proposed commands:

```text
paper fmt invoice.paper
paper check invoice.paper --scenario typical
paper render invoice.paper --data invoice.json --output invoice.pdf
paper capture invoice.paper --target @lines --overlay break
paper explain invoice.paper --target @lines/row[key=SKU-187]
paper diff old.paper new.paper --scenario many-rows
paper scenario invoice.paper --find page-boundary --target @lines
paper verify invoice.paper --profiles pdfua,pdfa
```

`paperd` owns open documents, lossless syntax trees, candidate revisions,
compiled templates, plans, raster tiles, spatial indexes, and tool/RPC handles.
The CLI, Studio, LSP, and agent adapter call the same service operations where
appropriate.

## 22. Migration plan

### Stage 0: characterization and ADR

Deliver:

- Architecture decision record.
- Feature matrix for current typed and HTML engines.
- Representative compatibility corpus.
- Page/text/annotation/tag/raster baselines.
- Named performance and allocation baselines.
- Fixed-point and identity prototypes.

Gate:

- Every documented current feature has a fixture or is explicitly deprecated.
- Baseline commands are reproducible.

### Stage 1: identity, diagnostics, and plan contracts

Deliver:

- `NodeKey`, source span, instance, fragment, revision, policy, plan, and render
  identity contracts.
- Structured diagnostics, break codes, provenance tables, and trace schemas.
- Minimal immutable `LayoutPlan` and `PlanStore` prototypes built from synthetic
  fixtures.
- Resource catalog and fixed-coordinate prototypes.
- Capture/hit-test manifest schema and a tiny internal test-plan viewer.

Gate:

- Identity, diagnostic, plan, capture, and trace schemas round-trip and hash
  deterministically on prototype plans.
- No claim of full break explainability is made before the planner exists.

### Stage 2: planner kernel, painter, and internal Plan Viewer

Deliver:

- Fixed-point geometry and immutable resource catalog.
- Canonical private tree and computed style templates.
- Page masters, boxes, simple flow, paragraph lines, and images.
- `LayoutPlan` containing positioned fragments and display commands.
- No-layout PDF painter and direct preview rasterizer.
- Internal interactive viewer for selection, hit testing, bounds, break traces,
  tiles, and crops.
- Shadow planner for comparison without production painting.

The foundation bridge begins as a private observational shadow rather than a
`WriteDocument` switch. It accepts only fresh documents containing plain,
core-font, keep-together paragraphs; measures them on an isolated scratch
document; converts legacy user units to fixed PDF-point coordinates; and
compares page allocation with the current typed renderer. Unsupported
callbacks, templates, block types, styling, or document state return a stable
unsupported result and leave production rendering untouched.

The next private kernel slice owns immutable, already-shaped line metrics and
fragments them through opaque paragraph-bound continuation tokens. Canonical
plans now retain positioned line bounds, baselines, paragraph-local indexes,
fragment ownership, and source spans. The fragmenter distinguishes placement
from legal region deferral, uses next-region height lookahead for variable-line
widow rules, supports preferred and strict widow/orphan policy, records early
policy breaks truthfully, and must emit an oversized individual line exactly
once rather than retrying blank pages. This is still a prebroken-line contract:
legacy-compatible wrapping/shaping and glyph display commands remain separate
gates before any production cutover.

The atomic legacy shadow is further restricted to printable ASCII plus line
feeds for core fonts. Current `SplitTextCount` measurement and `MultiCell`
painting do not agree for every UTF-8 byte sequence or Unicode whitespace, so
the shadow must reject that uncharacterized input instead of reporting false
page-count parity.

A shared private streaming wrapper now owns the legacy split mechanics. Each
line records a visible byte range separately from the consumed range so spaces,
explicit line feeds, empty lines, and resume positions are not conflated. The
existing UTF-8 and byte-oriented split/count APIs use this scanner with their
original normalization and separator profiles. A separate core-font
`MultiCell` profile preserves its one-terminal-line-feed rule and mandatory
final empty cell without changing production painting yet; synchronous yields
leave a path to preserve lifecycle callback behavior when painting migrates.

The first text-to-line bridge accepts one fresh plain splittable paragraph,
resolves core-font metrics on an isolated document, and lowers exact wrapper
results into the paragraph kernel. Planned line geometry includes alignment
offset, natural width, fixed line advance, and the legacy cell baseline. It
uses greedy one-line widow/orphan compatibility to compare current page
allocation, retains visible and consumed byte ranges privately, and rejects
decorations, styled segments, links, non-ASCII input, custom lifecycle policy,
and oversized line heights. It remains observational and emits no glyph
commands.

Gate:

- Same page count and text order for simple typed documents.
- A CLI/viewer can inspect coordinates, explain supported breaks, and capture a
  deterministic node crop.
- Painter performs no layout.
- Planning failures occur before final PDF publication.
- Calibrated Stage 0 planner/painter/end-to-end budgets pass.

### Stage 3: complete typed layout and cut over

Deliver:

- Lists, tables, spans, metadata grids, visible signature/form placeholders,
  QR, page breaks, repeated regions, links, semantics, and tagging.
- `LayoutDocument -> lower -> plan -> preflight -> paint`.
- Document-envelope adapter for metadata, attachments, output policy, and
  signing configuration.
- Detailed break ledger in debug mode.

Gate:

- All typed tests use the new planner.
- Every typed break is explainable through the ledger.
- Typed goldens, semantics, compliance, and calibrated benchmark gates pass.
- Legacy typed renderer remains private as a temporary rollback path.

### Stage 4: HTML-to-IR migration cohorts

Retain HTML tokenizer, parser, CSS cascade/specificity resolution, templates,
compiled cache, validation, data-image handling, and SVG compilation. Replace
layout and direct drawing feature cohorts:

1. Inline/block text, headings, links, and lists.
2. Box model, borders, backgrounds, and break rules.
3. Images and figures.
4. Tables.
5. Flex row/column/wrap.
6. Inline SVG and structured table-cell content.

CSS cascade, inheritance, and provenance resolve before IR creation.
Percentages, `auto`, intrinsic sizes, and containing-block-relative values
remain typed planner inputs. The planner sees primitives, not selectors.

`HTML.Write` compatibility also needs a `StartFrame` contract because HTML may
begin at the current page/cursor amid manual drawing. The frame snapshots page
size/profile, margins, cursor, current font context, auto-page-break policy, and
the body region. Planning returns the final page/cursor state that the adapter
applies after painting.

For this mid-document entry case, plan-before-paint means the HTML call performs
no additional `Document` mutation until its fragment plan and resources pass;
it cannot undo manual content the caller created before the captured frame.

Migration is whole-fragment:

- If the adapter supports the complete compiled fragment, use the new planner.
- Otherwise use the legacy HTML path temporarily.
- Do not place legacy layout islands inside a unified flow.

Gate per cohort:

- IR snapshots, entry/exit frame behavior, pages, fragments, raster, semantics,
  limits, and calibrated benchmark parity pass.

### Stage 5: HTML default and stabilization

Deliver:

- Unified planner becomes default for every documented supported HTML feature.
- Current public HTML APIs remain compatibility frontends.
- `LongFormHTMLDocumentModel` uses HTML-to-IR instead of lossy flattening.
- Legacy HTML and typed paths remain private rollback options during a defined
  stabilization window; no permanent public legacy switch is introduced.

Gate:

- A full release completes with compatibility, performance, compliance, and
  issue feedback inside the agreed error budget.
- The canonical IR is considered proven against both existing engines before
  the `.paper` grammar is stabilized on it.

### Stage 6: `.paper` language foundation

Deliver:

- Lossless parser, semantic AST, formatter, types, bindings, expressions,
  imports, package lock, themes, components, slots, scenarios, and resource
  manifest.
- Separate source, semantic-template, scenario, and policy revisions.
- CLI compile/check/render.
- Typed source diagnostics and prototype visual-edit/minimal-patch sequences.

Gate:

- Parse and format round trips are stable.
- Mixed edit sequences preserve trivia and produce minimal diffs.
- Equivalent `.paper`, typed, and HTML documents produce equivalent plans.
- Warm `.paper` rendering meets its calibrated benchmark budget.

### Stage 7: headless agent and preview tools

Deliver:

- `paperd` incremental service.
- Semantic transactions, candidate branches, fixture transactions, and working
  copy recovery.
- Tool protocol, capabilities, protected nodes, transitive authorization, and
  anchored audit log.
- Contact sheets, crops, overlays, hit tests, explain, and four-part diff.
- Scenario boundary finder and minimizer.

Gate:

- An agent can create, locate, explain, patch, preview, stress-test, and propose
  a changeset without reading or rewriting the whole file.
- No GUI is required for the full workflow.

### Stage 8: read-first Paper Studio

Deliver:

- Desktop shell and exact page canvas evolved from the internal Plan Viewer.
- Source, outline, inspector, page rail, issues, and scenarios.
- Bidirectional selection and stale-plan protection.
- Break, overflow, box, grid, table, and accessibility overlays.
- Hot reload and incremental visible-page rendering.

Gate:

- Any visual result can be traced to source/data/style/layout causality.
- Normal visible-page updates meet calibrated interaction budgets.

### Stage 9: semantic direct manipulation and review

Deliver:

- Typed component insertion, slot-aware drop targets, and schema binding picker.
- Resource manager for fonts, assets, hashes, licenses, fallback, and crop focus.
- Primitive-specific handles and speculative-state labeling.
- Inline literal, binding, and separate scenario-fixture editing.
- Page master and canvas editing.
- Revision journal, semantic undo/redo, refactors, and external reload conflicts.
- Agent changesets, screenshot annotation, reference mode, and review bundles.

Gate:

- The complete create-to-deliver workflow is usable.
- Common visual edits preserve readable source and minimal diffs.
- Every agent change is reviewable by source, semantics, plan, pixels, and
  accessibility.

### Stage 10: legacy engine deletion

After the stabilization window, delete legacy automatic layout code in a
planned breaking release. Candidates include layout logic in:

- `layout/measure.go`
- `document/document_measure.go`
- `document/document_render.go`
- `document/html_render_session.go`
- `document/html_layout.go`
- `document/html_flex.go`
- `document/html_tables.go`

Keep public typed and HTML entry points as adapters. Deletion occurs only after
the default cutover has already shipped and rollback/issue criteria are met.

### Stage 11: ecosystem and production hardening

Deliver:

- Component gallery and package/version lock.
- Team themes and policies.
- Shared scenario and visual-baseline libraries.
- Collaboration and semantic conflict resolution.
- Web Studio backed by isolated `paperd`.
- Compliance profiles and organization controls.
- Reproducibility manifest and signed audit export.

Gate:

- Security review, fuzzing, workload limits, reproducibility, protected-content,
  publish, export, and signing authorization tests pass.

## 23. First implementation slices

Avoid beginning with the entire language or GUI. The first concrete pull
requests should be:

1. ADR, compatibility corpus, and benchmark baselines.
2. Fixed-point geometry and deterministic rounding tests.
3. Resource catalog, private node tree, fragment IDs, diagnostics, and minimal
   plan serializer.
4. Flow + paragraph + box planner with a no-layout PDF painter.
5. Direct display-list page raster and internal node crop/hit-test viewer.
6. Typed adapter shadow comparison.
7. Table planner prototype against current typed and HTML fixtures.
8. Break ledger and `explain` CLI.
9. HTML `StartFrame` plus the first text/box HTML-to-IR cohort.
10. Lossless `.paper` parser and minimal-patch prototype for the proven
    primitive set.
11. Candidate revision and semantic patch prototype.

Do not begin Paper Studio until the headless engine can inspect, capture, explain,
and patch a document deterministically.

## 24. Risks and mitigations

### Risk: scope becomes a browser/typesetter rewrite

Mitigation: strict primitive set, feature cohorts, explicit non-goals, no CSS in
the engine, no general scripting, and no global solver.

### Risk: language design freezes too early

Mitigation: private IR, versioned grammar, formatter/migration tooling, and a
prototype corpus before a stable-language promise.

### Risk: preview differs from PDF

Mitigation: one display list, exact positioned glyph runs, and an authoritative
PDF-raster comparison after idle/export.

### Risk: observability makes production slow

Mitigation: concise always-on reason codes; detailed traces opt-in, bounded, and
separately allocated.

### Risk: incremental layout is incorrect

Mitigation: page continuation hashes and mandatory differential tests comparing
incremental plans with clean full rebuilds.

### Risk: HTML parity traps the new engine

Mitigation: resolve CSS at the adapter edge, migrate whole fragments in cohorts,
and preserve only documented compatibility behavior.

### Risk: AI edits become broad or unsafe

Mitigation: stable IDs, typed operations, immutable candidates, preconditions,
slots, capabilities, protected nodes, scenario validation, and approval gates.

### Risk: visual editor destroys source readability

Mitigation: lossless CST, minimal token patches, semantic handles, canonical
insertion formatting, and no generated absolute coordinates in flow.

### Risk: complex text delays engine replacement

Mitigation: preserve current shaping behind a stable contract first; improve
Unicode behavior later without changing the planner or painter.

### Risk: large data causes memory blowups

Mitigation: work limits, page-windowed tables, streaming production mode,
bounded caches, cancellation, and plan-detail levels.

## 25. Decisions that require prototypes

Before locking the design, prototype and measure:

- 1/1024-point fixed coordinates versus another fixed precision.
- Lossless parser implementation strategy and incremental reparsing cost.
- Direct raster backend versus SVG/tile preview for initial Studio.
- Table auto-width premeasurement versus fixed-track streaming.
- Compact break ledger storage.
- Page continuation signatures for suffix convergence.
- Text shaping cache boundaries.
- Source ID insertion and minimal patch behavior.
- Candidate revision storage and semantic three-way merge.
- Desktop shell choice after the `paperd` protocol is proven.

## 26. Definition of success

The project is successful when:

- All automatic frontends share one planner.
- The painter performs no measurement or pagination.
- `.paper` is readable and produces small Git diffs.
- A human can select any pixel and reach source, data, style, semantics, and
  layout cause.
- An agent can make a scoped change without rewriting the document.
- The agent receives deterministic page/crop evidence plus structured context.
- Every break and overflow is explainable.
- Preview and serialized PDF agree within defined rendering tolerances.
- Normal edits update affected pages interactively.
- Typed and HTML compatibility tests pass through adapters.
- Accessibility and compliance are visible during authoring.
- Production rendering is faster and allocates less than the current HTML path.
- Legacy automatic layout implementations can be deleted without removing
  current public entry points.

The durable product is not merely a file format or editor. It is one semantic
document system shared by human-readable source, exact visual editing,
deterministic layout, PDF production, validation, and agent tools.
