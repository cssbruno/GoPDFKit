// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/testsupport/example"
)

func TestFontCacheMatchesUTF8FontFromBytes(t *testing.T) {
	fontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed.ttf"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	cache := document.NewFontCache()
	if err := cache.AddUTF8FontFromBytes("DejaVu", "", fontBytes); err != nil {
		t.Fatalf("AddUTF8FontFromBytes() error = %v", err)
	}

	build := func(addFont func(*document.Document)) []byte {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.SetCatalogSort(true)
		pdf.SetCreationDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		pdf.SetModificationDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		addFont(pdf)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.MultiCell(0, 6, "Cached UTF-8: Hello, 世界, مرحبا", "", "L", false)
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		return out.Bytes()
	}

	uncached := build(func(pdf *document.Document) {
		pdf.AddUTF8FontFromBytes("DejaVu", "", fontBytes)
	})
	cached := build(func(pdf *document.Document) {
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
	})
	if !bytes.Equal(uncached, cached) {
		t.Fatal("cached UTF-8 font output differs from AddUTF8FontFromBytes output")
	}
}

func TestAddUTF8FontUsesSharedCacheWithoutChangingOutput(t *testing.T) {
	fontPath := example.FontFile("DejaVuSansCondensed.ttf")
	fontBytes, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	build := func(addFont func(*document.Document)) []byte {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.SetCatalogSort(true)
		pdf.SetCreationDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		pdf.SetModificationDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		addFont(pdf)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.MultiCell(0, 6, "Shared UTF-8 cache: Hello, 世界, مرحبا", "", "L", false)
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		return out.Bytes()
	}

	fromBytes := build(func(pdf *document.Document) {
		pdf.AddUTF8FontFromBytes("DejaVu", "", fontBytes)
	})
	firstPathLoad := build(func(pdf *document.Document) {
		pdf.AddUTF8Font("DejaVu", "", fontPath)
	})
	secondPathLoad := build(func(pdf *document.Document) {
		pdf.AddUTF8Font("DejaVu", "", fontPath)
	})

	if !bytes.Equal(fromBytes, firstPathLoad) {
		t.Fatal("first AddUTF8Font output differs from AddUTF8FontFromBytes output")
	}
	if !bytes.Equal(firstPathLoad, secondPathLoad) {
		t.Fatal("shared cached AddUTF8Font output differs from first AddUTF8Font output")
	}
}
