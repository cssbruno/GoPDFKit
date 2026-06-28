# v0.9.0 Release Plan: Production Policy Layer

`v0.9.0` should be the production-readiness gate before `v1.0.0`, not just
another fixes release. `v0.8.0` already hardened a lot of internals: wrapper
functions replaced mutable aliases, explicit resource cache policy exists, image
parsing no longer needs temporary `Document` construction, UTF-8 subset cache
identity is content-based, template identity and immutability improved, signed
file output is atomic, and imported PDF source caching is bounded.

The remaining `v0.9.0` work is a coherent pass over resource identity,
operational limits, output policy, error handling, security posture, and API
stability. Compression, file-backed resources, caches, attachments, templates,
imported PDFs, signing, protection, and HTML rendering should all be governed by
a clear production surface that callers can apply at construction time or output
time.

The release goal is not only to add more knobs. The goal is to make the
operational contract explicit: how much memory and CPU a document may use, which
loaders and caches are allowed, which risky features are enabled, how output is
made deterministic, and how callers observe expensive work.

The release story should be:

> `v0.9.0` is the production-stability release. It makes output behavior
> explicit, resource identity stable, file-backed IO bounded, low-level APIs
> error-aware, legacy protection panic-free, and server deployments easier to
> configure.

## Blocker: Image Resource Identity

Image identity must be made intrinsic before `v1.0.0`. Resource identity should
not depend on output-time object numbers or document-unit scale.

Use SHA-256 and hash only intrinsic image resource fields:

```go
type imageResourceID [32]byte

func generateImageResourceID(info *ImageInfo) (imageResourceID, error) {
    // Hash only intrinsic image resource fields.
}
```

Fields to include:

* `data`,
* `smask`,
* `w`, `h`,
* `cs`,
* `pal`,
* `bpc`,
* `f`,
* `dp`,
* `trns`,
* `dpi`.

Fields to exclude:

* `n`, because it is output-time object state,
* `scale`, because it is document-unit state,
* `i`, because it is derived identity.

The existing PDF resource name can be derived from the SHA-256 value:

```go
info.i = hex.EncodeToString(id[:])
```

Acceptance tests must prove that the same image registered in `mm`, `pt`, and
`cm` documents gets the same intrinsic image ID, and that assigning object
numbers during output does not change identity.

## Target API Shape

The central type should be:

```go
type ProductionPolicy struct {
    Limits        Limits
    Compression   CompressionPolicy
    Cache         ResourceCachePolicy
    Security      SecurityPolicy
    Output        OutputPolicy
    Hooks         Hooks
    Deterministic bool
}

func WithProductionPolicy(policy ProductionPolicy) Option
func ServerSafePolicy() ProductionPolicy
func BatchPolicy() ProductionPolicy
func DeterministicPolicy() ProductionPolicy
```

Policy helpers should be conservative and documented:

* `ServerSafePolicy` should prefer bounded memory, explicit limits, disabled
  risky features, and predictable worker counts.
* `BatchPolicy` may allow larger resources and more compression work.
* `DeterministicPolicy` should favor reproducible bytes for tests, golden PDFs,
  and CI.

## Precedence And Zero Values

Policy precedence must be deterministic:

1. Per-call `OutputOptions` override output-time behavior for that call.
2. Functional options are applied in the order provided to `NewDocument`.
3. A later functional option overrides fields set by an earlier
   `WithProductionPolicy`.
4. Explicit `ImageCache` and `FontCache` override `ResourceCachePolicy` for
   that resource type.
5. Per-document defaults override package-wide defaults.
6. Package-wide defaults are only the fallback.

Zero-value semantics:

| Type | Zero Value |
| --- | --- |
| `ProductionPolicy` | Applies default compression/cache fields and installs an explicit zero `SecurityPolicy`; use named presets for real deployments. |
| `Limits` fields | No override; keep the package default or current document setting. Negative values are invalid. |
| `CompressionPolicy` | Package compression defaults. |
| `SecurityPolicy` when unset | Compatibility mode; existing behavior remains allowed. |
| `SecurityPolicy{}` when explicitly set | Denies gated features. |
| `OutputOptions` | Use the document's current output settings and durable file sync. |
| `Hooks` | No-op. |

Initial preset values:

| Preset | Cache | Compression | Attachment Limit | Security |
| --- | --- | --- | ---: | --- |
| `ServerSafePolicy` | `ResourceCacheDocument` | Best speed, 4 page workers, 2 attachment workers | 64 MiB | Deny JavaScript, local HTML images, file-backed attachments, raw writes, and legacy RC4 protection. |
| `BatchPolicy` | `ResourceCacheShared` | Default zlib level, `GOMAXPROCS` workers | 512 MiB | Allow compatibility features for trusted offline jobs. |
| `DeterministicPolicy` | Server-safe cache | Stable explicit compression | 64 MiB | Server-safe gates plus deterministic output. |

## Limits

Add one limits object for resource and document bounds:

```go
type Limits struct {
    MaxImageSourceBytes         int64
    MaxImageDecodedBytes        int64
    MaxAttachmentBytes          int64
    MaxImportedPDFBytes         int64
    MaxHTMLBytes                int
    MaxHTMLGeneratedPages       int
    MaxTemplateSerializedBytes  int
    MaxPages                    int
    MaxReferencedObjects        int
}

func WithLimits(limits Limits) Option
func ServerSafeLimits() Limits
func BatchLimits() Limits
```

The implementation must replace scattered hardcoded caps with this shared
contract. File-backed attachments need special attention because deferred output
loading must enforce a max-size guard before reading content into memory.

## Attachment Limits And Eager Validation

File-backed attachments are convenient, but deferred filesystem access can make
output fail long after the document is constructed. `v0.9.0` should make
attachment limits explicit and allow eager validation:

```go
type AttachmentOptions struct {
    MaxBytes int64
    Eager    bool
}

func AttachmentFromFileWithOptions(fileStr string, options AttachmentOptions) (Attachment, error)
func (f *Document) SetMaxAttachmentBytes(maxBytes int64)
```

The recommended default maximum is `64 * 1024 * 1024` bytes.

Acceptance tests should cover:

* file deleted after `AttachmentFromFile`,
* file changed after `AttachmentFromFile`,
* file exceeds max size,
* empty file,
* same file attached globally and as an annotation.

The embedded-file name typo should be fixed before output behavior is more
stable:

```text
Attachement1 -> Attachment1
```

## Context Cancellation

Production callers need cancellation across output, signing, compression,
resource loading, and HTML rendering:

```go
func (f *Document) OutputContext(ctx context.Context, w io.Writer) error
func (f *Document) OutputFileContext(ctx context.Context, fileStr string) error
func (f *Document) OutputSignedContext(ctx context.Context, w io.Writer, options sign.Options) error
func (html *HTML) WriteContext(ctx context.Context, lineHt float64, htmlStr string) error
```

Background page and attachment compression workers must observe cancellation and
stop scheduling new work after `ctx` is done. Existing non-context methods should
call the context variants with `context.Background()`.

Initial semantics:

* cancellation before output returns `ErrOutputCanceled` and writes no bytes,
* cancellation before the final write returns `ErrOutputCanceled`,
* `OutputFileContext` removes the temporary file on cancellation,
* cancellation during arbitrary `io.Writer` work depends on the writer,
* deeper cancellation inside page compression, attachment loading, HTML
  rendering, and signing remains a follow-up until those paths accept context.

## Output Options

`OutputFileOptions` currently controls file sync only. `v0.9.0` should introduce
output-wide options because compression, attachment loading, imported PDF
writing, signing, and deterministic output all happen during output:

```go
type OutputOptions struct {
    DisableSync   bool
    Compression   CompressionPolicy
    Limits        Limits
    Deterministic bool
}

func (f *Document) OutputWithOptions(w io.Writer, options OutputOptions) error
func (f *Document) OutputFileWithOptions(fileStr string, options OutputOptions) error
func (f *Document) OutputSignedFileWithOptions(fileStr string, signOptions sign.Options, outputOptions OutputOptions) error
```

Signed file output should continue to share the same atomic file-writing path as
normal output.

`OutputPolicy` can hold output-specific defaults when embedded in
`ProductionPolicy`:

```go
type OutputPolicy struct {
    DisableSync   bool
    Deterministic bool
}
```

## Deterministic Output

Expose a single deterministic mode:

```go
func WithDeterministicOutput() Option
```

Deterministic output should set or require:

* catalog/resource sorting,
* stable compression behavior chosen by the caller,
* fixed or explicitly provided creation and modification dates,
* stable file IDs,
* stable attachment ordering,
* stable template/resource ordering.

This mode is intended for tests, reproducible builds, golden PDFs, and CI.

Deterministic mode does not promise byte identity across different Go versions,
different zlib implementations, wall-clock signing timestamps, randomized
legacy protection owner passwords, or caller-supplied nondeterministic metadata.
Those inputs must be fixed by the caller.

## Security Policy

Add an explicit security policy that server applications can tighten:

```go
type SecurityPolicy struct {
    AllowLegacyRC4Protection bool
    AllowJavaScript          bool
    AllowLocalHTMLImages     bool
    AllowFileAttachments     bool
    AllowRawWrites           bool
    MaxEmbeddedFileBytes     int64
}

func WithSecurityPolicy(policy SecurityPolicy) Option
```

Legacy RC4 protection, JavaScript actions, local HTML images, file-backed
attachments, PDF signing, and imported PDFs should be reviewed against this
policy. Legacy protection must return errors instead of panicking on random
generation failure.

Legacy protection should be renamed in the public API:

```go
func (f *Document) SetLegacyProtection(actionFlag byte, userPass, ownerPass string) error
```

`SetProtection` should remain as a compatibility shim during `v0.x`, but docs
must describe legacy PDF standard-security behavior as advisory compatibility,
not modern encryption.

Initial enforcement matrix:

| Policy Field | Applies To |
| --- | --- |
| `AllowJavaScript` | `SetJavascript` / JavaScript catalog action output |
| `AllowLocalHTMLImages` | `HTML.AllowLocalImages` local file paths |
| `AllowFileAttachments` | `Attachment{FilePath: ...}` during output |
| `AllowLegacyRC4Protection` | `SetLegacyProtection` and `SetProtection` shim |
| `AllowRawWrites` | `RawWrite*` APIs |
| `MaxEmbeddedFileBytes` | Embedded attachment bytes |

## Resource Loaders

Use narrow loader interfaces rather than one generic stringly typed loader.
This keeps permissions and limits easier to audit:

```go
type ImageLoader interface {
    OpenImage(name string) (io.ReadCloser, error)
}

type AttachmentLoader interface {
    OpenAttachment(name string) (io.ReadCloser, error)
}

type FontLoader interface {
    Open(name string) (io.Reader, error)
}
```

The loader path must cover UTF-8 fonts, images, HTML-local images when allowed,
file-backed attachments, and imported PDFs where practical.

## Observability Hooks

Add optional callbacks for production diagnostics:

```go
type Hooks struct {
    OnResourceCacheHit  func(kind, key string)
    OnResourceCacheMiss func(kind, key string)
    OnPageCompressed    func(page int, inputBytes, outputBytes int)
    OnAttachmentLoaded  func(filename string, bytes int64)
    OnOutputObject      func(objectNumber int, kind string)
    OnWarning           func(message string)
}

func WithHooks(hooks Hooks) Option
```

Hooks must never be required for correctness. Implementations should snapshot
hook values before invoking them and should document that callbacks must be
fast and non-blocking.

## Error Taxonomy

Add sentinel errors that server code can handle with `errors.Is`:

```go
var (
    ErrInvalidPageSize      = errors.New("invalid page size")
    ErrAttachmentTooLarge   = errors.New("attachment too large")
    ErrUnsupportedImageType = errors.New("unsupported image type")
    ErrUnsupportedPDFImport = errors.New("unsupported PDF import")
    ErrHTMLLimitExceeded    = errors.New("HTML limit exceeded")
    ErrOutputCanceled       = errors.New("output canceled")
)
```

New code should wrap these errors, for example:

```go
return fmt.Errorf("%w: %s", ErrAttachmentTooLarge, fileStr)
```

The release should audit current formatted errors and convert the ones callers
are likely to branch on.

## Raw Write Error APIs

Raw writes must not silently ignore reader or copy failures:

```go
func (f *Document) RawWriteBufError(r io.Reader) error
func (f *Document) RawWriteArtifactBufError(r io.Reader) error
func (f *Document) RawWriteStrError(str string) error
func (f *Document) RawWriteArtifactStrError(str string) error
```

Existing raw-write methods should remain as compatibility shims and call
`SetError` when the error-returning implementation fails.

## Imported PDF API Cleanup

Finish the `importpdf` ergonomics before `v1.0.0`:

```go
func (r ObjRef) ObjectNumber() int
func (r ObjRef) Generation() int
func (r ObjRef) String() string

func (p *PageRef) ContentWithError() ([]byte, error)
func (p *PageRef) ContentErr() error
func (p *PageRef) ForEachObjectCopy(fn func(ObjRef, []byte) error) error
func (p *PageRef) ForEachObjectBorrowed(fn func(ObjRef, []byte) error) error
```

`ForEachObject` should prefer the safe copy behavior before `v1.0.0`, or it
should be clearly documented as borrowed/advanced if compatibility prevents that
change.

## Concurrency Contract

Document the concurrency behavior explicitly:

* `Document` is not safe for concurrent mutation or output.
* `ImageCache`, `FontCache`, and imported `SourceCache` are safe for concurrent
  use.
* `CompiledHTML` is safe for concurrent reuse.
* Template values are immutable after creation.
* Hooks and loaders supplied by callers must be safe for the concurrency they
  enable.

This contract matters because `v0.8.0` made caches more explicit and sharable,
while `Document` remains a mutable builder.

## Template Serialization Contract

Split the template API internally so rendering paths can depend on narrower
interfaces:

```go
type TemplateView interface {
    ID() string
    Size() (Point, Size)
    Bytes() []byte
    Images() map[string]*ImageInfo
    Templates() []Template
}

type PagedTemplate interface {
    NumPages() int
    FromPage(int) (Template, error)
    FromPages() []Template
}

type SerializableTemplate interface {
    Serialize() ([]byte, error)
}
```

Keep the broad compatibility interface:

```go
type Template interface {
    TemplateView
    PagedTemplate
    SerializableTemplate
    gob.GobDecoder
    gob.GobEncoder
}
```

Define the template serialization compatibility promise:

```go
func TemplateSerializationVersion() string
```

Document the contract as stable within the `v0.9.x` minor line unless the
release notes say otherwise. `TemplateSerializationVersion()` reports the
serialized-template format marker (`GPKTPL1` in `v0.9.x`); the template
fingerprint uses its own internal version marker (`GPKTPL2`).

## Validation Integration

Clarify that GoPDFKit can generate standards metadata and structure, but full
PDF/A, PDF/UA, and Arlington conformance is a validation workflow:

```go
type ValidationReport struct {
    Issues []ValidationIssue
}

type Validator interface {
    ValidatePDF(data []byte) (ValidationReport, error)
}
```

The built-in API should not shell out to validators by default. It should offer
clean interfaces and examples for veraPDF, PDF/UA checkers, and Arlington model
checkers.

## Production Examples

Add examples for operational usage, not only rendering features:

* server-safe generation,
* explicit shared caches,
* per-tenant document-local caches,
* deterministic PDFs in tests,
* bounded attachments,
* cancelable output,
* signed output with atomic file writing,
* legacy protection warnings,
* fuzz/minimal parser examples.

## Benchmark Budgets

Initial manual budgets are tracked in `doc/benchmark-budgets-v0.9.md`. Budgets
should be treated as regression gates for `v1.0.0` readiness, not as absolute
performance promises.

## Fuzzing Targets

Initial fuzz targets cover parser and serialization boundaries:

* `importpdf.OpenBytes`,
* page content extraction,
* `DeserializeTemplate`,
* `CompileHTML`,
* `HTMLTokenize`,
* `SVGParse`,
* `compileHTMLDataImageSource`,
* `parseImageOptionsReader`,
* DER/CMS parsing and verification helpers,
* PDF literal escaping,
* Unicode translation.

No fuzz target may panic on arbitrary input. Malformed input must return bounded
errors.

## No Hidden Globals Audit

Run a final global-state audit before `v1.0.0`:

* package-wide defaults,
* shared image/font/subset caches,
* zlib writer pools or free lists,
* global HTML/name intern tables, if any,
* test/example `init` mutations.

The outcome should document which globals are performance caches, which affect
output, which are concurrency-safe, and how callers can disable or scope them.

## Implementation Order

Must-have before `v0.9.0`:

1. SHA-256 image resource identity that excludes output-time and unit-scale
   state.
2. Raw-write error-returning APIs.
3. Attachment max-size limits and eager validation.
4. Configurable page compression workers.
5. Panic-free `SetLegacyProtection`.
6. `ObjRef` accessors.
7. `PageRef.ContentWithError`.
8. Embedded attachment spelling fix.
9. Production docs.

Strongly recommended for `v0.9.0`:

1. `CompressionPolicy`.
2. `WithFontCache` and `WithUTF8FontCache`.
3. `OutputOptions`.
4. `Limits`.
5. `SecurityPolicy`.
6. Template interface split.
7. Fuzz targets.

Can wait until `v0.10.0` or `v1.0.0` if needed:

1. Full `ResourceLoader` abstraction.
2. Context support everywhere.
3. Observability hooks.
4. Benchmark regression automation.
5. AES-based modern PDF encryption.

Avoid before `v0.9.0`:

* full browser HTML/CSS,
* DOCX conversion,
* OCR,
* general PDF rewriting,
* interactive form filling or flattening,
* full modern encrypted-PDF import.

Detailed readiness tracking is in `doc/v0.9-readiness-checklist.md`.

## Release Acceptance Criteria

`v0.9.0` is ready when:

* the production policy can configure limits, compression, cache policy,
  security policy, hooks, and deterministic output from one place,
* output-time options can override output-time behavior without requiring a new
  document,
* file-backed resources are bounded before large reads,
* context cancellation stops output and background compression promptly,
* production examples compile,
* the migration table for pre-`v1.0.0` API movement is published,
* benchmark and validation workflows remain documented and runnable.
