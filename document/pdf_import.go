// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cssbruno/gopdfkit/importpdf"
)

const (
	maxPDFImportSourceBytes        = importpdf.MaxSourceBytes
	maxPDFImportDecodedStreamBytes = importpdf.MaxDecodedStreamBytes
	maxPDFImportArrayItems         = importpdf.MaxArrayItems
	maxPDFImportReferencedObjects  = importpdf.MaxReferencedObjects
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
	if err := f.requireSecurityFeature("PDF import", f.securityPolicy.AllowPDFImport); err != nil {
		return 0, err
	}
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
	return f.ImportPageStreamContext(context.Background(), source, pageNo, box)
}

// ImportPageStreamContext imports one page from a PDF stream and checks ctx
// before and during bounded source reads.
func (f *Document) ImportPageStreamContext(ctx context.Context, source io.Reader, pageNo int, box string) (int, error) {
	if err := f.requireSecurityFeature("PDF import", f.securityPolicy.AllowPDFImport); err != nil {
		return 0, err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return 0, err
	}
	sourcePDF, err := f.openImportReaderContext(ctx, source)
	if err != nil {
		f.SetError(err)
		return 0, err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return 0, err
	}
	return f.importPageFromSourceErrorContext(ctx, sourcePDF, pageNo, box)
}

// ImportPageSource imports one page from an already parsed PDF source.
func (f *Document) ImportPageSource(source *importpdf.Source, pageNo int, box string) int {
	if err := f.requireSecurityFeature("PDF import", f.securityPolicy.AllowPDFImport); err != nil {
		return 0
	}
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
	ids, _ := f.ImportPagesFromSourceContext(context.Background(), source, box)
	return ids
}

// ImportPagesFromSourceContext imports every page from source and checks ctx
// before parsing and between pages.
func (f *Document) ImportPagesFromSourceContext(ctx context.Context, source any, box string) ([]int, error) {
	if err := f.requireSecurityFeature("PDF import", f.securityPolicy.AllowPDFImport); err != nil {
		return nil, err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return nil, err
	}
	sourcePDF, err := f.openImportSourceContext(ctx, source)
	if err != nil {
		f.SetError(err)
		return nil, err
	}
	ids := make([]int, 0, sourcePDF.PageCount())
	for pageNo := 1; pageNo <= sourcePDF.PageCount(); pageNo++ {
		if err := outputCanceledError(ctx); err != nil {
			f.SetError(err)
			return ids, err
		}
		page, err := sourcePDF.PageContext(ctx, pageNo, box)
		if err != nil {
			f.SetError(err)
			return ids, err
		}
		if err := f.validateImportedPageLimits(page); err != nil {
			return ids, err
		}
		ids = append(ids, f.addImportedPDFPage(page))
	}
	return ids, nil
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
	if _, ok := source.(string); ok && f.resourceLoader != nil {
		sourcePDF, err := f.openImportSourceContext(context.Background(), source)
		if err != nil {
			f.SetError(err)
			return nil
		}
		return documentPageSizes(sourcePDF.PageSizes())
	}
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
	return f.importPageFromSourceErrorContext(context.Background(), source, pageNo, box)
}

func (f *Document) importPageFromSourceErrorContext(ctx context.Context, source *importpdf.Source, pageNo int, box string) (int, error) {
	if err := f.requireSecurityFeature("PDF import", f.securityPolicy.AllowPDFImport); err != nil {
		return 0, err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return 0, err
	}
	page, err := source.PageContext(ctx, pageNo, box)
	if err != nil {
		err = importContextOutputError(ctx, err)
		f.SetError(err)
		return 0, err
	}
	if err := f.validateImportedPageLimits(page); err != nil {
		return 0, err
	}
	return f.addImportedPDFPage(page), nil
}

func (f *Document) openImportSource(source any) (*importpdf.Source, error) {
	return f.openImportSourceContext(context.Background(), source)
}

func (f *Document) openImportSourceContext(ctx context.Context, source any) (*importpdf.Source, error) {
	switch src := source.(type) {
	case *importpdf.Source:
		if src == nil {
			return nil, fmt.Errorf("%w: source is nil", ErrUnsupportedPDFImport)
		}
		return src, nil
	case string:
		return f.openImportFileContext(ctx, src)
	case []byte:
		if err := f.checkImportedPDFBytes(int64(len(src))); err != nil {
			return nil, err
		}
		source, err := importpdf.OpenBytesWithOptionsContext(ctx, src, f.importOptions())
		if err != nil {
			return nil, importContextOutputError(ctx, err)
		}
		return source, nil
	case io.Reader:
		return f.openImportReaderContext(ctx, src)
	default:
		return nil, fmt.Errorf("%w: unsupported source type %T", ErrUnsupportedPDFImport, source)
	}
}

func (f *Document) openImportFile(path string) (*importpdf.Source, error) {
	return f.openImportFileContext(context.Background(), path)
}

func (f *Document) openImportFileContext(ctx context.Context, path string) (*importpdf.Source, error) {
	if f.resourceLoader != nil {
		reader, info, err := f.resourceLoader.OpenResource(ctx, ResourcePDFImport, path)
		if err != nil {
			return nil, err
		}
		if reader == nil {
			return nil, fmt.Errorf("resource loader returned nil reader")
		}
		defer func() { _ = reader.Close() }()
		if err := f.checkImportedPDFBytes(info.Size); err != nil {
			return nil, err
		}
		return f.openImportReaderContext(ctx, reader)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		if err := f.checkImportedPDFBytes(info.Size()); err != nil {
			return nil, err
		}
	}
	source, err := importpdf.OpenFileWithOptionsContext(ctx, path, f.importOptions())
	if err != nil {
		return nil, importContextOutputError(ctx, err)
	}
	return source, nil
}

func (f *Document) openImportReader(source io.Reader) (*importpdf.Source, error) {
	return f.openImportReaderContext(context.Background(), source)
}

func (f *Document) openImportReaderContext(ctx context.Context, source io.Reader) (*importpdf.Source, error) {
	if source == nil {
		return nil, fmt.Errorf("%w: source is nil", ErrUnsupportedPDFImport)
	}
	limit := f.importedPDFByteLimit()
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, r: source}, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%w: PDF import source exceeds maximum size", ErrUnsupportedPDFImport)
	}
	if err != nil {
		return nil, err
	}
	sourcePDF, err := importpdf.OpenBytesImmutableWithOptionsContext(ctx, data, f.importOptions())
	if err != nil {
		return nil, importContextOutputError(ctx, err)
	}
	return sourcePDF, nil
}

func importContextOutputError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := outputCanceledError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := outputCanceledError(r.ctx); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		return n, outputCanceledError(r.ctx)
	}
	return n, nil
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

func (f *Document) importOptions() importpdf.ImportOptions {
	return importpdf.ImportOptions{
		MaxSourceBytes:       f.importedPDFByteLimit(),
		MaxReferencedObjects: f.importedPDFReferencedObjectLimit(),
	}
}

func (f *Document) importedPDFReferencedObjectLimit() int {
	if f.limits.MaxReferencedObjects > 0 {
		return f.limits.MaxReferencedObjects
	}
	return importpdf.MaxReferencedObjects
}

func (f *Document) validateImportedPageLimits(page *importpdf.PageRef) error {
	if page == nil {
		return fmt.Errorf("%w: page is nil", ErrUnsupportedPDFImport)
	}
	limit := f.importedPDFReferencedObjectLimit()
	if limit > 0 && len(page.ObjectRefs()) > limit {
		err := fmt.Errorf("%w: referenced object limit exceeded: %d > %d", ErrUnsupportedPDFImport, len(page.ObjectRefs()), limit)
		f.SetError(err)
		return err
	}
	return nil
}

func (f *Document) addImportedPDFPage(page *importpdf.PageRef) int {
	f.importedPageSeq++
	f.ensureResourceStore().addImportedPage(f.importedPageSeq, &importedPDFPage{
		id:   f.importedPageSeq,
		page: page,
	})
	return f.importedPageSeq
}

func (page *importedPDFPage) rewrittenImportData(baseID int, refs []importpdf.ObjRef, refMap map[importpdf.ObjRef]int) ([][]byte, []byte) {
	if page.rewrittenImportDataMatches(baseID, refs) {
		return page.rewrittenObjects, page.rewrittenResources
	}
	rewrittenObjects := make([][]byte, 0, len(refs))
	rewrittenRefs := make([]importpdf.ObjRef, 0, len(refs))
	err := page.page.ForEachObjectBorrowed(func(ref importpdf.ObjRef, body []byte) error {
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
	page, ok := f.ensureResourceStore().importedPage(pageID)
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
	content = append(content, "cm "...)
	content = append(content, importedPagePDFResourceName(pageID).String()...)
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
