# Architecture

GoPDFKit has one high-level facade: `document.Document`. New applications
construct it through `document.NewDocument` or `document.MustNew`. The module
root intentionally contains no facade package.

## Package boundaries

- `document` owns PDF construction and the compatibility facade.
- `layout` owns renderer-independent public document models and measurement.
- `importpdf`, `inspect`, `sign`, and `font` remain independent capabilities.
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
