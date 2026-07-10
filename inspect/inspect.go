// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package inspect provides lightweight PDF inspection helpers.
package inspect

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/cssbruno/gopdfkit/importpdf"
	"golang.org/x/text/encoding/charmap"
)

const (
	maxDecodedStreamBytes = 64 * 1024 * 1024
	maxDecodedStreamCount = 4096
	maxDecodedTotalBytes  = 128 * 1024 * 1024
	mediaBoxMatchCount    = 5
	textTokenCapacity     = 8
	pdfOctalBase          = 8
	utf16BOMBytes         = 2
)

var mediaBoxPattern = regexp.MustCompile(`/MediaBox\s*\[\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+))\s+([-+]?(?:\d+(?:\.\d*)?|\.\d+))\s+([-+]?(?:\d+(?:\.\d*)?|\.\d+))\s+([-+]?(?:\d+(?:\.\d*)?|\.\d+))`)

// ValidateStructure checks that data can be parsed as an unencrypted classic
// PDF with at least one importable page.
func ValidateStructure(data []byte) error {
	return ValidateStructureContext(context.Background(), data)
}

// ValidateStructureContext checks that data can be parsed as an unencrypted
// classic PDF with at least one importable page and honors ctx during parsing.
func ValidateStructureContext(ctx context.Context, data []byte) error {
	count, err := PageCountContext(ctx, data)
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
	return PageCountContext(context.Background(), data)
}

// PageCountContext returns the number of pages GoPDFKit can import from data
// and honors ctx while importing the page tree.
func PageCountContext(ctx context.Context, data []byte) (int, error) {
	source, err := importpdf.OpenBytesWithOptionsContext(ctx, data, importpdf.ImportOptions{})
	if err != nil {
		return 0, fmt.Errorf("parse pdf: %w", err)
	}
	return source.PageCount(), nil
}

// FirstPageSizePoints returns the first MediaBox dimensions in PDF points.
func FirstPageSizePoints(data []byte) (float64, float64, error) {
	match := mediaBoxPattern.FindSubmatch(data)
	if len(match) != mediaBoxMatchCount {
		return 0, 0, errors.New("pdf MediaBox not found")
	}

	llx, err := strconv.ParseFloat(string(match[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse MediaBox lower-left x: %w", err)
	}
	lly, err := strconv.ParseFloat(string(match[2]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse MediaBox lower-left y: %w", err)
	}
	urx, err := strconv.ParseFloat(string(match[3]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse MediaBox upper-right x: %w", err)
	}
	ury, err := strconv.ParseFloat(string(match[4]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse MediaBox upper-right y: %w", err)
	}
	return urx - llx, ury - lly, nil
}

// Text extracts literal text operators from PDF content streams.
func Text(data []byte) (string, error) {
	return TextContext(context.Background(), data)
}

// TextContext extracts literal text operators from PDF content streams and
// honors ctx during stream scanning and text tokenization.
func TextContext(ctx context.Context, data []byte) (string, error) {
	if err := inspectContextErr(ctx); err != nil {
		return "", err
	}
	streams, err := DecodedStreamsContext(ctx, data)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, stream := range streams {
		if err := inspectContextErr(ctx); err != nil {
			return "", err
		}
		streamText, err := textFromContentStreamContext(ctx, stream)
		if err != nil {
			return "", err
		}
		text.WriteString(streamText)
	}
	return text.String(), nil
}

// PageText extracts text from one importable PDF page.
func PageText(data []byte, pageNum int) (string, error) {
	return PageTextContext(context.Background(), data, pageNum)
}

// PageTextContext extracts text from one importable PDF page while honoring
// ctx.
func PageTextContext(ctx context.Context, data []byte, pageNum int) (string, error) {
	if err := inspectContextErr(ctx); err != nil {
		return "", err
	}
	if pageNum < 1 {
		return "", errors.New("pdf page number must be positive")
	}

	source, err := importpdf.OpenBytesWithOptionsContext(ctx, data, importpdf.ImportOptions{})
	if err != nil {
		return "", fmt.Errorf("parse pdf page: %w", err)
	}
	page, err := source.PageContext(ctx, pageNum, "MediaBox")
	if err != nil {
		return "", fmt.Errorf("parse pdf page: %w", err)
	}
	if page == nil {
		return "", fmt.Errorf("pdf page %d not found", pageNum)
	}
	content, err := page.ContentWithContext(ctx)
	if err != nil {
		return "", fmt.Errorf("parse pdf page: %w", err)
	}
	return textFromContentStreamContext(ctx, content)
}

// DecodedStreams returns raw or Flate-decoded PDF streams in file order.
func DecodedStreams(data []byte) ([][]byte, error) {
	return DecodedStreamsContext(context.Background(), data)
}

// DecodedStreamsContext returns raw or Flate-decoded PDF streams in file order
// and honors ctx while scanning and decoding streams.
func DecodedStreamsContext(ctx context.Context, data []byte) ([][]byte, error) {
	return decodedStreamsContext(ctx, data, maxDecodedStreamBytes, maxDecodedTotalBytes, maxDecodedStreamCount)
}

func decodedStreamsContext(ctx context.Context, data []byte, maxStreamBytes, maxTotalBytes, maxStreams int) ([][]byte, error) {
	if err := inspectContextErr(ctx); err != nil {
		return nil, err
	}
	if maxStreamBytes < 0 || maxTotalBytes < 0 || maxStreams < 0 {
		return nil, errors.New("pdf stream limits are invalid")
	}
	streams := make([][]byte, 0)
	totalBytes := 0
	searchFrom := 0

	for {
		if err := inspectContextErr(ctx); err != nil {
			return nil, err
		}
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
		if len(streams) >= maxStreams {
			return nil, errors.New("pdf stream count exceeds maximum size")
		}
		decoded, err := decodeStreamWithLimitContext(ctx, streamDictionaryBytes(data[:streamIdx]), stream, maxStreamBytes)
		if err != nil {
			return nil, err
		}
		if len(decoded) > maxTotalBytes-totalBytes {
			return nil, errors.New("decoded pdf streams exceed maximum size")
		}

		streams = append(streams, decoded)
		totalBytes += len(decoded)
		searchFrom = streamEnd + len("endstream")
	}
}

func inspectContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func streamDictionaryBytes(beforeStream []byte) []byte {
	start := bytes.LastIndex(beforeStream, []byte("<<"))
	if start < 0 {
		return nil
	}
	return beforeStream[start:]
}

func decodeStreamContext(ctx context.Context, dict []byte, stream []byte) ([]byte, error) {
	return decodeStreamWithLimitContext(ctx, dict, stream, maxDecodedStreamBytes)
}

func decodeStreamWithLimitContext(ctx context.Context, dict []byte, stream []byte, maxBytes int) ([]byte, error) {
	if err := inspectContextErr(ctx); err != nil {
		return nil, err
	}
	if hasFlateFilter(dict) {
		return inflateStreamWithLimitContext(ctx, stream, maxBytes)
	}
	if len(stream) > maxBytes {
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

func inflateStreamContext(ctx context.Context, stream []byte) ([]byte, error) {
	return inflateStreamWithLimitContext(ctx, stream, maxDecodedStreamBytes)
}

func inflateStreamWithLimitContext(ctx context.Context, stream []byte, maxBytes int) ([]byte, error) {
	if err := inspectContextErr(ctx); err != nil {
		return nil, err
	}
	reader, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, fmt.Errorf("decode flate stream: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	decoded, err := io.ReadAll(io.LimitReader(inspectContextReader{ctx: ctx, r: reader}, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read flate stream: %w", err)
	}
	if len(decoded) > maxBytes {
		return nil, errors.New("decoded pdf stream exceeds maximum size")
	}
	return decoded, nil
}

type inspectContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r inspectContextReader) Read(p []byte) (int, error) {
	if err := inspectContextErr(r.ctx); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		return n, inspectContextErr(r.ctx)
	}
	return n, nil
}

type pdfTextToken struct {
	text   string
	isText bool
}

func textFromContentStreamContext(ctx context.Context, stream []byte) (string, error) {
	var out strings.Builder
	tokens := make([]pdfTextToken, 0, textTokenCapacity)
	inText := false

	for i := 0; i < len(stream); {
		if i%1024 == 0 {
			if err := inspectContextErr(ctx); err != nil {
				return "", err
			}
		}
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

	if err := inspectContextErr(ctx); err != nil {
		return "", err
	}
	return out.String(), nil
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
