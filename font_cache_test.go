/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/internal/example"
)

func TestFontCacheMatchesUTF8FontFromBytes(t *testing.T) {
	fontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed.ttf"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	cache := gopdfkit.NewFontCache()
	if err := cache.AddUTF8FontFromBytes("DejaVu", "", fontBytes); err != nil {
		t.Fatalf("AddUTF8FontFromBytes() error = %v", err)
	}

	build := func(addFont func(*gopdfkit.Fpdf)) []byte {
		pdf := gopdfkit.New("P", "mm", "A4", "")
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

	uncached := build(func(pdf *gopdfkit.Fpdf) {
		pdf.AddUTF8FontFromBytes("DejaVu", "", fontBytes)
	})
	cached := build(func(pdf *gopdfkit.Fpdf) {
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
	})
	if !bytes.Equal(uncached, cached) {
		t.Fatal("cached UTF-8 font output differs from AddUTF8FontFromBytes output")
	}
}
