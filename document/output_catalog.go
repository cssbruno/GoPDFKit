// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

// SetDefaultCatalogSort sets the default value of the catalog sort flag that
// will be used when initializing a new Document instance. See SetCatalogSort() for
// more details.
func SetDefaultCatalogSort(flag bool) {
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
// SetCreationDate() for more details.
func SetDefaultCreationDate(tm time.Time) {
	_gl.creationDate = tm
}

// SetDefaultModificationDate sets the default document modification date used
// when initializing a new Document instance. See SetCreationDate() for more
// details.
func SetDefaultModificationDate(tm time.Time) {
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
	f.aliasMap[alias] = replacement
}

func (f *Document) putresourcedict() {
	if !f.omitDeprecatedPDF2Entries() {
		f.out("/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")
	}
	f.out("/Font <<")
	{
		var keyList []string
		var font fontDefinition
		var key string
		for key = range f.fonts {
			keyList = append(keyList, key)
		}
		if f.catalogSort {
			sort.SliceStable(keyList, func(i, j int) bool {
				return f.fonts[keyList[i]].i < f.fonts[keyList[j]].i
			})
		}
		for _, key = range keyList {
			font = f.fonts[key]
			f.outf("/F%s %d 0 R", font.i, font.N)
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
			f.outf("/GS%d %d 0 R", j, f.blendList[j].objNum)
		}
		f.out(">>")
	}
	count = len(f.gradientList)
	if count > 1 {
		f.out("/Shading <<")
		for j := 1; j < count; j++ {
			f.outf("/Sh%d %d 0 R", j, f.gradientList[j].objNum)
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
	f.outf("/JS %s", f.textstring(*f.javascript))
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
	f.out("/Names <<")
	if f.javascript != nil {
		f.outf("/JavaScript %d 0 R", f.nJs)
	}
	f.outf("/EmbeddedFiles %s", f.getEmbeddedFiles())
	f.out(">>")
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
		lru := make(map[int]int)
		level := 0
		for i, o := range f.outlines {
			if o.level > 0 {
				parent := lru[o.level-1]
				f.outlines[i].parent = parent
				f.outlines[parent].last = i
				if o.level > level {
					f.outlines[parent].first = i
				}
			} else {
				f.outlines[i].parent = nb
			}
			if o.level <= level && i > 0 {
				prev := lru[o.level]
				f.outlines[prev].next = i
				f.outlines[i].prev = prev
			}
			lru[o.level] = i
			level = o.level
		}
		n := f.n + 1
		for _, o := range f.outlines {
			f.newobj()
			f.outf("<</Title %s", f.textstring(o.text))
			f.outf("/Parent %d 0 R", n+o.parent)
			if o.prev != -1 {
				f.outf("/Prev %d 0 R", n+o.prev)
			}
			if o.next != -1 {
				f.outf("/Next %d 0 R", n+o.next)
			}
			if o.first != -1 {
				f.outf("/First %d 0 R", n+o.first)
			}
			if o.last != -1 {
				f.outf("/Last %d 0 R", n+o.last)
			}
			f.outf("/Dest [%d 0 R /XYZ 0 %.2f null]", 1+2*o.p, (f.h-o.y)*f.k)
			f.out("/Count 0>>")
			f.out("endobj")
		}
		f.newobj()
		f.outlineRoot = f.n
		f.outf("<</Type /Outlines /First %d 0 R", n)
		f.outf("/Last %d 0 R>>", n+lru[0])
		f.out("endobj")
	}
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
	f.layerEndDoc()
	f.putheader()
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
	f.outf("0 %d", f.n+1)
	f.out("0000000000 65535 f ")
	for j := 1; j <= f.n; j++ {
		f.outf("%010d 00000 n ", f.offsets[j])
	}
	f.out("trailer")
	f.out("<<")
	f.puttrailer()
	f.out(">>")
	f.out("startxref")
	f.outf("%d", o)
	f.out("%%EOF")
	f.state = 3
}
