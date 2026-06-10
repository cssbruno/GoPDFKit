// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"sort"
	"strings"
)

func (f *Document) replaceAliases() {
	if len(f.aliasMap) == 0 {
		return
	}
	aliases := make([]aliasReplacement, 0, len(f.aliasMap))
	for alias, replacement := range f.aliasMap {
		if alias == "" {
			continue
		}
		aliases = append(aliases, aliasReplacement{alias: alias, replacement: replacement})
	}
	if len(aliases) == 0 {
		return
	}
	sort.Slice(aliases, func(i, j int) bool {
		if len(aliases[i].alias) == len(aliases[j].alias) {
			return aliases[i].alias < aliases[j].alias
		}
		return len(aliases[i].alias) > len(aliases[j].alias)
	})
	pairs := make([]string, 0, len(aliases)*4)
	needles := make([][]byte, 0, len(aliases)*2)
	for _, alias := range aliases {
		pairs = append(pairs, alias.alias, f.escape(alias.replacement))
		needles = append(needles, []byte(alias.alias))
		utf16Alias := utf8toutf16(alias.alias, false)
		pairs = append(pairs, utf16Alias, f.escape(utf8toutf16(alias.replacement, false)))
		needles = append(needles, []byte(utf16Alias))
	}
	replacer := strings.NewReplacer(pairs...)
	for n := 1; n <= f.page; n++ {
		pageBytes := f.pages[n].Bytes()
		if !containsAnyBytes(pageBytes, needles) {
			continue
		}
		s := f.pages[n].String()
		replaced := replacer.Replace(s)
		if replaced != s {
			f.pages[n].Truncate(0)
			_, _ = f.pages[n].WriteString(replaced)
		}
	}
}

type aliasReplacement struct {
	alias       string
	replacement string
}

func containsAnyBytes(data []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if bytes.Contains(data, needle) {
			return true
		}
	}
	return false
}

func (f *Document) putpages() {
	var wPt, hPt float64
	var pageSize Size
	var ok bool
	nb := f.page
	if len(f.aliasNbPagesStr) > 0 {
		f.RegisterAlias(f.aliasNbPagesStr, sprintf("%d", nb))
	}
	f.replaceAliases()
	if f.defOrientation == "P" {
		wPt = f.defPageSize.Wd * f.k
		hPt = f.defPageSize.Ht * f.k
	} else {
		wPt = f.defPageSize.Ht * f.k
		hPt = f.defPageSize.Wd * f.k
	}
	pagesObjectNumbers := make([]int, nb+1)
	nextObj := f.n + 1
	for n := 1; n <= nb; n++ {
		pagesObjectNumbers[n] = nextObj
		nextObj += 2 + len(f.pageLinks[n])
	}
	f.tagged.pageObjNums = ensureIntSliceLen(f.tagged.pageObjNums, nb+1)
	for n := 1; n <= nb; n++ {
		f.newobj()
		f.tagged.pageObjNums[n] = f.n
		f.out("<</Type /Page")
		f.out("/Parent 1 0 R")
		pageSize, ok = f.pageSizes[n]
		if ok {
			f.outPDFMediaBox(pageSize.Wd, pageSize.Ht)
		}
		if rotation := f.pageRotations[n]; rotation != 0 {
			f.outPDFKeyInt("/Rotate ", rotation, "")
		}
		for t, pb := range f.pageBoxes[n] {
			var scratch [96]byte
			buf := append(scratch[:0], '/')
			buf = append(buf, t...)
			buf = append(buf, " ["...)
			buf = appendPDFNumberSpace(buf, pb.X, 2)
			buf = appendPDFNumberSpace(buf, pb.Y, 2)
			buf = appendPDFNumberSpace(buf, pb.Wd, 2)
			buf = appendPDFNumber(buf, pb.Ht, 2)
			buf = append(buf, ']')
			f.outbytes(buf)
		}
		f.out("/Resources 2 0 R")
		if structParents := f.taggedPageStructParents(n); structParents >= 0 {
			f.outPDFKeyInt("/StructParents ", structParents, "")
		}
		if len(f.pageLinks[n])+len(f.pageAttachments[n]) > 0 {
			var annots fmtBuffer
			annots.printf("/Annots [")
			linkObjNum := f.n + 2
			for i := range f.pageLinks[n] {
				f.pageLinks[n][i].objNum = linkObjNum
				if f.pageLinks[n][i].structElem != nil {
					f.pageLinks[n][i].structElem.ObjRef = linkObjNum
				}
				annots.printf("%d 0 R ", linkObjNum)
				linkObjNum++
			}
			f.putAttachmentAnnotationLinks(&annots, n)
			annots.printf("]")
			f.out(annots.String())
			if f.compliance.PDFUA2 {
				f.out("/Tabs /S")
			}
		}
		if f.pdfVersion > "1.3" {
			f.out("/Group <</Type /Group /S /Transparency /CS /DeviceRGB>>")
		}
		f.outPDFKeyInt("/Contents ", f.n+1, " 0 R>>")
		f.out("endobj")
		f.newobj()
		if f.compress {
			data := f.compressBytes(f.pages[n].Bytes())
			if f.err != nil {
				return
			}
			f.outPDFKeyInt("<</Filter /FlateDecode /Length ", len(data), ">>")
			f.putstream(data)
		} else {
			f.outPDFKeyInt("<</Length ", f.pages[n].Len(), ">>")
			f.putstream(f.pages[n].Bytes())
		}
		f.out("endobj")
		for _, pl := range f.pageLinks[n] {
			f.putLinkAnnotation(pl, pagesObjectNumbers, hPt)
		}
	}
	f.offsets[1] = f.buffer.Len()
	f.out("1 0 obj")
	f.out("<</Type /Pages")
	var kids fmtBuffer
	kids.printf("/Kids [")
	for i := 1; i <= nb; i++ {
		kids.printf("%d 0 R ", pagesObjectNumbers[i])
	}
	kids.printf("]")
	f.out(kids.String())
	f.outPDFKeyInt("/Count ", nb, "")
	f.outPDFMediaBox(wPt, hPt)
	f.out(">>")
	f.out("endobj")
}

func (f *Document) putLinkAnnotation(pl pageLink, pagesObjectNumbers []int, defaultPageHeight float64) {
	f.newobj()
	var scratch [160]byte
	buf := append(scratch[:0], "<< /Type /Annot /Subtype /Link /Rect ["...)
	buf = appendPDFNumberSpace(buf, pl.x, 2)
	buf = appendPDFNumberSpace(buf, pl.y, 2)
	buf = appendPDFNumberSpace(buf, pl.x+pl.wd, 2)
	buf = appendPDFNumber(buf, pl.y-pl.ht, 2)
	buf = append(buf, "] /Border [0 0 0] /F 4"...)
	f.outbytes(buf)
	if pl.structParent >= 0 {
		f.outPDFKeyInt("/StructParent ", pl.structParent, "")
	}
	if pl.link == 0 {
		f.outf("/A << /S /URI /URI %s >>", f.textstring(pl.linkStr))
	} else {
		l := f.links[pl.link]
		h := defaultPageHeight
		if sz, ok := f.pageSizes[l.page]; ok {
			h = sz.Ht
		}
		pageObj := 0
		if l.page > 0 && l.page < len(pagesObjectNumbers) {
			pageObj = pagesObjectNumbers[l.page]
		}
		buf = append(scratch[:0], "/Dest ["...)
		buf = appendPDFIndirectRef(buf, pageObj)
		buf = append(buf, " /XYZ 0 "...)
		buf = appendPDFNumber(buf, h-l.y*f.k, 2)
		buf = append(buf, " null]"...)
		f.outbytes(buf)
	}
	f.out(">>")
	f.out("endobj")
}
