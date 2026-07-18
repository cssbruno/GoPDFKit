# Typed `WriteDocument` unified migration

`Document.WriteDocument` keeps its existing signature and receiver-error model.
Fresh supported `layout.LayoutDocument` values are lowered to one immutable
Paper Engine plan before the PDF page is opened, then painted by the unified
painter.

Unsupported receiver state, custom mutable page lifecycle callbacks, invalid
model contracts, and resource-limit failures store an error on the receiver
without opening pages or retrying through a legacy renderer. Applications that
need a hard accept/reject decision can call `PlanLayoutDocument` followed by
`WriteLayoutDocumentPlan`.

`Hooks.OnLayoutEngineRoute` remains available for bounded observability.
Successful automatic typed writes report `WriteDocument`/`unified`; rejected
writes do not invoke a compatibility engine. Categories never include document
text, source paths, or other authored values.

The deletion release requires the accepted typed corpus to pass deterministic
page-count, cursor, extracted-text, link, semantic, raster, benchmark, race,
security, PDF/A, and PDF/UA checks, plus the repository static searches that
prove automatic layout is confined to the unified planner and painter.
