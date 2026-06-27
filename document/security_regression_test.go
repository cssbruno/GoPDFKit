// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/testpdf"
)

func TestSecurityMalformedUTF8DoesNotPanic(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Output panicked: %v", r)
		}
	}()

	pdf.SetTitle(string([]byte{0xe2}), true)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(10, 10, "ok")

	if err := pdf.Output(&bytes.Buffer{}); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
}

func TestSecurityPNGAlphaOverflowReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterImageOptionsReader panicked: %v", r)
		}
	}()

	_ = pdf.RegisterImageOptionsReader("huge-alpha", ImageOptions{ImageType: "png"}, bytes.NewReader(securityPNG(
		0x7fffffff,
		0x7fffffff,
		6,
		securityPNGChunk("IDAT", securityZlibBytes(nil)),
		securityPNGChunk("IEND", nil),
	)))
	if pdf.Error() == nil {
		t.Fatal("expected invalid PNG alpha channel size error")
	}
}

func TestSecurityPNGAlphaDecodedLimitReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterImageOptionsReader panicked: %v", r)
		}
	}()

	_ = pdf.RegisterImageOptionsReader("large-alpha", ImageOptions{ImageType: "png"}, bytes.NewReader(securityPNG(
		100000,
		1000,
		6,
		securityPNGChunk("IDAT", securityZlibBytes(nil)),
		securityPNGChunk("IEND", nil),
	)))
	if pdf.Error() == nil {
		t.Fatal("expected PNG alpha decoded-size error")
	}
}

func TestSecurityPNGNonAlphaPixelLimitReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterImageOptionsReader panicked: %v", r)
		}
	}()

	_ = pdf.RegisterImageOptionsReader("large-rgb", ImageOptions{ImageType: "png"}, bytes.NewReader(securityPNG(
		10000,
		6000,
		2,
		securityPNGChunk("IDAT", securityZlibBytes([]byte{0})),
		securityPNGChunk("IEND", nil),
	)))
	if pdf.Error() == nil {
		t.Fatal("expected PNG pixel-count limit error")
	}
}

func TestSecurityPNGDimensionLimitReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterImageOptionsReader panicked: %v", r)
		}
	}()

	_ = pdf.RegisterImageOptionsReader("wide-rgb", ImageOptions{ImageType: "png"}, bytes.NewReader(securityPNG(
		uint32(maxImageDimension+1),
		1,
		2,
		securityPNGChunk("IDAT", securityZlibBytes([]byte{0})),
		securityPNGChunk("IEND", nil),
	)))
	if pdf.Error() == nil {
		t.Fatal("expected PNG dimension limit error")
	}
}

func TestSecurityOversizedGIFDimensionsRejected(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterImageOptionsReader panicked: %v", r)
		}
	}()

	_ = pdf.RegisterImageOptionsReader("huge-gif", ImageOptions{ImageType: "gif"}, bytes.NewReader(securityGIF(65535, 65535)))
	if pdf.Error() == nil {
		t.Fatal("expected GIF dimension limit error")
	}
}

func TestSecurityMaskImageFileSizeLimit(t *testing.T) {
	maskPath := filepath.Join(t.TempDir(), "oversized-mask.png")
	file, err := os.Create(maskPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := file.Truncate(int64(maxImageSourceBytes) + 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	pdf := New("P", "mm", "A4", "")
	pdf.applyExternalImageMask(&ImageInfo{w: 1, h: 1}, maskPath, ImageOptions{ImageType: "png"})
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "image data exceeds maximum size") {
		t.Fatalf("mask error = %v, want image data size limit", pdf.Error())
	}
}

func TestSecurityPDFImportFileSizeLimit(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "oversized.pdf")
	file, err := os.Create(sourcePath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := file.Truncate(int64(maxPDFImportSourceBytes) + 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	pdf := New("P", "mm", "A4", "")
	if page := pdf.ImportPage(sourcePath, 1, "MediaBox"); page != 0 {
		t.Fatalf("ImportPage() page = %d, want 0", page)
	}
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "PDF import source exceeds maximum size") {
		t.Fatalf("ImportPage() error = %v, want PDF import size limit", pdf.Error())
	}
}

func TestSecuritySVGSourceSizeLimit(t *testing.T) {
	_, err := SVGParse([]byte(strings.Repeat(" ", maxSVGSourceBytes+1)))
	if err == nil || !strings.Contains(err.Error(), "SVG source exceeds maximum size") {
		t.Fatalf("SVGParse() error = %v, want SVG source size limit", err)
	}
}

func TestSecurityThumbnailHugeDimensionsRejected(t *testing.T) {
	_, _, err := GenerateThumbnail(bytes.NewReader(securityGIF(65535, 65535)), ThumbnailOptions{MaxWidth: 16, MaxHeight: 16})
	if err == nil || !strings.Contains(err.Error(), "thumbnail dimensions exceed maximum image size") {
		t.Fatalf("GenerateThumbnail() error = %v, want thumbnail dimension limit", err)
	}
}

func TestSecurityFontDefinitionReaderSizeLimit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddFontFromReader("bad", "", strings.NewReader(strings.Repeat(" ", maxFontDefinitionBytes+1)))
	if pdf.Error() == nil || !strings.Contains(pdf.Error().Error(), "font data exceeds maximum size") {
		t.Fatalf("AddFontFromReader() error = %v, want font data size limit", pdf.Error())
	}
}

func TestSecurityFontCacheFileSizeLimit(t *testing.T) {
	fontPath := filepath.Join(t.TempDir(), "oversized.ttf")
	file, err := os.Create(fontPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := file.Truncate(int64(maxFontSourceBytes) + 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err = NewFontCache().AddUTF8Font("bad", "", fontPath)
	if err == nil || !strings.Contains(err.Error(), "font data exceeds maximum size") {
		t.Fatalf("AddUTF8Font() error = %v, want font data size limit", err)
	}
}

func TestSecurityHTMLHugeColspanDoesNotAllocateUnbounded(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)

	html := pdf.HTMLNew()
	html.Write(5, `<table><tr><td colspan="1000000000">x</td></tr></table>`)
	if err := pdf.Output(&bytes.Buffer{}); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
}

func TestSecurityInvalidLinkIDReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.Link(10, 10, 20, 20, 999)
	if pdf.Error() == nil {
		t.Fatal("expected invalid link id error")
	}
}

func TestSecurityValidLinkIDStillWorks(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	link := pdf.AddLink()
	pdf.SetLink(link, 10, 1)
	pdf.Link(10, 10, 20, 20, link)
	if err := pdf.Output(&bytes.Buffer{}); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
}

func TestSecurityInvalidLinkDestinationPageReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	link := pdf.AddLink()
	pdf.SetLink(link, 10, 99)
	if pdf.Error() == nil {
		t.Fatal("expected invalid link destination page error")
	}
}

func TestSecurityImportedObjectOffsetReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	hash := strings.Repeat("a", 40)
	pdf.ImportObjects(map[string][]byte{hash: []byte("short")})
	pdf.ImportObjPos(map[string]map[int]string{hash: {8: hash}})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Output panicked: %v", r)
		}
	}()
	err := pdf.Output(&bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid imported object replacement offset") {
		t.Fatalf("Output() error = %v, want invalid replacement offset", err)
	}
}

func TestSecurityImportedTemplateNameRejected(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.UseImportedTemplate("/Tpl Q 1 0 0 1 0 0 cm", 1, 1, 0, 0)
	if pdf.Error() == nil {
		t.Fatal("expected invalid imported template name error")
	}
}

func TestSecurityInvalidTemplateDecodeRejected(t *testing.T) {
	tpl := &DocumentTpl{
		bytes: [][]byte{nil, []byte("q")},
		page:  3,
	}
	encoded, err := tpl.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}

	_, err = DeserializeTemplate(encoded)
	if err == nil || !strings.Contains(err.Error(), "invalid template page index") {
		t.Fatalf("DeserializeTemplate() error = %v, want invalid page index", err)
	}
}

func TestSecuritySerializedTemplateSizeLimit(t *testing.T) {
	_, err := DeserializeTemplate(bytes.Repeat([]byte{'x'}, maxTemplateSerializedBytes+1))
	if err == nil || !strings.Contains(err.Error(), "serialized template exceeds maximum size") {
		t.Fatalf("DeserializeTemplate() error = %v, want serialized template size limit", err)
	}
}

func TestSecurityTemplatePageContentSizeLimit(t *testing.T) {
	tpl := &DocumentTpl{
		bytes: [][]byte{nil, bytes.Repeat([]byte{'q'}, maxTemplatePageBytes+1)},
		page:  1,
	}
	if err := tpl.validate(); err == nil || !strings.Contains(err.Error(), "template page content exceeds maximum size") {
		t.Fatalf("template validate error = %v, want page content size limit", err)
	}
}

func TestSecurityInvalidTemplateImageRejected(t *testing.T) {
	tpl := &DocumentTpl{
		bytes: [][]byte{nil, []byte("q")},
		page:  1,
		images: map[string]*ImageInfo{
			"bad": {
				data: []byte("x"),
				w:    1,
				h:    1,
				cs:   "DeviceRGB",
				bpc:  8,
				f:    "FlateDecode",
				dp:   "/Predictor 15 >> /AA <<",
			},
		},
	}
	encoded, err := tpl.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}

	_, err = DeserializeTemplate(encoded)
	if err == nil || !strings.Contains(err.Error(), "invalid image decode parameters") {
		t.Fatalf("DeserializeTemplate() error = %v, want invalid image decode params", err)
	}
}

func TestSecurityMalformedFontJSONReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddFontFromBytes("bad", "", []byte(`{"File":"x"}`), []byte("font"))
	if pdf.Error() == nil {
		t.Fatal("expected invalid font definition error")
	}
}

func TestSecurityMalformedFontReaderReturnsError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddFontFromReader("bad", "", strings.NewReader(`{"File":"x"}`))
	if pdf.Error() == nil {
		t.Fatal("expected invalid font definition error")
	}
}

func TestSecurityCompareHelpers(t *testing.T) {
	if err := testpdf.ComparePDFs(bytes.NewReader([]byte("same")), bytes.NewReader([]byte("same")), false); err != nil {
		t.Fatalf("ComparePDFs() error = %v", err)
	}
	if err := testpdf.CompareBytes([]byte("a"), []byte("ab"), false); err == nil {
		t.Fatal("CompareBytes() accepted different lengths")
	}
}

func TestSecuritySVGRejectsNonFiniteNumbers(t *testing.T) {
	for _, svg := range []string{
		`<svg width="10" height="10"><path d="M NaN 0 L 1 1"/></svg>`,
		`<svg width="NaN" height="10"><path d="M0 0 L1 1"/></svg>`,
		`<svg width="10" height="10" viewBox="0 0 Inf 10"><path d="M0 0 L1 1"/></svg>`,
		`<svg width="10" height="10"><polyline points="0,0 NaN,1"/></svg>`,
		`<svg width="10" height="10"><g transform="translate(NaN,1)"><path d="M0 0 L1 1"/></g></svg>`,
	} {
		if _, err := SVGParse([]byte(svg)); err == nil {
			t.Fatalf("SVGParse(%q) accepted non-finite numeric value", svg)
		}
	}
}

func TestSecuritySVGRejectsExcessiveNesting(t *testing.T) {
	var svg strings.Builder
	svg.WriteString(`<svg width="1" height="1">`)
	for range svgMaxNestingDepth + 2 {
		svg.WriteString(`<g>`)
	}
	svg.WriteString(`<path d="M0 0 L1 1"/>`)
	for range svgMaxNestingDepth + 2 {
		svg.WriteString(`</g>`)
	}
	svg.WriteString(`</svg>`)
	if _, err := SVGParse([]byte(svg.String())); err == nil {
		t.Fatal("SVGParse accepted excessive nesting")
	}
}

func TestSecurityHTMLDataImageSizeLimit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.MaxDataImageBytes = 8
	html.Write(5, `<img src="data:image/png;base64,`+strings.Repeat("A", 64)+`"/>`)
	if pdf.Error() == nil {
		t.Fatal("expected HTML data image size error")
	}
}

func TestSecurityHTMLInputSizeLimit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.MaxHTMLBytes = 8
	html.Write(5, "<p>too large</p>")
	if pdf.Error() == nil {
		t.Fatal("expected HTML input size error")
	}
	if !strings.Contains(pdf.Error().Error(), "HTML input exceeds maximum size") {
		t.Fatalf("HTML input size error = %v", pdf.Error())
	}

	validatorPDF := New("P", "mm", "A4", "")
	validator := validatorPDF.HTMLNew()
	validator.MaxHTMLBytes = 8
	messages := validator.ValidateHTML("<p>too large</p>")
	if len(messages) != 1 || messages[0] != "HTML input exceeds maximum size" {
		t.Fatalf("ValidateHTML messages = %#v, want input size diagnostic", messages)
	}
}

func TestSecurityHTMLTableRowLimit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.MaxTableRows = 1
	html.Write(5, `<table><tr><td>one</td></tr><tr><td>two</td></tr></table>`)
	if pdf.Error() == nil {
		t.Fatal("expected HTML table row limit error")
	}
	if !strings.Contains(pdf.Error().Error(), "HTML table row count exceeds maximum size") {
		t.Fatalf("HTML table row limit error = %v", pdf.Error())
	}
}

func TestSecurityHTMLElementDepthLimit(t *testing.T) {
	fragment := `<div><section><p>too deep</p></section></div>`

	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.MaxElementDepth = 2
	html.Write(5, fragment)
	if pdf.Error() == nil {
		t.Fatal("expected HTML element depth error")
	}
	if !strings.Contains(pdf.Error().Error(), "HTML element depth exceeds maximum size") {
		t.Fatalf("HTML element depth error = %v", pdf.Error())
	}

	validatorPDF := New("P", "mm", "A4", "")
	validator := validatorPDF.HTMLNew()
	validator.MaxElementDepth = 2
	messages := validator.ValidateHTML(fragment)
	if len(messages) != 1 || messages[0] != "HTML element depth exceeds maximum size" {
		t.Fatalf("ValidateHTML messages = %#v, want element depth diagnostic", messages)
	}
}

func TestSecurityHTMLGeneratedPageLimit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.MaxGeneratedPages = 1
	html.Write(5, `<div style="break-before: page">one</div><div style="break-before: page">two</div>`)
	if pdf.Error() == nil {
		t.Fatal("expected HTML generated page limit error")
	}
	if !strings.Contains(pdf.Error().Error(), "HTML rendering exceeded maximum generated pages") {
		t.Fatalf("HTML generated page limit error = %v", pdf.Error())
	}
}

func TestSecurityHTMLLocalImageDisabledByDefault(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(5, `<img src="/tmp/local.png"/>`)
	if pdf.Error() == nil {
		t.Fatal("expected local HTML image error")
	}
}

func TestSecurityHTMLRejectsUnsafeLinkSchemes(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(5, `<a href="javascript:app.alert(1)">x</a>`)
	if pdf.Error() == nil {
		t.Fatal("expected unsafe HTML link scheme error")
	}
}

func TestSecurityHTMLCSSParsingIsCapped(t *testing.T) {
	var css strings.Builder
	for i := range htmlMaxCSSRules + 128 {
		fmt.Fprintf(&css, ".c%d{color:#000}", i)
	}
	rules := parseHTMLCSSRules(css.String())
	if len(rules) != htmlMaxCSSRules {
		t.Fatalf("len(rules) = %d, want cap %d", len(rules), htmlMaxCSSRules)
	}
}

func TestSecurityTemplateTypedNilChildReturnsError(t *testing.T) {
	tpl := &DocumentTpl{
		bytes:     [][]byte{nil, []byte("q")},
		page:      1,
		templates: []Template{(*DocumentTpl)(nil)},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("template validation panicked: %v", r)
		}
	}()
	_ = tpl.childrenImages()
	_ = tpl.childrenTemplates()
	if err := tpl.validate(); err == nil {
		t.Fatal("validate accepted typed-nil child template")
	}
}

func TestSecurityAliasReplacementEscaped(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Write(5, "{alias}")
	pdf.RegisterAlias("{alias}", "\") Tj ET\nq 1 0 0 rg\nBT (")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if regexp.MustCompile(`[^\\]\) Tj ET\nq 1 0 0 rg`).MatchString(out.String()) {
		t.Fatal("alias replacement injected raw PDF operators")
	}
}

func TestSecurityMalformedFontDiffRejected(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddFontFromBytes("bad", "", []byte(securityFontJSON(`0 /A] >> /Injected <<`)), []byte("font"))
	if pdf.Error() == nil {
		t.Fatal("expected invalid font diff error")
	}
}

func TestSecurityUnsafeUTF8FontNameRejected(t *testing.T) {
	fontBytes, err := os.ReadFile(securityFixturePath(t, "assets", "static", "font", "DejaVuSansCondensed.ttf"))
	if err != nil {
		t.Fatalf("ReadFile font: %v", err)
	}
	pdf := New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("Bad/Font", "", fontBytes)
	if pdf.Error() == nil {
		t.Fatal("expected invalid UTF-8 font name error")
	}
}

func securityFixturePath(t *testing.T, elems ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	parts := append([]string{filepath.Dir(file), ".."}, elems...)
	return filepath.Clean(filepath.Join(parts...))
}

func TestSecurityNonFiniteDrawingInputsRejected(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SubWrite(math.NaN(), "x", 6, 0, 0, "")
	if pdf.Error() == nil {
		t.Fatal("expected non-finite SubWrite error")
	}
}

func securityFontJSON(diff string) string {
	widths := strings.TrimRight(strings.Repeat("1,", 256), ",")
	return fmt.Sprintf(`{"Tp":"TrueType","Name":"SafeFont","Cw":[%s],"File":"safe.z","OriginalSize":1,"Diff":%q}`, widths, diff)
}

func securityPNG(width, height uint32, pdfColor byte, chunks ...[]byte) []byte {
	var out bytes.Buffer
	out.WriteString("\x89PNG\x0d\x0a\x1a\x0a")
	var ihdr bytes.Buffer
	_ = binary.Write(&ihdr, binary.BigEndian, width)
	_ = binary.Write(&ihdr, binary.BigEndian, height)
	ihdr.Write([]byte{8, pdfColor, 0, 0, 0})
	out.Write(securityPNGChunk("IHDR", ihdr.Bytes()))
	for _, chunk := range chunks {
		out.Write(chunk)
	}
	return out.Bytes()
}

func securityPNGChunk(chunkType string, data []byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
	out.WriteString(chunkType)
	out.Write(data)
	_ = binary.Write(&out, binary.BigEndian, uint32(0))
	return out.Bytes()
}

func securityGIF(width, height uint16) []byte {
	var out bytes.Buffer
	out.WriteString("GIF89a")
	_ = binary.Write(&out, binary.LittleEndian, width)
	_ = binary.Write(&out, binary.LittleEndian, height)
	out.Write([]byte{0, 0, 0, ';'})
	return out.Bytes()
}

func securityZlibBytes(data []byte) []byte {
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	_, _ = w.Write(data)
	_ = w.Close()
	return out.Bytes()
}

func TestSecurityImageInfoValidationRejectsPDFSyntax(t *testing.T) {
	info := &ImageInfo{
		data: []byte("x"),
		w:    1,
		h:    1,
		cs:   "DeviceRGB",
		bpc:  8,
		f:    "FlateDecode",
		dp:   "/Predictor 15 /Colors 3 /BitsPerComponent 8 /Columns 1 " + ">>",
	}
	if err := info.validForPDF(); err == nil {
		t.Fatal("expected invalid decode parameters error")
	}
}
