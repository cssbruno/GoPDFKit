# Typed `WriteDocument` unified-default migration

`Document.WriteDocument` keeps its existing signature and receiver-error model,
but fresh supported `layout.LayoutDocument` values now run through the immutable
Paper Engine plan and painter by default. Planning finishes before the first PDF
page is opened; invalid constraints and resource-limit failures therefore cannot
leave a partially rendered document.

Applications do not need to change ordinary calls. Code that configures a custom
page lifecycle, writes content before `WriteDocument`, or uses a typed contract
not yet represented by the exact planner is rendered by the private legacy
engine as one whole document. The two engines are never mixed within a model.

Production callers can set `Hooks.OnLayoutEngineRoute` to aggregate migration
coverage. The callback receives `WriteDocument`, either `unified` or `legacy`,
and a bounded category such as `document-policy`, `page-template`, or
`unsupported-layout-contract`. Categories never include document text, source
paths, or other authored values.

Rollback criteria are an increase in PDF generation errors, page-count or
extracted-text drift in the characterization corpus, a race failure, or a breach
of the calibrated time/allocation budgets. A caller needing custom mutable page
lifecycle behavior can retain that behavior during the compatibility window;
the fallback event makes the remaining migration work measurable.
