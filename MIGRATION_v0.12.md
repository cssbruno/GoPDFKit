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

The exported `document.Options` struct was removed. Map its fields as follows:

| Removed `Options` field | Functional option |
| --- | --- |
| `Orientation` | `WithOrientation` |
| `Unit` | `WithUnit` |
| `PageSize` | `WithPageSize` |
| `Size` | `WithCustomPageSize` |
| `FontDir` | `WithFontDir` |
| `Optimize` | `WithOptimize` or `WithBestCompression` |
| `CompressionPolicy` | `WithCompressionPolicy` |
| `PageCompressionWorkers` | `WithPageCompressionWorkers` |
| `AttachmentCompressionWorkers` | `WithAttachmentCompressionWorkers` |
| `CachePolicy` | `WithResourceCachePolicy` |
| `ImageCache` | `WithImageCache` |
| `FontCache` | `WithFontCache` |
| `ResourceLoader` | `WithResourceLoader` |
| `Limits` | `WithLimits` |
| `SecurityPolicy` | `WithSecurityPolicy` |
| `OutputPolicy` | `WithOutputPolicy` |
| `Hooks` | `WithHooks` |
| `DeterministicOutput` | `WithDeterministicOutput` |

`WithUTF8FontCache` becomes `WithFontCache`. Replace
`WithLegacyConstructorArgs` with the typed orientation, unit, page-size, custom
size, and font-directory options above. Options are applied left to right; a
later option wins when two options configure the same behavior.

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

Replace `document.NewMeasureContext(pdf, width)` with
`layout.NewMeasureContext(width, defaultStyle)`. The old helper installed a
document-specific text measurer that is intentionally private in v0.12. For
renderer-specific wrapping, implement `layout.TextMeasurer` and assign it to
the returned context's `TextMeasurer` field.

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
| `sign/pkcs7.Options` | `sign.CMSOptions` |
| `sign/pkcs7.VerifyResult` | `sign.CMSVerifyResult` |
| `sign/pkcs7.Info` | `sign.CMSInfo` |
| `sign/pkcs7.Create` | `sign.CreateCMS` |
| `sign/pkcs7.Verify` | `sign.VerifyCMS` |
| `sign/pkcs7.VerifyDetached` | `sign.VerifyDetachedCMS` |
| `sign/pkcs7.Inspect` | `sign.InspectCMS` |
| `sign/pkcs7.EmbedDetached` | `sign.EmbedDetachedCMS` |

The revocation and signed-attribute helper names already exist unchanged in
`sign`; update only their import path.

Set `CompressionPolicy.Mode` to `CompressionEnabled` or
`CompressionDisabled`; a zero mode keeps the normal enabled default when the
rest of a partial compression policy is supplied.

## Output

Existing output method names remain public, but all writer, file, context,
streaming, sync, option, and signing variants now share one internal output
coordinator. No output migration is required beyond the `OutputOptions` rename.
