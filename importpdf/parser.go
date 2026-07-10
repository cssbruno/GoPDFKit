// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type ObjRef struct {
	num int
	gen int
}

// ObjectNumber returns the referenced PDF indirect object number.
func (r ObjRef) ObjectNumber() int {
	return r.num
}

// Generation returns the referenced PDF indirect object generation number.
func (r ObjRef) Generation() int {
	return r.gen
}

// String formats the reference as "object generation".
func (r ObjRef) String() string {
	return fmt.Sprintf("%d %d", r.num, r.gen)
}

// Size reports a PDF page box width and height in points.
type Size struct {
	Wd float64
	Ht float64
}

// Object is an indirect object referenced by an imported page.
type Object struct {
	Ref  ObjRef
	Body []byte
}

// PageRef is a parsed PDF page ready to be embedded by a renderer.
type PageRef struct {
	widthPt              float64
	heightPt             float64
	resources            []byte
	content              []byte
	contentErr           error
	contentOnce          sync.Once
	source               *Source
	sourcePage           sourcePage
	box                  pdfBox
	encodedContent       []byte
	encodedContentFilter string
	objects              map[ObjRef][]byte
	objectRefs           []ObjRef
}

// WidthPoints returns the imported page width in PDF points.
func (p *PageRef) WidthPoints() float64 {
	if p == nil {
		return 0
	}
	return p.widthPt
}

// HeightPoints returns the imported page height in PDF points.
func (p *PageRef) HeightPoints() float64 {
	if p == nil {
		return 0
	}
	return p.heightPt
}

// Resources returns a copy of the imported page resource dictionary bytes.
func (p *PageRef) Resources() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.resources...)
}

// Content returns a copy of the imported page content stream bytes. Use
// ContentErr to check whether lazy content loading failed.
func (p *PageRef) Content() []byte {
	if p == nil {
		return nil
	}
	p.ensureContent()
	return append([]byte(nil), p.content...)
}

// ContentErr reports the lazy content-loading error, if any.
func (p *PageRef) ContentErr() error {
	if p == nil {
		return nil
	}
	p.ensureContent()
	return p.contentErr
}

// ContentWithError returns a copy of the imported page content stream bytes and
// reports lazy content-loading errors directly.
func (p *PageRef) ContentWithError() ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	p.ensureContent()
	if p.contentErr != nil {
		return nil, p.contentErr
	}
	return append([]byte(nil), p.content...), nil
}

// ContentWithContext returns a copy of the imported page content stream bytes,
// checking ctx while lazy content is loaded.
func (p *PageRef) ContentWithContext(ctx context.Context) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	p.ensureContentContext(ctx)
	if p.contentErr != nil {
		return nil, p.contentErr
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return append([]byte(nil), p.content...), nil
}

func (p *PageRef) ensureContent() {
	p.ensureContentContext(context.Background())
}

func (p *PageRef) ensureContentContext(ctx context.Context) {
	if p == nil || len(p.content) > 0 || p.source == nil {
		return
	}
	if err := importContextErr(ctx); err != nil {
		return
	}
	p.contentOnce.Do(func() {
		if err := importContextErr(ctx); err != nil {
			p.contentErr = err
			return
		}
		p.source.mu.Lock()
		defer p.source.mu.Unlock()
		content, err := p.source.pageContentContext(ctx, p.sourcePage)
		if err != nil {
			p.contentErr = err
			return
		}
		p.content = wrapImportedPageContent(content, p.box)
	})
}

// EncodedContent returns a preserved encoded content stream and its PDF filter
// name when the imported page can be embedded without re-encoding.
func (p *PageRef) EncodedContent() ([]byte, string, bool) {
	if p == nil || len(p.encodedContent) == 0 || p.encodedContentFilter == "" {
		return nil, "", false
	}
	return append([]byte(nil), p.encodedContent...), p.encodedContentFilter, true
}

// Objects returns the indirect objects referenced by the imported page, sorted
// by object number and generation.
func (p *PageRef) Objects() []Object {
	if p == nil {
		return nil
	}
	refs := p.objectRefs
	if refs == nil {
		refs = sortedObjRefs(p.objects)
	}
	objects := make([]Object, 0, len(refs))
	for _, ref := range refs {
		objects = append(objects, Object{Ref: ref, Body: append([]byte(nil), p.objects[ref]...)})
	}
	return objects
}

// ObjectCount returns the number of indirect objects referenced by the imported
// page.
func (p *PageRef) ObjectCount() int {
	if p == nil {
		return 0
	}
	return len(p.objects)
}

// ObjectRefs returns imported object references sorted by object number and
// generation.
func (p *PageRef) ObjectRefs() []ObjRef {
	if p == nil {
		return nil
	}
	refs := p.objectRefs
	if refs == nil {
		refs = sortedObjRefs(p.objects)
	}
	return append([]ObjRef(nil), refs...)
}

// ForEachObject calls fn for each imported object in sorted reference order.
// The body slice passed to fn is a copy. Use ForEachObjectBorrowed only for
// performance-sensitive internal code that can honor borrowed-slice semantics.
func (p *PageRef) ForEachObject(fn func(ObjRef, []byte) error) error {
	return p.ForEachObjectCopy(fn)
}

// ForEachObjectCopy calls fn for each imported object in sorted reference
// order. The body slice passed to fn is a copy.
func (p *PageRef) ForEachObjectCopy(fn func(ObjRef, []byte) error) error {
	if p == nil || fn == nil {
		return nil
	}
	refs := p.objectRefs
	if refs == nil {
		refs = sortedObjRefs(p.objects)
	}
	for _, ref := range refs {
		if err := fn(ref, append([]byte(nil), p.objects[ref]...)); err != nil {
			return err
		}
	}
	return nil
}

// ForEachObjectBorrowed calls fn for each imported object in sorted reference
// order. The body slice is owned by PageRef and must not be retained or
// modified.
func (p *PageRef) ForEachObjectBorrowed(fn func(ObjRef, []byte) error) error {
	if p == nil || fn == nil {
		return nil
	}
	refs := p.objectRefs
	if refs == nil {
		refs = sortedObjRefs(p.objects)
	}
	for _, ref := range refs {
		if err := fn(ref, p.objects[ref]); err != nil {
			return err
		}
	}
	return nil
}

func sortedObjRefs(objects map[ObjRef][]byte) []ObjRef {
	refs := make([]ObjRef, 0, len(objects))
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

type pdfValueKind int

const (
	pdfValueRaw pdfValueKind = iota
	pdfValueNumber
	pdfValueName
	pdfValueArray
	pdfValueDict
	pdfValueRef
	pdfValueNull
)

type pdfValue struct {
	kind   pdfValueKind
	raw    []byte
	number float64
	name   string
	array  []pdfValue
	dict   pdfDict
	ref    ObjRef
}

type pdfDict map[string]pdfValue

type Source struct {
	mu         sync.Mutex
	data       []byte
	readerAt   io.ReaderAt
	readerSize int64
	offsets    map[ObjRef]int
	cache      map[ObjRef][]byte
	trailer    pdfDict
	root       ObjRef
	pages      []sourcePage
	limits     ImportOptions
}

// PageCount returns the number of importable pages in the source.
func (doc *Source) PageCount() int {
	if doc == nil {
		return 0
	}
	return len(doc.pages)
}

type sourcePage struct {
	ref       ObjRef
	resources pdfValue
	boxes     map[string]pdfValue
	contents  pdfValue
}

const (
	MaxArrayItems          = 10000
	MaxDecodedStreamBytes  = 32 * 1024 * 1024
	MaxDictEntries         = 10000
	MaxPages               = 10000
	MaxPageContentBytes    = 32 * 1024 * 1024
	MaxPageTreeDepth       = 512
	MaxReferencedObjects   = 10000
	MaxXrefEntries         = 100000
	MaxXrefChainLength     = 128
	MaxValueNesting        = 128
	maxReaderAtObjectBytes = 32 * 1024 * 1024
	maxReaderAtTailBytes   = 1 * 1024 * 1024
	maxReaderAtXrefBytes   = 16 * 1024 * 1024
)

type pdfBox struct {
	llx, lly float64
	urx, ury float64
}

func parseSourceWithOptionsContext(ctx context.Context, data []byte, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	doc := &Source{
		data:    data,
		offsets: make(map[ObjRef]int),
		cache:   make(map[ObjRef][]byte),
		limits:  options,
	}
	start, err := findPDFStartXref(data)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	for start > 0 {
		if err := importContextErr(ctx); err != nil {
			return nil, err
		}
		if seen[start] {
			return nil, errors.New("PDF import found cyclic xref chain")
		}
		if len(seen) >= MaxXrefChainLength {
			return nil, errors.New("PDF import xref chain exceeds maximum size")
		}
		seen[start] = true
		trailer, prev, err := doc.parseXrefAtContext(ctx, start)
		if err != nil {
			return nil, err
		}
		if doc.trailer == nil {
			doc.trailer = trailer
		}
		start = prev
	}
	if _, ok := doc.trailer["Encrypt"]; ok {
		return nil, errors.New("encrypted PDFs are not supported by the built-in importer")
	}
	root, ok := pdfValueAsRef(doc.trailer["Root"])
	if !ok {
		return nil, errors.New("PDF trailer does not contain a root catalog")
	}
	doc.root = root
	if err := doc.loadPagesContext(ctx); err != nil {
		return nil, err
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return doc, nil
}

func parseSourceReaderAtWithOptionsContext(ctx context.Context, r io.ReaderAt, size int64, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	doc := &Source{
		readerAt:   r,
		readerSize: size,
		offsets:    make(map[ObjRef]int),
		cache:      make(map[ObjRef][]byte),
		limits:     options,
	}
	start, err := doc.findReaderAtStartXrefContext(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	for start > 0 {
		if err := importContextErr(ctx); err != nil {
			return nil, err
		}
		if seen[start] {
			return nil, errors.New("PDF import found cyclic xref chain")
		}
		if len(seen) >= MaxXrefChainLength {
			return nil, errors.New("PDF import xref chain exceeds maximum size")
		}
		seen[start] = true
		trailer, prev, err := doc.parseXrefAtContext(ctx, start)
		if err != nil {
			return nil, err
		}
		if doc.trailer == nil {
			doc.trailer = trailer
		}
		start = prev
	}
	if _, ok := doc.trailer["Encrypt"]; ok {
		return nil, errors.New("encrypted PDFs are not supported by the built-in importer")
	}
	root, ok := pdfValueAsRef(doc.trailer["Root"])
	if !ok {
		return nil, errors.New("PDF trailer does not contain a root catalog")
	}
	doc.root = root
	if err := doc.loadPagesContext(ctx); err != nil {
		return nil, err
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return doc, nil
}

func findPDFStartXref(data []byte) (int, error) {
	pos := bytes.LastIndex(data, []byte("startxref"))
	if pos < 0 {
		return 0, errors.New("PDF startxref not found")
	}
	p := pos + len("startxref")
	p = skipPDFSpace(data, p)
	n, _, _, ok := readPDFIntToken(data, p)
	if !ok {
		return 0, errors.New("PDF startxref offset is invalid")
	}
	return n, nil
}

func (doc *Source) parseXrefAtContext(ctx context.Context, offset int) (pdfDict, int, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, 0, err
	}
	if doc.data == nil {
		return doc.parseReaderAtXrefAtContext(ctx, offset)
	}
	p := skipPDFSpace(doc.data, offset)
	if !hasPDFWord(doc.data, p, "xref") {
		return nil, 0, errors.New("PDF xref streams are not supported by the built-in importer")
	}
	p += len("xref")
	for {
		if err := importContextErr(ctx); err != nil {
			return nil, 0, err
		}
		p = skipPDFSpace(doc.data, p)
		if p >= len(doc.data) {
			return nil, 0, errors.New("PDF xref table ended before trailer")
		}
		if hasPDFWord(doc.data, p, "trailer") {
			p += len("trailer")
			p = skipPDFSpace(doc.data, p)
			parser := newPDFValueParserContext(ctx, doc.data[p:])
			value, err := parser.parseValue()
			if err != nil {
				return nil, 0, fmt.Errorf("invalid PDF trailer: %w", err)
			}
			if value.kind != pdfValueDict {
				return nil, 0, errors.New("PDF trailer is not a dictionary")
			}
			prev := 0
			if prevValue, ok := value.dict["Prev"]; ok {
				prev = int(math.Round(prevValue.number))
			}
			return value.dict, prev, nil
		}
		startObj, _, next, ok := readPDFIntToken(doc.data, p)
		if !ok {
			return nil, 0, errors.New("invalid PDF xref subsection")
		}
		p = skipPDFSpace(doc.data, next)
		count, _, next, ok := readPDFIntToken(doc.data, p)
		if !ok || count < 0 {
			return nil, 0, errors.New("invalid PDF xref subsection length")
		}
		if count > MaxXrefEntries || len(doc.offsets)+count > MaxXrefEntries {
			return nil, 0, errors.New("PDF xref entry count exceeds maximum size")
		}
		p = next
		for i := 0; i < count; i++ {
			if err := importContextErr(ctx); err != nil {
				return nil, 0, err
			}
			p = skipPDFLineBreaks(doc.data, p)
			entryOffset, _, next, ok := readPDFIntToken(doc.data, p)
			if !ok {
				return nil, 0, errors.New("invalid PDF xref entry")
			}
			p = skipPDFSpace(doc.data, next)
			gen, _, next, ok := readPDFIntToken(doc.data, p)
			if !ok {
				return nil, 0, errors.New("invalid PDF xref generation")
			}
			p = skipPDFSpace(doc.data, next)
			if p >= len(doc.data) {
				return nil, 0, errors.New("invalid PDF xref status")
			}
			status := doc.data[p]
			p = skipToNextPDFLine(doc.data, p)
			if status == 'n' {
				ref := ObjRef{num: startObj + i, gen: gen}
				if _, exists := doc.offsets[ref]; !exists {
					doc.offsets[ref] = entryOffset
				}
			}
		}
	}
}

func (doc *Source) findReaderAtStartXrefContext(ctx context.Context) (int, error) {
	if err := importContextErr(ctx); err != nil {
		return 0, err
	}
	if doc.readerSize <= 0 {
		return 0, errors.New("PDF startxref not found")
	}
	size := int64(maxReaderAtTailBytes)
	if doc.readerSize < size {
		size = doc.readerSize
	}
	tail, err := doc.readAtContext(ctx, int(doc.readerSize-size), int(size))
	if err != nil {
		return 0, err
	}
	return findPDFStartXref(tail)
}

func (doc *Source) parseReaderAtXrefAtContext(ctx context.Context, offset int) (pdfDict, int, error) {
	chunkSize := maxReaderAtXrefBytes
	if remain := int(doc.readerSize) - offset; remain < chunkSize {
		chunkSize = remain
	}
	if chunkSize <= 0 {
		return nil, 0, errors.New("PDF xref offset is invalid")
	}
	data, err := doc.readAtContext(ctx, offset, chunkSize)
	if err != nil {
		return nil, 0, err
	}
	p := skipPDFSpace(data, 0)
	if !hasPDFWord(data, p, "xref") {
		return nil, 0, errors.New("PDF xref streams are not supported by the built-in importer")
	}
	p += len("xref")
	for {
		if err := importContextErr(ctx); err != nil {
			return nil, 0, err
		}
		p = skipPDFSpace(data, p)
		if p >= len(data) {
			return nil, 0, errors.New("PDF xref table ended before trailer")
		}
		if hasPDFWord(data, p, "trailer") {
			p += len("trailer")
			p = skipPDFSpace(data, p)
			parser := newPDFValueParserContext(ctx, data[p:])
			value, err := parser.parseValue()
			if err != nil {
				return nil, 0, fmt.Errorf("invalid PDF trailer: %w", err)
			}
			if value.kind != pdfValueDict {
				return nil, 0, errors.New("PDF trailer is not a dictionary")
			}
			prev := 0
			if prevValue, ok := value.dict["Prev"]; ok {
				prev = int(math.Round(prevValue.number))
			}
			return value.dict, prev, nil
		}
		startObj, _, next, ok := readPDFIntToken(data, p)
		if !ok {
			return nil, 0, errors.New("invalid PDF xref subsection")
		}
		p = skipPDFSpace(data, next)
		count, _, next, ok := readPDFIntToken(data, p)
		if !ok || count < 0 {
			return nil, 0, errors.New("invalid PDF xref subsection length")
		}
		if count > MaxXrefEntries || len(doc.offsets)+count > MaxXrefEntries {
			return nil, 0, errors.New("PDF xref entry count exceeds maximum size")
		}
		p = next
		for i := 0; i < count; i++ {
			if err := importContextErr(ctx); err != nil {
				return nil, 0, err
			}
			p = skipPDFLineBreaks(data, p)
			entryOffset, _, next, ok := readPDFIntToken(data, p)
			if !ok {
				return nil, 0, errors.New("invalid PDF xref entry")
			}
			p = skipPDFSpace(data, next)
			gen, _, next, ok := readPDFIntToken(data, p)
			if !ok {
				return nil, 0, errors.New("invalid PDF xref generation")
			}
			p = skipPDFSpace(data, next)
			if p >= len(data) {
				return nil, 0, errors.New("invalid PDF xref status")
			}
			status := data[p]
			p = skipToNextPDFLine(data, p)
			if status == 'n' {
				ref := ObjRef{num: startObj + i, gen: gen}
				if _, exists := doc.offsets[ref]; !exists {
					doc.offsets[ref] = entryOffset
				}
			}
		}
	}
}

func (doc *Source) loadPagesContext(ctx context.Context) error {
	if err := importContextErr(ctx); err != nil {
		return err
	}
	rootBody, err := doc.objectBodyContext(ctx, doc.root)
	if err != nil {
		return err
	}
	rootDict, err := parsePDFObjectDictContext(ctx, rootBody)
	if err != nil {
		return fmt.Errorf("invalid PDF catalog: %w", err)
	}
	pagesRef, ok := pdfValueAsRef(rootDict["Pages"])
	if !ok {
		return errors.New("PDF catalog does not contain a page tree")
	}
	return doc.walkPageTreeContext(ctx, pagesRef, pdfPageInherited{}, 0, make(map[ObjRef]bool))
}

type pdfPageInherited struct {
	resources pdfValue
	boxes     map[string]pdfValue
}

func (doc *Source) walkPageTreeContext(ctx context.Context, ref ObjRef, inherited pdfPageInherited, depth int, visiting map[ObjRef]bool) error {
	if err := importContextErr(ctx); err != nil {
		return err
	}
	if depth > MaxPageTreeDepth {
		return errors.New("PDF page tree exceeds maximum depth")
	}
	if visiting[ref] {
		return errors.New("PDF page tree contains a cycle")
	}
	visiting[ref] = true
	defer func() { visiting[ref] = false }()

	body, err := doc.objectBodyContext(ctx, ref)
	if err != nil {
		return err
	}
	dict, err := parsePDFObjectDictContext(ctx, body)
	if err != nil {
		return fmt.Errorf("invalid PDF page tree object %d %d R: %w", ref.num, ref.gen, err)
	}
	nextInherited := inherited
	boxesCopied := false
	if nextInherited.boxes == nil {
		nextInherited.boxes = make(map[string]pdfValue)
		boxesCopied = true
	}
	if resources, ok := dict["Resources"]; ok {
		nextInherited.resources = resources
	}
	for _, name := range pdfPageBoxNames() {
		if box, ok := dict[name]; ok {
			if boxesCopied {
				nextInherited.boxes[name] = box
			} else {
				boxes := make(map[string]pdfValue, len(inherited.boxes)+1)
				for k, v := range inherited.boxes {
					boxes[k] = v
				}
				boxes[name] = box
				nextInherited.boxes = boxes
				boxesCopied = true
			}
		}
	}
	typeName := ""
	if typ, ok := dict["Type"]; ok && typ.kind == pdfValueName {
		typeName = typ.name
	}
	contents, hasContents := dict["Contents"]
	if typeName == "Page" || (typeName == "" && hasContents) {
		if len(doc.pages) >= MaxPages {
			return errors.New("PDF page count exceeds maximum size")
		}
		pageBoxes := make(map[string]pdfValue, len(nextInherited.boxes))
		for k, v := range nextInherited.boxes {
			pageBoxes[k] = v
		}
		doc.pages = append(doc.pages, sourcePage{
			ref:       ref,
			resources: nextInherited.resources,
			boxes:     pageBoxes,
			contents:  contents,
		})
		return nil
	}
	kids, ok := dict["Kids"]
	if !ok || kids.kind != pdfValueArray {
		return fmt.Errorf("PDF page tree object %d %d R does not contain page kids", ref.num, ref.gen)
	}
	for _, kid := range kids.array {
		if err := importContextErr(ctx); err != nil {
			return err
		}
		kidRef, ok := pdfValueAsRef(kid)
		if !ok {
			return errors.New("PDF page tree contains a non-reference kid")
		}
		if err := doc.walkPageTreeContext(ctx, kidRef, nextInherited, depth+1, visiting); err != nil {
			return err
		}
	}
	return nil
}

func (doc *Source) Page(pageNo int, boxName string) (*PageRef, error) {
	return doc.PageContext(context.Background(), pageNo, boxName)
}

// PageContext returns an imported page and checks ctx while resolving page
// resources, content streams, and referenced objects.
func (doc *Source) PageContext(ctx context.Context, pageNo int, boxName string) (*PageRef, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	doc.mu.Lock()
	defer doc.mu.Unlock()
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	if pageNo < 1 || pageNo > len(doc.pages) {
		return nil, fmt.Errorf("PDF page number %d is out of range", pageNo)
	}
	page := doc.pages[pageNo-1]
	box, err := page.selectedBox(boxName)
	if err != nil {
		return nil, err
	}
	resources := page.resources.raw
	if len(bytes.TrimSpace(resources)) == 0 {
		resources = []byte("<<>>")
	}
	encodedContent, encodedFilter, encoded, err := doc.encodedPageContentContext(ctx, page, box)
	if err != nil {
		return nil, err
	}
	var wrapped []byte
	if !encoded {
		content, err := doc.pageContentContext(ctx, page)
		if err != nil {
			return nil, err
		}
		wrapped = wrapImportedPageContent(content, box)
	}
	objects := make(map[ObjRef][]byte)
	if err := doc.collectReferencedObjectsContext(ctx, resources, objects, make(map[ObjRef]bool)); err != nil {
		return nil, err
	}
	objectRefs := sortedObjRefs(objects)
	return &PageRef{
		widthPt:              box.urx - box.llx,
		heightPt:             box.ury - box.lly,
		resources:            append([]byte(nil), resources...),
		content:              wrapped,
		source:               doc,
		sourcePage:           page,
		box:                  box,
		encodedContent:       encodedContent,
		encodedContentFilter: encodedFilter,
		objects:              objects,
		objectRefs:           objectRefs,
	}, nil
}

func (doc *Source) pageContentContext(ctx context.Context, page sourcePage) ([]byte, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	if page.contents.kind == pdfValueNull || len(page.contents.raw) == 0 {
		return nil, nil
	}
	var out bytes.Buffer
	values := []pdfValue{page.contents}
	if page.contents.kind == pdfValueArray {
		values = page.contents.array
	}
	for _, value := range values {
		if err := importContextErr(ctx); err != nil {
			return nil, err
		}
		ref, ok := pdfValueAsRef(value)
		if !ok {
			return nil, errors.New("PDF page content must be an indirect stream")
		}
		body, err := doc.objectBodyContext(ctx, ref)
		if err != nil {
			return nil, err
		}
		dict, stream, err := doc.parseStreamContext(ctx, body)
		if err != nil {
			return nil, fmt.Errorf("invalid PDF content stream %d %d R: %w", ref.num, ref.gen, err)
		}
		decoded, err := decodePDFStreamContext(ctx, dict, stream)
		if err != nil {
			return nil, fmt.Errorf("unsupported PDF content stream %d %d R: %w", ref.num, ref.gen, err)
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		if out.Len()+len(decoded) > MaxPageContentBytes {
			return nil, errors.New("PDF page content exceeds maximum size")
		}
		out.Write(decoded)
	}
	return out.Bytes(), nil
}

func (doc *Source) encodedPageContentContext(ctx context.Context, page sourcePage, box pdfBox) ([]byte, string, bool, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, "", false, err
	}
	if box.llx != 0 || box.lly != 0 {
		return nil, "", false, nil
	}
	ref, ok := singleContentStreamRef(page.contents)
	if !ok {
		return nil, "", false, nil
	}
	body, err := doc.objectBodyContext(ctx, ref)
	if err != nil {
		if ctxErr := importContextErr(ctx); ctxErr != nil {
			return nil, "", false, ctxErr
		}
		return nil, "", false, nil
	}
	dict, stream, err := doc.parseStreamContext(ctx, body)
	if err != nil {
		if ctxErr := importContextErr(ctx); ctxErr != nil {
			return nil, "", false, ctxErr
		}
		return nil, "", false, nil
	}
	filter, ok := preservedPDFStreamFilter(dict)
	if !ok {
		return nil, "", false, nil
	}
	return append([]byte(nil), stream...), filter, true, nil
}

func singleContentStreamRef(value pdfValue) (ObjRef, bool) {
	if value.kind == pdfValueNull || len(value.raw) == 0 {
		return ObjRef{}, false
	}
	if value.kind == pdfValueArray {
		if len(value.array) != 1 {
			return ObjRef{}, false
		}
		value = value.array[0]
	}
	return pdfValueAsRef(value)
}

func wrapImportedPageContent(content []byte, box pdfBox) []byte {
	var out bytes.Buffer
	out.WriteString("q\n")
	if box.llx != 0 || box.lly != 0 {
		fmt.Fprintf(&out, "1 0 0 1 %.5f %.5f cm\n", -box.llx, -box.lly)
	}
	out.Write(content)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("Q")
	return out.Bytes()
}

func (doc *Source) collectReferencedObjectsContext(ctx context.Context, data []byte, objects map[ObjRef][]byte, visiting map[ObjRef]bool) error {
	if err := importContextErr(ctx); err != nil {
		return err
	}
	for _, ref := range refsInPDFValueBytesContext(ctx, data) {
		if err := importContextErr(ctx); err != nil {
			return err
		}
		if _, ok := objects[ref]; ok {
			continue
		}
		if visiting[ref] {
			continue
		}
		if limit := doc.maxReferencedObjects(); limit > 0 && len(objects) >= limit {
			return errors.New("PDF referenced object count exceeds maximum size")
		}
		visiting[ref] = true
		body, err := doc.objectBodyContext(ctx, ref)
		if err != nil {
			return err
		}
		objects[ref] = append([]byte(nil), body...)
		if err := doc.collectReferencedObjectsContext(ctx, pdfObjectReferenceSection(body), objects, visiting); err != nil {
			return err
		}
		visiting[ref] = false
	}
	return nil
}

func (doc *Source) maxReferencedObjects() int {
	if doc == nil || doc.limits.MaxReferencedObjects <= 0 {
		return MaxReferencedObjects
	}
	return doc.limits.MaxReferencedObjects
}

func (doc *Source) objectBodyContext(ctx context.Context, ref ObjRef) ([]byte, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	if body, ok := doc.cache[ref]; ok {
		return body, nil
	}
	offset, ok := doc.offsets[ref]
	if !ok {
		return nil, fmt.Errorf("PDF object %d %d R was not found", ref.num, ref.gen)
	}
	if doc.data == nil {
		if offset < 0 || int64(offset) >= doc.readerSize {
			return nil, fmt.Errorf("PDF object %d %d R has invalid offset", ref.num, ref.gen)
		}
		body, err := doc.readerAtObjectBodyContext(ctx, ref, offset)
		if err != nil {
			return nil, err
		}
		doc.cache[ref] = body
		return body, nil
	}
	if offset < 0 || offset >= len(doc.data) {
		return nil, fmt.Errorf("PDF object %d %d R has invalid offset", ref.num, ref.gen)
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	body, found, err := doc.objectBodyFromBytesContext(ctx, ref, doc.data[offset:])
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("PDF object %d %d R is missing endobj", ref.num, ref.gen)
	}
	doc.cache[ref] = body
	return body, nil
}

func (doc *Source) readerAtObjectBodyContext(ctx context.Context, ref ObjRef, offset int) ([]byte, error) {
	limit := maxReaderAtObjectBytes
	if remain := int(doc.readerSize) - offset; remain < limit {
		limit = remain
	}
	for size := minInt(64*1024, limit); size <= limit; size *= 2 {
		if err := importContextErr(ctx); err != nil {
			return nil, err
		}
		data, err := doc.readAtContext(ctx, offset, size)
		if err != nil {
			return nil, err
		}
		if body, ok, err := doc.objectBodyFromBytesContext(ctx, ref, data); err != nil || ok {
			return body, err
		}
		if size == limit {
			break
		}
		if size*2 > limit {
			size = limit / 2
		}
	}
	return nil, fmt.Errorf("PDF object %d %d R is missing endobj", ref.num, ref.gen)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (doc *Source) objectBodyFromBytesContext(ctx context.Context, ref ObjRef, data []byte) ([]byte, bool, error) {
	bodyStart, err := objectBodyStart(data, ref)
	if err != nil {
		return nil, false, err
	}
	end, found, err := doc.objectBodyEndContext(ctx, ref, data, bodyStart)
	if err != nil || !found {
		return nil, found, err
	}
	body := bytes.TrimSpace(data[bodyStart:end])
	if body == nil {
		body = []byte{}
	}
	return body, true, nil
}

func objectBodyStart(data []byte, ref ObjRef) (int, error) {
	pos := skipPDFSpace(data, 0)
	object, _, next, ok := readPDFIntToken(data, pos)
	if !ok {
		return 0, fmt.Errorf("PDF object %d %d R is missing object number", ref.num, ref.gen)
	}
	pos = skipPDFSpace(data, next)
	generation, _, next, ok := readPDFIntToken(data, pos)
	if !ok {
		return 0, fmt.Errorf("PDF object %d %d R is missing generation", ref.num, ref.gen)
	}
	pos = skipPDFSpace(data, next)
	if !hasPDFWord(data, pos, "obj") {
		return 0, fmt.Errorf("PDF object %d %d R is missing obj marker", ref.num, ref.gen)
	}
	if object != ref.num || generation != ref.gen {
		return 0, fmt.Errorf("PDF object header is %d %d, want %d %d", object, generation, ref.num, ref.gen)
	}
	return pos + len("obj"), nil
}

func (doc *Source) objectBodyEndContext(ctx context.Context, ref ObjRef, data []byte, bodyStart int) (int, bool, error) {
	for pos := bodyStart; pos < len(data); {
		if pos%1024 == 0 {
			if err := importContextErr(ctx); err != nil {
				return 0, false, err
			}
		}
		switch data[pos] {
		case '(':
			pos = skipPDFLiteralString(data, pos)
			continue
		case '<':
			if pos+1 < len(data) && data[pos+1] != '<' {
				pos = skipPDFHexString(data, pos)
				continue
			}
		case '%':
			pos = skipToNextPDFLine(data, pos)
			continue
		case '/':
			pos++
			for pos < len(data) && !isPDFDelimiter(data[pos]) && !isPDFSpace(data[pos]) {
				pos++
			}
			continue
		}
		if isPDFSpace(data[pos]) || isPDFDelimiter(data[pos]) {
			pos++
			continue
		}
		start := pos
		for pos < len(data) && !isPDFSpace(data[pos]) && !isPDFDelimiter(data[pos]) {
			pos++
		}
		word := string(data[start:pos])
		switch word {
		case "endobj":
			return start, true, nil
		case "stream":
			next, complete, err := doc.skipObjectStreamContext(ctx, ref, data, bodyStart, start, pos)
			if err != nil || !complete {
				return 0, complete, err
			}
			pos = next
		}
	}
	return 0, false, nil
}

func (doc *Source) skipObjectStreamContext(ctx context.Context, ref ObjRef, data []byte, bodyStart, streamStart, afterStream int) (int, bool, error) {
	dict, err := parsePDFObjectDictContext(ctx, data[bodyStart:streamStart])
	if err != nil {
		return 0, false, fmt.Errorf("PDF object %d %d R has invalid stream dictionary: %w", ref.num, ref.gen, err)
	}
	pos := afterStream
	if pos+1 < len(data) && data[pos] == '\r' && data[pos+1] == '\n' {
		pos += 2
	} else if pos < len(data) && (data[pos] == '\r' || data[pos] == '\n') {
		pos++
	} else if pos >= len(data) {
		return 0, false, nil
	} else {
		return 0, false, errors.New("PDF stream marker is not followed by a line break")
	}

	length, known, err := doc.pdfStreamLengthContext(ctx, dict)
	if err != nil {
		return 0, false, err
	}
	if known {
		if length > len(data)-pos {
			return 0, false, nil
		}
		pos += length
	} else {
		end := bytes.Index(data[pos:], []byte("endstream"))
		if end < 0 {
			return 0, false, nil
		}
		pos += end
	}
	pos = skipPDFLineBreaks(data, pos)
	if !hasPDFWord(data, pos, "endstream") {
		if pos >= len(data) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("PDF object %d %d R is missing endstream", ref.num, ref.gen)
	}
	return pos + len("endstream"), true, nil
}

func (doc *Source) pdfStreamLengthContext(ctx context.Context, dict pdfDict) (int, bool, error) {
	value, ok := dict["Length"]
	if !ok {
		return 0, false, nil
	}
	if value.kind == pdfValueNumber {
		length := int(math.Round(value.number))
		if length < 0 || float64(length) != value.number {
			return 0, false, errors.New("PDF stream length is invalid")
		}
		return length, true, nil
	}
	ref, ok := pdfValueAsRef(value)
	if !ok {
		return 0, false, errors.New("PDF stream length is invalid")
	}
	body, err := doc.objectBodyContext(ctx, ref)
	if err != nil {
		return 0, false, err
	}
	parser := newPDFValueParserContext(ctx, body)
	value, err = parser.parseValue()
	if err != nil {
		return 0, false, err
	}
	if value.kind != pdfValueNumber {
		return 0, false, errors.New("PDF stream length is invalid")
	}
	length := int(math.Round(value.number))
	if length < 0 || float64(length) != value.number {
		return 0, false, errors.New("PDF stream length is invalid")
	}
	return length, true, nil
}

func (doc *Source) readAtContext(ctx context.Context, offset, length int) ([]byte, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	if offset < 0 || length < 0 || int64(offset)+int64(length) > doc.readerSize {
		return nil, errors.New("PDF read offset is invalid")
	}
	buf := make([]byte, length)
	if doc.readerAt == nil {
		return nil, errors.New("PDF reader is unavailable")
	}
	_, err := doc.readerAt.ReadAt(buf, int64(offset))
	if err != nil && err != io.EOF {
		return nil, err
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return buf, nil
}

func parsePDFObjectDictContext(ctx context.Context, body []byte) (pdfDict, error) {
	parser := newPDFValueParserContext(ctx, body)
	value, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	if value.kind != pdfValueDict {
		return nil, errors.New("object is not a dictionary")
	}
	return value.dict, nil
}

func (doc *Source) parseStreamContext(ctx context.Context, body []byte) (pdfDict, []byte, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, nil, err
	}
	if body == nil {
		body = []byte{}
	}
	streamPos := bytes.Index(body, []byte("stream"))
	if streamPos < 0 {
		return nil, nil, errors.New("stream marker not found")
	}
	dict, err := parsePDFObjectDictContext(ctx, body[:streamPos])
	if err != nil {
		return nil, nil, err
	}
	start := streamPos + len("stream")
	if start+1 < len(body) && body[start] == '\r' && body[start+1] == '\n' {
		start += 2
	} else if start < len(body) && (body[start] == '\n' || body[start] == '\r') {
		start++
	}
	length := -1
	if lengthValue, ok := dict["Length"]; ok {
		if lengthValue.kind == pdfValueNumber {
			length = int(math.Round(lengthValue.number))
		} else if ref, ok := pdfValueAsRef(lengthValue); ok {
			body, err := doc.objectBodyContext(ctx, ref)
			if err != nil {
				return nil, nil, err
			}
			parser := newPDFValueParserContext(ctx, body)
			value, err := parser.parseValue()
			if err != nil {
				return nil, nil, err
			}
			if value.kind == pdfValueNumber {
				length = int(math.Round(value.number))
			}
		}
	}
	if length >= 0 && start+length <= len(body) {
		return dict, body[start : start+length], nil
	}
	end := bytes.Index(body[start:], []byte("endstream"))
	if end < 0 {
		return nil, nil, errors.New("endstream marker not found")
	}
	stream := body[start : start+end]
	stream = bytes.TrimRight(stream, "\r\n")
	return dict, stream, nil
}

func decodePDFStream(dict pdfDict, stream []byte) ([]byte, error) {
	return decodePDFStreamContext(context.Background(), dict, stream)
}

func decodePDFStreamContext(ctx context.Context, dict pdfDict, stream []byte) ([]byte, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	filters := pdfStreamFilters(dict["Filter"])
	if len(filters) == 0 {
		if len(stream) > MaxDecodedStreamBytes {
			return nil, errors.New("PDF stream exceeds maximum size")
		}
		return append([]byte(nil), stream...), nil
	}
	if len(filters) == 1 && (filters[0] == "FlateDecode" || filters[0] == "Fl") {
		return uncompressStreamContext(ctx, stream, MaxDecodedStreamBytes)
	}
	return nil, fmt.Errorf("filters %v are not supported", filters)
}

func preservedPDFStreamFilter(dict pdfDict) (string, bool) {
	if _, ok := dict["DecodeParms"]; ok {
		return "", false
	}
	filters := pdfStreamFilters(dict["Filter"])
	if len(filters) != 1 {
		return "", false
	}
	if filters[0] != "FlateDecode" && filters[0] != "Fl" {
		return "", false
	}
	return "FlateDecode", true
}

func uncompressStreamContext(ctx context.Context, data []byte, limit int) ([]byte, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	var out bytes.Buffer
	_, err = out.ReadFrom(io.LimitReader(importContextReader{ctx: ctx, r: reader}, int64(limit)+1))
	if err == nil && out.Len() > limit {
		err = errors.New("uncompressed data exceeds expected size")
	}
	if err != nil {
		return nil, err
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func pdfStreamFilters(value pdfValue) []string {
	switch value.kind {
	case pdfValueName:
		return []string{value.name}
	case pdfValueArray:
		filters := make([]string, 0, len(value.array))
		for _, item := range value.array {
			if item.kind == pdfValueName {
				filters = append(filters, item.name)
			}
		}
		return filters
	default:
		return nil
	}
}

func (page sourcePage) selectedBox(name string) (pdfBox, error) {
	boxName := normalizePDFPageBoxName(name)
	value, ok := page.boxes[boxName]
	if !ok && boxName != "MediaBox" {
		value, ok = page.boxes["MediaBox"]
	}
	if !ok {
		return pdfBox{}, fmt.Errorf("PDF page does not define %s", boxName)
	}
	return pdfValueBox(value)
}

func (doc *Source) PageSizes() map[int]map[string]Size {
	result := make(map[int]map[string]Size, len(doc.pages))
	for i, page := range doc.pages {
		pageResult := make(map[string]Size)
		for _, name := range pdfPageBoxNames() {
			if value, ok := page.boxes[name]; ok {
				if box, err := pdfValueBox(value); err == nil {
					pageResult[name] = Size{Wd: box.urx - box.llx, Ht: box.ury - box.lly}
				}
			}
		}
		result[i+1] = pageResult
	}
	return result
}

func pdfValueBox(value pdfValue) (pdfBox, error) {
	if value.kind != pdfValueArray || len(value.array) != 4 {
		return pdfBox{}, errors.New("PDF page box is invalid")
	}
	nums := make([]float64, 4)
	for i, item := range value.array {
		if item.kind != pdfValueNumber {
			return pdfBox{}, errors.New("PDF page box contains a non-number")
		}
		nums[i] = item.number
	}
	return pdfBox{llx: nums[0], lly: nums[1], urx: nums[2], ury: nums[3]}, nil
}

func normalizePDFPageBoxName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "MediaBox"
	}
	lower := strings.ToLower(name)
	for _, candidate := range pdfPageBoxNames() {
		if strings.ToLower(candidate) == lower {
			return candidate
		}
	}
	return name
}

func pdfPageBoxNames() []string {
	return []string{"MediaBox", "CropBox", "BleedBox", "TrimBox", "ArtBox"}
}

func pdfValueAsRef(value pdfValue) (ObjRef, bool) {
	if value.kind != pdfValueRef {
		return ObjRef{}, false
	}
	return value.ref, true
}

type pdfValueParser struct {
	data  []byte
	pos   int
	depth int
	ctx   context.Context
}

func newPDFValueParser(data []byte) *pdfValueParser {
	return newPDFValueParserContext(context.Background(), data)
}

func newPDFValueParserContext(ctx context.Context, data []byte) *pdfValueParser {
	return &pdfValueParser{data: data, ctx: ctx}
}

func (p *pdfValueParser) parseValue() (pdfValue, error) {
	if err := importContextErr(p.ctx); err != nil {
		return pdfValue{}, err
	}
	if p.depth > MaxValueNesting {
		return pdfValue{}, errors.New("PDF value nesting exceeds maximum size")
	}
	p.pos = skipPDFSpace(p.data, p.pos)
	start := p.pos
	if p.pos >= len(p.data) {
		return pdfValue{}, errors.New("unexpected end of PDF value")
	}
	var (
		value pdfValue
		err   error
	)
	switch p.data[p.pos] {
	case '<':
		if p.pos+1 < len(p.data) && p.data[p.pos+1] == '<' {
			value, err = p.parseDict()
		} else {
			value, err = p.parseHexString()
		}
	case '[':
		value, err = p.parseArray()
	case '/':
		value, err = p.parseName()
	case '(':
		value, err = p.parseLiteralString()
	default:
		switch {
		case isPDFNumberStart(p.data[p.pos]):
			value, err = p.parseNumberOrRef()
		case hasPDFWord(p.data, p.pos, "true") || hasPDFWord(p.data, p.pos, "false"):
			value, err = p.parseKeyword()
		case hasPDFWord(p.data, p.pos, "null"):
			p.pos += len("null")
			value = pdfValue{kind: pdfValueNull}
		default:
			err = fmt.Errorf("unexpected PDF token at byte %d", p.pos)
		}
	}
	if err != nil {
		return pdfValue{}, err
	}
	value.raw = append([]byte(nil), p.data[start:p.pos]...)
	return value, nil
}

func (p *pdfValueParser) parseDict() (pdfValue, error) {
	p.pos += 2
	dict := make(pdfDict)
	for {
		if err := importContextErr(p.ctx); err != nil {
			return pdfValue{}, err
		}
		p.pos = skipPDFSpace(p.data, p.pos)
		if p.pos+1 < len(p.data) && p.data[p.pos] == '>' && p.data[p.pos+1] == '>' {
			p.pos += 2
			return pdfValue{kind: pdfValueDict, dict: dict}, nil
		}
		if len(dict) >= MaxDictEntries {
			return pdfValue{}, errors.New("PDF dictionary exceeds maximum size")
		}
		key, err := p.parseName()
		if err != nil {
			return pdfValue{}, err
		}
		p.depth++
		value, err := p.parseValue()
		p.depth--
		if err != nil {
			return pdfValue{}, err
		}
		dict[key.name] = value
	}
}

func (p *pdfValueParser) parseArray() (pdfValue, error) {
	p.pos++
	var array []pdfValue
	for {
		if err := importContextErr(p.ctx); err != nil {
			return pdfValue{}, err
		}
		p.pos = skipPDFSpace(p.data, p.pos)
		if p.pos >= len(p.data) {
			return pdfValue{}, errors.New("unterminated PDF array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return pdfValue{kind: pdfValueArray, array: array}, nil
		}
		if len(array) >= MaxArrayItems {
			return pdfValue{}, errors.New("PDF array exceeds maximum size")
		}
		p.depth++
		value, err := p.parseValue()
		p.depth--
		if err != nil {
			return pdfValue{}, err
		}
		array = append(array, value)
	}
}

func (p *pdfValueParser) parseName() (pdfValue, error) {
	if err := importContextErr(p.ctx); err != nil {
		return pdfValue{}, err
	}
	if p.pos >= len(p.data) || p.data[p.pos] != '/' {
		return pdfValue{}, errors.New("PDF name expected")
	}
	p.pos++
	start := p.pos
	for p.pos < len(p.data) && !isPDFDelimiter(p.data[p.pos]) && !isPDFSpace(p.data[p.pos]) {
		p.pos++
	}
	return pdfValue{kind: pdfValueName, name: string(p.data[start:p.pos])}, nil
}

func (p *pdfValueParser) parseNumberOrRef() (pdfValue, error) {
	if err := importContextErr(p.ctx); err != nil {
		return pdfValue{}, err
	}
	start := p.pos
	first, firstIsInt, next, ok := readPDFNumberToken(p.data, p.pos)
	if !ok {
		return pdfValue{}, errors.New("PDF number expected")
	}
	if firstIsInt {
		save := next
		next = skipPDFSpace(p.data, next)
		second, secondIsInt, afterSecond, ok := readPDFNumberToken(p.data, next)
		if ok && secondIsInt {
			afterSecond = skipPDFSpace(p.data, afterSecond)
			if afterSecond < len(p.data) && p.data[afterSecond] == 'R' && isPDFBoundary(p.data, afterSecond+1) {
				p.pos = afterSecond + 1
				return pdfValue{kind: pdfValueRef, ref: ObjRef{num: int(first), gen: int(second)}}, nil
			}
		}
		p.pos = save
	} else {
		p.pos = next
	}
	number, err := strconv.ParseFloat(string(p.data[start:p.pos]), 64)
	if err != nil {
		return pdfValue{}, err
	}
	return pdfValue{kind: pdfValueNumber, number: number}, nil
}

func (p *pdfValueParser) parseLiteralString() (pdfValue, error) {
	depth := 0
	for p.pos < len(p.data) {
		if p.pos%1024 == 0 {
			if err := importContextErr(p.ctx); err != nil {
				return pdfValue{}, err
			}
		}
		ch := p.data[p.pos]
		p.pos++
		if ch == '\\' && p.pos < len(p.data) {
			p.pos++
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pdfValue{kind: pdfValueRaw}, nil
			}
		}
	}
	return pdfValue{}, errors.New("unterminated PDF literal string")
}

func (p *pdfValueParser) parseHexString() (pdfValue, error) {
	p.pos++
	for p.pos < len(p.data) {
		if p.pos%1024 == 0 {
			if err := importContextErr(p.ctx); err != nil {
				return pdfValue{}, err
			}
		}
		if p.data[p.pos] == '>' {
			p.pos++
			return pdfValue{kind: pdfValueRaw}, nil
		}
		p.pos++
	}
	return pdfValue{}, errors.New("unterminated PDF hex string")
}

func (p *pdfValueParser) parseKeyword() (pdfValue, error) {
	if err := importContextErr(p.ctx); err != nil {
		return pdfValue{}, err
	}
	for p.pos < len(p.data) && !isPDFBoundary(p.data, p.pos) {
		p.pos++
	}
	return pdfValue{kind: pdfValueRaw}, nil
}

type foundPDFRef struct {
	ref        ObjRef
	start, end int
}

func refsInPDFValueBytesContext(ctx context.Context, data []byte) []ObjRef {
	parser := newPDFValueParserContext(ctx, data)
	value, err := parser.parseValue()
	if err != nil {
		if importContextErr(ctx) != nil {
			return nil
		}
		found := findPDFIndirectRefs(data)
		refs := make([]ObjRef, 0, len(found))
		for _, ref := range found {
			refs = append(refs, ref.ref)
		}
		return refs
	}
	refs := make(map[ObjRef]bool)
	collectPDFValueRefs(value, refs)
	out := make([]ObjRef, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].num == out[j].num {
			return out[i].gen < out[j].gen
		}
		return out[i].num < out[j].num
	})
	return out
}

// RewriteIndirectRefs rewrites indirect object references found outside stream
// bodies according to refMap.
func RewriteIndirectRefs(data []byte, refMap map[ObjRef]int) []byte {
	if data == nil {
		return []byte{}
	}
	streamPos := bytes.Index(data, []byte("stream"))
	if streamPos < 0 {
		return rewriteIndirectRefsInSection(data, refMap)
	}
	out := rewriteIndirectRefsInSection(data[:streamPos], refMap)
	out = append(out, data[streamPos:]...)
	return out
}

func rewriteIndirectRefsInSection(data []byte, refMap map[ObjRef]int) []byte {
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

func collectPDFValueRefs(value pdfValue, refs map[ObjRef]bool) {
	switch value.kind {
	case pdfValueRef:
		refs[value.ref] = true
	case pdfValueArray:
		for _, item := range value.array {
			collectPDFValueRefs(item, refs)
		}
	case pdfValueDict:
		for _, item := range value.dict {
			collectPDFValueRefs(item, refs)
		}
	}
}

func findPDFIndirectRefs(data []byte) []foundPDFRef {
	var refs []foundPDFRef
	for p := 0; p < len(data); {
		switch data[p] {
		case '(':
			p = skipPDFLiteralString(data, p)
			continue
		case '<':
			if p+1 < len(data) && data[p+1] != '<' {
				p = skipPDFHexString(data, p)
				continue
			}
		case '%':
			p = skipToNextPDFLine(data, p)
			continue
		}
		if !isPDFDigit(data[p]) {
			p++
			continue
		}
		if p > 0 && !isPDFBoundary(data, p) {
			p++
			continue
		}
		firstStart := p
		first, _, next, ok := readPDFIntToken(data, p)
		if !ok {
			p++
			continue
		}
		next = skipPDFSpace(data, next)
		second, _, afterSecond, ok := readPDFIntToken(data, next)
		if !ok {
			p = firstStart + 1
			continue
		}
		afterSecond = skipPDFSpace(data, afterSecond)
		if afterSecond < len(data) && data[afterSecond] == 'R' && isPDFBoundary(data, afterSecond+1) {
			refs = append(refs, foundPDFRef{
				ref:   ObjRef{num: first, gen: second},
				start: firstStart,
				end:   afterSecond + 1,
			})
			p = afterSecond + 1
			continue
		}
		p = firstStart + 1
	}
	return refs
}

func pdfObjectReferenceSection(body []byte) []byte {
	if body == nil {
		return []byte{}
	}
	streamPos := bytes.Index(body, []byte("stream"))
	if streamPos < 0 {
		return body
	}
	return body[:streamPos]
}

func readPDFIntToken(data []byte, pos int) (value int, isInt bool, next int, ok bool) {
	number, isInt, next, ok := readPDFNumberToken(data, pos)
	if !ok || !isInt {
		return 0, false, pos, false
	}
	return int(number), true, next, true
}

func readPDFNumberToken(data []byte, pos int) (value float64, isInt bool, next int, ok bool) {
	if pos >= len(data) {
		return 0, false, pos, false
	}
	start := pos
	if data[pos] == '+' || data[pos] == '-' {
		pos++
	}
	digits := 0
	dot := false
	for pos < len(data) {
		ch := data[pos]
		if isPDFDigit(ch) {
			digits++
			pos++
			continue
		}
		if ch == '.' && !dot {
			dot = true
			pos++
			continue
		}
		break
	}
	if digits == 0 {
		return 0, false, start, false
	}
	value, err := strconv.ParseFloat(string(data[start:pos]), 64)
	if err != nil {
		return 0, false, start, false
	}
	return value, !dot, pos, true
}

func skipPDFSpace(data []byte, pos int) int {
	for pos < len(data) {
		if data[pos] == '%' {
			pos = skipToNextPDFLine(data, pos)
			continue
		}
		if !isPDFSpace(data[pos]) {
			break
		}
		pos++
	}
	return pos
}

func skipPDFLineBreaks(data []byte, pos int) int {
	for pos < len(data) && (data[pos] == '\r' || data[pos] == '\n') {
		pos++
	}
	return pos
}

func skipToNextPDFLine(data []byte, pos int) int {
	for pos < len(data) && data[pos] != '\n' && data[pos] != '\r' {
		pos++
	}
	for pos < len(data) && (data[pos] == '\n' || data[pos] == '\r') {
		pos++
	}
	return pos
}

func skipPDFLiteralString(data []byte, pos int) int {
	depth := 0
	for pos < len(data) {
		ch := data[pos]
		pos++
		if ch == '\\' && pos < len(data) {
			pos++
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pos
			}
		}
	}
	return pos
}

func skipPDFHexString(data []byte, pos int) int {
	pos++
	for pos < len(data) {
		if data[pos] == '>' {
			return pos + 1
		}
		pos++
	}
	return pos
}

func hasPDFWord(data []byte, pos int, word string) bool {
	end := pos + len(word)
	if pos < 0 || end > len(data) {
		return false
	}
	return bytes.Equal(data[pos:end], []byte(word)) && isPDFBoundary(data, end)
}

func isPDFBoundary(data []byte, pos int) bool {
	if pos <= 0 || pos >= len(data) {
		return true
	}
	return isPDFSpace(data[pos]) || isPDFDelimiter(data[pos])
}

func isPDFNumberStart(ch byte) bool {
	return isPDFDigit(ch) || ch == '+' || ch == '-' || ch == '.'
}

func isPDFDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isPDFSpace(ch byte) bool {
	switch ch {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

func isPDFDelimiter(ch byte) bool {
	switch ch {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}
