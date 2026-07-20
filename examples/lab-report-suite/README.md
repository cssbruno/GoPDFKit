# Brazilian Laboratory Report Suite

This example creates three independent, single-page A4 reports with the
low-level `document.Document` API:

- `lab-bioquimica-report.pdf` - cardiometabolic biochemistry panel with current,
  previous, reference, and visual-status columns.
- `lab-urinalise-report.pdf` - urine type I with physical, chemical, and urinary
  sediment sections.
- `lab-microbiologia-report.pdf` - quantitative urine culture with a qualitative
  S/I/R antibiogram and the current BrCAST category names.

Run the complete suite from the repository root:

```sh
go run ./examples/lab-report-suite
```

The files are written to `assets/generated/pdf/examples`.

Every laboratory name, person, identifier, credential, address, result, and
verification code is fictional. The PDFs are visual and programming examples,
not clinical templates ready for production use. See `examples/README.md` for
the public Brazilian references used to shape the information hierarchy.
