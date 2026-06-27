// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"errors"
	"fmt"
	"maps"
)

// createTemplate creates a template, copying graphics settings from a Document
// when one is provided.
func createTemplate(corner Point, size Size, orientationStr, unitStr, fontDirStr string, fn func(*Tpl), copyFrom *Document) Template {
	sizeStr := ""

	pdf := documentNew(orientationStr, unitStr, sizeStr, fontDirStr, size)
	if pdf == nil || pdf.err != nil {
		return nil
	}
	tpl := Tpl{*pdf}
	if copyFrom != nil {
		tpl.loadParamsFromDocument(copyFrom)
	}
	tpl.AddPage()
	if fn != nil {
		fn(&tpl)
	}

	bytes := make([][]byte, len(tpl.pages))
	// Skip the first page because it is always empty.
	for x := 1; x < len(bytes); x++ {
		bytes[x] = tpl.pages[x].Bytes()
	}

	templates := make([]Template, 0, len(tpl.templates))
	for _, key := range templateKeyList(tpl.templates, true) {
		templates = append(templates, tpl.templates[key])
	}
	images := tpl.images

	template := DocumentTpl{corner, size, bytes, images, templates, tpl.page}
	return &template
}

// DocumentTpl is a concrete implementation of the Template interface.
type DocumentTpl struct {
	corner    Point
	size      Size
	bytes     [][]byte
	images    map[string]*ImageInfo
	templates []Template
	page      int
}

const (
	maxTemplateChildren        = 1024
	maxTemplateDepth           = 128
	maxTemplateImages          = 4096
	maxTemplatePageBytes       = 16 * 1024 * 1024
	maxTemplatePages           = 1000
	maxTemplateSerializedBytes = 16 * 1024 * 1024
)

// ID returns the global template identifier.
func (t *DocumentTpl) ID() string {
	return fmt.Sprintf("%x", sha1.Sum(t.Bytes()))
}

// Size returns the bounding dimensions of this template.
func (t *DocumentTpl) Size() (corner Point, size Size) {
	return t.corner, t.size
}

// Bytes returns the template data, not including resources.
func (t *DocumentTpl) Bytes() []byte {
	if t.page <= 0 || t.page >= len(t.bytes) {
		return nil
	}
	return t.bytes[t.page]
}

// FromPage creates a new template from a specific page.
func (t *DocumentTpl) FromPage(page int) (Template, error) {
	// Pages start at 1.
	if page < 1 {
		return nil, errors.New("pages start at 1; no template will have a page 0")
	}

	if page > t.NumPages() {
		return nil, fmt.Errorf("template does not have a page %d", page)
	}
	// If it is already pointing to the correct page, there is no need to create
	// a new template.
	if t.page == page {
		return t, nil
	}

	t2 := *t
	t2.page = page
	return &t2, nil
}

// FromPages creates a template slice with all the pages within a template.
func (t *DocumentTpl) FromPages() []Template {
	p := make([]Template, t.NumPages())
	for x := 1; x <= t.NumPages(); x++ {
		// The only error is from accessing a nonexistent template, which cannot
		// happen here.
		p[x-1], _ = t.FromPage(x)
	}

	return p
}

// Images returns the images used in this template.
func (t *DocumentTpl) Images() map[string]*ImageInfo {
	return t.images
}

// Templates returns the templates used in this template.
func (t *DocumentTpl) Templates() []Template {
	return t.templates
}

// NumPages returns the number of available pages within the template. Use
// FromPage or FromPages to access that content.
func (t *DocumentTpl) NumPages() int {
	// The first page is empty to make pages begin at one.
	return len(t.bytes) - 1
}

// Serialize turns a template into a byte slice for later deserialization.
func (t *DocumentTpl) Serialize() ([]byte, error) {
	b := new(bytes.Buffer)
	enc := gob.NewEncoder(b)
	err := enc.Encode(t)

	return b.Bytes(), err
}

// DeserializeTemplate creates a template from a previously serialized template.
func DeserializeTemplate(b []byte) (Template, error) {
	if len(b) > maxTemplateSerializedBytes {
		return nil, errors.New("serialized template exceeds maximum size")
	}
	tpl := new(DocumentTpl)
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	err := dec.Decode(tpl)
	if err == nil {
		err = tpl.validate()
	}
	return tpl, err
}

// childrenImages returns the next layer of child images without recursing into
// grandchildren. It applies the template namespace to keys to avoid collisions.
// See UseTemplateScaled.
func (t *DocumentTpl) childrenImages() map[string]*ImageInfo {
	images := make(map[string]*ImageInfo)

	for _, child := range t.templates {
		if invalidTemplate(child) {
			continue
		}
		for key, image := range child.Images() {
			images[sprintf("t%s-%s", child.ID(), key)] = image
		}
	}

	return images
}

// childrenTemplates returns the next layer of child templates without
// recursing into grandchildren.
func (t *DocumentTpl) childrenTemplates() []Template {
	templates := make([]Template, 0)
	for _, child := range t.templates {
		if invalidTemplate(child) {
			continue
		}
		templates = append(templates, child.Templates()...)
	}

	return templates
}

func (t *DocumentTpl) topLevelTemplates() []Template {
	nestedIDs := make(map[string]bool)
	for _, child := range t.childrenTemplates() {
		if !invalidTemplate(child) {
			nestedIDs[child.ID()] = true
		}
	}

	templates := make([]Template, 0, len(t.templates))
	for _, child := range t.templates {
		if invalidTemplate(child) || nestedIDs[child.ID()] {
			continue
		}
		templates = append(templates, child)
	}

	return templates
}

func (t *DocumentTpl) ownImages() map[string]*ImageInfo {
	childImages := t.childrenImages()
	images := make(map[string]*ImageInfo)
	for key, image := range t.images {
		if _, inherited := childImages[key]; !inherited {
			images[key] = image
		}
	}
	return images
}

// GobEncode encodes the receiving template into a byte buffer. Use GobDecode
// to decode the byte buffer back to a template.
func (t *DocumentTpl) GobEncode() ([]byte, error) {
	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)

	fields := []any{
		t.topLevelTemplates(),
		t.ownImages(),
		t.corner,
		t.size,
		t.bytes,
		t.page,
	}
	for _, field := range fields {
		if err := encoder.Encode(field); err != nil {
			return nil, err
		}
	}

	return w.Bytes(), nil
}

// GobDecode decodes the specified byte buffer into the receiving template.
func (t *DocumentTpl) GobDecode(buf []byte) error {
	r := bytes.NewBuffer(buf)
	decoder := gob.NewDecoder(r)

	templates, err := decodeTemplateList(decoder)
	if err != nil {
		return err
	}
	t.templates = templates

	childImages := t.childrenImages()
	t.templates = append(t.childrenTemplates(), t.templates...)

	if err := decoder.Decode(&t.images); err != nil {
		return err
	}
	if t.images == nil {
		t.images = make(map[string]*ImageInfo)
	}
	maps.Copy(t.images, childImages)

	fields := []any{&t.corner, &t.size, &t.bytes, &t.page}
	for _, field := range fields {
		if err := decoder.Decode(field); err != nil {
			return err
		}
	}

	return nil
}

func decodeTemplateList(decoder *gob.Decoder) ([]Template, error) {
	children := make([]*DocumentTpl, 0)
	if err := decoder.Decode(&children); err != nil {
		return nil, err
	}

	templates := make([]Template, 0, len(children))
	for _, child := range children {
		if child == nil {
			return nil, errors.New("invalid nil child template")
		}
		templates = append(templates, child)
	}
	return templates, nil
}

func (t *DocumentTpl) validate() error {
	return t.validateDepth(0, make(map[*DocumentTpl]bool))
}

func (t *DocumentTpl) validateDepth(depth int, visiting map[*DocumentTpl]bool) error {
	if depth > maxTemplateDepth {
		return errors.New("template nesting exceeds maximum size")
	}
	if t == nil {
		return errors.New("invalid nil template")
	}
	if visiting[t] {
		return errors.New("template nesting contains a cycle")
	}
	visiting[t] = true
	defer func() { visiting[t] = false }()

	if t.page <= 0 || t.page >= len(t.bytes) {
		return errors.New("invalid template page index")
	}
	if len(t.bytes)-1 > maxTemplatePages {
		return errors.New("template page count exceeds maximum size")
	}
	if len(t.images) > maxTemplateImages {
		return errors.New("template image count exceeds maximum size")
	}
	if len(t.templates) > maxTemplateChildren {
		return errors.New("template child count exceeds maximum size")
	}
	for _, page := range t.bytes {
		if len(page) > maxTemplatePageBytes {
			return errors.New("template page content exceeds maximum size")
		}
	}
	for name, img := range t.images {
		if err := img.validForPDF(); err != nil {
			return fmt.Errorf("invalid template image %s: %w", name, err)
		}
	}
	for _, tpl := range t.templates {
		if invalidTemplate(tpl) {
			return errors.New("invalid nil child template")
		}
		if child, ok := tpl.(*DocumentTpl); ok {
			if err := child.validateDepth(depth+1, visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

func invalidTemplate(tpl Template) bool {
	if tpl == nil {
		return true
	}
	switch t := tpl.(type) {
	case *DocumentTpl:
		return t == nil
	default:
		return false
	}
}

// Tpl is a Document used for writing a template. It has most of the facilities of
// a Document, but cannot add more pages. Tpl is used directly only during the
// limited time a template is writable.
type Tpl struct {
	Document
}

func (t *Tpl) loadParamsFromDocument(f *Document) {
	t.compress = false

	t.k = f.k
	t.x = f.x
	t.y = f.y
	t.lineWidth = f.lineWidth
	t.capStyle = f.capStyle
	t.joinStyle = f.joinStyle

	t.color.draw = f.color.draw
	t.color.fill = f.color.fill
	t.color.text = f.color.text

	t.fonts = f.fonts
	t.currentFont = f.currentFont
	t.fontFamily = f.fontFamily
	t.fontSize = f.fontSize
	t.fontSizePt = f.fontSizePt
	t.fontStyle = f.fontStyle
	t.ws = f.ws

	maps.Copy(t.images, f.images)
}
