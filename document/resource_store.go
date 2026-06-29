// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"maps"
	"sort"
)

type resourceStore struct {
	fonts                map[string]fontDefinition
	fontFiles            map[string]fontFile
	templates            map[string]TemplateView
	templateObjects      map[string]int
	importedObjs         map[string][]byte
	importedObjPos       map[string]map[int]string
	importedTplObjs      map[string]string
	importedTplIDs       map[string]int
	importedPages        map[int]*importedPDFPage
	images               map[string]*ImageInfo
	attachmentStreams    map[attachmentStreamKey]int
	attachmentFiles      map[attachmentFileKey]int
	attachmentCompressed map[attachmentStreamKey]attachmentStream
}

func newResourceStore() *resourceStore {
	return &resourceStore{
		fonts:                make(map[string]fontDefinition),
		fontFiles:            make(map[string]fontFile),
		templates:            make(map[string]TemplateView),
		templateObjects:      make(map[string]int),
		importedObjs:         make(map[string][]byte, 0),
		importedObjPos:       make(map[string]map[int]string, 0),
		importedTplObjs:      make(map[string]string),
		importedTplIDs:       make(map[string]int, 0),
		importedPages:        make(map[int]*importedPDFPage),
		images:               make(map[string]*ImageInfo),
		attachmentStreams:    make(map[attachmentStreamKey]int),
		attachmentFiles:      make(map[attachmentFileKey]int),
		attachmentCompressed: make(map[attachmentStreamKey]attachmentStream),
	}
}

func (f *Document) initResourceStore() {
	f.resources = newResourceStore()
}

func (f *Document) ensureResourceStore() *resourceStore {
	if f.resources == nil {
		f.resources = newResourceStore()
	}
	return f.resources
}

func (s *resourceStore) image(name string) (*ImageInfo, bool) {
	info, ok := s.images[name]
	return info, ok
}

func (s *resourceStore) setImage(name string, info *ImageInfo) {
	s.images[name] = info
}

func (s *resourceStore) font(key string) (fontDefinition, bool) {
	font, ok := s.fonts[key]
	return font, ok
}

func (s *resourceStore) setFont(key string, font fontDefinition) {
	s.fonts[key] = font
}

func (s *resourceStore) fontsByResourceID(sorted bool) []fontDefinition {
	if !sorted {
		fonts := make([]fontDefinition, 0, len(s.fonts))
		for _, font := range s.fonts {
			fonts = append(fonts, font)
		}
		return fonts
	}
	keys := make([]string, 0, len(s.fonts))
	for key := range s.fonts {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := s.fonts[keys[i]]
		right := s.fonts[keys[j]]
		if left.i != right.i {
			return left.i < right.i
		}
		return keys[i] < keys[j]
	})
	fonts := make([]fontDefinition, 0, len(keys))
	for _, key := range keys {
		fonts = append(fonts, s.fonts[key])
	}
	return fonts
}

func (s *resourceStore) fontsByKey(sorted bool) []fontDefinition {
	keys := make([]string, 0, len(s.fonts))
	for key := range s.fonts {
		keys = append(keys, key)
	}
	if sorted {
		sort.Strings(keys)
	}
	fonts := make([]fontDefinition, 0, len(keys))
	for _, key := range keys {
		fonts = append(fonts, s.fonts[key])
	}
	return fonts
}

func (s *resourceStore) fontFile(file string) (fontFile, bool) {
	info, ok := s.fontFiles[file]
	return info, ok
}

func (s *resourceStore) setFontFile(file string, info fontFile) {
	s.fontFiles[file] = info
}

func (s *resourceStore) imagesForOutput(sorted bool) []*ImageInfo {
	images := make([]*ImageInfo, 0, len(s.images))
	if !sorted {
		for _, image := range s.images {
			images = append(images, image)
		}
		return images
	}
	keys := make([]string, 0, len(s.images))
	for key := range s.images {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := s.images[keys[i]]
		right := s.images[keys[j]]
		if left == nil || right == nil {
			return left != nil
		}
		if left.w != right.w {
			return left.w < right.w
		}
		if left.i != right.i {
			return left.i < right.i
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		images = append(images, s.images[key])
	}
	return images
}

func (s *resourceStore) imagesByResourceID(sorted bool) []*ImageInfo {
	if !sorted {
		images := make([]*ImageInfo, 0, len(s.images))
		for _, image := range s.images {
			images = append(images, image)
		}
		return images
	}
	keys := make([]string, 0, len(s.images))
	for key := range s.images {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := s.images[keys[i]]
		right := s.images[keys[j]]
		if left == nil || right == nil {
			return left != nil
		}
		if left.i != right.i {
			return left.i < right.i
		}
		return keys[i] < keys[j]
	})
	images := make([]*ImageInfo, 0, len(keys))
	for _, key := range keys {
		images = append(images, s.images[key])
	}
	return images
}

func (s *resourceStore) addTemplate(tpl TemplateView) {
	s.templates[tpl.ID()] = tpl
}

func (s *resourceStore) templatesForOutput(sorted bool) []TemplateView {
	return sortTemplates(s.templates, sorted)
}

func (s *resourceStore) templateCatalogKeys(sorted bool) []string {
	return templateKeyList(s.templates, sorted)
}

func (s *resourceStore) template(id string) (TemplateView, bool) {
	tpl, ok := s.templates[id]
	return tpl, ok
}

func (s *resourceStore) templateObject(id string) (int, bool) {
	objID, ok := s.templateObjects[id]
	return objID, ok
}

func (s *resourceStore) setTemplateObject(id string, objID int) {
	s.templateObjects[id] = objID
}

func (s *resourceStore) templateOutputImage(tplID, name string, image *ImageInfo) *ImageInfo {
	if image == nil {
		return nil
	}
	if stored := s.images[sprintf("t%s-%s", tplID, name)]; stored != nil {
		return stored
	}
	keys := make([]string, 0, len(s.images))
	for key := range s.images {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		stored := s.images[key]
		if stored != nil && stored.i == image.i {
			return stored
		}
	}
	return image
}

func (s *resourceStore) addImportedObject(name string, data []byte) {
	s.importedObjs[name] = append([]byte(nil), data...)
}

func (s *resourceStore) importedObjectHashes(sorted bool) []string {
	hashes := make([]string, 0, len(s.importedObjs))
	for hash := range s.importedObjs {
		hashes = append(hashes, hash)
	}
	if sorted {
		sort.Strings(hashes)
	}
	return hashes
}

func (s *resourceStore) importedObjectData(hash string) []byte {
	return s.importedObjs[hash]
}

func (s *resourceStore) addImportedObjectPositions(name string, positions map[int]string) {
	copied := make(map[int]string, len(positions))
	maps.Copy(copied, positions)
	s.importedObjPos[name] = copied
}

func (s *resourceStore) importedObjectPositions(hash string) map[int]string {
	return s.importedObjPos[hash]
}

func (s *resourceStore) addImportedTemplates(tpls map[string]string) {
	maps.Copy(s.importedTplObjs, tpls)
}

func (s *resourceStore) importedTemplateNames(sorted bool) []string {
	return importedTemplateOutputNames(s.importedTplObjs, sorted)
}

func (s *resourceStore) setImportedTemplateObjectID(hash string, objID int) {
	s.importedTplIDs[hash] = objID
}

func (s *resourceStore) importedTemplateResourceRefs(sorted bool) []pdfResourceRef {
	names := s.importedTemplateNames(sorted)
	refs := make([]pdfResourceRef, 0, len(names))
	for _, name := range names {
		hash := s.importedTplObjs[name]
		refs = append(refs, pdfResourceRef{
			name:         pdfResourceName(name),
			objectNumber: s.importedTplIDs[hash],
		})
	}
	return refs
}

func (s *resourceStore) addImportedPage(id int, page *importedPDFPage) {
	s.importedPages[id] = page
}

func (s *resourceStore) hasImportedPages() bool {
	return len(s.importedPages) > 0
}

func (s *resourceStore) importedPage(id int) (*importedPDFPage, bool) {
	page, ok := s.importedPages[id]
	return page, ok
}

func (s *resourceStore) compressedAttachment(key attachmentStreamKey) (attachmentStream, bool) {
	stream, ok := s.attachmentCompressed[key]
	return stream, ok
}

func (s *resourceStore) addCompressedAttachment(key attachmentStreamKey, stream attachmentStream) bool {
	if _, ok := s.attachmentCompressed[key]; ok {
		return false
	}
	s.attachmentCompressed[key] = stream
	return true
}

func (s *resourceStore) attachmentStreamObject(key attachmentStreamKey) int {
	return s.attachmentStreams[key]
}

func (s *resourceStore) setAttachmentStreamObject(key attachmentStreamKey, objectNumber int) {
	s.attachmentStreams[key] = objectNumber
}

func (s *resourceStore) attachmentFileObject(key attachmentFileKey) int {
	return s.attachmentFiles[key]
}

func (s *resourceStore) setAttachmentFileObject(key attachmentFileKey, objectNumber int) {
	s.attachmentFiles[key] = objectNumber
}

func (s *resourceStore) cleanupAttachmentCompressedFiles() {
	for key, stream := range s.attachmentCompressed {
		stream.cleanup()
		if stream.tempFile != "" {
			delete(s.attachmentCompressed, key)
		}
	}
}
