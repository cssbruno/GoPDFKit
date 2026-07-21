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

type effectiveXrefEntry struct {
	Offset     int
	Generation int
	InUse      bool
}

type cachedIndirectValue struct {
	data  []byte
	start int
}

type pdfXrefResolver struct {
	input          []byte
	entries        map[int]effectiveXrefEntry
	indirectValues map[pdfRef]cachedIndirectValue
	maxObject      int
}

func analyzePDF(input []byte) (pdfContext, error) {
	return analyzePDFContext(context.Background(), input, DefaultMaxXrefChainLength, DefaultMaxXrefEntries)
}

func analyzePDFContext(ctx context.Context, input []byte, maxXrefChain, maxXrefEntries int) (pdfContext, error) {
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
	if _, encrypted, err := findDictionaryEntryContext(ctx, trailer, "/Encrypt"); err != nil {
		return pdfContext{}, err
	} else if encrypted {
		return pdfContext{}, fmt.Errorf("%w: encrypted PDFs are not supported", ErrUnsupportedPDF)
	}
	size, root, err := parseTrailerContext(ctx, trailer, maxXrefEntries, 3)
	if err != nil {
		return pdfContext{}, err
	}
	xref, err := newPDFXrefResolverContext(ctx, input, previousXref, maxXrefChain, maxXrefEntries)
	if err != nil {
		return pdfContext{}, err
	}
	if size <= xref.maxObject {
		return pdfContext{}, fmt.Errorf("%w: trailer /Size %d does not cover xref object %d", ErrUnsupportedPDF, size, xref.maxObject)
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

func newPDFXrefResolverContext(ctx context.Context, input []byte, offset, maxXrefChain, maxXrefEntries int) (*pdfXrefResolver, error) {
	if maxXrefChain <= 0 {
		return nil, errors.New("pdfsigning: xref chain limit must be positive")
	}
	if maxXrefEntries <= 0 || maxXrefEntries > maxPDFObjectCount {
		return nil, errors.New("pdfsigning: xref entry limit must be positive")
	}
	resolver := &pdfXrefResolver{input: input, entries: make(map[int]effectiveXrefEntry), maxObject: -1}
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
		if len(seen) >= maxXrefChain {
			return nil, errors.New("pdfsigning: xref chain exceeds maximum size")
		}
		seen[offset] = true
		trailer, err := xrefTrailerContext(ctx, input, offset)
		if err != nil {
			return nil, err
		}
		if _, encrypted, err := findDictionaryEntryContext(ctx, trailer, "/Encrypt"); err != nil {
			return nil, err
		} else if encrypted {
			return nil, fmt.Errorf("%w: encrypted PDFs are not supported", ErrUnsupportedPDF)
		}
		tableMax, err := resolver.loadXrefTableContext(ctx, input, offset, maxXrefEntries)
		if err != nil {
			return nil, err
		}
		resolver.maxObject = max(resolver.maxObject, tableMax)
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

func (resolver *pdfXrefResolver) objectOffsetContext(ctx context.Context, ref pdfRef) (int, error) {
	if resolver == nil {
		return 0, errors.New("pdfsigning: xref resolver not initialized")
	}
	if err := signContextErr(ctx); err != nil {
		return 0, err
	}
	entry, present := resolver.entries[ref.Object]
	if !present {
		return 0, fmt.Errorf("pdfsigning: object %d %d not found in xref", ref.Object, ref.Generation)
	}
	if !entry.InUse || entry.Generation != ref.Generation {
		return 0, fmt.Errorf("pdfsigning: object %d %d is free or has a different generation", ref.Object, ref.Generation)
	}
	return entry.Offset, nil
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

func parseTrailerContext(ctx context.Context, trailer []byte, maxXrefEntries, reservedObjects int) (int, pdfRef, error) {
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
	size, ok := parseExactNonnegativeInt(sizeValue)
	if !ok {
		return 0, pdfRef{}, errors.New("pdfsigning: invalid trailer /Size")
	}
	if reservedObjects < 0 || maxXrefEntries <= reservedObjects || maxPDFObjectCount <= reservedObjects ||
		size <= 0 || size > maxXrefEntries-reservedObjects || size > maxPDFObjectCount-reservedObjects {
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
	prev, ok := parseExactNonnegativeInt(prevValue)
	if !ok {
		return 0, false, errors.New("pdfsigning: invalid trailer /Prev")
	}
	return prev, true, nil
}

func parseExactNonnegativeInt(value []byte) (int, bool) {
	pos := skipPDFSpaces(value, 0)
	parsed, end, ok := parseLeadingInt(value, pos)
	return parsed, ok && skipPDFSpaces(value, end) == len(value)
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
	start := skipPDFSpaces(trailer, 0)
	if hasPDFKeywordAt(trailer, start, "trailer") {
		start = skipPDFSpaces(trailer, start+len("trailer"))
	}
	if start+1 >= len(trailer) || trailer[start] != '<' || trailer[start+1] != '<' {
		return nil, errors.New("pdfsigning: trailer dictionary not found")
	}
	end, err := findDictionaryEndContext(ctx, trailer[start:])
	if err != nil {
		return nil, err
	}
	return trailer[start : start+end], nil
}

func trailerEntryValueContext(ctx context.Context, dict []byte, key string) ([]byte, bool, error) {
	entry, ok, err := findDictionaryEntryContext(ctx, dict, key)
	if err != nil {
		return nil, false, fmt.Errorf("pdfsigning: invalid trailer %s value: %w", key, err)
	}
	if !ok {
		return nil, false, nil
	}
	return bytes.TrimSpace(dict[entry.ValueStart:entry.ValueEnd]), true, nil
}

type pdfDictionaryEntry struct {
	KeyStart   int
	KeyEnd     int
	ValueStart int
	ValueEnd   int
}

// findDictionaryEntryContext finds a direct entry in the outer dictionary.
// It deliberately does not match names in nested dictionaries, strings, hex
// strings, or comments. Duplicate keys are rejected because selecting one of
// two security-sensitive values would create parser ambiguity.
func findDictionaryEntryContext(ctx context.Context, input []byte, key string) (pdfDictionaryEntry, bool, error) {
	if len(key) < 2 || key[0] != '/' {
		return pdfDictionaryEntry{}, false, errors.New("pdfsigning: invalid dictionary key")
	}
	pos := skipPDFSpaces(input, 0)
	if hasPDFKeywordAt(input, pos, "trailer") {
		pos = skipPDFSpaces(input, pos+len("trailer"))
	}
	if pos+1 >= len(input) || input[pos] != '<' || input[pos+1] != '<' {
		return pdfDictionaryEntry{}, false, errors.New("pdfsigning: dictionary start not found")
	}
	dictEnd, err := findDictionaryEndContext(ctx, input[pos:])
	if err != nil {
		return pdfDictionaryEntry{}, false, err
	}
	limit := pos + dictEnd - 2
	pos += 2
	var result pdfDictionaryEntry
	found := false
	for {
		if err := signContextErr(ctx); err != nil {
			return pdfDictionaryEntry{}, false, err
		}
		pos = skipPDFSpaces(input, pos)
		if pos >= limit {
			break
		}
		if input[pos] != '/' {
			return pdfDictionaryEntry{}, false, errors.New("pdfsigning: dictionary key is not a name")
		}
		decodedKey, keyEnd, err := decodePDFNameToken(input, pos)
		if err != nil {
			return pdfDictionaryEntry{}, false, err
		}
		valueStart := skipPDFSpaces(input, keyEnd)
		if valueStart >= limit {
			return pdfDictionaryEntry{}, false, fmt.Errorf("pdfsigning: dictionary %s value missing", decodedKey)
		}
		valueEnd, err := pdfValueEndContext(ctx, input, valueStart)
		if err != nil {
			return pdfDictionaryEntry{}, false, fmt.Errorf("pdfsigning: dictionary %s value: %w", decodedKey, err)
		}
		if valueEnd > limit {
			return pdfDictionaryEntry{}, false, fmt.Errorf("pdfsigning: dictionary %s value exceeds dictionary", decodedKey)
		}
		if decodedKey == key {
			if found {
				return pdfDictionaryEntry{}, false, fmt.Errorf("pdfsigning: duplicate dictionary key %s", key)
			}
			result = pdfDictionaryEntry{KeyStart: pos, KeyEnd: keyEnd, ValueStart: valueStart, ValueEnd: valueEnd}
			found = true
		}
		pos = valueEnd
	}
	return result, found, nil
}

func decodePDFNameToken(input []byte, start int) (string, int, error) {
	if start < 0 || start >= len(input) || input[start] != '/' {
		return "", start, errors.New("pdfsigning: PDF name start not found")
	}
	decoded := make([]byte, 1, 16)
	decoded[0] = '/'
	pos := start + 1
	for pos < len(input) && isPDFNameChar(input[pos]) {
		value := input[pos]
		if value == '#' {
			if pos+2 >= len(input) {
				return "", start, errors.New("pdfsigning: truncated PDF name escape")
			}
			high, highOK := pdfHexDigit(input[pos+1])
			low, lowOK := pdfHexDigit(input[pos+2])
			if !highOK || !lowOK {
				return "", start, errors.New("pdfsigning: invalid PDF name escape")
			}
			value = high<<4 | low
			pos += 3
		} else {
			pos++
		}
		decoded = append(decoded, value)
	}
	return string(decoded), pos, nil
}

func pdfValueEndContext(ctx context.Context, input []byte, start int) (int, error) {
	if err := signContextErr(ctx); err != nil {
		return 0, err
	}
	if start >= len(input) {
		return 0, errors.New("value missing")
	}
	switch input[start] {
	case '/':
		_, end, err := decodePDFNameToken(input, start)
		return end, err
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
	if !bytes.HasPrefix(input[offset:], []byte("xref")) {
		return nil, fmt.Errorf("%w: only classic xref tables are supported", ErrUnsupportedPDF)
	}
	for pos := offset + len("xref"); pos < len(input); {
		if err := signContextErr(ctx); err != nil {
			return nil, err
		}
		line, next := nextPDFLine(input, pos)
		pos = next
		if !bytes.Equal(bytes.TrimSpace(line), []byte("trailer")) {
			continue
		}
		dict, err := trailerDictionaryContext(ctx, input[pos:])
		if err != nil {
			return nil, err
		}
		return dict, nil
	}
	return nil, errors.New("pdfsigning: xref trailer not found")
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
	return parseXrefTableContext(context.Background(), input, offset, DefaultMaxXrefEntries)
}

func (resolver *pdfXrefResolver) loadXrefTableContext(ctx context.Context, input []byte, offset, maxXrefEntries int) (int, error) {
	if offset < 0 || offset >= len(input) {
		return -1, errors.New("pdfsigning: xref offset outside PDF")
	}
	if !bytes.HasPrefix(input[offset:], []byte("xref")) {
		return -1, fmt.Errorf("%w: only classic xref tables are supported", ErrUnsupportedPDF)
	}
	maximum := -1
	totalEntries := 0
	seenObjects := make(map[int]struct{})
	for pos := offset + len("xref"); pos < len(input); {
		if err := signContextErr(ctx); err != nil {
			return -1, err
		}
		line, next := nextPDFLine(input, pos)
		pos = next
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.Equal(line, []byte("trailer")) {
			return maximum, nil
		}
		first, nextInt, ok := parseLeadingInt(line, 0)
		if !ok {
			return -1, errors.New("pdfsigning: invalid xref subsection")
		}
		count, end, ok := parseLeadingInt(line, skipPDFSpaces(line, nextInt))
		if !ok || skipPDFSpaces(line, end) != len(line) {
			return -1, errors.New("pdfsigning: invalid xref subsection")
		}
		if err := validateXrefSubsection(first, count, &totalEntries, maxXrefEntries); err != nil {
			return -1, err
		}
		if count > 0 {
			maximum = max(maximum, first+count-1)
		}
		for n := 0; n < count; n++ {
			if pos >= len(input) {
				return -1, errors.New("pdfsigning: truncated xref subsection")
			}
			entry, next := nextPDFLine(input, pos)
			pos = next
			objectOffset, generation, status, ok := parseXrefEntry(entry)
			if !ok {
				return -1, errors.New("pdfsigning: invalid xref entry")
			}
			objectNumber := first + n
			if _, duplicate := seenObjects[objectNumber]; duplicate {
				return -1, fmt.Errorf("pdfsigning: duplicate xref entry for object %d", objectNumber)
			}
			seenObjects[objectNumber] = struct{}{}
			if _, resolvedByNewerRevision := resolver.entries[objectNumber]; !resolvedByNewerRevision {
				resolver.entries[objectNumber] = effectiveXrefEntry{
					Offset:     objectOffset,
					Generation: generation,
					InUse:      status == 'n',
				}
			}
		}
	}
	return -1, errors.New("pdfsigning: xref trailer not found")
}

func parseXrefTableContext(ctx context.Context, input []byte, offset, maxXrefEntries int) (map[int]int, error) {
	if offset < 0 || offset >= len(input) {
		return nil, errors.New("pdfsigning: xref offset outside PDF")
	}
	if !bytes.HasPrefix(input[offset:], []byte("xref")) {
		return nil, fmt.Errorf("%w: only classic xref tables are supported", ErrUnsupportedPDF)
	}
	var offsets map[int]int
	seenObjects := make(map[int]struct{})
	totalEntries := 0
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
		count, end, ok := parseLeadingInt(line, skipPDFSpaces(line, nextInt))
		if !ok || skipPDFSpaces(line, end) != len(line) {
			return nil, errors.New("pdfsigning: invalid xref subsection")
		}
		if err := validateXrefSubsection(first, count, &totalEntries, maxXrefEntries); err != nil {
			return nil, err
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
			objectNumber := first + n
			if _, duplicate := seenObjects[objectNumber]; duplicate {
				return nil, fmt.Errorf("pdfsigning: duplicate xref entry for object %d", objectNumber)
			}
			seenObjects[objectNumber] = struct{}{}
			objectOffset, _, status, ok := parseXrefEntry(entry)
			if !ok {
				return nil, errors.New("pdfsigning: invalid xref entry")
			}
			if status == 'n' {
				offsets[objectNumber] = objectOffset
			}
		}
	}
	if offsets == nil {
		offsets = make(map[int]int)
	}
	return offsets, nil
}

func validateXrefSubsection(first, count int, total *int, maxEntries int) error {
	if first < 0 || count < 0 || maxEntries <= 0 || first >= maxEntries || count > maxEntries-first || *total > maxEntries-count {
		return errors.New("pdfsigning: xref entries exceed maximum size")
	}
	*total += count
	return nil
}

func parseXrefEntry(entry []byte) (int, int, byte, bool) {
	offset, next, ok := parseLeadingInt(entry, 0)
	if !ok {
		return 0, 0, 0, false
	}
	generation, next, ok := parseLeadingInt(entry, skipPDFSpaces(entry, next))
	if !ok || generation > 65535 {
		return 0, 0, 0, false
	}
	statusPos := skipPDFSpaces(entry, next)
	if statusPos >= len(entry) || (entry[statusPos] != 'n' && entry[statusPos] != 'f') || skipPDFSpaces(entry, statusPos+1) != len(entry) {
		return 0, 0, 0, false
	}
	return offset, generation, entry[statusPos], true
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
	offset, err := xref.objectOffsetContext(ctx, ref)
	if err != nil {
		return pdfRef{}, err
	}
	dict, err := readObjectDictContext(ctx, input, ref, offset)
	if err != nil {
		return pdfRef{}, err
	}
	typeEntry, hasType, err := findDictionaryEntryContext(ctx, dict, "/Type")
	if err != nil {
		return pdfRef{}, err
	}
	if hasType && pdfNameValueEquals(dict[typeEntry.ValueStart:typeEntry.ValueEnd], "/Page") {
		return ref, nil
	}
	if pages, ok, err := findReferenceContext(ctx, dict, "/Pages"); err != nil {
		return pdfRef{}, err
	} else if ok {
		return findFirstPageContext(ctx, input, xref, pages, depth+1)
	}
	if kid, ok, err := findFirstKidContext(ctx, dict); err != nil {
		return pdfRef{}, err
	} else if ok {
		return findFirstPageContext(ctx, input, xref, kid, depth+1)
	}
	return pdfRef{}, errors.New("pdfsigning: first page not found")
}

func readObjectDict(input []byte, ref pdfRef, offset int) ([]byte, error) {
	return readObjectDictContext(context.Background(), input, ref, offset)
}

func readObjectDictContext(ctx context.Context, input []byte, ref pdfRef, offset int) ([]byte, error) {
	dict, _, err := readObjectDictPositionContext(ctx, input, ref, offset)
	return dict, err
}

func readObjectDictPositionContext(ctx context.Context, input []byte, ref pdfRef, offset int) ([]byte, int, error) {
	if err := signContextErr(ctx); err != nil {
		return nil, 0, err
	}
	if offset < 0 || offset >= len(input) {
		return nil, 0, errors.New("pdfsigning: object offset outside PDF")
	}
	objectNumber, next, ok := parseLeadingInt(input, offset)
	if !ok {
		return nil, 0, errors.New("pdfsigning: object header not found")
	}
	pos := skipPDFSpaces(input, next)
	generation, next, ok := parseLeadingInt(input, pos)
	if !ok {
		return nil, 0, errors.New("pdfsigning: object generation not found")
	}
	pos = skipPDFSpaces(input, next)
	if !hasPDFKeywordAt(input, pos, "obj") {
		return nil, 0, errors.New("pdfsigning: object header marker not found")
	}
	if objectNumber != ref.Object || generation != ref.Generation {
		return nil, 0, fmt.Errorf("pdfsigning: xref points to object %d %d, want %d %d", objectNumber, generation, ref.Object, ref.Generation)
	}
	start := skipPDFSpaces(input, pos+len("obj"))
	if start+1 >= len(input) || input[start] != '<' || input[start+1] != '<' {
		return nil, 0, errors.New("pdfsigning: object dictionary not found")
	}
	dictEnd, err := findDictionaryEndContext(ctx, input[start:])
	if err != nil {
		return nil, 0, err
	}
	endobj := skipPDFSpaces(input, start+dictEnd)
	if !hasPDFKeywordAt(input, endobj, "endobj") {
		return nil, 0, errors.New("pdfsigning: object terminator not found after dictionary")
	}
	return input[start : start+dictEnd], start, nil
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
		_, exists, err := findDictionaryEntryContext(context.Background(), dict, entry.key)
		if err != nil {
			return nil, err
		}
		if exists {
			if entry.skipExisting {
				continue
			}
			return nil, fmt.Errorf("%w: existing %s dictionaries are not supported", ErrUnsupportedPDF, entry.key)
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
	entry, exists, err := findDictionaryEntryContext(context.Background(), dict, "/Annots")
	if err != nil {
		return nil, err
	}
	if !exists {
		return addDictEntry(dict, "/Annots", "["+annotationRef+"]")
	}
	arrayStart := entry.ValueStart
	if arrayStart >= len(dict) || dict[arrayStart] != '[' {
		return nil, fmt.Errorf("%w: referenced /Annots arrays are not supported", ErrUnsupportedPDF)
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
	entry, exists, err := findDictionaryEntryContext(ctx, dict, key)
	if err != nil {
		return pdfRef{}, false, err
	}
	if !exists {
		return pdfRef{}, false, nil
	}
	valueStart := entry.ValueStart
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
	ref, ok, _ := findFirstKidContext(context.Background(), dict)
	return ref, ok
}

func findFirstKidContext(ctx context.Context, dict []byte) (pdfRef, bool, error) {
	entry, exists, err := findDictionaryEntryContext(ctx, dict, "/Kids")
	if err != nil {
		return pdfRef{}, false, err
	}
	if !exists {
		return pdfRef{}, false, nil
	}
	pos := entry.ValueStart
	if pos >= len(dict) || dict[pos] != '[' {
		return pdfRef{}, false, nil
	}
	pos = skipPDFSpaces(dict, pos+1)
	obj, next, ok := parseLeadingInt(dict, pos)
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
	inComment := false
	escaped := false
	for i := 0; i < len(input); i++ {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return 0, err
			}
		}
		b := input[i]
		if inComment {
			if b == '\r' || b == '\n' {
				inComment = false
			}
			continue
		}
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
		case '%':
			inComment = true
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

func matchPDFNameToken(input []byte, name string) (int, bool) {
	if len(input) == 0 || input[0] != '/' || len(name) < 2 || name[0] != '/' {
		return 0, false
	}
	rawPos := 1
	namePos := 1
	for rawPos < len(input) && isPDFNameChar(input[rawPos]) {
		decoded := input[rawPos]
		if decoded == '#' {
			if rawPos+2 >= len(input) {
				return 0, false
			}
			high, highOK := pdfHexDigit(input[rawPos+1])
			low, lowOK := pdfHexDigit(input[rawPos+2])
			if !highOK || !lowOK {
				return 0, false
			}
			decoded = high<<4 | low
			rawPos += 3
		} else {
			rawPos++
		}
		if namePos >= len(name) || decoded != name[namePos] {
			return 0, false
		}
		namePos++
	}
	return rawPos, namePos == len(name)
}

func pdfHexDigit(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func isPDFNameChar(b byte) bool {
	return b > 0x20 && !pdfDelimiter[b]
}

func pdfNameValueEquals(value []byte, name string) bool {
	valueStart := skipPDFSpaces(value, 0)
	length, ok := matchPDFNameToken(value[valueStart:], name)
	return ok && skipPDFSpaces(value, valueStart+length) == len(value)
}

func hasPDFKeywordAt(input []byte, pos int, keyword string) bool {
	if pos < 0 || pos+len(keyword) > len(input) || !bytes.Equal(input[pos:pos+len(keyword)], []byte(keyword)) {
		return false
	}
	if pos > 0 && !isPDFTokenEnd(input, pos-1) {
		return false
	}
	return isPDFTokenEnd(input, pos+len(keyword))
}

func skipPDFSpaces(input []byte, pos int) int {
	for pos < len(input) {
		switch input[pos] {
		case 0, '\t', '\n', '\f', '\r', ' ':
			pos++
		case '%':
			for pos < len(input) && input[pos] != '\r' && input[pos] != '\n' {
				pos++
			}
		default:
			return pos
		}
	}
	return pos
}
