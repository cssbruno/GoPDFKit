// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
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
		bytes[x] = append([]byte(nil), tpl.pages[x].Bytes()...)
	}

	templates := make([]Template, 0, len(tpl.templates))
	for _, key := range templateKeyList(tpl.templates, true) {
		templates = append(templates, tpl.templates[key])
	}
	images := tpl.images

	template := DocumentTpl{
		corner:    corner,
		size:      size,
		bytes:     bytes,
		images:    images,
		templates: templates,
		page:      tpl.page,
	}
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
	id        string
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
	if t.id == "" {
		t.id = fmt.Sprintf("%x", sha1.Sum(t.Bytes()))
	}
	return t.id
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
	t2.id = ""
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
	var w templateBinaryWriter
	w.writeRaw([]byte(templateBinaryMagic))
	if err := w.writeTemplate(t, 0); err != nil {
		return nil, err
	}
	return w.bytes(), nil
}

// DeserializeTemplate creates a template from a previously serialized template.
func DeserializeTemplate(b []byte) (Template, error) {
	if len(b) > maxTemplateSerializedBytes {
		return nil, errors.New("serialized template exceeds maximum size")
	}
	r := templateBinaryReader{data: b}
	if !bytes.Equal(r.readRawBytes(len(templateBinaryMagic)), []byte(templateBinaryMagic)) {
		return nil, errors.New("invalid serialized template header")
	}
	tpl, err := r.readTemplate(0)
	if err != nil {
		return nil, err
	}
	if err := tpl.validate(); err != nil {
		return nil, err
	}
	return tpl, nil
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
		childID := child.ID()
		for key, image := range child.Images() {
			images[sprintf("t%s-%s", childID, key)] = image
		}
	}

	return images
}

func (t *DocumentTpl) childImageKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, child := range t.templates {
		if invalidTemplate(child) {
			continue
		}
		childID := child.ID()
		for key := range child.Images() {
			keys[sprintf("t%s-%s", childID, key)] = true
		}
	}
	return keys
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
		if invalidTemplate(child) {
			continue
		}
		childID := child.ID()
		if nestedIDs[childID] {
			continue
		}
		templates = append(templates, child)
	}

	return templates
}

func (t *DocumentTpl) ownImages() map[string]*ImageInfo {
	childImageKeys := t.childImageKeys()
	images := make(map[string]*ImageInfo)
	for key, image := range t.images {
		if !childImageKeys[key] {
			images[key] = image
		}
	}
	return images
}

const templateBinaryMagic = "GPKTPL1\x00"

// GobEncode encodes the receiving template into a byte buffer. Use GobDecode
// to decode the byte buffer back to a template.
func (t *DocumentTpl) GobEncode() ([]byte, error) {
	return t.Serialize()
}

// GobDecode decodes the specified byte buffer into the receiving template.
func (t *DocumentTpl) GobDecode(buf []byte) error {
	if len(buf) > maxTemplateSerializedBytes {
		return errors.New("serialized template exceeds maximum size")
	}
	r := templateBinaryReader{data: buf}
	if !bytes.Equal(r.readRawBytes(len(templateBinaryMagic)), []byte(templateBinaryMagic)) {
		return errors.New("invalid serialized template header")
	}
	tpl, err := r.readTemplate(0)
	if err != nil {
		return err
	}
	*t = *tpl
	return nil
}

type templateBinaryWriter struct {
	buf bytes.Buffer
	err error
}

func (w *templateBinaryWriter) bytes() []byte {
	if w.err != nil {
		return nil
	}
	return w.buf.Bytes()
}

func (w *templateBinaryWriter) writeTemplate(t *DocumentTpl, depth int) error {
	if w.err != nil {
		return w.err
	}
	if t == nil {
		return errors.New("invalid nil template")
	}
	if depth > maxTemplateDepth {
		return errors.New("template nesting depth exceeded")
	}
	children := t.topLevelTemplates()
	w.writeUint(uint64(len(children)))
	for _, child := range children {
		childTpl, ok := child.(*DocumentTpl)
		if !ok || childTpl == nil {
			return errors.New("unsupported template implementation")
		}
		if err := w.writeTemplate(childTpl, depth+1); err != nil {
			return err
		}
	}
	w.writeImages(t.ownImages())
	w.writeFloat(t.corner.X)
	w.writeFloat(t.corner.Y)
	w.writeFloat(t.size.Wd)
	w.writeFloat(t.size.Ht)
	w.writeBytesList(t.bytes)
	w.writeInt(t.page)
	return w.err
}

func (w *templateBinaryWriter) writeImages(images map[string]*ImageInfo) {
	keys := make([]string, 0, len(images))
	for key := range images {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	w.writeUint(uint64(len(keys)))
	for _, key := range keys {
		w.writeString(key)
		w.writeImage(images[key])
	}
}

func (w *templateBinaryWriter) writeImage(info *ImageInfo) {
	if info == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeBytes(info.data)
	w.writeBytes(info.smask)
	w.writeInt(info.n)
	w.writeFloat(info.w)
	w.writeFloat(info.h)
	w.writeString(info.cs)
	w.writeBytes(info.pal)
	w.writeInt(info.bpc)
	w.writeString(info.f)
	w.writeString(info.dp)
	w.writeUint(uint64(len(info.trns)))
	for _, value := range info.trns {
		w.writeInt(value)
	}
	w.writeFloat(info.scale)
	w.writeFloat(info.dpi)
}

func (w *templateBinaryWriter) writeBytesList(values [][]byte) {
	w.writeUint(uint64(len(values)))
	for _, value := range values {
		w.writeBytes(value)
	}
}

func (w *templateBinaryWriter) writeString(value string) {
	w.writeBytes([]byte(value))
}

func (w *templateBinaryWriter) writeRaw(value []byte) {
	if w.err == nil {
		_, w.err = w.buf.Write(value)
	}
}

func (w *templateBinaryWriter) writeBytes(value []byte) {
	w.writeUint(uint64(len(value)))
	if w.err == nil {
		_, w.err = w.buf.Write(value)
	}
}

func (w *templateBinaryWriter) writeBool(value bool) {
	if value {
		w.writeByte(1)
	} else {
		w.writeByte(0)
	}
}

func (w *templateBinaryWriter) writeInt(value int) {
	w.writeUint(uint64(int64(value)))
}

func (w *templateBinaryWriter) writeFloat(value float64) {
	w.writeUint(math.Float64bits(value))
}

func (w *templateBinaryWriter) writeUint(value uint64) {
	var scratch [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(scratch[:], value)
	if w.err == nil {
		_, w.err = w.buf.Write(scratch[:n])
	}
}

func (w *templateBinaryWriter) writeByte(value byte) {
	if w.err == nil {
		w.err = w.buf.WriteByte(value)
	}
}

type templateBinaryReader struct {
	data []byte
	pos  int
	err  error
}

func (r *templateBinaryReader) readTemplate(depth int) (*DocumentTpl, error) {
	if r.err != nil {
		return nil, r.err
	}
	if depth > maxTemplateDepth {
		return nil, errors.New("template nesting depth exceeded")
	}
	childCount := r.readCount(maxTemplateChildren, "template children")
	children := make([]Template, 0, childCount)
	for i := 0; i < childCount; i++ {
		child, err := r.readTemplate(depth + 1)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	t := &DocumentTpl{templates: children}
	childImages := t.childrenImages()
	t.templates = append(t.childrenTemplates(), t.templates...)
	t.images = r.readImages()
	if t.images == nil {
		t.images = make(map[string]*ImageInfo)
	}
	maps.Copy(t.images, childImages)
	t.corner = Point{X: r.readFloat(), Y: r.readFloat()}
	t.size = Size{Wd: r.readFloat(), Ht: r.readFloat()}
	t.bytes = r.readBytesList()
	t.page = r.readInt()
	if r.err != nil {
		return nil, r.err
	}
	return t, nil
}

func (r *templateBinaryReader) readImages() map[string]*ImageInfo {
	count := r.readCount(maxTemplateImages, "template images")
	images := make(map[string]*ImageInfo, count)
	for i := 0; i < count; i++ {
		key := r.readString()
		images[key] = r.readImage()
	}
	return images
}

func (r *templateBinaryReader) readImage() *ImageInfo {
	if !r.readBool() {
		return nil
	}
	info := &ImageInfo{}
	info.data = r.readLimitedBytes(maxImageSourceBytes)
	info.smask = r.readLimitedBytes(maxImageSourceBytes)
	info.n = r.readInt()
	info.w = r.readFloat()
	info.h = r.readFloat()
	info.cs = r.readString()
	info.pal = r.readLimitedBytes(maxImageSourceBytes)
	info.bpc = r.readInt()
	info.f = r.readString()
	info.dp = r.readString()
	trnsCount := r.readCount(4096, "image transparency entries")
	info.trns = make([]int, trnsCount)
	for i := range info.trns {
		info.trns[i] = r.readInt()
	}
	info.scale = r.readFloat()
	info.dpi = r.readFloat()
	if r.err == nil {
		info.i, _ = generateImageID(info)
	}
	return info
}

func (r *templateBinaryReader) readBytesList() [][]byte {
	count := r.readCount(maxTemplatePages, "template pages")
	values := make([][]byte, count)
	for i := range values {
		values[i] = r.readLimitedBytes(maxTemplatePageBytes)
	}
	return values
}

func (r *templateBinaryReader) readString() string {
	return string(r.readLimitedBytes(maxTemplateSerializedBytes))
}

func (r *templateBinaryReader) readLimitedBytes(maxLen int) []byte {
	n := r.readCount(maxLen, "byte slice")
	return r.readRawBytes(n)
}

func (r *templateBinaryReader) readRawBytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.data)-r.pos {
		r.err = errors.New("truncated serialized template")
		return nil
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *templateBinaryReader) readBool() bool {
	return r.readByte() != 0
}

func (r *templateBinaryReader) readByte() byte {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.data) {
		r.err = errors.New("truncated serialized template")
		return 0
	}
	value := r.data[r.pos]
	r.pos++
	return value
}

func (r *templateBinaryReader) readInt() int {
	return int(int64(r.readUint()))
}

func (r *templateBinaryReader) readFloat() float64 {
	return math.Float64frombits(r.readUint())
}

func (r *templateBinaryReader) readCount(max int, name string) int {
	value := r.readUint()
	if r.err != nil {
		return 0
	}
	if value > uint64(max) {
		r.err = fmt.Errorf("%s exceeds maximum size", name)
		return 0
	}
	return int(value)
}

func (r *templateBinaryReader) readUint() uint64 {
	if r.err != nil {
		return 0
	}
	value, n := binary.Uvarint(r.data[r.pos:])
	if n <= 0 {
		r.err = errors.New("invalid serialized template integer")
		return 0
	}
	r.pos += n
	return value
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
