// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"fmt"
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
	f.outf("q %.5f 0 0 %.5f %.5f %.5f cm /I%s Do Q", w*f.k, h*f.k, x*f.k, (f.h-(y+h))*f.k, imageID)
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
	content := []byte(sprintf("q %.5f 0 0 %.5f %.5f %.5f cm /I%s Do Q", placement.w*f.k, placement.h*f.k, placement.x*f.k, (f.h-(placement.y+placement.h))*f.k, info.i))
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
			objIDPadded := fmt.Sprintf("%40d", objID)
			objIDBytes := []byte(objIDPadded)
			for j := pos; j < pos+40; j++ {
				objsIDData[i][j] = objIDBytes[j-pos]
			}
		}
		f.importedTplIDs[hash] = i + nOffset
	}
	for i = range objsIDData {
		f.newobj()
		f.out(string(objsIDData[i]))
	}
}

func (f *Document) putImportedPages() {
	if len(f.importedPages) == 0 {
		return
	}
	ids := make([]int, 0, len(f.importedPages))
	for id := range f.importedPages {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		page := f.importedPages[id]
		if page == nil || page.page == nil {
			continue
		}
		objects := page.page.Objects()
		refMap := make(map[importpdf.ObjRef]int, len(objects))
		nextID := f.n + 1
		for _, object := range objects {
			refMap[object.Ref] = nextID
			nextID++
		}
		for _, object := range objects {
			body := importpdf.RewriteIndirectRefs(object.Body, refMap)
			f.newobj()
			f.outbuf(bytes.NewReader(body))
			f.out("endobj")
		}
		resources := importpdf.RewriteIndirectRefs(page.page.Resources(), refMap)
		content := page.page.Content()
		filter := ""
		if f.compress {
			content = f.compressBytes(content)
			if f.err != nil {
				return
			}
			filter = "/Filter /FlateDecode\n"
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
	keyList := make([]string, 0, len(f.images))

	var key string
	for key = range f.images {
		keyList = append(keyList, key)
	}
	if f.catalogSort {
		sort.SliceStable(keyList, func(i, j int) bool {
			return f.images[keyList[i]].w < f.images[keyList[j]].w
		})
	}
	insertedImages := make(map[string]int, len(f.images))
	for _, key = range keyList {
		image := f.images[key]
		if image == nil {
			continue
		}
		insertedImageObjN, isFound := insertedImages[image.i]
		if isFound {
			image.n = insertedImageObjN
		} else {
			f.putimage(image)
			insertedImages[image.i] = image.n
		}
	}
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
		var trns fmtBuffer
		for _, v := range info.trns {
			trns.printf("%d %d ", v, v)
		}
		f.outf("/Mask [%s]", trns.String())
	}
	if info.smask != nil {
		f.outf("/SMask %d 0 R", f.n+1)
	}
	f.outf("/Length %d>>", len(info.data))
	f.putstream(info.data)
	f.out("endobj")
	if len(info.smask) > 0 {
		smask := &ImageInfo{w: info.w, h: info.h, cs: "DeviceGray", bpc: 8, f: "FlateDecode", dp: sprintf("/Predictor 15 /Colors 1 /BitsPerComponent 8 /Columns %d", int(info.w)), data: info.smask, scale: f.k}
		f.putimage(smask)
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

func (f *Document) putxobjectdict() {
	{
		var image *ImageInfo
		var key string
		keyList := make([]string, 0, len(f.images))
		for key = range f.images {
			keyList = append(keyList, key)
		}
		if f.catalogSort {
			sort.SliceStable(keyList, func(i, j int) bool {
				return f.images[keyList[i]].i < f.images[keyList[j]].i
			})
		}
		for _, key = range keyList {
			image = f.images[key]
			if image == nil {
				continue
			}
			f.outf("/I%s %d 0 R", image.i, image.n)
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
		ids := make([]int, 0, len(f.importedPages))
		for id := range f.importedPages {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			page := f.importedPages[id]
			if page != nil && page.objectID > 0 {
				f.outf("/IPG%d %d 0 R", id, page.objectID)
			}
		}
	}
}
