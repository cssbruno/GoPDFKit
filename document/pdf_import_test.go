// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/importpdf"
)

func importSourcePDF(t *testing.T, compress bool) []byte {
	t.Helper()

	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetCompression(compress)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 16)
	pdf.Text(72, 96, "Imported PDF source page")
	pdf.Rect(70, 110, 160, 36, "D")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("source Output() error = %v", err)
	}
	return out.Bytes()
}

func TestImportPageFromFileBytesAndReader(t *testing.T) {
	source := importSourcePDF(t, true)

	sizes, err := document.GetPageSizes(source)
	if err != nil {
		t.Fatalf("GetPageSizes() error = %v", err)
	}
	if got := sizes[1]["MediaBox"]; math.Abs(got.Wd-595.28) > 0.01 || math.Abs(got.Ht-841.89) > 0.01 {
		t.Fatalf("unexpected MediaBox size: %#v", got)
	}

	file, err := os.CreateTemp(t.TempDir(), "source-*.pdf")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := file.Write(source); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetCompression(false)
	if methodSizes := pdf.GetPageSizes(bytes.NewReader(source)); pdf.Err() || len(methodSizes) != 1 {
		t.Fatalf("method GetPageSizes() failed: %v", pdf.Error())
	}

	filePage := pdf.ImportPage(file.Name(), 1, "MediaBox")
	readerPage := pdf.ImportPageStream(bytes.NewReader(source), 1, "/MediaBox")
	allPages := pdf.ImportPagesFromSource(source, "MediaBox")
	if pdf.Err() {
		t.Fatalf("import error = %v", pdf.Error())
	}
	if filePage == 0 || readerPage == 0 || len(allPages) != 1 || allPages[0] == 0 {
		t.Fatalf("invalid imported page IDs: file=%d reader=%d all=%v", filePage, readerPage, allPages)
	}

	pdf.AddPage()
	pdf.UseImportedPage(filePage, 36, 36, 180, 0)
	pdf.UseImportedPage(readerPage, 280, 36, 0, 180)
	pdf.UseImportedPage(allPages[0], 36, 320, 120, 120)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("target Output() error = %v", err)
	}
	output := out.String()
	for _, needle := range []string{"/Subtype /Form", "/IPG1", "/IPG2", "/IPG3", "/Filter /FlateDecode"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("imported PDF output missing %q", needle)
		}
	}
}

func TestImportPageFromParsedSource(t *testing.T) {
	source := importSourcePDF(t, true)
	sourcePDF, err := importpdf.OpenBytes(source)
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}

	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetCompression(false)
	pdf.AddPage()
	pageID := pdf.ImportPageSource(sourcePDF, 1, "MediaBox")
	if pageID == 0 {
		t.Fatalf("ImportPageSource() page ID = 0, error = %v", pdf.Error())
	}
	pdf.UseImportedPage(pageID, 72, 72, 200, 0)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("/Subtype /Form")) {
		t.Fatal("output missing imported page form XObject")
	}
	if !bytes.Contains(out.Bytes(), []byte("/Filter /FlateDecode")) {
		t.Fatal("output did not preserve imported FlateDecode content")
	}
}

func TestImportPageRejectsUnsupportedSource(t *testing.T) {
	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	if ids := pdf.ImportPagesFromSource(123, "MediaBox"); ids != nil {
		t.Fatalf("expected no IDs for unsupported source, got %v", ids)
	}
	if !pdf.Err() {
		t.Fatal("expected unsupported source error")
	}
	if !errors.Is(pdf.Error(), document.ErrUnsupportedPDFImport) {
		t.Fatalf("unsupported source error = %v, want ErrUnsupportedPDFImport", pdf.Error())
	}
}

func TestDocumentGetPageSizesAppliesImportLimit(t *testing.T) {
	pdf := document.MustNew(document.WithLimits(document.Limits{MaxImportedPDFBytes: 3}))
	if sizes := pdf.GetPageSizes(importSourcePDF(t, false)); sizes != nil {
		t.Fatalf("GetPageSizes() = %#v, want nil", sizes)
	}
	if !errors.Is(pdf.Error(), document.ErrUnsupportedPDFImport) {
		t.Fatalf("GetPageSizes() error = %v, want ErrUnsupportedPDFImport", pdf.Error())
	}
}
