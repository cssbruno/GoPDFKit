// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package inspect

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cssbruno/paperrune/importpdf"
)

type streamObjectRef struct {
	number     int
	generation int
}

type indirectInteger struct {
	value      int
	candidates int
}

type streamTokenKind uint8

const (
	streamTokenWord streamTokenKind = iota
	streamTokenName
	streamTokenDictStart
	streamTokenDictEnd
	streamTokenArrayStart
	streamTokenArrayEnd
	streamTokenOpaque
)

type streamToken struct {
	kind       streamTokenKind
	text       string
	start, end int
}

type streamSourceObject struct {
	ref  streamObjectRef
	body []byte
}

func scanDecodedStreamsContext(ctx context.Context, source *importpdf.Source, maxStreamBytes, maxTotalBytes, maxStreams int) ([][]byte, error) {
	objects := make([]streamSourceObject, 0)
	err := source.ForEachObjectBorrowedContext(ctx, func(ref importpdf.ObjRef, body []byte) error {
		objects = append(objects, streamSourceObject{
			ref:  streamObjectRef{number: ref.ObjectNumber(), generation: ref.Generation()},
			body: body,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read pdf objects: %w", err)
	}
	integers := indexIndirectIntegers(objects)
	streams := make([][]byte, 0)
	totalBytes := 0
	for _, object := range objects {
		if err := inspectContextErr(ctx); err != nil {
			return nil, err
		}
		dict, stream, found, err := objectStream(object.body, integers)
		if err != nil {
			return nil, fmt.Errorf("pdf object %d %d: %w", object.ref.number, object.ref.generation, err)
		}
		if !found {
			continue
		}
		if len(streams) >= maxStreams {
			return nil, errors.New("pdf stream count exceeds maximum size")
		}
		decoded, err := decodeStreamWithLimitContext(ctx, dict, stream, maxStreamBytes)
		if err != nil {
			return nil, err
		}
		if len(decoded) > maxTotalBytes-totalBytes {
			return nil, errors.New("decoded pdf streams exceed maximum size")
		}
		streams = append(streams, decoded)
		totalBytes += len(decoded)
	}
	return streams, nil
}

func objectStream(body []byte, integers map[streamObjectRef]indirectInteger) ([]byte, []byte, bool, error) {
	first, ok := nextStreamToken(body, 0)
	if !ok || first.kind != streamTokenDictStart {
		return nil, nil, false, nil
	}
	depth := 1
	pos := first.end
	dictEnd := -1
	for depth > 0 {
		token, ok := nextStreamToken(body, pos)
		if !ok {
			return nil, nil, false, errors.New("pdf stream dictionary is unterminated")
		}
		pos = token.end
		switch token.kind {
		case streamTokenDictStart:
			depth++
		case streamTokenDictEnd:
			depth--
			if depth == 0 {
				dictEnd = token.end
			}
		}
	}
	streamToken, ok := nextStreamToken(body, dictEnd)
	if !ok || streamToken.kind != streamTokenWord || streamToken.text != "stream" {
		return nil, nil, false, nil
	}
	dict := body[first.start:dictEnd]
	length, err := streamLength(dict, integers)
	if err != nil {
		return nil, nil, false, err
	}
	streamStart, err := streamDataStart(body, streamToken.end)
	if err != nil {
		return nil, nil, false, err
	}
	if length > len(body)-streamStart {
		return nil, nil, false, errors.New("pdf stream length exceeds object size")
	}
	streamEnd := streamStart + length
	endToken, err := streamEndToken(body, streamEnd)
	if err != nil {
		return nil, nil, false, err
	}
	if trailing, ok := nextStreamToken(body, endToken.end); ok {
		return nil, nil, false, fmt.Errorf("unexpected token %q after endstream", trailing.text)
	}
	return dict, body[streamStart:streamEnd], true, nil
}

func streamLength(dict []byte, integers map[streamObjectRef]indirectInteger) (int, error) {
	pos := 0
	depth := 0
	for {
		token, ok := nextStreamToken(dict, pos)
		if !ok {
			return 0, errors.New("pdf stream dictionary is missing /Length")
		}
		pos = token.end
		switch token.kind {
		case streamTokenDictStart:
			depth++
		case streamTokenDictEnd:
			depth--
		case streamTokenName:
			if depth != 1 || decodeStreamName(token.text) != "Length" {
				continue
			}
			valueToken, ok := nextStreamToken(dict, pos)
			if !ok {
				return 0, errors.New("pdf stream /Length is missing a value")
			}
			value, ok := tokenInteger(valueToken)
			if !ok || value < 0 {
				return 0, errors.New("pdf stream /Length must be a non-negative integer or indirect integer reference")
			}

			generationToken, hasGeneration := nextStreamToken(dict, valueToken.end)
			generation, generationOK := tokenInteger(generationToken)
			var refToken streamToken
			var hasRef bool
			if hasGeneration && generationOK {
				refToken, hasRef = nextStreamToken(dict, generationToken.end)
			}
			if value >= 0 && generation >= 0 && hasRef && refToken.kind == streamTokenWord && refToken.text == "R" {
				candidate, exists := integers[streamObjectRef{number: value, generation: generation}]
				if !exists {
					return 0, fmt.Errorf("pdf stream /Length reference %d %d R is not a direct integer object", value, generation)
				}
				if candidate.candidates != 1 {
					return 0, fmt.Errorf("pdf stream /Length reference %d %d R is ambiguous", value, generation)
				}
				if candidate.value < 0 {
					return 0, errors.New("pdf stream /Length must not be negative")
				}
				return candidate.value, nil
			}
			return value, nil
		}
	}
}

func indexIndirectIntegers(objects []streamSourceObject) map[streamObjectRef]indirectInteger {
	values := make(map[streamObjectRef]indirectInteger)
	for _, object := range objects {
		valueToken, ok := nextStreamToken(object.body, 0)
		if !ok {
			continue
		}
		value, ok := tokenInteger(valueToken)
		if !ok {
			continue
		}
		if _, hasTrailingValue := nextStreamToken(object.body, valueToken.end); hasTrailingValue {
			continue
		}
		candidate := values[object.ref]
		candidate.value = value
		candidate.candidates++
		values[object.ref] = candidate
	}
	return values
}

func streamDataStart(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, errors.New("pdf stream is missing its data line")
	}
	if data[pos] == '\r' {
		pos++
		if pos < len(data) && data[pos] == '\n' {
			pos++
		}
		return pos, nil
	}
	if data[pos] == '\n' {
		return pos + 1, nil
	}
	return 0, errors.New("pdf stream keyword must be followed by an end-of-line marker")
}

func streamEndToken(data []byte, pos int) (streamToken, error) {
	if pos < len(data) && data[pos] == '\r' {
		pos++
		if pos < len(data) && data[pos] == '\n' {
			pos++
		}
	} else if pos < len(data) && data[pos] == '\n' {
		pos++
	}
	token, ok := nextStreamToken(data, pos)
	if !ok || token.kind != streamTokenWord || token.text != "endstream" {
		return streamToken{}, errors.New("pdf stream length does not end at an endstream token")
	}
	return token, nil
}

func nextStreamToken(data []byte, pos int) (streamToken, bool) {
	pos = skipPDFWhitespaceAndComments(data, pos)
	if pos >= len(data) {
		return streamToken{}, false
	}
	start := pos
	switch data[pos] {
	case '(':
		_, pos = readPDFLiteralString(data, pos)
		return streamToken{kind: streamTokenOpaque, start: start, end: pos}, true
	case '<':
		if pos+1 < len(data) && data[pos+1] == '<' {
			return streamToken{kind: streamTokenDictStart, text: "<<", start: start, end: pos + 2}, true
		}
		_, pos = readPDFHexString(data, pos)
		return streamToken{kind: streamTokenOpaque, start: start, end: pos}, true
	case '>':
		if pos+1 < len(data) && data[pos+1] == '>' {
			return streamToken{kind: streamTokenDictEnd, text: ">>", start: start, end: pos + 2}, true
		}
		return streamToken{kind: streamTokenOpaque, start: start, end: pos + 1}, true
	case '[':
		return streamToken{kind: streamTokenArrayStart, text: "[", start: start, end: pos + 1}, true
	case ']':
		return streamToken{kind: streamTokenArrayEnd, text: "]", start: start, end: pos + 1}, true
	case '/':
		pos++
		nameStart := pos
		for pos < len(data) && !isPDFDelimiter(data[pos]) && !isPDFWhitespace(data[pos]) {
			pos++
		}
		return streamToken{kind: streamTokenName, text: string(data[nameStart:pos]), start: start, end: pos}, true
	default:
		if isPDFDelimiter(data[pos]) {
			return streamToken{kind: streamTokenOpaque, start: start, end: pos + 1}, true
		}
		word, end := readPDFWord(data, pos)
		return streamToken{kind: streamTokenWord, text: word, start: start, end: end}, true
	}
}

func tokenInteger(token streamToken) (int, bool) {
	if token.kind != streamTokenWord || token.text == "" || strings.ContainsAny(token.text, ".eE") {
		return 0, false
	}
	value, err := strconv.Atoi(token.text)
	if err != nil {
		return 0, false
	}
	return value, true
}

func decodeStreamName(name string) string {
	if !strings.Contains(name, "#") {
		return name
	}
	decoded := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '#' && i+2 < len(name) {
			value, err := strconv.ParseUint(name[i+1:i+3], 16, 8)
			if err == nil {
				decoded = append(decoded, byte(value))
				i += 2
				continue
			}
		}
		decoded = append(decoded, name[i])
	}
	return string(decoded)
}
