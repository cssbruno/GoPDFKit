// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import "github.com/cssbruno/gopdfkit/document"

// Document is the high-level PDF document API exposed by the document package.
type Document = document.Document

// Options customizes a new Document.
type Options = document.Options

// Option customizes a new Document through the functional construction API.
type Option = document.Option

// Defaults customizes per-document generation defaults.
type Defaults = document.Defaults

// CompressionPolicy controls generated stream compression and background work.
type CompressionPolicy = document.CompressionPolicy

// ResourceCachePolicy controls file-backed resource caching.
type ResourceCachePolicy = document.ResourceCachePolicy

// Limits bounds resource and document sizes for production deployments.
type Limits = document.Limits

// OutputOptions controls behavior applied during PDF output.
type OutputOptions = document.OutputOptions

// OutputFileOptions is kept for source compatibility.
type OutputFileOptions = document.OutputFileOptions

// OutputPolicy controls output-specific production defaults.
type OutputPolicy = document.OutputPolicy

// SecurityPolicy gates features that server callers often disable.
type SecurityPolicy = document.SecurityPolicy

// Hooks receives optional production diagnostics.
type Hooks = document.Hooks

// ProductionPolicy groups operational controls for server and batch use.
type ProductionPolicy = document.ProductionPolicy

// ImageCache stores parsed image data for reuse across documents.
type ImageCache = document.ImageCache

// FontCache stores parsed UTF-8 font data for reuse across documents.
type FontCache = document.FontCache

// Attachment defines content to include in a PDF.
type Attachment = document.Attachment

// AttachmentOptions controls file-backed attachment validation.
type AttachmentOptions = document.AttachmentOptions

// ValidationSeverity classifies an external validator issue.
type ValidationSeverity = document.ValidationSeverity

// ValidationIssue stores one finding from an external validator.
type ValidationIssue = document.ValidationIssue

// ValidationReport stores external validation findings.
type ValidationReport = document.ValidationReport

// Validator integrates external PDF/A, PDF/UA, or Arlington validation tools.
type Validator = document.Validator

// Orientation identifies a document or page orientation.
type Orientation = document.Orientation

// Unit identifies the unit of measure used for document geometry.
type Unit = document.Unit

// PageSizeName identifies a named page size.
type PageSizeName = document.PageSizeName

// Size fields Wd and Ht specify the horizontal and vertical extents of a
// document element such as a page.
type Size = document.Size

const (
	OrientationPortrait  = document.OrientationPortrait
	OrientationLandscape = document.OrientationLandscape

	UnitPoint      = document.UnitPoint
	UnitMillimeter = document.UnitMillimeter
	UnitCentimeter = document.UnitCentimeter
	UnitInch       = document.UnitInch

	PageSizeA1      = document.PageSizeA1
	PageSizeA2      = document.PageSizeA2
	PageSizeA3      = document.PageSizeA3
	PageSizeA4      = document.PageSizeA4
	PageSizeA5      = document.PageSizeA5
	PageSizeA6      = document.PageSizeA6
	PageSizeLetter  = document.PageSizeLetter
	PageSizeLegal   = document.PageSizeLegal
	PageSizeTabloid = document.PageSizeTabloid

	ResourceCacheShared   = document.ResourceCacheShared
	ResourceCacheDocument = document.ResourceCacheDocument
	ResourceCacheDisabled = document.ResourceCacheDisabled

	MaxAttachmentBytes = document.MaxAttachmentBytes
)

var (
	ErrInvalidPageSize      = document.ErrInvalidPageSize
	ErrAttachmentTooLarge   = document.ErrAttachmentTooLarge
	ErrUnsupportedImageType = document.ErrUnsupportedImageType
	ErrUnsupportedPDFImport = document.ErrUnsupportedPDFImport
	ErrHTMLLimitExceeded    = document.ErrHTMLLimitExceeded
	ErrPageLimitExceeded    = document.ErrPageLimitExceeded
	ErrOutputCanceled       = document.ErrOutputCanceled
	ErrSecurityPolicyDenied = document.ErrSecurityPolicyDenied
)

// WithOrientation sets the default page orientation.
func WithOrientation(orientation Orientation) Option {
	return document.WithOrientation(orientation)
}

// WithUnit sets the unit of measure used for document geometry.
func WithUnit(unit Unit) Option {
	return document.WithUnit(unit)
}

// WithPageSize sets the named default page size.
func WithPageSize(pageSize PageSizeName) Option {
	return document.WithPageSize(pageSize)
}

// WithCustomPageSize sets an explicit default page size in the configured unit.
func WithCustomPageSize(size Size) Option {
	return document.WithCustomPageSize(size)
}

// WithFontDir sets the directory used for font resources.
func WithFontDir(fontDir string) Option {
	return document.WithFontDir(fontDir)
}

// WithOptimize switches generated page and template streams to best zlib
// compression.
func WithOptimize(optimize bool) Option {
	return document.WithOptimize(optimize)
}

// WithBestCompression switches generated page and template streams to best zlib
// compression.
func WithBestCompression() Option {
	return document.WithBestCompression()
}

// WithCompressionPolicy sets explicit stream-compression behavior.
func WithCompressionPolicy(policy CompressionPolicy) Option {
	return document.WithCompressionPolicy(policy)
}

// WithProductionPolicy applies an operational policy.
func WithProductionPolicy(policy ProductionPolicy) Option {
	return document.WithProductionPolicy(policy)
}

// WithLimits sets resource and document limits.
func WithLimits(limits Limits) Option {
	return document.WithLimits(limits)
}

// WithSecurityPolicy sets explicit feature gates.
func WithSecurityPolicy(policy SecurityPolicy) Option {
	return document.WithSecurityPolicy(policy)
}

// WithHooks installs optional production diagnostics callbacks.
func WithHooks(hooks Hooks) Option {
	return document.WithHooks(hooks)
}

// WithDeterministicOutput enables deterministic output defaults.
func WithDeterministicOutput() Option {
	return document.WithDeterministicOutput()
}

// WithNoCompression disables Flate compression for generated streams.
func WithNoCompression() Option {
	return document.WithNoCompression()
}

// WithPageCompressionWorkers sets background page compression concurrency.
func WithPageCompressionWorkers(workers int) Option {
	return document.WithPageCompressionWorkers(workers)
}

// WithAttachmentCompressionWorkers sets background attachment compression
// concurrency.
func WithAttachmentCompressionWorkers(workers int) Option {
	return document.WithAttachmentCompressionWorkers(workers)
}

// WithResourceCachePolicy sets the cache policy for file-backed images and
// UTF-8 fonts loaded by path.
func WithResourceCachePolicy(policy ResourceCachePolicy) Option {
	return document.WithResourceCachePolicy(policy)
}

// WithImageCache uses cache for file-backed image registration.
func WithImageCache(cache *ImageCache) Option {
	return document.WithImageCache(cache)
}

// WithFontCache uses cache for UTF-8 font registration.
func WithFontCache(cache *FontCache) Option {
	return document.WithFontCache(cache)
}

// WithUTF8FontCache is an alias for WithFontCache.
func WithUTF8FontCache(cache *FontCache) Option {
	return document.WithUTF8FontCache(cache)
}

// WithLegacyConstructorArgs applies the string arguments accepted by New.
func WithLegacyConstructorArgs(orientationStr, unitStr, sizeStr, fontDirStr string) Option {
	return document.WithLegacyConstructorArgs(orientationStr, unitStr, sizeStr, fontDirStr)
}

// NewImageCache creates an empty reusable image cache.
func NewImageCache() *ImageCache {
	return document.NewImageCache()
}

// NewFontCache creates an empty reusable UTF-8 font cache.
func NewFontCache() *FontCache {
	return document.NewFontCache()
}

// ServerSafeLimits returns conservative resource limits.
func ServerSafeLimits() Limits {
	return document.ServerSafeLimits()
}

// BatchLimits returns larger limits for trusted offline generation.
func BatchLimits() Limits {
	return document.BatchLimits()
}

// ServerSafePolicy returns a production profile for request-scoped generation.
func ServerSafePolicy() ProductionPolicy {
	return document.ServerSafePolicy()
}

// BatchPolicy returns a profile for trusted offline generation.
func BatchPolicy() ProductionPolicy {
	return document.BatchPolicy()
}

// DeterministicPolicy returns a server-safe deterministic profile.
func DeterministicPolicy() ProductionPolicy {
	return document.DeterministicPolicy()
}

// DeterministicDefaults returns fixed generation defaults for byte-stable output.
func DeterministicDefaults() Defaults {
	return document.DeterministicDefaults()
}

// TemplateSerializationVersion returns the current serialized-template format
// version.
func TemplateSerializationVersion() string {
	return document.TemplateSerializationVersion()
}

// AttachmentFromFile returns a file-backed attachment descriptor.
func AttachmentFromFile(fileStr string) Attachment {
	return document.AttachmentFromFile(fileStr)
}

// AttachmentFromFileWithOptions returns a file-backed attachment descriptor and
// optionally validates it immediately.
func AttachmentFromFileWithOptions(fileStr string, options AttachmentOptions) (Attachment, error) {
	return document.AttachmentFromFileWithOptions(fileStr, options)
}

// New returns a new PDF document using the document package defaults.
func New() *Document {
	return document.New("", "", "", "")
}

// NewDocument returns a new PDF document using functional construction options
// and normal Go error handling.
func NewDocument(options ...Option) (*Document, error) {
	return document.NewDocument(options...)
}

// MustNew returns a new PDF document using functional construction options and
// panics if construction fails.
func MustNew(options ...Option) *Document {
	return document.MustNew(options...)
}

// NewWithOptions returns a new PDF document using explicit construction options.
func NewWithOptions(options Options) *Document {
	return document.NewWithOptions(options)
}

// NewDocumentWithOptions returns a new PDF document using explicit construction
// options and normal Go error handling.
func NewDocumentWithOptions(options Options) (*Document, error) {
	return document.NewDocumentWithOptions(options)
}

// DefaultSettings returns the document package defaults used by New.
func DefaultSettings() Defaults {
	return document.DefaultSettings()
}

// NewWithDefaults returns a new PDF document using explicit per-document defaults.
func NewWithDefaults(options Options, defaults Defaults) *Document {
	return document.NewWithDefaults(options, defaults)
}

// NewDocumentWithDefaults returns a new PDF document using explicit
// per-document defaults and normal Go error handling.
func NewDocumentWithDefaults(options Options, defaults Defaults) (*Document, error) {
	return document.NewDocumentWithDefaults(options, defaults)
}
