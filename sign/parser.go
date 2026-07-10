// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const maxPDFObjectCount = 10000000

var pdfDelimiter = [256]bool{
	'(': true,
	')': true,
	'<': true,
	'>': true,
	'[': true,
	']': true,
	'{': true,
	'}': true,
	'/': true,
	'%': true,
}

type pdfRef struct {
	Object     int
	Generation int
}

type pdfContext struct {
	Data         []byte
	PreviousXref int
	Size         int
	Root         pdfRef
	Page         pdfRef
	Xref         *pdfXrefResolver
	Trailer      []byte
}

type xrefEntry struct {
	Object     int
	Generation int
	Offset     int
}

type pdfXrefResolver struct {
	input   []byte
	tables  []int
	offsets map[int]int
}

func analyzePDF(input []byte) (pdfContext, error) {
	return analyzePDFContext(context.Background(), input)
}

func analyzePDFContext(ctx context.Context, input []byte) (pdfContext, error) {
	if err := signContextErr(ctx); err != nil {
		return pdfContext{}, err
	}
	previousXref, err := findStartXref(input)
	if err != nil {
		return pdfContext{}, err
	}
	if err := signContextErr(ctx); err != nil {
		return pdfContext{}, err
	}
	if previousXref < 0 || previousXref >= len(input) {
		return pdfContext{}, errors.New("pdfsigning: startxref points outside PDF")
	}
	trailer, err := xrefTrailerContext(ctx, input, previousXref)
	if err != nil {
		return pdfContext{}, err
	}
	if err := signContextErr(ctx); err != nil {
		return pdfContext{}, err
	}
	if encrypted, err := hasPDFNameContext(ctx, trailer, "/Encrypt"); err != nil {
		return pdfContext{}, err
	} else if encrypted {
		return pdfContext{}, errors.New("pdfsigning: encrypted PDFs are not supported")
	}
	size, root, err := parseTrailerContext(ctx, trailer)
	if err != nil {
		return pdfContext{}, err
	}
	xref, err := newPDFXrefResolverContext(ctx, input, previousXref)
	if err != nil {
		return pdfContext{}, err
	}
	page, err := findFirstPageContext(ctx, input, xref, root, 0)
	if err != nil {
		return pdfContext{}, err
	}
	return pdfContext{Data: input, PreviousXref: previousXref, Size: size, Root: root, Page: page, Xref: xref, Trailer: trailer}, nil
}

func signContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func newPDFXrefResolverContext(ctx context.Context, input []byte, offset int) (*pdfXrefResolver, error) {
	resolver := &pdfXrefResolver{input: input, offsets: make(map[int]int)}
	seen := make(map[int]bool)
	for {
		if err := signContextErr(ctx); err != nil {
			return nil, err
		}
		if offset < 0 || offset >= len(input) {
			return nil, errors.New("pdfsigning: xref offset outside PDF")
		}
		if seen[offset] {
			return nil, errors.New("pdfsigning: cyclic xref /Prev chain")
		}
		seen[offset] = true
		trailer, err := xrefTrailerContext(ctx, input, offset)
		if err != nil {
			return nil, err
		}
		if encrypted, err := hasPDFNameContext(ctx, trailer, "/Encrypt"); err != nil {
			return nil, err
		} else if encrypted {
			return nil, errors.New("pdfsigning: encrypted PDFs are not supported")
		}
		resolver.tables = append(resolver.tables, offset)
		prev, ok, err := parsePrevXrefContext(ctx, trailer)
		if err != nil {
			return nil, err
		}
		if !ok {
			return resolver, nil
		}
		offset = prev
	}
}

func (resolver *pdfXrefResolver) objectOffsetContext(ctx context.Context, object int) (int, error) {
	if resolver == nil {
		return 0, errors.New("pdfsigning: xref resolver not initialized")
	}
	if offset, ok := resolver.offsets[object]; ok {
		return offset, nil
	}
	for _, tableOffset := range resolver.tables {
		if err := signContextErr(ctx); err != nil {
			return 0, err
		}
		offset, ok, err := parseXrefTableObjectOffsetContext(ctx, resolver.input, tableOffset, object)
		if err != nil {
			return 0, err
		}
		if ok {
			resolver.offsets[object] = offset
			return offset, nil
		}
	}
	return 0, fmt.Errorf("pdfsigning: object %d not found in xref", object)
}

func findStartXref(input []byte) (int, error) {
	idx := bytes.LastIndex(input, []byte("startxref"))
	if idx < 0 {
		return 0, errors.New("pdfsigning: startxref not found")
	}
	rest := bytes.TrimSpace(input[idx+len("startxref"):])
	lines := bytes.Split(rest, []byte{'\n'})
	if len(lines) == 0 {
		return 0, errors.New("pdfsigning: startxref value not found")
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(lines[0])))
	if err != nil {
		return 0, fmt.Errorf("pdfsigning: invalid startxref: %w", err)
	}
	return value, nil
}

func parseTrailerContext(ctx context.Context, trailer []byte) (int, pdfRef, error) {
	if err := signContextErr(ctx); err != nil {
		return 0, pdfRef{}, err
	}
	dict, err := trailerDictionaryContext(ctx, trailer)
	if err != nil {
		return 0, pdfRef{}, err
	}
	sizeValue, ok, err := trailerEntryValueContext(ctx, dict, "/Size")
	if err != nil {
		return 0, pdfRef{}, err
	}
	if !ok {
		return 0, pdfRef{}, errors.New("pdfsigning: trailer /Size not found")
	}
	size, _, ok := parseLeadingInt(sizeValue, 0)
	if !ok {
		return 0, pdfRef{}, errors.New("pdfsigning: invalid trailer /Size")
	}
	if size <= 0 || size > maxPDFObjectCount {
		return 0, pdfRef{}, fmt.Errorf("pdfsigning: unsupported trailer /Size %d", size)
	}
	root, ok, err := findReferenceContext(ctx, dict, "/Root")
	if err != nil {
		return 0, pdfRef{}, err
	}
	if !ok {
		return 0, pdfRef{}, errors.New("pdfsigning: trailer /Root not found")
	}
	return size, root, nil
}

func parsePrevXrefContext(ctx context.Context, trailer []byte) (int, bool, error) {
	if err := signContextErr(ctx); err != nil {
		return 0, false, err
	}
	dict, err := trailerDictionaryContext(ctx, trailer)
	if err != nil {
		return 0, false, err
	}
	prevValue, ok, err := trailerEntryValueContext(ctx, dict, "/Prev")
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	prev, _, ok := parseLeadingInt(prevValue, 0)
	if !ok {
		return 0, false, errors.New("pdfsigning: invalid trailer /Prev")
	}
	return prev, true, nil
}

func preservedTrailerEntries(trailer []byte) (string, error) {
	return preservedTrailerEntriesContext(context.Background(), trailer)
}

func preservedTrailerEntriesContext(ctx context.Context, trailer []byte) (string, error) {
	dict, err := trailerDictionaryContext(ctx, trailer)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, key := range []string{"/Info", "/ID"} {
		if err := signContextErr(ctx); err != nil {
			return "", err
		}
		value, ok, err := trailerEntryValueContext(ctx, dict, key)
		if err != nil {
			return "", err
		}
		if ok {
			out.WriteByte(' ')
			out.WriteString(key)
			out.WriteByte(' ')
			out.Write(value)
		}
	}
	return out.String(), nil
}

func trailerDictionaryContext(ctx context.Context, trailer []byte) ([]byte, error) {
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	start, err := indexBytesContext(ctx, trailer, []byte("<<"))
	if err != nil {
		return nil, err
	}
	if start < 0 {
		return nil, errors.New("pdfsigning: trailer dictionary not found")
	}
	end, err := findDictionaryEndContext(ctx, trailer[start:])
	if err != nil {
		return nil, err
	}
	return trailer[start : start+end], nil
}

func trailerEntryValueContext(ctx context.Context, dict []byte, key string) ([]byte, bool, error) {
	idx, err := findPDFNameContext(ctx, dict, key)
	if err != nil {
		return nil, false, err
	}
	if idx < 0 {
		return nil, false, nil
	}
	valueStart := skipPDFSpaces(dict, idx+len(key))
	if valueStart >= len(dict) {
		return nil, false, fmt.Errorf("pdfsigning: trailer %s value missing", key)
	}
	valueEnd, err := pdfValueEndContext(ctx, dict, valueStart)
	if err != nil {
		return nil, false, fmt.Errorf("pdfsigning: invalid trailer %s value: %w", key, err)
	}
	return bytes.TrimSpace(dict[valueStart:valueEnd]), true, nil
}

func pdfValueEndContext(ctx context.Context, input []byte, start int) (int, error) {
	if err := signContextErr(ctx); err != nil {
		return 0, err
	}
	if start >= len(input) {
		return 0, errors.New("value missing")
	}
	switch input[start] {
	case '[':
		end, err := findArrayEndContext(ctx, input[start:])
		if err != nil {
			return 0, err
		}
		return start + end + 1, nil
	case '<':
		if start+1 < len(input) && input[start+1] == '<' {
			end, err := findDictionaryEndContext(ctx, input[start:])
			if err != nil {
				return 0, err
			}
			return start + end, nil
		}
		end, err := findByteContext(ctx, input[start+1:], '>')
		if err != nil {
			return 0, err
		}
		if end < 0 {
			return 0, errors.New("hex string end not found")
		}
		return start + 1 + end + 1, nil
	case '(':
		end, err := findStringEndContext(ctx, input[start:])
		if err != nil {
			return 0, err
		}
		return start + end + 1, nil
	default:
		firstEnd, err := pdfTokenEndContext(ctx, input, start)
		if err != nil {
			return 0, err
		}
		if _, _, ok := parseLeadingInt(input, start); ok {
			secondStart := skipPDFSpaces(input, firstEnd)
			if _, secondEnd, ok := parseLeadingInt(input, secondStart); ok {
				thirdStart := skipPDFSpaces(input, secondEnd)
				if thirdStart < len(input) && input[thirdStart] == 'R' && isPDFTokenEnd(input, thirdStart+1) {
					return thirdStart + 1, nil
				}
			}
		}
		return firstEnd, nil
	}
}

func pdfTokenEndContext(ctx context.Context, input []byte, start int) (int, error) {
	pos := start
	for pos < len(input) && !isPDFTokenEnd(input, pos) {
		if (pos-start)%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return pos, err
			}
		}
		pos++
	}
	return pos, nil
}

func isPDFTokenEnd(input []byte, pos int) bool {
	if pos >= len(input) {
		return true
	}
	return input[pos] <= 0x20 || pdfDelimiter[input[pos]]
}

func xrefTrailerContext(ctx context.Context, input []byte, offset int) ([]byte, error) {
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	if offset < 0 || offset >= len(input) {
		return nil, errors.New("pdfsigning: xref offset outside PDF")
	}
	rest := input[offset:]
	trailerStart, err := indexBytesContext(ctx, rest, []byte("trailer"))
	if err != nil {
		return nil, err
	}
	if trailerStart < 0 {
		return nil, errors.New("pdfsigning: xref trailer not found")
	}
	trailerStart += offset
	afterTrailer := input[trailerStart:]
	startxref, err := indexBytesContext(ctx, afterTrailer, []byte("startxref"))
	if err != nil {
		return nil, err
	}
	if startxref < 0 {
		return afterTrailer, nil
	}
	return afterTrailer[:startxref], nil
}

func indexBytesContext(ctx context.Context, input, needle []byte) (int, error) {
	if err := signContextErr(ctx); err != nil {
		return -1, err
	}
	if len(needle) == 0 {
		return 0, nil
	}
	if len(needle) > len(input) {
		return -1, nil
	}
	for i := 0; i+len(needle) <= len(input); i++ {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return -1, err
			}
		}
		if input[i] == needle[0] && bytes.Equal(input[i:i+len(needle)], needle) {
			return i, nil
		}
	}
	return -1, nil
}

func findByteContext(ctx context.Context, input []byte, needle byte) (int, error) {
	if err := signContextErr(ctx); err != nil {
		return -1, err
	}
	for i, b := range input {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return -1, err
			}
		}
		if b == needle {
			return i, nil
		}
	}
	return -1, nil
}

func parseXrefTable(input []byte, offset int) (map[int]int, error) {
	return parseXrefTableContext(context.Background(), input, offset)
}

func parseXrefTableContext(ctx context.Context, input []byte, offset int) (map[int]int, error) {
	if !bytes.HasPrefix(input[offset:], []byte("xref")) {
		return nil, errors.New("pdfsigning: only classic xref tables are supported")
	}
	var offsets map[int]int
	pos := offset + len("xref")
	for pos < len(input) {
		if err := signContextErr(ctx); err != nil {
			return nil, err
		}
		line, next := nextPDFLine(input, pos)
		pos = next
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.Equal(line, []byte("trailer")) {
			break
		}
		first, nextInt, ok := parseLeadingInt(line, 0)
		if !ok {
			return nil, errors.New("pdfsigning: invalid xref subsection")
		}
		count, _, ok := parseLeadingInt(line, skipPDFSpaces(line, nextInt))
		if !ok {
			return nil, errors.New("pdfsigning: invalid xref subsection")
		}
		if offsets == nil {
			offsets = make(map[int]int, max(count, 0))
		}
		for n := 0; n < count; n++ {
			if err := signContextErr(ctx); err != nil {
				return nil, err
			}
			if pos >= len(input) {
				return nil, errors.New("pdfsigning: truncated xref subsection")
			}
			entry, next := nextPDFLine(input, pos)
			pos = next
			if len(entry) < 18 {
				return nil, errors.New("pdfsigning: invalid xref entry")
			}
			if entry[17] == 'n' {
				objectOffset, ok := parseXrefEntryOffset(entry)
				if !ok {
					return nil, errors.New("pdfsigning: invalid xref entry offset")
				}
				offsets[first+n] = objectOffset
			}
		}
	}
	if offsets == nil {
		offsets = make(map[int]int)
	}
	return offsets, nil
}

func parseXrefTableObjectOffsetContext(ctx context.Context, input []byte, offset, targetObject int) (int, bool, error) {
	if !bytes.HasPrefix(input[offset:], []byte("xref")) {
		return 0, false, errors.New("pdfsigning: only classic xref tables are supported")
	}
	pos := offset + len("xref")
	for pos < len(input) {
		if err := signContextErr(ctx); err != nil {
			return 0, false, err
		}
		line, next := nextPDFLine(input, pos)
		pos = next
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.Equal(line, []byte("trailer")) {
			break
		}
		first, nextInt, ok := parseLeadingInt(line, 0)
		if !ok {
			return 0, false, errors.New("pdfsigning: invalid xref subsection")
		}
		count, _, ok := parseLeadingInt(line, skipPDFSpaces(line, nextInt))
		if !ok {
			return 0, false, errors.New("pdfsigning: invalid xref subsection")
		}
		if targetObject < first || targetObject >= first+count {
			for n := 0; n < count; n++ {
				if err := signContextErr(ctx); err != nil {
					return 0, false, err
				}
				if pos >= len(input) {
					return 0, false, errors.New("pdfsigning: truncated xref subsection")
				}
				_, pos = nextPDFLine(input, pos)
			}
			continue
		}
		targetIndex := targetObject - first
		for n := 0; n < count; n++ {
			if err := signContextErr(ctx); err != nil {
				return 0, false, err
			}
			if pos >= len(input) {
				return 0, false, errors.New("pdfsigning: truncated xref subsection")
			}
			entry, next := nextPDFLine(input, pos)
			pos = next
			if n != targetIndex {
				continue
			}
			if len(entry) < 18 {
				return 0, false, errors.New("pdfsigning: invalid xref entry")
			}
			if entry[17] != 'n' {
				return 0, false, nil
			}
			objectOffset, ok := parseXrefEntryOffset(entry)
			if !ok {
				return 0, false, errors.New("pdfsigning: invalid xref entry offset")
			}
			return objectOffset, true, nil
		}
	}
	return 0, false, nil
}

func parseXrefEntryOffset(entry []byte) (int, bool) {
	if len(entry) >= 10 {
		offset := 0
		for _, c := range entry[:10] {
			if c < '0' || c > '9' {
				return parseLeadingXrefEntryOffset(entry)
			}
			offset = offset*10 + int(c-'0')
		}
		return offset, true
	}
	return parseLeadingXrefEntryOffset(entry)
}

func parseLeadingXrefEntryOffset(entry []byte) (int, bool) {
	objectOffset, _, ok := parseLeadingInt(entry, 0)
	return objectOffset, ok
}

func nextPDFLine(input []byte, pos int) ([]byte, int) {
	start := pos
	for pos < len(input) && input[pos] != '\n' {
		pos++
	}
	line := input[start:pos]
	if pos < len(input) {
		pos++
	}
	return line, pos
}

func findFirstPageContext(ctx context.Context, input []byte, xref *pdfXrefResolver, ref pdfRef, depth int) (pdfRef, error) {
	if err := signContextErr(ctx); err != nil {
		return pdfRef{}, err
	}
	if depth > 32 {
		return pdfRef{}, errors.New("pdfsigning: page tree is too deep")
	}
	offset, err := xref.objectOffsetContext(ctx, ref.Object)
	if err != nil {
		return pdfRef{}, err
	}
	dict, err := readObjectDictContext(ctx, input, ref, offset)
	if err != nil {
		return pdfRef{}, err
	}
	if bytes.Contains(dict, []byte("/Type /Page")) && !bytes.Contains(dict, []byte("/Type /Pages")) {
		return ref, nil
	}
	if pages, ok := findReference(dict, "/Pages"); ok {
		return findFirstPageContext(ctx, input, xref, pages, depth+1)
	}
	if kid, ok := findFirstKid(dict); ok {
		return findFirstPageContext(ctx, input, xref, kid, depth+1)
	}
	return pdfRef{}, errors.New("pdfsigning: first page not found")
}

func readObjectDict(input []byte, ref pdfRef, offset int) ([]byte, error) {
	return readObjectDictContext(context.Background(), input, ref, offset)
}

func readObjectDictContext(ctx context.Context, input []byte, ref pdfRef, offset int) ([]byte, error) {
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	if offset < 0 || offset >= len(input) {
		return nil, errors.New("pdfsigning: object offset outside PDF")
	}
	end, err := indexBytesContext(ctx, input[offset:], []byte("endobj"))
	if err != nil {
		return nil, err
	}
	if end < 0 {
		return nil, errors.New("pdfsigning: object terminator not found")
	}
	object := input[offset : offset+end]
	if err := validateObjectHeader(object, ref); err != nil {
		return nil, err
	}
	start, err := indexBytesContext(ctx, object, []byte("<<"))
	if err != nil {
		return nil, err
	}
	if start < 0 {
		return nil, errors.New("pdfsigning: object dictionary not found")
	}
	dictEnd, err := findDictionaryEndContext(ctx, object[start:])
	if err != nil {
		return nil, err
	}
	return object[start : start+dictEnd], nil
}

func validateObjectHeader(object []byte, ref pdfRef) error {
	i := 0
	objectNumber, next, ok := parseLeadingInt(object, i)
	if !ok {
		return errors.New("pdfsigning: object header not found")
	}
	i = skipPDFSpaces(object, next)
	generation, next, ok := parseLeadingInt(object, i)
	if !ok {
		return errors.New("pdfsigning: object generation not found")
	}
	i = skipPDFSpaces(object, next)
	if !bytes.HasPrefix(object[i:], []byte("obj")) {
		return errors.New("pdfsigning: object header marker not found")
	}
	if objectNumber != ref.Object || generation != ref.Generation {
		return fmt.Errorf("pdfsigning: xref points to object %d %d, want %d %d", objectNumber, generation, ref.Object, ref.Generation)
	}
	return nil
}

func parseLeadingInt(input []byte, pos int) (int, int, bool) {
	if pos >= len(input) {
		return 0, pos, false
	}
	start := pos
	for pos < len(input) && input[pos] >= '0' && input[pos] <= '9' {
		pos++
	}
	if pos == start {
		return 0, pos, false
	}
	value := 0
	maxInt := int(^uint(0) >> 1)
	for _, b := range input[start:pos] {
		digit := int(b - '0')
		if value > (maxInt-digit)/10 {
			return 0, pos, false
		}
		value = value*10 + digit
	}
	return value, pos, true
}

func findDictionaryEnd(input []byte) (int, error) {
	return findDictionaryEndContext(context.Background(), input)
}

func findDictionaryEndContext(ctx context.Context, input []byte) (int, error) {
	if len(input) < 2 || input[0] != '<' || input[1] != '<' {
		return 0, errors.New("pdfsigning: dictionary start not found")
	}
	depth := 0
	inString := false
	stringDepth := 0
	inHex := false
	inComment := false
	escaped := false
	for i := 0; i < len(input)-1; i++ {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return 0, err
			}
		}
		if inComment {
			if input[i] == '\r' || input[i] == '\n' {
				inComment = false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch input[i] {
			case '\\':
				escaped = true
			case '(':
				stringDepth++
			case ')':
				if stringDepth == 0 {
					inString = false
				} else {
					stringDepth--
				}
			}
			continue
		}
		if inHex {
			if input[i] == '>' {
				inHex = false
			}
			continue
		}
		switch {
		case input[i] == '%':
			inComment = true
		case input[i] == '(':
			inString = true
			stringDepth = 0
		case input[i] == '<' && input[i+1] == '<':
			depth++
			i++
		case input[i] == '<':
			inHex = true
		case input[i] == '>' && input[i+1] == '>':
			depth--
			i++
			if depth == 0 {
				return i + 1, nil
			}
			if depth < 0 {
				return 0, errors.New("pdfsigning: invalid dictionary nesting")
			}
		}
	}
	return 0, errors.New("pdfsigning: dictionary end not found")
}

func addDictEntry(dict []byte, key, value string) ([]byte, error) {
	return addDictEntries(dict, pdfDictEntry{key: key, value: value})
}

type pdfDictEntry struct {
	key          string
	value        string
	skipExisting bool
}

func addDictEntries(dict []byte, entries ...pdfDictEntry) ([]byte, error) {
	insert := make([]pdfDictEntry, 0, len(entries))
	extra := 0
	for _, entry := range entries {
		if findPDFName(dict, entry.key) >= 0 {
			if entry.skipExisting {
				continue
			}
			return nil, fmt.Errorf("pdfsigning: existing %s dictionaries are not supported yet", entry.key)
		}
		insert = append(insert, entry)
		extra += len(entry.key) + len(entry.value) + 3
	}
	if len(insert) == 0 {
		return dict, nil
	}
	idx := bytes.LastIndex(dict, []byte(">>"))
	if idx < 0 {
		return nil, errors.New("pdfsigning: dictionary end not found")
	}
	out := make([]byte, 0, len(dict)+extra+1)
	out = append(out, dict[:idx]...)
	for _, entry := range insert {
		out = append(out, ' ')
		out = append(out, entry.key...)
		out = append(out, ' ')
		out = append(out, entry.value...)
	}
	out = append(out, ' ')
	out = append(out, dict[idx:]...)
	return out, nil
}

func addAnnotation(dict []byte, annotationRef string) ([]byte, error) {
	idx := findPDFName(dict, "/Annots")
	if idx < 0 {
		return addDictEntry(dict, "/Annots", "["+annotationRef+"]")
	}
	arrayStart := skipPDFSpaces(dict, idx+len("/Annots"))
	if arrayStart >= len(dict) || dict[arrayStart] != '[' {
		return nil, errors.New("pdfsigning: referenced /Annots arrays are not supported yet")
	}
	arrayEnd, err := findArrayEnd(dict[arrayStart:])
	if err != nil {
		return nil, err
	}
	arrayEnd += arrayStart
	out := make([]byte, 0, len(dict)+len(annotationRef)+1)
	out = append(out, dict[:arrayEnd]...)
	out = append(out, ' ')
	out = append(out, annotationRef...)
	out = append(out, dict[arrayEnd:]...)
	return out, nil
}

func findReference(dict []byte, key string) (pdfRef, bool) {
	ref, ok, _ := findReferenceContext(context.Background(), dict, key)
	return ref, ok
}

func findReferenceContext(ctx context.Context, dict []byte, key string) (pdfRef, bool, error) {
	idx, err := findPDFNameContext(ctx, dict, key)
	if err != nil {
		return pdfRef{}, false, err
	}
	if idx < 0 {
		return pdfRef{}, false, nil
	}
	valueStart := skipPDFSpaces(dict, idx+len(key))
	obj, next, ok := parseLeadingInt(dict, valueStart)
	if !ok {
		return pdfRef{}, false, nil
	}
	gen, next, ok := parseLeadingInt(dict, skipPDFSpaces(dict, next))
	if !ok {
		return pdfRef{}, false, nil
	}
	refMarker := skipPDFSpaces(dict, next)
	if refMarker >= len(dict) || dict[refMarker] != 'R' || !isPDFTokenEnd(dict, refMarker+1) {
		return pdfRef{}, false, nil
	}
	return pdfRef{Object: obj, Generation: gen}, true, nil
}

func findFirstKid(dict []byte) (pdfRef, bool) {
	idx := findPDFName(dict, "/Kids")
	if idx < 0 {
		return pdfRef{}, false
	}
	pos := skipPDFSpaces(dict, idx+len("/Kids"))
	if pos >= len(dict) || dict[pos] != '[' {
		return pdfRef{}, false
	}
	pos = skipPDFSpaces(dict, pos+1)
	obj, next, ok := parseLeadingInt(dict, pos)
	if !ok {
		return pdfRef{}, false
	}
	gen, next, ok := parseLeadingInt(dict, skipPDFSpaces(dict, next))
	if !ok {
		return pdfRef{}, false
	}
	refMarker := skipPDFSpaces(dict, next)
	if refMarker >= len(dict) || dict[refMarker] != 'R' || !isPDFTokenEnd(dict, refMarker+1) {
		return pdfRef{}, false
	}
	return pdfRef{Object: obj, Generation: gen}, true
}

func findArrayEnd(input []byte) (int, error) {
	return findArrayEndContext(context.Background(), input)
}

func findArrayEndContext(ctx context.Context, input []byte) (int, error) {
	if len(input) == 0 || input[0] != '[' {
		return 0, errors.New("pdfsigning: array start not found")
	}
	depth := 0
	inString := false
	stringDepth := 0
	inHex := false
	escaped := false
	for i := 0; i < len(input); i++ {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return 0, err
			}
		}
		b := input[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch b {
			case '\\':
				escaped = true
			case '(':
				stringDepth++
			case ')':
				if stringDepth == 0 {
					inString = false
				} else {
					stringDepth--
				}
			}
			continue
		}
		if inHex {
			if b == '>' {
				inHex = false
			}
			continue
		}
		switch b {
		case '(':
			inString = true
			stringDepth = 0
		case '<':
			if i+1 < len(input) && input[i+1] == '<' {
				i++
			} else {
				inHex = true
			}
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i, nil
			}
			if depth < 0 {
				return 0, errors.New("pdfsigning: invalid array nesting")
			}
		}
	}
	return 0, errors.New("pdfsigning: array end not found")
}

func findStringEndContext(ctx context.Context, input []byte) (int, error) {
	if len(input) == 0 || input[0] != '(' {
		return 0, errors.New("pdfsigning: string start not found")
	}
	depth := 0
	escaped := false
	for i := 1; i < len(input); i++ {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return 0, err
			}
		}
		b := input[i]
		if escaped {
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return i, nil
			}
			depth--
		}
	}
	return 0, errors.New("pdfsigning: string end not found")
}

func findPDFName(input []byte, name string) int {
	idx, _ := findPDFNameContext(context.Background(), input, name)
	return idx
}

func findPDFNameContext(ctx context.Context, input []byte, name string) (int, error) {
	start := 0
	for {
		if err := signContextErr(ctx); err != nil {
			return -1, err
		}
		idx, err := indexPDFNameContext(ctx, input, name, start)
		if err != nil {
			return -1, err
		}
		if idx < 0 {
			return -1, nil
		}
		if isPDFNameBoundary(input, idx, len(name)) {
			return idx, nil
		}
		start = idx + len(name)
	}
}

func hasPDFNameContext(ctx context.Context, input []byte, name string) (bool, error) {
	idx, err := findPDFNameContext(ctx, input, name)
	return idx >= 0, err
}

func countPDFName(input []byte, name string) int {
	count := 0
	start := 0
	for {
		idx := indexPDFName(input, name, start)
		if idx < 0 {
			return count
		}
		if isPDFNameBoundary(input, idx, len(name)) {
			count++
		}
		start = idx + len(name)
	}
}

func findLastPDFName(input []byte, name string) int {
	end := len(input)
	for {
		idx := lastIndexPDFName(input[:end], name)
		if idx < 0 {
			return -1
		}
		if isPDFNameBoundary(input, idx, len(name)) {
			return idx
		}
		end = idx
	}
}

func indexPDFName(input []byte, name string, start int) int {
	idx, _ := indexPDFNameContext(context.Background(), input, name, start)
	return idx
}

func indexPDFNameContext(ctx context.Context, input []byte, name string, start int) (int, error) {
	if name == "" || start >= len(input) {
		return -1, nil
	}
	for i := start; i+len(name) <= len(input); i++ {
		if (i-start)%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return -1, err
			}
		}
		if input[i] != name[0] {
			continue
		}
		if matchPDFName(input[i:], name) {
			return i, nil
		}
	}
	return -1, nil
}

func lastIndexPDFName(input []byte, name string) int {
	if name == "" || len(name) > len(input) {
		return -1
	}
	for i := len(input) - len(name); i >= 0; i-- {
		if input[i] != name[0] {
			continue
		}
		if matchPDFName(input[i:], name) {
			return i
		}
	}
	return -1
}

func matchPDFName(input []byte, name string) bool {
	if len(input) < len(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if input[i] != name[i] {
			return false
		}
	}
	return true
}

func isPDFNameBoundary(input []byte, idx, length int) bool {
	if idx > 0 && isPDFNameChar(input[idx-1]) {
		return false
	}
	after := idx + length
	return after >= len(input) || !isPDFNameChar(input[after])
}

func isPDFNameChar(b byte) bool {
	return b > 0x20 && !pdfDelimiter[b]
}

func skipPDFSpaces(input []byte, pos int) int {
	for pos < len(input) {
		switch input[pos] {
		case 0, '\t', '\n', '\f', '\r', ' ':
			pos++
		default:
			return pos
		}
	}
	return pos
}
