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
			f.outf("/MediaBox [0 0 %.2f %.2f]", pageSize.Wd, pageSize.Ht)
		}
		if rotation := f.pageRotations[n]; rotation != 0 {
			f.outf("/Rotate %d", rotation)
		}
		for t, pb := range f.pageBoxes[n] {
			f.outf("/%s [%.2f %.2f %.2f %.2f]", t, pb.X, pb.Y, pb.Wd, pb.Ht)
		}
		f.out("/Resources 2 0 R")
		if structParents := f.taggedPageStructParents(n); structParents >= 0 {
			f.outf("/StructParents %d", structParents)
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
		f.outf("/Contents %d 0 R>>", f.n+1)
		f.out("endobj")
		f.newobj()
		if f.compress {
			data := f.compressBytes(f.pages[n].Bytes())
			if f.err != nil {
				return
			}
			f.outf("<</Filter /FlateDecode /Length %d>>", len(data))
			f.putstream(data)
		} else {
			f.outf("<</Length %d>>", f.pages[n].Len())
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
	f.outf("/Count %d", nb)
	f.outf("/MediaBox [0 0 %.2f %.2f]", wPt, hPt)
	f.out(">>")
	f.out("endobj")
}

func (f *Document) putLinkAnnotation(pl pageLink, pagesObjectNumbers []int, defaultPageHeight float64) {
	f.newobj()
	f.outf("<< /Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0]", pl.x, pl.y, pl.x+pl.wd, pl.y-pl.ht)
	if pl.structParent >= 0 {
		f.outf("/StructParent %d", pl.structParent)
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
		f.outf("/Dest [%d 0 R /XYZ 0 %.2f null]", pageObj, h-l.y*f.k)
	}
	f.out(">>")
	f.out("endobj")
}
