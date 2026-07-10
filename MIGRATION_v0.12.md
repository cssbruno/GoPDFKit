# Migrating to v0.12

v0.12 is an intentionally breaking pre-v1 release. It removes the duplicate
root facade, layout aliases, legacy constructors, mutable package defaults, and
deprecated compatibility names.

## Imports

Import the package that owns each API:

```go
import (
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/layout"
)
```

The module root no longer contains a Go package. Replace root imports and
`gopdfkit.*` references with `document.*`. Typed blocks, page templates,
measurement types, and layout constructors must use `layout.*`.

## Construction

Use `document.NewDocument(options...)` when construction can fail, or
`document.MustNew(options...)` when invalid static configuration should panic.

| Before | v0.12 |
| --- | --- |
| `document.New("P", "mm", "A4", "")` | `document.MustNew()` |
| `document.NewWithOptions(document.Options{...})` | `document.NewDocument(document.With...(...))` |
| `document.NewDocumentWithOptions(...)` | `document.NewDocument(...)` |
| `document.NewWithDefaults(options, defaults)` | `document.NewDocumentWithDefaults(defaults, options...)` |
| `document.NewWithDefaults(document.Options{SizeStr: "Letter"}, d)` | `document.NewDocumentWithDefaults(d, document.WithPageSize(document.PageSizeLetter))` |

The exported `document.Options` struct was removed. Use functional options such
as `WithOrientation`, `WithUnit`, `WithPageSize`, `WithCustomPageSize`,
`WithFontDir`, `WithCompressionPolicy`, and `WithProductionPolicy`.

## Layout

The `document` package no longer aliases `layout` declarations.

```go
model := layout.NewDocumentModel("Invoice",
	layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "Paid"}},
	},
)
pdf := document.MustNew()
pdf.WriteDocument(model)
```

Move `LayoutDocument`, all block and style types, `PageTemplate`,
`PageNumberOptions`, `MeasureContext`, `BlockMeasurement`, `MeasureBlock`, and
`MeasureBlocks` references to `layout`.

## Defaults and renamed APIs

Package-wide default setters were removed. Copy `document.DefaultSettings()`,
modify the copy, and pass it to `NewDocumentWithDefaults`.

| Removed | Replacement |
| --- | --- |
| `SetDefaultCompression`, `SetDefaultCatalogSort` | `NewDocumentWithDefaults` |
| `SetDefaultCreationDate`, `SetDefaultModificationDate` | `NewDocumentWithDefaults` |
| `SetJavascript` | `SetJavascriptError` |
| `SetProtection`, `SetProtectionError` | `SetLegacyProtection` |
| `PointToUnitConvert` | `PointConvert` |
| `OutputFileOptions` | `OutputOptions` |
| `ValidationReport`, `ValidationIssue`, `ValidationSeverity` | `ComplianceValidationReport`, `ComplianceValidationIssue`, `ComplianceValidationSeverity` |
| `CompressionPolicy.Enabled` | `CompressionPolicy.Mode` |
| `Document.CurveCubic` | `Document.CurveBezierCubic` |
| `sign/pkcs7` wrappers | CMS-first names in `sign` |

Set `CompressionPolicy.Mode` to `CompressionEnabled` or
`CompressionDisabled`; a zero mode keeps the normal enabled default when the
rest of a partial compression policy is supplied.

## Output

Existing output method names remain public, but all writer, file, context,
streaming, sync, option, and signing variants now share one internal output
coordinator. No output migration is required beyond the `OutputOptions` rename.
