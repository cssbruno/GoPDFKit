# Deterministic Output

Deterministic output is intended for golden PDFs, reproducible builds,
compliance fixtures, and CI. It should be exposed as one explicit mode in
`v0.9.0`.

## Target APIs

```go
func WithDeterministicOutput() Option
func DeterministicDefaults() Defaults
func DeterministicPolicy() ProductionPolicy
```

## Contract

Deterministic output should set or require:

* catalog and resource sorting,
* stable attachment ordering,
* stable template ordering,
* stable image/resource identity,
* fixed creation and modification dates,
* stable file IDs where possible,
* stable compression behavior selected by the caller.

Compression does not have to be disabled, but it must be configured explicitly
and produce stable bytes for the same inputs and Go/runtime version.

## Boundaries

Deterministic output does not guarantee byte identity when these inputs vary:

* Go version or zlib implementation,
* signing timestamps or externally generated signatures,
* legacy protection with an empty owner password,
* caller-supplied metadata dates or IDs,
* map iteration in features not yet covered by deterministic ordering tests,
* external validators or post-processing tools.

Callers that need golden PDFs should fix metadata, avoid randomized protection,
use explicit compression settings, and keep signing inputs stable.

## Testing

Tests should cover:

* byte-identical output for repeated generation,
* stable image IDs across units such as `mm`, `pt`, and `cm`,
* stable output after object numbers are assigned,
* stable attachment and template ordering,
* deterministic signed-output behavior where the signature inputs are fixed.
