// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package inspect provides lightweight PDF inspection helpers.
package inspect

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/cssbruno/gopdfkit/document"
	"golang.org/x/text/encoding/charmap"
)

const (
	maxDecodedStreamBytes = 64 * 1024 * 1024
	mediaBoxMatchCount    = 3
	textTokenCapacity     = 8
	pdfOctalBase          = 8
	utf16BOMBytes         = 2
)

var mediaBoxPattern = regexp.MustCompile(`/MediaBox\s*\[\s*[-+]?(?:\d+(?:\.\d*)?|\.\d+)\s+[-+]?(?:\d+(?:\.\d*)?|\.\d+)\s+([-+]?(?:\d+(?:\.\d*)?|\.\d+))\s+([-+]?(?:\d+(?:\.\d*)?|\.\d+))`)

// ValidateStructure checks that data can be parsed as an unencrypted classic
// PDF with at least one importable page.
func ValidateStructure(data []byte) error {
	count, err := PageCount(data)
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("pdf has no pages")
	}
	return nil
}

// PageCount returns the number of pages GoPDFKit can import from data.
func PageCount(data []byte) (int, error) {
	pdf := document.New("", "", "", "")
	ids := pdf.ImportPagesFromSource(data, "MediaBox")
	if err := pdf.Error(); err != nil {
		return 0, fmt.Errorf("parse pdf: %w", err)
	}
	return len(ids), nil
}

// FirstPageSizePoints returns the first MediaBox dimensions in PDF points.
func FirstPageSizePoints(data []byte) (float64, float64, error) {
	match := mediaBoxPattern.FindSubmatch(data)
	if len(match) != mediaBoxMatchCount {
		return 0, 0, errors.New("pdf MediaBox not found")
	}

	width, err := strconv.ParseFloat(string(match[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse MediaBox width: %w", err)
	}
	height, err := strconv.ParseFloat(string(match[2]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse MediaBox height: %w", err)
	}
	return width, height, nil
}

// Text extracts literal text operators from PDF content streams.
func Text(data []byte) (string, error) {
	streams, err := DecodedStreams(data)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, stream := range streams {
		text.WriteString(textFromContentStream(stream))
	}
	return text.String(), nil
}

// PageText imports one PDF page through GoPDFKit and extracts text from the
// resulting single-page PDF.
func PageText(data []byte, pageNum int) (string, error) {
	if pageNum < 1 {
		return "", errors.New("pdf page number must be positive")
	}

	pdf := document.New("", "", "", "")
	pageID := pdf.ImportPageStream(bytes.NewReader(data), pageNum, "MediaBox")
	if err := pdf.Error(); err != nil {
		return "", fmt.Errorf("parse pdf page: %w", err)
	}
	if pageID == 0 {
		return "", fmt.Errorf("pdf page %d not found", pageNum)
	}

	pdf.AddPage()
	pdf.UseImportedPage(pageID, 0, 0, 0, 0)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return "", fmt.Errorf("render imported pdf page: %w", err)
	}
	return Text(buf.Bytes())
}

// DecodedStreams returns raw or Flate-decoded PDF streams in file order.
func DecodedStreams(data []byte) ([][]byte, error) {
	streams := make([][]byte, 0)
	searchFrom := 0

	for {
		streamIdxRel := bytes.Index(data[searchFrom:], []byte("stream"))
		if streamIdxRel < 0 {
			return streams, nil
		}

		streamIdx := searchFrom + streamIdxRel
		streamStart := streamIdx + len("stream")
		if streamStart+1 < len(data) && data[streamStart] == '\r' && data[streamStart+1] == '\n' {
			streamStart += 2
		} else if streamStart < len(data) && (data[streamStart] == '\n' || data[streamStart] == '\r') {
			streamStart++
		}

		endRel := bytes.Index(data[streamStart:], []byte("endstream"))
		if endRel < 0 {
			return nil, errors.New("pdf stream missing endstream")
		}

		streamEnd := streamStart + endRel
		stream := bytes.TrimRight(data[streamStart:streamEnd], "\r\n")
		decoded, err := decodeStream(streamDictionaryBytes(data[:streamIdx]), stream)
		if err != nil {
			return nil, err
		}

		streams = append(streams, decoded)
		searchFrom = streamEnd + len("endstream")
	}
}

func streamDictionaryBytes(beforeStream []byte) []byte {
	start := bytes.LastIndex(beforeStream, []byte("<<"))
	if start < 0 {
		return nil
	}
	return beforeStream[start:]
}

func decodeStream(dict []byte, stream []byte) ([]byte, error) {
	if hasFlateFilter(dict) {
		return inflateStream(stream)
	}
	if len(stream) > maxDecodedStreamBytes {
		return nil, errors.New("pdf stream exceeds maximum size")
	}
	return append([]byte(nil), stream...), nil
}

func hasFlateFilter(dict []byte) bool {
	return containsPDFName(dict, "flatedecode") || containsPDFName(dict, "fl")
}

func containsPDFName(data []byte, name string) bool {
	name = strings.ToLower(name)
	for pos := 0; pos < len(data); pos++ {
		if data[pos] != '/' {
			continue
		}

		start := pos + 1
		end := start
		for end < len(data) && !isPDFDelimiter(data[end]) && !isPDFWhitespace(data[end]) {
			end++
		}
		if strings.ToLower(string(data[start:end])) == name {
			return true
		}
		pos = end
	}
	return false
}

func inflateStream(stream []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, fmt.Errorf("decode flate stream: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	decoded, err := io.ReadAll(io.LimitReader(reader, maxDecodedStreamBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read flate stream: %w", err)
	}
	if len(decoded) > maxDecodedStreamBytes {
		return nil, errors.New("decoded pdf stream exceeds maximum size")
	}
	return decoded, nil
}

type pdfTextToken struct {
	text   string
	isText bool
}

func textFromContentStream(stream []byte) string {
	var out strings.Builder
	tokens := make([]pdfTextToken, 0, textTokenCapacity)
	inText := false

	for i := 0; i < len(stream); {
		i = skipPDFWhitespaceAndComments(stream, i)
		if i >= len(stream) {
			break
		}

		switch stream[i] {
		case '(':
			raw, next := readPDFLiteralString(stream, i)
			tokens = append(tokens, pdfTextToken{text: decodePDFTextBytes(raw), isText: true})
			i = next
		case '<':
			if i+1 < len(stream) && stream[i+1] == '<' {
				i += 2
				continue
			}
			raw, next := readPDFHexString(stream, i)
			tokens = append(tokens, pdfTextToken{text: decodePDFTextBytes(raw), isText: true})
			i = next
		case '[':
			text, next := readPDFArrayText(stream, i)
			tokens = append(tokens, pdfTextToken{text: text, isText: true})
			i = next
		default:
			word, next := readPDFWord(stream, i)
			if word == "" {
				i++
				continue
			}

			switch word {
			case "BT":
				inText = true
				tokens = tokens[:0]
			case "ET":
				inText = false
				tokens = tokens[:0]
			case "Tj", "'", "\"", "TJ":
				if inText {
					out.WriteString(lastTextToken(tokens))
				}
				tokens = tokens[:0]
			default:
				if isPDFTextStateOperator(word) {
					tokens = tokens[:0]
				}
			}
			i = next
		}
	}

	return out.String()
}

func isPDFTextStateOperator(word string) bool {
	switch word {
	case "Tf", "Td", "TD", "Tm", "Tr", "Ts", "Tc", "TL", "Tw", "Tz", "T*", "cm", "q", "Q":
		return true
	default:
		return false
	}
}

func lastTextToken(tokens []pdfTextToken) string {
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].isText {
			return tokens[i].text
		}
	}
	return ""
}

func skipPDFWhitespaceAndComments(data []byte, pos int) int {
	for pos < len(data) {
		switch data[pos] {
		case 0, '\t', '\n', '\f', '\r', ' ':
			pos++
		case '%':
			for pos < len(data) && data[pos] != '\n' && data[pos] != '\r' {
				pos++
			}
		default:
			return pos
		}
	}
	return pos
}

func readPDFWord(data []byte, pos int) (string, int) {
	start := pos
	for pos < len(data) && !isPDFDelimiter(data[pos]) && !isPDFWhitespace(data[pos]) {
		pos++
	}
	return string(data[start:pos]), pos
}

func isPDFDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}

func isPDFWhitespace(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

func readPDFArrayText(data []byte, pos int) (string, int) {
	var out strings.Builder
	pos++
	depth := 1

	for pos < len(data) && depth > 0 {
		pos = skipPDFWhitespaceAndComments(data, pos)
		if pos >= len(data) {
			break
		}

		switch data[pos] {
		case '[':
			depth++
			pos++
		case ']':
			depth--
			pos++
		case '(':
			raw, next := readPDFLiteralString(data, pos)
			out.WriteString(decodePDFTextBytes(raw))
			pos = next
		case '<':
			if pos+1 < len(data) && data[pos+1] == '<' {
				pos += 2
				continue
			}
			raw, next := readPDFHexString(data, pos)
			out.WriteString(decodePDFTextBytes(raw))
			pos = next
		default:
			_, next := readPDFWord(data, pos)
			if next <= pos {
				pos++
			} else {
				pos = next
			}
		}
	}

	return out.String(), pos
}

func readPDFLiteralString(data []byte, pos int) ([]byte, int) {
	var out []byte
	pos++
	depth := 1

	for pos < len(data) && depth > 0 {
		c := data[pos]
		pos++

		switch c {
		case '\\':
			if pos >= len(data) {
				break
			}
			escaped := data[pos]
			pos++

			switch escaped {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, escaped)
			case '\r':
				if pos < len(data) && data[pos] == '\n' {
					pos++
				}
			case '\n':
			default:
				if escaped >= '0' && escaped <= '7' {
					value := int(escaped - '0')
					for count := 1; count < 3 && pos < len(data) && data[pos] >= '0' && data[pos] <= '7'; count++ {
						value = value*pdfOctalBase + int(data[pos]-'0')
						pos++
					}
					out = append(out, byte(value))
				} else {
					out = append(out, escaped)
				}
			}
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth > 0 {
				out = append(out, c)
			}
		default:
			out = append(out, c)
		}
	}

	return out, pos
}

func readPDFHexString(data []byte, pos int) ([]byte, int) {
	pos++
	start := pos
	for pos < len(data) && data[pos] != '>' {
		pos++
	}

	hexText := make([]byte, 0, pos-start+1)
	for _, c := range data[start:pos] {
		if !isPDFWhitespace(c) {
			hexText = append(hexText, c)
		}
	}
	if len(hexText)%2 != 0 {
		hexText = append(hexText, '0')
	}

	out := make([]byte, hex.DecodedLen(len(hexText)))
	if _, err := hex.Decode(out, hexText); err != nil {
		out = nil
	}
	if pos < len(data) && data[pos] == '>' {
		pos++
	}
	return out, pos
}

func decodePDFTextBytes(raw []byte) string {
	if len(raw) >= utf16BOMBytes && raw[0] == 0xfe && raw[1] == 0xff {
		u16 := make([]uint16, 0, (len(raw)-utf16BOMBytes)/utf16BOMBytes)
		for i := utf16BOMBytes; i+1 < len(raw); i += utf16BOMBytes {
			u16 = append(u16, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
		return string(utf16.Decode(u16))
	}

	text, err := charmap.Windows1252.NewDecoder().String(string(raw))
	if err != nil {
		return string(raw)
	}
	return text
}
