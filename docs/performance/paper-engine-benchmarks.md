# Paper Engine performance evidence

Reproduce the bounded typed and HTML compatibility projections—including
fixture outcomes, page counts, extracted page text, semantic reading roles,
PDF structure counts, hashes, command, and runtime fingerprint—with:

```sh
make characterize-paper-engine
```

The reports are written to `artifacts/characterization/typed.json` and
`artifacts/characterization/html.json`.

The Paper Engine suite separates the unified automatic-layout pipeline into
comparable stages over a deterministic 48-paragraph fixture:

- `BenchmarkPaperEnginePlannerTyped` measures typed lowering, measurement,
  wrapping, pagination, display-list construction, and plan hashing.
- `BenchmarkPaperEnginePainterTyped` reuses one immutable plan and measures
  production preflight plus positioned-command replay. It does not serialize
  the final PDF.
- The three `BenchmarkPaperEngineEndToEnd*` cases measure planning, painting,
  and deterministic PDF serialization from typed, reusable compiled HTML, and
  `.paper` inputs. The `.paper` case includes parsing and semantic compilation;
  compiled HTML tokenization is intentionally outside its retained-template
  case.
- `BenchmarkPaperEngineWarmCompiledPaper` retains one immutable `.paper` plan
  outside the timer and measures only fresh-target painting plus deterministic
  PDF serialization. Parsing, semantic compilation, measurement, wrapping,
  and pagination are excluded from this Stage 6 warm path.
- `BenchmarkPaperEngineConcurrentPlanWrite16` measures synchronized batches
  from exactly sixteen persistent workers reading one immutable retained plan
  and writing independent documents.
- `BenchmarkPaperEngineTableLarge128` and
  `BenchmarkPaperEngineTableWide32` expose typed measurement, occupancy, and
  pagination costs for large and wide fixed-track tables. The 128x4 fixture
  uses the normal 220-point page; the 8x32 fixture uses a deterministic
  640-point wide page so every 19.125-point track satisfies the planner's exact
  unbreakable-word intrinsic minimum instead of benchmarking a rejected
  document. The large fixture also exercises plan-local reuse of font metric
  contexts across same-style cells.
- `BenchmarkPaperEngineTableRows10000` isolates the unified table kernel's
  premeasured 10,000-row vertical occupancy and pagination cost. It is kept
  separate from the typed cohorts so text measurement does not hide the table
  kernel's scaling behavior.
- `BenchmarkHTMLUnifiedNestedFlexPlanning` isolates selector-free HTML
  lowering plus bounded recursive flex measurement/composition. Its checked
  Apple M2 characterization baseline is
  `docs/performance/baselines/html-flex-nested-apple-m2.txt`; it is deliberately
  separate from the Stage 0 release budget until the broader HTML cutover
  calibration is approved.
- `BenchmarkHTMLUnifiedDefaultWriteCompiled` measures the public default
  compiled-HTML route through lowering, planning, whole-fragment painting, and
  deterministic PDF serialization. Its initial three-sample Apple M2
  characterization is
  `docs/performance/baselines/html-unified-default-apple-m2.txt`; the migration
  guide defines how later calibrated samples participate in the cutover error
  budget.
- `BenchmarkHTMLUnifiedImagePlanning` isolates resolved HTML image/figure
  lowering, bounded source snapshotting, intrinsic sizing, image decode,
  display-list construction, caption semantics, and plan hashing. Its checked
  Apple M2 characterization baseline is
  `docs/performance/baselines/html-image-figure-apple-m2.txt`; like the flex
  baseline, it is characterization evidence rather than a release budget.
- `BenchmarkHTMLUnifiedStructuredTablePlanning` isolates selector resolution,
  fixed/percentage/intrinsic track lowering, structured cell expansion,
  intrinsic and constrained cell measurement, table pagination, image
  resources, semantics, and plan hashing. Its checked Apple M2 characterization
  baseline is `docs/performance/baselines/html-structured-table-apple-m2.txt`.
- `BenchmarkHTMLUnifiedNestedTableCellPlanning` and
  `BenchmarkHTMLUnifiedTableRichCellPlanning` isolate recursive table grafting,
  semantic/display remapping, wrapped flex/grid-like cell geometry, and
  decorated structural boxes. Their checked three-sample Apple M2 baseline is
  `docs/performance/baselines/html-table-rich-cells-apple-m2.txt`.
- `BenchmarkHTMLUnifiedNestedListWhitespacePlanning` isolates recursive list
  expansion, marker/counter formatting, semantic ownership, tab expansion,
  whitespace-aware wrapping, links, and plan hashing. Its checked three-sample
  Apple M2 characterization is
  `docs/performance/baselines/html-nested-lists-whitespace-apple-m2.txt`.

Run a ten-sample report with its command and environment fingerprint:

```sh
make bench-paper-engine-ci
```

The default output is `artifacts/paper-engine-benchmarks.txt`. It remains valid
Go benchmark text and can be compared with a named baseline:

```sh
tools/bin/benchstat docs/performance/baselines/paper-engine-stage0-apple-m2.txt \
  artifacts/paper-engine-benchmarks.txt
```

Use the same machine, power mode, Go toolchain, fixture, benchmark duration,
and plan-detail mode for timing comparisons. At least ten samples should be
used for release decisions. The checked Stage 0 report is characterization
evidence, not a promise that Apple M2 timings apply to another host.

`make bench-paper-engine-budget` first records ten samples, then validates them
against the named calibration in
`docs/performance/calibrations/apple-m2-go1.26.json`. The validator requires an
exact GOOS, GOARCH, CPU identity, compatible Go patch release, complete report
fingerprint, and every named cohort. It gates the upper median `ns/op` and the
worst observed `B/op` and `allocs/op`; a report from another host fails instead
of applying Apple M2 timings to unrelated hardware. Override the profile only
with another reviewed, checked calibration:

```sh
make bench-paper-engine-budget \
  PAPER_ENGINE_CALIBRATION_PROFILE=docs/performance/calibrations/another-host.json
```

Ordinary unit tests parse fixed report fixtures and never assert live elapsed
time. Release comparisons should still use `benchstat` on the same machine,
power mode, toolchain, fixture, and benchmark duration; the calibrated gate is
the broad regression boundary, not a replacement for statistical review.

## CPU and allocation profiles

Generate bounded CPU and allocation profiles for the representative
planner-only, retained-plan painter, and typed end-to-end workloads:

```sh
make profile-paper-engine-check
```

The command writes raw `pprof` data, bounded 25-node text summaries, and
`artifacts/profiles/paper-engine/report.txt`. The report records the exact
benchmark and reproduction commands, Go version, GOOS, GOARCH, CPU count,
source revision, worktree state, CPU duration, and allocation iteration count.
Raw profiles remain ignored build artifacts because addresses and samples are
specific to the binary, host, and run.

The defaults cap each CPU workload at two benchmark seconds and each allocation
workload at 20 iterations. `PAPER_ENGINE_PROFILE_CPU_SECONDS` accepts 1–30 and
`PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS` accepts 1–100; values outside those
ranges fail before profiling. Use the same host and settings when comparing
hotspots. Profile output is characterization evidence, not a portable timing
budget.
