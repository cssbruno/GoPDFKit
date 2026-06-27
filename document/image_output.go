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
	f.outbytes(f.wrapTaggedContent(content, tag))
	if link > 0 || len(linkStr) > 0 {
		f.newLink(placement.x, placement.y, placement.w, placement.h, link, linkStr)
	}
}

// putImportedTemplates writes the imported template objects to the PDF.
func (f *Document) putImportedTemplates() {
	nOffset := f.n + 1
	objsIDHash := make([]string, len(f.importedObjs))
	objsIDData := make([][]byte, len(f.importedObjs))
	i := 0
	for k, v := range f.importedObjs {
		objsIDHash[i] = k
		objsIDData[i] = v
		i++
	}
	hashToObjID := make(map[string]int, len(f.importedObjs))
	for i = 0; i < len(objsIDHash); i++ {
		hashToObjID[objsIDHash[i]] = i + nOffset
	}
	for i = 0; i < len(objsIDData); i++ {
		hash := objsIDHash[i]
		for pos, h := range f.importedObjPos[hash] {
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
		f.importedTplIDs[hash] = i + nOffset
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
	if len(f.importedPages) == 0 {
		return
	}
	for id := 1; id <= f.importedPageSeq; id++ {
		page := f.importedPages[id]
		if page == nil || page.page == nil {
			continue
		}
		objects := page.page.Objects()
		refMap := make(map[importpdf.ObjRef]int, len(objects))
		baseID := f.n + 1
		nextID := baseID
		for _, object := range objects {
			refMap[object.Ref] = nextID
			nextID++
		}
		rewrittenObjects, resources := page.rewrittenImportData(baseID, objects, refMap)
		for _, body := range rewrittenObjects {
			f.newobj()
			f.outbuf(bytes.NewReader(body))
			f.out("endobj")
		}
		filter := ""
		content, encodedFilter, encoded := page.page.EncodedContent()
		if encoded {
			filter = "/Filter /" + encodedFilter + "\n"
		} else {
			content = page.page.Content()
			if compressedContent, compressed := f.compressStreamBytes(content); compressed {
				content = compressedContent
				filter = "/Filter /FlateDecode\n"
			} else if f.err != nil {
				return
			}
		}
		f.newobj()
		page.objectID = f.n
		f.outf("<</Type /XObject\n/Subtype /Form\n/FormType 1\n/BBox [0 0 %.2f %.2f]\n/Matrix [1 0 0 1 0 0]\n/Resources %s\n%s/Length %d>>",
			page.page.WidthPoints(), page.page.HeightPoints(), string(resources), filter, len(content))
		f.putstream(content)
		f.out("endobj")
	}
}

func (f *Document) putimages() {
	insertedImages := make(map[string]int, len(f.images))
	if !f.catalogSort {
		for _, image := range f.images {
			f.putImageOnce(image, insertedImages)
		}
		return
	}
	keyList := make([]string, 0, len(f.images))
	for key := range f.images {
		keyList = append(keyList, key)
	}
	sort.SliceStable(keyList, func(i, j int) bool {
		return f.images[keyList[i]].w < f.images[keyList[j]].w
	})
	for _, key := range keyList {
		image := f.images[key]
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
	f.newobj()
	info.n = f.n
	f.out("<</Type /XObject")
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
		f.outf("/DecodeParms <<%s>>", info.dp)
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
	f.outf("/Length %d>>", len(info.data))
	f.putstream(info.data)
	f.out("endobj")
	if len(info.smask) > 0 {
		f.putSoftMaskImage(info)
	}
	if info.cs == "Indexed" {
		f.newobj()
		if f.compress {
			pal := f.compressBytes(info.pal)
			if f.err != nil {
				return
			}
			f.outf("<</Filter /FlateDecode /Length %d>>", len(pal))
			f.putstream(pal)
		} else {
			f.outf("<</Length %d>>", len(info.pal))
			f.putstream(info.pal)
		}
		f.out("endobj")
	}
}

func (f *Document) putSoftMaskImage(info *ImageInfo) {
	f.newobj()
	f.out("<</Type /XObject")
	f.out("/Subtype /Image")
	f.outf("/Width %d", int(info.w))
	f.outf("/Height %d", int(info.h))
	f.out("/ColorSpace /DeviceGray")
	f.out("/BitsPerComponent 8")
	f.out("/Filter /FlateDecode")
	f.outf("/DecodeParms <</Predictor 15 /Colors 1 /BitsPerComponent 8 /Columns %d>>", int(info.w))
	f.outf("/Length %d>>", len(info.smask))
	f.putstream(info.smask)
	f.out("endobj")
}

func (f *Document) putxobjectdict() {
	{
		if !f.catalogSort {
			for _, image := range f.images {
				if image == nil {
					continue
				}
				f.outf("/I%s %d 0 R", image.i, image.n)
			}
		} else {
			keyList := make([]string, 0, len(f.images))
			for key := range f.images {
				keyList = append(keyList, key)
			}
			sort.SliceStable(keyList, func(i, j int) bool {
				return f.images[keyList[i]].i < f.images[keyList[j]].i
			})
			for _, key := range keyList {
				image := f.images[key]
				if image == nil {
					continue
				}
				f.outf("/I%s %d 0 R", image.i, image.n)
			}
		}
	}
	{
		var keyList []string
		var key string
		var tpl Template
		keyList = templateKeyList(f.templates, f.catalogSort)
		for _, key = range keyList {
			tpl = f.templates[key]
			if tpl == nil || invalidTemplate(tpl) {
				continue
			}
			id := tpl.ID()
			if objID, ok := f.templateObjects[id]; ok {
				f.outf("/TPL%s %d 0 R", id, objID)
			}
		}
	}
	{
		for tplName, objID := range f.importedTplObjs {
			if !validPDFResourceName(tplName) {
				f.SetErrorf("invalid imported template name: %s", tplName)
				return
			}
			f.outf("%s %d 0 R", tplName, f.importedTplIDs[objID])
		}
	}
	{
		for id := 1; id <= f.importedPageSeq; id++ {
			page := f.importedPages[id]
			if page != nil && page.objectID > 0 {
				f.outf("/IPG%d %d 0 R", id, page.objectID)
			}
		}
	}
}
