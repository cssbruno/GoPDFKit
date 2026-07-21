// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPDFNameLookupCanonicalizesHexEscapes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		input string
		want  bool
	}{
		{name: "plain", input: "/Encrypt", want: true},
		{name: "middle escape", input: "/En#63rypt", want: true},
		{name: "leading escape", input: "/#45ncrypt", want: true},
		{name: "lowercase hex", input: "/En#63rypt", want: true},
		{name: "wrong decoded name", input: "/En#6crypt", want: false},
		{name: "prefix only", input: "/EncryptExtra", want: false},
		{name: "delimiter-adjacent", input: "/Encrypt", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, got := matchPDFNameToken([]byte(test.input), "/Encrypt")
			if got != test.want {
				t.Fatalf("matchPDFNameToken(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}

	byteRange, err := parseByteRangeValue([]byte("[0 1 2 0]"))
	if err != nil {
		t.Fatalf("parseByteRangeValue() error = %v", err)
	}
	if len(byteRange) != 4 || byteRange[1] != 1 || byteRange[2] != 2 {
		t.Fatalf("extractByteRange() = %v, want [0 1 2 0]", byteRange)
	}
}

func TestAnalyzePDFRecognizesCompactAndEscapedPageNames(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		catalog string
		pages   string
		page    string
	}{
		{
			name:    "compact",
			catalog: "<</Type/Catalog/Pages 2 0 R>>",
			pages:   "<</Type/Pages/Kids[3 0 R]/Count 1>>",
			page:    "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>",
		},
		{
			name:    "escaped",
			catalog: "<< /T#79pe /Cat#61log /P#61ges 2 0 R >>",
			pages:   "<< /T#79pe /P#61ges /K#69ds [3 0 R] /Count 1 >>",
			page:    "<< /T#79pe /P#61ge /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			analyzed, err := analyzePDF(minimalClassicPDF(test.catalog, test.pages, test.page))
			if err != nil {
				t.Fatalf("analyzePDF() error = %v", err)
			}
			if analyzed.Page != (pdfRef{Object: 3}) {
				t.Fatalf("first page = %+v, want 3 0 R", analyzed.Page)
			}
		})
	}
}

func TestAnalyzePDFUsesLexicalObjectBoundaries(t *testing.T) {
	t.Parallel()

	input := minimalClassicPDF(
		"% comment with << and endobj\n<< /Type /Catalog /Pages 2 0 R /Lang (endobj) >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 /Note (<< endobj) >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	)
	analyzed, err := analyzePDF(input)
	if err != nil {
		t.Fatalf("analyzePDF() error = %v", err)
	}
	if analyzed.Root != (pdfRef{Object: 1}) || analyzed.Page != (pdfRef{Object: 3}) {
		t.Fatalf("analyzed refs = root %+v page %+v", analyzed.Root, analyzed.Page)
	}
}

func TestIncrementalXrefHonorsFreeAndReusedGenerations(t *testing.T) {
	t.Parallel()

	base := minimalClassicPDF(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	)
	previous, err := findStartXref(base)
	if err != nil {
		t.Fatalf("findStartXref() error = %v", err)
	}

	freed := appendClassicRevision(base, previous, "", "1 1\n0000000000 00001 f \n", "1 0 R")
	if _, err := analyzePDF(freed); err == nil {
		t.Fatal("analyzePDF() resurrected a root object freed by the newest revision")
	}

	reusedBody := "1 1 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	reusedOffset := len(base) + 1
	reusedXref := fmt.Sprintf("1 1\n%010d 00001 n \n", reusedOffset)
	reused := appendClassicRevision(base, previous, reusedBody, reusedXref, "1 1 R")
	analyzed, err := analyzePDF(reused)
	if err != nil {
		t.Fatalf("analyzePDF(reused generation) error = %v", err)
	}
	if analyzed.Root != (pdfRef{Object: 1, Generation: 1}) || analyzed.Page != (pdfRef{Object: 3}) {
		t.Fatalf("analyzed refs = root %+v page %+v", analyzed.Root, analyzed.Page)
	}
}

func TestAnalyzePDFRejectsUnderreportedTrailerSize(t *testing.T) {
	t.Parallel()

	input := minimalClassicPDF(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	)
	input = bytes.Replace(input, []byte("/Size 4"), []byte("/Size 2"), 1)
	if _, err := analyzePDF(input); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("analyzePDF() error = %v, want ErrUnsupportedPDF for underreported /Size", err)
	}
}

func TestVerificationRejectsMissingInvalidAndUnderreportedTrailerSize(t *testing.T) {
	t.Parallel()

	valid := minimalClassicPDF(
		"<< /Type /Catalog /Pages 2 0 R /AcroForm 4 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Fields [5 0 R] >>",
		"<< /FT /Sig /V 6 0 R >>",
		"<< /ByteRange [0 1 2 0] /Contents <00> >>",
	)
	if signatures, err := scanSignatureDictionaries(valid); err != nil || len(signatures) != 1 {
		t.Fatalf("valid setup: signatures=%d error=%v", len(signatures), err)
	}
	tests := []struct {
		name        string
		replacement string
		wantError   string
		unsupported bool
	}{
		{name: "missing", replacement: "/Siz_ 7", wantError: "trailer /Size not found"},
		{name: "name", replacement: "/Size /Seven", wantError: "invalid trailer /Size"},
		{name: "fractional", replacement: "/Size 7.5", wantError: "invalid trailer /Size"},
		{name: "trailing token data", replacement: "/Size 7x", wantError: "invalid trailer /Size"},
		{name: "underreported", replacement: "/Size 1", wantError: "does not cover xref object", unsupported: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := bytes.Replace(valid, []byte("/Size 7"), []byte(test.replacement), 1)
			if bytes.Equal(input, valid) {
				t.Fatal("test setup did not replace trailer /Size")
			}
			_, err := scanSignatureDictionaries(input)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("scanSignatureDictionaries() error = %v, want %q", err, test.wantError)
			}
			if test.unsupported && !errors.Is(err, ErrUnsupportedPDF) {
				t.Fatalf("scanSignatureDictionaries() error = %v, want ErrUnsupportedPDF", err)
			}
		})
	}
}

func TestVerificationRejectsNonIntegerPrev(t *testing.T) {
	t.Parallel()

	base := minimalClassicPDF(
		"<< /Type /Catalog /Pages 2 0 R /AcroForm 4 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Fields [5 0 R] >>",
		"<< /FT /Sig /V 6 0 R >>",
		"<< /ByteRange [0 1 2 0] /Contents <00> >>",
	)
	previous, err := findStartXref(base)
	if err != nil {
		t.Fatalf("findStartXref() error = %v", err)
	}
	for _, suffix := range []string{".5", "x"} {
		t.Run(suffix, func(t *testing.T) {
			var revision bytes.Buffer
			revision.WriteByte('\n')
			currentXref := len(base) + revision.Len()
			revision.WriteString("xref\n0 1\n0000000000 65535 f \n")
			fmt.Fprintf(&revision, "trailer\n<< /Size 7 /Root 1 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n", previous, suffix, currentXref)
			input := append(append([]byte(nil), base...), revision.Bytes()...)
			_, err := scanSignatureDictionaries(input)
			if err == nil || !strings.Contains(err.Error(), "invalid trailer /Prev") {
				t.Fatalf("scanSignatureDictionaries() error = %v, want invalid trailer /Prev", err)
			}
		})
	}
}

func TestVerificationRejectsTrailingXrefLineSyntax(t *testing.T) {
	t.Parallel()

	valid := minimalClassicPDF(
		"<< /Type /Catalog /Pages 2 0 R /AcroForm 4 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Fields [5 0 R] >>",
		"<< /FT /Sig /V 6 0 R >>",
		"<< /ByteRange [0 1 2 0] /Contents <00> >>",
	)
	tests := []struct {
		name        string
		old         string
		replacement string
		wantError   string
	}{
		{name: "fractional subsection count", old: "xref\n0 7\n", replacement: "xref\n0 7.5\n", wantError: "invalid xref subsection"},
		{name: "trailing subsection token", old: "xref\n0 7\n", replacement: "xref\n0 7 junk\n", wantError: "invalid xref subsection"},
		{name: "trailing entry token", old: "0000000000 65535 f \n", replacement: "0000000000 65535 f junk\n", wantError: "invalid xref entry"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := bytes.Replace(valid, []byte(test.old), []byte(test.replacement), 1)
			if bytes.Equal(input, valid) {
				t.Fatal("test setup did not mutate xref")
			}
			_, err := scanSignatureDictionaries(input)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("scanSignatureDictionaries() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestTrailerSizeReservesSignatureObjectNumbers(t *testing.T) {
	t.Parallel()

	trailer := []byte(fmt.Sprintf("<< /Size %d /Root 1 0 R >>", DefaultMaxXrefEntries-2))
	if _, _, err := parseTrailerContext(context.Background(), trailer, DefaultMaxXrefEntries, 3); err == nil {
		t.Fatal("parseTrailerContext() accepted a /Size without room for three signature objects")
	}
}

func minimalClassicPDF(objects ...string) []byte {
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, body := range objects {
		offsets[i+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xrefOffset := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(offsets))
	output.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefOffset)
	return output.Bytes()
}

func appendClassicRevision(base []byte, previous int, objectBody, xrefBody, root string) []byte {
	var revision bytes.Buffer
	revision.WriteByte('\n')
	revision.WriteString(objectBody)
	xrefOffset := len(base) + revision.Len()
	revision.WriteString("xref\n")
	revision.WriteString(xrefBody)
	fmt.Fprintf(&revision, "trailer\n<< /Size 4 /Root %s /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", root, previous, xrefOffset)
	return append(append([]byte(nil), base...), revision.Bytes()...)
}

func TestUnsupportedPDFErrorsAreClassifiable(t *testing.T) {
	t.Parallel()

	if _, err := parseXrefTable([]byte("not an xref"), 0); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("parseXrefTable() error = %v, want ErrUnsupportedPDF", err)
	}
	if _, err := addAnnotation([]byte("<< /Annots 3 0 R >>"), "4 0 R"); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("addAnnotation() error = %v, want ErrUnsupportedPDF", err)
	}
	annotation, err := addAnnotation([]byte("<< /#41nnots [3 0 R] >>"), "4 0 R")
	if err != nil {
		t.Fatalf("addAnnotation() encoded name error = %v", err)
	}
	if string(annotation) != "<< /#41nnots [3 0 R 4 0 R] >>" {
		t.Fatalf("addAnnotation() = %q", annotation)
	}
	if _, err := addDictEntries([]byte("<< /AcroForm 3 0 R >>"), pdfDictEntry{key: "/AcroForm", value: "4 0 R"}); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("addDictEntries() error = %v, want ErrUnsupportedPDF", err)
	}
}

func TestDictionaryLookupUsesEncodedNameTokenLength(t *testing.T) {
	t.Parallel()

	dict := []byte("<< /S#69ze 5 /R#6fot 12 3 R >>")
	value, ok, err := trailerEntryValueContext(context.Background(), dict, "/Size")
	if err != nil {
		t.Fatalf("trailerEntryValueContext() error = %v", err)
	}
	if !ok || string(value) != "5" {
		t.Fatalf("trailer /Size = %q, %v; want 5, true", value, ok)
	}
	ref, ok, err := findReferenceContext(context.Background(), dict, "/Root")
	if err != nil {
		t.Fatalf("findReferenceContext() error = %v", err)
	}
	if !ok || ref != (pdfRef{Object: 12, Generation: 3}) {
		t.Fatalf("trailer /Root = %+v, %v; want 12 3 R, true", ref, ok)
	}
}

func TestDictionaryLookupTraversesPageTreeEntries(t *testing.T) {
	t.Parallel()

	root := []byte("<< /Type /Catalog /Pages 2 0 R >>")
	pages, ok, err := findReferenceContext(context.Background(), root, "/Pages")
	if err != nil || !ok || pages != (pdfRef{Object: 2}) {
		t.Fatalf("findReferenceContext(/Pages) = %+v, %v, %v", pages, ok, err)
	}
	page := []byte("<< /Type /Page /Parent 2 0 R >>")
	entry, ok, err := findDictionaryEntryContext(context.Background(), page, "/Type")
	if err != nil || !ok {
		t.Fatalf("findDictionaryEntryContext(/Type) = %+v, %v, %v", entry, ok, err)
	}
	if value := page[entry.ValueStart:entry.ValueEnd]; !pdfNameValueEquals(value, "/Page") {
		t.Fatalf("page type %q was not recognized", value)
	}
}

func TestXrefResolverCachesValidatedIndirectValues(t *testing.T) {
	t.Parallel()

	input := minimalClassicPDF("<< /Payload (" + strings.Repeat("x", 1<<20) + ") >>")
	xrefOffset, err := findStartXref(input)
	if err != nil {
		t.Fatalf("findStartXref() error = %v", err)
	}
	resolver, err := newPDFXrefResolverContext(context.Background(), input, xrefOffset, DefaultMaxXrefChainLength, DefaultMaxXrefEntries)
	if err != nil {
		t.Fatalf("newPDFXrefResolverContext() error = %v", err)
	}
	ref := pdfRef{Object: 1}
	first, firstStart, err := resolver.indirectValueContext(context.Background(), ref)
	if err != nil {
		t.Fatalf("indirectValueContext(first) error = %v", err)
	}
	if len(resolver.indirectValues) != 1 {
		t.Fatalf("cached indirect values = %d, want 1", len(resolver.indirectValues))
	}
	second, secondStart, err := resolver.indirectValueContext(context.Background(), ref)
	if err != nil {
		t.Fatalf("indirectValueContext(second) error = %v", err)
	}
	if firstStart != secondStart || len(first) != len(second) || &first[0] != &second[0] {
		t.Fatal("second indirect-value lookup did not return the cached slice and position")
	}
}
