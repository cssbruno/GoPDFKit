// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
)

type importedPDFPage struct {
	id        int
	widthPt   float64
	heightPt  float64
	resources []byte
	content   []byte
	objects   map[pdfObjRef][]byte
	objectID  int
}

// ImportPage imports one page from a PDF file and returns its imported page ID.
//
// This built-in importer intentionally has a small dependency-free scope:
// classic xref-table PDFs, unencrypted documents, and pages whose content
// streams are unfiltered or FlateDecode-compressed. PDFs using xref streams or
// object streams are reported as unsupported.
func (f *Fpdf) ImportPage(sourceFile string, pageNo int, box string) int {
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		f.SetError(err)
		return 0
	}
	return f.importPageFromBytes(data, pageNo, box)
}

// ImportPageStream imports one page from a PDF stream and returns its imported
// page ID. The stream is read into memory, so callers may pass any io.Reader.
func (f *Fpdf) ImportPageStream(source io.Reader, pageNo int, box string) int {
	data, err := io.ReadAll(source)
	if err != nil {
		f.SetError(err)
		return 0
	}
	return f.importPageFromBytes(data, pageNo, box)
}

// ImportPagesFromSource imports every page from source and returns the imported
// page IDs. source may be a file path string, []byte, or io.Reader.
func (f *Fpdf) ImportPagesFromSource(source any, box string) []int {
	data, err := pdfImportSourceBytes(source)
	if err != nil {
		f.SetError(err)
		return nil
	}
	doc, err := parsePDFImportDocument(data)
	if err != nil {
		f.SetError(err)
		return nil
	}
	ids := make([]int, 0, len(doc.pages))
	for pageNo := 1; pageNo <= len(doc.pages); pageNo++ {
		page, err := doc.importPage(pageNo, box)
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
	data, err := pdfImportSourceBytes(source)
	if err != nil {
		return nil, err
	}
	doc, err := parsePDFImportDocument(data)
	if err != nil {
		return nil, err
	}
	return doc.pageSizes(), nil
}

// GetPageSizes returns the available page box sizes for a PDF source and stores
// any import error on the document. Sizes are reported in PDF points.
func (f *Fpdf) GetPageSizes(source any) map[int]map[string]Size {
	sizes, err := GetPageSizes(source)
	if err != nil {
		f.SetError(err)
		return nil
	}
	return sizes
}

func (f *Fpdf) importPageFromBytes(data []byte, pageNo int, box string) int {
	doc, err := parsePDFImportDocument(data)
	if err != nil {
		f.SetError(err)
		return 0
	}
	page, err := doc.importPage(pageNo, box)
	if err != nil {
		f.SetError(err)
		return 0
	}
	return f.addImportedPDFPage(page)
}

func (f *Fpdf) addImportedPDFPage(page *importedPDFPage) int {
	f.importedPageSeq++
	page.id = f.importedPageSeq
	f.importedPages[page.id] = page
	return page.id
}

// UseImportedPage draws a page imported by ImportPage, ImportPageStream, or
// ImportPagesFromSource. Coordinates and dimensions use the document unit.
// Passing zero for one dimension preserves the imported page aspect ratio;
// passing zero for both draws the page at its native size.
func (f *Fpdf) UseImportedPage(pageID int, x, y, w, h float64) {
	page, ok := f.importedPages[pageID]
	if !ok || page == nil {
		f.SetErrorf("unknown imported page ID: %d", pageID)
		return
	}
	nativeW := page.widthPt / f.k
	nativeH := page.heightPt / f.k
	if w == 0 && h == 0 {
		w = nativeW
		h = nativeH
	} else if w == 0 {
		w = h * nativeW / nativeH
	} else if h == 0 {
		h = w * nativeH / nativeW
	}
	if !finiteNumbers(x, y, w, h) || w <= 0 || h <= 0 {
		f.SetErrorf("invalid imported page placement: %.4f %.4f %.4f %.4f", x, y, w, h)
		return
	}
	f.outf("q %.5f 0 0 %.5f %.5f %.5f cm /IPG%d Do Q", w*f.k, h*f.k, x*f.k, (f.h-(y+h))*f.k, pageID)
}

func pdfImportSourceBytes(source any) ([]byte, error) {
	switch src := source.(type) {
	case string:
		return os.ReadFile(src)
	case []byte:
		return append([]byte(nil), src...), nil
	case io.Reader:
		return io.ReadAll(src)
	default:
		return nil, fmt.Errorf("unsupported PDF import source type %T", source)
	}
}

func sortedPDFObjRefs(objects map[pdfObjRef][]byte) []pdfObjRef {
	refs := make([]pdfObjRef, 0, len(objects))
	for ref := range objects {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].num == refs[j].num {
			return refs[i].gen < refs[j].gen
		}
		return refs[i].num < refs[j].num
	})
	return refs
}

func rewritePDFIndirectRefs(data []byte, refMap map[pdfObjRef]int) []byte {
	if data == nil {
		return []byte{}
	}
	streamPos := bytes.Index(data, []byte("stream"))
	if streamPos < 0 {
		return rewritePDFIndirectRefsInSection(data, refMap)
	}
	out := rewritePDFIndirectRefsInSection(data[:streamPos], refMap)
	out = append(out, data[streamPos:]...)
	return out
}

func rewritePDFIndirectRefsInSection(data []byte, refMap map[pdfObjRef]int) []byte {
	if data == nil {
		return []byte{}
	}
	refs := findPDFIndirectRefs(data)
	if len(refs) == 0 {
		return append([]byte(nil), data...)
	}
	var out bytes.Buffer
	last := 0
	for _, found := range refs {
		newObj, ok := refMap[found.ref]
		if !ok {
			continue
		}
		out.Write(data[last:found.start])
		fmt.Fprintf(&out, "%d 0 R", newObj)
		last = found.end
	}
	out.Write(data[last:])
	return out.Bytes()
}
