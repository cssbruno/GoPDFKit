// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func (f *Document) replaceAliases() {
	if len(f.aliasMap) == 0 {
		return
	}
	pairs := f.compiledAliasPairs()
	if len(pairs) == 0 {
		return
	}
	for n := 1; n <= f.page; n++ {
		if n < len(f.aliasPages) && !f.aliasPages[n] {
			continue
		}
		pageBytes := f.pages[n].Bytes()
		replaced := replaceAliasBytes(pageBytes, pairs)
		if replaced != nil {
			f.pages[n].Truncate(0)
			_, _ = f.pages[n].Write(replaced)
		}
	}
}

func (f *Document) compiledAliasPairs() []aliasReplacementBytes {
	if !f.aliasPairsDirty && f.aliasPairs != nil {
		return f.aliasPairs
	}
	aliases := make([]aliasReplacement, 0, len(f.aliasMap))
	for alias, replacement := range f.aliasMap {
		if alias == "" {
			continue
		}
		aliases = append(aliases, aliasReplacement{alias: alias, replacement: replacement})
	}
	if len(aliases) == 0 {
		f.aliasPairs = nil
		f.aliasPairsDirty = false
		return f.aliasPairs
	}
	sort.Slice(aliases, func(i, j int) bool {
		if len(aliases[i].alias) == len(aliases[j].alias) {
			return aliases[i].alias < aliases[j].alias
		}
		return len(aliases[i].alias) > len(aliases[j].alias)
	})
	pairs := make([]aliasReplacementBytes, 0, len(aliases)*2)
	for _, alias := range aliases {
		pairs = append(pairs, aliasReplacementBytes{
			old: []byte(alias.alias),
			new: []byte(f.escape(alias.replacement)),
		})
		utf16Alias := utf8toutf16(alias.alias, false)
		pairs = append(pairs, aliasReplacementBytes{
			old: []byte(utf16Alias),
			new: []byte(f.escape(utf8toutf16(alias.replacement, false))),
		})
	}
	f.aliasPairs = pairs
	f.aliasPairsDirty = false
	return f.aliasPairs
}

func (f *Document) compiledAliasNeedles() [][]byte {
	if !f.aliasNeedlesDirty && f.aliasNeedles != nil {
		return f.aliasNeedles
	}
	seen := make(map[string]bool, len(f.aliasMap)+1)
	aliases := make([]string, 0, len(f.aliasMap)+1)
	if f.aliasNbPagesStr != "" {
		seen[f.aliasNbPagesStr] = true
		aliases = append(aliases, f.aliasNbPagesStr)
	}
	for alias := range f.aliasMap {
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	needles := make([][]byte, 0, len(aliases)*2)
	needleStrings := make([]string, 0, len(aliases)*2)
	for _, alias := range aliases {
		utf16Alias := utf8toutf16(alias, false)
		needles = append(needles, []byte(alias), []byte(utf16Alias))
		needleStrings = append(needleStrings, alias, utf16Alias)
	}
	f.aliasNeedles = needles
	f.aliasNeedleStrings = needleStrings
	f.aliasNeedlesDirty = false
	return needles
}

func (f *Document) markAliasPageString(s string) {
	if s == "" || (len(f.aliasMap) == 0 && f.aliasNbPagesStr == "") {
		return
	}
	f.compiledAliasNeedles()
	for _, needle := range f.aliasNeedleStrings {
		if needle != "" && strings.Contains(s, needle) {
			f.markCurrentPageAliasCandidate()
			return
		}
	}
}

func (f *Document) markAliasPageBytes(data []byte) {
	if len(data) == 0 || (len(f.aliasMap) == 0 && f.aliasNbPagesStr == "") {
		return
	}
	for _, needle := range f.compiledAliasNeedles() {
		if len(needle) > 0 && bytes.Contains(data, needle) {
			f.markCurrentPageAliasCandidate()
			return
		}
	}
}

func (f *Document) markAliasPageConservative() {
	if len(f.aliasMap) == 0 && f.aliasNbPagesStr == "" {
		return
	}
	f.markCurrentPageAliasCandidate()
}

func (f *Document) markCurrentPageAliasCandidate() {
	if f.page <= 0 {
		return
	}
	for len(f.aliasPages) <= f.page {
		f.aliasPages = append(f.aliasPages, false)
	}
	f.aliasPages[f.page] = true
}

func (f *Document) markPagesContainingAlias(alias string) {
	if alias == "" || f.page <= 0 {
		return
	}
	needles := [][]byte{[]byte(alias), []byte(utf8toutf16(alias, false))}
	for n := 1; n <= f.page && n < len(f.pages); n++ {
		pageBytes := f.pages[n].Bytes()
		for _, needle := range needles {
			if len(needle) > 0 && bytes.Contains(pageBytes, needle) {
				for len(f.aliasPages) <= n {
					f.aliasPages = append(f.aliasPages, false)
				}
				f.aliasPages[n] = true
				break
			}
		}
	}
}

type aliasReplacement struct {
	alias       string
	replacement string
}

type aliasReplacementBytes struct {
	old []byte
	new []byte
}

type pageStreamData struct {
	page       int
	data       []byte
	compressed bool
	ready      bool
	err        error
}

type pageStreamCompressor struct {
	results   <-chan pageStreamData
	scheduled []bool
	pending   map[int]pageStreamData
}

func replaceAliasBytes(data []byte, pairs []aliasReplacementBytes) []byte {
	var out []byte
	for _, pair := range pairs {
		if len(pair.old) == 0 || !bytes.Contains(data, pair.old) {
			continue
		}
		if out == nil {
			out = append([]byte(nil), data...)
		}
		out = bytes.ReplaceAll(out, pair.old, pair.new)
		data = out
	}
	return out
}

func (f *Document) pageObjectNumber(page int) int {
	if page > 0 && page < len(f.pageObjectNumbers) {
		return f.pageObjectNumbers[page]
	}
	return 0
}

func (f *Document) pageHeightPt(page int) float64 {
	if pageSize, ok := f.pageSizes[page]; ok {
		return pageSize.Ht
	}
	if f.defOrientation == "P" {
		return f.defPageSize.Ht * f.k
	}
	return f.defPageSize.Wd * f.k
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
	pageHeights := make([]float64, nb+1)
	pageHeights[0] = hPt
	nextObj := f.n + 1
	for n := 1; n <= nb; n++ {
		pagesObjectNumbers[n] = nextObj
		pageHeights[n] = hPt
		if size, ok := f.pageSizes[n]; ok {
			pageHeights[n] = size.Ht
		}
		nextObj += 2 + len(f.pageLinks[n])
	}
	f.pageObjectNumbers = pagesObjectNumbers
	f.tagged.pageObjNums = ensureIntSliceLen(f.tagged.pageObjNums, nb+1)
	pageStreams := f.startPageStreamCompressor(nb)
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
			annots := make([]byte, 0, 32+len(f.pageLinks[n])*8+len(f.pageAttachments[n])*128)
			annots = append(annots, "/Annots ["...)
			linkObjNum := f.n + 2
			for i := range f.pageLinks[n] {
				f.pageLinks[n][i].objNum = linkObjNum
				if f.pageLinks[n][i].structElem != nil {
					f.pageLinks[n][i].structElem.ObjRef = linkObjNum
				}
				annots = appendPDFObjectRef(annots, linkObjNum)
				annots = append(annots, ' ')
				linkObjNum++
			}
			annots = f.appendAttachmentAnnotationLinks(annots, n)
			annots = append(annots, ']')
			f.outbytes(annots)
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
		data, compressed := f.pageStreamBytes(n, pageStreams)
		if compressed {
			f.outf("<</Filter /FlateDecode /Length %d>>", len(data))
			f.putstream(data)
		} else {
			f.outf("<</Length %d>>", f.pages[n].Len())
			f.putstream(f.pages[n].Bytes())
		}
		f.out("endobj")
		for _, pl := range f.pageLinks[n] {
			f.putLinkAnnotation(pl, pagesObjectNumbers, pageHeights)
		}
	}
	f.offsets[1] = f.buffer.Len()
	f.out("1 0 obj")
	f.out("<</Type /Pages")
	kids := make([]byte, 0, 16+nb*8)
	kids = append(kids, "/Kids ["...)
	for i := 1; i <= nb; i++ {
		kids = appendPDFObjectRef(kids, pagesObjectNumbers[i])
		kids = append(kids, ' ')
	}
	kids = append(kids, ']')
	f.outbytes(kids)
	f.outf("/Count %d", nb)
	f.outf("/MediaBox [0 0 %.2f %.2f]", wPt, hPt)
	f.out(">>")
	f.out("endobj")
}

func (f *Document) pageStreamBytes(page int, pageStreams *pageStreamCompressor) ([]byte, bool) {
	if pageStreams != nil {
		if stream, ok := pageStreams.page(page); ok {
			if stream.err != nil {
				f.SetError(stream.err)
				return nil, false
			}
			return stream.data, stream.compressed
		}
		if f.err != nil {
			return nil, false
		}
	}
	data, compressed := f.compressStreamBytes(f.pages[page].Bytes())
	if f.err != nil {
		return nil, false
	}
	return data, compressed
}

func (f *Document) startPageStreamCompressor(nb int) *pageStreamCompressor {
	if !f.compress || nb < 4 {
		return nil
	}
	level := f.compressLevel
	if !validCompressionLevel(level) {
		level = zlib.BestSpeed
	}
	scheduled := make([]bool, nb+1)
	scheduledCount := 0
	for page := 1; page <= nb; page++ {
		if f.pages[page].Len() >= tinyStreamCompressionThreshold {
			scheduled[page] = true
			scheduledCount++
		}
	}
	if scheduledCount == 0 {
		return nil
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	results := make(chan pageStreamData)
	go func() {
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for page := 1; page <= nb; page++ {
			if !scheduled[page] {
				continue
			}
			data := f.pages[page].Bytes()
			sem <- struct{}{}
			wg.Add(1)
			go func(page int, data []byte) {
				defer wg.Done()
				defer func() { <-sem }()
				compressed, err := sliceCompressLevel(data, level)
				results <- pageStreamData{
					page:       page,
					data:       compressed,
					compressed: err == nil,
					ready:      true,
					err:        err,
				}
			}(page, data)
		}
		wg.Wait()
		close(results)
	}()
	return &pageStreamCompressor{
		results:   results,
		scheduled: scheduled,
		pending:   make(map[int]pageStreamData),
	}
}

func (c *pageStreamCompressor) page(page int) (pageStreamData, bool) {
	if c == nil || page >= len(c.scheduled) || !c.scheduled[page] {
		return pageStreamData{}, false
	}
	if stream, ok := c.pending[page]; ok {
		delete(c.pending, page)
		return stream, true
	}
	for stream := range c.results {
		if stream.page == page {
			return stream, true
		}
		c.pending[stream.page] = stream
	}
	return pageStreamData{}, false
}

func (f *Document) putLinkAnnotation(pl pageLink, pagesObjectNumbers []int, pageHeights []float64) {
	f.newobj()
	f.outf("<< /Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /F 4", pl.x, pl.y, pl.x+pl.wd, pl.y-pl.ht)
	if pl.structParent >= 0 {
		f.outf("/StructParent %d", pl.structParent)
	}
	if pl.link == 0 {
		f.outf("/A << /S /URI /URI %s >>", f.textstring(pl.linkStr))
	} else {
		l := f.links[pl.link]
		h := pageHeights[0]
		pageObj := 0
		if l.page > 0 && l.page < len(pagesObjectNumbers) {
			pageObj = pagesObjectNumbers[l.page]
		}
		if l.page > 0 && l.page < len(pageHeights) {
			h = pageHeights[l.page]
		}
		f.outf("/Dest [%d 0 R /XYZ 0 %.2f null]", pageObj, h-l.y*f.k)
	}
	f.out(">>")
	f.out("endobj")
}
