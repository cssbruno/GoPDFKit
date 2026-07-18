# Legacy automatic-layout deletion

Status: pre-deletion gate. The unified typed and HTML planners are the default
for supported fresh inputs, but the private legacy renderers remain available
during the stabilization window. This document records the breaking-release
contract that applies only after that window is formally closed.

## Migration before the deletion release

Typed callers should plan and paint through `PlanLayoutDocument` and
`WriteLayoutDocumentPlan` when they need an explicit compatibility check.
Automatic `WriteDocument` calls should use a fresh document with declarative
`PageTemplate` regions. Existing-page writes and arbitrary header, footer, or
page-break callbacks are not immutable planner contracts; migrate those calls
to planned page regions or explicit drawing around the automatic-layout call.

HTML callers should keep fragments inside the documented strict unified cohort.
Use `PlanCompiledHTML` to preflight a fragment when the caller needs a hard
accept/reject decision. Forms, browser-only CSS, malformed recovery, and other
unsupported contracts must be rewritten as supported typed/layout content or
handled by explicit manual drawing before the deletion release.

The public entry points remain source-compatible: `Document.WriteDocument`,
`HTML.Write`, `HTML.WriteContext`, `HTML.WriteCompiled`,
`HTML.WriteTemplate`, and `HTML.WriteTemplateContext` are retained as lowering
adapters. After deletion, an unsupported automatic-layout contract is an
error; it is not retried through a legacy renderer. The receiver-error model
continues to apply to void methods, while context-returning methods return the
same planning error.

## Required release evidence

The deletion release must attach evidence for all of the following on the
accepted corpus and named platform baselines:

- one typed and one HTML stabilization release completed;
- fallback rate at or below the accepted threshold, with no newly supported
  cohort routed to legacy;
- compatibility, cursor, extracted-text, link, semantic, raster, benchmark,
  race, security, PDF/A, and PDF/UA gates passing;
- rollback criteria formally closed; and
- repository searches proving that automatic layout, measurement, wrapping,
  and pagination are confined to the unified planner and painter contracts.

The current migration guides and route-hook tests intentionally do not claim
that this evidence exists. Deleting the legacy files before those gates close
would remove the documented rollback path without proving compatibility.
