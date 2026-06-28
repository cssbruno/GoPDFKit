// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"compress/zlib"
	"errors"
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
