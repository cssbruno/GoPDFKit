// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"time"
)

// Sentinel errors for callers that need to branch on broad failure classes.
var (
	ErrInvalidPageSize          = errors.New("invalid page size")
	ErrAttachmentTooLarge       = errors.New("attachment too large")
	ErrUnsupportedImageType     = errors.New("unsupported image type")
	ErrUnsupportedPDFImport     = errors.New("unsupported PDF import")
	ErrImageTooLarge            = errors.New("image too large")
	ErrHTMLLimitExceeded        = errors.New("HTML input exceeds maximum size")
	ErrPageLimitExceeded        = errors.New("page limit exceeded")
	ErrOutputCanceled           = errors.New("output canceled")
	ErrSecurityPolicyDenied     = errors.New("security policy denied feature")
	ErrJavaScriptUnsupported    = errors.New("JavaScript actions are not supported")
	ErrAESProtectionUnsupported = errors.New("AES-based PDF encryption is not supported")
)

// Limits bounds resource and document sizes for production deployments. A zero
// field leaves the package default or existing document setting unchanged.
type Limits struct {
	MaxImageSourceBytes        int64
	MaxImageDecodedBytes       int64
	MaxAttachmentBytes         int64
	MaxImportedPDFBytes        int64
	MaxHTMLBytes               int
	MaxHTMLGeneratedPages      int
	MaxTemplateSerializedBytes int
	MaxPages                   int
	MaxReferencedObjects       int
}

// OutputPolicy controls output-specific defaults embedded in ProductionPolicy.
type OutputPolicy struct {
	DisableSync   bool
	Deterministic bool
	// StreamFinal makes regular Output* methods use the one-shot streaming
	// final-writer path. It is useful for very large unsigned PDFs when lower
	// peak memory is more important than repeatable output from one Document.
	StreamFinal bool
}

// SecurityPolicy gates features that server callers often disable. A policy is
// enforced only when explicitly installed with WithSecurityPolicy or
// WithProductionPolicy. When enforced, false booleans deny the corresponding
// feature.
type SecurityPolicy struct {
	AllowLegacyRC4Protection bool
	AllowLocalHTMLImages     bool
	AllowFileAttachments     bool
	AllowRawWrites           bool
	AllowPDFImport           bool
	AllowPDFSigning          bool
	MaxEmbeddedFileBytes     int64
}

// Hooks receives optional production diagnostics. Hooks are best-effort and
// must not be required for correctness. Hooks may be called concurrently from
// output worker goroutines, so hook implementations must be safe for the
// concurrency they observe.
type Hooks struct {
	OnResourceCacheHit  func(kind, key string)
	OnResourceCacheMiss func(kind, key string)
	OnPageCompressed    func(page int, inputBytes, outputBytes int)
	OnAttachmentLoaded  func(filename string, bytes int64)
	OnOutputObject      func(objectNumber int, kind string)
	OnWarning           func(message string)
	// OnLayoutEngineRoute reports which implementation served a public layout
	// entry point. Automatic layout reports only successful unified routes;
	// unsupported input fails before this hook is called. Values never contain
	// authored content or source paths. Callers can audit that no legacy route
	// was invoked without depending on private renderer types.
	OnLayoutEngineRoute func(entryPoint, engine, reason string)
}

// ProductionPolicy groups operational controls for server and batch use.
type ProductionPolicy struct {
	Limits        Limits
	Compression   CompressionPolicy
	Cache         ResourceCachePolicy
	CacheSet      bool
	Security      SecurityPolicy
	Output        OutputPolicy
	Hooks         Hooks
	Deterministic bool
}

// ServerSafeLimits returns conservative resource limits for request-scoped
// generation.
func ServerSafeLimits() Limits {
	return Limits{
		MaxImageSourceBytes:        32 * 1024 * 1024,
		MaxImageDecodedBytes:       256 * 1024 * 1024,
		MaxAttachmentBytes:         MaxAttachmentBytes,
		MaxImportedPDFBytes:        64 * 1024 * 1024,
		MaxHTMLBytes:               4 * 1024 * 1024,
		MaxHTMLGeneratedPages:      500,
		MaxTemplateSerializedBytes: 16 * 1024 * 1024,
		MaxPages:                   10_000,
		MaxReferencedObjects:       100_000,
	}
}

// BatchLimits returns larger limits for trusted offline generation.
func BatchLimits() Limits {
	return Limits{
		MaxImageSourceBytes:        256 * 1024 * 1024,
		MaxImageDecodedBytes:       1024 * 1024 * 1024,
		MaxAttachmentBytes:         512 * 1024 * 1024,
		MaxImportedPDFBytes:        importPDFMaxSourceBytes(),
		MaxHTMLBytes:               32 * 1024 * 1024,
		MaxHTMLGeneratedPages:      10_000,
		MaxTemplateSerializedBytes: 128 * 1024 * 1024,
		MaxPages:                   100_000,
		MaxReferencedObjects:       importPDFMaxReferencedObjects(),
	}
}

// ServerSafePolicy returns a production profile for multi-tenant or
// request-scoped generation. It avoids shared package-level caches and disables
// higher-risk features unless callers opt into them separately.
func ServerSafePolicy() ProductionPolicy {
	return ProductionPolicy{
		Limits: ServerSafeLimits(),
		Compression: CompressionPolicy{
			Mode:                     CompressionEnabled,
			Level:                    zlib.BestSpeed,
			PageWorkers:              4,
			AttachmentWorkers:        2,
			TinyStreamThresholdBytes: defaultTinyStreamCompressionThreshold,
		},
		Cache:    ResourceCacheDocument,
		CacheSet: true,
		Security: SecurityPolicy{
			MaxEmbeddedFileBytes: MaxAttachmentBytes,
		},
	}
}

// BatchPolicy returns a profile for trusted offline generation.
func BatchPolicy() ProductionPolicy {
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	return ProductionPolicy{
		Limits: BatchLimits(),
		Compression: CompressionPolicy{
			Mode:                     CompressionEnabled,
			Level:                    zlib.DefaultCompression,
			PageWorkers:              workers,
			AttachmentWorkers:        workers,
			TinyStreamThresholdBytes: defaultTinyStreamCompressionThreshold,
		},
		Cache:    ResourceCacheShared,
		CacheSet: true,
		Security: SecurityPolicy{
			AllowLegacyRC4Protection: true,
			AllowLocalHTMLImages:     true,
			AllowFileAttachments:     true,
			AllowRawWrites:           true,
			AllowPDFImport:           true,
			AllowPDFSigning:          true,
			MaxEmbeddedFileBytes:     512 * 1024 * 1024,
		},
	}
}

func importPDFMaxSourceBytes() int64 {
	return maxPDFImportSourceBytes
}

func importPDFMaxReferencedObjects() int {
	return maxPDFImportReferencedObjects
}

// DeterministicPolicy returns a server-safe profile with deterministic output
// enabled.
func DeterministicPolicy() ProductionPolicy {
	policy := ServerSafePolicy()
	policy.Deterministic = true
	policy.Output.Deterministic = true
	return policy
}

// DeterministicDefaults returns fixed generation defaults for byte-stable
// output in tests and CI.
func DeterministicDefaults() Defaults {
	fixed := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	return Defaults{
		CatalogSort:      true,
		Compression:      true,
		CreationDate:     fixed,
		ModificationDate: fixed,
	}
}

func validateLimits(limits Limits) error {
	if limits.MaxImageSourceBytes < 0 {
		return fmt.Errorf("invalid max image source bytes: %d", limits.MaxImageSourceBytes)
	}
	if limits.MaxImageDecodedBytes < 0 {
		return fmt.Errorf("invalid max image decoded bytes: %d", limits.MaxImageDecodedBytes)
	}
	if limits.MaxAttachmentBytes < 0 {
		return fmt.Errorf("invalid max attachment bytes: %d", limits.MaxAttachmentBytes)
	}
	if limits.MaxImportedPDFBytes < 0 {
		return fmt.Errorf("invalid max imported PDF bytes: %d", limits.MaxImportedPDFBytes)
	}
	if limits.MaxHTMLBytes < 0 {
		return fmt.Errorf("invalid max HTML bytes: %d", limits.MaxHTMLBytes)
	}
	if limits.MaxHTMLGeneratedPages < 0 {
		return fmt.Errorf("invalid max HTML generated pages: %d", limits.MaxHTMLGeneratedPages)
	}
	if limits.MaxTemplateSerializedBytes < 0 {
		return fmt.Errorf("invalid max template serialized bytes: %d", limits.MaxTemplateSerializedBytes)
	}
	if limits.MaxPages < 0 {
		return fmt.Errorf("invalid max pages: %d", limits.MaxPages)
	}
	if limits.MaxReferencedObjects < 0 {
		return fmt.Errorf("invalid max referenced objects: %d", limits.MaxReferencedObjects)
	}
	return nil
}

func (f *Document) applyLimits(limits Limits) error {
	if err := validateLimits(limits); err != nil {
		f.SetError(err)
		return err
	}
	if limits.MaxAttachmentBytes > 0 {
		f.SetMaxAttachmentBytes(limits.MaxAttachmentBytes)
	}
	f.limits = limits
	f.limitsSet = true
	return f.err
}

// SetLimits sets resource and document limits on an existing Document.
func (f *Document) SetLimits(limits Limits) error {
	return f.applyLimits(limits)
}

// SetSecurityPolicy installs feature gates on an existing Document. A zero
// SecurityPolicy denies all gated features because it was explicitly installed.
func (f *Document) SetSecurityPolicy(policy SecurityPolicy) error {
	return f.applySecurityPolicy(policy)
}

// SetHooks installs optional production diagnostics callbacks.
func (f *Document) SetHooks(hooks Hooks) {
	f.hooks = hooks
}

// SetProductionPolicy applies production controls to an existing Document.
func (f *Document) SetProductionPolicy(policy ProductionPolicy) error {
	f.applyRuntimePolicy(runtimePolicyFromProductionPolicy(policy))
	return f.err
}

func runtimePolicyFromProductionPolicy(policy ProductionPolicy) runtimePolicy {
	cachePolicy := policy.Cache
	if !policy.CacheSet && policy.Cache == ResourceCacheShared {
		cachePolicy = ResourceCacheDocument
	}
	return runtimePolicy{
		compressionPolicy:    policy.Compression,
		compressionPolicySet: true,
		cachePolicy:          cachePolicy,
		limits:               policy.Limits,
		limitsSet:            true,
		securityPolicy:       policy.Security,
		securityPolicySet:    true,
		outputPolicy:         policy.Output,
		outputPolicySet:      true,
		hooks:                policy.Hooks,
		hooksSet:             true,
		deterministicOutput:  policy.Deterministic || policy.Output.Deterministic,
	}
}

func (f *Document) imageSourceLimit() int {
	if f.limits.MaxImageSourceBytes <= 0 {
		return maxImageSourceBytes
	}
	if f.limits.MaxImageSourceBytes > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(f.limits.MaxImageSourceBytes)
}

func (f *Document) imageDecodedLimit() int {
	if f.limits.MaxImageDecodedBytes <= 0 {
		return maxImageDecodedBytes
	}
	if f.limits.MaxImageDecodedBytes > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(f.limits.MaxImageDecodedBytes)
}

func (f *Document) checkPageLimitForAdd() error {
	if f.limits.MaxPages <= 0 {
		return nil
	}
	if len(f.pages)-1 >= f.limits.MaxPages {
		err := fmt.Errorf("%w: %d >= %d", ErrPageLimitExceeded, len(f.pages)-1, f.limits.MaxPages)
		f.SetError(err)
		return err
	}
	return nil
}

func (f *Document) applySecurityPolicy(policy SecurityPolicy) error {
	if policy.MaxEmbeddedFileBytes < 0 {
		err := fmt.Errorf("invalid max embedded file bytes: %d", policy.MaxEmbeddedFileBytes)
		f.SetError(err)
		return err
	}
	f.securityPolicy = policy
	f.securityPolicySet = true
	if policy.MaxEmbeddedFileBytes > 0 {
		limit := policy.MaxEmbeddedFileBytes
		if f.maxAttachmentBytes > 0 && f.maxAttachmentBytes < limit {
			limit = f.maxAttachmentBytes
		}
		f.SetMaxAttachmentBytes(limit)
	}
	return f.err
}

func (f *Document) applyDeterministicOutput() {
	defaults := DeterministicDefaults()
	f.catalogSort = true
	if f.creationDate.IsZero() {
		f.creationDate = defaults.CreationDate
	}
	if f.modDate.IsZero() {
		f.modDate = defaults.ModificationDate
	}
}

func (f *Document) denyFeature(feature string) error {
	err := fmt.Errorf("%w: %s", ErrSecurityPolicyDenied, feature)
	f.SetError(err)
	return err
}

func (f *Document) requireSecurityFeature(feature string, allowed bool) error {
	if f.securityPolicySet && !allowed {
		return f.denyFeature(feature)
	}
	return nil
}

func outputCanceledError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrOutputCanceled, err)
	}
	return nil
}
