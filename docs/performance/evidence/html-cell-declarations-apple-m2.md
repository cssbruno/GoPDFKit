# Compiled HTML and plan-validation allocation profile

Date: 2026-07-19  
Host: Apple M2, darwin/arm64, 8 logical CPUs  
Toolchain: Go 1.26.5  
Base revision: `2c199f99cbf3505aac0eff417e8b214f9bc0aba7` plus the current
uncommitted worktree

This is local engineering evidence, not a portable release baseline. Timing
acceptance remains owned by the calibrated multi-sample gates and the named
release review.

## Findings and changes

The compiled-HTML large-table profile showed that every styled cell converted
an already parsed declaration map back into sorted CSS text, parsed that text
again, and allocated a fresh allowed-property map. The lowering path now passes
the resolved declaration map directly to the strict cell-style applicator.
A closed switch still rejects unknown properties and empty values, preserving
the former strict boundary without rebuilding parser state for every cell.

The resolved-declaration validator also rebuilt and discarded a combined CSS
property allowlist for every element. It now checks the existing immutable
property sets directly, retaining the same tag-specific property rules without
allocating a temporary map.

Finally, valid plan checks eagerly formatted indexed error paths such as
`semantic_nodes[123]`, `grid_tracks[4]`, and `glyph_runs[90]` for every item.
Those paths are now formatted only when the associated validation fails. A
shared indexed-error formatter preserves the exact diagnostic text and is used
by the fragment, page, line, glyph-run, break, grid, page-region, semantic-node,
semantic-association, and reading-order validators.

These changes delete redundant parsing and successful-path diagnostic work.
They do not bypass resolved-style validation, immutable-plan validation, PDF
resource preflight, or canonical plan serialization.

## Allocation comparison

The same benchmark and 50-iteration, `memprofilerate=1` profile settings were
used before and after the change:

```text
go test ./document -run '^$' \
  -bench '^BenchmarkGenerationHTMLLargeTableCompiled$' \
  -benchtime=50x -count=1 -memprofile=alloc.pprof -memprofilerate=1
```

| Measure | Before | After | Change |
| --- | ---: | ---: | ---: |
| Sampled allocation over 50 iterations | 5,284.06 MB | 4,927.16 MB | -356.90 MB (-6.75%) |
| Approximate sampled allocation per operation | 105.68 MB | 98.54 MB | -7.14 MB |
| Sampled allocation objects over 50 iterations | 14,048,229 | 10,358,250 | -3,689,979 (-26.27%) |
| Benchmark allocations per operation | 335,279 | 210,657 median | -124,622 (-37.17%) |
| Benchmark bytes per operation | 108,451,917 | 101,154,481 median | -7,297,436 (-6.73%) |
| Deterministic PDF size | 635,097 bytes | 635,097 bytes | unchanged |

The before allocation profile attributed 100.75 MB flat and 125.42 MB
cumulative to `htmlPlanApplyStrictCellStyle`. After the change the function is
absent from the top 25 allocation nodes. The temporary resolved-property
allowlist formerly contributed another 167.77 MB and 173,553 sampled objects;
it is also absent from the final profile. `fmt.Sprintf` fell from 2,987,806 to
369,362 sampled objects as successful validation stopped constructing error
paths. The final ten-sample benchmark median was approximately 50.28 ms/op,
but no timing improvement is claimed against the single initial profile sample.

Profile SHA-256 values:

- before allocation profile:
  `8aa781e513f8422e28658c3b208acd71005f85d79ac9f292367616b1fac31c00`;
- after allocation profile:
  `21f2e203b4e04ac61ab61a9cec34a98bf74ab3ec7ba165f870b1544f9a5ce9eb`.

The raw after profile is temporary local evidence; raw profiles remain ignored
because their samples and binary addresses are host-specific.

## Wider profile review

The representative planner, retained-plan painter, and typed end-to-end CPU
and allocation profiles pass `make profile-paper-engine-check`. The complete
Paper Engine budget passes all 11 calibrated cohorts, the generation-core
budget passes all 60 rows, and the Paper Studio native and browser/WASM latency
budgets pass.

The lazy diagnostic formatting also benefits non-HTML validation. In the
repeated calibrated gate, planner allocations fell from roughly 14,093 to
11,020 per operation, retained-plan painter allocations from 2,964 to 2,443,
typed end-to-end allocations from roughly 17,200 to 13,607, and compiled-HTML
end-to-end allocations from roughly 21,154 to 16,357. The generation-core
large-table cohort records 210,484–210,496 allocations per operation and the
wide-table cohort 102,752–102,754; all remain inside their calibrated budgets.

The generic compiled-HTML profile found no application blocking bottleneck.
The block profile is entirely benchmark-harness channel receive. The mutex
profile records 68.33 ms across the run, 97.96% in the Go runtime, with roughly
1 ms cumulative in the HTML write path. A 5.5 MB runtime trace was produced
(SHA-256
`a16368296755f71a872b26ea18fdd446148e5cedc53adc9d9033bb375f58dbc2`),
but this Go distribution does not include the `go tool trace` viewer.

The remaining largest allocation nodes are `LayoutPlan.Validate`, immutable
slice cloning, canonical JSON serialization, resource preflight, and compact
provenance construction. Those enforce plan immutability, validation,
reproducibility, or output safety. They are valid future profiling targets, but
removing or caching them without a separately tested invariant-preserving
design would trade correctness for benchmark numbers.

## Verification

- focused `document` and `internal/layoutengine` tests pass, including direct
  rejection of unfiltered or empty cell declarations, the resolved property
  cohorts, and exact indexed diagnostic formatting;
- the complete Paper Engine calibrated budget passes;
- the generation-core budget passes;
- Paper Studio native and browser/WASM latency budgets pass;
- output byte size is unchanged for the profiled fixture.
