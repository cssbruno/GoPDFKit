// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"time"
)

var gl struct {
	catalogSort  bool
	noCompress   bool // Initial zero value indicates compression
	creationDate time.Time
	modDate      time.Time
}

type fmtBuffer struct {
	bytes.Buffer
}

func (b *fmtBuffer) printf(fmtStr string, args ...any) {
	fmt.Fprintf(&b.Buffer, fmtStr, args...)
}

func documentNew(orientationStr, unitStr, sizeStr, fontDirStr string, size Size) (f *Document) {
	f = new(Document)
	if orientationStr == "" {
		orientationStr = "p"
	} else {
		orientationStr = strings.ToLower(orientationStr)
	}
	if unitStr == "" {
		unitStr = "mm"
	}
	if sizeStr == "" {
		sizeStr = "A4"
	}
	if fontDirStr == "" {
		fontDirStr = "."
	}
	f.page = 0
	f.n = 2
	f.pages = make([]*bytes.Buffer, 0, 8)
	f.pages = append(f.pages, bytes.NewBufferString("")) // pages[0] is unused (1-based)
	f.pageSizes = make(map[int]Size)
	f.pageRotations = make(map[int]int)
	f.pageBoxes = make(map[int]map[string]PageBox)
	f.defPageBoxes = make(map[string]PageBox)
	f.state = 0
	f.fonts = make(map[string]fontDefinition)
	f.fontFiles = make(map[string]fontFile)
	f.diffs = make([]string, 0, 8)
	f.templates = make(map[string]Template)
	f.templateObjects = make(map[string]int)
	f.importedObjs = make(map[string][]byte, 0)
	f.importedObjPos = make(map[string]map[int]string, 0)
	f.importedTplObjs = make(map[string]string)
	f.importedTplIDs = make(map[string]int, 0)
	f.importedPages = make(map[int]*importedPDFPage)
	f.images = make(map[string]*ImageInfo)
	f.pageLinks = make([][]pageLink, 0, 8)
	f.pageLinks = append(f.pageLinks, make([]pageLink, 0)) // pageLinks[0] is unused (1-based)
	f.links = make([]internalLink, 0, 8)
	f.links = append(f.links, internalLink{}) // links[0] is unused (1-based)
	f.pageAttachments = make([][]annotationAttach, 0, 8)
	f.pageAttachments = append(f.pageAttachments, []annotationAttach{}) //
	f.aliasMap = make(map[string]string)
	f.inHeader = false
	f.inFooter = false
	f.lasth = 0
	f.fontFamily = ""
	f.fontStyle = ""
	f.SetFontSize(12)
	f.underline = false
	f.strikeout = false
	f.setDrawColor(0, 0, 0)
	f.setFillColor(0, 0, 0)
	f.setTextColor(0, 0, 0)
	f.colorFlag = false
	f.ws = 0
	f.fontpath = fontDirStr
	// Core fonts
	f.coreFonts = map[string]bool{
		"courier":      true,
		"helvetica":    true,
		"times":        true,
		"symbol":       true,
		"zapfdingbats": true,
	}
	// Scale factor
	switch unitStr {
	case "pt", "point":
		f.k = 1.0
	case "mm":
		f.k = 72.0 / 25.4
	case "cm":
		f.k = 72.0 / 2.54
	case "in", "inch":
		f.k = 72.0
	default:
		f.err = fmt.Errorf("incorrect unit %s", unitStr)
		return
	}
	f.unitStr = unitStr
	// Page sizes
	f.stdPageSizes = make(map[string]Size)
	f.stdPageSizes["a3"] = Size{841.89, 1190.55}
	f.stdPageSizes["a4"] = Size{595.28, 841.89}
	f.stdPageSizes["a5"] = Size{420.94, 595.28}
	f.stdPageSizes["a6"] = Size{297.64, 420.94}
	f.stdPageSizes["a2"] = Size{1190.55, 1683.78}
	f.stdPageSizes["a1"] = Size{1683.78, 2383.94}
	f.stdPageSizes["letter"] = Size{612, 792}
	f.stdPageSizes["legal"] = Size{612, 1008}
	f.stdPageSizes["tabloid"] = Size{792, 1224}
	if size.Wd > 0 && size.Ht > 0 {
		f.defPageSize = size
	} else {
		f.defPageSize = f.getpagesizestr(sizeStr)
		if f.err != nil {
			return
		}
	}
	f.curPageSize = f.defPageSize
	// Page orientation
	switch orientationStr {
	case "p", "portrait":
		f.defOrientation = "P"
		f.w = f.defPageSize.Wd
		f.h = f.defPageSize.Ht
	case "l", "landscape":
		f.defOrientation = "L"
		f.w = f.defPageSize.Ht
		f.h = f.defPageSize.Wd
	default:
		f.err = fmt.Errorf("incorrect orientation: %s", orientationStr)
		return
	}
	f.curOrientation = f.defOrientation
	f.wPt = f.w * f.k
	f.hPt = f.h * f.k
	// Page margins (1 cm)
	margin := 28.35 / f.k
	f.SetMargins(margin, margin, margin)
	// Interior cell margin (1 mm)
	f.cMargin = margin / 10
	// Line width (0.2 mm)
	f.lineWidth = 0.567 / f.k
	// Automatic page break
	f.SetAutoPageBreak(true, 2*margin)
	// Default display mode
	f.SetDisplayMode("default", "default")
	if f.err != nil {
		return
	}
	f.acceptPageBreak = func() bool {
		return f.autoPageBreak
	}
	// Enable compression
	f.compressLevel = zlib.BestSpeed
	f.SetCompression(!gl.noCompress)
	f.spotColorMap = make(map[string]spotColorType)
	f.blendList = make([]blendModeType, 0, 8)
	f.blendList = append(f.blendList, blendModeType{}) // blendList[0] is unused (1-based)
	f.blendMap = make(map[string]int)
	f.blendMode = "Normal"
	f.alpha = 1
	f.gradientList = make([]gradientType, 0, 8)
	f.gradientList = append(f.gradientList, gradientType{}) // gradientList[0] is unused
	// Set default PDF version number
	f.pdfVersion = "1.3"
	f.SetProducer("Document "+cnDocumentVersion, true)
	f.layerInit()
	f.catalogSort = gl.catalogSort
	f.creationDate = gl.creationDate
	f.modDate = gl.modDate
	f.userUnderlineThickness = 1
	return
}

// NewWithOptions returns a new Document instance using explicit construction
// options. It is an alternative to New when the default page size must be set by
// width and height instead of a named page size.
func NewWithOptions(options Options) (f *Document) {
	return documentNew(options.OrientationStr, options.UnitStr, options.SizeStr, options.FontDirStr, options.Size)
}

// New returns a new Document instance. Its methods are subsequently called to
// produce a single PDF document.
//
// orientationStr specifies the default page orientation. For portrait mode,
// specify "P" or "Portrait". For landscape mode, specify "L" or "Landscape".
// An empty string will be replaced with "P".
//
// unitStr specifies the unit of length used in size parameters for elements
// other than fonts, which are always measured in points. Specify "pt" for
// point, "mm" for millimeter, "cm" for centimeter, or "in" for inch. An empty
// string will be replaced with "mm".
//
// sizeStr specifies the page size. Acceptable values are "A3", "A4", "A5",
// "Letter", "Legal", or "Tabloid". An empty string will be replaced with "A4".
//
// fontDirStr specifies the file system location in which font resources will
// be found. An empty string is replaced with ".". This argument only needs to
// reference an actual directory if a font other than one of the core fonts is
// used. The core fonts are "courier", "helvetica" (also called "arial"),
// "times", and "zapfdingbats" (also called "symbol").
func New(orientationStr, unitStr, sizeStr, fontDirStr string) (f *Document) {
	return documentNew(orientationStr, unitStr, sizeStr, fontDirStr, Size{0, 0})
}

// Ok returns true if no processing errors have occurred.
func (f *Document) Ok() bool {
	return f.err == nil
}

// Err returns true if a processing error has occurred.
func (f *Document) Err() bool {
	return f.err != nil
}

// ClearError unsets the internal Document error. This method should be used with
// care, as an internal error condition usually indicates an unrecoverable
// problem with the generation of a document. It is intended to deal with cases
// in which an error is used to select an alternate form of the document.
func (f *Document) ClearError() {
	f.err = nil
}

// SetErrorf sets the internal Document error with formatted text to halt PDF
// generation; this may facilitate error handling by an application. If an error
// condition is already set, this call is ignored.
//
// See the documentation for printing in the standard fmt package for details
// about fmtStr and args.
func (f *Document) SetErrorf(fmtStr string, args ...any) {
	if f.err == nil {
		f.err = fmt.Errorf(fmtStr, args...)
	}
}

// String satisfies the fmt.Stringer interface and summarizes the Document
// instance.
func (f *Document) String() string {
	return "Document " + cnDocumentVersion
}

// SetError sets an error to halt PDF generation. This may facilitate error
// handling by an application. See also Ok(), Err(), and Error().
func (f *Document) SetError(err error) {
	if f.err == nil && err != nil {
		f.err = err
	}
}

// Error returns the internal Document error; this will be nil if no error has
// occurred.
func (f *Document) Error() error {
	return f.err
}
