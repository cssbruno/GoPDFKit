// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"hash"
	"time"
)

type documentState uint8

const (
	documentStateUnopened documentState = iota
	documentStateOpen
	documentStatePageOpen
	documentStateClosed
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

func (state *pdfSerializationState) allocateObject(offset int) int {
	state.n++
	state.recordObject(state.n, offset)
	return state.n
}

func (state *pdfSerializationState) recordObject(objectNumber, offset int) {
	for len(state.offsets) <= objectNumber {
		state.offsets = append(state.offsets, 0)
	}
	state.offsets[objectNumber] = offset
}

// resourceOwnershipState owns the resource registry and identifiers allocated
// for imported pages. Keeping it separate makes resource lifetime explicit
// without changing the Document facade.
type resourceOwnershipState struct {
	resources           *resourceStore
	importedPageSeq     int
	fontpath            string
	fontLoader          FontLoader
	resourceLoader      ResourceLoader
	utf8FontPathCache   map[string]utf8FontPathInfo
	utf8FontFileCache   map[sharedUTF8FontFileCacheKey]cachedUTF8Font
	fontCache           *FontCache
	resourceCachePolicy ResourceCachePolicy
	coreFonts           map[string]bool
	diffs               []string
	stringWidthCache    map[stringWidthCacheKey]int
	stringWidthKeys     []stringWidthCacheKey
	stringWidthKeyNext  int
	imageCache          *ImageCache
	attachments         []Attachment
	maxAttachmentBytes  int64
	pageAttachments     [][]annotationAttach
}

// pageGeometryState owns page dimensions, boxes, rotations, and unit
// conversion. Rendering state such as margins and the current cursor remains
// directly on Document because it changes continuously while content is drawn.
type pageGeometryState struct {
	page           int
	k              float64
	defOrientation string
	curOrientation string
	stdPageSizes   map[string]Size
	defPageSize    Size
	defPageBoxes   map[string]PageBox
	curPageSize    Size
	curRotation    int
	pageSizes      map[int]Size
	pageRotations  map[int]int
	pageBoxes      map[int]map[string]PageBox
	unitStr        string
	wPt, hPt       float64
	w, h           float64
}

// documentMetadataState owns catalog presentation, standards metadata, and
// descriptive information serialized into the PDF.
type documentMetadataState struct {
	zoomMode           string
	layoutMode         string
	xmp                []byte
	nXmp               int
	compliance         ComplianceMetadata
	outputIntent       outputIntent
	nOutputIntentICC   int
	tagged             taggedPDFState
	producer           string
	title              string
	subject            string
	author             string
	keywords           string
	creator            string
	creationDate       time.Time
	modDate            time.Time
	pdfVersion         string
	catalogSort        bool
	signatureFieldName string
}

// documentPolicyState owns production limits, security gates, output defaults,
// diagnostics, and encryption configuration.
type documentPolicyState struct {
	limits            Limits
	limitsSet         bool
	securityPolicy    SecurityPolicy
	securityPolicySet bool
	outputPolicy      OutputPolicy
	hooks             Hooks
	protect           protectType
}

// Document represents a single PDF document under construction.
//
// A Document is not safe for concurrent use. Serialize calls that mutate or
// render it, and create a separate Document for each independently generated
// PDF. Reusable inputs such as CompiledHTML, ImageCache, and FontCache may be
// shared across documents according to their own concurrency contracts.
type Document struct {
	pdfSerializationState
	resourceOwnershipState
	pageGeometryState
	documentMetadataState
	documentPolicyState

	isCurrentUTF8 bool // whether the current font uses UTF-8 mode
	isRTL         bool // whether right-to-left mode is enabled

	state         documentState // current document lifecycle state
	compress      bool          // compression flag
	compressLevel int           // zlib level for compressed streams

	pageCompressionWorkers         int // async page compression workers; 0 disables
	attachmentCompressionWorkers   int // async attachment compression workers; 0 disables
	compressionTinyStreamThreshold int // streams below this byte size are left uncompressed

	lMargin            float64                 // left margin
	tMargin            float64                 // top margin
	rMargin            float64                 // right margin
	bMargin            float64                 // page break margin
	cMargin            float64                 // cell margin
	x, y               float64                 // current position in user unit
	lasth              float64                 // height of last printed cell
	lineWidth          float64                 // line width in user unit
	fontFamily         string                  // current font family
	fontStyle          string                  // current font style
	underline          bool                    // underlining flag
	strikeout          bool                    // strikeout flag
	currentFont        fontDefinition          // current font info
	fontSizePt         float64                 // current font size in points
	fontSize           float64                 // current font size in user unit
	ws                 float64                 // word spacing
	contentScratch     []byte                  // bounded scratch for one content command
	aliasMap           map[string]string       // map of alias->replacement
	aliasPairs         []aliasReplacementBytes // compiled alias replacements
	aliasPairsDirty    bool                    // whether aliasPairs needs rebuilding
	aliasNeedles       [][]byte                // compiled alias search terms
	aliasNeedleStrings []string                // string form of alias search terms
	aliasNeedlesDirty  bool                    // whether aliasNeedles needs rebuilding
	aliasPages         []bool                  // pages that may contain aliases; 1-based
	pageLinks          [][]pageLink            // pageLinks[page][link], both 1-based
	links              []internalLink          // array of internal links
	outlines           []outlineEntry          // list of outlines
	outlineRoot        int                     // root of outlines
	autoPageBreak      bool                    // automatic page breaking
	acceptPageBreak    func() bool             // returns true to accept page break
	pageBreakTrigger   float64                 // threshold used to trigger page breaks
	inHeader           bool                    // flag set when processing header
	headerFnc          func()                  // function provided by app to write header
	headerHomeMode     bool                    // set position to home after headerFnc is called
	inFooter           bool                    // flag set when processing footer
	footerFnc          func()                  // function provided by app to write footer
	footerFncLpi       func(bool)              // function provided by app to write footer with last page flag
	aliasNbPagesStr    string                  // alias for total number of pages
	fontDirStr         string                  // location of font definition files
	capStyle           int                     // line cap style: butt 0, round 1, square 2
	joinStyle          int                     // line segment join style: miter 0, round 1, bevel 2
	dashArray          []float64               // dash array
	dashPhase          float64                 // dash phase
	blendList          []blendModeType         // slice[idx] of alpha transparency modes, 1-based
	blendMap           map[string]int          // map into blendList
	blendMode          string                  // current blend mode
	alpha              float64                 // current transparency
	gradientList       []gradientType          // slice[idx] of gradient records
	clipNest           int                     // number of active clipping contexts
	transformNest      int                     // number of active transformation contexts
	err                error                   // set if an error occurs during the instance lifecycle
	layer              layerRecType            // manages optional layers in document
	colorFlag          bool                    // indicates whether fill and text colors are different
	color              struct {
		// Composite values of colors
		draw, fill, text pdfColor
	}
	spotColorMap           map[string]spotColorType // map of named ink-based colors
	userUnderlineThickness float64                  // custom underline thickness multiplier
}
