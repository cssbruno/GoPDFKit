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
}

// The phpOrderedIntMap structure and its methods are copyrighted 2019 by
// Arteom Korotkiy (Gmail: arteomkorotkiy).
// phpOrderedIntMap imitates a PHP untyped ordered map.
type phpOrderedIntMap struct {
	keySet   []any
	valueSet []int
}

// getIndex returns the position of key in the PHP-style array.
func (pa *phpOrderedIntMap) getIndex(key any) int {
	if pa == nil {
		return -1
	}
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

// put stores key and value in the PHP-style array.
func (pa *phpOrderedIntMap) put(key any, value int) {
	if pa == nil {
		return
	}
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

// delete removes key and its value from the PHP-style array.
func (pa *phpOrderedIntMap) delete(key any) {
	if pa == nil || pa.keySet == nil || pa.valueSet == nil {
		return
	}
	i := pa.getIndex(key)
	if i >= 0 {
		switch {
		case i == 0:
			pa.keySet = pa.keySet[1:]
			pa.valueSet = pa.valueSet[1:]
		case i == len(pa.keySet)-1:
			pa.keySet = pa.keySet[:len(pa.keySet)-1]
			pa.valueSet = pa.valueSet[:len(pa.valueSet)-1]
		default:
			pa.keySet = append(pa.keySet[:i], pa.keySet[i+1:]...)
			pa.valueSet = append(pa.valueSet[:i], pa.valueSet[i+1:]...)
		}
	}
}

// get returns the value for key in the PHP-style array.
func (pa *phpOrderedIntMap) get(key any) int {
	if pa == nil {
		return 0
	}
	i := pa.getIndex(key)
	if i >= 0 {
		return pa.valueSet[i]
	}
	return 0
}

// pop imitates PHP's array_pop function.
func (pa *phpOrderedIntMap) pop() {
	if pa == nil || len(pa.keySet) == 0 || len(pa.valueSet) == 0 {
		return
	}
	pa.keySet = pa.keySet[:len(pa.keySet)-1]
	pa.valueSet = pa.valueSet[:len(pa.valueSet)-1]
}

// arrayMerge imitates PHP's array_merge function.
func arrayMerge(arr1, arr2 *phpOrderedIntMap) *phpOrderedIntMap {
	if arr1 == nil && arr2 == nil {
		return &phpOrderedIntMap{
			keySet:   make([]any, 0),
			valueSet: make([]int, 0),
		}
	}
	if arr1 == nil {
		return &phpOrderedIntMap{
			keySet:   append([]any{}, arr2.keySet...),
			valueSet: append([]int{}, arr2.valueSet...),
		}
	}
	if arr2 == nil {
		return &phpOrderedIntMap{
			keySet:   append([]any{}, arr1.keySet...),
			valueSet: append([]int{}, arr1.valueSet...),
		}
	}
	answer := phpOrderedIntMap{
		keySet:   append([]any{}, arr1.keySet...),
		valueSet: append([]int{}, arr1.valueSet...),
	}
	for i := 0; i < len(arr2.keySet); i++ {
		if arr2.keySet[i] == "interval" {
			if arr1.getIndex("interval") < 0 {
				answer.put("interval", arr2.valueSet[i])
			}
		} else {
			answer.put(nil, arr2.valueSet[i])
		}
	}
	return &answer
}
