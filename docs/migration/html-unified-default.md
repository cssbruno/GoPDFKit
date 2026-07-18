# HTML unified migration

`HTML.Write`, `HTML.WriteContext`, `HTML.WriteCompiled`, `HTML.WriteTemplate`,
and `HTML.WriteTemplateContext` retain their public signatures. Supported
fragments lower to one immutable HTML-to-IR plan and are painted through the
unified display-list or core painter.

Planning and resource preflight complete before PDF page bytes are mutated.
Cancellation, unsafe links, row limits, malformed recovery, unsupported styles,
and other rejected contracts return an error without partial output. There is
no automatic compatibility renderer, hidden pagination retry, or mixed-engine
fragment.

Use `PlanCompiledHTML` or `PlanCompiledHTMLContext` when an application needs
an explicit accept/reject decision before writing. Applications should keep
HTML within the documented strict cohort; forms, browser-only CSS, malformed
recovery, custom page lifecycle behavior, and unsupported SVG/table contracts
must be rewritten as supported HTML/layout content or handled by explicit
manual drawing.

`Hooks.OnLayoutEngineRoute` remains available as a bounded observability hook.
Supported public HTML writes report `unified`; rejected writes report no
successful engine route. Categories contain no authored text, URLs, template
values, filenames, or source snippets.

## Release contract

The deletion release requires the accepted corpus and named platform baselines
to pass compatibility, cursor, extracted-text, link, semantic, raster,
benchmark, race, security, PDF/A, and PDF/UA checks. It also requires a static
repository search showing that automatic layout, measurement, wrapping, and
pagination remain confined to the planner/painter implementation.

Rollback criteria are a newly introduced supported-corpus failure, semantic or
visual drift without approval, race failure, or a breached calibrated budget.
Those criteria are release governance; they are not a caller-visible engine
switch.
