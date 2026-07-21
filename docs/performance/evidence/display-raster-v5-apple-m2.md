# Display raster v5 performance evidence

Date: 2026-07-18  
Candidate: `c2b1ba9` (`darwin/arm64`, Apple M2, Go 1.26.5)
Workload: deterministic 480×320 mostly-white document page with text-like
rules, using zlib BestSpeed in both cohorts.

The legacy cohort uses Go's standard PNG encoder with its five-filter scanline
search. The candidate uses the production filter-none encoder and pools
resettable zlib writers on native targets. JS/WASM deliberately uses a fresh
writer because measured `Reset` cost regressed its latency budget. Both are selected
inside `BenchmarkDisplayRasterPNGEncode`, so benchmark names and fixture bytes
are identical.

Commands:

```text
PAPERRUNE_RASTER_BENCH_LEGACY=1 sh tools/run-benchmark.sh artifacts/display-raster-legacy.txt go test ./internal/layoutengine -run '^$' -bench '^BenchmarkDisplayRasterPNGEncode$' -benchmem -benchtime=250ms -count=10
sh tools/run-benchmark.sh artifacts/display-raster-candidate.txt go test ./internal/layoutengine -run '^$' -bench '^BenchmarkDisplayRasterPNGEncode$' -benchmem -benchtime=250ms -count=10
tools/bin/benchstat artifacts/display-raster-legacy.txt artifacts/display-raster-candidate.txt
```

`benchstat` result:

| Metric | Legacy | Candidate | Change |
| --- | ---: | ---: | ---: |
| time | 2.853 ms | 1.192 ms | -58.21%, p=0.000, n=10 |
| throughput | 205.5 MiB/s | 491.4 MiB/s | +139.18%, p=0.000 |
| bytes/op | 1,222.70 KiB | 17.34 KiB | -98.58%, p=0.000 |
| allocations/op | 34 | 19 | -44.12%, p=0.000 |

The host was noisy (legacy/candidate time confidence intervals ±28% and ±12%),
but the paired non-parametric result is significant and is corroborated by the
separate end-to-end WASM cohort below. Allocation profiles show that resetting
a pooled native zlib writer removes repeated flate table construction. Both
allocation count and total bytes fall, so no allocation regression is accepted.

Raw evidence hashes:

| Artifact | SHA-256 |
| --- | --- |
| legacy benchmark | `9196300eccfe5f08b667c2ea1af18caa83274856bc085ce1a1e0a7c55c1a85fa` |
| candidate benchmark | `9f3625e5d169bd15ee8a51184118feb88503be739d4e23741207d7e057e701e6` |
| legacy allocation profile | `b0200221271fb02faeb5c90751abb5ab5cbf2e2a2ce32593640deddd0837a410` |
| candidate allocation profile | `657f0bb1f1b52047ceb63dd7f6ee8b35ed2e4c4460846ffd59d29e7f1f1b279a` |
| passing WASM latency report | `cc8e7b8ec84982be3466bae69f5ed72ef59077b4b0968679a21164690c9044d5` |

The ten-sample Paper Studio Node/WASM budget also passes on renderer
`layoutengine/go-display-raster@5`. Relative to the pinned v2 baseline, first
visible page time remains within budget at 143.810 ms. Warm visible-update p95
is 86.744 ms: slower than the old 58.642 ms sample but below the calibrated
100 ms budget. A preceding run under host contention missed at 106.622 ms, so
the evidence does not claim a WASM speedup. Two attempted shared paint-state
caches produced reproducible 109–121 ms p95 failures and were removed. The
retained cache reuses only digest-validated decoded resources for the same plan
identity. All cold, initialization, notification, and incremental-workspace
budgets pass. The generated report is
`artifacts/paper-studio-wasm-latency.json`.
