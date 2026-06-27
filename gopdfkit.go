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
)

var (
	WithOrientation           = document.WithOrientation
	WithUnit                  = document.WithUnit
	WithPageSize              = document.WithPageSize
	WithCustomPageSize        = document.WithCustomPageSize
	WithFontDir               = document.WithFontDir
	WithOptimize              = document.WithOptimize
	WithBestCompression       = document.WithBestCompression
	WithLegacyConstructorArgs = document.WithLegacyConstructorArgs
)

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
