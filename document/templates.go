// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/gob"
	"fmt"
	"sort"
)

// CreateTemplate defines a new template using the current page size.
func (f *Document) CreateTemplate(fn func(*Tpl)) Template {
	if f.err != nil {
		return nil
	}
	return createTemplate(Point{0, 0}, f.curPageSize, f.defOrientation, f.unitStr, f.fontDirStr, fn, f)
}

// CreateTemplateCustom starts a template, using the given bounds.
func (f *Document) CreateTemplateCustom(corner Point, size Size, fn func(*Tpl)) Template {
	if f.err != nil {
		return nil
	}
	if err := validateTemplateGeometry(corner, size); err != nil {
		f.SetError(err)
		return nil
	}
	return createTemplate(corner, size, f.defOrientation, f.unitStr, f.fontDirStr, fn, f)
}

// CreateTpl creates a template not attached to any document.
func CreateTpl(corner Point, size Size, orientationStr, unitStr, fontDirStr string, fn func(*Tpl)) Template {
	if err := validateTemplateGeometry(corner, size); err != nil {
		return nil
	}
	return createTemplate(corner, size, orientationStr, unitStr, fontDirStr, fn, nil)
}

// UseTemplate adds a template to the current page or another template,
// using the size and position at which it was originally written.
func (f *Document) UseTemplate(t Template) {
	f.UseTemplateView(t)
}

// UseTemplateView adds a renderable template view to the current page or
// another template using the size and position at which it was originally
// written. It does not require paging or serialization support.
func (f *Document) UseTemplateView(t TemplateView) {
	if t == nil {
		f.SetErrorf("template is nil")
		return
	}
	corner, size := t.Size()
	f.UseTemplateViewScaled(t, corner, size)
}

// UseTemplateScaled adds a template to the current page or another template,
// using the given page coordinates.
func (f *Document) UseTemplateScaled(t Template, corner Point, size Size) {
	f.UseTemplateViewScaled(t, corner, size)
}

// UseTemplateViewScaled adds a renderable template view to the current page or
// another template using the given page coordinates. It does not require paging
// or serialization support.
func (f *Document) UseTemplateViewScaled(t TemplateView, corner Point, size Size) {
	if f.err != nil {
		return
	}
	if t == nil {
		f.SetErrorf("template is nil")
		return
	}
	if err := validateTemplateGeometry(corner, size); err != nil {
		f.SetError(err)
		return
	}

	// A page must exist before a template can be used.
	if f.page <= 0 {
		f.SetErrorf("cannot use a template without first adding a page")
		return
	}

	f.registerTemplate(t)
	f.registerTemplateImages(t)
	if f.err != nil {
		return
	}

	_, templateSize := t.Size()
	if templateSize.Wd == 0 || templateSize.Ht == 0 {
		f.SetErrorf("template has invalid size")
		return
	}

	scaleX := size.Wd / templateSize.Wd
	scaleY := size.Ht / templateSize.Ht
	tx := corner.X * f.k
	ty := (f.curPageSize.Ht - corner.Y - size.Ht) * f.k

	content := []byte(sprintf("q %.4f 0 0 %.4f %.4f %.4f cm\n%s Do Q", scaleX, scaleY, tx, ty, templatePDFResourceName(t.ID()).String()))
	f.outTaggedContent(content, taggedContentOptions{Artifact: true})
}

func validateTemplateGeometry(corner Point, size Size) error {
	if !finiteNumbers(corner.X, corner.Y, size.Wd, size.Ht) || size.Wd <= 0 || size.Ht <= 0 {
		return fmt.Errorf("invalid template geometry")
	}
	return nil
}

func (f *Document) registerTemplate(t TemplateView) {
	resources := f.ensureResourceStore()
	for _, tpl := range collectTemplates(t) {
		resources.addTemplate(tpl)
	}
}

func collectTemplates(root TemplateView) []TemplateView {
	templates := make([]TemplateView, 0)
	seen := make(map[string]bool)

	var visit func(TemplateView)
	visit = func(t TemplateView) {
		if invalidTemplate(t) {
			return
		}
		id := t.ID()
		if seen[id] {
			return
		}
		seen[id] = true
		templates = append(templates, t)

		for _, child := range templateChildren(t) {
			visit(child)
		}
	}

	visit(root)
	return templates
}

func (f *Document) registerTemplateImages(t TemplateView) {
	resources := f.ensureResourceStore()
	existingImages := make(map[string]bool, len(resources.images))
	for _, image := range resources.images {
		if image != nil {
			existingImages[image.i] = true
		}
	}

	images := templateImages(t)
	for _, name := range templateImageKeys(images, true) {
		image := images[name]
		if image == nil {
			f.SetErrorf("invalid template image %s: image info is nil", name)
			return
		}
		if existingImages[image.i] {
			continue
		}
		resources.setImage(sprintf("t%s-%s", t.ID(), name), cloneTemplateImage(image))
	}
}

// TemplateView exposes the renderable content and resources of a template.
// Rendering code should accept this narrow interface unless it needs paging or
// serialization.
type TemplateView interface {
	ID() string
	Size() (Point, Size)
	Bytes() []byte
	Images() map[string]*ImageInfo
}

// TemplateChildrenView exposes render-only child template dependencies without
// requiring those children to support paging or serialization. Implement this
// alongside TemplateView when a render-only template view depends on other
// render-only templates.
type TemplateChildrenView interface {
	TemplateViews() []TemplateView
}

// PagedTemplate exposes page selection for multi-page templates.
type PagedTemplate interface {
	NumPages() int
	FromPage(int) (Template, error)
	FromPages() []Template
}

// SerializableTemplate exposes template persistence for cache/storage use.
type SerializableTemplate interface {
	Serialize() ([]byte, error)
}

// Template is an object that can be written to, then reused any number of times
// within a document. New rendering integrations should prefer TemplateView and
// only require Template when they need paging, persistence, or gob support.
type Template interface {
	TemplateView
	PagedTemplate
	SerializableTemplate
	gob.GobDecoder
	gob.GobEncoder
	Templates() []Template
}

func (f *Document) templateFontCatalog() {
	f.out("/Font")
	f.beginPDFDict()
	for _, font := range f.ensureResourceStore().fontsByKey(f.catalogSort) {
		f.outbytes(appendPDFResourceRefValue(nil, fontPDFResourceRef(font)))
	}
	f.endPDFDict()
}

// putTemplates writes the templates to the PDF.
func (f *Document) putTemplates() {
	resources := f.ensureResourceStore()
	templates := resources.templatesForOutput(f.catalogSort)
	for _, t := range templates {
		corner, size := t.Size()

		f.newPDFDictObject()
		resources.setTemplateObject(t.ID(), f.n)
		f.out("/Type /XObject")
		f.out("/Subtype /Form")
		f.out("/Formtype 1")
		f.outf("/BBox [%.2f %.2f %.2f %.2f]", corner.X*f.k, corner.Y*f.k, (corner.X+size.Wd)*f.k, (corner.Y+size.Ht)*f.k)
		if corner.X != 0 || corner.Y != 0 {
			f.outf("/Matrix [1 0 0 1 %.5f %.5f]", -corner.X*f.k*2, corner.Y*f.k*2)
		}

		// Template's resource dictionary
		f.out("/Resources ")
		f.beginPDFDict()
		if !f.omitDeprecatedPDF2Entries() {
			f.out("/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")
		}

		f.templateFontCatalog()
		f.templateXObjectCatalog(t)

		f.endPDFDict()

		buffer := t.Bytes()
		buffer, compressed := f.compressStreamBytes(buffer)
		if f.err != nil {
			return
		}
		if compressed {
			f.out("/Filter /FlateDecode")
		}
		f.outf("/Length %d", len(buffer))
		f.endPDFDict()
		f.putstream(buffer)
		f.endPDFObject()
	}
}

func (f *Document) templateXObjectCatalog(t TemplateView) {
	images := templateImages(t)
	templates := templateChildren(t)
	if len(images) == 0 && len(templates) == 0 {
		return
	}

	f.out("/XObject")
	f.beginPDFDict()
	f.templateImageCatalog(t, images)
	f.templateDependencyCatalog(templates)
	f.endPDFDict()
}

func (f *Document) templateImageCatalog(t TemplateView, images map[string]*ImageInfo) {
	for _, key := range templateImageKeys(images, f.catalogSort) {
		if image := f.templateOutputImage(t, key, images[key]); image != nil {
			f.outbytes(appendPDFResourceRefValue(nil, imagePDFResourceRef(image)))
		}
	}
}

func (f *Document) templateOutputImage(t TemplateView, name string, image *ImageInfo) *ImageInfo {
	return f.ensureResourceStore().templateOutputImage(t.ID(), name, image)
}

func (f *Document) templateDependencyCatalog(templates []TemplateView) {
	resources := f.ensureResourceStore()
	for _, t := range templates {
		if invalidTemplate(t) {
			continue
		}
		if objID, ok := resources.templateObject(t.ID()); ok {
			f.outbytes(appendPDFResourceRefValue(nil, templatePDFResourceRef(t.ID(), objID)))
		}
	}
}

func templateKeyList(mp map[string]TemplateView, sorted bool) (keyList []string) {
	for key := range mp {
		keyList = append(keyList, key)
	}
	if sorted {
		sort.Strings(keyList)
	}
	return
}

// sortTemplates orders templates so dependencies are written before the
// templates that use them.
func sortTemplates(templates map[string]TemplateView, catalogSort bool) []TemplateView {
	sorted := make([]TemplateView, 0, len(templates))
	visited := make(map[string]bool, len(templates))
	visiting := make(map[string]bool, len(templates))

	for _, key := range templateKeyList(templates, catalogSort) {
		template := templates[key]
		if template == nil {
			continue
		}
		appendTemplateDependencies(template, catalogSort, visited, visiting, &sorted)
	}

	return sorted
}

func appendTemplateDependencies(
	t TemplateView,
	catalogSort bool,
	visited map[string]bool,
	visiting map[string]bool,
	sorted *[]TemplateView,
) {
	if t == nil || invalidTemplate(t) {
		return
	}

	id := t.ID()
	if visited[id] || visiting[id] {
		return
	}

	visiting[id] = true
	for _, child := range sortedTemplateDependencies(t, catalogSort) {
		appendTemplateDependencies(child, catalogSort, visited, visiting, sorted)
	}
	visiting[id] = false

	visited[id] = true
	*sorted = append(*sorted, t)
}

func sortedTemplateDependencies(t TemplateView, catalogSort bool) []TemplateView {
	if t == nil || invalidTemplate(t) {
		return nil
	}
	dependencies := append([]TemplateView(nil), templateChildren(t)...)
	if catalogSort {
		sort.SliceStable(dependencies, func(i, j int) bool {
			if invalidTemplate(dependencies[i]) {
				return false
			}
			if invalidTemplate(dependencies[j]) {
				return true
			}
			return dependencies[i].ID() < dependencies[j].ID()
		})
	}
	return dependencies
}

func templateImages(t TemplateView) map[string]*ImageInfo {
	if tpl, ok := t.(*DocumentTpl); ok && tpl != nil {
		return tpl.images
	}
	return t.Images()
}

func templateImageKeys(images map[string]*ImageInfo, sorted bool) []string {
	keys := make([]string, 0, len(images))
	for key := range images {
		keys = append(keys, key)
	}
	if sorted {
		sort.Strings(keys)
	}
	return keys
}

func templateChildren(t TemplateView) []TemplateView {
	if withChildren, ok := t.(TemplateChildrenView); ok && withChildren != nil {
		return withChildren.TemplateViews()
	}
	withChildren, ok := t.(interface{ Templates() []Template })
	if !ok || withChildren == nil {
		return nil
	}
	children := withChildren.Templates()
	views := make([]TemplateView, 0, len(children))
	for _, child := range children {
		views = append(views, child)
	}
	return views
}
