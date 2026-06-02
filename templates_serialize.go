/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"errors"
	"fmt"
	"maps"
	"reflect"
)

// newTpl creates a template, copying graphics settings from a template if one is given
func newTpl(corner Point, size Size, orientationStr, unitStr, fontDirStr string, fn func(*Tpl), copyFrom *Fpdf) Template {
	sizeStr := ""

	fpdf := fpdfNew(orientationStr, unitStr, sizeStr, fontDirStr, size)
	tpl := Tpl{*fpdf}
	if copyFrom != nil {
		tpl.loadParamsFromFpdf(copyFrom)
	}
	tpl.Fpdf.AddPage()
	fn(&tpl)

	bytes := make([][]byte, len(tpl.Fpdf.pages))
	// skip the first page as it will always be empty
	for x := 1; x < len(bytes); x++ {
		bytes[x] = tpl.Fpdf.pages[x].Bytes()
	}

	templates := make([]Template, 0, len(tpl.Fpdf.templates))
	for _, key := range templateKeyList(tpl.Fpdf.templates, true) {
		templates = append(templates, tpl.Fpdf.templates[key])
	}
	images := tpl.Fpdf.images

	template := FpdfTpl{corner, size, bytes, images, templates, tpl.Fpdf.page}
	return &template
}

// FpdfTpl is a concrete implementation of the Template interface.
type FpdfTpl struct {
	corner    Point
	size      Size
	bytes     [][]byte
	images    map[string]*ImageInfo
	templates []Template
	page      int
}

// ID returns the global template identifier
func (t *FpdfTpl) ID() string {
	return fmt.Sprintf("%x", sha1.Sum(t.Bytes()))
}

// Size gives the bounding dimensions of this template
func (t *FpdfTpl) Size() (corner Point, size Size) {
	return t.corner, t.size
}

// Bytes returns the actual template data, not including resources
func (t *FpdfTpl) Bytes() []byte {
	if t.page <= 0 || t.page >= len(t.bytes) {
		return nil
	}
	return t.bytes[t.page]
}

// FromPage creates a new template from a specific Page
func (t *FpdfTpl) FromPage(page int) (Template, error) {
	// pages start at 1
	if page == 0 {
		return nil, errors.New("Pages start at 1 No template will have a page 0")
	}

	if page > t.NumPages() {
		return nil, fmt.Errorf("The template does not have a page %d", page)
	}
	// if it is already pointing to the correct page
	// there is no need to create a new template
	if t.page == page {
		return t, nil
	}

	t2 := *t
	t2.page = page
	return &t2, nil
}

// FromPages creates a template slice with all the pages within a template.
func (t *FpdfTpl) FromPages() []Template {
	p := make([]Template, t.NumPages())
	for x := 1; x <= t.NumPages(); x++ {
		// the only error is when accessing a
		// non existing template... that can't happen
		// here
		p[x-1], _ = t.FromPage(x)
	}

	return p
}

// Images returns a list of the images used in this template
func (t *FpdfTpl) Images() map[string]*ImageInfo {
	return t.images
}

// Templates returns a list of templates used in this template
func (t *FpdfTpl) Templates() []Template {
	return t.templates
}

// NumPages returns the number of available pages within the template. Look at FromPage and FromPages on access to that content.
func (t *FpdfTpl) NumPages() int {
	// the first page is empty to
	// make the pages begin at one
	return len(t.bytes) - 1
}

// Serialize turns a template into a byte string for later deserialization
func (t *FpdfTpl) Serialize() ([]byte, error) {
	b := new(bytes.Buffer)
	enc := gob.NewEncoder(b)
	err := enc.Encode(t)

	return b.Bytes(), err
}

// DeserializeTemplate creaties a template from a previously serialized
// template
func DeserializeTemplate(b []byte) (Template, error) {
	tpl := new(FpdfTpl)
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	err := dec.Decode(tpl)
	if err == nil {
		err = tpl.validate()
	}
	return tpl, err
}

// childrenImages returns the next layer of children images, it doesn't dig into
// children of children. Applies template namespace to keys to ensure
// no collisions. See UseTemplateScaled
func (t *FpdfTpl) childrenImages() map[string]*ImageInfo {
	childrenImgs := make(map[string]*ImageInfo)

	for x := 0; x < len(t.templates); x++ {
		if invalidTemplate(t.templates[x]) {
			continue
		}
		imgs := t.templates[x].Images()
		for key, val := range imgs {
			name := sprintf("t%s-%s", t.templates[x].ID(), key)
			childrenImgs[name] = val
		}
	}

	return childrenImgs
}

// childrensTemplates returns the next layer of children templates, it doesn't dig into
// children of children.
func (t *FpdfTpl) childrensTemplates() []Template {
	childrenTmpls := make([]Template, 0)

	for x := 0; x < len(t.templates); x++ {
		if invalidTemplate(t.templates[x]) {
			continue
		}
		tmpls := t.templates[x].Templates()
		childrenTmpls = append(childrenTmpls, tmpls...)
	}

	return childrenTmpls
}

// GobEncode encodes the receiving template into a byte buffer. Use GobDecode
// to decode the byte buffer back to a template.
func (t *FpdfTpl) GobEncode() ([]byte, error) {
	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)

	childrensTemplates := t.childrensTemplates()
	firstClassTemplates := make([]Template, 0)

found_continue:
	for x := 0; x < len(t.templates); x++ {
		for y := range childrensTemplates {
			if childrensTemplates[y].ID() == t.templates[x].ID() {
				continue found_continue
			}
		}

		firstClassTemplates = append(firstClassTemplates, t.templates[x])
	}
	err := encoder.Encode(firstClassTemplates)

	childrenImgs := t.childrenImages()
	firstClassImgs := make(map[string]*ImageInfo)

	for key, img := range t.images {
		if _, ok := childrenImgs[key]; !ok {
			firstClassImgs[key] = img
		}
	}

	if err == nil {
		err = encoder.Encode(firstClassImgs)
	}
	if err == nil {
		err = encoder.Encode(t.corner)
	}
	if err == nil {
		err = encoder.Encode(t.size)
	}
	if err == nil {
		err = encoder.Encode(t.bytes)
	}
	if err == nil {
		err = encoder.Encode(t.page)
	}

	return w.Bytes(), err
}

// GobDecode decodes the specified byte buffer into the receiving template.
func (t *FpdfTpl) GobDecode(buf []byte) error {
	r := bytes.NewBuffer(buf)
	decoder := gob.NewDecoder(r)

	firstClassTemplates := make([]*FpdfTpl, 0)
	err := decoder.Decode(&firstClassTemplates)
	t.templates = make([]Template, len(firstClassTemplates))

	if err == nil {
		for x := 0; x < len(t.templates); x++ {
			if firstClassTemplates[x] == nil {
				return fmt.Errorf("invalid nil child template")
			}
			t.templates[x] = Template(firstClassTemplates[x])
		}
	}

	var firstClassImages map[string]*ImageInfo
	if err == nil {
		firstClassImages = t.childrenImages()
	}

	if err == nil {
		t.templates = append(t.childrensTemplates(), t.templates...)
	}

	t.images = make(map[string]*ImageInfo)
	if err == nil {
		err = decoder.Decode(&t.images)
	}

	maps.Copy(t.images, firstClassImages)

	if err == nil {
		err = decoder.Decode(&t.corner)
	}
	if err == nil {
		err = decoder.Decode(&t.size)
	}
	if err == nil {
		err = decoder.Decode(&t.bytes)
	}
	if err == nil {
		err = decoder.Decode(&t.page)
	}

	return err
}

func (t *FpdfTpl) validate() error {
	if t.page <= 0 || t.page >= len(t.bytes) {
		return fmt.Errorf("invalid template page index")
	}
	for name, img := range t.images {
		if err := img.validForPDF(); err != nil {
			return fmt.Errorf("invalid template image %s: %w", name, err)
		}
	}
	for _, tpl := range t.templates {
		if invalidTemplate(tpl) {
			return fmt.Errorf("invalid nil child template")
		}
	}
	return nil
}

func invalidTemplate(tpl Template) bool {
	if tpl == nil {
		return true
	}
	value := reflect.ValueOf(tpl)
	return value.Kind() == reflect.Ptr && value.IsNil()
}

// Tpl is an Fpdf used for writing a template. It has most of the facilities of
// an Fpdf, but cannot add more pages. Tpl is used directly only during the
// limited time a template is writable.
type Tpl struct {
	Fpdf
}

func (t *Tpl) loadParamsFromFpdf(f *Fpdf) {
	t.Fpdf.compress = false

	t.Fpdf.k = f.k
	t.Fpdf.x = f.x
	t.Fpdf.y = f.y
	t.Fpdf.lineWidth = f.lineWidth
	t.Fpdf.capStyle = f.capStyle
	t.Fpdf.joinStyle = f.joinStyle

	t.Fpdf.color.draw = f.color.draw
	t.Fpdf.color.fill = f.color.fill
	t.Fpdf.color.text = f.color.text

	t.Fpdf.fonts = f.fonts
	t.Fpdf.currentFont = f.currentFont
	t.Fpdf.fontFamily = f.fontFamily
	t.Fpdf.fontSize = f.fontSize
	t.Fpdf.fontSizePt = f.fontSizePt
	t.Fpdf.fontStyle = f.fontStyle
	t.Fpdf.ws = f.ws

	maps.Copy(t.Fpdf.images, f.images)
}
