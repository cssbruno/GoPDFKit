// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"io"

	"github.com/cssbruno/gopdfkit/importpdf"
)

const (
	maxPDFImportSourceBytes        = importpdf.MaxSourceBytes
	maxPDFImportDecodedStreamBytes = importpdf.MaxDecodedStreamBytes
	maxPDFImportArrayItems         = importpdf.MaxArrayItems
)

type importedPDFPage struct {
	id                 int
	page               *importpdf.PageRef
	objectID           int
	rewrittenBaseID    int
	rewrittenRefs      []importpdf.ObjRef
	rewrittenObjects   [][]byte
	rewrittenResources []byte
}

// ImportPage imports one page from a PDF file and returns its imported page ID.
//
// This built-in importer intentionally has a small dependency-free scope:
// classic xref-table PDFs, unencrypted documents, and pages whose content
// streams are unfiltered or FlateDecode-compressed. PDFs using xref streams or
// object streams are reported as unsupported.
func (f *Document) ImportPage(sourceFile string, pageNo int, box string) int {
	source, err := importpdf.OpenFile(sourceFile)
	if err != nil {
		f.SetError(err)
		return 0
	}
	return f.importPageFromSource(source, pageNo, box)
}

// ImportPageStream imports one page from a PDF stream and returns its imported
// page ID. The stream is read into memory, so callers may pass any io.Reader.
func (f *Document) ImportPageStream(source io.Reader, pageNo int, box string) int {
	sourcePDF, err := importpdf.OpenReader(source)
	if err != nil {
		f.SetError(err)
		return 0
	}
	return f.importPageFromSource(sourcePDF, pageNo, box)
}

// ImportPageSource imports one page from an already parsed PDF source.
func (f *Document) ImportPageSource(source *importpdf.Source, pageNo int, box string) int {
	if source == nil {
		f.SetErrorf("PDF import source is nil")
		return 0
	}
	return f.importPageFromSource(source, pageNo, box)
}

// ImportPagesFromSource imports every page from source and returns the imported
// page IDs. source may be a file path string, []byte, io.Reader, or
// *importpdf.Source.
func (f *Document) ImportPagesFromSource(source any, box string) []int {
	sourcePDF, err := importpdf.Open(source)
	if err != nil {
		f.SetError(err)
		return nil
	}
	ids := make([]int, 0, sourcePDF.PageCount())
	for pageNo := 1; pageNo <= sourcePDF.PageCount(); pageNo++ {
		page, err := sourcePDF.Page(pageNo, box)
		if err != nil {
			f.SetError(err)
			return ids
		}
		ids = append(ids, f.addImportedPDFPage(page))
	}
	return ids
}

// GetPageSizes returns the available page box sizes for a PDF source. Sizes are
// reported in PDF points. source may be a file path string, []byte, or
// io.Reader.
func GetPageSizes(source any) (map[int]map[string]Size, error) {
	sizes, err := importpdf.GetPageSizes(source)
	if err != nil {
		return nil, err
	}
	return documentPageSizes(sizes), nil
}

// GetPageSizes returns the available page box sizes for a PDF source and stores
// any import error on the document. Sizes are reported in PDF points.
func (f *Document) GetPageSizes(source any) map[int]map[string]Size {
	sizes, err := GetPageSizes(source)
	if err != nil {
		f.SetError(err)
		return nil
	}
	return sizes
}

func (f *Document) importPageFromSource(source *importpdf.Source, pageNo int, box string) int {
	page, err := source.Page(pageNo, box)
	if err != nil {
		f.SetError(err)
		return 0
	}
	return f.addImportedPDFPage(page)
}

func (f *Document) addImportedPDFPage(page *importpdf.PageRef) int {
	f.importedPageSeq++
	f.importedPages[f.importedPageSeq] = &importedPDFPage{
		id:   f.importedPageSeq,
		page: page,
	}
	return f.importedPageSeq
}

func (page *importedPDFPage) rewrittenImportData(baseID int, objects []importpdf.Object, refMap map[importpdf.ObjRef]int) ([][]byte, []byte) {
	if page.rewrittenImportDataMatches(baseID, objects) {
		return page.rewrittenObjects, page.rewrittenResources
	}
	rewrittenObjects := make([][]byte, len(objects))
	rewrittenRefs := make([]importpdf.ObjRef, len(objects))
	for i, object := range objects {
		rewrittenObjects[i] = importpdf.RewriteIndirectRefs(object.Body, refMap)
		rewrittenRefs[i] = object.Ref
	}
	page.rewrittenBaseID = baseID
	page.rewrittenRefs = rewrittenRefs
	page.rewrittenObjects = rewrittenObjects
	page.rewrittenResources = importpdf.RewriteIndirectRefs(page.page.Resources(), refMap)
	return page.rewrittenObjects, page.rewrittenResources
}

func (page *importedPDFPage) rewrittenImportDataMatches(baseID int, objects []importpdf.Object) bool {
	if page.rewrittenBaseID != baseID || len(page.rewrittenObjects) != len(objects) || len(page.rewrittenRefs) != len(objects) || page.rewrittenResources == nil {
		return false
	}
	for i, object := range objects {
		if page.rewrittenRefs[i] != object.Ref {
			return false
		}
	}
	return true
}

// UseImportedPage draws a page imported by ImportPage, ImportPageStream, or
// ImportPagesFromSource. Coordinates and dimensions use the document unit.
// Passing zero for one dimension preserves the imported page aspect ratio;
// passing zero for both draws the page at its native size.
func (f *Document) UseImportedPage(pageID int, x, y, w, h float64) {
	if f.err != nil {
		return
	}
	if f.page <= 0 {
		f.SetErrorf("cannot use an imported page without first adding a page")
		return
	}
	page, ok := f.importedPages[pageID]
	if !ok || page == nil || page.page == nil {
		f.SetErrorf("unknown imported page ID: %d", pageID)
		return
	}
	nativeW := page.page.WidthPoints() / f.k
	nativeH := page.page.HeightPoints() / f.k
	switch {
	case w == 0 && h == 0:
		w = nativeW
		h = nativeH
	case w == 0:
		w = h * nativeW / nativeH
	case h == 0:
		h = w * nativeH / nativeW
	}
	if !finiteNumbers(x, y, w, h) || w <= 0 || h <= 0 {
		f.SetErrorf("invalid imported page placement: %.4f %.4f %.4f %.4f", x, y, w, h)
		return
	}
	content := make([]byte, 0, 64)
	content = append(content, "q "...)
	content = appendPDFNumberSpace(content, w*f.k, 5)
	content = append(content, "0 0 "...)
	content = appendPDFNumberSpace(content, h*f.k, 5)
	content = appendPDFNumberSpace(content, x*f.k, 5)
	content = appendPDFNumberSpace(content, (f.h-(y+h))*f.k, 5)
	content = append(content, "cm /IPG"...)
	content = appendPDFInt(content, pageID)
	content = append(content, " Do Q"...)
	f.outbytes(f.wrapTaggedContent(content, taggedContentOptions{Artifact: true}))
}

func documentPageSizes(sizes map[int]map[string]importpdf.Size) map[int]map[string]Size {
	out := make(map[int]map[string]Size, len(sizes))
	for pageNo, pageSizes := range sizes {
		converted := make(map[string]Size, len(pageSizes))
		for name, size := range pageSizes {
			converted[name] = Size{Wd: size.Wd, Ht: size.Ht}
		}
		out[pageNo] = converted
	}
	return out
}
