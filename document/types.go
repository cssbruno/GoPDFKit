// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "time"

// cnDocumentVersion is the producer version emitted by this package.
const (
	cnDocumentVersion = "1.9"
)

// Orientation identifies a document or page orientation.
type Orientation string

const (
	// OrientationPortrait represents the portrait orientation.
	OrientationPortrait = "portrait"

	// OrientationLandscape represents the landscape orientation.
	OrientationLandscape = "landscape"
)

// String returns the constructor representation of the orientation.
func (orientation Orientation) String() string {
	return string(orientation)
}

// Unit identifies the unit of measure used for document geometry.
type Unit string

const (
	// UnitPoint represents points.
	UnitPoint = "pt"
	// UnitMillimeter represents millimeters.
	UnitMillimeter = "mm"
	// UnitCentimeter represents centimeters.
	UnitCentimeter = "cm"
	// UnitInch represents inches.
	UnitInch = "inch"
)

// String returns the constructor representation of the unit.
func (unit Unit) String() string {
	return string(unit)
}

// PageSizeName identifies a named page size.
type PageSizeName string

const (
	// PageSizeA1 represents the DIN/ISO A1 page size.
	PageSizeA1 = "A1"
	// PageSizeA2 represents the DIN/ISO A2 page size.
	PageSizeA2 = "A2"
	// PageSizeA3 represents the DIN/ISO A3 page size.
	PageSizeA3 = "A3"
	// PageSizeA4 represents the DIN/ISO A4 page size.
	PageSizeA4 = "A4"
	// PageSizeA5 represents the DIN/ISO A5 page size.
	PageSizeA5 = "A5"
	// PageSizeA6 represents the DIN/ISO A6 page size.
	PageSizeA6 = "A6"
	// PageSizeLetter represents the US Letter page size.
	PageSizeLetter = "Letter"
	// PageSizeLegal represents the US Legal page size.
	PageSizeLegal = "Legal"
	// PageSizeTabloid represents the US Tabloid page size.
	PageSizeTabloid = "Tabloid"
)

// String returns the constructor representation of the page size name.
func (pageSize PageSizeName) String() string {
	return string(pageSize)
}

const (
	// BorderNone draws no border.
	BorderNone = ""
	// BorderFull draws a full border.
	BorderFull = "1"
	// BorderLeft draws the left border.
	BorderLeft = "L"
	// BorderTop draws the top border.
	BorderTop = "T"
	// BorderRight draws the right border.
	BorderRight = "R"
	// BorderBottom draws the bottom border.
	BorderBottom = "B"
)

const (
	// LineBreakNone disables line breaks.
	LineBreakNone = 0
	// LineBreakNormal enables normal line breaks.
	LineBreakNormal = 1
	// LineBreakBelow enables a line break below the current element.
	LineBreakBelow = 2
)

const (
	// AlignLeft aligns the cell content to the left.
	AlignLeft = "L"
	// AlignRight aligns the cell content to the right.
	AlignRight = "R"
	// AlignCenter centers the cell content.
	AlignCenter = "C"
	// AlignTop aligns the cell content to the top.
	AlignTop = "T"
	// AlignBottom aligns the cell content to the bottom.
	AlignBottom = "B"
	// AlignMiddle vertically centers the cell content.
	AlignMiddle = "M"
	// AlignBaseline aligns the cell content to the baseline.
	AlignBaseline = "B"
)

// Size fields Wd and Ht specify the horizontal and vertical extents of a
// document element such as a page.
type Size struct {
	Wd float64 // Width.
	Ht float64 // Height.
}

// Point fields X and Y specify the horizontal and vertical coordinates of
// a point, typically used in drawing.
type Point struct {
	X float64 // Horizontal coordinate.
	Y float64 // Vertical coordinate.
}

// XY returns the X and Y components of the receiver point.
func (p Point) XY() (float64, float64) {
	return p.X, p.Y
}

// Defaults groups per-document generation defaults. Use DefaultSettings as a
// base when only some values should be overridden for one document.
type Defaults struct {
	CatalogSort      bool      // Consistently order internal resource catalogs.
	Compression      bool      // Compress generated page and template streams.
	CreationDate     time.Time // Fixed CreationDate; zero uses generation time.
	ModificationDate time.Time // Fixed ModDate; zero uses generation time.
}

// CompressionMode selects whether CompressionPolicy should enable or disable
// generated stream compression. CompressionDefault lets the policy infer the
// mode from the other fields.
type CompressionMode int

const (
	// CompressionDefault uses package defaults unless other policy fields imply
	// compression should be enabled.
	CompressionDefault CompressionMode = iota
	// CompressionEnabled enables Flate compression.
	CompressionEnabled
	// CompressionDisabled disables Flate compression.
	CompressionDisabled
)

const (
	// CompressionWorkersDefault uses the package default worker count inside a
	// CompressionPolicy.
	CompressionWorkersDefault = 0
	// CompressionWorkersDisabled disables background compression workers inside
	// a CompressionPolicy. The legacy Set*Workers methods still use 0 to
	// disable workers.
	CompressionWorkersDisabled = -1
)

// CompressionPolicy controls generated stream compression and background
// compression work. The zero value means package defaults. For partial structs,
// fields such as Level or PageWorkers keep compression enabled; use Mode:
// CompressionDisabled to disable compression.
type CompressionPolicy struct {
	Mode                     CompressionMode // Explicit compression mode; 0 infers from other fields.
	Level                    int             // zlib compression level; 0 uses the default level.
	PageWorkers              int             // Background page compression workers; 0 defaults, -1 disables.
	AttachmentWorkers        int             // Background attachment compression workers; 0 defaults, -1 disables.
	TinyStreamThresholdBytes int             // Streams smaller than this are left uncompressed; 0 uses the default threshold.
}

// ResourceCachePolicy controls file-backed resource caching for images and
// UTF-8 fonts loaded by path.
type ResourceCachePolicy int

const (
	// ResourceCacheShared uses bounded package-level caches. This preserves the
	// historical default behavior.
	ResourceCacheShared ResourceCachePolicy = iota
	// ResourceCacheDocument keeps file-backed resource cache entries on the
	// document instance only.
	ResourceCacheDocument
	// ResourceCacheDisabled parses file-backed resources without cache reuse.
	ResourceCacheDisabled
)

// PageBox defines the coordinates and extent of a PDF page box.
type PageBox struct {
	Size
	Point
}
