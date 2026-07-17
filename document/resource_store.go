// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"maps"
	"sort"
)

type resourceStore struct {
	fonts           map[string]fontDefinition
	fontFiles       map[string]fontFile
	templates       map[string]TemplateView
	importedObjs    map[string][]byte
	importedObjPos  map[string]map[int]string
	importedTplObjs map[string]string
	importedPages   map[int]*importedPDFPage
	images          map[string]*ImageInfo
	imageAliases    map[string]string
	objects         resourceObjectNumbers
	attachments     attachmentResourceStore
}

type resourceObjectNumbers struct {
	templates         map[string]int
	importedTemplates map[string]int
}

type attachmentResourceStore struct {
	streams    map[attachmentStreamKey]int
	files      map[attachmentFileKey]int
	compressed map[attachmentStreamKey]attachmentStream
}

func newResourceStore() *resourceStore {
	return &resourceStore{
		fonts:           make(map[string]fontDefinition),
		fontFiles:       make(map[string]fontFile),
		templates:       make(map[string]TemplateView),
		importedObjs:    make(map[string][]byte),
		importedObjPos:  make(map[string]map[int]string),
		importedTplObjs: make(map[string]string),
		importedPages:   make(map[int]*importedPDFPage),
		images:          make(map[string]*ImageInfo),
		imageAliases:    make(map[string]string),
		objects: resourceObjectNumbers{
			templates:         make(map[string]int),
			importedTemplates: make(map[string]int),
		},
		attachments: attachmentResourceStore{
			streams:    make(map[attachmentStreamKey]int),
			files:      make(map[attachmentFileKey]int),
			compressed: make(map[attachmentStreamKey]attachmentStream),
		},
	}
}

func (state *resourceOwnershipState) initResourceStore() {
	state.resources = newResourceStore()
}

func (state *resourceOwnershipState) ensureResourceStore() *resourceStore {
	if state.resources == nil {
		state.resources = newResourceStore()
	}
	return state.resources
}

func (s *resourceStore) image(name string) (*ImageInfo, bool) {
	info, ok := s.images[name]
	if !ok && s.imageAliases != nil {
		info, ok = s.images[s.imageAliases[name]]
	}
	return info, ok
}

func (s *resourceStore) setImage(name string, info *ImageInfo) {
	s.images[name] = info
}

func (s *resourceStore) setImageAlias(alias, name string) {
	if alias == "" || name == "" || alias == name {
		return
	}
	if s.imageAliases == nil {
		s.imageAliases = make(map[string]string)
	}
	s.imageAliases[alias] = name
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
	objID, ok := s.objects.templates[id]
	return objID, ok
}

func (s *resourceStore) setTemplateObject(id string, objID int) {
	s.objects.templates[id] = objID
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
	s.objects.importedTemplates[hash] = objID
}

func (s *resourceStore) importedTemplateResourceRefs(sorted bool) []pdfResourceRef {
	names := s.importedTemplateNames(sorted)
	refs := make([]pdfResourceRef, 0, len(names))
	for _, name := range names {
		hash := s.importedTplObjs[name]
		refs = append(refs, pdfResourceRef{
			name:         pdfResourceName(name),
			objectNumber: s.objects.importedTemplates[hash],
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
	stream, ok := s.attachments.compressed[key]
	return stream, ok
}

func (s *resourceStore) addCompressedAttachment(key attachmentStreamKey, stream attachmentStream) bool {
	if _, ok := s.attachments.compressed[key]; ok {
		return false
	}
	s.attachments.compressed[key] = stream
	return true
}

func (s *resourceStore) attachmentStreamObject(key attachmentStreamKey) int {
	return s.attachments.streams[key]
}

func (s *resourceStore) setAttachmentStreamObject(key attachmentStreamKey, objectNumber int) {
	s.attachments.streams[key] = objectNumber
}

func (s *resourceStore) attachmentFileObject(key attachmentFileKey) int {
	return s.attachments.files[key]
}

func (s *resourceStore) setAttachmentFileObject(key attachmentFileKey, objectNumber int) {
	s.attachments.files[key] = objectNumber
}

func (s *resourceStore) cleanupAttachmentCompressedFiles() {
	for key, stream := range s.attachments.compressed {
		stream.cleanup()
		if stream.tempFile != "" {
			delete(s.attachments.compressed, key)
		}
	}
}
