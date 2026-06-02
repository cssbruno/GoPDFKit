/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"bytes"
	"time"
)

type Fpdf struct {
	isCurrentUTF8    bool                       // is current font used in utf-8 mode
	isRTL            bool                       // is is right to left mode enabled
	page             int                        // current page number
	n                int                        // current object number
	offsets          []int                      // array of object offsets
	templates        map[string]Template        // templates used in this document
	templateObjects  map[string]int             // template object IDs within this document
	importedObjs     map[string][]byte          // imported template objects
	importedObjPos   map[string]map[int]string  // imported template object hash positions
	importedTplObjs  map[string]string          // imported template names and IDs
	importedTplIDs   map[string]int             // imported template hash to object ID
	importedPages    map[int]*importedPDFPage   // native imported PDF pages
	importedPageSeq  int                        // next native imported PDF page ID
	buffer           fmtBuffer                  // buffer holding in-memory PDF
	pages            []*bytes.Buffer            // slice[page] of page content; 1-based
	state            int                        // current document state
	compress         bool                       // compression flag
	compressLevel    int                        // zlib level for compressed streams
	k                float64                    // scale factor (number of points in user unit)
	defOrientation   string                     // default orientation
	curOrientation   string                     // current orientation
	stdPageSizes     map[string]Size            // standard page sizes
	defPageSize      Size                       // default page size
	defPageBoxes     map[string]PageBox         // default page size
	curPageSize      Size                       // current page size
	curRotation      int                        // current page rotation
	pageSizes        map[int]Size               // used for pages with non default sizes or orientations
	pageRotations    map[int]int                // used for pages with non-zero /Rotate values
	pageBoxes        map[int]map[string]PageBox // used to define the crop, trim, bleed and art boxes
	unitStr          string                     // unit of measure for all rendered objects except fonts
	wPt, hPt         float64                    // dimensions of current page in points
	w, h             float64                    // dimensions of current page in user unit
	lMargin          float64                    // left margin
	tMargin          float64                    // top margin
	rMargin          float64                    // right margin
	bMargin          float64                    // page break margin
	cMargin          float64                    // cell margin
	x, y             float64                    // current position in user unit
	lasth            float64                    // height of last printed cell
	lineWidth        float64                    // line width in user unit
	fontpath         string                     // path containing fonts
	fontLoader       FontLoader                 // used to load font files from arbitrary locations
	coreFonts        map[string]bool            // array of core font names
	fonts            map[string]fontDefinition  // array of used fonts
	fontFiles        map[string]fontFile        // array of font files
	diffs            []string                   // array of encoding differences
	fontFamily       string                     // current font family
	fontStyle        string                     // current font style
	underline        bool                       // underlining flag
	strikeout        bool                       // strike out flag
	currentFont      fontDefinition             // current font info
	fontSizePt       float64                    // current font size in points
	fontSize         float64                    // current font size in user unit
	ws               float64                    // word spacing
	images           map[string]*ImageInfo      // array of used images
	aliasMap         map[string]string          // map of alias->replacement
	pageLinks        [][]pageLink               // pageLinks[page][link], both 1-based
	links            []internalLink             // array of internal links
	attachments      []Attachment               // slice of content to embed globally
	pageAttachments  [][]annotationAttach       // 1-based array of annotation for file attachments (per page)
	outlines         []outlineEntry             // array of outlines
	outlineRoot      int                        // root of outlines
	autoPageBreak    bool                       // automatic page breaking
	acceptPageBreak  func() bool                // returns true to accept page break
	pageBreakTrigger float64                    // threshold used to trigger page breaks
	inHeader         bool                       // flag set when processing header
	headerFnc        func()                     // function provided by app and called to write header
	headerHomeMode   bool                       // set position to home after headerFnc is called
	inFooter         bool                       // flag set when processing footer
	footerFnc        func()                     // function provided by app and called to write footer
	footerFncLpi     func(bool)                 // function provided by app and called to write footer with last page flag
	zoomMode         string                     // zoom display mode
	layoutMode       string                     // layout display mode
	xmp              []byte                     // XMP metadata
	producer         string                     // producer
	title            string                     // title
	subject          string                     // subject
	author           string                     // author
	keywords         string                     // keywords
	creator          string                     // creator
	creationDate     time.Time                  // override for document CreationDate value
	modDate          time.Time                  // override for document ModDate value
	aliasNbPagesStr  string                     // alias for total number of pages
	pdfVersion       string                     // PDF version number
	fontDirStr       string                     // location of font definition files
	capStyle         int                        // line cap style: butt 0, round 1, square 2
	joinStyle        int                        // line segment join style: miter 0, round 1, bevel 2
	dashArray        []float64                  // dash array
	dashPhase        float64                    // dash phase
	blendList        []blendModeType            // slice[idx] of alpha transparency modes, 1-based
	blendMap         map[string]int             // map into blendList
	blendMode        string                     // current blend mode
	alpha            float64                    // current transpacency
	gradientList     []gradientType             // slice[idx] of gradient records
	clipNest         int                        // Number of active clipping contexts
	transformNest    int                        // Number of active transformation contexts
	err              error                      // Set if error occurs during life cycle of instance
	protect          protectType                // document protection structure
	layer            layerRecType               // manages optional layers in document
	catalogSort      bool                       // sort resource catalogs in document
	nJs              int                        // JavaScript object number
	javascript       *string                    // JavaScript code to include in the PDF
	colorFlag        bool                       // indicates whether fill and text colors are different
	color            struct {
		// Composite values of colors
		draw, fill, text pdfColor
	}
	spotColorMap           map[string]spotColorType // Map of named ink-based colors
	userUnderlineThickness float64                  // A custom user underline thickness multiplier.
}
