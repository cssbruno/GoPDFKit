// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"strings"
	"unicode/utf8"
)

// TokenKind identifies lexical .paper elements.
type TokenKind string

const (
	TokenIdentifier TokenKind = "identifier"
	TokenReadableID TokenKind = "readable_id"
	TokenColon      TokenKind = "colon"
	TokenString     TokenKind = "string"
	TokenBool       TokenKind = "bool"
	TokenNumber     TokenKind = "number"
	TokenUnit       TokenKind = "unit"
	TokenNull       TokenKind = "null"
	TokenIndent     TokenKind = "indent"
	TokenDedent     TokenKind = "dedent"
	TokenNewline    TokenKind = "newline"
	TokenInvalid    TokenKind = "invalid"
	TokenEOF        TokenKind = "eof"
)

// Token retains exact source spelling and its half-open source span.
type Token struct {
	Kind   TokenKind `json:"kind"`
	Lexeme string    `json:"lexeme"`
	Span   Span      `json:"span"`
}

// LexResult contains a complete token stream, including EOF and indentation
// tokens, plus recoverable diagnostics.
type LexResult struct {
	Tokens      []Token
	Diagnostics []Diagnostic
}

type sourceLine struct {
	text         string
	startOffset  int
	line         uint32
	newlineWidth int
}

// Lex tokenizes source without applying the document-node grammar.
func Lex(file, source string) LexResult {
	lexer := paperLexer{file: file, source: source, indents: []int{0}, lineStarts: sourceLineStarts(source)}
	for _, line := range splitSourceLines(source) {
		lexer.lexLine(line)
	}
	eof := lexer.position(len(source))
	for len(lexer.indents) > 1 {
		lexer.indents = lexer.indents[:len(lexer.indents)-1]
		lexer.emit(TokenDedent, "", eof, eof)
	}
	lexer.emit(TokenEOF, "", eof, eof)
	return LexResult{Tokens: lexer.tokens, Diagnostics: lexer.diagnostics}
}

type paperLexer struct {
	file        string
	source      string
	indents     []int
	tokens      []Token
	diagnostics []Diagnostic
	lineStarts  []int
}

func (l *paperLexer) lexLine(line sourceLine) {
	indent := 0
	for indent < len(line.text) {
		switch line.text[indent] {
		case ' ':
			indent++
		case '\t':
			start := line.startOffset + indent
			end := start + 1
			l.diagnostics = append(l.diagnostics, errorDiagnostic(
				"PAPER_TAB_INDENT", "tabs are not allowed in indentation", "replace the tab with spaces", l.span(start, end),
			))
			indent++
		default:
			goto indentationDone
		}
	}

indentationDone:
	trimmed := strings.TrimSpace(line.text[indent:])
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		l.emitLineNewline(line)
		return
	}

	contentOffset := line.startOffset + indent
	l.applyIndent(indent, contentOffset)
	for cursor := indent; cursor < len(line.text); {
		for cursor < len(line.text) && (line.text[cursor] == ' ' || line.text[cursor] == '\t') {
			cursor++
		}
		if cursor >= len(line.text) || line.text[cursor] == '#' {
			break
		}
		start := cursor
		switch character := line.text[cursor]; {
		case character == ':':
			cursor++
			l.emitOffsets(TokenColon, line.text[start:cursor], line.startOffset+start, line.startOffset+cursor)
		case character == '@':
			cursor++
			for cursor < len(line.text) && isIDContinue(line.text[cursor]) {
				cursor++
			}
			lexeme := line.text[start:cursor]
			kind := TokenReadableID
			if !validReadableID(lexeme) {
				kind = TokenInvalid
				l.diagnostics = append(l.diagnostics, errorDiagnostic(
					"PAPER_INVALID_ID", "readable IDs must look like @invoice-lines", "start with a letter and use letters, digits, '-' or '_'", l.span(line.startOffset+start, line.startOffset+cursor),
				))
			}
			l.emitOffsets(kind, lexeme, line.startOffset+start, line.startOffset+cursor)
		case character == '"':
			cursor = l.lexString(line, cursor)
		case isIdentifierStart(character):
			cursor++
			for cursor < len(line.text) && isIdentifierContinue(line.text[cursor]) {
				cursor++
			}
			lexeme := line.text[start:cursor]
			kind := TokenIdentifier
			switch lexeme {
			case "true", "false":
				kind = TokenBool
			case "null":
				kind = TokenNull
			}
			l.emitOffsets(kind, lexeme, line.startOffset+start, line.startOffset+cursor)
		case character == '+' || character == '-' || character >= '0' && character <= '9':
			cursor = l.lexNumber(line, cursor)
		default:
			_, size := utf8.DecodeRuneInString(line.text[cursor:])
			if size <= 0 {
				size = 1
			}
			cursor += size
			l.emitOffsets(TokenInvalid, line.text[start:cursor], line.startOffset+start, line.startOffset+cursor)
			l.diagnostics = append(l.diagnostics, errorDiagnostic(
				"PAPER_UNEXPECTED_CHARACTER", "unexpected character in .paper source", "use a name, @id, quoted string, boolean, number, or unit value", l.span(line.startOffset+start, line.startOffset+cursor),
			))
		}
	}
	l.emitLineNewline(line)
}

func (l *paperLexer) applyIndent(indent, offset int) {
	current := l.indents[len(l.indents)-1]
	position := l.position(offset)
	switch {
	case indent > current:
		l.indents = append(l.indents, indent)
		l.emit(TokenIndent, "", position, position)
	case indent < current:
		for len(l.indents) > 1 && indent < l.indents[len(l.indents)-1] {
			l.indents = l.indents[:len(l.indents)-1]
			l.emit(TokenDedent, "", position, position)
		}
		if indent != l.indents[len(l.indents)-1] {
			l.diagnostics = append(l.diagnostics, errorDiagnostic(
				"PAPER_INCONSISTENT_INDENT", "indentation does not match an enclosing block", "align this line with an earlier indentation level", l.span(offset, offset),
			))
			l.indents = append(l.indents, indent)
			l.emit(TokenIndent, "", position, position)
		}
	}
}

func (l *paperLexer) lexString(line sourceLine, cursor int) int {
	start := cursor
	cursor++
	escaped := false
	closed := false
	for cursor < len(line.text) {
		character := line.text[cursor]
		cursor++
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			closed = true
			break
		}
	}
	kind := TokenString
	if !closed {
		kind = TokenInvalid
		l.diagnostics = append(l.diagnostics, errorDiagnostic(
			"PAPER_UNTERMINATED_STRING", "quoted string is not terminated", "add a closing double quote", l.span(line.startOffset+start, line.startOffset+cursor),
		))
	}
	l.emitOffsets(kind, line.text[start:cursor], line.startOffset+start, line.startOffset+cursor)
	return cursor
}

func (l *paperLexer) lexNumber(line sourceLine, cursor int) int {
	start := cursor
	if line.text[cursor] == '+' || line.text[cursor] == '-' {
		cursor++
	}
	digits := 0
	for cursor < len(line.text) && line.text[cursor] >= '0' && line.text[cursor] <= '9' {
		cursor++
		digits++
	}
	if cursor < len(line.text) && line.text[cursor] == '.' {
		cursor++
		for cursor < len(line.text) && line.text[cursor] >= '0' && line.text[cursor] <= '9' {
			cursor++
			digits++
		}
	}
	if digits == 0 {
		l.emitOffsets(TokenInvalid, line.text[start:cursor], line.startOffset+start, line.startOffset+cursor)
		l.diagnostics = append(l.diagnostics, errorDiagnostic(
			"PAPER_INVALID_NUMBER", "number requires at least one digit", "write a decimal number such as 12 or -0.5", l.span(line.startOffset+start, line.startOffset+cursor),
		))
		return cursor
	}
	unitStart := cursor
	for cursor < len(line.text) && ((line.text[cursor] >= 'a' && line.text[cursor] <= 'z') || line.text[cursor] == '%') {
		cursor++
	}
	kind := TokenNumber
	if cursor > unitStart {
		kind = TokenUnit
	}
	l.emitOffsets(kind, line.text[start:cursor], line.startOffset+start, line.startOffset+cursor)
	return cursor
}

func (l *paperLexer) emitLineNewline(line sourceLine) {
	start := line.startOffset + len(line.text)
	end := start + line.newlineWidth
	lexeme := l.source[start:end]
	l.emitOffsets(TokenNewline, lexeme, start, end)
}

func (l *paperLexer) emitOffsets(kind TokenKind, lexeme string, start, end int) {
	l.emit(kind, lexeme, l.position(start), l.position(end))
}

func (l *paperLexer) emit(kind TokenKind, lexeme string, start, end Position) {
	l.tokens = append(l.tokens, Token{Kind: kind, Lexeme: lexeme, Span: Span{File: l.file, Start: start, End: end}})
}

func (l *paperLexer) span(start, end int) Span {
	return Span{File: l.file, Start: l.position(start), End: l.position(end)}
}

func (l *paperLexer) position(offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(l.source) {
		offset = len(l.source)
	}
	lineIndex := 0
	low, high := 0, len(l.lineStarts)
	for low < high {
		middle := low + (high-low)/2
		if l.lineStarts[middle] <= offset {
			lineIndex = middle
			low = middle + 1
		} else {
			high = middle
		}
	}
	lineStart := l.lineStarts[lineIndex]
	column := uint32(utf8.RuneCountInString(l.source[lineStart:offset]) + 1)
	return Position{Offset: uint64(offset), Line: uint32(lineIndex + 1), Column: column}
}

func sourceLineStarts(source string) []int {
	starts := make([]int, 1, strings.Count(source, "\n")+1)
	for index := 0; index < len(source); index++ {
		if source[index] == '\n' {
			starts = append(starts, index+1)
		}
	}
	return starts
}

func splitSourceLines(source string) []sourceLine {
	if source == "" {
		return nil
	}
	lines := make([]sourceLine, 0, strings.Count(source, "\n")+1)
	start := 0
	lineNumber := uint32(1)
	for start < len(source) {
		newline := strings.IndexByte(source[start:], '\n')
		if newline < 0 {
			lines = append(lines, sourceLine{text: source[start:], startOffset: start, line: lineNumber})
			break
		}
		newline += start
		textEnd := newline
		newlineWidth := 1
		if textEnd > start && source[textEnd-1] == '\r' {
			textEnd--
			newlineWidth = 2
		}
		lines = append(lines, sourceLine{text: source[start:textEnd], startOffset: start, line: lineNumber, newlineWidth: newlineWidth})
		start = newline + 1
		lineNumber++
	}
	return lines
}

func isIdentifierStart(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character == '_'
}

func isIdentifierContinue(character byte) bool {
	return isIdentifierStart(character) || character >= '0' && character <= '9' || character == '-'
}

func isIDContinue(character byte) bool {
	return isIdentifierContinue(character)
}

func validReadableID(value string) bool {
	if len(value) < 2 || value[0] != '@' || !isIdentifierStart(value[1]) || value[1] == '_' {
		return false
	}
	for index := 2; index < len(value); index++ {
		if !isIDContinue(value[index]) {
			return false
		}
	}
	return true
}
