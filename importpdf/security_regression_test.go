// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSecurityDecodedStreamLimit(t *testing.T) {
	compressed := zlibBytes(bytes.Repeat([]byte{'A'}, MaxDecodedStreamBytes+1))
	_, err := decodePDFStream(pdfDict{"Filter": pdfValue{kind: pdfValueName, name: "FlateDecode"}}, compressed)
	if err == nil || !strings.Contains(err.Error(), "uncompressed data exceeds expected size") {
		t.Fatalf("decodePDFStream() error = %v, want decoded stream size limit", err)
	}
}

func TestObjRefAccessors(t *testing.T) {
	ref := ObjRef{num: 12, gen: 3}
	if ref.ObjectNumber() != 12 {
		t.Fatalf("ObjectNumber() = %d, want 12", ref.ObjectNumber())
	}
	if ref.Generation() != 3 {
		t.Fatalf("Generation() = %d, want 3", ref.Generation())
	}
	if ref.String() != "12 3" {
		t.Fatalf("String() = %q, want 12 3", ref.String())
	}
}

func TestOpenBytesWithOptionsAppliesSourceLimit(t *testing.T) {
	_, err := OpenBytesWithOptions([]byte("%PDF-too-large"), ImportOptions{MaxSourceBytes: 3})
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("OpenBytesWithOptions() error = %v, want source size limit", err)
	}
}

func TestOpenReaderWithOptionsAppliesSourceLimit(t *testing.T) {
	_, err := OpenReaderWithOptions(strings.NewReader("%PDF-too-large"), ImportOptions{MaxSourceBytes: 3})
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("OpenReaderWithOptions() error = %v, want source size limit", err)
	}
}

func TestOpenReaderRejectsNil(t *testing.T) {
	if _, err := OpenReader(nil); err == nil {
		t.Fatal("OpenReader(nil) error = nil, want error")
	}
	if _, err := OpenReaderWithOptions(nil, ImportOptions{}); err == nil {
		t.Fatal("OpenReaderWithOptions(nil) error = nil, want error")
	}
}

func TestOpenReaderWithOptionsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := OpenReaderWithOptionsContext(ctx, strings.NewReader("%PDF-1.4\n%%EOF"), ImportOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenReaderWithOptionsContext() error = %v, want context.Canceled", err)
	}
}

func TestOpenReaderAtWithOptionsContextCanceledDuringParse(t *testing.T) {
	source := minimalImportPDF()
	ctx, cancel := context.WithCancel(context.Background())
	reader := cancelingReaderAt{data: source, cancel: cancel}

	_, err := OpenReaderAtWithOptionsContext(ctx, reader, int64(len(source)), ImportOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenReaderAtWithOptionsContext() error = %v, want context.Canceled", err)
	}
}

func TestPageContextCanceled(t *testing.T) {
	source, err := OpenBytes(minimalImportPDF())
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = source.PageContext(ctx, 1, "MediaBox")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PageContext() error = %v, want context.Canceled", err)
	}
}

func TestPageRefForEachObjectPassesCopies(t *testing.T) {
	ref := ObjRef{num: 1}
	page := &PageRef{
		objects:    map[ObjRef][]byte{ref: []byte("object")},
		objectRefs: []ObjRef{ref},
	}
	if err := page.ForEachObject(func(_ ObjRef, body []byte) error {
		body[0] = 'X'
		return nil
	}); err != nil {
		t.Fatalf("ForEachObject() error = %v", err)
	}
	if got := string(page.objects[ref]); got != "object" {
		t.Fatalf("stored object body = %q, want object", got)
	}
	if err := page.ForEachObjectBorrowed(func(_ ObjRef, body []byte) error {
		body[0] = 'X'
		return nil
	}); err != nil {
		t.Fatalf("ForEachObjectBorrowed() error = %v", err)
	}
	if got := string(page.objects[ref]); got != "Xbject" {
		t.Fatalf("borrowed object body = %q, want mutation visible", got)
	}
}

func TestPageRefContentErrReportsLazyError(t *testing.T) {
	want := errors.New("content failed")
	page := &PageRef{contentErr: want}
	if !errors.Is(page.ContentErr(), want) {
		t.Fatalf("ContentErr() = %v, want %v", page.ContentErr(), want)
	}
	if content, err := page.ContentWithError(); !errors.Is(err, want) || content != nil {
		t.Fatalf("ContentWithError() = %q, %v; want nil, %v", content, err, want)
	}

	page = &PageRef{content: []byte("content")}
	content, err := page.ContentWithError()
	if err != nil {
		t.Fatalf("ContentWithError() error = %v", err)
	}
	content[0] = 'X'
	if got := string(page.content); got != "content" {
		t.Fatalf("ContentWithError returned borrowed content; stored = %q", got)
	}
}

func TestPageRefContentWithContextCanceled(t *testing.T) {
	page := &PageRef{
		source: &Source{},
		box:    pdfBox{urx: 10, ury: 10},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	content, err := page.ContentWithContext(ctx)
	if !errors.Is(err, context.Canceled) || content != nil {
		t.Fatalf("ContentWithContext() = %q, %v; want nil, context.Canceled", content, err)
	}
	if page.contentErr != nil {
		t.Fatalf("ContentWithContext canceled before lazy load poisoned ContentErr: %v", page.contentErr)
	}
	content, err = page.ContentWithError()
	if err != nil {
		t.Fatalf("ContentWithError() after canceled context error = %v", err)
	}
	if string(content) != "q\nQ" {
		t.Fatalf("ContentWithError() after canceled context = %q, want wrapped empty content", content)
	}
}

func TestSecurityValueArrayLimit(t *testing.T) {
	var input strings.Builder
	input.WriteByte('[')
	for range MaxArrayItems + 1 {
		input.WriteString("0 ")
	}
	input.WriteByte(']')

	_, err := newPDFValueParser([]byte(input.String())).parseValue()
	if err == nil || !strings.Contains(err.Error(), "PDF array exceeds maximum size") {
		t.Fatalf("parseValue() error = %v, want array size limit", err)
	}
}

func zlibBytes(data []byte) []byte {
	var out bytes.Buffer
	writer := zlib.NewWriter(&out)
	_, _ = writer.Write(data)
	_ = writer.Close()
	return out.Bytes()
}

type cancelingReaderAt struct {
	data   []byte
	cancel context.CancelFunc
}

func (r cancelingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if r.cancel != nil {
		r.cancel()
	}
	if off < 0 || off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func minimalImportPDF() []byte {
	return []byte("%PDF-1.4\n" +
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>\nendobj\n" +
		"4 0 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n" +
		"xref\n0 5\n" +
		"0000000000 65535 f \n" +
		"0000000009 00000 n \n" +
		"0000000058 00000 n \n" +
		"0000000115 00000 n \n" +
		"0000000216 00000 n \n" +
		"trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n265\n%%EOF\n")
}
