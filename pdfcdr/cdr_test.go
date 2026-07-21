// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/importpdf"
	"github.com/cssbruno/gopdfkit/inspect"
)

func TestSanitizeRemovesActiveDocumentStructures(t *testing.T) {
	source := activeSourcePDF(t)
	clean, err := Sanitize(source)
	if err != nil {
		t.Fatalf("Sanitize() error = %v", err)
	}
	if !bytes.HasPrefix(clean, []byte("%PDF-1.7")) {
		t.Fatalf("output does not start with a PDF header: %q", clean[:min(len(clean), 16)])
	}
	for _, forbidden := range []string{"/OpenAction", "/JavaScript", "/JS", "/AA", "/Annots", "/Metadata", "/Names"} {
		if bytes.Contains(clean, []byte(forbidden)) {
			t.Fatalf("output contains removed structure %s", forbidden)
		}
	}
	if !bytes.Contains(clean, []byte("Hello CDR")) {
		t.Fatal("output does not preserve page text")
	}
	if pages, err := inspect.PageCount(clean); err != nil || pages != 1 {
		t.Fatalf("PageCount() = %d, error = %v; want one page", pages, err)
	}
	text, err := inspect.Text(clean)
	if err != nil {
		t.Fatalf("inspect.Text() error = %v", err)
	}
	if !strings.Contains(text, "Hello CDR") {
		t.Fatalf("extracted text = %q, want Hello CDR", text)
	}
	pageSource, err := importpdf.OpenBytes(clean)
	if err != nil {
		t.Fatalf("reconstructed PDF cannot be imported: %v", err)
	}
	page, err := pageSource.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("reconstructed page cannot be opened: %v", err)
	}
	if page.ObjectCount() != 2 {
		t.Fatalf("reconstructed page object count = %d, want the resources and font objects", page.ObjectCount())
	}
}

func TestSanitizeIsIdempotent(t *testing.T) {
	first, err := Sanitize(activeSourcePDF(t))
	if err != nil {
		t.Fatalf("first Sanitize() error = %v", err)
	}
	second, err := Sanitize(first)
	if err != nil {
		t.Fatalf("second Sanitize() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Sanitize() output changed on a second pass")
	}
}

func TestSanitizePreservesTrailingContentLineBreaks(t *testing.T) {
	for name, content := range map[string][]byte{
		"single LF":         []byte("BT (one) Tj ET\n"),
		"single CRLF":       []byte("BT (one) Tj ET\r\n"),
		"multiple LF":       []byte("BT (one) Tj ET\n\n\n"),
		"multiple CRLF":     []byte("BT (one) Tj ET\r\n\r\n"),
		"mixed line breaks": []byte("BT (one) Tj ET\r\n\n\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			first, err := Sanitize(pageContentSourcePDF(t, content))
			if err != nil {
				t.Fatalf("first Sanitize() error = %v", err)
			}
			source, err := importpdf.OpenBytes(first)
			if err != nil {
				t.Fatalf("OpenBytes() error = %v", err)
			}
			page, err := source.Page(1, "MediaBox")
			if err != nil {
				t.Fatalf("Page() error = %v", err)
			}
			wrapped, err := page.ContentWithError()
			if err != nil {
				t.Fatalf("ContentWithError() error = %v", err)
			}
			got, err := standalonePageContent(wrapped)
			if err != nil {
				t.Fatalf("standalonePageContent() error = %v", err)
			}
			if !bytes.Equal(got, content) {
				t.Fatalf("reconstructed content = %q, want %q", got, content)
			}
			second, err := Sanitize(first)
			if err != nil {
				t.Fatalf("second Sanitize() error = %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("Sanitize() changed trailing line breaks on a second pass")
			}
		})
	}
}

func TestSanitizeDeduplicatesSharedResourcesAcrossPages(t *testing.T) {
	for name, test := range map[string]struct {
		source      []byte
		objectCount int
	}{
		"same source objects":      {source: sharedResourceSourcePDF(t, 2), objectCount: 8},
		"identical source bodies":  {source: equivalentResourceSourcePDF(t), objectCount: 8},
		"identical direct objects": {source: twoPageSourcePDF(t), objectCount: 7},
	} {
		t.Run(name, func(t *testing.T) {
			clean, err := Sanitize(test.source)
			if err != nil {
				t.Fatalf("Sanitize() error = %v", err)
			}
			if got := bytes.Count(clean, []byte(" 0 obj\n")); got != test.objectCount {
				t.Fatalf("reconstructed object count = %d, want %d shared objects", got, test.objectCount)
			}
			again, err := Sanitize(clean)
			if err != nil {
				t.Fatalf("second Sanitize() error = %v", err)
			}
			if !bytes.Equal(clean, again) {
				t.Fatal("shared-resource reconstruction changed on a second pass")
			}
		})
	}
}

func TestSanitizeSharedResourceBudgetsCountUniqueObjects(t *testing.T) {
	root, err := sanitizePDFObject([]byte(sharedResourceRootBody))
	if err != nil {
		t.Fatal(err)
	}
	font, err := sanitizePDFObject([]byte(sharedFontBody))
	if err != nil {
		t.Fatal(err)
	}
	// Both source references and their deterministic output IDs are one digit,
	// so rewriting the reference does not change the retained byte count.
	uniqueResourceBytes := int64(len(root) + len(font))
	source := sharedResourceSourcePDF(t, 2)
	if _, err := SanitizeContext(context.Background(), source, Options{
		MaxResourceBytes: uniqueResourceBytes,
		MaxObjects:       8,
	}); err != nil {
		t.Fatalf("SanitizeContext() with one-copy budgets error = %v", err)
	}
	for name, options := range map[string]Options{
		"resource bytes": {MaxResourceBytes: uniqueResourceBytes - 1},
		"object count":   {MaxObjects: 7},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := SanitizeContext(context.Background(), source, options)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("SanitizeContext() error = %v, want ErrLimitExceeded", err)
			}
		})
	}
}

func TestSanitizeRejectsResourceRoleChangesAcrossPages(t *testing.T) {
	_, err := Sanitize(crossPageResourceRolePDF(t))
	if err == nil || !strings.Contains(err.Error(), "both as a resource-name dictionary and as an ordinary object") {
		t.Fatalf("Sanitize() error = %v, want cross-page resource-role rejection", err)
	}
}

func TestSanitizeRewritesRenderingResourceReferences(t *testing.T) {
	clean, err := Sanitize(nonSequentialResourcePDF(t))
	if err != nil {
		t.Fatalf("Sanitize() error = %v", err)
	}
	pageSource, err := importpdf.OpenBytes(clean)
	if err != nil {
		t.Fatalf("reconstructed PDF cannot be imported: %v", err)
	}
	page, err := pageSource.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("reconstructed page resources cannot be resolved: %v", err)
	}
	if page.ObjectCount() != 2 {
		t.Fatalf("reconstructed page object count = %d, want the resources and font objects", page.ObjectCount())
	}
}

func TestSanitizeRewritesReferencesAfterStreamText(t *testing.T) {
	for name, marker := range map[string]string{
		"literal string": "(stream)",
		"PDF name":       "/stream",
	} {
		t.Run(name, func(t *testing.T) {
			clean, err := Sanitize(resourcePDFWithStreamText(t, marker))
			if err != nil {
				t.Fatalf("Sanitize() error = %v", err)
			}
			pageSource, err := importpdf.OpenBytes(clean)
			if err != nil {
				t.Fatalf("reconstructed PDF cannot be imported: %v", err)
			}
			page, err := pageSource.Page(1, "MediaBox")
			if err != nil {
				t.Fatalf("reconstructed page resources cannot be resolved: %v", err)
			}
			if page.ObjectCount() != 3 {
				t.Fatalf("reconstructed page object count = %d, want resources, font, and descriptor objects", page.ObjectCount())
			}
		})
	}
}

func TestSanitizePreservesRenderingResourceNames(t *testing.T) {
	for _, indirect := range []bool{false, true} {
		name := "direct"
		if indirect {
			name = "indirect"
		}
		t.Run(name, func(t *testing.T) {
			clean, err := Sanitize(resourceNamesMatchingRemovedKeysPDF(t, indirect))
			if err != nil {
				t.Fatalf("Sanitize() error = %v", err)
			}
			pageSource, err := importpdf.OpenBytes(clean)
			if err != nil {
				t.Fatalf("reconstructed PDF cannot be imported: %v", err)
			}
			page, err := pageSource.Page(1, "MediaBox")
			if err != nil {
				t.Fatalf("reconstructed page resources cannot be resolved: %v", err)
			}
			var resources bytes.Buffer
			if err := page.ForEachObjectBorrowed(func(_ importpdf.ObjRef, body []byte) error {
				resources.Write(body)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for _, resourceName := range []string{"/A ", "/F ", "/OpenAction "} {
				if !bytes.Contains(resources.Bytes(), []byte(resourceName)) {
					t.Fatalf("reconstructed resources = %q, want rendering resource name %s", resources.Bytes(), resourceName)
				}
			}
		})
	}
}

func TestSanitizeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SanitizeContext(ctx, activeSourcePDF(t), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SanitizeContext() error = %v, want context.Canceled", err)
	}
}

func TestSanitizeFileAtomicOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.pdf")
	output := filepath.Join(dir, "output.pdf")
	if err := os.WriteFile(input, activeSourcePDF(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SanitizeFile(input, output); err != nil {
		t.Fatalf("SanitizeFile() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importpdf.OpenBytes(data); err != nil {
		t.Fatalf("output file is not a PDF: %v", err)
	}
	if mode := fileMode(t, output); mode.Perm() != 0o600 {
		t.Fatalf("output mode = %o, want 600", mode.Perm())
	}
}

func TestSanitizeRejectsUnsupportedInput(t *testing.T) {
	_, err := Sanitize(42)
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("Sanitize() error = %v, want ErrInvalidSource", err)
	}
}

func TestSanitizeRejectsActiveResourceObjects(t *testing.T) {
	for name, body := range map[string]string{
		"PostScript XObject": "<< /Type /XObject /Subt#79pe /P#53 /Length 0 >>\nstream\n\nendstream",
		"Reference XObject":  "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /R#65f << /#46 (external.pdf) /Page 1 >> /Length 0 >>\nstream\n\nendstream",
		"external stream":    "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /#46 (external.bin) /Length 0 >>\nstream\n\nendstream",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Sanitize(activeResourcePDF(t, body))
			if err == nil {
				t.Fatal("Sanitize() error = nil, want active resource rejection")
			}
		})
	}
}

func TestSanitizeRejectsTokensAfterResourceStream(t *testing.T) {
	source := activeResourcePDF(t, "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 3 >>\nstream\nabc\nendstream\n/JavaScript true\nendstream")
	_, err := Sanitize(source)
	if err == nil || !strings.Contains(err.Error(), "unexpected bytes after PDF endstream") {
		t.Fatalf("Sanitize() error = %v, want trailing stream-token rejection", err)
	}
}

func TestSanitizeRejectsAmbiguousResourceObjectRole(t *testing.T) {
	source := resourceGraphPDF(t,
		"<< /Font 5 0 R /XObject << /Same 5 0 R >> >>",
		"<< /F1 6 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	)
	_, err := Sanitize(source)
	if err == nil || !strings.Contains(err.Error(), "both as a resource-name dictionary and as an ordinary object") {
		t.Fatalf("Sanitize() error = %v, want ambiguous resource-role rejection", err)
	}
}

func TestSanitizeDoesNotReinterpretProducerResourceNames(t *testing.T) {
	source := resourceGraphPDF(t,
		"<< /Font 5 0 R >>",
		"<< /XObject 6 0 R >>",
		"<< /Type /XObject /Subtype /PS /Length 0 >>\nstream\n\nendstream",
	)
	if _, err := Sanitize(source); err == nil || !strings.Contains(err.Error(), "PostScript") {
		t.Fatalf("Sanitize() error = %v, want nested PostScript resource rejection", err)
	}
}

func TestSanitizeAggregateLimits(t *testing.T) {
	source := activeSourcePDF(t)
	for name, options := range map[string]Options{
		"decoded bytes":  {MaxDecodedBytes: 1},
		"resource bytes": {MaxResourceBytes: 1},
		"output bytes":   {MaxOutputBytes: 100},
		"object count":   {MaxObjects: 2},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := SanitizeContext(context.Background(), source, options)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("SanitizeContext() error = %v, want ErrLimitExceeded", err)
			}
		})
	}
}

func TestSanitizeLimitsAreAggregateAcrossPages(t *testing.T) {
	t.Run("decoded bytes", func(t *testing.T) {
		_, err := SanitizeContext(context.Background(), twoPageSourcePDF(t), Options{MaxDecodedBytes: 5})
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("SanitizeContext() error = %v, want aggregate ErrLimitExceeded", err)
		}
	})
	t.Run("distinct resource bytes", func(t *testing.T) {
		first, err := sanitizePDFObject([]byte("<< /ProcSet [/PDF] >>"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := sanitizePDFObject([]byte("<< /ProcSet [/Text] >>"))
		if err != nil {
			t.Fatal(err)
		}
		limit := max(len(first), len(second))
		_, err = SanitizeContext(context.Background(), twoPageDistinctResourcesPDF(t), Options{MaxResourceBytes: int64(limit)})
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("SanitizeContext() error = %v, want aggregate ErrLimitExceeded", err)
		}
	})
}

func TestSanitizeAppliesLimitsToExistingSource(t *testing.T) {
	data := activeSourcePDF(t)
	source, err := importpdf.OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
	_, err = SanitizeContext(context.Background(), source, Options{MaxSourceBytes: 1})
	if !errors.Is(err, importpdf.ErrSourceTooLarge) {
		t.Fatalf("SanitizeContext() source-size error = %v, want ErrSourceTooLarge", err)
	}
	_, err = SanitizeContext(context.Background(), source, Options{MaxReferencedObjects: 1})
	if err == nil || !strings.Contains(err.Error(), "referenced object count exceeds") {
		t.Fatalf("SanitizeContext() referenced-object error = %v, want referenced-object limit", err)
	}
}

func activeSourcePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 9)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R /OpenAction 7 0 R /Names << /JavaScript << /Names [(run) 7 0 R] >> >> >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources 5 0 R /Contents 4 0 R /Annots [9 0 R] >>"))
	builder.set(ids[3], pdfStreamBody([]byte("BT /F1 12 Tf 72 72 Td (Hello CDR) Tj ET")))
	builder.set(ids[4], []byte("<< /Font << /F1 6 0 R >> /AA 7 0 R /Metadata 8 0 R >>"))
	builder.set(ids[5], []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	builder.set(ids[6], []byte("<< /Type /Action /S /JavaScript /JS (app.alert) >>"))
	builder.set(ids[7], []byte("<< /Type /Metadata /Subtype /XML /Length 11 >>\nstream\n<xml></xml>\nendstream"))
	builder.set(ids[8], []byte("<< /Type /Annot /Subtype /Link /A 7 0 R /Rect [0 0 1 1] >>"))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func activeResourcePDF(t testing.TB, resourceBody string) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 5)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /XObject << /R1 5 0 R >> >> /Contents 4 0 R >>"))
	builder.set(ids[3], pdfStreamBody([]byte("/R1 Do")))
	builder.set(ids[4], []byte(resourceBody))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func resourceGraphPDF(t testing.TB, resources string, objectBodies ...string) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 4+len(objectBodies))
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte(fmt.Sprintf(
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources %s /Contents 4 0 R >>",
		resources,
	)))
	builder.set(ids[3], pdfStreamBody(nil))
	for i, body := range objectBodies {
		builder.set(ids[4+i], []byte(body))
	}
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func twoPageSourcePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 6)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 5 0 R >>"))
	builder.set(ids[3], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 6 0 R >>"))
	builder.set(ids[4], pdfStreamBody([]byte("q Q")))
	builder.set(ids[5], pdfStreamBody([]byte("q Q")))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func pageContentSourcePDF(t testing.TB, content []byte) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 4)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 4 0 R >>"))
	builder.set(ids[3], pdfStreamBody(content))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func twoPageDistinctResourcesPDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 6)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ProcSet [/PDF] >> /Contents 5 0 R >>"))
	builder.set(ids[3], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /ProcSet [/Text] >> /Contents 6 0 R >>"))
	builder.set(ids[4], pdfStreamBody([]byte("q Q")))
	builder.set(ids[5], pdfStreamBody([]byte("q Q")))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

const (
	sharedResourceRootBody = "<< /Font << /F1 8 0 R >> >>"
	sharedFontBody         = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
)

func sharedResourceSourcePDF(t testing.TB, pages int) []byte {
	t.Helper()
	if pages != 1 && pages != 2 {
		t.Fatalf("sharedResourceSourcePDF pages = %d, want 1 or 2", pages)
	}
	builder := &pdfBuilder{}
	ids := make([]int, 8)
	for i := range ids {
		ids[i] = builder.reserve()
		builder.set(ids[i], []byte("null"))
	}
	kids := "3 0 R"
	if pages == 2 {
		kids += " 4 0 R"
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, pages)))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources 7 0 R /Contents 5 0 R >>"))
	if pages == 2 {
		builder.set(ids[3], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources 7 0 R /Contents 6 0 R >>"))
	}
	builder.set(ids[4], pdfStreamBody([]byte("BT /F1 12 Tf (one) Tj ET")))
	builder.set(ids[5], pdfStreamBody([]byte("BT /F1 12 Tf (two) Tj ET")))
	builder.set(ids[6], []byte(sharedResourceRootBody))
	builder.set(ids[7], []byte(sharedFontBody))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func equivalentResourceSourcePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 8)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 7 0 R >> >> /Contents 5 0 R >>"))
	builder.set(ids[3], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 8 0 R >> >> /Contents 6 0 R >>"))
	builder.set(ids[4], pdfStreamBody([]byte("BT /F1 12 Tf (one) Tj ET")))
	builder.set(ids[5], pdfStreamBody([]byte("BT /F1 12 Tf (two) Tj ET")))
	builder.set(ids[6], []byte(sharedFontBody))
	builder.set(ids[7], []byte(sharedFontBody))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func crossPageResourceRolePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 8)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font 7 0 R >> /Contents 5 0 R >>"))
	builder.set(ids[3], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources 7 0 R /Contents 6 0 R >>"))
	builder.set(ids[4], pdfStreamBody(nil))
	builder.set(ids[5], pdfStreamBody(nil))
	builder.set(ids[6], []byte("<< /F1 8 0 R >>"))
	builder.set(ids[7], []byte(sharedFontBody))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func nonSequentialResourcePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 9)
	for i := range ids {
		ids[i] = builder.reserve()
		builder.set(ids[i], []byte("null"))
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 9 0 R >> >> /Contents 4 0 R >>"))
	builder.set(ids[3], pdfStreamBody([]byte("BT /F1 12 Tf 10 10 Td (resource reference) Tj ET")))
	builder.set(ids[8], []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func resourcePDFWithStreamText(t testing.TB, marker string) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 10)
	for i := range ids {
		ids[i] = builder.reserve()
		builder.set(ids[i], []byte("null"))
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 9 0 R >> >> /Contents 4 0 R >>"))
	builder.set(ids[3], pdfStreamBody([]byte("BT /F1 12 Tf 10 10 Td (stream text) Tj ET")))
	builder.set(ids[8], []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Marker "+marker+" /FontDescriptor 10 0 R >>"))
	builder.set(ids[9], []byte("<< /Type /FontDescriptor /FontName /Helvetica >>"))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func resourceNamesMatchingRemovedKeysPDF(t testing.TB, indirect bool) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 9)
	for i := range ids {
		ids[i] = builder.reserve()
		builder.set(ids[i], []byte("null"))
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	resources := "<< /Font << /A 9 0 R /F 9 0 R /OpenAction 9 0 R >> >>"
	if indirect {
		resources = "<< /Font 8 0 R >>"
		builder.set(ids[7], []byte("<< /A 9 0 R /F 9 0 R /OpenAction 9 0 R >>"))
	}
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources "+resources+" /Contents 4 0 R >>"))
	builder.set(ids[3], pdfStreamBody([]byte("BT /A 12 Tf 10 10 Td (A) Tj /F 12 Tf (F) Tj /OpenAction 12 Tf (O) Tj ET")))
	builder.set(ids[8], []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
