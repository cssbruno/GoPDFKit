// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"context"
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
	mapAliases := make([]string, 0, len(f.aliasMap))
	for alias := range f.aliasMap {
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		mapAliases = append(mapAliases, alias)
	}
	sort.Strings(mapAliases)
	aliases = append(aliases, mapAliases...)
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
	cancel    context.CancelFunc
	done      <-chan struct{}
	stopOnce  sync.Once
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
	f.putpagesContext(context.Background())
}

func (f *Document) putpagesContext(ctx context.Context) {
	var wPt, hPt float64
	var pageSize Size
	var ok bool
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return
	}
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
	pageStreams := f.startPageStreamCompressorContext(ctx, nb)
	if pageStreams != nil {
		defer pageStreams.stop()
	}
	for n := 1; n <= nb; n++ {
		if err := outputCanceledError(ctx); err != nil {
			f.SetError(err)
			return
		}
		f.newPDFDictObject()
		f.tagged.pageObjNums[n] = f.n
		f.out("/Type /Page")
		f.out("/Parent 1 0 R")
		pageSize, ok = f.pageSizes[n]
		if ok {
			f.outf("/MediaBox [0 0 %.2f %.2f]", pageSize.Wd, pageSize.Ht)
		}
		if rotation := f.pageRotations[n]; rotation != 0 {
			f.outf("/Rotate %d", rotation)
		}
		pageBoxKeys := pageBoxOutputKeys(f.pageBoxes[n], f.catalogSort)
		for _, t := range pageBoxKeys {
			pb := f.pageBoxes[n][t]
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
		f.outf("/Contents %d 0 R", f.n+1)
		f.endPDFDict()
		f.endPDFObject()
		f.newPDFDictObject()
		data, compressed := f.pageStreamBytesContext(ctx, n, pageStreams)
		if f.err != nil {
			return
		}
		if compressed {
			f.out("/Filter /FlateDecode")
			f.outf("/Length %d", len(data))
		} else {
			f.outf("/Length %d", f.pages[n].Len())
		}
		f.endPDFDict()
		if compressed {
			f.putstream(data)
		} else {
			f.putstream(f.pages[n].Bytes())
		}
		f.endPDFObject()
		for _, pl := range f.pageLinks[n] {
			f.putLinkAnnotation(pl, pagesObjectNumbers, pageHeights)
		}
	}
	f.beginPDFObject(1)
	f.beginPDFDict()
	f.out("/Type /Pages")
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
	f.endPDFDict()
	f.endPDFObject()
}

func (f *Document) pageStreamBytes(page int, pageStreams *pageStreamCompressor) ([]byte, bool) {
	return f.pageStreamBytesContext(context.Background(), page, pageStreams)
}

func (f *Document) pageStreamBytesContext(ctx context.Context, page int, pageStreams *pageStreamCompressor) ([]byte, bool) {
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return nil, false
	}
	if pageStreams != nil {
		if stream, ok := pageStreams.pageContext(ctx, page); ok {
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
	if compressed && f.hooks.OnPageCompressed != nil {
		f.hooks.OnPageCompressed(page, f.pages[page].Len(), len(data))
	}
	return data, compressed
}

func (f *Document) startPageStreamCompressor(nb int) *pageStreamCompressor {
	return f.startPageStreamCompressorContext(context.Background(), nb)
}

func (f *Document) startPageStreamCompressorContext(ctx context.Context, nb int) *pageStreamCompressor {
	if ctx == nil {
		ctx = context.Background()
	}
	if !f.compress || nb < 4 {
		return nil
	}
	if f.pageCompressionWorkers == 0 {
		return nil
	}
	level := f.compressLevel
	if !validCompressionLevel(level) {
		level = zlib.BestSpeed
	}
	scheduled := make([]bool, nb+1)
	scheduledCount := 0
	threshold := f.compressionTinyStreamThreshold
	if threshold <= 0 {
		threshold = defaultTinyStreamCompressionThreshold
	}
	for page := 1; page <= nb; page++ {
		if f.pages[page].Len() >= threshold {
			scheduled[page] = true
			scheduledCount++
		}
	}
	if scheduledCount == 0 {
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	workers := f.pageCompressionWorkers
	if workers < 1 {
		workers = 1
	}
	results := make(chan pageStreamData)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(results)
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
	schedule:
		for page := 1; page <= nb; page++ {
			if !scheduled[page] {
				continue
			}
			if workerCtx.Err() != nil {
				break schedule
			}
			data := f.pages[page].Bytes()
			select {
			case sem <- struct{}{}:
			case <-workerCtx.Done():
				break schedule
			}
			wg.Add(1)
			go func(page int, data []byte) {
				defer wg.Done()
				defer func() { <-sem }()
				compressed, err := sliceCompressLevel(data, level)
				if err == nil && f.hooks.OnPageCompressed != nil {
					f.hooks.OnPageCompressed(page, len(data), len(compressed))
				}
				result := pageStreamData{
					page:       page,
					data:       compressed,
					compressed: err == nil,
					ready:      true,
					err:        err,
				}
				select {
				case results <- result:
				case <-workerCtx.Done():
				}
			}(page, data)
		}
		wg.Wait()
	}()
	return &pageStreamCompressor{
		results:   results,
		scheduled: scheduled,
		pending:   make(map[int]pageStreamData),
		cancel:    cancel,
		done:      done,
	}
}

func pageBoxOutputKeys(pageBoxes map[string]PageBox, sorted bool) []string {
	keys := make([]string, 0, len(pageBoxes))
	for key := range pageBoxes {
		keys = append(keys, key)
	}
	if sorted {
		sort.Strings(keys)
	}
	return keys
}

func (c *pageStreamCompressor) page(page int) (pageStreamData, bool) {
	return c.pageContext(context.Background(), page)
}

func (c *pageStreamCompressor) stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		c.cancel()
		<-c.done
	})
}

func (c *pageStreamCompressor) pageContext(ctx context.Context, page int) (pageStreamData, bool) {
	if c == nil || page >= len(c.scheduled) || !c.scheduled[page] {
		return pageStreamData{}, false
	}
	if err := outputCanceledError(ctx); err != nil {
		return pageStreamData{page: page, ready: true, err: err}, true
	}
	if stream, ok := c.pending[page]; ok {
		delete(c.pending, page)
		return stream, true
	}
	for {
		select {
		case stream, ok := <-c.results:
			if !ok {
				return pageStreamData{}, false
			}
			if stream.page == page {
				return stream, true
			}
			c.pending[stream.page] = stream
		case <-ctx.Done():
			return pageStreamData{page: page, ready: true, err: outputCanceledError(ctx)}, true
		}
	}
}

func (f *Document) putLinkAnnotation(pl pageLink, pagesObjectNumbers []int, pageHeights []float64) {
	f.newPDFDictObject()
	f.outf("/Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /F 4", pl.x, pl.y, pl.x+pl.wd, pl.y-pl.ht)
	if pl.structParent >= 0 {
		f.outf("/StructParent %d", pl.structParent)
	}
	if pl.link == 0 {
		f.out("/A")
		f.beginPDFDict()
		f.out("/S /URI")
		f.outf("/URI %s", f.textstring(pl.linkStr))
		f.endPDFDict()
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
	f.endPDFDict()
	f.endPDFObject()
}
