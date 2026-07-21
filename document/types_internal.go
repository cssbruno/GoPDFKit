// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

type blendModeType struct {
	strokeStr, fillStr, modeStr string
	objNum                      int
}

type gradientStopType struct {
	offset float64
	clrStr string
}

type gradientType struct {
	tp                int // 2: linear, 3: radial
	clr1Str, clr2Str  string
	stops             []gradientStopType
	x1, y1, x2, y2, r float64
	objNum            int
}
type colorMode int

const (
	colorModeRGB colorMode = iota
	colorModeSpot
	colorModeCMYK
)

type pdfColor struct {
	r, g, b    float64
	ir, ig, ib int
	mode       colorMode
	spotStr    string // name of current spot color
	gray       bool
	str        string
}

// spotColorType specifies a named spot color value.
type spotColorType struct {
	id, objID int
	cmyk      cmykColorType
}

// cmykColorType specifies an ink-based CMYK color value.
type cmykColorType struct {
	c, m, y, k byte // 0% to 100%
}
type fontFile struct {
	length1, length2 int64
	n                int
	embedded         bool
	content          []byte
	fontType         string
}

type pageLink struct {
	x, y, wd, ht float64
	link         int    // Auto-generated internal link ID or...
	linkStr      string // ...application-provided external link string
	objNum       int
	structParent int
	structElem   *taggedElement
}

type internalLink struct {
	page int
	y    float64
}

// outlineEntry is used for a sidebar outline of bookmarks.
type outlineEntry struct {
	text                                   string
	level, parent, first, last, next, prev int
	y                                      float64
	p                                      int
	utf8                                   bool
}
