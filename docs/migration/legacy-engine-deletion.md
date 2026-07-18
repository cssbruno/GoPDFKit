# Legacy automatic-layout deletion

The legacy automatic-layout production files have been removed. The public
typed and HTML entry points remain as lowering adapters over the unified
planner and painter.

## Current contract

Typed callers should use `PlanLayoutDocument` and
`WriteLayoutDocumentPlan` when they need an explicit compatibility check.
`WriteDocument` accepts a fresh receiver and declarative `PageTemplate` model;
unsupported receiver state or model contracts store an error without opening
pages or retrying through a legacy renderer.

HTML callers should keep fragments inside the strict unified cohort. Forms,
browser-only CSS, malformed recovery, unsupported styles, and other rejected
contracts must be rewritten as supported typed/layout content or handled by
explicit manual drawing. `PlanCompiledHTML` is the preflight API for callers
that need a hard accept/reject decision.

The public entry points remain source-compatible:
`Document.WriteDocument`, `HTML.Write`, `HTML.WriteContext`,
`HTML.WriteCompiled`, `HTML.WriteTemplate`, and `HTML.WriteTemplateContext`.
Void methods preserve the receiver-error model; context-returning methods
return the planning error. No public or private automatic engine switch remains.

## Deletion evidence

The deletion implementation is covered by the typed and HTML cutover tests,
golden and characterization baselines, public-adapter tests, route-hook tests,
and the repository static search used during the Stage 10 exit review. The
measurement-only `layout` APIs and the removed direct HTML renderer files are
not part of the production surface.

Release closure still requires the accepted stabilization-window record and
the named platform performance/compliance evidence. Those are external release
artifacts and must not be inferred from a local test run. Use the
[`stabilization-window-record.md`](stabilization-window-record.md) template to
capture the accepted corpus, thresholds, platform evidence, rollback decision,
and approvals.
