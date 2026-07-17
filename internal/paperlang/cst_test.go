// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestCSTLosslessPrintPreservesTriviaOrderSpellingAndNewlines(t *testing.T) {
	t.Parallel()

	source := "# leading comment\r\n" +
		"\r\n" +
		"document @doc: # document comment\r\n" +
		"  title: \"A # B\" # inline comment\r\n" +
		"  language: \"pt-BR\"\r\n" +
		"  page:\r\n" +
		"    body:\r\n" +
		"      paragraph:\r\n" +
		"        size: +0012.00pt\r\n" +
		"        font: \"Helvetica\"\r\n" +
		"        text: \"hello\""
	cst, err := ParseCST("lossless.paper", source)
	if err != nil {
		t.Fatal(err)
	}
	printed, err := PrintLossless(cst)
	if err != nil || !bytes.Equal(printed, []byte(source)) {
		t.Fatalf("lossless print differs: %q, %v", printed, err)
	}
	lines := cst.Lines()
	if len(lines) != 11 || lines[0].Kind != CSTComment || lines[1].Kind != CSTBlank || lines[2].Kind != CSTNodeStatement {
		t.Fatalf("line trivia = %#v", lines)
	}
	if lines[3].Kind != CSTProperty || lines[3].Name != "title" || lines[3].ScalarRaw != `"A # B"` || lines[3].CommentSpan.Start.Offset == 0 {
		t.Fatalf("inline property projection = %#v", lines[3])
	}
	if lines[8].ScalarRaw != "+0012.00pt" || lines[8].Raw != "        size: +0012.00pt\r\n" {
		t.Fatalf("scalar spelling/newline = %#v", lines[8])
	}
	if lines[8].Name != "size" || lines[9].Name != "font" {
		t.Fatalf("property order changed: %#v / %#v", lines[8], lines[9])
	}

	// Returned bytes and line slices are detached.
	printed[0] = '!'
	lines[3].Raw = "mutated"
	again, _ := PrintLossless(cst)
	if string(again) != source || cst.Lines()[3].Raw == "mutated" {
		t.Fatal("CST snapshot was mutated through a returned projection")
	}
}

func TestCSTPreservesUnknownFutureStatementsAsOpaqueSubtrees(t *testing.T) {
	t.Parallel()

	const source = `document @doc:
  page:
    body:
      future-widget @chart: # unknown header
        mode: "dense"
        future-child:
          raw: 001
      paragraph:
        text: "known"
`
	parsed := ParseLossless("future.paper", source)
	if !parsed.OK() || parsed.Semantic.OK() {
		t.Fatalf("lossless/semantic status = %t/%t, %#v", parsed.OK(), parsed.Semantic.OK(), parsed.Semantic.Diagnostics)
	}
	opaque := parsed.CST.OpaqueNodes()
	if len(opaque) < 1 || opaque[0].Name != "future-widget" || !strings.Contains(opaque[0].Raw, "future-child") || strings.Contains(opaque[0].Raw, "paragraph") {
		t.Fatalf("opaque regions = %#v", opaque)
	}
	childOffset := uint64(strings.Index(source, "raw: 001"))
	owner, ok := parsed.CST.OpaqueAtOffset(childOffset)
	if !ok || owner.Name != "future-widget" {
		t.Fatalf("opaque lookup = %#v, %t", owner, ok)
	}
	printed, err := PrintParsed(parsed, SourcePrintOptions{})
	if err != nil || string(printed) != source {
		t.Fatalf("opaque no-op print = %q, %v", printed, err)
	}
	if _, err := PrintParsed(parsed, SourcePrintOptions{Canonical: true}); !errors.Is(err, ErrCSTInvalid) {
		t.Fatalf("canonical invalid semantic source = %v", err)
	}
}

func TestCSTSourceSpanLookupUsesExactUTF8ByteRanges(t *testing.T) {
	t.Parallel()

	const source = "# Olá 🚀\ndocument:\n  title: \"Olá\"\n  page:\n    body:\n      text: \"x\"\n"
	cst, err := ParseCST("utf8.paper", source)
	if err != nil {
		t.Fatal(err)
	}
	titleOffset := uint64(strings.Index(source, "title"))
	line, ok := cst.LookupOffset(titleOffset)
	if !ok || line.Name != "title" || line.Span.Start.Line != 3 || line.ContentSpan.Start.Column != 3 {
		t.Fatalf("offset lookup = %#v, %t", line, ok)
	}
	selected := cst.LookupSpan(Span{File: "utf8.paper", Start: line.ContentSpan.Start, End: line.ContentSpan.End})
	if len(selected) != 1 || selected[0].Raw != line.Raw {
		t.Fatalf("span lookup = %#v", selected)
	}
	if got := cst.LookupSpan(Span{File: "other.paper", Start: line.Span.Start, End: line.Span.End}); got != nil {
		t.Fatalf("cross-file lookup = %#v", got)
	}
}

func TestCSTLimitsAreCompleteAndEnforced(t *testing.T) {
	t.Parallel()

	const source = "document:\n  page:\n    body:\n      text: \"x\"\n"
	tests := []struct {
		name   string
		limits CSTLimits
	}{
		{name: "partial", limits: CSTLimits{MaxBytes: 1}},
		{name: "bytes", limits: func() CSTLimits { value := DefaultCSTLimits(); value.MaxBytes = uint32(len(source) - 1); return value }()},
		{name: "lines", limits: func() CSTLimits { value := DefaultCSTLimits(); value.MaxLines = 2; return value }()},
		{name: "line-bytes", limits: func() CSTLimits { value := DefaultCSTLimits(); value.MaxLineBytes = 4; return value }()},
		{name: "indent", limits: func() CSTLimits { value := DefaultCSTLimits(); value.MaxIndentBytes = 2; return value }()},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseCSTWithLimits("limits.paper", source, test.limits); !errors.Is(err, ErrCSTLimit) {
				t.Fatalf("limit error = %v", err)
			}
		})
	}
	if _, err := ParseCST("invalid.paper", string([]byte{0xff})); !errors.Is(err, ErrCSTInvalid) {
		t.Fatalf("UTF-8 error = %v", err)
	}
}

func TestPrintParsedKeepsCanonicalFormattingExplicit(t *testing.T) {
	t.Parallel()

	const source = `# keep unless canonical is requested
document @doc:
  title: "Example"
  language: "en"

  page:
    body:
      paragraph:
        size: +0012.00pt
        font: "Helvetica"
        text: "hello"
`
	parsed := ParseLossless("format-mode.paper", source)
	if !parsed.OK() || !parsed.Semantic.OK() {
		t.Fatalf("parse = %#v / %#v", parsed.Err, parsed.Semantic.Diagnostics)
	}
	preserved, err := PrintParsed(parsed, SourcePrintOptions{})
	if err != nil || string(preserved) != source {
		t.Fatalf("default print changed source: %q, %v", preserved, err)
	}
	canonical, err := PrintParsed(parsed, SourcePrintOptions{Canonical: true})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(canonical, []byte(source)) || bytes.Contains(canonical, []byte("# keep")) || !bytes.Contains(canonical, []byte("        font: \"Helvetica\"\n        size: 12pt\n")) {
		t.Fatalf("canonical output =\n%s", canonical)
	}
}

func TestCSTPreservesTypedComponentContractsExactly(t *testing.T) {
	t.Parallel()
	const source = "# component contract\r\ndocument:\r\n  component @card:\r\n    prop @title: # public prop\r\n      type: \"string\"\r\n      required: true\r\n    slot @body:\r\n      cardinality: \"one\"\r\n      layout-affecting: true\r\n      scenarios: \"@compact, @expanded\"\r\n  page:\r\n    body:\r\n      use @one:\r\n        component: \"@card\"\r\n        arg @title: \"Hello\"\r\n"
	parsed := ParseLossless("component-contract.paper", source)
	if !parsed.OK() || !parsed.Semantic.OK() {
		t.Fatalf("parse = %v / %+v", parsed.Err, parsed.Semantic.Diagnostics)
	}
	printed, err := PrintLossless(parsed.CST)
	if err != nil || string(printed) != source {
		t.Fatalf("lossless contract = %q, %v", printed, err)
	}
}
