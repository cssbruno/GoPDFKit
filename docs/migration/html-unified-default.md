# HTML unified-default migration

The documented `HTML.Write`, `HTML.WriteContext`, `HTML.WriteCompiled`,
`HTML.WriteTemplate`, and `HTML.WriteTemplateContext` entry points retain their
public signatures. Supported complete fragments now lower to the immutable
HTML-to-IR plan and no-layout painter by default. `CompiledHTML` and compiled
templates remain immutable and safe for concurrent render-local reuse.

Planning and resource preflight complete before PDF page bytes are mutated.
Cancellation, unsafe links, row limits, and other policy failures return an
error without invoking the compatibility renderer. Existing start-frame
behavior is retained: the active page, margins, cursor, page-break policy, and
selected core-font context determine the fragment body and final cursor.

## Private whole-fragment fallback

An unsupported compatibility contract may use the private legacy renderer for
the complete fragment. A unified prefix is never combined with a legacy island.
Malformed recovery, custom page lifecycle behavior, unsupported styles, and
unsupported layout contracts remain measurable compatibility categories rather
than public engine-selection APIs.

`Hooks.OnLayoutEngineRoute` reports the public entry point, `unified` or
`legacy`, and a bounded category. Current categories include
`malformed-recovery`, `frame-contract`, `svg-contract`, `stylesheet-contract`,
`table-contract`, and `unsupported-layout-contract`. Categories contain no
authored text, URLs, template values, filenames, or source snippets.

Applications should aggregate counts and rates by release, entry point, and
category. They must not treat the hook as a document-content log.

## Initial error budget

A release candidate is outside the HTML cutover error budget if any of these
conditions occurs:

- a safety or policy failure paints bytes or invokes fallback;
- the compatibility corpus changes page count, final cursor, extracted text,
  links, semantics, or reviewed raster output without explicit approval;
- compiled reuse or concurrent rendering fails normal or race testing;
- a previously supported corpus fragment changes from `unified` to `legacy`;
- the measured fallback rate increases by more than one percentage point from
  the previous accepted release on the same corpus; or
- the named default-path benchmark regresses by more than 10% in time or 15%
  in allocations against its calibrated platform baseline without approval.

The repository benchmark is `BenchmarkHTMLUnifiedDefaultWriteCompiled`.
Release evidence must record the Go version, platform, benchmark samples,
corpus identity, route counts, and fallback categories; a single local timing
sample is not release evidence.

## Rollback

The legacy implementation remains private during stabilization. If the error
budget is breached, stop the release and revert the default-routing cutover as
one change while retaining the planner and characterization fixtures for
diagnosis. Do not expose a caller-controlled engine switch, silently widen an
unsupported cohort, or mix engines inside a fragment. Re-enable the unified
default only after the failing corpus case has deterministic semantic and
visual evidence plus normal and race coverage.

This document does not claim that a stabilization release has completed. The
legacy engine can be deleted only after the separately tracked stabilization,
fallback-rate, compatibility, performance, and compliance gates pass.
