# Architecture

GoPDFKit has one high-level facade: `document.Document`. New applications
construct it through `document.NewDocument` or `document.MustNew`. The module
root intentionally contains no facade package.

## Package boundaries

- `document` owns PDF construction and the compatibility facade.
- `layout` owns renderer-independent public document models and measurement.
- `importpdf` owns the bounded classic-xref parser and imported-page model.
- `inspect` and `pdfcdr` consume `importpdf`; inspection reports source
  structure, while CDR creates a new document from a constrained safe subset.
- `sign` owns classic-xref incremental signing and CMS verification, and
  `font` owns standalone font-definition generation.
- `internal/layoutgeom` owns pure geometry shared by typed and HTML renderers.

Lower-level packages must not import `document`. This keeps the dependency graph
acyclic and lets them be used without the high-level renderer.

## Document ownership

`Document` preserves its public FPDF-style methods, but private state has
concrete owners:

- `pdfSerializationState` allocates and records PDF object numbers.
- `resourceOwnershipState` initializes the document resource registry.
- `resourceStore` owns fonts, images, templates, and imported resources.
- `resourceObjectNumbers` owns resource-specific PDF object references.
- `attachmentResourceStore` owns attachment object references and compressed
  temporary data.

New code should add behavior to the owning private type. Do not add another
field directly to `Document` when an existing owner is responsible for its
lifetime.

## Rendering and concurrency

A `Document` is a mutable, single-owner build session and is not safe for
concurrent calls. Create one document per independently generated PDF.
`CompiledHTML`, `ImageCache`, and `FontCache` are the reusable cross-document
inputs and carry their own concurrency guarantees.

An opened `importpdf.Source` is immutable after parsing. Concurrent page
resolution is supported and serialized around its bounded object cache;
`PageRef` also coordinates lazy content decoding so cancellation does not
poison later retries. Package-level inspection, CDR, and signing operations do
not share mutable document state between calls.

Each compiled HTML render creates a private render session that owns its style,
element, and list stacks. Render-local state must not be added to `CompiledHTML`
or retained by `HTML` after the call completes.

## Layout invariants

Typed layout measurement and rendering must consume the same geometry rules.
HTML may retain CSS-specific parsing and styling, but track offsets, spans,
column constraints, image fitting, and pagination comparisons belong in pure
shared geometry. Any change to those rules requires typed-versus-HTML parity
coverage.

Public layout fields are behavioral contracts. A field must not be added until
measurement, rendering, pagination, and regression tests implement it.

GoPDFKit is migrating automatic layout to one private planner shared by typed,
HTML, and future human-readable frontends. During the migration, new automatic
layout behavior belongs in the unified planner rather than a third renderer.
Frontends resolve syntax-specific rules and lower content; the final painter
must consume positioned plan commands without measuring, wrapping, or
paginating. See [ADR 0001](docs/adr/0001-unified-automatic-layout-engine.md).

Plan font resources are immutable identities, not live `Document` lookups.
Standard 14 resources pin canonical metrics. Embedded UTF-8 resources pin the
font-program digest, byte length, and exact planning metrics in the canonical
plan while keeping the verified bytes in a private content-addressed sidecar.
The painter parses and verifies every embedded program during bounded
preflight, before opening the target, then reuses the established TrueType
subset and ToUnicode serializer. This preserves core-font compatibility and
allows one plan to be replayed concurrently without ambient filesystem or font
catalog access.

## Plan preview and Studio boundary

The immutable layout plan is the authority for both PDF painting and visual
inspection. `document.PaperPlan` exposes bounded page SVG, geometry, hit-test,
query, and explain projections without exposing the evolving private IR. Paper
Studio consumes those projections; browser CSS may arrange workspace chrome
but must never substitute for page measurement, wrapping, positioning, or
pagination.

Every Studio page, overlay, hit, and explanation request is bound to the exact
plan revision. A revision mismatch fails instead of mixing evidence, and the
canvas is visibly stale and non-interactive while a replacement is loading.
The page inspector is another bounded retained-plan projection: border/content
rectangles, fragment region membership, causal breaks, semantic roles, and
reading indexes are plan facts. Studio does not synthesize unavailable margin,
padding, baseline, font-fallback, or final-PDF verification evidence. Overlap
selection follows the deterministic reverse fragment order returned by the
plan hit-test contract.
The development Studio server accepts only explicit loopback hosts because it
serves local source and plan evidence without a remote authentication boundary.
Scenario snapshots and page artifacts are immutable, bounded, and discarded
when the source digest changes.

On a source transition Studio retains at most one detached previous
`PaperPlan`; it never retains that revision's source or AST. Per-page hashes
cover exact page geometry, display payloads, breaks, positioned diagnostics,
semantics, reading order, and provenance. The rail compares those hashes only
when the previous and current scenario identities match. Otherwise it labels
the baseline mismatch and emits no changed-page evidence. Master labels are
the retained first/even/odd selector state plus actual fragment regions and
repetition state; Studio does not invent an authored master identity absent
from the plan.

## Agent transport boundary

`internal/paperd.ProtocolServer` owns authenticated envelopes, version
negotiation, replay rejection, capability-filtered dispatch, and redacted
responses. Concrete transports may only add stricter boundaries; they do not
deserialize workspace handles or bypass the dispatcher.

The Unix-domain adapter uses a length-prefixed, one-request connection with
bounded concurrency and deadlines. It refuses existing paths, requires a
non-group/world-writable parent, creates the endpoint as `0600`, and verifies
Linux `SO_PEERCRED` or macOS `LOCAL_PEERCRED`/`LOCAL_PEERPID` against an
explicit UID allowlist before reading the envelope. Platforms without a proven
peer-credential implementation fail closed. A TCP or web adapter would require
a separately reviewed mutually authenticated TLS identity boundary; loopback
location alone is not authority.
The matching Unix client applies the same restricted-path checks and verifies
the kernel-reported server UID before transmitting an authenticated envelope;
filesystem ownership or loopback location alone is never treated as server
identity.

## Performance workflow

Benchmarks, regression budgets, `benchstat` comparisons, pprof profiles, and
runtime traces are local developer tools; they do not run in CI. Optimize from
profiles, compare repeated samples against a named baseline, and keep behavior
coverage with the optimization. See the performance section in `README.md` for
the supported Make targets.

## Public API policy

The `document` surface is compatibility-sensitive and intentionally frozen.
Prefer private helpers or focused capability packages over new aliases and
wrapper combinations. Public removals and package moves require a planned
breaking release and a migration guide.
