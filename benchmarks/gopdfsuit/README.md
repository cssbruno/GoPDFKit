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
- `text_short`
- `text_240_lines`
- `invoice_40_rows`
- `png_table_180_rows`
- `png_rows_60`

Each workload is run in `single` and `workers_40` modes. The harness reports
`workers`, `pdf_bytes`, `pdf/s`, and `total_MB` custom metrics in addition to
the standard Go benchmark timing and allocation metrics.

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

## Local Snapshot

Run on `12th Gen Intel(R) Core(TM) i7-12700` with 20 logical CPUs. Rows are
medians from `make bench-gopdfsuit-ci`.

| Workload | Library | Mode | ns/PDF | PDF/sec | Memory/PDF | Allocs/PDF | Output size |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| text_short | GoPDFKit | single | 145,648 | 6,866 | 93,342 B | 244 | 1,505 B |
| text_short | gopdflib | single | 66,812 | 14,967 | 164,192 B | 127 | 4,303 B |
| text_short | GoPDFKit | workers_40 | 27,722 | 36,072 | 56,503 B | 243 | 1,505 B |
| text_short | gopdflib | workers_40 | 8,790 | 113,761 | 111,574 B | 126 | 4,303 B |
| text_240_lines | GoPDFKit | single | 483,609 | 2,068 | 549,983 B | 635 | 4,706 B |
| text_240_lines | gopdflib | single | 676,105 | 1,479 | 2,404,057 B | 840 | 13,074 B |
| text_240_lines | GoPDFKit | workers_40 | 104,211 | 9,596 | 402,145 B | 628 | 4,706 B |
| text_240_lines | gopdflib | workers_40 | 97,332 | 10,274 | 1,060,076 B | 812 | 13,074 B |
| table_180_rows | GoPDFKit | single | 1,231,625 | 812 | 638,074 B | 941 | 8,043 B |
| table_180_rows | gopdflib | single | 639,289 | 1,564 | 1,702,625 B | 961 | 22,885 B |
| table_180_rows | GoPDFKit | workers_40 | 93,979 | 10,641 | 515,462 B | 935 | 8,043 B |
| table_180_rows | gopdflib | workers_40 | 127,675 | 7,832 | 709,387 B | 941 | 22,885 B |
| table_900_rows | GoPDFKit | single | 2,469,137 | 405 | 2,570,611 B | 3,773 | 34,997 B |
| table_900_rows | gopdflib | single | 2,025,632 | 494 | 3,225,596 B | 4,253 | 97,915 B |
| table_900_rows | GoPDFKit | workers_40 | 437,130 | 2,288 | 2,230,411 B | 3,754 | 34,997 B |
| table_900_rows | gopdflib | workers_40 | 532,450 | 1,878 | 3,140,435 B | 4,248 | 97,915 B |
| invoice_40_rows | GoPDFKit | single | 203,854 | 4,905 | 195,076 B | 405 | 3,232 B |
| invoice_40_rows | gopdflib | single | 255,818 | 3,909 | 487,131 B | 309 | 8,633 B |
| invoice_40_rows | GoPDFKit | workers_40 | 38,844 | 25,744 | 143,570 B | 403 | 3,232 B |
| invoice_40_rows | gopdflib | workers_40 | 40,462 | 24,714 | 218,851 B | 303 | 8,633 B |
| png_table_180_rows | GoPDFKit | single | 985,687 | 1,015 | 963,882 B | 1,120 | 15,784 B |
| png_table_180_rows | gopdflib | single | 900,476 | 1,111 | 1,774,978 B | 986 | 28,500 B |
| png_table_180_rows | GoPDFKit | workers_40 | 130,782 | 7,646 | 794,895 B | 1,110 | 15,784 B |
| png_table_180_rows | gopdflib | workers_40 | 158,873 | 6,294 | 715,035 B | 963 | 28,500 B |
| png_rows_60 | GoPDFKit | single | 1,396,648 | 716 | 1,047,804 B | 2,347 | 32,082 B |
| png_rows_60 | gopdflib | single | 1,785,107 | 560 | 4,293,184 B | 1,075 | 322,532 B |
| png_rows_60 | GoPDFKit | workers_40 | 185,016 | 5,405 | 833,812 B | 2,336 | 32,082 B |
| png_rows_60 | gopdflib | workers_40 | 193,189 | 5,176 | 1,999,991 B | 1,028 | 322,532 B |
