// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	DefaultCSTMaxBytes       = 8 << 20
	DefaultCSTMaxLines       = 200_000
	DefaultCSTMaxLineBytes   = 1 << 20
	DefaultCSTMaxIndentBytes = 4096
)

var (
	ErrCSTLimit   = errors.New("paperlang: lossless CST limit exceeded")
	ErrCSTInvalid = errors.New("paperlang: invalid lossless CST input")
)

// CSTLimits bounds whole-source lossless parsing. Zero selects defaults;
// nonzero policies must be complete and cannot exceed the defaults.
type CSTLimits struct {
	MaxBytes       uint32
	MaxLines       uint32
	MaxLineBytes   uint32
	MaxIndentBytes uint32
}

func DefaultCSTLimits() CSTLimits {
	return CSTLimits{MaxBytes: DefaultCSTMaxBytes, MaxLines: DefaultCSTMaxLines, MaxLineBytes: DefaultCSTMaxLineBytes, MaxIndentBytes: DefaultCSTMaxIndentBytes}
}

type CSTLineKind string

const (
	CSTBlank           CSTLineKind = "blank"
	CSTComment         CSTLineKind = "comment"
	CSTNodeStatement   CSTLineKind = "node"
	CSTProperty        CSTLineKind = "property"
	CSTOpaqueStatement CSTLineKind = "opaque"
)

// CSTLine is one exact physical source line. Raw includes its original LF,
// CRLF, or absent final newline. ScalarRaw and comment spans are projections;
// printing always uses Raw/source bytes rather than reconstructing syntax.
type CSTLine struct {
	Kind        CSTLineKind
	Name        string
	IndentBytes uint32
	Raw         string
	ScalarRaw   string
	Span        Span
	ContentSpan Span
	ScalarSpan  Span
	CommentSpan Span
}

// CSTOpaqueNode preserves one unrecognized statement and its entire indented
// subtree as an indivisible future-language region.
type CSTOpaqueNode struct {
	Name        string
	IndentBytes uint32
	HeaderSpan  Span
	Span        Span
	Raw         string
}

// CST owns an exact immutable source snapshot plus bounded line and opaque
// projections. It intentionally does not replace AST as the semantic model.
type CST struct {
	file   string
	source string
	lines  []CSTLine
	opaque []CSTOpaqueNode
}

func (c *CST) File() string {
	if c == nil {
		return ""
	}
	return c.file
}

func (c *CST) Lines() []CSTLine {
	if c == nil {
		return nil
	}
	return append([]CSTLine(nil), c.lines...)
}

func (c *CST) OpaqueNodes() []CSTOpaqueNode {
	if c == nil {
		return nil
	}
	return append([]CSTOpaqueNode(nil), c.opaque...)
}

// LookupOffset returns the physical line containing one UTF-8 byte offset.
func (c *CST) LookupOffset(offset uint64) (CSTLine, bool) {
	if c == nil || offset >= uint64(len(c.source)) {
		return CSTLine{}, false
	}
	index := sort.Search(len(c.lines), func(index int) bool { return c.lines[index].Span.End.Offset > offset })
	if index == len(c.lines) || c.lines[index].Span.Start.Offset > offset {
		return CSTLine{}, false
	}
	return c.lines[index], true
}

// LookupSpan returns every physical line intersecting a same-file half-open
// span. Returned values are detached from the CST.
func (c *CST) LookupSpan(span Span) []CSTLine {
	if c == nil || span.File != "" && span.File != c.file || span.End.Offset < span.Start.Offset {
		return nil
	}
	result := make([]CSTLine, 0)
	for _, line := range c.lines {
		if line.Span.End.Offset <= span.Start.Offset || line.Span.Start.Offset >= span.End.Offset {
			continue
		}
		result = append(result, line)
	}
	return result
}

// OpaqueAtOffset locates the outer opaque future-language subtree owning an
// offset. Descendant unknown statements remain bytes inside that region.
func (c *CST) OpaqueAtOffset(offset uint64) (CSTOpaqueNode, bool) {
	if c == nil {
		return CSTOpaqueNode{}, false
	}
	for _, node := range c.opaque {
		if offset >= node.Span.Start.Offset && offset < node.Span.End.Offset {
			return node, true
		}
	}
	return CSTOpaqueNode{}, false
}

type CSTError struct {
	Line    uint32
	Problem string
	Cause   error
}

func (e *CSTError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Line != 0 {
		return fmt.Sprintf("paperlang: lossless CST line %d: %s", e.Line, e.Problem)
	}
	return "paperlang: lossless CST: " + e.Problem
}

func (e *CSTError) Unwrap() error { return e.Cause }

func ParseCST(file, source string) (*CST, error) {
	return ParseCSTWithLimits(file, source, CSTLimits{})
}

func ParseCSTWithLimits(file, source string, limits CSTLimits) (*CST, error) {
	normalized, err := normalizeCSTLimits(limits)
	if err != nil {
		return nil, err
	}
	if !utf8.ValidString(source) {
		return nil, &CSTError{Problem: "source is not valid UTF-8", Cause: ErrCSTInvalid}
	}
	if uint64(len(source)) > uint64(normalized.MaxBytes) {
		return nil, &CSTError{Problem: "source exceeds MaxBytes", Cause: ErrCSTLimit}
	}
	physical := splitSourceLines(source)
	if uint64(len(physical)) > uint64(normalized.MaxLines) {
		return nil, &CSTError{Problem: "source exceeds MaxLines", Cause: ErrCSTLimit}
	}
	starts := sourceLineStarts(source)
	result := &CST{file: file, source: string([]byte(source)), lines: make([]CSTLine, 0, len(physical))}
	for _, line := range physical {
		rawEnd := line.startOffset + len(line.text) + line.newlineWidth
		if uint64(rawEnd-line.startOffset) > uint64(normalized.MaxLineBytes) {
			return nil, &CSTError{Line: line.line, Problem: "physical line exceeds MaxLineBytes", Cause: ErrCSTLimit}
		}
		indent := cstIndent(line.text)
		if uint64(indent) > uint64(normalized.MaxIndentBytes) {
			return nil, &CSTError{Line: line.line, Problem: "indentation exceeds MaxIndentBytes", Cause: ErrCSTLimit}
		}
		entry := classifyCSTLine(file, source, starts, line, rawEnd, indent)
		result.lines = append(result.lines, entry)
	}
	result.buildOpaqueNodes()
	return result, nil
}

func normalizeCSTLimits(limits CSTLimits) (CSTLimits, error) {
	if limits == (CSTLimits{}) {
		return DefaultCSTLimits(), nil
	}
	hard := DefaultCSTLimits()
	if limits.MaxBytes == 0 || limits.MaxBytes > hard.MaxBytes || limits.MaxLines == 0 || limits.MaxLines > hard.MaxLines ||
		limits.MaxLineBytes == 0 || limits.MaxLineBytes > hard.MaxLineBytes || limits.MaxIndentBytes == 0 || limits.MaxIndentBytes > hard.MaxIndentBytes {
		return CSTLimits{}, &CSTError{Problem: "limits are incomplete or exceed hard caps", Cause: ErrCSTLimit}
	}
	return limits, nil
}

func classifyCSTLine(file, source string, starts []int, line sourceLine, rawEnd, indent int) CSTLine {
	raw := source[line.startOffset:rawEnd]
	contentEnd := line.startOffset + len(line.text)
	entry := CSTLine{
		Kind: CSTBlank, IndentBytes: uint32(indent), Raw: raw,
		Span:        cstSpan(file, source, starts, line.startOffset, rawEnd),
		ContentSpan: cstSpan(file, source, starts, line.startOffset+indent, contentEnd),
	}
	content := line.text[indent:]
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return entry
	}
	comment := cstCommentOffset(content)
	if strings.HasPrefix(strings.TrimLeft(content, " \t"), "#") {
		entry.Kind = CSTComment
		entry.CommentSpan = cstSpan(file, source, starts, line.startOffset+indent+comment, contentEnd)
		return entry
	}
	if comment >= 0 {
		entry.CommentSpan = cstSpan(file, source, starts, line.startOffset+indent+comment, contentEnd)
		content = content[:comment]
	}
	nameEnd := 0
	for nameEnd < len(content) && isIdentifierContinue(content[nameEnd]) {
		nameEnd++
	}
	entry.Name = content[:nameEnd]
	colon := strings.IndexByte(content, ':')
	if colon >= 0 {
		rawValue := content[colon+1:]
		leading := len(rawValue) - len(strings.TrimLeft(rawValue, " \t"))
		trailing := len(rawValue) - len(strings.TrimRight(rawValue, " \t"))
		valueStart := colon + 1 + leading
		valueEnd := len(content) - trailing
		if valueEnd >= valueStart {
			entry.ScalarRaw = content[valueStart:valueEnd]
			entry.ScalarSpan = cstSpan(file, source, starts, line.startOffset+indent+valueStart, line.startOffset+indent+valueEnd)
		}
	}
	if kind, known := parseNodeKind(entry.Name); known {
		_ = kind
		entry.Kind = CSTNodeStatement
		return entry
	}
	afterName := strings.TrimSpace(content[nameEnd:])
	unknownHeader := strings.HasPrefix(afterName, "@") || colon >= 0 && strings.TrimSpace(content[colon+1:]) == ""
	if unknownHeader {
		entry.Kind = CSTOpaqueStatement
	} else {
		entry.Kind = CSTProperty
	}
	return entry
}

func (c *CST) buildOpaqueNodes() {
	starts := sourceLineStarts(c.source)
	for index := 0; index < len(c.lines); index++ {
		line := c.lines[index]
		if line.Kind != CSTOpaqueStatement {
			continue
		}
		end := line.Span.End.Offset
		next := index + 1
		for ; next < len(c.lines); next++ {
			candidate := c.lines[next]
			if candidate.Kind == CSTBlank || candidate.Kind == CSTComment && candidate.IndentBytes <= line.IndentBytes ||
				candidate.Kind != CSTComment && candidate.IndentBytes <= line.IndentBytes {
				break
			}
			end = candidate.Span.End.Offset
		}
		span := Span{File: c.file, Start: line.Span.Start, End: cstPosition(c.source, starts, int(end))}
		c.opaque = append(c.opaque, CSTOpaqueNode{
			Name: line.Name, IndentBytes: line.IndentBytes, HeaderSpan: line.ContentSpan, Span: span,
			Raw: c.source[line.Span.Start.Offset:end],
		})
		index = next - 1
	}
}

func cstIndent(line string) int {
	index := 0
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	return index
}

func cstCommentOffset(content string) int {
	inString, escaped := false, false
	for index := 0; index < len(content); index++ {
		character := content[index]
		if inString {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
		} else if character == '#' {
			return index
		}
	}
	return -1
}

func cstSpan(file, source string, starts []int, start, end int) Span {
	return Span{File: file, Start: cstPosition(source, starts, start), End: cstPosition(source, starts, end)}
}

func cstPosition(source string, starts []int, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	lineIndex := sort.Search(len(starts), func(index int) bool { return starts[index] > offset }) - 1
	if lineIndex < 0 {
		lineIndex = 0
	}
	return Position{Offset: uint64(offset), Line: uint32(lineIndex + 1), Column: uint32(utf8.RuneCountInString(source[starts[lineIndex]:offset]) + 1)}
}

// PrintLossless returns a detached byte-identical copy of the CST snapshot.
func PrintLossless(cst *CST) ([]byte, error) {
	if cst == nil {
		return nil, &CSTError{Problem: "CST is nil", Cause: ErrCSTInvalid}
	}
	return append([]byte(nil), cst.source...), nil
}

// LosslessParseResult exposes the exact CST beside the existing semantic AST.
// OK reports only lossless-layer success; inspect Semantic.OK independently.
type LosslessParseResult struct {
	CST      *CST
	Semantic ParseResult
	Err      error
}

func (r LosslessParseResult) OK() bool { return r.Err == nil && r.CST != nil }

func ParseLossless(file, source string) LosslessParseResult {
	return ParseLosslessWithLimits(file, source, CSTLimits{})
}

func ParseLosslessWithLimits(file, source string, limits CSTLimits) LosslessParseResult {
	cst, err := ParseCSTWithLimits(file, source, limits)
	result := LosslessParseResult{CST: cst, Err: err}
	if err == nil {
		result.Semantic = Parse(file, source)
	}
	return result
}

// SourcePrintOptions keeps canonical formatting explicit. The zero value is a
// no-op lossless print suitable for inspection/edit paths that made no patch.
type SourcePrintOptions struct {
	Canonical bool
	Format    FormatOptions
}

func PrintParsed(result LosslessParseResult, options SourcePrintOptions) ([]byte, error) {
	if result.Err != nil {
		return nil, result.Err
	}
	if !options.Canonical {
		return PrintLossless(result.CST)
	}
	if !result.Semantic.OK() {
		return nil, &CSTError{Problem: "canonical formatting requires a valid semantic AST", Cause: ErrCSTInvalid}
	}
	return FormatWithOptions(result.Semantic.AST, options.Format)
}
