# Contributing

PaperRune uses the Go version declared in `go.mod`; tool dependencies are pinned
separately in `tools/go.mod`.

Before submitting a change, run:

```sh
make check
make modules
make coverage-check
make lint
make nilaway
make gosec
make race
make govulncheck
```

Changes to PDF parsing, importing, inspection, CDR, signing, encryption, or
binary template decoding need malformed-input regressions. Preserve minimized
fuzz inputs in the relevant `testdata/fuzz` corpus when they expose a distinct
failure mode. Security boundaries must return bounded, classifiable errors;
they must not panic or silently reinterpret unsupported syntax.

Follow the ownership and public-surface rules in `ARCHITECTURE.md`. Prefer a
small private helper over a new exported alias or wrapper. Public removals and
package moves require a planned breaking release and migration notes.

The PDFs under `assets/generated/pdf` are checked-in visual/reference artifacts,
not disposable build output. `make clean` must leave tracked files untouched.
Compliance fixtures are generated under ignored `artifacts/` paths.
