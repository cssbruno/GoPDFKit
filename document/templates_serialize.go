// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"maps"
	"math"
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

	resources := tpl.ensureResourceStore()
	templateViews := make([]TemplateView, 0, len(resources.templates))
	templates := make([]Template, 0, len(resources.templates))
	for _, key := range resources.templateCatalogKeys(true) {
		child, _ := resources.template(key)
		if invalidTemplate(child) {
			continue
		}
		templateViews = append(templateViews, child)
		if child, ok := child.(Template); ok {
			templates = append(templates, child)
		}
	}
	images := cloneTemplateImages(resources.images)

	template := DocumentTpl{
		corner:        corner,
		size:          size,
		bytes:         bytes,
		images:        images,
		templates:     templates,
		templateViews: templateViews,
		page:          tpl.page,
	}
	return &template
}

// DocumentTpl is a concrete implementation of the Template interface.
type DocumentTpl struct {
	corner        Point
	size          Size
	bytes         [][]byte
	images        map[string]*ImageInfo
	templates     []Template
	templateViews []TemplateView
	page          int
	id            string
}

const (
	maxTemplateChildren        = 1024
	maxTemplateDepth           = 128
	maxTemplateImages          = 4096
	maxTemplatePageBytes       = 16 * 1024 * 1024
	maxTemplatePages           = 1000
	maxTemplateSerializedBytes = 16 * 1024 * 1024
	maxTemplateNodes           = 10000
	maxTemplateTotalImages     = 4096
	maxTemplateTotalPages      = 10000
)

// TemplateDecodeOptions controls limits used when deserializing templates.
// Zero fields use package defaults.
type TemplateDecodeOptions struct {
	MaxSerializedBytes int
	MaxPages           int
	MaxImages          int
	MaxChildren        int
	MaxPageBytes       int
	MaxNodes           int
	MaxTotalImages     int
	MaxTotalPages      int
}

func normalizeTemplateDecodeOptions(options TemplateDecodeOptions) (TemplateDecodeOptions, error) {
	if options.MaxSerializedBytes == 0 {
		options.MaxSerializedBytes = maxTemplateSerializedBytes
	}
	if options.MaxPages == 0 {
		options.MaxPages = maxTemplatePages
	}
	if options.MaxImages == 0 {
		options.MaxImages = maxTemplateImages
	}
	if options.MaxChildren == 0 {
		options.MaxChildren = maxTemplateChildren
	}
	if options.MaxPageBytes == 0 {
		options.MaxPageBytes = maxTemplatePageBytes
	}
	if options.MaxNodes == 0 {
		options.MaxNodes = maxTemplateNodes
	}
	if options.MaxTotalImages == 0 {
		options.MaxTotalImages = maxTemplateTotalImages
	}
	if options.MaxTotalPages == 0 {
		options.MaxTotalPages = maxTemplateTotalPages
	}
	if options.MaxSerializedBytes < 0 || options.MaxPages < 0 || options.MaxImages < 0 || options.MaxChildren < 0 || options.MaxPageBytes < 0 || options.MaxNodes < 0 || options.MaxTotalImages < 0 || options.MaxTotalPages < 0 {
		return TemplateDecodeOptions{}, errors.New("invalid template decode limit")
	}
	return options, nil
}

// TemplateSerializationVersion returns the current serialized-template format
// version. The v0.9.x series preserves this format within the minor series; the
// long-term compatibility promise is reserved for v1.0.
func TemplateSerializationVersion() string {
	return templateSerializationVersion
}

// TemplateFingerprintVersion returns the current template identity hash format
// version. This is separate from TemplateSerializationVersion.
func TemplateFingerprintVersion() string {
	return templateFingerprintVersion
}

// ID returns the global template identifier.
func (t *DocumentTpl) ID() string {
	if t.id == "" {
		t.id = t.fingerprint()
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
	return append([]byte(nil), t.bytes[t.page]...)
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
	return cloneTemplateImages(t.images)
}

// Templates returns the templates used in this template.
func (t *DocumentTpl) Templates() []Template {
	return append([]Template(nil), t.templates...)
}

// TemplateViews returns the renderable child templates used in this template.
func (t *DocumentTpl) TemplateViews() []TemplateView {
	return t.childTemplateViews()
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
	return DeserializeTemplateWithOptions(b, TemplateDecodeOptions{})
}

// DeserializeTemplateWithOptions creates a template using explicit decode
// limits.
func DeserializeTemplateWithOptions(b []byte, options TemplateDecodeOptions) (Template, error) {
	options, err := normalizeTemplateDecodeOptions(options)
	if err != nil {
		return nil, err
	}
	if len(b) > options.MaxSerializedBytes {
		return nil, errors.New("serialized template exceeds maximum size")
	}
	r := templateBinaryReader{data: b, options: options}
	if !bytes.Equal(r.readRawBytes(len(templateBinaryMagic)), []byte(templateBinaryMagic)) {
		return nil, errors.New("invalid serialized template header")
	}
	tpl, err := r.readTemplate(0)
	if err != nil {
		return nil, err
	}
	if r.pos != len(r.data) {
		return nil, errors.New("trailing data after serialized template")
	}
	if err := tpl.validateWithOptions(options); err != nil {
		return nil, err
	}
	return tpl, nil
}

// childrenImages returns the next layer of child images without recursing into
// grandchildren. It applies the template namespace to keys to avoid collisions.
// See UseTemplateScaled.
func (t *DocumentTpl) childrenImages() map[string]*ImageInfo {
	images := make(map[string]*ImageInfo)

	for _, child := range t.childTemplateViews() {
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
	for _, child := range t.childTemplateViews() {
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

func (t *DocumentTpl) childTemplateViews() []TemplateView {
	if len(t.templateViews) > 0 || len(t.templates) == 0 {
		return append([]TemplateView(nil), t.templateViews...)
	}
	return templateViewsFromTemplates(t.templates)
}

func templateViewsFromTemplates(templates []Template) []TemplateView {
	views := make([]TemplateView, 0, len(templates))
	for _, child := range templates {
		views = append(views, child)
	}
	return views
}

func (t *DocumentTpl) hasRenderOnlyChildTemplates() bool {
	views := t.childTemplateViews()
	if len(views) != len(t.templates) {
		return true
	}
	for i, view := range views {
		child, ok := view.(Template)
		if !ok || invalidTemplate(child) || invalidTemplate(t.templates[i]) || child.ID() != t.templates[i].ID() {
			return true
		}
	}
	return false
}

func (t *DocumentTpl) fingerprint() string {
	h := sha256.New()
	hashTemplate(h, t, 0)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func hashTemplate(h hash.Hash, tpl TemplateView, depth int) {
	hashImageString(h, 'v', templateFingerprintVersion)
	if invalidTemplate(tpl) {
		hashImageString(h, 'e', "nil")
		return
	}
	if depth > maxTemplateDepth {
		hashImageString(h, 'e', "max-depth")
		return
	}
	corner, size := tpl.Size()
	hashImageFloat(h, 'x', corner.X)
	hashImageFloat(h, 'y', corner.Y)
	hashImageFloat(h, 'w', size.Wd)
	hashImageFloat(h, 'h', size.Ht)
	if paged, ok := tpl.(PagedTemplate); ok {
		hashImageInt(h, 'p', paged.NumPages())
	} else {
		hashImageInt(h, 'p', 1)
	}
	hashImageBytes(h, 'b', tpl.Bytes())

	images := tpl.Images()
	imageKeys := templateImageKeys(images, true)
	hashImageInt(h, 'i', len(imageKeys))
	for _, key := range imageKeys {
		hashImageString(h, 'k', key)
		hashTemplateImage(h, images[key])
	}

	children := templateChildren(tpl)
	hashImageInt(h, 'c', len(children))
	for _, child := range children {
		if invalidTemplate(child) {
			hashImageString(h, 'e', "nil-child")
			continue
		}
		hashImageString(h, 'd', child.ID())
		hashTemplate(h, child, depth+1)
	}
}

func hashTemplateImage(h hash.Hash, info *ImageInfo) {
	if info == nil {
		hashImageString(h, 'z', "nil")
		return
	}
	hashImageBytes(h, 'd', info.data)
	hashImageBytes(h, 'm', info.smask)
	hashImageFloat(h, 'w', info.w)
	hashImageFloat(h, 'h', info.h)
	hashImageString(h, 'c', info.cs)
	hashImageBytes(h, 'p', info.pal)
	hashImageInt(h, 'b', info.bpc)
	hashImageString(h, 'f', info.f)
	hashImageString(h, 'q', info.dp)
	hashImageInt(h, 't', len(info.trns))
	for _, value := range info.trns {
		hashImageInt(h, 'v', value)
	}
	hashImageFloat(h, 's', info.scale)
	hashImageFloat(h, 'i', info.dpi)
}

func cloneTemplateImages(images map[string]*ImageInfo) map[string]*ImageInfo {
	if len(images) == 0 {
		return nil
	}
	clones := make(map[string]*ImageInfo, len(images))
	for key, image := range images {
		clones[key] = cloneTemplateImage(image)
	}
	return clones
}

func cloneTemplateImage(info *ImageInfo) *ImageInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.data = append([]byte(nil), info.data...)
	clone.smask = append([]byte(nil), info.smask...)
	clone.pal = append([]byte(nil), info.pal...)
	clone.trns = append([]int(nil), info.trns...)
	return &clone
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

const (
	templateSerializationVersion = "GPKTPL1"
	templateFingerprintVersion   = "GPKTPL2"
	templateBinaryMagic          = templateSerializationVersion + "\x00"
)

// GobEncode encodes the receiving template into a byte buffer. Use GobDecode
// to decode the byte buffer back to a template.
func (t *DocumentTpl) GobEncode() ([]byte, error) {
	return t.Serialize()
}

// GobDecode decodes the specified byte buffer into the receiving template.
func (t *DocumentTpl) GobDecode(buf []byte) error {
	tpl, err := DeserializeTemplateWithOptions(buf, TemplateDecodeOptions{})
	if err != nil {
		return err
	}
	documentTpl, ok := tpl.(*DocumentTpl)
	if !ok || documentTpl == nil {
		return errors.New("invalid serialized template")
	}
	*t = *documentTpl
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
	if t.hasRenderOnlyChildTemplates() {
		return errors.New("template contains non-serializable child template")
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
	keys := templateImageKeys(images, true)
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
	// Preserve the signed value's two's-complement bits in the established
	// unsigned-varint wire format.
	w.writeUint(uint64(int64(value))) // #nosec G115 -- Deliberate signed binary serialization; readInt validates the platform int range.
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
	data        []byte
	pos         int
	err         error
	options     TemplateDecodeOptions
	nodes       int
	totalImages int
	totalPages  int
}

func (r *templateBinaryReader) readTemplate(depth int) (*DocumentTpl, error) {
	if r.err != nil {
		return nil, r.err
	}
	if depth > maxTemplateDepth {
		return nil, errors.New("template nesting depth exceeded")
	}
	if r.nodes >= r.options.MaxNodes {
		return nil, errors.New("template node count exceeds maximum size")
	}
	r.nodes++
	childCount := r.readCount(r.options.MaxChildren, "template children")
	children := make([]Template, 0, childCount)
	for i := 0; i < childCount; i++ {
		child, err := r.readTemplate(depth + 1)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	t := &DocumentTpl{templates: children, templateViews: templateViewsFromTemplates(children)}
	childImages := t.childrenImages()
	t.templates = append(t.childrenTemplates(), t.templates...)
	t.templateViews = templateViewsFromTemplates(t.templates)
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
	count := r.readCount(r.options.MaxImages, "template images")
	r.addAggregateCount(&r.totalImages, count, r.options.MaxTotalImages, "total template images")
	if r.err != nil {
		return nil
	}
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
	count := r.readCount(r.options.MaxPages, "template pages")
	r.addAggregateCount(&r.totalPages, count, r.options.MaxTotalPages, "total template pages")
	if r.err != nil {
		return nil
	}
	values := make([][]byte, count)
	for i := range values {
		values[i] = r.readLimitedBytes(r.options.MaxPageBytes)
	}
	return values
}

func (r *templateBinaryReader) addAggregateCount(total *int, count, max int, name string) {
	if r.err != nil {
		return
	}
	if count < 0 || count > max-*total {
		r.err = fmt.Errorf("%s exceeds maximum size", name)
		return
	}
	*total += count
}

func (r *templateBinaryReader) readString() string {
	return string(r.readLimitedBytes(r.options.MaxSerializedBytes))
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
	value := r.readUint()
	if r.err != nil {
		return 0
	}
	signed := int64(value) // #nosec G115 -- Inverse of the documented two's-complement wire encoding above.
	decoded := int(signed) // #nosec G115 -- The round-trip check below rejects values outside the platform int range.
	if int64(decoded) != signed {
		r.err = errors.New("serialized template integer exceeds platform range")
		return 0
	}
	return decoded
}

func (r *templateBinaryReader) readFloat() float64 {
	return math.Float64frombits(r.readUint())
}

func (r *templateBinaryReader) readCount(max int, name string) int {
	value := r.readUint()
	if r.err != nil {
		return 0
	}
	if max < 0 || value > uint64(max) { // #nosec G115 -- max is rejected when negative before its use as an unsigned bound.
		r.err = fmt.Errorf("%s exceeds maximum size", name)
		return 0
	}
	return int(value) // #nosec G115 -- value is bounded by the non-negative platform int max above.
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
	options, _ := normalizeTemplateDecodeOptions(TemplateDecodeOptions{})
	return t.validateWithOptions(options)
}

func (t *DocumentTpl) validateWithOptions(options TemplateDecodeOptions) error {
	options, err := normalizeTemplateDecodeOptions(options)
	if err != nil {
		return err
	}
	return t.validateDepth(0, make(map[*DocumentTpl]bool), options)
}

func (t *DocumentTpl) validateDepth(depth int, visiting map[*DocumentTpl]bool, options TemplateDecodeOptions) error {
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
	if len(t.bytes)-1 > options.MaxPages {
		return errors.New("template page count exceeds maximum size")
	}
	if len(t.images) > options.MaxImages {
		return errors.New("template image count exceeds maximum size")
	}
	if len(t.childTemplateViews()) > options.MaxChildren {
		return errors.New("template child count exceeds maximum size")
	}
	for _, page := range t.bytes {
		if len(page) > options.MaxPageBytes {
			return errors.New("template page content exceeds maximum size")
		}
	}
	for name, img := range t.images {
		if err := img.validForPDF(); err != nil {
			return fmt.Errorf("invalid template image %s: %w", name, err)
		}
	}
	for _, tpl := range t.childTemplateViews() {
		if invalidTemplate(tpl) {
			return errors.New("invalid nil child template")
		}
		if child, ok := tpl.(*DocumentTpl); ok {
			if err := child.validateDepth(depth+1, visiting, options); err != nil {
				return err
			}
		}
	}
	return nil
}

func invalidTemplate(tpl TemplateView) bool {
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
	tResources := t.ensureResourceStore()
	fResources := f.ensureResourceStore()

	t.k = f.k
	t.x = f.x
	t.y = f.y
	t.lineWidth = f.lineWidth
	t.capStyle = f.capStyle
	t.joinStyle = f.joinStyle

	t.color.draw = f.color.draw
	t.color.fill = f.color.fill
	t.color.text = f.color.text

	tResources.fonts = fResources.fonts
	t.currentFont = f.currentFont
	t.fontFamily = f.fontFamily
	t.fontSize = f.fontSize
	t.fontSizePt = f.fontSizePt
	t.fontStyle = f.fontStyle
	t.ws = f.ws

	maps.Copy(tResources.images, fResources.images)
}
