// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type pdfObjRef struct {
	num int
	gen int
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
	ref    pdfObjRef
}

type pdfDict map[string]pdfValue

type pdfImportDocument struct {
	data    []byte
	offsets map[pdfObjRef]int
	cache   map[pdfObjRef][]byte
	trailer pdfDict
	root    pdfObjRef
	pages   []pdfImportPage
}

type pdfImportPage struct {
	ref       pdfObjRef
	resources pdfValue
	boxes     map[string]pdfValue
	contents  pdfValue
}

type pdfBox struct {
	llx, lly float64
	urx, ury float64
}

func parsePDFImportDocument(data []byte) (*pdfImportDocument, error) {
	doc := &pdfImportDocument{
		data:    data,
		offsets: make(map[pdfObjRef]int),
		cache:   make(map[pdfObjRef][]byte),
	}
	start, err := findPDFStartXref(data)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	for start > 0 {
		if seen[start] {
			return nil, fmt.Errorf("PDF import found cyclic xref chain")
		}
		seen[start] = true
		trailer, prev, err := doc.parseXrefAt(start)
		if err != nil {
			return nil, err
		}
		if doc.trailer == nil {
			doc.trailer = trailer
		}
		start = prev
	}
	if _, ok := doc.trailer["Encrypt"]; ok {
		return nil, fmt.Errorf("encrypted PDFs are not supported by the built-in importer")
	}
	root, ok := pdfValueAsRef(doc.trailer["Root"])
	if !ok {
		return nil, fmt.Errorf("PDF trailer does not contain a root catalog")
	}
	doc.root = root
	if err := doc.loadPages(); err != nil {
		return nil, err
	}
	return doc, nil
}

func findPDFStartXref(data []byte) (int, error) {
	pos := bytes.LastIndex(data, []byte("startxref"))
	if pos < 0 {
		return 0, fmt.Errorf("PDF startxref not found")
	}
	p := pos + len("startxref")
	p = skipPDFSpace(data, p)
	n, _, _, ok := readPDFIntToken(data, p)
	if !ok {
		return 0, fmt.Errorf("PDF startxref offset is invalid")
	}
	return n, nil
}

func (doc *pdfImportDocument) parseXrefAt(offset int) (pdfDict, int, error) {
	p := skipPDFSpace(doc.data, offset)
	if !hasPDFWord(doc.data, p, "xref") {
		return nil, 0, fmt.Errorf("PDF xref streams are not supported by the built-in importer")
	}
	p += len("xref")
	for {
		p = skipPDFSpace(doc.data, p)
		if p >= len(doc.data) {
			return nil, 0, fmt.Errorf("PDF xref table ended before trailer")
		}
		if hasPDFWord(doc.data, p, "trailer") {
			p += len("trailer")
			p = skipPDFSpace(doc.data, p)
			parser := newPDFValueParser(doc.data[p:])
			value, err := parser.parseValue()
			if err != nil {
				return nil, 0, fmt.Errorf("invalid PDF trailer: %w", err)
			}
			if value.kind != pdfValueDict {
				return nil, 0, fmt.Errorf("PDF trailer is not a dictionary")
			}
			prev := 0
			if prevValue, ok := value.dict["Prev"]; ok {
				prev = int(math.Round(prevValue.number))
			}
			return value.dict, prev, nil
		}
		startObj, _, next, ok := readPDFIntToken(doc.data, p)
		if !ok {
			return nil, 0, fmt.Errorf("invalid PDF xref subsection")
		}
		p = skipPDFSpace(doc.data, next)
		count, _, next, ok := readPDFIntToken(doc.data, p)
		if !ok || count < 0 {
			return nil, 0, fmt.Errorf("invalid PDF xref subsection length")
		}
		p = next
		for i := 0; i < count; i++ {
			p = skipPDFLineBreaks(doc.data, p)
			entryOffset, _, next, ok := readPDFIntToken(doc.data, p)
			if !ok {
				return nil, 0, fmt.Errorf("invalid PDF xref entry")
			}
			p = skipPDFSpace(doc.data, next)
			gen, _, next, ok := readPDFIntToken(doc.data, p)
			if !ok {
				return nil, 0, fmt.Errorf("invalid PDF xref generation")
			}
			p = skipPDFSpace(doc.data, next)
			if p >= len(doc.data) {
				return nil, 0, fmt.Errorf("invalid PDF xref status")
			}
			status := doc.data[p]
			p = skipToNextPDFLine(doc.data, p)
			if status == 'n' {
				ref := pdfObjRef{num: startObj + i, gen: gen}
				if _, exists := doc.offsets[ref]; !exists {
					doc.offsets[ref] = entryOffset
				}
			}
		}
	}
}

func (doc *pdfImportDocument) loadPages() error {
	rootBody, err := doc.objectBody(doc.root)
	if err != nil {
		return err
	}
	rootDict, err := parsePDFObjectDict(rootBody)
	if err != nil {
		return fmt.Errorf("invalid PDF catalog: %w", err)
	}
	pagesRef, ok := pdfValueAsRef(rootDict["Pages"])
	if !ok {
		return fmt.Errorf("PDF catalog does not contain a page tree")
	}
	return doc.walkPageTree(pagesRef, pdfPageInherited{})
}

type pdfPageInherited struct {
	resources pdfValue
	boxes     map[string]pdfValue
}

func (doc *pdfImportDocument) walkPageTree(ref pdfObjRef, inherited pdfPageInherited) error {
	body, err := doc.objectBody(ref)
	if err != nil {
		return err
	}
	dict, err := parsePDFObjectDict(body)
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
		pageBoxes := make(map[string]pdfValue, len(nextInherited.boxes))
		for k, v := range nextInherited.boxes {
			pageBoxes[k] = v
		}
		doc.pages = append(doc.pages, pdfImportPage{
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
		kidRef, ok := pdfValueAsRef(kid)
		if !ok {
			return fmt.Errorf("PDF page tree contains a non-reference kid")
		}
		if err := doc.walkPageTree(kidRef, nextInherited); err != nil {
			return err
		}
	}
	return nil
}

func (doc *pdfImportDocument) importPage(pageNo int, boxName string) (*importedPDFPage, error) {
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
	content, err := doc.pageContent(page)
	if err != nil {
		return nil, err
	}
	wrapped := wrapImportedPageContent(content, box)
	objects := make(map[pdfObjRef][]byte)
	if err := doc.collectReferencedObjects(resources, objects, make(map[pdfObjRef]bool)); err != nil {
		return nil, err
	}
	return &importedPDFPage{
		widthPt:   box.urx - box.llx,
		heightPt:  box.ury - box.lly,
		resources: append([]byte(nil), resources...),
		content:   wrapped,
		objects:   objects,
	}, nil
}

func (doc *pdfImportDocument) pageContent(page pdfImportPage) ([]byte, error) {
	if page.contents.kind == pdfValueNull || len(page.contents.raw) == 0 {
		return nil, nil
	}
	var out bytes.Buffer
	values := []pdfValue{page.contents}
	if page.contents.kind == pdfValueArray {
		values = page.contents.array
	}
	for _, value := range values {
		ref, ok := pdfValueAsRef(value)
		if !ok {
			return nil, fmt.Errorf("PDF page content must be an indirect stream")
		}
		body, err := doc.objectBody(ref)
		if err != nil {
			return nil, err
		}
		dict, stream, err := doc.parseStream(body)
		if err != nil {
			return nil, fmt.Errorf("invalid PDF content stream %d %d R: %w", ref.num, ref.gen, err)
		}
		decoded, err := decodePDFStream(dict, stream)
		if err != nil {
			return nil, fmt.Errorf("unsupported PDF content stream %d %d R: %w", ref.num, ref.gen, err)
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.Write(decoded)
	}
	return out.Bytes(), nil
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

func (doc *pdfImportDocument) collectReferencedObjects(data []byte, objects map[pdfObjRef][]byte, visiting map[pdfObjRef]bool) error {
	for _, ref := range refsInPDFValueBytes(data) {
		if _, ok := objects[ref]; ok {
			continue
		}
		if visiting[ref] {
			continue
		}
		visiting[ref] = true
		body, err := doc.objectBody(ref)
		if err != nil {
			return err
		}
		objects[ref] = append([]byte(nil), body...)
		if err := doc.collectReferencedObjects(pdfObjectReferenceSection(body), objects, visiting); err != nil {
			return err
		}
		visiting[ref] = false
	}
	return nil
}

func (doc *pdfImportDocument) objectBody(ref pdfObjRef) ([]byte, error) {
	if body, ok := doc.cache[ref]; ok {
		return body, nil
	}
	offset, ok := doc.offsets[ref]
	if !ok {
		return nil, fmt.Errorf("PDF object %d %d R was not found", ref.num, ref.gen)
	}
	if offset < 0 || offset >= len(doc.data) {
		return nil, fmt.Errorf("PDF object %d %d R has invalid offset", ref.num, ref.gen)
	}
	end := bytes.Index(doc.data[offset:], []byte("endobj"))
	if end < 0 {
		return nil, fmt.Errorf("PDF object %d %d R is missing endobj", ref.num, ref.gen)
	}
	objectBytes := doc.data[offset : offset+end]
	objPos := bytes.Index(objectBytes, []byte("obj"))
	if objPos < 0 {
		return nil, fmt.Errorf("PDF object %d %d R is missing obj marker", ref.num, ref.gen)
	}
	body := bytes.TrimSpace(objectBytes[objPos+len("obj"):])
	if body == nil {
		body = []byte{}
	}
	doc.cache[ref] = body
	return body, nil
}

func parsePDFObjectDict(body []byte) (pdfDict, error) {
	parser := newPDFValueParser(body)
	value, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	if value.kind != pdfValueDict {
		return nil, fmt.Errorf("object is not a dictionary")
	}
	return value.dict, nil
}

func (doc *pdfImportDocument) parseStream(body []byte) (pdfDict, []byte, error) {
	if body == nil {
		body = []byte{}
	}
	streamPos := bytes.Index(body, []byte("stream"))
	if streamPos < 0 {
		return nil, nil, fmt.Errorf("stream marker not found")
	}
	dict, err := parsePDFObjectDict(body[:streamPos])
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
			body, err := doc.objectBody(ref)
			if err != nil {
				return nil, nil, err
			}
			parser := newPDFValueParser(body)
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
		return nil, nil, fmt.Errorf("endstream marker not found")
	}
	stream := body[start : start+end]
	stream = bytes.TrimRight(stream, "\r\n")
	return dict, stream, nil
}

func decodePDFStream(dict pdfDict, stream []byte) ([]byte, error) {
	filters := pdfStreamFilters(dict["Filter"])
	if len(filters) == 0 {
		return append([]byte(nil), stream...), nil
	}
	if len(filters) == 1 && (filters[0] == "FlateDecode" || filters[0] == "Fl") {
		return sliceUncompress(stream)
	}
	return nil, fmt.Errorf("filters %v are not supported", filters)
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

func (page pdfImportPage) selectedBox(name string) (pdfBox, error) {
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

func (doc *pdfImportDocument) pageSizes() map[int]map[string]Size {
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
		return pdfBox{}, fmt.Errorf("PDF page box is invalid")
	}
	nums := make([]float64, 4)
	for i, item := range value.array {
		if item.kind != pdfValueNumber {
			return pdfBox{}, fmt.Errorf("PDF page box contains a non-number")
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

func pdfValueAsRef(value pdfValue) (pdfObjRef, bool) {
	if value.kind != pdfValueRef {
		return pdfObjRef{}, false
	}
	return value.ref, true
}

type pdfValueParser struct {
	data []byte
	pos  int
}

func newPDFValueParser(data []byte) *pdfValueParser {
	return &pdfValueParser{data: data}
}

func (p *pdfValueParser) parseValue() (pdfValue, error) {
	p.pos = skipPDFSpace(p.data, p.pos)
	start := p.pos
	if p.pos >= len(p.data) {
		return pdfValue{}, fmt.Errorf("unexpected end of PDF value")
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
		if isPDFNumberStart(p.data[p.pos]) {
			value, err = p.parseNumberOrRef()
		} else if hasPDFWord(p.data, p.pos, "true") || hasPDFWord(p.data, p.pos, "false") {
			value, err = p.parseKeyword()
		} else if hasPDFWord(p.data, p.pos, "null") {
			p.pos += len("null")
			value = pdfValue{kind: pdfValueNull}
		} else {
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
		p.pos = skipPDFSpace(p.data, p.pos)
		if p.pos+1 < len(p.data) && p.data[p.pos] == '>' && p.data[p.pos+1] == '>' {
			p.pos += 2
			return pdfValue{kind: pdfValueDict, dict: dict}, nil
		}
		key, err := p.parseName()
		if err != nil {
			return pdfValue{}, err
		}
		value, err := p.parseValue()
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
		p.pos = skipPDFSpace(p.data, p.pos)
		if p.pos >= len(p.data) {
			return pdfValue{}, fmt.Errorf("unterminated PDF array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return pdfValue{kind: pdfValueArray, array: array}, nil
		}
		value, err := p.parseValue()
		if err != nil {
			return pdfValue{}, err
		}
		array = append(array, value)
	}
}

func (p *pdfValueParser) parseName() (pdfValue, error) {
	if p.pos >= len(p.data) || p.data[p.pos] != '/' {
		return pdfValue{}, fmt.Errorf("PDF name expected")
	}
	p.pos++
	start := p.pos
	for p.pos < len(p.data) && !isPDFDelimiter(p.data[p.pos]) && !isPDFSpace(p.data[p.pos]) {
		p.pos++
	}
	return pdfValue{kind: pdfValueName, name: string(p.data[start:p.pos])}, nil
}

func (p *pdfValueParser) parseNumberOrRef() (pdfValue, error) {
	start := p.pos
	first, firstIsInt, next, ok := readPDFNumberToken(p.data, p.pos)
	if !ok {
		return pdfValue{}, fmt.Errorf("PDF number expected")
	}
	if firstIsInt {
		save := next
		next = skipPDFSpace(p.data, next)
		second, secondIsInt, afterSecond, ok := readPDFNumberToken(p.data, next)
		if ok && secondIsInt {
			afterSecond = skipPDFSpace(p.data, afterSecond)
			if afterSecond < len(p.data) && p.data[afterSecond] == 'R' && isPDFBoundary(p.data, afterSecond+1) {
				p.pos = afterSecond + 1
				return pdfValue{kind: pdfValueRef, ref: pdfObjRef{num: int(first), gen: int(second)}}, nil
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
	return pdfValue{}, fmt.Errorf("unterminated PDF literal string")
}

func (p *pdfValueParser) parseHexString() (pdfValue, error) {
	p.pos++
	for p.pos < len(p.data) {
		if p.data[p.pos] == '>' {
			p.pos++
			return pdfValue{kind: pdfValueRaw}, nil
		}
		p.pos++
	}
	return pdfValue{}, fmt.Errorf("unterminated PDF hex string")
}

func (p *pdfValueParser) parseKeyword() (pdfValue, error) {
	for p.pos < len(p.data) && !isPDFBoundary(p.data, p.pos) {
		p.pos++
	}
	return pdfValue{kind: pdfValueRaw}, nil
}

type foundPDFRef struct {
	ref        pdfObjRef
	start, end int
}

func refsInPDFValueBytes(data []byte) []pdfObjRef {
	parser := newPDFValueParser(data)
	value, err := parser.parseValue()
	if err != nil {
		found := findPDFIndirectRefs(data)
		refs := make([]pdfObjRef, 0, len(found))
		for _, ref := range found {
			refs = append(refs, ref.ref)
		}
		return refs
	}
	refs := make(map[pdfObjRef]bool)
	collectPDFValueRefs(value, refs)
	out := make([]pdfObjRef, 0, len(refs))
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

func collectPDFValueRefs(value pdfValue, refs map[pdfObjRef]bool) {
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
				ref:   pdfObjRef{num: first, gen: second},
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
