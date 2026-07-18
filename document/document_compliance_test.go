// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/testsupport/example"
)

func TestSetXmpMetadataReferencedFromCatalog(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetXmpMetadata([]byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/">custom</x:xmpmeta>`))
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "XMP")

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "/Type /Metadata /Subtype /XML") {
		t.Fatal("generated PDF does not contain XMP metadata object")
	}
	if !strings.Contains(text, "/Metadata ") {
		t.Fatal("generated PDF catalog does not reference XMP metadata")
	}
	if !strings.Contains(text, "custom") {
		t.Fatal("generated PDF does not contain custom XMP payload")
	}
}

func TestComplianceMetadataGeneratesPDFA4AndPDFUA2Identifiers(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetTitle("Compliance metadata", false)
	pdf.SetSubject("Generated standards metadata", false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{
		PDFA:       document.PDFAMode4F,
		PDFUA2:     true,
		Arlington:  true,
		Lang:       "en-US",
		Identifier: "urn:uuid:gopdfkit-test",
	})
	if err := pdf.SetOutputIntent([]byte("test-icc-profile"), "sRGB IEC61966-2.1"); err != nil {
		t.Fatalf("SetOutputIntent() error = %v", err)
	}
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.Cell(40, 10, "Compliance")

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"%PDF-2.0",
		"/Metadata ",
		"/MarkInfo << /Marked true >>",
		"/Lang (en-US)",
		"/ViewerPreferences << /DisplayDocTitle true >>",
		"/OutputIntents [ << /Type /OutputIntent /S /GTS_PDFA1",
		"/OutputConditionIdentifier (sRGB IEC61966-2.1)",
		"/DestOutputProfile ",
		"/N 3 /Alternate /DeviceRGB",
		"test-icc-profile",
		"<pdfaid:part>4</pdfaid:part>",
		"<pdfaid:conformance>F</pdfaid:conformance>",
		"<pdfuaid:part>2</pdfuaid:part>",
		"<pdfuaid:rev>2024</pdfuaid:rev>",
		"<gopdfkit:ArlingtonValidationRequired>True</gopdfkit:ArlingtonValidationRequired>",
		"urn:uuid:gopdfkit-test",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated PDF does not contain %q", want)
		}
	}
}

func TestPDFUA2TaggedPDFStructureTreeAndMarkedContent(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{
		PDFUA2: true,
		Lang:   "en-US",
		Title:  "Tagged PDF",
	})
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.SetNextTextRole("H1")
	pdf.Cell(0, 8, "Heading")
	pdf.Ln(10)
	pdf.CellFormat(0, 8, "External link", "", 1, "L", false, 0, "https://example.com")
	pdf.ImageOptions(example.ImageFile("logo.png"), 10, 35, 18, 0, false, document.ImageOptions{
		ImageType: "png",
		AltText:   "GoPDFKit logo",
	}, 0, "")

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/StructTreeRoot ",
		"/Type /StructTreeRoot",
		"/ParentTree ",
		"/Type /StructElem",
		"/S /Document",
		"/S /H1",
		"/S /Link",
		"/S /Figure",
		"/StructParents 0",
		"/StructParent ",
		"/H1 <</MCID 0>> BDC",
		"/Link <</MCID 1>> BDC",
		"/Figure <</MCID 2>> BDC",
		"/Type /Annot /Subtype /Link",
		"/Type /OBJR /Obj ",
		"/K << /Type /MCR",
		"/MCID 0",
		"/MCID 1",
		"/MCID 2",
		"/Nums [0 [",
		"/Subtype /Link /Rect",
		"/F 4",
		"/Alt (",
		string([]byte{0xfe, 0xff, 0x00, 'G', 0x00, 'o'}),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged PDF does not contain %q", want)
		}
	}
}

func TestPDFUA2HTMLUsesSemanticRolesAndImageAlt(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged HTML"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.AllowLocalImages = true
	html.Write(6, `<h2>HTML heading</h2><p>HTML paragraph</p><img src="`+example.ImageFile("logo.png")+`" alt="HTML logo" width="12">`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/H2 <</MCID 0>> BDC",
		"/P <</MCID 1>> BDC",
		"/Figure <</MCID 2>> BDC",
		"/S /H2",
		"/S /P",
		"/S /Figure",
		"/Alt (",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged HTML PDF does not contain %q", want)
		}
	}
}

func TestPDFUA2HTMLListsAndTablesUseStructureRoles(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged HTML table"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, `<ul><li>One</li><li>Two</li></ul><table><caption>Totals</caption><tr><th>Name</th></tr><tr><td>Alpha</td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/S /L",
		"/S /LI",
		"/S /Lbl",
		"/S /LBody",
		"/S /Table",
		"/S /Caption",
		"/S /TR",
		"/S /TH",
		"/S /TD",
		"/Caption <</MCID ",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged HTML list/table PDF does not contain %q", want)
		}
	}
	if got := strings.Count(text, "/S /TH"); got != 1 {
		t.Fatalf("generated tagged HTML table has %d TH structure elements, want 1", got)
	}
	if got := strings.Count(text, "/S /TD"); got != 1 {
		t.Fatalf("generated tagged HTML table has %d TD structure elements, want 1", got)
	}
}

func TestPDFUA2HTMLTableCellsUseStructureAttributes(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged HTML table attributes"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, `<table><tr><th rowspan="2">Group</th><td colspan="2">Alpha</td></tr><tr><td>Beta</td><td>Gamma</td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/A << /O /Table /Scope /Column /RowSpan 2 >>",
		"/A << /O /Table /ColSpan 2 >>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged HTML table PDF does not contain %q", want)
		}
	}
}

func TestPDFUA2HTMLTableCellNestedListsUseListStructure(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged nested table list"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, `<table><tr><td><ul><li>Outer<ul><li>Inner</li></ul></li></ul></td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/S /Table",
		"/S /TD",
		"/S /L",
		"/S /LI",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged HTML table nested list PDF does not contain %q", want)
		}
	}
	if got := strings.Count(text, "/S /L"); got < 2 {
		t.Fatalf("generated tagged HTML table nested list has %d list structures, want at least 2", got)
	}
	if got := strings.Count(text, "/S /LI"); got < 2 {
		t.Fatalf("generated tagged HTML table nested list has %d list item structures, want at least 2", got)
	}
}

func TestPDFUA2HTMLTableCellNestedTableUsesTableStructure(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged nested table"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, `<table><tr><td>Outer<table><tr><td>Inner</td></tr></table></td><td>Sibling</td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/S /Table",
		"/S /TR",
		"/S /TD",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged HTML nested table PDF does not contain %q", want)
		}
	}
	if got := strings.Count(text, "/S /Table"); got < 2 {
		t.Fatalf("generated tagged HTML nested table has %d table structures, want at least 2", got)
	}
	if got := strings.Count(text, "/S /TR"); got < 2 {
		t.Fatalf("generated tagged HTML nested table has %d row structures, want at least 2", got)
	}
	if got := strings.Count(text, "/S /TD"); got < 3 {
		t.Fatalf("generated tagged HTML nested table has %d cell structures, want at least 3", got)
	}
}

func TestPDFUA2HTMLTableCellMixedBlocksUseParagraphStructure(t *testing.T) {
	pdf := document.MustNew(document.WithSecurityPolicy(document.SecurityPolicy{AllowLocalHTMLImages: true}))
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged mixed table cell blocks"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, `<table><tr><td><p>First paragraph</p><div>Second block</div></td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/S /Table",
		"/S /TD",
		"/S /P",
		"/P <</MCID ",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged HTML mixed table cell PDF does not contain %q", want)
		}
	}
	if got := strings.Count(text, "/S /P"); got < 2 {
		t.Fatalf("generated tagged HTML mixed table cell has %d paragraph structures, want at least 2", got)
	}
}

func TestPDFUA2LinkedInlineSVGTextSharesLinkStructureWithAnnotation(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Tagged SVG link"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, `<a href="https://example.test/svg"><svg role="presentation" width="48" height="24" viewBox="0 0 48 24"><rect x="1" y="1" width="46" height="22" fill="#00ff00" stroke="#000" stroke-width="1"/><text x="24" y="16" text-anchor="middle" font-size="10" fill="#000000">Inline SVG</text></svg></a>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/Link <</MCID 0>> BDC",
		"/S /Link",
		"/Type /OBJR /Obj",
		"/Type /Annot /Subtype /Link",
		"/K [<< /Type /MCR",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged SVG link PDF does not contain %q", want)
		}
	}
	if got := strings.Count(text, "/S /Link"); got != 1 {
		t.Fatalf("generated tagged SVG link has %d Link structure elements, want 1", got)
	}
}

func TestPDFUA2ImageRequiresAltTextOrArtifact(t *testing.T) {
	render := func(options document.ImageOptions) error {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Title: "Tagged image"})
		pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.ImageOptions(example.ImageFile("logo.png"), 10, 10, 18, 0, false, options, 0, "")
		pdf.Cell(0, 8, "Tagged text")
		var output bytes.Buffer
		return pdf.Output(&output)
	}

	err := render(document.ImageOptions{ImageType: "png"})
	if err == nil {
		t.Fatal("Output() error = nil, want missing alt-text rejection")
	}
	if !strings.Contains(err.Error(), "alternate text") {
		t.Fatalf("Output() error = %v, want alternate text rejection", err)
	}
	if err := render(document.ImageOptions{ImageType: "png", Artifact: true}); err != nil {
		t.Fatalf("artifact image Output() error = %v", err)
	}
}

func TestPDFUA2DirectDrawingAndRawContentCanBeArtifacts(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Title: "Artifacts"})
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.Cell(0, 8, "Tagged text")
	pdf.Line(10, 20, 40, 20)
	pdf.Rect(10, 25, 20, 8, "D")
	pdf.RawWriteArtifactStr("0 0 m 10 10 l S")

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	if got := strings.Count(text, "/Artifact BMC"); got < 3 {
		t.Fatalf("artifact count = %d, want at least 3", got)
	}
	if !strings.Contains(text, "0 0 m 10 10 l S") {
		t.Fatal("generated PDF does not contain raw artifact content")
	}
	if !strings.Contains(text, "/P <</MCID 0>> BDC") {
		t.Fatal("generated PDF does not contain tagged text MCID")
	}
}

func TestPDFUA2TemplatesAndImportedPagesAreArtifacts(t *testing.T) {
	source := document.MustNew()
	source.SetCompression(false)
	source.AddPage()
	source.SetFont("Helvetica", "", 12)
	source.Cell(0, 8, "Imported source")
	var sourceBytes bytes.Buffer
	if err := source.Output(&sourceBytes); err != nil {
		t.Fatalf("source Output() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Title: "Artifacts"})
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.Cell(0, 8, "Tagged text")
	tpl := pdf.CreateTemplate(func(t *document.Tpl) {
		t.Rect(0, 0, 20, 8, "D")
	})
	pdf.UseTemplate(tpl)
	pageID := pdf.ImportPageStream(bytes.NewReader(sourceBytes.Bytes()), 1, "MediaBox")
	pdf.UseImportedPage(pageID, 30, 30, 20, 0)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/P <</MCID 0>> BDC",
		"/Artifact BMC",
		"/TPL",
		"/IPG",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated tagged artifact PDF does not contain %q", want)
		}
	}
}

func TestPDFUA2RejectsUntaggedRawWrites(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Title: "Raw"})
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.Cell(0, 8, "Tagged text")
	pdf.RawWriteStr("0 0 m 10 10 l S")

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want tagged raw write rejection")
	}
	if !strings.Contains(err.Error(), "raw writes") {
		t.Fatalf("Output() error = %v, want raw write rejection", err)
	}
}

func TestPDFUA2RejectsUnclosedStructure(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Title: "Unclosed"})
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.BeginStructure("Sect")
	pdf.Cell(0, 8, "Tagged text")

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want unclosed structure rejection")
	}
	if !strings.Contains(err.Error(), "unclosed structure") {
		t.Fatalf("Output() error = %v, want unclosed structure rejection", err)
	}
}

func TestComplianceMetadataPDFARejectsJavaScript(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFA: document.PDFAMode4})
	if err := pdf.SetOutputIntent([]byte("test-icc-profile"), "sRGB IEC61966-2.1"); err != nil {
		t.Fatalf("SetOutputIntent() error = %v", err)
	}
	_ = pdf.SetJavascriptError("app.alert('no')")
	pdf.AddPage()

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want PDF/A JavaScript rejection")
	}
	if !strings.Contains(err.Error(), "JavaScript") {
		t.Fatalf("Output() error = %v, want JavaScript rejection", err)
	}
}

func TestComplianceMetadataPDFARejectsEncryption(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFA: document.PDFAMode4})
	if err := pdf.SetOutputIntent([]byte("test-icc-profile"), "sRGB IEC61966-2.1"); err != nil {
		t.Fatalf("SetOutputIntent() error = %v", err)
	}
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	if err := pdf.SetLegacyProtection(document.CnProtectPrint, "reader", "owner"); err != nil {
		t.Fatalf("SetLegacyProtection() error = %v", err)
	}
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.Cell(40, 10, "Encrypted")

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want PDF/A encryption rejection")
	}
	if !strings.Contains(err.Error(), "encrypted") {
		t.Fatalf("Output() error = %v, want encryption rejection", err)
	}
}

func TestComplianceMetadataPDFARejectsCoreFonts(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFA: document.PDFAMode4})
	if err := pdf.SetOutputIntent([]byte("test-icc-profile"), "sRGB IEC61966-2.1"); err != nil {
		t.Fatalf("SetOutputIntent() error = %v", err)
	}
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "Core font")

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want core font rejection")
	}
	if !strings.Contains(err.Error(), "UTF-8 fonts") {
		t.Fatalf("Output() error = %v, want UTF-8 font rejection", err)
	}
}

func TestComplianceMetadataPDFA4RejectsAttachmentsUnless4fOr4e(t *testing.T) {
	render := func(mode document.PDFAMode) error {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFA: mode})
		if err := pdf.SetOutputIntent([]byte("test-icc-profile"), "sRGB IEC61966-2.1"); err != nil {
			t.Fatalf("SetOutputIntent() error = %v", err)
		}
		pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
		pdf.SetAttachments([]document.Attachment{{Content: []byte("attachment"), Filename: "a.txt"}})
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.Cell(40, 10, "Attachment")
		var output bytes.Buffer
		return pdf.Output(&output)
	}

	if err := render(document.PDFAMode4); err == nil || !strings.Contains(err.Error(), "PDF/A-4f") {
		t.Fatalf("PDF/A-4 attachment error = %v, want PDF/A-4f/4e rejection", err)
	}
	if err := render(document.PDFAMode4F); err != nil {
		t.Fatalf("PDF/A-4f attachment Output() error = %v", err)
	}
}

func TestComplianceMetadataPDFARequiresOutputIntent(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFA: document.PDFAMode4})
	pdf.AddPage()

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want output intent rejection")
	}
	if !strings.Contains(err.Error(), "output intent") {
		t.Fatalf("Output() error = %v, want output intent rejection", err)
	}
}

func TestComplianceValidationReportTracksFailuresSeparately(t *testing.T) {
	var report document.ComplianceValidationReport
	report.Add(document.ComplianceValidationIssue{
		Standard: "Arlington",
		Rule:     "Catalog::Pages",
		Message:  "example failure",
	})
	report.Add(document.ComplianceValidationIssue{
		Standard: "PDF/A-4",
		Severity: document.ComplianceValidationWarning,
		Rule:     "metadata",
		Message:  "example warning",
	})
	if !report.Failed() {
		t.Fatal("Failed() = false, want true after default error issue")
	}

	pdf := document.MustNew()
	pdf.AddPage()
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
}
