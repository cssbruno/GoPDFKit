# Data-driven Brazilian lab report

From the repository root, validate and render ordinary JSON data:

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
  examples/paper-lab-report/lab-report.paper
```

Repeat the same seed to reproduce a failure exactly. The command exits nonzero
when any generated case exposes a layout or PDF problem; generated inputs and
successful PDFs remain available under `--edge-output`.
