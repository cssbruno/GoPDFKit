# ADR 0002: Decorated container fragmentation

## Status

Accepted.

## Decision

Decorated typed containers are lowered into finalized group fragments by the
unified planner. Background and left/right borders repeat on every continuation
fragment. Top margin, padding, and border occur only on the first fragment;
bottom margin, padding, and border occur only on the last. Internal page
boundaries add no vertical margin. A whole fragment uses every edge.

Pagination policy remains independent from paint. Preferred keeps use the
existing bounded relaxation and `KEEP_TOO_LARGE` evidence. The painter receives
only immutable positioned fragments and display commands and never measures or
reconstructs a box.

Signature envelopes do not gain a new public box property. Each authored
signature row retains its own box, while the existing envelope grouping and
keep policy govern row placement.

## Consequences

The rule is deterministic for preview, capture, raster, and PDF output. It also
keeps vertical margin from appearing at artificial page boundaries. Nested
visual child boxes remain outside this initial exact cohort because combining
independently painted box layers needs an explicit stacking contract.
