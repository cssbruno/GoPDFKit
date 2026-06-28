# Security Policy

This document describes the intended `v0.9.0` security posture. GoPDFKit is a
PDF generation library, not a sandbox. Server applications should explicitly
disable features they do not need.

## Target Policy

```go
type SecurityPolicy struct {
    AllowLegacyRC4Protection bool
    AllowJavaScript          bool
    AllowLocalHTMLImages     bool
    AllowFileAttachments     bool
    AllowRawWrites           bool
    MaxEmbeddedFileBytes     int64
}
```

Apply the policy to:

* JavaScript actions,
* local images referenced from HTML,
* file-backed attachments,
* legacy PDF standard-security protection,
* raw PDF writes,
* imported PDFs where limits or parser support apply.

## Enforcement Matrix

| Policy Field | Initial Enforcement |
| --- | --- |
| `AllowJavaScript` | `SetJavascript` |
| `AllowLocalHTMLImages` | Local paths when `HTML.AllowLocalImages` is true |
| `AllowFileAttachments` | `Attachment.FilePath` during output |
| `AllowLegacyRC4Protection` | `SetLegacyProtection` and `SetProtection` |
| `AllowRawWrites` | `RawWriteStrError`, `RawWriteArtifactStrError`, `RawWriteBufError`, `RawWriteArtifactBufError` |
| `MaxEmbeddedFileBytes` | Attachment content and file-backed attachment reads |

If no security policy is installed, compatibility behavior remains allowed. If
`SecurityPolicy{}` is explicitly installed, all gated features are denied.

## Legacy Protection

`SetLegacyProtection` should implement legacy PDF standard-security behavior for
compatibility. It should return errors instead of panicking and must not be
described as modern encryption. `SetProtection` can remain as a compatibility
wrapper during `v0.x`.

## Raw Writes

Raw writes are powerful and bypass semantic checks. Server-safe policy should
disable raw writes unless the caller owns all content. Error-returning raw-write
APIs should be preferred so failed readers cannot silently produce partial PDF
syntax.

## File-Backed Resources

File-backed images, attachments, imported PDFs, and UTF-8 fonts should go
through explicit limits and, where possible, loader interfaces. Do not rely on a
document constructed successfully as proof that every deferred file-backed
resource is still present or still within limits at output time.
