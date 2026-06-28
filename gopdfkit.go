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

// ResourceCachePolicy controls file-backed resource caching.
type ResourceCachePolicy = document.ResourceCachePolicy

// ImageCache stores parsed image data for reuse across documents.
type ImageCache = document.ImageCache

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

// WithResourceCachePolicy sets the cache policy for file-backed images and
// UTF-8 fonts loaded by path.
func WithResourceCachePolicy(policy ResourceCachePolicy) Option {
	return document.WithResourceCachePolicy(policy)
}

// WithImageCache uses cache for file-backed image registration.
func WithImageCache(cache *ImageCache) Option {
	return document.WithImageCache(cache)
}

// WithLegacyConstructorArgs applies the string arguments accepted by New.
func WithLegacyConstructorArgs(orientationStr, unitStr, sizeStr, fontDirStr string) Option {
	return document.WithLegacyConstructorArgs(orientationStr, unitStr, sizeStr, fontDirStr)
}

// NewImageCache creates an empty reusable image cache.
func NewImageCache() *ImageCache {
	return document.NewImageCache()
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
