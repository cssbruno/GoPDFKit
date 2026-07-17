// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

const (
	maxPDFValueNesting = 128
	maxPDFDictEntries  = 10000
	maxPDFArrayItems   = 10000
)

// These names either introduce executable behavior, external navigation, or
// non-rendering data that should not cross a reconstruction boundary. Page
// content and ordinary rendering resources remain available.
var removedPDFKeys = map[string]bool{
	"A":             true,
	"AA":            true,
	"AcroForm":      true,
	"Collection":    true,
	"DSS":           true,
	"EF":            true,
	"EmbeddedFiles": true,
	"F":             true,
	"Filespec":      true,
	"GoToE":         true,
	"GoToR":         true,
	"JavaScript":    true,
	"JS":            true,
	"Launch":        true,
	"Metadata":      true,
	"Movie":         true,
	"Names":         true,
	"OpenAction":    true,
	"Perms":         true,
	"PieceInfo":     true,
	"Rendition":     true,
	"RichMedia":     true,
	"Screen":        true,
	"Sound":         true,
	"SubmitForm":    true,
	"Threads":       true,
	"URI":           true,
	"XFA":           true,
}

var removedActionNames = map[string]bool{
	"GoToE":      true,
	"GoToR":      true,
	"JavaScript": true,
	"Launch":     true,
	"Movie":      true,
	"Rendition":  true,
	"ResetForm":  true,
	"SubmitForm": true,
	"Thread":     true,
	"URI":        true,
}

var removedObjectTypes = map[string]bool{
	"Action":       true,
	"Annot":        true,
	"EmbeddedFile": true,
	"Filespec":     true,
	"Metadata":     true,
	"ObjStm":       true,
	"XRef":         true,
}

var rejectedExternalKeys = map[string]bool{
	"F":            true,
	"FDecodeParms": true,
	"FFilter":      true,
	"Ref":          true,
}

// Values of these resource dictionary entries are keyed by names chosen by the
// PDF producer. A rendering resource may legitimately be named /A, /F,
// /OpenAction, or any other name that has structural meaning elsewhere.
var resourceNameDictionaryKeys = map[string]bool{
	"CharProcs":  true,
	"Colorants":  true,
	"ColorSpace": true,
	"ExtGState":  true,
	"Font":       true,
	"Pattern":    true,
	"Properties": true,
	"Shading":    true,
	"XObject":    true,
}

type pdfValueScanner struct {
	data             []byte
	pos              int
	depth            int
	resourceNameRefs map[refKey]bool
	normalRefCounts  map[refKey]int
	rootLength       []byte
	rootLengthSeen   bool
}

type sanitizedPDFObject struct {
	value            []byte
	resourceNameRefs map[refKey]bool
	normalRefs       map[refKey]bool
}

func sanitizePDFObject(data []byte) ([]byte, error) {
	value, _, err := sanitizePDFObjectWithContext(data, false)
	return value, err
}

func sanitizePDFObjectWithContext(data []byte, preserveKeys bool) ([]byte, map[refKey]bool, error) {
	result, err := sanitizePDFObjectWithReferences(data, preserveKeys)
	return result.value, result.resourceNameRefs, err
}

func sanitizePDFObjectWithReferences(data []byte, preserveKeys bool) (sanitizedPDFObject, error) {
	if len(data) == 0 {
		return sanitizedPDFObject{value: []byte("null")}, nil
	}
	scanner := &pdfValueScanner{
		data:             data,
		resourceNameRefs: make(map[refKey]bool),
		normalRefCounts:  make(map[refKey]int),
	}
	value, end, drop, err := scanner.sanitizeValueWithDictionaryKeys(preserveKeys)
	if err != nil {
		return sanitizedPDFObject{}, err
	}
	if drop {
		return sanitizedPDFObject{value: []byte("null")}, nil
	}
	tail := &pdfValueScanner{data: data, pos: end}
	tail.skipSpace()
	if tail.pos < len(data) {
		// Preserve a validated stream suffix byte-for-byte. It is not a PDF
		// value and parsing it as one could mistake image or font bytes for
		// syntax, but arbitrary trailing bytes must never be accepted as a
		// stream implicitly.
		if err := validatePDFStreamSuffix(data, tail.pos, scanner.rootLength, scanner.rootLengthSeen); err != nil {
			return sanitizedPDFObject{}, err
		}
		value = append(value, data[end:]...)
	}
	// Classify references from the sanitized value rather than the hostile
	// source. References in entries that were removed above must not remain
	// reachable or create false role conflicts.
	classifier := &pdfValueScanner{
		data:             value,
		resourceNameRefs: make(map[refKey]bool),
		normalRefCounts:  make(map[refKey]int),
	}
	if _, _, drop, err := classifier.sanitizeValueWithDictionaryKeys(preserveKeys); err != nil {
		return sanitizedPDFObject{}, fmt.Errorf("classify sanitized PDF references: %w", err)
	} else if drop {
		return sanitizedPDFObject{}, errors.New("sanitized PDF value retained an active object")
	}
	normalRefs := make(map[refKey]bool, len(classifier.normalRefCounts))
	for ref, count := range classifier.normalRefCounts {
		if count > 0 {
			normalRefs[ref] = true
		}
	}
	return sanitizedPDFObject{
		value:            value,
		resourceNameRefs: classifier.resourceNameRefs,
		normalRefs:       normalRefs,
	}, nil
}

func (s *pdfValueScanner) sanitizeValue() ([]byte, int, bool, error) {
	return s.sanitizeValueWithDictionaryKeys(false)
}

func (s *pdfValueScanner) sanitizeValueWithDictionaryKeys(preserveKeys bool) ([]byte, int, bool, error) {
	s.skipSpace()
	start := s.pos
	if s.pos >= len(s.data) {
		return nil, s.pos, false, errors.New("unexpected end of PDF value")
	}
	switch s.data[s.pos] {
	case '<':
		if s.has("<<") {
			return s.sanitizeDict(preserveKeys)
		}
		end, err := s.hexStringEnd()
		return append([]byte(nil), s.data[start:end]...), end, false, err
	case '[':
		return s.sanitizeArray()
	case '(':
		end, err := s.literalStringEnd()
		return append([]byte(nil), s.data[start:end]...), end, false, err
	case '/':
		end := s.nameEnd()
		return append([]byte(nil), s.data[start:end]...), end, false, nil
	default:
		end := s.tokenEnd()
		if end == start {
			return nil, s.pos, false, fmt.Errorf("invalid PDF token at byte %d", start)
		}
		end = indirectReferenceEnd(s.data, start, end)
		s.pos = end
		value := append([]byte(nil), s.data[start:end]...)
		if ref, ok := singleIndirectRefKey(value); ok {
			s.normalRefCounts[ref]++
		}
		return value, end, false, nil
	}
}

func (s *pdfValueScanner) sanitizeDict(preserveKeys bool) ([]byte, int, bool, error) {
	s.pos += 2
	if s.depth >= maxPDFValueNesting {
		return nil, s.pos, false, errors.New("PDF value nesting exceeds maximum size")
	}
	s.depth++
	defer func() { s.depth-- }()
	var out bytes.Buffer
	out.WriteString("<<")
	dropDict := false
	entries := 0
	for {
		s.skipSpace()
		if s.has(">>") {
			s.pos += 2
			out.WriteString(" >>")
			return out.Bytes(), s.pos, dropDict, nil
		}
		if s.pos >= len(s.data) || s.data[s.pos] != '/' {
			return nil, s.pos, false, errors.New("unterminated PDF dictionary")
		}
		entries++
		if entries > maxPDFDictEntries {
			return nil, s.pos, false, errors.New("PDF dictionary exceeds maximum size")
		}
		keyTokenStart := s.pos
		keyStart := s.pos + 1
		keyEnd := s.nameEnd()
		key, err := canonicalPDFName(s.data[keyStart:keyEnd])
		if err != nil {
			return nil, s.pos, false, err
		}
		keyBytes := append([]byte(nil), s.data[keyTokenStart:keyEnd]...)
		s.pos = keyEnd
		valueStart := s.pos
		// Once a dictionary is known to contain producer-chosen resource names,
		// none of those names are structural even if one happens to be /Font,
		// /XObject, or another resource-category spelling.
		preserveChildKeys := !preserveKeys && resourceNameDictionaryKeys[key]
		value, _, valueDrop, err := s.sanitizeValueWithDictionaryKeys(preserveChildKeys)
		if err != nil {
			return nil, s.pos, false, fmt.Errorf("PDF dictionary key /%s: %w", key, err)
		}
		if s.depth == 1 && key == "Length" {
			if s.rootLengthSeen {
				return nil, s.pos, false, errors.New("PDF stream dictionary contains duplicate Length entries")
			}
			s.rootLengthSeen = true
			s.rootLength = append([]byte(nil), value...)
		}
		if preserveChildKeys {
			if ref, ok := singleIndirectRefKey(value); ok {
				s.resourceNameRefs[ref] = true
				if count := s.normalRefCounts[ref]; count <= 1 {
					delete(s.normalRefCounts, ref)
				} else {
					s.normalRefCounts[ref] = count - 1
				}
			}
		}
		valueIsAction, err := pdfValueIsName(s.data, valueStart, removedActionNames)
		if err != nil {
			return nil, s.pos, false, fmt.Errorf("PDF dictionary key /%s: %w", key, err)
		}
		if !preserveKeys && rejectedExternalKeys[key] {
			return nil, s.pos, false, fmt.Errorf("disallowed PDF external resource key /%s", key)
		}
		if !preserveKeys && key == "Subtype" {
			if _, indirect := singleIndirectRefKey(value); indirect {
				return nil, s.pos, false, errors.New("indirect PDF /Subtype values are not allowed")
			}
			postScript, err := pdfValueIsName(s.data, valueStart, map[string]bool{"PS": true})
			if err != nil {
				return nil, s.pos, false, fmt.Errorf("PDF dictionary key /Subtype: %w", err)
			}
			if postScript {
				return nil, s.pos, false, errors.New("disallowed PostScript XObject")
			}
		}
		if !preserveKeys && (key == "Type" || key == "S") {
			if _, indirect := singleIndirectRefKey(value); indirect {
				return nil, s.pos, false, fmt.Errorf("indirect PDF /%s values are not allowed", key)
			}
		}
		if !preserveKeys && (key == "Filter" || key == "DecodeParms") && len(indirectRefKeys(value)) > 0 {
			return nil, s.pos, false, fmt.Errorf("indirect PDF /%s values are not allowed", key)
		}
		if (!preserveKeys && removedPDFKeys[key]) || valueDrop || valueIsAction {
			if !preserveKeys && (key == "Type" || key == "S") {
				dropDict = true
			}
			continue
		}
		// /Type /Action is an action dictionary even when a producer uses a
		// non-standard action key that is not in removedPDFKeys.
		removedType, err := pdfValueIsName(s.data, valueStart, removedObjectTypes)
		if err != nil {
			return nil, s.pos, false, fmt.Errorf("PDF dictionary key /Type: %w", err)
		}
		if !preserveKeys && key == "Type" && removedType {
			dropDict = true
			continue
		}
		out.WriteByte(' ')
		out.Write(keyBytes)
		out.WriteByte(' ')
		out.Write(value)
	}
}

func (s *pdfValueScanner) sanitizeArray() ([]byte, int, bool, error) {
	s.pos++
	if s.depth >= maxPDFValueNesting {
		return nil, s.pos, false, errors.New("PDF value nesting exceeds maximum size")
	}
	s.depth++
	defer func() { s.depth-- }()
	var values [][]byte
	for {
		s.skipSpace()
		if s.pos >= len(s.data) {
			return nil, s.pos, false, errors.New("unterminated PDF array")
		}
		if s.data[s.pos] == ']' {
			s.pos++
			var out bytes.Buffer
			out.WriteByte('[')
			for _, value := range values {
				out.WriteByte(' ')
				out.Write(value)
			}
			if len(values) > 0 {
				out.WriteByte(' ')
			}
			out.WriteByte(']')
			return out.Bytes(), s.pos, false, nil
		}
		if len(values) >= maxPDFArrayItems {
			return nil, s.pos, false, errors.New("PDF array exceeds maximum size")
		}
		value, _, drop, err := s.sanitizeValue()
		if err != nil {
			return nil, s.pos, false, err
		}
		if !drop {
			values = append(values, value)
		}
	}
}

func (s *pdfValueScanner) skipSpace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\r', '\n', '\f', '\x00':
			s.pos++
		case '%':
			for s.pos < len(s.data) && s.data[s.pos] != '\r' && s.data[s.pos] != '\n' {
				s.pos++
			}
		default:
			return
		}
	}
}

func (s *pdfValueScanner) has(value string) bool {
	return s.pos+len(value) <= len(s.data) && string(s.data[s.pos:s.pos+len(value)]) == value
}

func (s *pdfValueScanner) nameEnd() int {
	if s.pos < len(s.data) && s.data[s.pos] == '/' {
		s.pos++
	}
	for s.pos < len(s.data) && !pdfDelimiter(s.data[s.pos]) && !pdfSpace(s.data[s.pos]) {
		s.pos++
	}
	return s.pos
}

func (s *pdfValueScanner) tokenEnd() int {
	for s.pos < len(s.data) && !pdfDelimiter(s.data[s.pos]) && !pdfSpace(s.data[s.pos]) {
		s.pos++
	}
	return s.pos
}

func (s *pdfValueScanner) literalStringEnd() (int, error) {
	depth := 0
	for s.pos < len(s.data) {
		ch := s.data[s.pos]
		s.pos++
		if ch == '\\' {
			if s.pos < len(s.data) {
				s.pos++
			}
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s.pos, nil
			}
		}
	}
	return s.pos, errors.New("unterminated PDF literal string")
}

func (s *pdfValueScanner) hexStringEnd() (int, error) {
	s.pos++
	for s.pos < len(s.data) {
		if s.data[s.pos] == '>' {
			s.pos++
			return s.pos, nil
		}
		s.pos++
	}
	return s.pos, errors.New("unterminated PDF hex string")
}

func pdfValueIsName(data []byte, start int, names map[string]bool) (bool, error) {
	s := &pdfValueScanner{data: data, pos: start}
	s.skipSpace()
	if s.pos >= len(data) || data[s.pos] != '/' {
		return false, nil
	}
	nameStart := s.pos + 1
	nameEnd := s.nameEnd()
	name, err := canonicalPDFName(data[nameStart:nameEnd])
	if err != nil {
		return false, err
	}
	return names[name], nil
}

func canonicalPDFName(raw []byte) (string, error) {
	decoded := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] != '#' {
			decoded = append(decoded, raw[i])
			continue
		}
		if i+2 >= len(raw) {
			return "", errors.New("PDF name contains an invalid escape")
		}
		hi, ok := pdfHexValue(raw[i+1])
		if !ok {
			return "", errors.New("PDF name contains an invalid escape")
		}
		lo, ok := pdfHexValue(raw[i+2])
		if !ok {
			return "", errors.New("PDF name contains an invalid escape")
		}
		decoded = append(decoded, hi<<4|lo)
		i += 2
	}
	return string(decoded), nil
}

func pdfHexValue(ch byte) (byte, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch >= 'A' && ch <= 'F':
		return ch - 'A' + 10, true
	case ch >= 'a' && ch <= 'f':
		return ch - 'a' + 10, true
	default:
		return 0, false
	}
}

func validatePDFStreamSuffix(data []byte, pos int, lengthValue []byte, lengthSeen bool) error {
	if pos < 0 || pos > len(data)-len("stream") || string(data[pos:pos+len("stream")]) != "stream" {
		return errors.New("unexpected bytes after PDF value")
	}
	pos += len("stream")
	if pos+1 < len(data) && data[pos] == '\r' && data[pos+1] == '\n' {
		pos += 2
	} else if pos < len(data) && (data[pos] == '\r' || data[pos] == '\n') {
		pos++
	} else {
		return errors.New("PDF stream marker is not followed by a line break")
	}
	if !lengthSeen {
		return errors.New("PDF stream dictionary does not contain Length")
	}
	length, direct, err := directPDFStreamLength(lengthValue)
	if err != nil {
		return err
	}
	var end int
	if direct {
		if length > len(data)-pos {
			return errors.New("PDF stream length exceeds object bounds")
		}
		end = skipPDFLineBreaks(data, pos+length)
		if !hasPDFToken(data, end, "endstream") {
			return errors.New("PDF stream length does not end at endstream")
		}
	} else {
		end = firstPDFEndstream(data, pos)
	}
	if end < 0 {
		return errors.New("PDF stream is missing endstream")
	}
	after := end + len("endstream")
	tail := &pdfValueScanner{data: data, pos: after}
	tail.skipSpace()
	if tail.pos != len(data) {
		return errors.New("unexpected bytes after PDF endstream")
	}
	return nil
}

func directPDFStreamLength(value []byte) (int, bool, error) {
	if _, indirect := singleIndirectRefKey(value); indirect {
		return 0, false, nil
	}
	value = bytes.TrimSpace(value)
	if len(value) == 0 {
		return 0, false, errors.New("PDF stream Length is not a nonnegative integer or indirect reference")
	}
	if value[0] == '+' {
		value = value[1:]
	}
	if len(value) == 0 {
		return 0, false, errors.New("PDF stream Length is invalid")
	}
	maximum := int(^uint(0) >> 1)
	length := 0
	for _, ch := range value {
		if !pdfDigit(ch) {
			return 0, false, errors.New("PDF stream Length is not a nonnegative integer or indirect reference")
		}
		digit := int(ch - '0')
		if length > (maximum-digit)/10 {
			return 0, false, errors.New("PDF stream Length is invalid")
		}
		length = length*10 + digit
	}
	return length, true, nil
}

func skipPDFLineBreaks(data []byte, pos int) int {
	for pos < len(data) {
		switch data[pos] {
		case '\r':
			pos++
			if pos < len(data) && data[pos] == '\n' {
				pos++
			}
		case '\n':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func firstPDFEndstream(data []byte, start int) int {
	for pos := start; pos <= len(data)-len("endstream"); {
		relative := bytes.Index(data[pos:], []byte("endstream"))
		if relative < 0 {
			return -1
		}
		pos += relative
		if hasPDFToken(data, pos, "endstream") && (pos == start || pdfSpace(data[pos-1])) {
			return pos
		}
		pos++
	}
	return -1
}

type refKey struct {
	objectNumber int
	generation   int
}

func indirectRefKeys(data []byte) []refKey {
	refs := make(map[refKey]bool)
	for pos := 0; pos < len(data); {
		switch data[pos] {
		case '(':
			s := &pdfValueScanner{data: data, pos: pos}
			end, err := s.literalStringEnd()
			if err != nil {
				return refKeys(refs)
			}
			pos = end
			continue
		case '<':
			if pos+1 < len(data) && data[pos+1] == '<' {
				pos += 2
				continue
			}
			if pos > 0 && data[pos-1] == '<' {
				pos++
				continue
			}
			if pos+1 < len(data) && data[pos+1] != '<' {
				s := &pdfValueScanner{data: data, pos: pos}
				end, err := s.hexStringEnd()
				if err != nil {
					return refKeys(refs)
				}
				pos = end
				continue
			}
		case '%':
			for pos < len(data) && data[pos] != '\r' && data[pos] != '\n' {
				pos++
			}
			continue
		}
		if !pdfDigit(data[pos]) && data[pos] != '+' && data[pos] != '-' {
			if hasPDFStreamKeyword(data, pos) {
				break
			}
			pos++
			continue
		}
		firstStart := pos
		firstEnd := pdfNumberEnd(data, pos)
		if firstEnd == firstStart || !pdfInteger(data[firstStart:firstEnd]) {
			pos++
			continue
		}
		pos = firstEnd
		for pos < len(data) && pdfSpace(data[pos]) {
			pos++
		}
		secondStart := pos
		secondEnd := pdfNumberEnd(data, pos)
		if secondEnd == secondStart || !pdfInteger(data[secondStart:secondEnd]) {
			pos = firstEnd
			continue
		}
		pos = secondEnd
		for pos < len(data) && pdfSpace(data[pos]) {
			pos++
		}
		if pos < len(data) && data[pos] == 'R' && (pos+1 == len(data) || pdfDelimiter(data[pos+1]) || pdfSpace(data[pos+1])) {
			objectNumber, objectErr := strconv.Atoi(string(data[firstStart:firstEnd]))
			generation, generationErr := strconv.Atoi(string(data[secondStart:secondEnd]))
			if objectErr != nil || generationErr != nil {
				pos = firstEnd
				continue
			}
			refs[refKey{objectNumber: objectNumber, generation: generation}] = true
			pos++
		} else {
			pos = firstEnd
		}
	}
	return refKeys(refs)
}

func singleIndirectRefKey(data []byte) (refKey, bool) {
	scanner := &pdfValueScanner{data: bytes.TrimSpace(data)}
	scanner.skipSpace()
	start := scanner.pos
	if start >= len(scanner.data) || (!pdfDigit(scanner.data[start]) && scanner.data[start] != '+' && scanner.data[start] != '-') {
		return refKey{}, false
	}
	firstEnd := scanner.tokenEnd()
	end := indirectReferenceEnd(scanner.data, start, firstEnd)
	if end == firstEnd {
		return refKey{}, false
	}
	for end < len(scanner.data) && pdfSpace(scanner.data[end]) {
		end++
	}
	if end != len(scanner.data) {
		return refKey{}, false
	}
	first, err := strconv.Atoi(string(scanner.data[start:firstEnd]))
	if err != nil {
		return refKey{}, false
	}
	secondStart := firstEnd
	for secondStart < len(scanner.data) && pdfSpace(scanner.data[secondStart]) {
		secondStart++
	}
	secondEnd := pdfNumberEnd(scanner.data, secondStart)
	second, err := strconv.Atoi(string(scanner.data[secondStart:secondEnd]))
	if err != nil {
		return refKey{}, false
	}
	return refKey{objectNumber: first, generation: second}, true
}

func refKeys(refs map[refKey]bool) []refKey {
	out := make([]refKey, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	return out
}

func pdfDelimiter(ch byte) bool {
	switch ch {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}

func pdfSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n', '\f', '\x00':
		return true
	default:
		return false
	}
}

func pdfDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

func pdfNumberEnd(data []byte, pos int) int {
	if pos < len(data) && (data[pos] == '+' || data[pos] == '-') {
		pos++
	}
	for pos < len(data) && ((data[pos] >= '0' && data[pos] <= '9') || data[pos] == '.') {
		pos++
	}
	return pos
}

func indirectReferenceEnd(data []byte, firstStart, firstEnd int) int {
	if !pdfInteger(data[firstStart:firstEnd]) {
		return firstEnd
	}
	pos := firstEnd
	for pos < len(data) && pdfSpace(data[pos]) {
		pos++
	}
	secondStart := pos
	secondEnd := pdfNumberEnd(data, pos)
	if secondEnd == secondStart || !pdfInteger(data[secondStart:secondEnd]) {
		return firstEnd
	}
	pos = secondEnd
	for pos < len(data) && pdfSpace(data[pos]) {
		pos++
	}
	if pos < len(data) && data[pos] == 'R' && (pos+1 == len(data) || pdfDelimiter(data[pos+1]) || pdfSpace(data[pos+1])) {
		return pos + 1
	}
	return firstEnd
}

func pdfInteger(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	start := 0
	if data[0] == '+' || data[0] == '-' {
		start++
	}
	if start == len(data) {
		return false
	}
	for _, ch := range data[start:] {
		if !pdfDigit(byte(ch)) {
			return false
		}
	}
	return true
}

func hasPDFToken(data []byte, pos int, token string) bool {
	if pos+len(token) > len(data) || string(data[pos:pos+len(token)]) != token {
		return false
	}
	if pos > 0 && !pdfSpace(data[pos-1]) && !pdfDelimiter(data[pos-1]) {
		return false
	}
	end := pos + len(token)
	return end == len(data) || pdfSpace(data[end]) || pdfDelimiter(data[end])
}

func hasPDFStreamKeyword(data []byte, pos int) bool {
	if !hasPDFToken(data, pos, "stream") ||
		(pos > 0 && !pdfSpace(data[pos-1]) && data[pos-1] != '>') {
		return false
	}
	end := pos + len("stream")
	return end < len(data) && (data[end] == '\r' || data[end] == '\n')
}
