# v0.9 Benchmark Budgets

These budgets are release gates for regression detection, not public performance
promises. They are intentionally looser than the current local benchmark medians
so CI can catch large regressions without failing on ordinary machine variance.

Baseline machine for the current README snapshot: Apple M2, 8 logical CPUs.

| Workload | Command / Benchmark Family | Budget |
| --- | --- | --- |
| 100-page text/table style generation | `make bench-generation-core-ci` text rows | <= 2,500 allocs/PDF and <= 600 KiB/PDF |
| Long text generation | `make bench-generation-core-ci` long text rows | <= 500 allocs/PDF and <= 150 KiB/PDF |
| 100-image or image-heavy generation | image rows | <= 2 MiB/PDF for uncached images; <= 150 KiB/PDF for cached images |
| Large HTML table render | compiled HTML large/wide table benchmarks | <= 8 ms/op and <= 8 MiB/op for large table; <= 3 ms/op and <= 3 MiB/op for wide table |
| Imported PDF pages | imported PDF benchmark rows | <= 500 allocs/PDF and <= 100 KiB/PDF |
| Attachments | attachment benchmark rows | <= 400 allocs/PDF and <= 250 KiB/PDF |
| Signed output | signed baseline benchmark rows | <= 2,000 allocs/PDF and <= 1.5 MiB/PDF |

Before `v1.0.0`, these should move from documented manual gates to a CI
comparison script that records the baseline, variance window, and failure
thresholds.
