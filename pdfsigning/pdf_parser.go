/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package pdfsigning

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const maxPDFObjectCount = 10000000

var (
	rootRefPattern = regexp.MustCompile(`/Root\s+(\d+)\s+(\d+)\s+R`)
	sizePattern    = regexp.MustCompile(`/Size\s+(\d+)`)
	prevPattern    = regexp.MustCompile(`/Prev\s+(\d+)`)
	kidsPattern    = regexp.MustCompile(`/Kids\s*\[\s*(\d+)\s+(\d+)\s+R`)
)

type pdfRef struct {
	Object     int
	Generation int
}

type pdfContext struct {
	Data          []byte
	PreviousXref  int
	Size          int
	Root          pdfRef
	Page          pdfRef
	ObjectOffsets map[int]int
	Trailer       []byte
}

type xrefEntry struct {
	Object     int
	Generation int
	Offset     int
}

func analyzePDF(input []byte) (pdfContext, error) {
	previousXref, err := findStartXref(input)
	if err != nil {
		return pdfContext{}, err
	}
	if previousXref < 0 || previousXref >= len(input) {
		return pdfContext{}, fmt.Errorf("pdfsigning: startxref points outside PDF")
	}
	trailer, err := xrefTrailer(input, previousXref)
	if err != nil {
		return pdfContext{}, err
	}
	if findPDFName(trailer, "/Encrypt") >= 0 {
		return pdfContext{}, fmt.Errorf("pdfsigning: encrypted PDFs are not supported")
	}
	size, root, err := parseTrailer(trailer)
	if err != nil {
		return pdfContext{}, err
	}
	offsets, err := parseXrefTables(input, previousXref)
	if err != nil {
		return pdfContext{}, err
	}
	page, err := findFirstPage(input, offsets, root, 0)
	if err != nil {
		return pdfContext{}, err
	}
	return pdfContext{Data: input, PreviousXref: previousXref, Size: size, Root: root, Page: page, ObjectOffsets: offsets, Trailer: trailer}, nil
}

func findStartXref(input []byte) (int, error) {
	idx := bytes.LastIndex(input, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("pdfsigning: startxref not found")
	}
	rest := bytes.TrimSpace(input[idx+len("startxref"):])
	lines := bytes.Split(rest, []byte{'\n'})
	if len(lines) == 0 {
		return 0, fmt.Errorf("pdfsigning: startxref value not found")
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(lines[0])))
	if err != nil {
		return 0, fmt.Errorf("pdfsigning: invalid startxref: %w", err)
	}
	return value, nil
}

func parseTrailer(trailer []byte) (int, pdfRef, error) {
	sizeMatch := sizePattern.FindSubmatch(trailer)
	if len(sizeMatch) != 2 {
		return 0, pdfRef{}, fmt.Errorf("pdfsigning: trailer /Size not found")
	}
	size, err := strconv.Atoi(string(sizeMatch[1]))
	if err != nil {
		return 0, pdfRef{}, fmt.Errorf("pdfsigning: invalid trailer /Size: %w", err)
	}
	if size <= 0 || size > maxPDFObjectCount {
		return 0, pdfRef{}, fmt.Errorf("pdfsigning: unsupported trailer /Size %d", size)
	}
	rootMatch := rootRefPattern.FindSubmatch(trailer)
	if len(rootMatch) != 3 {
		return 0, pdfRef{}, fmt.Errorf("pdfsigning: trailer /Root not found")
	}
	rootObj, err := strconv.Atoi(string(rootMatch[1]))
	if err != nil {
		return 0, pdfRef{}, err
	}
	rootGen, err := strconv.Atoi(string(rootMatch[2]))
	if err != nil {
		return 0, pdfRef{}, err
	}
	return size, pdfRef{Object: rootObj, Generation: rootGen}, nil
}

func parsePrevXref(trailer []byte) (int, bool, error) {
	prevMatch := prevPattern.FindSubmatch(trailer)
	if len(prevMatch) == 0 {
		return 0, false, nil
	}
	if len(prevMatch) != 2 {
		return 0, false, fmt.Errorf("pdfsigning: invalid trailer /Prev")
	}
	prev, err := strconv.Atoi(string(prevMatch[1]))
	if err != nil {
		return 0, false, fmt.Errorf("pdfsigning: invalid trailer /Prev: %w", err)
	}
	return prev, true, nil
}

func preservedTrailerEntries(trailer []byte) (string, error) {
	dict, err := trailerDictionary(trailer)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, key := range []string{"/Info", "/ID"} {
		value, ok, err := trailerEntryValue(dict, key)
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

func trailerDictionary(trailer []byte) ([]byte, error) {
	start := bytes.Index(trailer, []byte("<<"))
	if start < 0 {
		return nil, fmt.Errorf("pdfsigning: trailer dictionary not found")
	}
	end, err := findDictionaryEnd(trailer[start:])
	if err != nil {
		return nil, err
	}
	return trailer[start : start+end], nil
}

func trailerEntryValue(dict []byte, key string) ([]byte, bool, error) {
	idx := findPDFName(dict, key)
	if idx < 0 {
		return nil, false, nil
	}
	valueStart := skipPDFSpaces(dict, idx+len(key))
	if valueStart >= len(dict) {
		return nil, false, fmt.Errorf("pdfsigning: trailer %s value missing", key)
	}
	valueEnd, err := pdfValueEnd(dict, valueStart)
	if err != nil {
		return nil, false, fmt.Errorf("pdfsigning: invalid trailer %s value: %w", key, err)
	}
	return bytes.TrimSpace(dict[valueStart:valueEnd]), true, nil
}

func pdfValueEnd(input []byte, start int) (int, error) {
	if start >= len(input) {
		return 0, fmt.Errorf("value missing")
	}
	switch input[start] {
	case '[':
		end, err := findArrayEnd(input[start:])
		if err != nil {
			return 0, err
		}
		return start + end + 1, nil
	case '<':
		if start+1 < len(input) && input[start+1] == '<' {
			end, err := findDictionaryEnd(input[start:])
			if err != nil {
				return 0, err
			}
			return start + end, nil
		}
		end := bytes.IndexByte(input[start+1:], '>')
		if end < 0 {
			return 0, fmt.Errorf("hex string end not found")
		}
		return start + 1 + end + 1, nil
	case '(':
		end, err := findStringEnd(input[start:])
		if err != nil {
			return 0, err
		}
		return start + end + 1, nil
	default:
		firstEnd := pdfTokenEnd(input, start)
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

func pdfTokenEnd(input []byte, start int) int {
	pos := start
	for pos < len(input) && !isPDFTokenEnd(input, pos) {
		pos++
	}
	return pos
}

func isPDFTokenEnd(input []byte, pos int) bool {
	if pos >= len(input) {
		return true
	}
	return input[pos] <= 0x20 || bytes.ContainsRune([]byte("()<>[]{}/%"), rune(input[pos]))
}

func parseXrefTables(input []byte, offset int) (map[int]int, error) {
	merged := make(map[int]int)
	seen := make(map[int]bool)
	for {
		if offset < 0 || offset >= len(input) {
			return nil, fmt.Errorf("pdfsigning: xref offset outside PDF")
		}
		if seen[offset] {
			return nil, fmt.Errorf("pdfsigning: cyclic xref /Prev chain")
		}
		seen[offset] = true
		offsets, err := parseXrefTable(input, offset)
		if err != nil {
			return nil, err
		}
		for object, objectOffset := range offsets {
			if _, exists := merged[object]; !exists {
				merged[object] = objectOffset
			}
		}
		trailer, err := xrefTrailer(input, offset)
		if err != nil {
			return nil, err
		}
		if findPDFName(trailer, "/Encrypt") >= 0 {
			return nil, fmt.Errorf("pdfsigning: encrypted PDFs are not supported")
		}
		prev, ok, err := parsePrevXref(trailer)
		if err != nil {
			return nil, err
		}
		if !ok {
			return merged, nil
		}
		offset = prev
	}
}

func xrefTrailer(input []byte, offset int) ([]byte, error) {
	if offset < 0 || offset >= len(input) {
		return nil, fmt.Errorf("pdfsigning: xref offset outside PDF")
	}
	rest := input[offset:]
	trailerStart := bytes.Index(rest, []byte("trailer"))
	if trailerStart < 0 {
		return nil, fmt.Errorf("pdfsigning: xref trailer not found")
	}
	trailerStart += offset
	afterTrailer := input[trailerStart:]
	startxref := bytes.Index(afterTrailer, []byte("startxref"))
	if startxref < 0 {
		return afterTrailer, nil
	}
	return afterTrailer[:startxref], nil
}

func parseXrefTable(input []byte, offset int) (map[int]int, error) {
	if !bytes.HasPrefix(input[offset:], []byte("xref")) {
		return nil, fmt.Errorf("pdfsigning: only classic xref tables are supported")
	}
	lines := bytes.Split(input[offset:], []byte{'\n'})
	offsets := make(map[int]int)
	for i := 1; i < len(lines); {
		line := strings.TrimSpace(string(lines[i]))
		i++
		if line == "" {
			continue
		}
		if line == "trailer" {
			break
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("pdfsigning: invalid xref subsection")
		}
		first, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		for n := 0; n < count; n++ {
			if i >= len(lines) {
				return nil, fmt.Errorf("pdfsigning: truncated xref subsection")
			}
			entry := string(lines[i])
			i++
			if len(entry) < 18 {
				return nil, fmt.Errorf("pdfsigning: invalid xref entry")
			}
			if entry[17] == 'n' {
				objectOffset, err := strconv.Atoi(strings.TrimSpace(entry[:10]))
				if err != nil {
					return nil, err
				}
				offsets[first+n] = objectOffset
			}
		}
	}
	return offsets, nil
}

func findFirstPage(input []byte, offsets map[int]int, ref pdfRef, depth int) (pdfRef, error) {
	if depth > 32 {
		return pdfRef{}, fmt.Errorf("pdfsigning: page tree is too deep")
	}
	offset, ok := offsets[ref.Object]
	if !ok {
		return pdfRef{}, fmt.Errorf("pdfsigning: object %d not found in xref", ref.Object)
	}
	dict, err := readObjectDict(input, ref, offset)
	if err != nil {
		return pdfRef{}, err
	}
	if bytes.Contains(dict, []byte("/Type /Page")) && !bytes.Contains(dict, []byte("/Type /Pages")) {
		return ref, nil
	}
	if pages, ok := findReference(dict, "/Pages"); ok {
		return findFirstPage(input, offsets, pages, depth+1)
	}
	if kid, ok := findFirstKid(dict); ok {
		return findFirstPage(input, offsets, kid, depth+1)
	}
	return pdfRef{}, fmt.Errorf("pdfsigning: first page not found")
}

func readObjectDict(input []byte, ref pdfRef, offset int) ([]byte, error) {
	if offset < 0 || offset >= len(input) {
		return nil, fmt.Errorf("pdfsigning: object offset outside PDF")
	}
	end := bytes.Index(input[offset:], []byte("endobj"))
	if end < 0 {
		return nil, fmt.Errorf("pdfsigning: object terminator not found")
	}
	object := input[offset : offset+end]
	if err := validateObjectHeader(object, ref); err != nil {
		return nil, err
	}
	start := bytes.Index(object, []byte("<<"))
	if start < 0 {
		return nil, fmt.Errorf("pdfsigning: object dictionary not found")
	}
	dictEnd, err := findDictionaryEnd(object[start:])
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), object[start:start+dictEnd]...), nil
}

func validateObjectHeader(object []byte, ref pdfRef) error {
	i := 0
	objectNumber, next, ok := parseLeadingInt(object, i)
	if !ok {
		return fmt.Errorf("pdfsigning: object header not found")
	}
	i = skipPDFSpaces(object, next)
	generation, next, ok := parseLeadingInt(object, i)
	if !ok {
		return fmt.Errorf("pdfsigning: object generation not found")
	}
	i = skipPDFSpaces(object, next)
	if !bytes.HasPrefix(object[i:], []byte("obj")) {
		return fmt.Errorf("pdfsigning: object header marker not found")
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
	value, err := strconv.Atoi(string(input[start:pos]))
	if err != nil {
		return 0, pos, false
	}
	return value, pos, true
}

func findDictionaryEnd(input []byte) (int, error) {
	depth := 0
	for i := 0; i < len(input)-1; i++ {
		switch {
		case input[i] == '<' && input[i+1] == '<':
			depth++
			i++
		case input[i] == '>' && input[i+1] == '>':
			depth--
			i++
			if depth == 0 {
				return i + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("pdfsigning: dictionary end not found")
}

func addDictEntry(dict []byte, key, value string) ([]byte, error) {
	if findPDFName(dict, key) >= 0 {
		return nil, fmt.Errorf("pdfsigning: existing %s dictionaries are not supported yet", key)
	}
	idx := bytes.LastIndex(dict, []byte(">>"))
	if idx < 0 {
		return nil, fmt.Errorf("pdfsigning: dictionary end not found")
	}
	out := make([]byte, 0, len(dict)+len(key)+len(value)+4)
	out = append(out, dict[:idx]...)
	out = append(out, ' ')
	out = append(out, key...)
	out = append(out, ' ')
	out = append(out, value...)
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
		return nil, fmt.Errorf("pdfsigning: referenced /Annots arrays are not supported yet")
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
	pattern := regexp.MustCompile(fmt.Sprintf(refPatternFormat, regexp.QuoteMeta(key)))
	match := pattern.FindSubmatch(dict)
	if len(match) != 3 {
		return pdfRef{}, false
	}
	obj, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return pdfRef{}, false
	}
	gen, err := strconv.Atoi(string(match[2]))
	if err != nil {
		return pdfRef{}, false
	}
	return pdfRef{Object: obj, Generation: gen}, true
}

func findFirstKid(dict []byte) (pdfRef, bool) {
	match := kidsPattern.FindSubmatch(dict)
	if len(match) != 3 {
		return pdfRef{}, false
	}
	obj, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return pdfRef{}, false
	}
	gen, err := strconv.Atoi(string(match[2]))
	if err != nil {
		return pdfRef{}, false
	}
	return pdfRef{Object: obj, Generation: gen}, true
}

func findArrayEnd(input []byte) (int, error) {
	if len(input) == 0 || input[0] != '[' {
		return 0, fmt.Errorf("pdfsigning: array start not found")
	}
	depth := 0
	inString := false
	stringDepth := 0
	inHex := false
	escaped := false
	for i := 0; i < len(input); i++ {
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
				return 0, fmt.Errorf("pdfsigning: invalid array nesting")
			}
		}
	}
	return 0, fmt.Errorf("pdfsigning: array end not found")
}

func findStringEnd(input []byte) (int, error) {
	if len(input) == 0 || input[0] != '(' {
		return 0, fmt.Errorf("pdfsigning: string start not found")
	}
	depth := 0
	escaped := false
	for i := 1; i < len(input); i++ {
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
	return 0, fmt.Errorf("pdfsigning: string end not found")
}

func findPDFName(input []byte, name string) int {
	start := 0
	needle := []byte(name)
	for {
		idx := bytes.Index(input[start:], needle)
		if idx < 0 {
			return -1
		}
		idx += start
		if isPDFNameBoundary(input, idx, len(needle)) {
			return idx
		}
		start = idx + len(needle)
	}
}

func findLastPDFName(input []byte, name string) int {
	end := len(input)
	needle := []byte(name)
	for {
		idx := bytes.LastIndex(input[:end], needle)
		if idx < 0 {
			return -1
		}
		if isPDFNameBoundary(input, idx, len(needle)) {
			return idx
		}
		end = idx
	}
}

func isPDFNameBoundary(input []byte, idx, length int) bool {
	if idx > 0 && isPDFNameChar(input[idx-1]) {
		return false
	}
	after := idx + length
	return after >= len(input) || !isPDFNameChar(input[after])
}

func isPDFNameChar(b byte) bool {
	return b > 0x20 && !bytes.ContainsRune([]byte("()<>[]{}/%"), rune(b))
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
