// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/gob"
	"sort"
)

// CreateTemplate defines a new template using the current page size.
func (f *Document) CreateTemplate(fn func(*Tpl)) Template {
	return createTemplate(Point{0, 0}, f.curPageSize, f.defOrientation, f.unitStr, f.fontDirStr, fn, f)
}

// CreateTemplateCustom starts a template, using the given bounds.
func (f *Document) CreateTemplateCustom(corner Point, size Size, fn func(*Tpl)) Template {
	return createTemplate(corner, size, f.defOrientation, f.unitStr, f.fontDirStr, fn, f)
}

// CreateTpl creates a template not attached to any document.
func CreateTpl(corner Point, size Size, orientationStr, unitStr, fontDirStr string, fn func(*Tpl)) Template {
	return createTemplate(corner, size, orientationStr, unitStr, fontDirStr, fn, nil)
}

// UseTemplate adds a template to the current page or another template,
// using the size and position at which it was originally written.
func (f *Document) UseTemplate(t Template) {
	if t == nil {
		f.SetErrorf("template is nil")
		return
	}
	corner, size := t.Size()
	f.UseTemplateScaled(t, corner, size)
}

// UseTemplateScaled adds a template to the current page or another template,
// using the given page coordinates.
func (f *Document) UseTemplateScaled(t Template, corner Point, size Size) {
	if t == nil {
		f.SetErrorf("template is nil")
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

	content := []byte(sprintf("q %.4f 0 0 %.4f %.4f %.4f cm\n/TPL%s Do Q", scaleX, scaleY, tx, ty, t.ID()))
	f.outbytes(f.wrapTaggedContent(content, taggedContentOptions{Artifact: true}))
}

func (f *Document) registerTemplate(t Template) {
	for _, tpl := range collectTemplates(t) {
		f.templates[tpl.ID()] = tpl
	}
}

func collectTemplates(root Template) []Template {
	templates := make([]Template, 0)
	seen := make(map[string]bool)

	var visit func(Template)
	visit = func(t Template) {
		if invalidTemplate(t) {
			return
		}
		id := t.ID()
		if seen[id] {
			return
		}
		seen[id] = true
		templates = append(templates, t)

		for _, child := range t.Templates() {
			visit(child)
		}
	}

	visit(root)
	return templates
}

func (f *Document) registerTemplateImages(t Template) {
	existingImages := make(map[string]bool, len(f.images))
	for _, image := range f.images {
		if image != nil {
			existingImages[image.i] = true
		}
	}

	for name, image := range t.Images() {
		if image == nil {
			f.SetErrorf("invalid template image %s: image info is nil", name)
			return
		}
		if existingImages[image.i] {
			continue
		}
		f.images[sprintf("t%s-%s", t.ID(), name)] = image
	}
}

// Template is an object that can be written to, then reused any number of times
// within a document.
type Template interface {
	ID() string
	Size() (Point, Size)
	Bytes() []byte
	Images() map[string]*ImageInfo
	Templates() []Template
	NumPages() int
	FromPage(int) (Template, error)
	FromPages() []Template
	Serialize() ([]byte, error)
	gob.GobDecoder
	gob.GobEncoder
}

func (f *Document) templateFontCatalog() {
	var keyList []string
	var font fontDefinition
	var key string
	f.out("/Font <<")
	for key = range f.fonts {
		keyList = append(keyList, key)
	}
	if f.catalogSort {
		sort.Strings(keyList)
	}
	for _, key = range keyList {
		font = f.fonts[key]
		f.outf("/F%s %d 0 R", font.i, font.N)
	}
	f.out(">>")
}

// putTemplates writes the templates to the PDF.
func (f *Document) putTemplates() {
	filter := ""
	if f.compress {
		filter = "/Filter /FlateDecode "
	}

	templates := sortTemplates(f.templates, f.catalogSort)
	for _, t := range templates {
		corner, size := t.Size()

		f.newobj()
		f.templateObjects[t.ID()] = f.n
		f.outf("<<%s/Type /XObject", filter)
		f.out("/Subtype /Form")
		f.out("/Formtype 1")
		f.outf("/BBox [%.2f %.2f %.2f %.2f]", corner.X*f.k, corner.Y*f.k, (corner.X+size.Wd)*f.k, (corner.Y+size.Ht)*f.k)
		if corner.X != 0 || corner.Y != 0 {
			f.outf("/Matrix [1 0 0 1 %.5f %.5f]", -corner.X*f.k*2, corner.Y*f.k*2)
		}

		// Template's resource dictionary
		f.out("/Resources ")
		f.out("<<")
		if !f.omitDeprecatedPDF2Entries() {
			f.out("/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")
		}

		f.templateFontCatalog()
		f.templateXObjectCatalog(t)

		f.out(">>")

		buffer := t.Bytes()
		if f.compress {
			buffer = f.compressBytes(buffer)
			if f.err != nil {
				return
			}
		}
		f.outf("/Length %d >>", len(buffer))
		f.putstream(buffer)
		f.out("endobj")
	}
}

func (f *Document) templateXObjectCatalog(t Template) {
	images := t.Images()
	templates := t.Templates()
	if len(images) == 0 && len(templates) == 0 {
		return
	}

	f.out("/XObject <<")
	f.templateImageCatalog(images)
	f.templateDependencyCatalog(templates)
	f.out(">>")
}

func (f *Document) templateImageCatalog(images map[string]*ImageInfo) {
	keyList := make([]string, 0, len(images))
	for key := range images {
		keyList = append(keyList, key)
	}
	if f.catalogSort {
		sort.Strings(keyList)
	}

	for _, key := range keyList {
		image := images[key]
		if image != nil {
			f.outf("/I%s %d 0 R", image.i, image.n)
		}
	}
}

func (f *Document) templateDependencyCatalog(templates []Template) {
	for _, t := range templates {
		if invalidTemplate(t) {
			continue
		}
		if objID, ok := f.templateObjects[t.ID()]; ok {
			f.outf("/TPL%s %d 0 R", t.ID(), objID)
		}
	}
}

func templateKeyList(mp map[string]Template, sorted bool) (keyList []string) {
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
func sortTemplates(templates map[string]Template, catalogSort bool) []Template {
	sorted := make([]Template, 0, len(templates))
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
	t Template,
	catalogSort bool,
	visited map[string]bool,
	visiting map[string]bool,
	sorted *[]Template,
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

func sortedTemplateDependencies(t Template, catalogSort bool) []Template {
	if t == nil || invalidTemplate(t) {
		return nil
	}
	dependencies := append([]Template(nil), t.Templates()...)
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
