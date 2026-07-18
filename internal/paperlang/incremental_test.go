// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

const incrementalFixture = `# file comment
document @doc:
  title: "Old" # preserve inline
  language: "en"
  page:
    body:
      text @lead: "lead"
      # paragraph comment
      paragraph @first:
        size: 12pt
        text: "Olá"

      paragraph @second:
        text: "Second"
`

func TestIncrementalCSTScalarPatchMatchesCleanParseAndReusesLines(t *testing.T) {
	t.Parallel()

	cst, err := ParseCST("incremental.paper", incrementalFixture)
	if err != nil {
		t.Fatal(err)
	}
	propertyOffset := uint64(strings.Index(incrementalFixture, "title:"))
	patch, err := cst.PlanSetPropertyScalar(propertyOffset, `"New"`)
	if err != nil {
		t.Fatal(err)
	}
	if got := incrementalFixture[patch.Span.Start.Offset:patch.Span.End.Offset]; got != `"Old"` {
		t.Fatalf("scalar patch range = %q", got)
	}
	result, err := ApplyCSTPatches(cst, []CSTPatch{patch}, CSTPatchLimits{})
	if err != nil {
		t.Fatal(err)
	}
	printed, _ := PrintLossless(result.CST)
	want := strings.Replace(incrementalFixture, `title: "Old"`, `title: "New"`, 1)
	if string(printed) != want || !strings.Contains(string(printed), `# preserve inline`) {
		t.Fatalf("incremental source = %q", printed)
	}
	if string(printed[:patch.Span.Start.Offset]) != incrementalFixture[:patch.Span.Start.Offset] ||
		string(printed[patch.Span.Start.Offset+uint64(len(patch.Replacement)):]) != incrementalFixture[patch.Span.End.Offset:] {
		t.Fatal("bytes outside the minimal scalar patch changed")
	}
	if !result.Changed || result.ReusedPrefixLines == 0 || result.ReusedSuffixLines == 0 || result.RelexedNew.End.Line-result.RelexedNew.Start.Line > 2 {
		t.Fatalf("incremental reuse evidence = %#v", result)
	}
	assertIncrementalEqualsClean(t, result)
	if !result.Semantic.OK() {
		t.Fatalf("semantic diagnostics = %#v", result.Semantic.Diagnostics)
	}
}

func TestIncrementalCSTPreservesCRLFAndShiftsOpaqueSpansLikeCleanParse(t *testing.T) {
	t.Parallel()

	source := "document:\r\n" +
		"  title: \"A\" # inline\r\n" +
		"  future-widget @x:\r\n" +
		"    raw: 001\r\n" +
		"  page:\r\n" +
		"    body:\r\n" +
		"      text: \"x\"\r\n"
	cst, err := ParseCST("opaque-edit.paper", source)
	if err != nil || len(cst.OpaqueNodes()) != 1 {
		t.Fatalf("parse = %v, opaque=%#v", err, cst.OpaqueNodes())
	}
	patch, err := cst.PlanSetPropertyScalar(uint64(strings.Index(source, "title:")), `"Longer"`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyCSTPatches(cst, []CSTPatch{patch}, CSTPatchLimits{})
	if err != nil {
		t.Fatal(err)
	}
	assertIncrementalEqualsClean(t, result)
	printed, _ := PrintLossless(result.CST)
	if bytes.Count(printed, []byte("\r\n")) != bytes.Count([]byte(source), []byte("\r\n")) || bytes.Contains(printed, []byte("\n")) && bytes.Contains(bytes.ReplaceAll(printed, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatalf("line endings changed: %q", printed)
	}
	before, after := cst.OpaqueNodes()[0], result.CST.OpaqueNodes()[0]
	if before.Name != after.Name || before.Raw != after.Raw || after.Span.Start.Offset-before.Span.Start.Offset != uint64(len(`"Longer"`)-len(`"A"`)) {
		t.Fatalf("opaque span shift = %#v / %#v", before, after)
	}
}

func TestIncrementalCSTMultipleMinimalPatchesAndUTF8Spans(t *testing.T) {
	t.Parallel()

	cst, err := ParseCST("multiple.paper", incrementalFixture)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cst.PlanSetPropertyScalar(uint64(strings.Index(incrementalFixture, "size:")), "14pt")
	if err != nil {
		t.Fatal(err)
	}
	second, err := cst.PlanSetPropertyScalar(uint64(strings.Index(incrementalFixture, "language:")), `"fr"`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyCSTPatches(cst, []CSTPatch{second, first}, CSTPatchLimits{})
	if err != nil {
		t.Fatal(err)
	}
	assertIncrementalEqualsClean(t, result)
	printed, _ := PrintLossless(result.CST)
	if !bytes.Contains(printed, []byte("size: 14pt")) || !bytes.Contains(printed, []byte(`language: "fr"`)) || !bytes.Contains(printed, []byte(`text: "Olá"`)) {
		t.Fatalf("multi-patch source = %s", printed)
	}
}

func TestIncrementalTypedComponentContractMatchesCleanParse(t *testing.T) {
	t.Parallel()
	const source = "document:\n  component @card:\n    prop @title:\n      type: \"string\" # edit type\n      required: true\n    text: \"${title}\"\n  page:\n    body:\n      use @one:\n        component: \"@card\"\n        arg @title: \"Hello\"\n"
	cst, err := ParseCST("component-edit.paper", source)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := cst.PlanSetPropertyScalar(uint64(strings.Index(source, "type:")), `"any"`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyCSTPatches(cst, []CSTPatch{patch}, CSTPatchLimits{})
	if err != nil {
		t.Fatal(err)
	}
	assertIncrementalEqualsClean(t, result)
	printed, _ := PrintLossless(result.CST)
	if !bytes.Contains(printed, []byte(`type: "any" # edit type`)) || !result.Semantic.OK() {
		t.Fatalf("incremental contract = %s / %+v", printed, result.Semantic.Diagnostics)
	}
}

func TestCSTMoveWrapUnwrapAndExtractUseExplicitTriviaOwnership(t *testing.T) {
	t.Parallel()

	policy := DefaultCSTTriviaPolicy()
	if !policy.LeadingCommentsFollowStatement || !policy.BlankLinesStayInPlace || !policy.InlineCommentAndNewlineOwned {
		t.Fatalf("trivia policy = %#v", policy)
	}

	cst, _ := ParseCST("structure.paper", incrementalFixture)
	firstOffset := uint64(strings.Index(incrementalFixture, "paragraph @first"))
	owned, err := cst.OwnedStatementSpan(firstOffset)
	if err != nil {
		t.Fatal(err)
	}
	ownedText := incrementalFixture[owned.Start.Offset:owned.End.Offset]
	if !strings.HasPrefix(ownedText, "      # paragraph comment\n") || strings.HasSuffix(ownedText, "\n\n") {
		t.Fatalf("owned trivia = %q", ownedText)
	}

	move, err := cst.PlanMoveStatement(firstOffset, uint64(len(incrementalFixture)))
	if err != nil {
		t.Fatal(err)
	}
	moved, err := ApplyCSTPatches(cst, move, CSTPatchLimits{})
	if err != nil {
		t.Fatal(err)
	}
	assertIncrementalEqualsClean(t, moved)
	movedSource, _ := PrintLossless(moved.CST)
	if strings.Index(string(movedSource), "paragraph @first") < strings.Index(string(movedSource), "paragraph @second") ||
		!strings.Contains(string(movedSource), "\n\n      paragraph @second") || !moved.Semantic.OK() {
		t.Fatalf("moved source =\n%s\ndiagnostics=%#v", movedSource, moved.Semantic.Diagnostics)
	}

	wrap, err := cst.PlanWrapStatement(firstOffset, "column @wrapper:")
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := ApplyCSTPatches(cst, []CSTPatch{wrap}, CSTPatchLimits{})
	if err != nil || !wrapped.Semantic.OK() {
		t.Fatalf("wrap = %v, %#v", err, wrapped.Semantic.Diagnostics)
	}
	wrappedSource, _ := PrintLossless(wrapped.CST)
	if !strings.Contains(string(wrappedSource), "      column @wrapper:\n        # paragraph comment\n        paragraph @first:") {
		t.Fatalf("wrapped source =\n%s", wrappedSource)
	}
	unwrap, err := wrapped.CST.PlanUnwrapStatement(uint64(strings.Index(string(wrappedSource), "column @wrapper")))
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := ApplyCSTPatches(wrapped.CST, []CSTPatch{unwrap}, CSTPatchLimits{})
	if err != nil {
		t.Fatal(err)
	}
	unwrappedSource, _ := PrintLossless(unwrapped.CST)
	if string(unwrappedSource) != incrementalFixture {
		t.Fatalf("wrap/unwrap was not lossless:\n%s", unwrappedSource)
	}

	pageOffset := uint64(strings.Index(incrementalFixture, "  page:"))
	extract, err := cst.PlanExtractStatement(firstOffset, pageOffset, "page-break @first-placeholder:", "component @extracted-first:")
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := ApplyCSTPatches(cst, extract, CSTPatchLimits{})
	if err != nil || !extracted.Semantic.OK() {
		t.Fatalf("extract = %v, %#v", err, extracted.Semantic.Diagnostics)
	}
	extractedSource, _ := PrintLossless(extracted.CST)
	if !strings.Contains(string(extractedSource), "  component @extracted-first:\n    # paragraph comment\n    paragraph @first:") ||
		!strings.Contains(string(extractedSource), "      page-break @first-placeholder:") {
		t.Fatalf("extracted source =\n%s", extractedSource)
	}
}

func TestIncrementalCSTPatchLimitsRangesAndNoOp(t *testing.T) {
	t.Parallel()

	cst, _ := ParseCST("limits.paper", incrementalFixture)
	title := uint64(strings.Index(incrementalFixture, `"Old"`))
	span := func(start, end uint64) Span {
		starts := sourceLineStarts(incrementalFixture)
		return Span{File: "limits.paper", Start: cstPosition(incrementalFixture, starts, int(start)), End: cstPosition(incrementalFixture, starts, int(end))}
	}
	noOp, err := ApplyCSTPatches(cst, []CSTPatch{{Span: span(title, title+5), Replacement: `"Old"`}}, CSTPatchLimits{})
	if err != nil || noOp.Changed || noOp.CST != cst || noOp.ReusedPrefixLines != uint32(len(cst.lines)) {
		t.Fatalf("no-op result = %#v, %v", noOp, err)
	}

	tests := []struct {
		name    string
		patches []CSTPatch
		limits  CSTPatchLimits
		cause   error
	}{
		{name: "overlap", patches: []CSTPatch{{Span: span(title, title+5)}, {Span: span(title+1, title+2)}}, cause: ErrCSTPatch},
		{name: "utf8-boundary", patches: []CSTPatch{{Span: span(uint64(strings.Index(incrementalFixture, "á")+1), uint64(strings.Index(incrementalFixture, "á")+2))}}, cause: ErrCSTPatch},
		{name: "replacement", patches: []CSTPatch{{Span: span(title, title+5), Replacement: strings.Repeat("x", 20)}}, limits: func() CSTPatchLimits { value := DefaultCSTPatchLimits(); value.MaxReplacementBytes = 10; return value }(), cause: ErrCSTPatchLimit},
		{name: "relex", patches: []CSTPatch{{Span: span(title, title+5), Replacement: `"New"`}}, limits: func() CSTPatchLimits { value := DefaultCSTPatchLimits(); value.MaxRelexBytes = 4; return value }(), cause: ErrCSTPatchLimit},
		{name: "partial-limits", patches: []CSTPatch{{Span: span(title, title+5)}}, limits: CSTPatchLimits{MaxPatches: 1}, cause: ErrCSTPatchLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ApplyCSTPatches(cst, test.patches, test.limits); !errors.Is(err, test.cause) {
				t.Fatalf("patch error = %v, want %v", err, test.cause)
			}
		})
	}
}

func FuzzIncrementalCSTMatchesCleanParse(f *testing.F) {
	f.Add(incrementalFixture, uint16(0), uint16(0), "# inserted\n")
	f.Add(incrementalFixture, uint16(40), uint16(45), `"X"`)
	f.Add("document:\n  page:\n    body:\n      text: \"x\"\n", uint16(10), uint16(10), "# c\n")
	f.Fuzz(func(t *testing.T, source string, rawStart, rawEnd uint16, replacement string) {
		if len(source) == 0 || len(source) > 4096 || len(replacement) > 128 || !utf8.ValidString(source) || !utf8.ValidString(replacement) {
			t.Skip()
		}
		cst, err := ParseCST("fuzz.paper", source)
		if err != nil {
			t.Skip()
		}
		start := int(rawStart) % (len(source) + 1)
		end := int(rawEnd) % (len(source) + 1)
		if start > end {
			start, end = end, start
		}
		for start < len(source) && !utf8.RuneStart(source[start]) {
			start++
		}
		for end < len(source) && !utf8.RuneStart(source[end]) {
			end++
		}
		starts := sourceLineStarts(source)
		patch := CSTPatch{Span: Span{File: "fuzz.paper", Start: cstPosition(source, starts, start), End: cstPosition(source, starts, end)}, Replacement: replacement}
		result, err := ApplyCSTPatches(cst, []CSTPatch{patch}, CSTPatchLimits{})
		if err != nil {
			if errors.Is(err, ErrCSTPatchLimit) || errors.Is(err, ErrCSTPatch) {
				return
			}
			t.Fatal(err)
		}
		assertIncrementalEqualsClean(t, result)
	})
}

func assertIncrementalEqualsClean(t testing.TB, result IncrementalParseResult) {
	t.Helper()
	printed, err := PrintLossless(result.CST)
	if err != nil {
		t.Fatal(err)
	}
	clean, err := ParseCST(result.CST.file, string(printed))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.CST.Lines(), clean.Lines()) || !reflect.DeepEqual(result.CST.OpaqueNodes(), clean.OpaqueNodes()) {
		t.Fatalf("incremental CST differs from clean parse:\n%#v\n%#v\nopaque=%#v/%#v", result.CST.Lines(), clean.Lines(), result.CST.OpaqueNodes(), clean.OpaqueNodes())
	}
	cleanSemantic := Parse(result.CST.file, string(printed))
	if !reflect.DeepEqual(result.Semantic, cleanSemantic) {
		t.Fatalf("incremental semantic projection differs from clean parse: %#v / %#v", result.Semantic, cleanSemantic)
	}
}
