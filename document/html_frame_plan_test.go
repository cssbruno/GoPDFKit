// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/inspect"
)

func newHTMLFrameTestDocument(t *testing.T, height float64) *Document {
	t.Helper()
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: height}), WithNoCompression(), WithDeterministicOutput())
	pdf.SetMargins(16, 18, 20)
	pdf.SetAutoPageBreak(true, 22)
	pdf.AddPage()
	pdf.SetFont("Times", "BIU", 11)
	if err := pdf.Error(); err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestHTMLStartFrameCapturesLiveCompatibilityState(t *testing.T) {
	pdf := newHTMLFrameTestDocument(t, 140)
	pdf.SetXY(27, 43)
	frame, err := pdf.captureHTMLStartFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame.page != 1 || frame.pageCount != 1 || frame.pageSize != (Size{Wd: 180, Ht: 140}) ||
		frame.orientation != "P" || frame.rotation != 0 || frame.left != 16 || frame.top != 18 ||
		frame.right != 20 || frame.bottom != 22 || frame.x != 27 || frame.y != 43 ||
		frame.fontFamily != "times" || frame.fontStyle != "BI" || frame.fontSizePoints != 11 ||
		!frame.underline || frame.strikeout || !frame.autoPageBreak || frame.customPageBreak {
		t.Fatalf("captured frame = %#v", frame)
	}
	if frame.body.X.Points() != 16 || frame.body.Y.Points() != 18 || frame.body.Width.Points() != 144 || frame.body.Height.Points() != 100 {
		t.Fatalf("captured body = %#v", frame.body)
	}
}

func TestHTMLFragmentPlanningIsAtomicAndReturnsDeterministicExit(t *testing.T) {
	pdf := newHTMLFrameTestDocument(t, 140)
	pdf.Text(16, 28, "manual-before")
	pdf.SetXY(16, 44)
	before := append([]byte(nil), pdf.pages[1].Bytes()...)
	compiled, err := CompileHTML(`<p>planned middle</p><p>second planned line</p>`)
	if err != nil {
		t.Fatal(err)
	}
	html := pdf.HTMLNew()
	first, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	second, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	if first.plan.Hash() == "" || first.plan.Hash() != second.plan.Hash() || first.final != second.final || first.final.page != 1 || first.final.y <= 44 {
		t.Fatalf("fragment plans = hash %q/%q final %#v/%#v", first.plan.Hash(), second.plan.Hash(), first.final, second.final)
	}
	if pdf.PageCount() != 1 || pdf.PageNo() != 1 || pdf.GetX() != 16 || pdf.GetY() != 44 || !bytes.Equal(pdf.pages[1].Bytes(), before) {
		t.Fatal("planning mutated the live document")
	}
}

func TestHTMLUnifiedFragmentBetweenManualDrawingBeforeAndAfter(t *testing.T) {
	pdf := newHTMLFrameTestDocument(t, 140)
	pdf.Text(16, 28, "manual-before")
	pdf.SetXY(16, 44)
	html := pdf.HTMLNew()
	if err := html.WriteContext(context.Background(), 12, `<p>planned-middle</p><p>planned-tail</p>`); err != nil {
		t.Fatal(err)
	}
	if pdf.PageNo() != 1 || pdf.GetX() != 16 || pdf.GetY() <= 44 {
		t.Fatalf("HTML exit = page %d cursor %.2f,%.2f", pdf.PageNo(), pdf.GetX(), pdf.GetY())
	}
	if pdf.fontFamily != "times" || pdf.fontStyle != "BI" || !pdf.underline || pdf.fontSizePt != 11 {
		t.Fatalf("font context after HTML = %q %q underline=%v size=%g", pdf.fontFamily, pdf.fontStyle, pdf.underline, pdf.fontSizePt)
	}
	pdf.Text(16, pdf.GetY()+12, "manual-after")
	var out bytes.Buffer
	if err := pdf.OutputWithOptions(&out, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	text, err := inspect.PageTextContext(context.Background(), out.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	text = strings.ReplaceAll(text, "\x00", "")
	for _, want := range []string{"manual-before", "planned-middle", "planned-tail", "manual-after"} {
		if !strings.Contains(text, want) {
			t.Fatalf("page text lacks %q: %q", want, text)
		}
	}
}

func TestHTMLUnifiedFragmentPreflightFailureDoesNotChangePageOrCursor(t *testing.T) {
	pdf := newHTMLFrameTestDocument(t, 140)
	pdf.SetXY(16, 44)
	before := append([]byte(nil), pdf.pages[1].Bytes()...)
	compiled, err := CompileHTML(`<p>must not paint</p>`)
	if err != nil {
		t.Fatal(err)
	}
	html := pdf.HTMLNew()
	html.WriteCompiled(0, compiled)
	if pdf.Error() == nil {
		t.Fatal("zero line-height failure = nil")
	}
	if pdf.PageCount() != 1 || pdf.PageNo() != 1 || pdf.GetX() != 16 || pdf.GetY() != 44 || !bytes.Equal(pdf.pages[1].Bytes(), before) {
		t.Fatal("failed whole-fragment write changed page content or cursor")
	}
}

func TestHTMLUnifiedFragmentContinuationReturnsFinalPageAndCursor(t *testing.T) {
	pdf := newHTMLFrameTestDocument(t, 82)
	pdf.SetXY(16, 35)
	html := pdf.HTMLNew()
	source := strings.Repeat(`<p>one planned continuation line</p>`, 8)
	if err := html.WriteContext(context.Background(), 11, source); err != nil {
		t.Fatal(err)
	}
	if pdf.PageCount() < 2 || pdf.PageNo() != pdf.PageCount() || pdf.GetX() != 16 || pdf.GetY() < 18 || pdf.GetY() >= 82 {
		t.Fatalf("continued HTML exit = page %d/%d cursor %.2f,%.2f", pdf.PageNo(), pdf.PageCount(), pdf.GetX(), pdf.GetY())
	}
	if pdf.fontFamily != "times" || pdf.fontStyle != "BI" || !pdf.underline || pdf.fontSizePt != 11 {
		t.Fatal("continued HTML did not restore captured font context")
	}
}
