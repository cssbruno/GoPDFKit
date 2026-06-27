// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestBookmarkDestinationsUseActualPageObjectNumbers(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetAttachments([]Attachment{{Content: []byte("payload"), Filename: "payload.txt"}})
	pdf.AddPage()
	pdf.Bookmark("start", 0, -1)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatal(err)
	}
	pageObj := pdf.pageObjectNumber(1)
	if pageObj == 0 || pageObj == 3 {
		t.Fatalf("page object = %d, want shifted object number", pageObj)
	}
	if !strings.Contains(out.String(), sprintf("/Dest [%d 0 R", pageObj)) {
		t.Fatalf("bookmark destination does not reference actual page object %d", pageObj)
	}
}

func TestBookmarkValidationRejectsInvalidLevelsAndMissingPage(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.Bookmark("missing page", 0, -1)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "active page") {
		t.Fatalf("Bookmark before AddPage error = %v, want active page error", pdf.Error())
	}

	pdf = New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.Bookmark("bad first", 1, -1)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "first bookmark level") {
		t.Fatalf("first bookmark level error = %v", pdf.Error())
	}

	pdf = New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.Bookmark("root", 0, -1)
	pdf.Bookmark("skip", 2, -1)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "cannot jump") {
		t.Fatalf("skipped bookmark level error = %v", pdf.Error())
	}
}

func TestSplitTextPreservesCJKCharacters(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)

	const text = "中文かな한글"
	lines := pdf.SplitText(text, 4)
	if got := strings.Join(lines, ""); got != text {
		t.Fatalf("SplitText joined lines = %q, want %q; lines=%q", got, text, lines)
	}
}

func TestAddPageFormatRejectsInvalidOrientationAndSize(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPageFormat("banana", Size{Wd: 100, Ht: 100})
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "incorrect orientation") {
		t.Fatalf("invalid orientation error = %v", pdf.Error())
	}
	if pdf.PageNo() != 0 {
		t.Fatalf("invalid AddPageFormat added page %d", pdf.PageNo())
	}

	pdf = New("P", "mm", "A4", "")
	pdf.AddPageFormat("P", Size{Wd: math.NaN(), Ht: 100})
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "invalid page size") {
		t.Fatalf("invalid page size error = %v", pdf.Error())
	}
	if pdf.PageNo() != 0 {
		t.Fatalf("invalid AddPageFormat added page %d", pdf.PageNo())
	}
}

func TestGridRestoresAutoPageBreak(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetAutoPageBreak(true, 17)

	grid := NewGrid(10, 10, 40, 40)
	grid.Grid(pdf)

	auto, margin := pdf.GetAutoPageBreak()
	if !auto || margin != 17 {
		t.Fatalf("auto page break = %v, %.2f; want true, 17", auto, margin)
	}
}

func TestClipPolygonRejectsInvalidPointCountWithoutEnteringClipState(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()

	pdf.ClipPolygon([]Point{{X: 1, Y: 1}, {X: 2, Y: 2}}, false)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "at least 3 points") {
		t.Fatalf("ClipPolygon error = %v", pdf.Error())
	}
	if pdf.clipNest != 0 {
		t.Fatalf("clipNest = %d, want 0", pdf.clipNest)
	}
}

func TestGetStringWidthWithoutFontSetsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	if width := pdf.GetStringWidth("abc"); width != 0 {
		t.Fatalf("GetStringWidth without font = %.2f, want 0", width)
	}
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "font must be selected") {
		t.Fatalf("GetStringWidth error = %v", pdf.Error())
	}
}

func TestImageAndAttachmentBoundaryValidation(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	if info := pdf.RegisterImageOptionsReader("", ImageOptions{ImageType: "png"}, bytes.NewReader(nil)); info != nil {
		t.Fatal("RegisterImageOptionsReader with blank name returned image info")
	}
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "image name") {
		t.Fatalf("blank image name error = %v", pdf.Error())
	}

	pdf = New("P", "mm", "A4", "")
	pdf.AddAttachmentAnnotation(nil, 1, 1, 1, 1)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "requires an attachment") {
		t.Fatalf("nil attachment annotation error = %v", pdf.Error())
	}
}

func TestSetAttachmentsCopiesContent(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	content := []byte("original")
	attachments := []Attachment{{Content: content, Filename: "a.txt"}}
	pdf.SetAttachments(attachments)
	content[0] = 'X'
	attachments[0].Filename = "changed.txt"

	if got := string(pdf.attachments[0].Content); got != "original" {
		t.Fatalf("attachment content = %q, want original", got)
	}
	if got := pdf.attachments[0].Filename; got != "a.txt" {
		t.Fatalf("attachment filename = %q, want a.txt", got)
	}
}

func TestSetAttachmentsImmutableSharesContent(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	content := []byte("original")
	pdf.SetAttachmentsImmutable([]Attachment{{Content: content, Filename: "a.txt", MIMEType: " text/plain ", AFRelationship: " Source "}})
	content[0] = 'X'

	if got := string(pdf.attachments[0].Content); got != "Xriginal" {
		t.Fatalf("attachment content = %q, want shared content", got)
	}
	if got := pdf.attachments[0].mimeType; got != "text/plain" {
		t.Fatalf("normalized MIME type = %q, want text/plain", got)
	}
	if got := pdf.attachments[0].afRelationship; got != "Source" {
		t.Fatalf("normalized AFRelationship = %q, want Source", got)
	}
}

func TestAttachmentOutputDedupesEquivalentFiles(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	a1 := Attachment{Content: []byte("same attachment"), Filename: "a.txt", Description: "same"}
	a2 := Attachment{Content: []byte("same attachment"), Filename: "a.txt", Description: "same"}
	pdf.SetAttachments([]Attachment{a1})
	pdf.AddPage()
	pdf.AddAttachmentAnnotation(&a2, 10, 10, 20, 10)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if got := bytes.Count(out.Bytes(), []byte("/Type /EmbeddedFile")); got != 1 {
		t.Fatalf("embedded file stream count = %d, want 1", got)
	}
	if got := bytes.Count(out.Bytes(), []byte("/Type /Filespec")); got != 1 {
		t.Fatalf("filespec count = %d, want 1", got)
	}
}

func TestCatalogOmitsNamesWhenUnused(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(10, 10, "plain")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("/Names <<")) {
		t.Fatal("catalog contains /Names without JavaScript or attachments")
	}
	if bytes.Contains(out.Bytes(), []byte("/EmbeddedFiles")) {
		t.Fatal("catalog contains /EmbeddedFiles without attachments")
	}
}

func TestGetImageInfoReturnsClone(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	info := pdf.newImageInfo()
	info.w = 72
	info.h = 72
	pdf.images["img"] = info

	got := pdf.GetImageInfo("img")
	got.SetDpi(144)
	if pdf.images["img"].dpi != 72 {
		t.Fatalf("registered image dpi = %.2f, want 72", pdf.images["img"].dpi)
	}
}

func TestSetDpiRejectsInvalidValues(t *testing.T) {
	info := (&Document{k: 1}).newImageInfo()
	info.SetDpi(0)
	if info.dpi != 72 {
		t.Fatalf("SetDpi(0) changed dpi to %.2f", info.dpi)
	}
	info.SetDpi(math.Inf(1))
	if info.dpi != 72 {
		t.Fatalf("SetDpi(+Inf) changed dpi to %.2f", info.dpi)
	}
	info.SetDpi(144)
	if info.dpi != 144 {
		t.Fatalf("SetDpi(144) = %.2f, want 144", info.dpi)
	}
}

func TestHTMLFragmentLinksAreRejected(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()

	html.Write(5, `<a href="#section">section</a>`)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "fragment links") {
		t.Fatalf("HTML fragment link error = %v", pdf.Error())
	}
}

func TestSetPageBoxRejectsInvalidExtent(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetPageBox("crop", 1, 1, 0, 10)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "invalid page box") {
		t.Fatalf("SetPageBox error = %v", pdf.Error())
	}
}

func TestTemplateGeometryValidation(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	if tpl := pdf.CreateTemplateCustom(Point{}, Size{Wd: -1, Ht: 10}, nil); tpl != nil {
		t.Fatal("CreateTemplateCustom returned template for invalid size")
	}
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "invalid template geometry") {
		t.Fatalf("CreateTemplateCustom error = %v", pdf.Error())
	}

	if tpl := CreateTpl(Point{}, Size{Wd: 10, Ht: math.NaN()}, "P", "mm", "", nil); tpl != nil {
		t.Fatal("CreateTpl returned template for invalid size")
	}
}

func TestUseTemplateScaledRejectsInvalidPlacement(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	tpl := CreateTpl(Point{}, Size{Wd: 10, Ht: 10}, "P", "mm", "", nil)
	pdf.AddPage()

	pdf.UseTemplateScaled(tpl, Point{}, Size{Wd: 0, Ht: 10})
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "invalid template geometry") {
		t.Fatalf("UseTemplateScaled error = %v", pdf.Error())
	}
}

func TestImportedPageAndTemplateRequireActivePage(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.UseImportedPage(1, 1, 1, 1, 1)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "without first adding a page") {
		t.Fatalf("UseImportedPage error = %v", pdf.Error())
	}

	pdf = New("P", "mm", "A4", "")
	pdf.UseImportedTemplate("/Tpl1", 1, 1, 0, 0)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "without first adding a page") {
		t.Fatalf("UseImportedTemplate error = %v", pdf.Error())
	}
}

func TestUseImportedTemplateRejectsInvalidTransform(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.UseImportedTemplate("/Tpl1", 0, 1, 0, 0)
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "invalid imported template placement") {
		t.Fatalf("UseImportedTemplate error = %v", pdf.Error())
	}
}

func TestImportObjectsCopiesInputMaps(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	objs := map[string][]byte{"a": []byte("object")}
	pos := map[string]map[int]string{"a": {1: "old"}}
	pdf.ImportObjects(objs)
	pdf.ImportObjPos(pos)

	objs["a"][0] = 'X'
	pos["a"][1] = "new"

	if got := string(pdf.importedObjs["a"]); got != "object" {
		t.Fatalf("imported object = %q, want object", got)
	}
	if got := pdf.importedObjPos["a"][1]; got != "old" {
		t.Fatalf("imported object position = %q, want old", got)
	}
}
