// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"strings"
)

func (f *Document) replaceAliases() {
	if len(f.aliasMap) == 0 {
		return
	}
	rawPairs := make([]string, 0, len(f.aliasMap)*2)
	utf16Pairs := make([]string, 0, len(f.aliasMap)*2)
	for alias, replacement := range f.aliasMap {
		if alias == "" {
			continue
		}
		rawPairs = append(rawPairs, alias, f.escape(replacement))
		utf16Pairs = append(utf16Pairs, utf8toutf16(alias, false), f.escape(utf8toutf16(replacement, false)))
	}
	if len(rawPairs) == 0 {
		return
	}
	rawReplacer := strings.NewReplacer(rawPairs...)
	utf16Replacer := strings.NewReplacer(utf16Pairs...)
	for n := 1; n <= f.page; n++ {
		s := f.pages[n].String()
		replaced := rawReplacer.Replace(s)
		replaced = utf16Replacer.Replace(replaced)
		if replaced != s {
			f.pages[n].Truncate(0)
			_, _ = f.pages[n].WriteString(replaced)
		}
	}
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
	for n := 1; n <= nb; n++ {
		f.newobj()
		pagesObjectNumbers[n] = f.n
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
		if len(f.pageLinks[n])+len(f.pageAttachments[n]) > 0 {
			var annots fmtBuffer
			annots.printf("/Annots [")
			for _, pl := range f.pageLinks[n] {
				annots.printf("<</Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] ", pl.x, pl.y, pl.x+pl.wd, pl.y-pl.ht)
				if pl.link == 0 {
					annots.printf("/A <</S /URI /URI %s>>>>", f.textstring(pl.linkStr))
				} else {
					l := f.links[pl.link]
					var sz Size
					var h float64
					sz, ok = f.pageSizes[l.page]
					if ok {
						h = sz.Ht
					} else {
						h = hPt
					}
					annots.printf("/Dest [%d 0 R /XYZ 0 %.2f null]>>", 1+2*l.page, h-l.y*f.k)
				}
			}
			f.putAttachmentAnnotationLinks(&annots, n)
			annots.printf("]")
			f.out(annots.String())
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
