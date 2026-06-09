# GoPDFKit vs gopdfsuit Benchmarks

This directory is a separate Go module for apples-to-apples comparison against
`github.com/chinmay-sawant/gopdfsuit/v5`.

The module pins:

- GoPDFKit: local checkout through `replace github.com/cssbruno/gopdfkit => ../..`
- gopdfsuit: `e61b05028120937d62408ca700d10a41f48e3899`

Run from this directory:

```shell
go test -run '^TestComparableOutputsArePDF$' -bench 'Benchmark(GoPDFKit|GoPDFLib)' -benchmem -count=3
```

The comparable workloads intentionally use features both libraries expose
through public API:

- `table_180_rows`
- `table_900_rows`
- `png_table_180_rows`

Each workload is run in `single` and `workers_40` modes. The harness reports
`workers`, `pdf_bytes`, and `pdf/s` custom metrics in addition to the standard
Go benchmark timing and allocation metrics.

HTML conversion is included as an opt-in benchmark because the implementations
are not equivalent: GoPDFKit renders its supported HTML subset in-process, while
gopdfsuit uses Chrome/Chromium. Enable it only on machines with Chrome:

```shell
GOPDFKIT_COMPARE_HTML=1 go test -run '^$' -bench 'HTML' -benchmem -count=3
```

Compliance workloads are not included in the apples-to-apples table unless both
libraries can be configured with equivalent PDF/A, PDF/UA, Arlington, metadata,
font, signing, and external validation behavior. Document unsupported or
non-equivalent features instead of mixing them into throughput rows.
