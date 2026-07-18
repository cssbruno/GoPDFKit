# Display raster v5 performance evidence

Date: 2026-07-18  
Candidate source: `ed1ee08` plus `27eba1c` (`darwin/arm64`, Apple M2,
Go 1.26.5)  
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
GOPDFKIT_RASTER_BENCH_LEGACY=1 sh tools/run-benchmark.sh artifacts/display-raster-legacy.txt go test ./internal/layoutengine -run '^$' -bench '^BenchmarkDisplayRasterPNGEncode$' -benchmem -benchtime=250ms -count=10
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
| passing WASM latency report | `94db71747077177fe6dd553fc2f5dfff2a1491210d64e9fc860abc4b6ecf7802` |

The final ten-sample Paper Studio browser/WASM budget also passes on renderer
`layoutengine/go-display-raster@5`. The candidate serves one deterministic
gzip module, initializes and paints in a Worker, transfers the resulting
`ImageBitmap`, debounces zoom rendering, and retains at most six decoded page
bitmaps. Cold workspace is 47.370 ms, WASM initialization is 94.707 ms, first
visible page is 90.186 ms, and warm visible-update p95 is 33.583 ms against the
100 ms budget. That final p95 is 61.3% below the earlier validated 86.744 ms
candidate measurement. Change notification is 254.237 ms and incremental
workspace refresh is 1.874 ms. Two attempted shared paint-state caches had
produced reproducible 109–121 ms p95 failures and remain removed; the native
zlib pool and digest-validated display-resource cache are retained. The
generated report is `artifacts/paper-studio-wasm-latency.json`.
