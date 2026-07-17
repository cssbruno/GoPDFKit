// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"reflect"
	"testing"
)

func TestLexEmitsIndentationAndTypedScalarTokens(t *testing.T) {
	source := "document @invoice:\n  enabled: true\n  width: 210mm\n  title: \"Hi # still text\" # comment\n"
	result := Lex("invoice.paper", source)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Lex diagnostics = %#v", result.Diagnostics)
	}
	kinds := make([]TokenKind, len(result.Tokens))
	lexemes := make([]string, len(result.Tokens))
	for index, token := range result.Tokens {
		kinds[index] = token.Kind
		lexemes[index] = token.Lexeme
	}
	wantKinds := []TokenKind{
		TokenIdentifier, TokenReadableID, TokenColon, TokenNewline,
		TokenIndent, TokenIdentifier, TokenColon, TokenBool, TokenNewline,
		TokenIdentifier, TokenColon, TokenUnit, TokenNewline,
		TokenIdentifier, TokenColon, TokenString, TokenNewline,
		TokenDedent, TokenEOF,
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("token kinds = %#v, want %#v; lexemes=%#v", kinds, wantKinds, lexemes)
	}
	if got := result.Tokens[15]; got.Lexeme != `"Hi # still text"` || got.Span.Start.Offset != 59 || got.Span.End.Offset != 76 {
		t.Fatalf("string token = %#v", got)
	}
}

func TestLexBlankAndCommentLinesDoNotChangeIndentation(t *testing.T) {
	source := "document:\n  # comment\n\n  page:\n    body:\n      text: \"ok\"\n"
	result := Lex("blank.paper", source)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Lex diagnostics = %#v", result.Diagnostics)
	}
	indentCount, dedentCount := 0, 0
	for _, token := range result.Tokens {
		switch token.Kind {
		case TokenIndent:
			indentCount++
		case TokenDedent:
			dedentCount++
		}
	}
	if indentCount != 3 || dedentCount != 3 {
		t.Fatalf("indent/dedent count = %d/%d, want 3/3", indentCount, dedentCount)
	}
}

func TestLexPositionsUseByteOffsetsAndRuneColumns(t *testing.T) {
	source := "document:\n  title: \"café\"\n"
	result := Lex("unicode.paper", source)
	var stringToken Token
	for _, token := range result.Tokens {
		if token.Kind == TokenString {
			stringToken = token
			break
		}
	}
	if stringToken.Lexeme != `"café"` {
		t.Fatalf("string token = %#v", stringToken)
	}
	if got, want := stringToken.Span.Start, (Position{Offset: 19, Line: 2, Column: 10}); got != want {
		t.Fatalf("start = %#v, want %#v", got, want)
	}
	if got, want := stringToken.Span.End, (Position{Offset: 26, Line: 2, Column: 16}); got != want {
		t.Fatalf("end = %#v, want %#v", got, want)
	}
}

func TestLexCRLFNewlineSpansRemainExact(t *testing.T) {
	source := "document:\r\n  page:\r\n    body:\r\n      text: \"ok\"\r\n"
	result := Lex("windows.paper", source)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Lex diagnostics = %#v", result.Diagnostics)
	}
	var newlines []Token
	for _, token := range result.Tokens {
		if token.Kind == TokenNewline {
			newlines = append(newlines, token)
		}
	}
	if len(newlines) != 4 {
		t.Fatalf("newline count = %d, want 4", len(newlines))
	}
	for _, token := range newlines {
		if token.Lexeme != "\r\n" || token.Span.End.Offset-token.Span.Start.Offset != 2 {
			t.Fatalf("CRLF token = %#v", token)
		}
	}
	parsed := Parse("windows.paper", source)
	if !parsed.OK() || parsed.AST.Root.Span.End.Offset != uint64(len(source)-2) {
		t.Fatalf("CRLF parse = root %#v, diagnostics %#v", parsed.AST.Root, parsed.Diagnostics)
	}
}

func TestLexReportsIndentationAndLexemeProblems(t *testing.T) {
	source := "document:\n\tpage:\n   body:\n     text: \"unterminated\n  bad: @9x\n"
	result := Lex("bad.paper", source)
	codes := diagnosticCodes(result.Diagnostics)
	for _, want := range []string{"PAPER_TAB_INDENT", "PAPER_INCONSISTENT_INDENT", "PAPER_UNTERMINATED_STRING", "PAPER_INVALID_ID"} {
		if !codes[want] {
			t.Fatalf("diagnostic codes = %#v, want %s", codes, want)
		}
	}
}

func diagnosticCodes(diagnostics []Diagnostic) map[string]bool {
	result := make(map[string]bool, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result[diagnostic.Code] = true
	}
	return result
}
