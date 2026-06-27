// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SetDefaultCatalogSort sets the default value of the catalog sort flag that
// will be used when initializing a new Document instance. See SetCatalogSort() for
// more details. Prefer NewWithDefaults for per-document configuration.
func SetDefaultCatalogSort(flag bool) {
	_gl.Lock()
	defer _gl.Unlock()
	_gl.catalogSort = flag
}

// SetCatalogSort sets a flag that will be used, if true, to consistently order
// the document's internal resource catalogs. This method is typically only
// used for test purposes to facilitate PDF comparison.
func (f *Document) SetCatalogSort(flag bool) {
	f.catalogSort = flag
}

// SetDefaultCreationDate sets the default value of the document creation date
// that will be used when initializing a new Document instance. See
// SetCreationDate() for more details. Prefer NewWithDefaults for per-document
// configuration.
func SetDefaultCreationDate(tm time.Time) {
	_gl.Lock()
	defer _gl.Unlock()
	_gl.creationDate = tm
}

// SetDefaultModificationDate sets the default document modification date used
// when initializing a new Document instance. See SetCreationDate() for more
// details. Prefer NewWithDefaults for per-document configuration.
func SetDefaultModificationDate(tm time.Time) {
	_gl.Lock()
	defer _gl.Unlock()
	_gl.modDate = tm
}

// SetCreationDate fixes the document's internal CreationDate value. By
// default, the time when the document is generated is used for this value.
// This method is typically only used for testing purposes to facilitate PDF
// comparison. Specify a zero-value time to revert to the default behavior.
func (f *Document) SetCreationDate(tm time.Time) {
	f.creationDate = tm
}

// SetModificationDate fixes the document's internal ModDate value.
// See SetCreationDate() for more details.
func (f *Document) SetModificationDate(tm time.Time) {
	f.modDate = tm
}

// SetJavascript adds Adobe JavaScript to the document.
func (f *Document) SetJavascript(script string) {
	f.javascript = &script
}

// RegisterAlias adds an (alias, replacement) pair to the document so we can
// replace all occurrences of that alias after writing but before the document
// is closed. Functions ExampleDocument_RegisterAlias() and
// ExampleDocument_RegisterAlias_utf8() in document_test.go demonstrate this method.
func (f *Document) RegisterAlias(alias, replacement string) {
	if current, ok := f.aliasMap[alias]; ok && current == replacement {
		return
	}
	f.aliasMap[alias] = replacement
	f.aliasPairsDirty = true
	f.aliasNeedlesDirty = true
	f.markPagesContainingAlias(alias)
}

func (f *Document) putresourcedict() {
	if !f.omitDeprecatedPDF2Entries() {
		f.out("/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")
	}
	f.out("/Font <<")
	{
		if !f.catalogSort {
			for _, font := range f.fonts {
				f.outbytes(appendPDFResourceRef(nil, "/F", font.i, font.N))
			}
		} else {
			keyList := make([]string, 0, len(f.fonts))
			for key := range f.fonts {
				keyList = append(keyList, key)
			}
			sort.SliceStable(keyList, func(i, j int) bool {
				return f.fonts[keyList[i]].i < f.fonts[keyList[j]].i
			})
			for _, key := range keyList {
				font := f.fonts[key]
				f.outbytes(appendPDFResourceRef(nil, "/F", font.i, font.N))
			}
		}
	}
	f.out(">>")
	f.out("/XObject <<")
	f.putxobjectdict()
	f.out(">>")
	count := len(f.blendList)
	if count > 1 {
		f.out("/ExtGState <<")
		for j := 1; j < count; j++ {
			buf := strconv.AppendInt([]byte("/GS"), int64(j), 10)
			buf = append(buf, ' ')
			buf = strconv.AppendInt(buf, int64(f.blendList[j].objNum), 10)
			buf = append(buf, " 0 R"...)
			f.outbytes(buf)
		}
		f.out(">>")
	}
	count = len(f.gradientList)
	if count > 1 {
		f.out("/Shading <<")
		for j := 1; j < count; j++ {
			buf := strconv.AppendInt([]byte("/Sh"), int64(j), 10)
			buf = append(buf, ' ')
			buf = strconv.AppendInt(buf, int64(f.gradientList[j].objNum), 10)
			buf = append(buf, " 0 R"...)
			f.outbytes(buf)
		}
		f.out(">>")
	}
	f.layerPutResourceDict()
	f.putSpotColorResourceDict()
}

func (f *Document) putjavascript() {
	if f.javascript == nil {
		return
	}
	f.newobj()
	f.nJs = f.n
	f.out("<<")
	f.outf("/Names [(EmbeddedJS) %d 0 R]", f.n+1)
	f.out(">>")
	f.out("endobj")
	f.newobj()
	f.out("<<")
	f.out("/S /JavaScript")
	f.outbytes(f.appendTextString([]byte("/JS "), *f.javascript))
	f.out(">>")
	f.out("endobj")
}

func (f *Document) putresources() {
	if f.err != nil {
		return
	}
	f.layerPutLayers()
	f.putBlendModes()
	f.putGradients()
	f.putSpotColors()
	f.putfonts()
	if f.err != nil {
		return
	}
	f.putimages()
	f.putTemplates()
	f.putImportedTemplates()
	f.putImportedPages()
	f.offsets[2] = f.buffer.Len()
	f.out("2 0 obj")
	f.out("<<")
	f.putresourcedict()
	f.out(">>")
	f.out("endobj")
	f.putjavascript()
	if f.protect.encrypted {
		f.newobj()
		f.protect.objNum = f.n
		f.out("<<")
		f.out("/Filter /Standard")
		f.out("/V 1")
		f.out("/R 2")
		f.outf("/O (%s)", f.escape(string(f.protect.oValue)))
		f.outf("/U (%s)", f.escape(string(f.protect.uValue)))
		f.outf("/P %d", f.protect.pValue)
		f.out(">>")
		f.out("endobj")
	}
}

func appendPDFResourceRef(buf []byte, prefix, name string, objNum int) []byte {
	buf = append(buf, prefix...)
	buf = append(buf, name...)
	buf = append(buf, ' ')
	buf = strconv.AppendInt(buf, int64(objNum), 10)
	buf = append(buf, " 0 R"...)
	return buf
}

// timeOrNow returns time.Now() if tm is zero.
func timeOrNow(tm time.Time) time.Time {
	if tm.IsZero() {
		return time.Now()
	}
	return tm
}

func (f *Document) putinfo() {
	if len(f.producer) > 0 {
		f.outf("/Producer %s", f.textstring(f.producer))
	}
	if len(f.title) > 0 {
		f.outf("/Title %s", f.textstring(f.title))
	}
	if len(f.subject) > 0 {
		f.outf("/Subject %s", f.textstring(f.subject))
	}
	if len(f.author) > 0 {
		f.outf("/Author %s", f.textstring(f.author))
	}
	if len(f.keywords) > 0 {
		f.outf("/Keywords %s", f.textstring(f.keywords))
	}
	if len(f.creator) > 0 {
		f.outf("/Creator %s", f.textstring(f.creator))
	}
	creation := timeOrNow(f.creationDate)
	f.outf("/CreationDate %s", f.textstring("D:"+creation.Format("20060102150405")))
	mod := timeOrNow(f.modDate)
	f.outf("/ModDate %s", f.textstring("D:"+mod.Format("20060102150405")))
}

func (f *Document) putcatalog() {
	f.out("/Type /Catalog")
	f.out("/Pages 1 0 R")
	if f.nXmp > 0 {
		f.outf("/Metadata %d 0 R", f.nXmp)
	}
	if f.compliance.PDFUA2 {
		f.out("/MarkInfo << /Marked true >>")
		if strings.TrimSpace(f.compliance.Lang) != "" {
			f.outf("/Lang %s", f.textstring(f.compliance.Lang))
		}
		f.out("/ViewerPreferences << /DisplayDocTitle true >>")
	}
	if f.tagged.structTreeRootObj > 0 {
		f.outf("/StructTreeRoot %d 0 R", f.tagged.structTreeRootObj)
	}
	if f.nOutputIntentICC > 0 {
		f.outf("/OutputIntents [ << /Type /OutputIntent /S /GTS_PDFA1 /OutputConditionIdentifier %s /Info %s /DestOutputProfile %d 0 R >> ]",
			f.textstring(f.outputIntent.identifier),
			f.textstring(firstNonEmpty(f.outputIntent.info, f.outputIntent.identifier)),
			f.nOutputIntentICC)
	}
	switch f.zoomMode {
	case "fullpage":
		f.out("/OpenAction [3 0 R /Fit]")
	case "fullwidth":
		f.out("/OpenAction [3 0 R /FitH null]")
	case "real":
		f.out("/OpenAction [3 0 R /XYZ null 1]")
	}
	switch f.layoutMode {
	case "single", "SinglePage":
		f.out("/PageLayout /SinglePage")
	case "continuous", "OneColumn":
		f.out("/PageLayout /OneColumn")
	case "two", "TwoColumnLeft":
		f.out("/PageLayout /TwoColumnLeft")
	case "TwoColumnRight":
		f.out("/PageLayout /TwoColumnRight")
	case "TwoPageLeft", "TwoPageRight":
		if f.pdfVersion < "1.5" {
			f.pdfVersion = "1.5"
		}
		f.out("/PageLayout /" + f.layoutMode)
	}
	if len(f.outlines) > 0 {
		f.outf("/Outlines %d 0 R", f.outlineRoot)
		f.out("/PageMode /UseOutlines")
	}
	f.layerPutCatalog()
	if f.javascript != nil || len(f.attachments) > 0 {
		f.out("/Names <<")
		if f.javascript != nil {
			f.outf("/JavaScript %d 0 R", f.nJs)
		}
		if len(f.attachments) > 0 {
			f.outf("/EmbeddedFiles %s", f.getEmbeddedFiles())
		}
		f.out(">>")
	}
}

func (f *Document) putheader() {
	if len(f.blendMap) > 0 && f.pdfVersion < "1.4" {
		f.pdfVersion = "1.4"
	}
	f.outf("%%PDF-%s", f.pdfVersion)
	if f.compliance.PDFA != PDFAModeNone {
		f.out("%\xE2\xE3\xCF\xD3")
	}
}

func (f *Document) puttrailer() {
	f.outf("/Size %d", f.n+1)
	f.outf("/Root %d 0 R", f.n)
	if !f.omitInfoDictionary() {
		f.outf("/Info %d 0 R", f.n-1)
	}
	if f.protect.encrypted {
		f.outf("/Encrypt %d 0 R", f.protect.objNum)
		f.out("/ID [()()]")
	} else if f.compliance.PDFA != PDFAModeNone || f.compliance.Arlington {
		id := f.fileIdentifier()
		f.outf("/ID [<%s><%s>]", id, id)
	}
}

func (f *Document) omitInfoDictionary() bool {
	return f.compliance.PDFA != PDFAModeNone || f.compliance.Arlington
}

func (f *Document) omitDeprecatedPDF2Entries() bool {
	return f.compliance.PDFA != PDFAModeNone || f.compliance.Arlington
}

func (f *Document) fileIdentifier() string {
	if f.fileIDHash != nil {
		return strings.ToUpper(hex.EncodeToString(f.fileIDHash.Sum(nil)[:16]))
	}
	sum := sha256.Sum256(f.buffer.Bytes())
	return strings.ToUpper(hex.EncodeToString(sum[:16]))
}

func (f *Document) putxmp() {
	if len(f.xmp) == 0 {
		return
	}
	f.newobj()
	f.nXmp = f.n
	f.outf("<< /Type /Metadata /Subtype /XML /Length %d >>", len(f.xmp))
	f.putstream(f.xmp)
	f.out("endobj")
}

func (f *Document) putOutputIntent() {
	if len(f.outputIntent.iccProfile) == 0 {
		return
	}
	f.newobj()
	f.nOutputIntentICC = f.n
	f.outf("<< /N 3 /Alternate /DeviceRGB /Length %d >>", len(f.outputIntent.iccProfile))
	f.putstream(f.outputIntent.iccProfile)
	f.out("endobj")
}

func (f *Document) putbookmarks() {
	nb := len(f.outlines)
	if nb > 0 {
		stack := make([]int, 0, nb)
		rootFirst, rootLast := -1, -1
		for i := range f.outlines {
			level := f.outlines[i].level
			if level < 0 || level > len(stack) {
				f.SetErrorf("invalid bookmark level: %d", level)
				return
			}
			if level < len(stack) {
				stack = stack[:level+1]
			}

			f.outlines[i].parent = nb
			f.outlines[i].first = -1
			f.outlines[i].last = -1
			f.outlines[i].next = -1
			f.outlines[i].prev = -1

			if level > 0 {
				parent := stack[level-1]
				f.outlines[i].parent = parent
				if f.outlines[parent].first == -1 {
					f.outlines[parent].first = i
				}
				f.outlines[parent].last = i
			} else {
				if rootFirst == -1 {
					rootFirst = i
				}
				rootLast = i
			}

			if level < len(stack) {
				prev := stack[level]
				f.outlines[prev].next = i
				f.outlines[i].prev = prev
				stack[level] = i
			} else {
				stack = append(stack, i)
			}
		}
		n := f.n + 1
		pageHeights := make([]float64, f.page+1)
		for page := 1; page <= f.page; page++ {
			pageHeights[page] = f.pageHeightPt(page)
		}
		for _, o := range f.outlines {
			pageObj := 0
			if o.p > 0 && o.p < len(f.pageObjectNumbers) {
				pageObj = f.pageObjectNumbers[o.p]
			}
			if pageObj == 0 {
				f.SetErrorf("invalid bookmark destination page: %d", o.p)
				return
			}
			f.newobj()
			title := o.text
			if o.utf8 {
				title = utf8toutf16(title)
			}
			buf := append([]byte("<</Title "), f.textstring(title)...)
			buf = append(buf, '\n')
			buf = appendPDFNamedObjectRef(buf, "/Parent ", n+o.parent)
			if o.prev != -1 {
				buf = appendPDFNamedObjectRef(buf, "/Prev ", n+o.prev)
			}
			if o.next != -1 {
				buf = appendPDFNamedObjectRef(buf, "/Next ", n+o.next)
			}
			if o.first != -1 {
				buf = appendPDFNamedObjectRef(buf, "/First ", n+o.first)
			}
			if o.last != -1 {
				buf = appendPDFNamedObjectRef(buf, "/Last ", n+o.last)
			}
			buf = append(buf, "/Dest ["...)
			buf = strconv.AppendInt(buf, int64(pageObj), 10)
			buf = append(buf, " 0 R /XYZ 0 "...)
			buf = strconv.AppendFloat(buf, pageHeights[o.p]-o.y*f.k, 'f', 2, 64)
			buf = append(buf, " null]\n/Count 0>>\nendobj"...)
			f.outbytes(buf)
		}
		f.newobj()
		f.outlineRoot = f.n
		buf := appendPDFNamedObjectRef([]byte("<</Type /Outlines "), "/First ", n+rootFirst)
		buf = appendPDFNamedObjectRef(buf, "/Last ", n+rootLast)
		buf = append(buf, ">>\nendobj"...)
		f.outbytes(buf)
	}
}

func appendPDFNamedObjectRef(buf []byte, prefix string, objNum int) []byte {
	buf = append(buf, prefix...)
	buf = strconv.AppendInt(buf, int64(objNum), 10)
	buf = append(buf, " 0 R\n"...)
	return buf
}

func (f *Document) enddoc() {
	if f.err != nil {
		return
	}
	f.validateComplianceMetadata()
	if f.err != nil {
		return
	}
	f.ensureComplianceMetadata()
	f.buffer.Grow(f.estimateFinalBufferSize())
	f.fileIDHash = sha256.New()
	f.layerEndDoc()
	f.putheader()
	f.prepareAttachmentCompression()
	f.putAttachments()
	f.putAnnotationsAttachments()
	f.putpages()
	f.putresources()
	if f.err != nil {
		return
	}
	f.putbookmarks()
	f.putOutputIntent()
	f.putxmp()
	f.putTaggedPDF()
	if !f.omitInfoDictionary() {
		f.newobj()
		f.out("<<")
		f.putinfo()
		f.out(">>")
		f.out("endobj")
	}
	f.newobj()
	f.out("<<")
	f.putcatalog()
	f.out(">>")
	f.out("endobj")
	o := f.buffer.Len()
	f.out("xref")
	f.outPDFXrefRange(f.n + 1)
	f.out("0000000000 65535 f ")
	for j := 1; j <= f.n; j++ {
		f.outPDFXrefOffset(f.offsets[j])
	}
	f.out("trailer")
	f.out("<<")
	f.puttrailer()
	f.out(">>")
	f.out("startxref")
	f.outPDFIntLine(o)
	f.out("%%EOF")
	f.state = 3
}

func (f *Document) estimateFinalBufferSize() int {
	size := f.buffer.Len() + 4096
	size += len(f.pages) * 1024
	size += len(f.images) * 2048
	size += len(f.fonts) * 1024
	size += len(f.templates) * 1024
	size += len(f.importedObjs) * 1024
	size += len(f.importedPages) * 1024
	size += len(f.attachments) * 1024
	size += len(f.xmp)
	if f.javascript != nil {
		size += len(*f.javascript)
	}
	if size < 0 {
		return 0
	}
	return size
}
