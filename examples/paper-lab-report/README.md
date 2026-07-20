# Data-driven Brazilian lab report

From the repository root, validate and render ordinary JSON data:

The source declares reusable `Patient` and `LabResult` objects, then uses them
directly as `Patient patient` and `list LabResult results:` schema fields.

```sh
go run ./cmd/paper check \
  --assets examples/paper-lab-report/assets.json \
  --data examples/paper-lab-report/example.json \
  examples/paper-lab-report/lab-report.paper

go run ./cmd/paper render \
  --assets examples/paper-lab-report/assets.json \
  --data examples/paper-lab-report/example.json \
  -o /tmp/lab-report.pdf \
  examples/paper-lab-report/lab-report.paper
```

Generate reproducible structural and layout edge cases. Every case is
schema-validated, planned, painted, and checked for a complete PDF:

```sh
go run ./cmd/paper check \
  --assets examples/paper-lab-report/assets.json \
  --edge-cases 12 --seed 42 \
  --edge-output /tmp/lab-report-edge-cases \
  --edge-visual \
  examples/paper-lab-report/lab-report.paper
```

Open `/tmp/lab-report-edge-cases/edge-visual-review.pdf` to inspect every page
as rasterized from the final PDF. The directory also contains one PNG per PDF
page. Read `edge-report.json` from tests or other tools to inspect
empty/whitespace/multiline values, longest-string and largest-list JSON
Pointers, deterministic input/PDF hashes, independent PDF page counts,
per-page extracted-text sizes and hashes, raster hashes and dimensions, and
positioned layout issues.

Use repeatable `--edge-input FILE` options for real laboratory payloads, and
`--edge-baseline edge-report.json` to reject output regressions. By default any
positioned layout issue fails the command; the page-issue, extracted-text, and
page-count thresholds can be changed explicitly when a paper has a documented
acceptance policy.

Repeat the same seed to reproduce a failure exactly. The command exits nonzero
when any generated case exposes a layout or PDF problem; generated inputs and
successful PDFs remain available under `--edge-output`.
