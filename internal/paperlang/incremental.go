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

var (
	ErrCSTPatch      = errors.New("paperlang: invalid CST patch")
	ErrCSTPatchLimit = errors.New("paperlang: CST patch limit exceeded")
)

type CSTPatchLimits struct {
	MaxPatches          uint32
	MaxReplacementBytes uint32
	MaxRelexBytes       uint32
	CST                 CSTLimits
}

func DefaultCSTPatchLimits() CSTPatchLimits {
	return CSTPatchLimits{MaxPatches: 256, MaxReplacementBytes: 1 << 20, MaxRelexBytes: 2 << 20, CST: DefaultCSTLimits()}
}

// CSTPatch is a minimal replacement against the original CST revision. Patches
// are sorted by byte span before application and must not overlap.
type CSTPatch struct {
	Span        Span
	Replacement string
}

type IncrementalParseResult struct {
	CST               *CST
	Semantic          ParseResult
	Changed           bool
	RelexedOld        Span
	RelexedNew        Span
	ReusedPrefixLines uint32
	ReusedSuffixLines uint32
}

// ApplyCSTPatches incrementally reclassifies only the physical-line envelope
// touched by sorted non-overlapping patches. Prefix lines are reused exactly;
// suffix projections are reused with deterministic byte/line span shifts.
// Semantic Parse is intentionally run on the candidate as a clean equivalence
// oracle until the separate semantic incremental parser is introduced.
func ApplyCSTPatches(cst *CST, patches []CSTPatch, limits CSTPatchLimits) (IncrementalParseResult, error) {
	if cst == nil {
		return IncrementalParseResult{}, fmt.Errorf("%w: CST is nil", ErrCSTPatch)
	}
	normalized, err := normalizeCSTPatchLimits(limits)
	if err != nil {
		return IncrementalParseResult{}, err
	}
	if len(patches) == 0 || uint64(len(patches)) > uint64(normalized.MaxPatches) {
		return IncrementalParseResult{}, fmt.Errorf("%w: patch count", ErrCSTPatchLimit)
	}
	ordered := append([]CSTPatch(nil), patches...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Span.Start.Offset != ordered[j].Span.Start.Offset {
			return ordered[i].Span.Start.Offset < ordered[j].Span.Start.Offset
		}
		return ordered[i].Span.End.Offset < ordered[j].Span.End.Offset
	})
	replacementBytes := uint64(0)
	for index, patch := range ordered {
		start, end := patch.Span.Start.Offset, patch.Span.End.Offset
		if patch.Span.File != "" && patch.Span.File != cst.file || end < start || end > uint64(len(cst.source)) ||
			!sourceBoundary(cst.source, start) || !sourceBoundary(cst.source, end) || !utf8.ValidString(patch.Replacement) {
			return IncrementalParseResult{}, fmt.Errorf("%w: patch %d has invalid file/range/UTF-8", ErrCSTPatch, index)
		}
		if index > 0 && (start < ordered[index-1].Span.End.Offset || start == ordered[index-1].Span.Start.Offset) {
			return IncrementalParseResult{}, fmt.Errorf("%w: patches %d and %d overlap or share an ambiguous insertion point", ErrCSTPatch, index-1, index)
		}
		replacementBytes = saturatingCSTAdd(replacementBytes, uint64(len(patch.Replacement)))
		if replacementBytes > uint64(normalized.MaxReplacementBytes) {
			return IncrementalParseResult{}, fmt.Errorf("%w: replacement bytes", ErrCSTPatchLimit)
		}
	}

	var builder strings.Builder
	delta := int64(0)
	cursor := uint64(0)
	for _, patch := range ordered {
		builder.WriteString(cst.source[cursor:patch.Span.Start.Offset])
		builder.WriteString(patch.Replacement)
		cursor = patch.Span.End.Offset
		delta += int64(len(patch.Replacement)) - int64(patch.Span.End.Offset-patch.Span.Start.Offset) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	builder.WriteString(cst.source[cursor:])
	candidate := builder.String()
	if uint64(len(candidate)) > uint64(normalized.CST.MaxBytes) {
		return IncrementalParseResult{}, fmt.Errorf("%w: candidate bytes", ErrCSTPatchLimit)
	}
	if candidate == cst.source {
		position := cstPosition(cst.source, sourceLineStarts(cst.source), int(ordered[0].Span.Start.Offset)) // #nosec G115 -- source offset is bounded by validated input or parser state
		return IncrementalParseResult{
			CST: cst, Semantic: Parse(cst.file, cst.source), RelexedOld: Span{File: cst.file, Start: position, End: position},
			RelexedNew: Span{File: cst.file, Start: position, End: position}, ReusedPrefixLines: uint32(len(cst.lines)), // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		}, nil
	}

	earliest := ordered[0].Span.Start.Offset
	latest := ordered[len(ordered)-1].Span.End.Offset
	startIndex := cst.editLineIndex(earliest)
	endIndex := cst.editLineIndex(latest)
	if endIndex < startIndex {
		endIndex = startIndex
	}
	endExclusive := endIndex
	if endExclusive < len(cst.lines) {
		endExclusive++
	}
	prefixBoundary := uint64(len(cst.source))
	if startIndex < len(cst.lines) {
		prefixBoundary = cst.lines[startIndex].Span.Start.Offset
	}
	suffixBoundary := uint64(len(cst.source))
	if endExclusive < len(cst.lines) {
		suffixBoundary = cst.lines[endExclusive].Span.Start.Offset
	}
	newSuffixSigned := int64(suffixBoundary) + delta                                        // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	if newSuffixSigned < int64(prefixBoundary) || newSuffixSigned > int64(len(candidate)) { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return IncrementalParseResult{}, fmt.Errorf("%w: derived incremental envelope", ErrCSTPatch)
	}
	newSuffixBoundary := uint64(newSuffixSigned)
	if suffixBoundary-prefixBoundary > uint64(normalized.MaxRelexBytes) || newSuffixBoundary-prefixBoundary > uint64(normalized.MaxRelexBytes) {
		return IncrementalParseResult{}, fmt.Errorf("%w: relex envelope bytes", ErrCSTPatchLimit)
	}

	starts := sourceLineStarts(candidate)
	region := candidate[prefixBoundary:newSuffixBoundary]
	physical := splitSourceLines(region)
	newLines := make([]CSTLine, 0, len(physical))
	baseLine := uint32(startIndex + 1) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	for _, local := range physical {
		local.startOffset += int(prefixBoundary) // #nosec G115 -- source offset is bounded by validated input or parser state
		local.line += baseLine - 1
		rawEnd := local.startOffset + len(local.text) + local.newlineWidth
		if uint64(rawEnd-local.startOffset) > uint64(normalized.CST.MaxLineBytes) { // #nosec G115 -- source offset is bounded by validated input or parser state
			return IncrementalParseResult{}, fmt.Errorf("%w: relexed line bytes", ErrCSTPatchLimit)
		}
		indent := cstIndent(local.text)
		if uint64(indent) > uint64(normalized.CST.MaxIndentBytes) { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			return IncrementalParseResult{}, fmt.Errorf("%w: relexed indentation", ErrCSTPatchLimit)
		}
		newLines = append(newLines, classifyCSTLine(cst.file, candidate, starts, local, rawEnd, indent))
	}
	lineDelta := int64(len(newLines)) - int64(endExclusive-startIndex)
	offsetDelta := int64(newSuffixBoundary) - int64(suffixBoundary) // #nosec G115 -- source offset is bounded by validated input or parser state
	lines := make([]CSTLine, 0, startIndex+len(newLines)+len(cst.lines)-endExclusive)
	lines = append(lines, cst.lines[:startIndex]...)
	lines = append(lines, newLines...)
	for _, old := range cst.lines[endExclusive:] {
		lines = append(lines, shiftCSTLine(old, offsetDelta, lineDelta))
	}
	if uint64(len(lines)) > uint64(normalized.CST.MaxLines) {
		return IncrementalParseResult{}, fmt.Errorf("%w: candidate lines", ErrCSTPatchLimit)
	}
	updated := &CST{file: cst.file, source: string([]byte(candidate)), lines: lines}
	updated.buildOpaqueNodes()
	oldStarts := sourceLineStarts(cst.source)
	newStarts := sourceLineStarts(candidate)
	result := IncrementalParseResult{
		CST: updated, Semantic: Parse(cst.file, candidate), Changed: true,
		RelexedOld:        Span{File: cst.file, Start: cstPosition(cst.source, oldStarts, int(prefixBoundary)), End: cstPosition(cst.source, oldStarts, int(suffixBoundary))},  // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		RelexedNew:        Span{File: cst.file, Start: cstPosition(candidate, newStarts, int(prefixBoundary)), End: cstPosition(candidate, newStarts, int(newSuffixBoundary))}, // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		ReusedPrefixLines: uint32(startIndex), ReusedSuffixLines: uint32(len(cst.lines) - endExclusive),                                                                        // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	return result, nil
}

func normalizeCSTPatchLimits(limits CSTPatchLimits) (CSTPatchLimits, error) {
	if limits == (CSTPatchLimits{}) {
		return DefaultCSTPatchLimits(), nil
	}
	hard := DefaultCSTPatchLimits()
	if limits.MaxPatches == 0 || limits.MaxPatches > hard.MaxPatches || limits.MaxReplacementBytes == 0 || limits.MaxReplacementBytes > hard.MaxReplacementBytes ||
		limits.MaxRelexBytes == 0 || limits.MaxRelexBytes > hard.CST.MaxBytes {
		return CSTPatchLimits{}, fmt.Errorf("%w: limits are incomplete or exceed hard caps", ErrCSTPatchLimit)
	}
	cst, err := normalizeCSTLimits(limits.CST)
	if err != nil {
		return CSTPatchLimits{}, err
	}
	limits.CST = cst
	return limits, nil
}

func (c *CST) editLineIndex(offset uint64) int {
	index := sort.Search(len(c.lines), func(index int) bool { return c.lines[index].Span.End.Offset > offset })
	if index == len(c.lines) && len(c.lines) != 0 && offset == uint64(len(c.source)) &&
		!strings.HasSuffix(c.lines[len(c.lines)-1].Raw, "\n") {
		return len(c.lines) - 1
	}
	return index
}

func shiftCSTLine(line CSTLine, offsetDelta, lineDelta int64) CSTLine {
	line.Span = shiftCSTSpan(line.Span, offsetDelta, lineDelta)
	line.ContentSpan = shiftCSTSpan(line.ContentSpan, offsetDelta, lineDelta)
	line.ScalarSpan = shiftCSTSpan(line.ScalarSpan, offsetDelta, lineDelta)
	line.CommentSpan = shiftCSTSpan(line.CommentSpan, offsetDelta, lineDelta)
	return line
}

func shiftCSTSpan(span Span, offsetDelta, lineDelta int64) Span {
	if span.File == "" {
		return span
	}
	span.Start = shiftCSTPosition(span.Start, offsetDelta, lineDelta)
	span.End = shiftCSTPosition(span.End, offsetDelta, lineDelta)
	return span
}

func shiftCSTPosition(position Position, offsetDelta, lineDelta int64) Position {
	position.Offset = uint64(int64(position.Offset) + offsetDelta) // #nosec G115 -- source offset is bounded by validated input or parser state
	position.Line = uint32(int64(position.Line) + lineDelta)       // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	return position
}

func sourceBoundary(source string, offset uint64) bool {
	return offset == uint64(len(source)) || offset < uint64(len(source)) && utf8.RuneStart(source[offset])
}

func saturatingCSTAdd(left, right uint64) uint64 {
	if left > ^uint64(0)-right {
		return ^uint64(0)
	}
	return left + right
}

// CSTTriviaPolicy is the fixed ownership contract used by structural planners:
// contiguous same-indent leading comments follow the statement; blank lines
// stay in place; inline comments and the physical newline belong to the line.
type CSTTriviaPolicy struct {
	LeadingCommentsFollowStatement bool
	BlankLinesStayInPlace          bool
	InlineCommentAndNewlineOwned   bool
}

func DefaultCSTTriviaPolicy() CSTTriviaPolicy {
	return CSTTriviaPolicy{LeadingCommentsFollowStatement: true, BlankLinesStayInPlace: true, InlineCommentAndNewlineOwned: true}
}

// OwnedStatementSpan applies DefaultCSTTriviaPolicy to the statement at offset.
func (c *CST) OwnedStatementSpan(offset uint64) (Span, error) {
	index := c.lineIndexAt(offset)
	if index < 0 || c.lines[index].Kind == CSTBlank || c.lines[index].Kind == CSTComment {
		return Span{}, fmt.Errorf("%w: offset does not select a statement", ErrCSTPatch)
	}
	line := c.lines[index]
	startIndex := index
	for startIndex > 0 && c.lines[startIndex-1].Kind == CSTComment && c.lines[startIndex-1].IndentBytes == line.IndentBytes {
		startIndex--
	}
	endExclusive := index + 1
	if line.Kind == CSTNodeStatement || line.Kind == CSTOpaqueStatement {
		for endExclusive < len(c.lines) {
			candidate := c.lines[endExclusive]
			if candidate.Kind == CSTBlank || candidate.Kind == CSTComment && candidate.IndentBytes <= line.IndentBytes ||
				candidate.Kind != CSTComment && candidate.IndentBytes <= line.IndentBytes {
				break
			}
			endExclusive++
		}
	}
	return Span{File: c.file, Start: c.lines[startIndex].Span.Start, End: c.lines[endExclusive-1].Span.End}, nil
}

func (c *CST) lineIndexAt(offset uint64) int {
	if c == nil || offset >= uint64(len(c.source)) {
		return -1
	}
	index := sort.Search(len(c.lines), func(index int) bool { return c.lines[index].Span.End.Offset > offset })
	if index == len(c.lines) || c.lines[index].Span.Start.Offset > offset {
		return -1
	}
	return index
}

// PlanSetPropertyScalar replaces only the scalar bytes, preserving spacing,
// inline comments, line ending, ordering, and all surrounding trivia.
func (c *CST) PlanSetPropertyScalar(propertyOffset uint64, scalarSpelling string) (CSTPatch, error) {
	index := c.lineIndexAt(propertyOffset)
	if index < 0 || c.lines[index].Kind != CSTProperty || c.lines[index].ScalarSpan.File == "" || c.lines[index].ScalarRaw == "" {
		return CSTPatch{}, fmt.Errorf("%w: offset does not select a scalar property", ErrCSTPatch)
	}
	if err := validateScalarSpelling(scalarSpelling); err != nil {
		return CSTPatch{}, err
	}
	return CSTPatch{Span: c.lines[index].ScalarSpan, Replacement: scalarSpelling}, nil
}

func validateScalarSpelling(value string) error {
	if value == "" || strings.ContainsAny(value, "\r\n") || !utf8.ValidString(value) {
		return fmt.Errorf("%w: replacement is not one scalar", ErrCSTPatch)
	}
	lexed := Lex("<scalar>", "value: "+value+"\n")
	if len(lexed.Diagnostics) != 0 || len(lexed.Tokens) < 5 || !isScalarToken(lexed.Tokens[2].Kind) || lexed.Tokens[3].Kind != TokenNewline {
		return fmt.Errorf("%w: replacement is not one valid scalar", ErrCSTPatch)
	}
	return nil
}

func (c *CST) PlanMoveStatement(statementOffset, destinationOffset uint64) ([]CSTPatch, error) {
	owned, err := c.OwnedStatementSpan(statementOffset)
	if err != nil {
		return nil, err
	}
	if !c.statementBoundary(destinationOffset) || destinationOffset > uint64(len(c.source)) ||
		destinationOffset > owned.Start.Offset && destinationOffset < owned.End.Offset {
		return nil, fmt.Errorf("%w: move destination is not an external line boundary", ErrCSTPatch)
	}
	if destinationOffset == owned.Start.Offset || destinationOffset == owned.End.Offset {
		return nil, nil
	}
	text := c.source[owned.Start.Offset:owned.End.Offset]
	return []CSTPatch{{Span: owned}, {Span: zeroWidthSpan(c.file, c.source, destinationOffset), Replacement: text}}, nil
}

func (c *CST) PlanWrapStatement(statementOffset uint64, wrapperHeader string) (CSTPatch, error) {
	owned, err := c.OwnedStatementSpan(statementOffset)
	if err != nil {
		return CSTPatch{}, err
	}
	index := c.lineIndexAt(statementOffset)
	header, err := validateHeader(wrapperHeader)
	if err != nil {
		return CSTPatch{}, err
	}
	indent := c.source[c.lines[index].Span.Start.Offset:c.lines[index].ContentSpan.Start.Offset]
	newline := cstLineEnding(c.lines[index].Raw)
	if newline == "" {
		newline = "\n"
	}
	body := indentCSTBlock(c.source[owned.Start.Offset:owned.End.Offset], "  ")
	return CSTPatch{Span: owned, Replacement: indent + header + newline + body}, nil
}

func (c *CST) PlanUnwrapStatement(statementOffset uint64) (CSTPatch, error) {
	index := c.lineIndexAt(statementOffset)
	if index < 0 || c.lines[index].Kind != CSTNodeStatement {
		return CSTPatch{}, fmt.Errorf("%w: unwrap requires a known node statement", ErrCSTPatch)
	}
	line := c.lines[index]
	endExclusive := index + 1
	for endExclusive < len(c.lines) {
		candidate := c.lines[endExclusive]
		if candidate.Kind != CSTBlank && candidate.Kind != CSTComment && candidate.IndentBytes <= line.IndentBytes {
			break
		}
		endExclusive++
	}
	if endExclusive == index+1 {
		return CSTPatch{}, fmt.Errorf("%w: wrapper has no indented children", ErrCSTPatch)
	}
	span := Span{File: c.file, Start: line.Span.Start, End: c.lines[endExclusive-1].Span.End}
	descendants := c.source[line.Span.End.Offset:span.End.Offset]
	dedented, err := dedentCSTBlock(descendants, int(line.IndentBytes), 2)
	if err != nil {
		return CSTPatch{}, err
	}
	return CSTPatch{Span: span, Replacement: dedented}, nil
}

// PlanExtractStatement replaces a statement with replacementHeader and inserts
// an extractedHeader containing the owned statement at a separate line boundary.
func (c *CST) PlanExtractStatement(statementOffset, destinationOffset uint64, replacementHeader, extractedHeader string) ([]CSTPatch, error) {
	owned, err := c.OwnedStatementSpan(statementOffset)
	if err != nil {
		return nil, err
	}
	replacement, err := validateHeader(replacementHeader)
	if err != nil {
		return nil, err
	}
	extracted, err := validateHeader(extractedHeader)
	if err != nil {
		return nil, err
	}
	if !c.statementBoundary(destinationOffset) || destinationOffset >= owned.Start.Offset && destinationOffset <= owned.End.Offset {
		return nil, fmt.Errorf("%w: extract destination is not an external line boundary", ErrCSTPatch)
	}
	statementIndex := c.lineIndexAt(statementOffset)
	statementIndent := c.source[c.lines[statementIndex].Span.Start.Offset:c.lines[statementIndex].ContentSpan.Start.Offset]
	destinationIndent := ""
	if destinationOffset < uint64(len(c.source)) {
		destinationIndex := c.lineIndexAt(destinationOffset)
		if destinationIndex < 0 || c.lines[destinationIndex].Span.Start.Offset != destinationOffset {
			return nil, fmt.Errorf("%w: extract destination must begin a line", ErrCSTPatch)
		}
		destinationIndent = c.source[c.lines[destinationIndex].Span.Start.Offset:c.lines[destinationIndex].ContentSpan.Start.Offset]
	}
	newline := cstLineEnding(c.lines[statementIndex].Raw)
	if newline == "" {
		newline = "\n"
	}
	ownedText := c.source[owned.Start.Offset:owned.End.Offset]
	reindented := reindentCSTBlock(ownedText, statementIndent, destinationIndent+"  ")
	useText := statementIndent + replacement + newline
	extractedText := destinationIndent + extracted + newline + reindented
	return []CSTPatch{
		{Span: owned, Replacement: useText},
		{Span: zeroWidthSpan(c.file, c.source, destinationOffset), Replacement: extractedText},
	}, nil
}

func (c *CST) statementBoundary(offset uint64) bool {
	if offset == uint64(len(c.source)) {
		return true
	}
	index := c.lineIndexAt(offset)
	return index >= 0 && c.lines[index].Span.Start.Offset == offset
}

func zeroWidthSpan(file, source string, offset uint64) Span {
	position := cstPosition(source, sourceLineStarts(source), int(offset)) // #nosec G115 -- source offset is bounded by validated input or parser state
	return Span{File: file, Start: position, End: position}
}

func validateHeader(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") || !strings.HasSuffix(value, ":") || strings.HasPrefix(value, "#") {
		return "", fmt.Errorf("%w: structural header must be one colon-terminated line", ErrCSTPatch)
	}
	return value, nil
}

func cstLineEnding(raw string) string {
	if strings.HasSuffix(raw, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(raw, "\n") {
		return "\n"
	}
	return ""
}

func indentCSTBlock(source, indentation string) string {
	lines := splitSourceLines(source)
	var builder strings.Builder
	for _, line := range lines {
		if line.text != "" {
			builder.WriteString(indentation)
		}
		builder.WriteString(source[line.startOffset : line.startOffset+len(line.text)+line.newlineWidth])
	}
	return builder.String()
}

func dedentCSTBlock(source string, parentIndent, amount int) (string, error) {
	lines := splitSourceLines(source)
	var builder strings.Builder
	for _, line := range lines {
		rawEnd := line.startOffset + len(line.text) + line.newlineWidth
		if strings.TrimSpace(line.text) == "" {
			builder.WriteString(source[line.startOffset:rawEnd])
			continue
		}
		if len(line.text) < parentIndent+amount || line.text[parentIndent:parentIndent+amount] != strings.Repeat(" ", amount) {
			return "", fmt.Errorf("%w: child indentation is not the canonical %d-space step", ErrCSTPatch, amount)
		}
		builder.WriteString(line.text[:parentIndent])
		builder.WriteString(line.text[parentIndent+amount:])
		builder.WriteString(source[line.startOffset+len(line.text) : rawEnd])
	}
	return builder.String(), nil
}

func reindentCSTBlock(source, oldIndent, newIndent string) string {
	lines := splitSourceLines(source)
	var builder strings.Builder
	for _, line := range lines {
		rawEnd := line.startOffset + len(line.text) + line.newlineWidth
		if strings.TrimSpace(line.text) == "" {
			builder.WriteString(source[line.startOffset:rawEnd])
			continue
		}
		text := line.text
		text = strings.TrimPrefix(text, oldIndent)
		builder.WriteString(newIndent)
		builder.WriteString(text)
		builder.WriteString(source[line.startOffset+len(line.text) : rawEnd])
	}
	return builder.String()
}
