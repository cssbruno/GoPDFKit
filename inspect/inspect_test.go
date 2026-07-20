// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package inspect

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestInspectGeneratedPDF(t *testing.T) {
	pdfBytes := inspectTestPDF(t)

	if err := ValidateStructure(pdfBytes); err != nil {
		t.Fatalf("ValidateStructure() error = %v", err)
	}

	count, err := PageCount(pdfBytes)
	if err != nil {
		t.Fatalf("PageCount() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("PageCount() = %d, want 2", count)
	}

	width, height, err := FirstPageSizePoints(pdfBytes)
	if err != nil {
		t.Fatalf("FirstPageSizePoints() error = %v", err)
	}
	if width <= 0 || height <= 0 {
		t.Fatalf("FirstPageSizePoints() = %f, %f, want positive dimensions", width, height)
	}

	text, err := Text(pdfBytes)
	if err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if !strings.Contains(text, "Inspect page one") || !strings.Contains(text, "Inspect page two") {
		t.Fatalf("Text() = %q, want both page strings", text)
	}

	pageText, err := PageText(pdfBytes, 2)
	if err != nil {
		t.Fatalf("PageText() error = %v", err)
	}
	if !strings.Contains(pageText, "Inspect page two") {
		t.Fatalf("PageText() = %q, want second page string", pageText)
	}

	streams, err := DecodedStreams(pdfBytes)
	if err != nil {
		t.Fatalf("DecodedStreams() error = %v", err)
	}
	if len(streams) == 0 {
		t.Fatal("DecodedStreams() returned no streams")
	}
}

func TestInspectContextCanceled(t *testing.T) {
	pdfBytes := inspectTestPDF(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := ValidateStructureContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateStructureContext() error = %v, want context.Canceled", err)
	}
	if _, err := PageCountContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("PageCountContext() error = %v, want context.Canceled", err)
	}
	if _, err := TextContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("TextContext() error = %v, want context.Canceled", err)
	}
	if _, err := PageTextContext(ctx, pdfBytes, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("PageTextContext() error = %v, want context.Canceled", err)
	}
	if _, err := DecodedStreamsContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodedStreamsContext() error = %v, want context.Canceled", err)
	}
}

func TestTextFromContentStreamPrefersActualTextWithoutDuplicatingVisibleGlyphs(t *testing.T) {
	stream := []byte(`/Span << /ActualText ( authored) >> BDC BT /F1 12 Tf [(a) (u) (t) (h) (o) (r) (e) (d)] TJ ET EMC`)
	text, err := textFromContentStreamContext(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if text != " authored" {
		t.Fatalf("text = %q, want extraction replacement once", text)
	}
}

func TestFirstPageSizePointsUsesMediaBoxDimensions(t *testing.T) {
	pdfBytes := inspectTestPDF(t)
	pdfBytes = bytes.Replace(
		pdfBytes,
		[]byte("/MediaBox [0 0 595.28 841.89]"),
		[]byte("/MediaBox [-10.5 20 110 220 ]"),
		1,
	)

	width, height, err := FirstPageSizePoints(pdfBytes)
	if err != nil {
		t.Fatalf("FirstPageSizePoints() error = %v", err)
	}
	if width != 120.5 || height != 200 {
		t.Fatalf("FirstPageSizePoints() = %v, %v; want 120.5, 200", width, height)
	}
}

func TestFirstPageSizePointsRejectsUnparsedFragment(t *testing.T) {
	if _, _, err := FirstPageSizePoints([]byte("/MediaBox [0 0 10 20]")); err == nil {
		t.Fatal("FirstPageSizePoints() error = nil, want invalid PDF error")
	}
}

func TestDecodedStreamsEnforcesAggregateLimits(t *testing.T) {
	data := inspectPDFWithContentStreams("abc", "def")
	if _, err := decodedStreamsContext(context.Background(), data, 3, 5, 2); err == nil || !strings.Contains(err.Error(), "decoded pdf streams exceed") {
		t.Fatalf("decodedStreamsContext() aggregate limit error = %v", err)
	}
	if _, err := decodedStreamsContext(context.Background(), data, 3, 6, 1); err == nil || !strings.Contains(err.Error(), "stream count") {
		t.Fatalf("decodedStreamsContext() stream-count limit error = %v", err)
	}
}

func TestInspectTextUsesOnlyPageContentsAndHonorsStreamLength(t *testing.T) {
	page := "BT /F1 12 Tf (before endstream after) Tj ET"
	attachment := "BT (attachment text must stay hidden) Tj ET"
	data := inspectPDFWithBodies([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(attachment), attachment),
	})
	text, err := Text(data)
	if err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if !strings.Contains(text, "before endstream after") {
		t.Fatalf("Text() = %q, want literal endstream text", text)
	}
	if strings.Contains(text, "attachment text") {
		t.Fatalf("Text() = %q, must not include a non-page stream", text)
	}
	streams, err := DecodedStreams(data)
	if err != nil {
		t.Fatalf("DecodedStreams() error = %v", err)
	}
	foundAttachment := false
	for _, stream := range streams {
		foundAttachment = foundAttachment || bytes.Contains(stream, []byte("attachment text must stay hidden"))
	}
	if !foundAttachment {
		t.Fatal("DecodedStreams() did not preserve its all-stream contract")
	}
}

func TestDecodedStreamsResolvesIndirectLengthAndIgnoresMarkersInData(t *testing.T) {
	content := "BT (literal endstream and stream markers) Tj ET"
	data := inspectPDFWithIndirectContentLength(content)
	streams, err := DecodedStreams(data)
	if err != nil {
		t.Fatalf("DecodedStreams() error = %v", err)
	}
	if len(streams) != 1 || string(streams[0]) != content {
		t.Fatalf("DecodedStreams() = %q, want [%q]", streams, content)
	}
}

func TestDecodedStreamsIgnoresFakeIntegerObjectsInsideStreamData(t *testing.T) {
	content := "5 0 obj 999 endobj"
	data := inspectPDFWithIndirectContentLength(content)
	streams, err := DecodedStreams(data)
	if err != nil {
		t.Fatalf("DecodedStreams() error = %v", err)
	}
	if len(streams) != 1 || string(streams[0]) != content {
		t.Fatalf("DecodedStreams() = %q, want [%q]", streams, content)
	}
}

func TestDecodedStreamsOnlyDecodesAnExactSingleFlateFilter(t *testing.T) {
	raw := []byte("not-zlib")
	for name, dict := range map[string]string{
		"filter chain":  "/Filter [/ASCII85Decode /FlateDecode]",
		"decode params": "/Filter /FlateDecode /DecodeParms << /Predictor 12 >>",
	} {
		t.Run(name, func(t *testing.T) {
			streams, err := DecodedStreams(inspectPDFWithFilteredContent(raw, dict))
			if err != nil {
				t.Fatalf("DecodedStreams() error = %v", err)
			}
			if len(streams) != 1 || !bytes.Equal(streams[0], raw) {
				t.Fatalf("DecodedStreams() = %q, want raw bytes %q", streams, raw)
			}
		})
	}

	var encoded bytes.Buffer
	writer := zlib.NewWriter(&encoded)
	if _, err := writer.Write([]byte("decoded")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	streams, err := DecodedStreams(inspectPDFWithFilteredContent(encoded.Bytes(), "/Filter [/Fl#61teDecode]"))
	if err != nil {
		t.Fatalf("DecodedStreams() single Flate error = %v", err)
	}
	if len(streams) != 1 || string(streams[0]) != "decoded" {
		t.Fatalf("DecodedStreams() single Flate = %q, want decoded", streams)
	}
}

func TestDecodedStreamsRejectsLengthThatDoesNotReachEndstream(t *testing.T) {
	data := inspectPDFWithBodies([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 4 0 R >>",
		"<< /Length 2 >>\nstream\nabc\nendstream",
	})
	if _, err := DecodedStreams(data); err == nil || !strings.Contains(err.Error(), "endstream") {
		t.Fatalf("DecodedStreams() error = %v, want explicit invalid endstream error", err)
	}
}

func inspectPDFWithContentStreams(contents ...string) []byte {
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
	}
	refs := make([]string, len(contents))
	for i := range contents {
		refs[i] = fmt.Sprintf("%d 0 R", i+4)
	}
	bodies = append(bodies, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents [%s] >>", strings.Join(refs, " ")))
	for _, content := range contents {
		bodies = append(bodies, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	return inspectPDFWithBodies(bodies)
}

func inspectPDFWithIndirectContentLength(content string) []byte {
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length 5 0 R >>\nstream\n%s\nendstream", content),
		fmt.Sprintf("%d", len(content)),
	}
	return inspectPDFWithBodies(bodies)
}

func inspectPDFWithFilteredContent(content []byte, filterDictionary string) []byte {
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d %s >>\nstream\n%s\nendstream", len(content), filterDictionary, content),
	}
	return inspectPDFWithBodies(bodies)
}

func inspectPDFWithBodies(bodies []string) []byte {
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(bodies)+1)
	for i, body := range bodies {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(pdf.String())
}

func inspectTestPDF(t *testing.T) []byte {
	t.Helper()
	first := "BT /F1 12 Tf (Inspect page one) Tj ET"
	second := "BT /F1 12 Tf (Inspect page two) Tj ET"
	return inspectPDFWithBodies([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.28 841.89] /Resources <<>> /Contents 5 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.28 841.89] /Resources <<>> /Contents 6 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(first), first),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(second), second),
	})
}
