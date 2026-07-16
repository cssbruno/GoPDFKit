// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestOpenWithOptionsAppliesSourceLimitToExistingSource(t *testing.T) {
	source, err := OpenBytes(minimalImportPDF())
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
	_, err = OpenWithOptions(source, ImportOptions{MaxSourceBytes: 1})
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("OpenWithOptions(*Source) error = %v, want ErrSourceTooLarge", err)
	}
}

func TestOpenBytesRejectsOversizedStartXref(t *testing.T) {
	data := []byte("startxref100000000000000000000")
	for name, open := range map[string]func() (*Source, error){
		"bytes": func() (*Source, error) { return OpenBytes(data) },
		"reader-at": func() (*Source, error) {
			return OpenReaderAt(bytes.NewReader(data), int64(len(data)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := open()
			if err == nil || !strings.Contains(err.Error(), "startxref offset is invalid") {
				t.Fatalf("open error = %v, want invalid startxref", err)
			}
		})
	}
}

func TestOpenBytesRejectsCyclicIndirectStreamLength(t *testing.T) {
	tests := map[string][]string{
		"self": {
			"<< /Type /Catalog /Pages 2 0 R /Length 1 0 R >>\nstream\n\nendstream",
			"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
			"<< /Length 0 >>\nstream\n\nendstream",
		},
		"mutual": {
			"<< /Type /Catalog /Pages 2 0 R /Length 5 0 R >>\nstream\n\nendstream",
			"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
			"<< /Length 0 >>\nstream\n\nendstream",
			"<< /Length 1 0 R >>\nstream\n\nendstream",
		},
	}
	for name, objects := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := OpenBytes(buildSecurityPDF(t, objects))
			if err == nil || !strings.Contains(err.Error(), "resolution contains a cycle") {
				t.Fatalf("OpenBytes() error = %v, want indirect object cycle", err)
			}
		})
	}
}

func TestOpenBytesRejectsMissingOrOutOfBoundsStreamLength(t *testing.T) {
	for name, stream := range map[string]string{
		"missing":       "<<>>\nstream\n\nendstream",
		"out-of-bounds": "<< /Length 100 >>\nstream\n\nendstream",
		"wrong-type":    "<< /Length /NotANumber >>\nstream\n\nendstream",
	} {
		t.Run(name, func(t *testing.T) {
			data := buildSecurityPDF(t, []string{
				"<< /Type /Catalog /Pages 2 0 R >>",
				"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
				"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
				stream,
			})
			for opener, open := range sourceOpeners(data) {
				t.Run(opener, func(t *testing.T) {
					source, err := open()
					if err == nil {
						_, err = source.Page(1, "MediaBox")
					}
					if err == nil {
						t.Fatal("page import accepted a stream without a valid in-bounds Length")
					}
				})
			}
		})
	}
}

func TestImportedPageWrapperOwnsTrailingDelimiter(t *testing.T) {
	box := pdfBox{urx: 10, ury: 10}
	for _, content := range [][]byte{
		nil,
		[]byte("BT ET"),
		[]byte("BT ET\n"),
		[]byte("BT ET\r\n"),
		[]byte("BT ET\n\n"),
	} {
		wrapped := wrapImportedPageContent(content, box)
		if !bytes.HasPrefix(wrapped, []byte("q\n")) || !bytes.HasSuffix(wrapped, []byte("Q")) {
			t.Fatalf("wrapped content = %q, want q/Q wrapper", wrapped)
		}
		inner := wrapped[len("q\n") : len(wrapped)-1]
		if len(content) > 0 {
			if len(inner) == 0 || inner[len(inner)-1] != '\n' {
				t.Fatalf("wrapped content = %q, want owned newline delimiter", wrapped)
			}
			inner = inner[:len(inner)-1]
		}
		if !bytes.Equal(inner, content) {
			t.Fatalf("wrapper round trip = %q, want %q", inner, content)
		}
	}
}

func TestOpenBytesAcceptsValidIndirectStreamLength(t *testing.T) {
	source := buildSecurityPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R /Length 5 0 R >>\nstream\n\nendstream",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
		"0",
	})
	if _, err := OpenBytes(source); err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
}

func TestOpenBytesRejectsExcessiveIndirectStreamLengthDepth(t *testing.T) {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /Length 5 0 R >>\nstream\n\nendstream",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	for objectNumber := 5; objectNumber < 5+maxObjectResolveDepth; objectNumber++ {
		objects = append(objects, fmt.Sprintf("<< /Length %d 0 R >>\nstream\n\nendstream", objectNumber+1))
	}
	objects = append(objects, "0")
	_, err := OpenBytes(buildSecurityPDF(t, objects))
	if err == nil || !strings.Contains(err.Error(), "resolution exceeds maximum depth") {
		t.Fatalf("OpenBytes() error = %v, want indirect object depth limit", err)
	}
}

func TestOpenBytesRejectsRepeatedPageTreeObject(t *testing.T) {
	source := buildSecurityPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 3 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	})
	_, err := OpenBytes(source)
	if err == nil || !strings.Contains(err.Error(), "page tree contains a repeated object") {
		t.Fatalf("OpenBytes() error = %v, want repeated page object", err)
	}
}

func TestPageRejectsIndirectFilterAndDecodeParms(t *testing.T) {
	tests := map[string][]string{
		"indirect filter": {
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
			"<< /Length 0 /Filter 5 0 R >>\nstream\n\nendstream",
			"/FlateDecode",
		},
		"decode parms": {
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
			"<< /Length 0 /Filter /FlateDecode /DecodeParms << /Predictor 1 >> >>\nstream\n\nendstream",
		},
	}
	for name, objects := range tests {
		t.Run(name, func(t *testing.T) {
			source, err := OpenBytes(buildSecurityPDF(t, objects))
			if err != nil {
				t.Fatalf("OpenBytes() error = %v", err)
			}
			if _, err := source.Page(1, "MediaBox"); err == nil {
				t.Fatal("Page() error = nil, want unsupported stream error")
			}
		})
	}
}

func TestOpenBytesCanonicalizesEscapedStructuralNames(t *testing.T) {
	source := buildSecurityPDF(t, []string{
		"<< /T#79pe /Catalog /Pa#67es 2 0 R >>",
		"<< /T#79pe /Pages /K#69ds [3 0 R] /Count 1 >>",
		"<< /T#79pe /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	})
	if _, err := OpenBytes(source); err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
}

func TestOpenBytesDoesNotTreatFreeXrefObjectNumberAsByteOffset(t *testing.T) {
	source := buildSecurityPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	})
	source = bytes.Replace(source, []byte("0000000000 65535 f"), []byte("0000099999 65535 f"), 1)
	if _, err := OpenBytes(source); err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
}

func TestIncrementalXrefDoesNotResurrectFreedObject(t *testing.T) {
	data := appendIncrementalXref(t, minimalImportPDF(), false)
	for name, open := range sourceOpeners(data) {
		t.Run(name, func(t *testing.T) {
			source, err := open()
			if err != nil {
				t.Fatalf("open error = %v", err)
			}
			if _, err := source.Page(1, "MediaBox"); err == nil || !strings.Contains(err.Error(), "object 4 0 R was not found") {
				t.Fatalf("Page() error = %v, want freed object rejection", err)
			}
			if err := source.ForEachObjectBorrowedContext(context.Background(), func(ref ObjRef, _ []byte) error {
				if ref.ObjectNumber() == 4 {
					t.Fatalf("freed object was enumerated as %s", ref)
				}
				return nil
			}); err != nil {
				t.Fatalf("ForEachObjectBorrowedContext() error = %v", err)
			}
		})
	}
}

func TestIncrementalXrefKeepsOnlyNewestGeneration(t *testing.T) {
	data := appendIncrementalXref(t, minimalImportPDF(), true)
	for name, open := range sourceOpeners(data) {
		t.Run(name, func(t *testing.T) {
			source, err := open()
			if err != nil {
				t.Fatalf("open error = %v", err)
			}
			if _, err := source.Page(1, "MediaBox"); err == nil || !strings.Contains(err.Error(), "object 4 0 R was not found") {
				t.Fatalf("Page() error = %v, want stale-generation rejection", err)
			}
			generations := make([]int, 0, 1)
			if err := source.ForEachObjectBorrowedContext(context.Background(), func(ref ObjRef, _ []byte) error {
				if ref.ObjectNumber() == 4 {
					generations = append(generations, ref.Generation())
				}
				return nil
			}); err != nil {
				t.Fatalf("ForEachObjectBorrowedContext() error = %v", err)
			}
			if len(generations) != 1 || generations[0] != 1 {
				t.Fatalf("object 4 generations = %v, want only generation 1", generations)
			}
		})
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

func TestPageRefCancellationDuringLazyLoadDoesNotPoisonRetry(t *testing.T) {
	source, err := OpenBytes(minimalImportPDF())
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
	page := &PageRef{
		source:     source,
		sourcePage: source.pages[0],
		box:        pdfBox{urx: 10, ury: 10},
	}
	ctx := &cancelAfterErrChecksContext{cancelAt: 3}
	if _, err := page.ContentWithContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ContentWithContext() error = %v, want context.Canceled", err)
	}

	content, err := page.ContentWithError()
	if err != nil {
		t.Fatalf("ContentWithError() retry error = %v", err)
	}
	if string(content) != "q\nQ" {
		t.Fatalf("ContentWithError() retry content = %q, want q\\nQ", content)
	}
}

func TestPageRefLazyLoadIsConcurrentSafe(t *testing.T) {
	source, err := OpenBytes(minimalImportPDF())
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
	page := &PageRef{
		source:     source,
		sourcePage: source.pages[0],
		box:        pdfBox{urx: 10, ury: 10},
	}
	const callers = 16
	var wait sync.WaitGroup
	errorsCh := make(chan error, callers)
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			content, err := page.ContentWithError()
			if err == nil && string(content) != "q\nQ" {
				err = fmt.Errorf("content = %q, want q\\nQ", content)
			}
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
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

type cancelAfterErrChecksContext struct {
	checks   atomic.Int32
	cancelAt int32
}

func (c *cancelAfterErrChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterErrChecksContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterErrChecksContext) Value(any) any               { return nil }
func (c *cancelAfterErrChecksContext) Err() error {
	if c.checks.Add(1) >= c.cancelAt {
		return context.Canceled
	}
	return nil
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

func sourceOpeners(data []byte) map[string]func() (*Source, error) {
	return map[string]func() (*Source, error){
		"bytes": func() (*Source, error) { return OpenBytes(data) },
		"reader-at": func() (*Source, error) {
			return OpenReaderAt(bytes.NewReader(data), int64(len(data)))
		},
	}
}

func appendIncrementalXref(t testing.TB, base []byte, reuseGeneration bool) []byte {
	t.Helper()
	previous, err := findPDFStartXref(base)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	out.Write(base)
	if len(base) == 0 || base[len(base)-1] != '\n' {
		out.WriteByte('\n')
	}
	objectOffset := 0
	if reuseGeneration {
		objectOffset = out.Len()
		out.WriteString("4 1 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n")
	}
	xref := out.Len()
	out.WriteString("xref\n4 1\n")
	if reuseGeneration {
		fmt.Fprintf(&out, "%010d 00001 n \n", objectOffset)
	} else {
		out.WriteString("0000000000 00001 f \n")
	}
	fmt.Fprintf(&out, "trailer\n<< /Size 5 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", previous, xref)
	return out.Bytes()
}

func buildSecurityPDF(t testing.TB, bodies []string) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(bodies)+1)
	for i, body := range bodies {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(offsets))
	out.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}
