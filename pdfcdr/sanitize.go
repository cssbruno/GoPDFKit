// SPDX-License-Identifier: MIT
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

type pdfValueScanner struct {
	data  []byte
	pos   int
	depth int
}

func sanitizePDFObject(data []byte) ([]byte, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return []byte("null"), nil
	}
	scanner := &pdfValueScanner{data: data}
	value, end, drop, err := scanner.sanitizeValue()
	if err != nil {
		return nil, err
	}
	if drop {
		return []byte("null"), nil
	}
	// Preserve a stream body byte-for-byte. It is not a PDF value and parsing
	// it as one could mistake arbitrary image or font bytes for syntax.
	value = append(value, data[end:]...)
	return value, nil
}

func (s *pdfValueScanner) sanitizeValue() ([]byte, int, bool, error) {
	s.skipSpace()
	start := s.pos
	if s.pos >= len(s.data) {
		return nil, s.pos, false, errors.New("unexpected end of PDF value")
	}
	switch s.data[s.pos] {
	case '<':
		if s.has("<<") {
			return s.sanitizeDict()
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
		return append([]byte(nil), s.data[start:end]...), end, false, nil
	}
}

func (s *pdfValueScanner) sanitizeDict() ([]byte, int, bool, error) {
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
		key := string(s.data[keyStart:keyEnd])
		keyBytes := append([]byte(nil), s.data[keyTokenStart:keyEnd]...)
		s.pos = keyEnd
		valueStart := s.pos
		value, _, valueDrop, err := s.sanitizeValue()
		if err != nil {
			return nil, s.pos, false, fmt.Errorf("PDF dictionary key /%s: %w", key, err)
		}
		valueIsAction := pdfValueIsName(s.data, valueStart, removedActionNames)
		if removedPDFKeys[key] || valueDrop || valueIsAction {
			if key == "Type" || key == "S" {
				dropDict = true
			}
			continue
		}
		// /Type /Action is an action dictionary even when a producer uses a
		// non-standard action key that is not in removedPDFKeys.
		if key == "Type" && pdfValueIsName(s.data, valueStart, removedObjectTypes) {
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

func pdfValueIsName(data []byte, start int, names map[string]bool) bool {
	s := &pdfValueScanner{data: data, pos: start}
	s.skipSpace()
	if s.pos >= len(data) || data[s.pos] != '/' {
		return false
	}
	nameStart := s.pos + 1
	nameEnd := s.nameEnd()
	return names[string(data[nameStart:nameEnd])]
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
			if hasPDFToken(data, pos, "stream") {
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
