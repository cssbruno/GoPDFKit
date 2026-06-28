// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"io"
	"os"

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
	id, _ := f.ImportPageError(sourceFile, pageNo, box)
	return id
}

// ImportPageError imports one page from a PDF file and returns its imported
// page ID or an error.
func (f *Document) ImportPageError(sourceFile string, pageNo int, box string) (int, error) {
	source, err := f.openImportFile(sourceFile)
	if err != nil {
		f.SetError(err)
		return 0, err
	}
	return f.importPageFromSourceError(source, pageNo, box)
}

// ImportPageStream imports one page from a PDF stream and returns its imported
// page ID. The stream is read into memory, so callers may pass any io.Reader.
func (f *Document) ImportPageStream(source io.Reader, pageNo int, box string) int {
	id, _ := f.ImportPageStreamError(source, pageNo, box)
	return id
}

// ImportPageStreamError imports one page from a PDF stream and returns its
// imported page ID or an error.
func (f *Document) ImportPageStreamError(source io.Reader, pageNo int, box string) (int, error) {
	sourcePDF, err := f.openImportReader(source)
	if err != nil {
		f.SetError(err)
		return 0, err
	}
	return f.importPageFromSourceError(sourcePDF, pageNo, box)
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
	sourcePDF, err := f.openImportSource(source)
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
		if err := f.validateImportedPageLimits(page); err != nil {
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
	id, _ := f.importPageFromSourceError(source, pageNo, box)
	return id
}

func (f *Document) importPageFromSourceError(source *importpdf.Source, pageNo int, box string) (int, error) {
	page, err := source.Page(pageNo, box)
	if err != nil {
		f.SetError(err)
		return 0, err
	}
	if err := f.validateImportedPageLimits(page); err != nil {
		return 0, err
	}
	return f.addImportedPDFPage(page), nil
}

func (f *Document) openImportSource(source any) (*importpdf.Source, error) {
	switch src := source.(type) {
	case *importpdf.Source:
		if src == nil {
			return nil, fmt.Errorf("%w: source is nil", ErrUnsupportedPDFImport)
		}
		return src, nil
	case string:
		return f.openImportFile(src)
	case []byte:
		if err := f.checkImportedPDFBytes(int64(len(src))); err != nil {
			return nil, err
		}
		return importpdf.OpenBytes(src)
	case io.Reader:
		return f.openImportReader(src)
	default:
		return nil, fmt.Errorf("%w: unsupported source type %T", ErrUnsupportedPDFImport, source)
	}
}

func (f *Document) openImportFile(path string) (*importpdf.Source, error) {
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		if err := f.checkImportedPDFBytes(info.Size()); err != nil {
			return nil, err
		}
	}
	source, err := importpdf.OpenFile(path)
	if err != nil {
		return nil, err
	}
	return source, nil
}

func (f *Document) openImportReader(source io.Reader) (*importpdf.Source, error) {
	if source == nil {
		return nil, fmt.Errorf("%w: source is nil", ErrUnsupportedPDFImport)
	}
	limit := f.importedPDFByteLimit()
	data, err := io.ReadAll(io.LimitReader(source, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%w: PDF import source exceeds maximum size", ErrUnsupportedPDFImport)
	}
	if err != nil {
		return nil, err
	}
	return importpdf.OpenBytesImmutable(data)
}

func (f *Document) checkImportedPDFBytes(size int64) error {
	limit := f.importedPDFByteLimit()
	if limit >= 0 && size > limit {
		return fmt.Errorf("%w: PDF import source exceeds maximum size", ErrUnsupportedPDFImport)
	}
	return nil
}

func (f *Document) importedPDFByteLimit() int64 {
	if f.limits.MaxImportedPDFBytes > 0 {
		return f.limits.MaxImportedPDFBytes
	}
	return importpdf.MaxSourceBytes
}

func (f *Document) validateImportedPageLimits(page *importpdf.PageRef) error {
	if page == nil {
		return fmt.Errorf("%w: page is nil", ErrUnsupportedPDFImport)
	}
	if f.limits.MaxReferencedObjects > 0 && len(page.ObjectRefs()) > f.limits.MaxReferencedObjects {
		err := fmt.Errorf("%w: referenced object limit exceeded: %d > %d", ErrUnsupportedPDFImport, len(page.ObjectRefs()), f.limits.MaxReferencedObjects)
		f.SetError(err)
		return err
	}
	return nil
}

func (f *Document) addImportedPDFPage(page *importpdf.PageRef) int {
	f.importedPageSeq++
	f.importedPages[f.importedPageSeq] = &importedPDFPage{
		id:   f.importedPageSeq,
		page: page,
	}
	return f.importedPageSeq
}

func (page *importedPDFPage) rewrittenImportData(baseID int, refs []importpdf.ObjRef, refMap map[importpdf.ObjRef]int) ([][]byte, []byte) {
	if page.rewrittenImportDataMatches(baseID, refs) {
		return page.rewrittenObjects, page.rewrittenResources
	}
	rewrittenObjects := make([][]byte, 0, len(refs))
	rewrittenRefs := make([]importpdf.ObjRef, 0, len(refs))
	err := page.page.ForEachObject(func(ref importpdf.ObjRef, body []byte) error {
		rewrittenObjects = append(rewrittenObjects, importpdf.RewriteIndirectRefs(body, refMap))
		rewrittenRefs = append(rewrittenRefs, ref)
		return nil
	})
	if err != nil {
		return nil, nil
	}
	page.rewrittenBaseID = baseID
	page.rewrittenRefs = rewrittenRefs
	page.rewrittenObjects = rewrittenObjects
	page.rewrittenResources = importpdf.RewriteIndirectRefs(page.page.Resources(), refMap)
	return page.rewrittenObjects, page.rewrittenResources
}

func (page *importedPDFPage) rewrittenImportDataMatches(baseID int, refs []importpdf.ObjRef) bool {
	if page.rewrittenBaseID != baseID || len(page.rewrittenObjects) != len(refs) || len(page.rewrittenRefs) != len(refs) || page.rewrittenResources == nil {
		return false
	}
	for i, ref := range refs {
		if page.rewrittenRefs[i] != ref {
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
	_ = f.UseImportedPageError(pageID, x, y, w, h)
}

// UseImportedPageError draws a page imported by ImportPage, ImportPageStream,
// or ImportPagesFromSource and returns any validation error directly.
func (f *Document) UseImportedPageError(pageID int, x, y, w, h float64) error {
	if f.err != nil {
		return f.err
	}
	if f.page <= 0 {
		f.SetErrorf("cannot use an imported page without first adding a page")
		return f.err
	}
	page, ok := f.importedPages[pageID]
	if !ok || page == nil || page.page == nil {
		f.SetErrorf("unknown imported page ID: %d", pageID)
		return f.err
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
		return f.err
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
	f.outTaggedContent(content, taggedContentOptions{Artifact: true})
	return f.err
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
