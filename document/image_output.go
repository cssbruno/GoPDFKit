// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"sort"

	"github.com/cssbruno/gopdfkit/importpdf"
)

type imagePlacement struct {
	x, y, w, h float64
}

func (f *Document) resolveImagePlacement(info *ImageInfo, x, y, w, h float64, allowNegativeX, flow bool) (imagePlacement, bool) {
	if info == nil {
		f.SetErrorf("image info is nil")
		return imagePlacement{}, false
	}
	if w == 0 && h == 0 {
		w = -96
		h = -96
	}
	if w == -1 {
		w = -info.dpi
	}
	if h == -1 {
		h = -info.dpi
	}
	if w < 0 {
		w = -info.w * 72.0 / w / f.k
	}
	if h < 0 {
		h = -info.h * 72.0 / h / f.k
	}
	if w == 0 {
		w = h * info.w / info.h
	}
	if h == 0 {
		h = w * info.h / info.w
	}
	if flow {
		if f.y+h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
			x2 := f.x
			f.addPageFormatRotation(f.curOrientation, f.curPageSize, f.curRotation)
			if f.err != nil {
				return imagePlacement{}, false
			}
			f.x = x2
		}
		y = f.y
		f.y += h
	}
	if !allowNegativeX {
		if x < 0 {
			x = f.x
		}
	}
	return imagePlacement{x: x, y: y, w: w, h: h}, true
}

func (f *Document) drawImageXObject(imageID string, x, y, w, h float64) {
	f.outbytes(f.appendImageXObject(nil, imageID, x, y, w, h))
}

func (f *Document) appendImageXObject(buf []byte, imageID string, x, y, w, h float64) []byte {
	buf = append(buf, "q "...)
	buf = appendPDFNumberSpace(buf, w*f.k, 5)
	buf = append(buf, "0 0 "...)
	buf = appendPDFNumberSpace(buf, h*f.k, 5)
	buf = appendPDFNumberSpace(buf, x*f.k, 5)
	buf = appendPDFNumberSpace(buf, (f.h-(y+h))*f.k, 5)
	buf = append(buf, "cm /I"...)
	buf = append(buf, imageID...)
	buf = append(buf, " Do Q"...)
	return buf
}

func (f *Document) imageOut(info *ImageInfo, x, y, w, h float64, allowNegativeX, flow bool, link int, linkStr string, tag taggedContentOptions) {
	if info == nil {
		f.err = errors.New("missing image info")
		return
	}
	placement, ok := f.resolveImagePlacement(info, x, y, w, h, allowNegativeX, flow)
	if !ok {
		return
	}
	if !f.validateTaggedImageOptions(tag) {
		return
	}
	content := f.appendImageXObject(make([]byte, 0, len(info.i)+96), info.i, placement.x, placement.y, placement.w, placement.h)
	f.outTaggedContent(content, tag)
	if link > 0 || len(linkStr) > 0 {
		f.newLink(placement.x, placement.y, placement.w, placement.h, link, linkStr)
	}
}

// putImportedTemplates writes the imported template objects to the PDF.
func (f *Document) putImportedTemplates() {
	resources := f.ensureResourceStore()
	nOffset := f.n + 1
	objsIDHash := resources.importedObjectHashes(f.catalogSort)
	objsIDData := make([][]byte, len(objsIDHash))
	var i int
	for i = 0; i < len(objsIDHash); i++ {
		objsIDData[i] = resources.importedObjectData(objsIDHash[i])
	}
	hashToObjID := make(map[string]int, len(objsIDHash))
	for i = 0; i < len(objsIDHash); i++ {
		hashToObjID[objsIDHash[i]] = i + nOffset
	}
	for i = 0; i < len(objsIDData); i++ {
		hash := objsIDHash[i]
		positions := resources.importedObjectPositions(hash)
		for _, pos := range importedObjectReplacementPositions(positions, f.catalogSort) {
			h := positions[pos]
			objID, ok := hashToObjID[h]
			if !ok {
				f.SetErrorf("invalid imported object reference: %s", h)
				return
			}
			if pos < 0 || pos+40 > len(objsIDData[i]) {
				f.SetErrorf("invalid imported object replacement offset: %d", pos)
				return
			}
			writePaddedObjectID(objsIDData[i][pos:pos+40], objID)
		}
		resources.setImportedTemplateObjectID(hash, i+nOffset)
	}
	for i = range objsIDData {
		f.newobj()
		f.outbytes(objsIDData[i])
	}
}

func writePaddedObjectID(dst []byte, objID int) {
	for i := range dst {
		dst[i] = ' '
	}
	var scratch [20]byte
	raw := appendPDFInt(scratch[:0], objID)
	copy(dst[len(dst)-len(raw):], raw)
}

func (f *Document) putImportedPages() {
	resources := f.ensureResourceStore()
	if !resources.hasImportedPages() {
		return
	}
	for id := 1; id <= f.importedPageSeq; id++ {
		page, _ := resources.importedPage(id)
		if page == nil || page.page == nil {
			continue
		}
		refs := page.page.ObjectRefs()
		refMap := make(map[importpdf.ObjRef]int, len(refs))
		baseID := f.n + 1
		nextID := baseID
		for _, ref := range refs {
			refMap[ref] = nextID
			nextID++
		}
		rewrittenObjects, resources := page.rewrittenImportData(baseID, refs, refMap)
		for _, body := range rewrittenObjects {
			f.newobj()
			_ = f.outbuf(bytes.NewReader(body))
			f.endPDFObject()
		}
		filter := ""
		content, encodedFilter, encoded := page.page.EncodedContent()
		if encoded {
			filter = encodedFilter
		} else {
			content = page.page.Content()
			if err := page.page.ContentErr(); err != nil {
				f.SetError(err)
				return
			}
			if compressedContent, compressed := f.compressStreamBytes(content); compressed {
				content = compressedContent
				filter = "FlateDecode"
			} else if f.err != nil {
				return
			}
		}
		f.newPDFDictObject()
		page.objectID = f.n
		f.out("/Type /XObject")
		f.out("/Subtype /Form")
		f.out("/FormType 1")
		f.outf("/BBox [0 0 %.2f %.2f]", page.page.WidthPoints(), page.page.HeightPoints())
		f.out("/Matrix [1 0 0 1 0 0]")
		f.outf("/Resources %s", string(resources))
		if filter != "" {
			f.outf("/Filter /%s", filter)
		}
		f.outf("/Length %d", len(content))
		f.endPDFDict()
		f.putstream(content)
		f.endPDFObject()
	}
}

func (f *Document) putimages() {
	images := f.ensureResourceStore().imagesForOutput(f.catalogSort)
	insertedImages := make(map[string]int, len(images))
	for _, image := range images {
		f.putImageOnce(image, insertedImages)
	}
}

func (f *Document) putImageOnce(image *ImageInfo, insertedImages map[string]int) {
	if image == nil {
		return
	}
	if insertedImageObjN, isFound := insertedImages[image.i]; isFound {
		image.n = insertedImageObjN
		return
	}
	f.putimage(image)
	insertedImages[image.i] = image.n
}

func (f *Document) putimage(info *ImageInfo) {
	if err := info.validForPDF(); err != nil {
		f.err = err
		return
	}
	f.newPDFDictObject()
	info.n = f.n
	f.out("/Type /XObject")
	f.out("/Subtype /Image")
	f.outf("/Width %d", int(info.w))
	f.outf("/Height %d", int(info.h))
	if info.cs == "Indexed" {
		f.outf("/ColorSpace [/Indexed /DeviceRGB %d %d 0 R]", len(info.pal)/3-1, f.n+1)
	} else {
		f.outf("/ColorSpace /%s", info.cs)
		if info.cs == "DeviceCMYK" {
			f.out("/Decode [1 0 1 0 1 0 1 0]")
		}
	}
	f.outf("/BitsPerComponent %d", info.bpc)
	if len(info.f) > 0 {
		f.outf("/Filter /%s", info.f)
	}
	if len(info.dp) > 0 {
		f.out("/DecodeParms")
		f.beginPDFDict()
		f.out(info.dp)
		f.endPDFDict()
	}
	if len(info.trns) > 0 {
		trns := make([]byte, 0, len(info.trns)*8)
		for _, v := range info.trns {
			trns = appendPDFInt(trns, v)
			trns = append(trns, ' ')
			trns = appendPDFInt(trns, v)
			trns = append(trns, ' ')
		}
		f.outbytes(append(append([]byte("/Mask ["), trns...), ']'))
	}
	if info.smask != nil {
		f.outf("/SMask %d 0 R", f.n+1)
	}
	f.outf("/Length %d", len(info.data))
	f.endPDFDict()
	f.putstream(info.data)
	f.endPDFObject()
	if len(info.smask) > 0 {
		f.putSoftMaskImage(info)
	}
	if info.cs == "Indexed" {
		f.newPDFDictObject()
		if f.compress {
			pal := f.compressBytes(info.pal)
			if f.err != nil {
				return
			}
			f.out("/Filter /FlateDecode")
			f.outf("/Length %d", len(pal))
			f.endPDFDict()
			f.putstream(pal)
		} else {
			f.outf("/Length %d", len(info.pal))
			f.endPDFDict()
			f.putstream(info.pal)
		}
		f.endPDFObject()
	}
}

func (f *Document) putSoftMaskImage(info *ImageInfo) {
	f.newPDFDictObject()
	f.out("/Type /XObject")
	f.out("/Subtype /Image")
	f.outf("/Width %d", int(info.w))
	f.outf("/Height %d", int(info.h))
	f.out("/ColorSpace /DeviceGray")
	f.out("/BitsPerComponent 8")
	f.out("/Filter /FlateDecode")
	f.out("/DecodeParms")
	f.beginPDFDict()
	f.outf("/Predictor 15 /Colors 1 /BitsPerComponent 8 /Columns %d", int(info.w))
	f.endPDFDict()
	f.outf("/Length %d", len(info.smask))
	f.endPDFDict()
	f.putstream(info.smask)
	f.endPDFObject()
}

func (f *Document) putxobjectdict() {
	{
		for _, image := range f.ensureResourceStore().imagesByResourceID(f.catalogSort) {
			if image == nil {
				continue
			}
			f.outbytes(appendPDFResourceRefValue(nil, imagePDFResourceRef(image)))
		}
	}
	{
		resources := f.ensureResourceStore()
		for _, key := range resources.templateCatalogKeys(f.catalogSort) {
			tpl, _ := resources.template(key)
			if tpl == nil || invalidTemplate(tpl) {
				continue
			}
			id := tpl.ID()
			if objID, ok := resources.templateObject(id); ok {
				f.outbytes(appendPDFResourceRefValue(nil, templatePDFResourceRef(id, objID)))
			}
		}
	}
	{
		resources := f.ensureResourceStore()
		for _, ref := range resources.importedTemplateResourceRefs(f.catalogSort) {
			if !validPDFResourceName(ref.name.String()) {
				f.SetErrorf("invalid imported template name: %s", ref.name.String())
				return
			}
			f.outbytes(appendPDFResourceRefValue(nil, ref))
		}
	}
	{
		resources := f.ensureResourceStore()
		for id := 1; id <= f.importedPageSeq; id++ {
			page, _ := resources.importedPage(id)
			if page != nil && page.objectID > 0 {
				f.outbytes(appendPDFResourceRefValue(nil, importedPagePDFResourceRef(id, page.objectID)))
			}
		}
	}
}

func importedObjectReplacementPositions(positions map[int]string, sorted bool) []int {
	keys := make([]int, 0, len(positions))
	for pos := range positions {
		keys = append(keys, pos)
	}
	if sorted {
		sort.Ints(keys)
	}
	return keys
}

func importedTemplateOutputNames(templates map[string]string, sorted bool) []string {
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	if sorted {
		sort.Strings(names)
	}
	return names
}
