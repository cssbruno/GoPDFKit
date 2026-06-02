/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

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

// SpotColorType specifies a named spot color value
type spotColorType struct {
	id, objID int
	cmyk      cmykColorType
}

// CMYKColorType specifies an ink-based CMYK color value
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
}

type internalLink struct {
	page int
	y    float64
}

// outlineEntry is used for a sidebar outline of bookmarks
type outlineEntry struct {
	text                                   string
	level, parent, first, last, next, prev int
	y                                      float64
	p                                      int
}

// The phpOrderedIntMap structure and its methods are copyrighted 2019 by Arteom Korotkiy (Gmail: arteomkorotkiy).
// Imitation of untyped Map Array
type phpOrderedIntMap struct {
	keySet   []any
	valueSet []int
}

// Get position of key=>value in PHP Array
func (pa *phpOrderedIntMap) getIndex(key any) int {
	if key != nil {
		for i, mKey := range pa.keySet {
			if mKey == key {
				return i
			}
		}
		return -1
	}
	return -1
}

// Put key=>value in PHP Array
func (pa *phpOrderedIntMap) put(key any, value int) {
	if key == nil {
		var i int
		for n := 0; ; n++ {
			i = pa.getIndex(n)
			if i < 0 {
				key = n
				break
			}
		}
		pa.keySet = append(pa.keySet, key)
		pa.valueSet = append(pa.valueSet, value)
	} else {
		i := pa.getIndex(key)
		if i < 0 {
			pa.keySet = append(pa.keySet, key)
			pa.valueSet = append(pa.valueSet, value)
		} else {
			pa.valueSet[i] = value
		}
	}
}

// Delete value in PHP Array
func (pa *phpOrderedIntMap) delete(key any) {
	if pa == nil || pa.keySet == nil || pa.valueSet == nil {
		return
	}
	i := pa.getIndex(key)
	if i >= 0 {
		if i == 0 {
			pa.keySet = pa.keySet[1:]
			pa.valueSet = pa.valueSet[1:]
		} else if i == len(pa.keySet)-1 {
			pa.keySet = pa.keySet[:len(pa.keySet)-1]
			pa.valueSet = pa.valueSet[:len(pa.valueSet)-1]
		} else {
			pa.keySet = append(pa.keySet[:i], pa.keySet[i+1:]...)
			pa.valueSet = append(pa.valueSet[:i], pa.valueSet[i+1:]...)
		}
	}
}

// Get value from PHP Array
func (pa *phpOrderedIntMap) get(key any) int {
	i := pa.getIndex(key)
	if i >= 0 {
		return pa.valueSet[i]
	}
	return 0
}

// Imitation of PHP function pop()
func (pa *phpOrderedIntMap) pop() {
	pa.keySet = pa.keySet[:len(pa.keySet)-1]
	pa.valueSet = pa.valueSet[:len(pa.valueSet)-1]
}

// Imitation of PHP function array_merge()
func arrayMerge(arr1, arr2 *phpOrderedIntMap) *phpOrderedIntMap {
	answer := phpOrderedIntMap{}
	if arr1 == nil && arr2 == nil {
		answer = phpOrderedIntMap{
			make([]any, 0),
			make([]int, 0),
		}
	} else if arr2 == nil {
		answer.keySet = arr1.keySet[:]
		answer.valueSet = arr1.valueSet[:]
	} else if arr1 == nil {
		answer.keySet = arr2.keySet[:]
		answer.valueSet = arr2.valueSet[:]
	} else {
		answer.keySet = arr1.keySet[:]
		answer.valueSet = arr1.valueSet[:]
		for i := 0; i < len(arr2.keySet); i++ {
			if arr2.keySet[i] == "interval" {
				if arr1.getIndex("interval") < 0 {
					answer.put("interval", arr2.valueSet[i])
				}
			} else {
				answer.put(nil, arr2.valueSet[i])
			}
		}
	}
	return &answer
}
