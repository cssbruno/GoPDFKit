// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"hash"
	"time"
)

// pdfSerializationState owns data that exists only to assemble final PDF
// objects. It is embedded so the public Document facade and its long-standing
// internal call sites remain source-compatible while serialization has a
// concrete home of its own.
type pdfSerializationState struct {
	n                 int             // current object number
	offsets           []int           // object offsets
	buffer            fmtBuffer       // in-memory final PDF buffer
	outputSink        *pdfOutputSink  // optional final PDF serialization sink
	streamedOutput    bool            // final bytes were streamed without retaining buffer
	fileIDHash        hash.Hash       // incremental hash for file identifiers
	pages             []*bytes.Buffer // page content; 1-based
	pageObjectNumbers []int           // PDF page object numbers; 1-based
}

// resourceOwnershipState owns the resource registry and identifiers allocated
// for imported pages. Keeping it separate makes resource lifetime explicit
// without changing the Document facade.
type resourceOwnershipState struct {
	resources       *resourceStore
	importedPageSeq int
}

// Document represents a single PDF document under construction.
type Document struct {
	pdfSerializationState
	resourceOwnershipState

	isCurrentUTF8 bool // whether the current font uses UTF-8 mode
	isRTL         bool // whether right-to-left mode is enabled
	page          int  // current page number

	state         int  // current document state
	compress      bool // compression flag
	compressLevel int  // zlib level for compressed streams

	pageCompressionWorkers         int // async page compression workers; 0 disables
	attachmentCompressionWorkers   int // async attachment compression workers; 0 disables
	compressionTinyStreamThreshold int // streams below this byte size are left uncompressed

	k                   float64                     // scale factor (number of points in user unit)
	defOrientation      string                      // default orientation
	curOrientation      string                      // current orientation
	stdPageSizes        map[string]Size             // standard page sizes
	defPageSize         Size                        // default page size
	defPageBoxes        map[string]PageBox          // default page boxes
	curPageSize         Size                        // current page size
	curRotation         int                         // current page rotation
	pageSizes           map[int]Size                // used for pages with non default sizes or orientations
	pageRotations       map[int]int                 // used for pages with non-zero /Rotate values
	pageBoxes           map[int]map[string]PageBox  // used to define the crop, trim, bleed and art boxes
	unitStr             string                      // unit of measure for all rendered objects except fonts
	wPt, hPt            float64                     // dimensions of current page in points
	w, h                float64                     // dimensions of current page in user unit
	lMargin             float64                     // left margin
	tMargin             float64                     // top margin
	rMargin             float64                     // right margin
	bMargin             float64                     // page break margin
	cMargin             float64                     // cell margin
	x, y                float64                     // current position in user unit
	lasth               float64                     // height of last printed cell
	lineWidth           float64                     // line width in user unit
	fontpath            string                      // path containing font resources
	fontLoader          FontLoader                  // used to load font files from arbitrary locations
	resourceLoader      ResourceLoader              // optional generalized resource loader
	utf8FontPathCache   map[string]utf8FontPathInfo // cached UTF-8 font path resolution
	utf8FontFileCache   map[sharedUTF8FontFileCacheKey]cachedUTF8Font
	fontCache           *FontCache              // explicit reusable UTF-8 font cache
	resourceCachePolicy ResourceCachePolicy     // file-backed resource cache behavior
	coreFonts           map[string]bool         // set of core font names
	diffs               []string                // list of encoding differences
	fontFamily          string                  // current font family
	fontStyle           string                  // current font style
	underline           bool                    // underlining flag
	strikeout           bool                    // strikeout flag
	currentFont         fontDefinition          // current font info
	fontSizePt          float64                 // current font size in points
	fontSize            float64                 // current font size in user unit
	stringWidthCache    map[string]int          // bounded cache of glyph-unit string widths for current document
	stringWidthKeys     []string                // insertion order for stringWidthCache eviction
	ws                  float64                 // word spacing
	imageCache          *ImageCache             // file-backed image cache for this document
	aliasMap            map[string]string       // map of alias->replacement
	aliasPairs          []aliasReplacementBytes // compiled alias replacements
	aliasPairsDirty     bool                    // whether aliasPairs needs rebuilding
	aliasNeedles        [][]byte                // compiled alias search terms
	aliasNeedleStrings  []string                // string form of alias search terms
	aliasNeedlesDirty   bool                    // whether aliasNeedles needs rebuilding
	aliasPages          []bool                  // pages that may contain aliases; 1-based
	pageLinks           [][]pageLink            // pageLinks[page][link], both 1-based
	links               []internalLink          // array of internal links
	attachments         []Attachment            // slice of content to embed globally
	maxAttachmentBytes  int64                   // largest attachment content accepted for embedding
	limits              Limits                  // optional production resource limits
	limitsSet           bool                    // whether limits were explicitly configured
	securityPolicy      SecurityPolicy          // optional security feature gates
	securityPolicySet   bool                    // whether securityPolicy is enforced
	outputPolicy        OutputPolicy            // optional output defaults
	hooks               Hooks                   // optional production diagnostics callbacks
	pageAttachments     [][]annotationAttach    // 1-based list of file attachment annotations per page
	outlines            []outlineEntry          // list of outlines
	outlineRoot         int                     // root of outlines
	autoPageBreak       bool                    // automatic page breaking
	acceptPageBreak     func() bool             // returns true to accept page break
	pageBreakTrigger    float64                 // threshold used to trigger page breaks
	inHeader            bool                    // flag set when processing header
	headerFnc           func()                  // function provided by app to write header
	headerHomeMode      bool                    // set position to home after headerFnc is called
	inFooter            bool                    // flag set when processing footer
	footerFnc           func()                  // function provided by app to write footer
	footerFncLpi        func(bool)              // function provided by app to write footer with last page flag
	zoomMode            string                  // zoom display mode
	layoutMode          string                  // layout display mode
	xmp                 []byte                  // XMP metadata
	nXmp                int                     // XMP metadata object number
	compliance          ComplianceMetadata      // standards metadata and catalog markers
	outputIntent        outputIntent            // document output color intent
	nOutputIntentICC    int                     // ICC output profile object number
	tagged              taggedPDFState          // tagged PDF structure tree state
	producer            string                  // producer
	title               string                  // title
	subject             string                  // subject
	author              string                  // author
	keywords            string                  // keywords
	creator             string                  // creator
	creationDate        time.Time               // override for document CreationDate value
	modDate             time.Time               // override for document ModDate value
	aliasNbPagesStr     string                  // alias for total number of pages
	pdfVersion          string                  // PDF version number
	fontDirStr          string                  // location of font definition files
	capStyle            int                     // line cap style: butt 0, round 1, square 2
	joinStyle           int                     // line segment join style: miter 0, round 1, bevel 2
	dashArray           []float64               // dash array
	dashPhase           float64                 // dash phase
	blendList           []blendModeType         // slice[idx] of alpha transparency modes, 1-based
	blendMap            map[string]int          // map into blendList
	blendMode           string                  // current blend mode
	alpha               float64                 // current transparency
	gradientList        []gradientType          // slice[idx] of gradient records
	clipNest            int                     // number of active clipping contexts
	transformNest       int                     // number of active transformation contexts
	err                 error                   // set if an error occurs during the instance lifecycle
	protect             protectType             // document protection structure
	layer               layerRecType            // manages optional layers in document
	catalogSort         bool                    // sort resource catalogs in document
	colorFlag           bool                    // indicates whether fill and text colors are different
	color               struct {
		// Composite values of colors
		draw, fill, text pdfColor
	}
	spotColorMap           map[string]spotColorType // map of named ink-based colors
	userUnderlineThickness float64                  // custom underline thickness multiplier
}
