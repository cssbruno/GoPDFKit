// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package font_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/font"
	"github.com/cssbruno/gopdfkit/testsupport/example"
)

func ExampleParseTTF() {
	ttf, err := font.ParseTTF(example.FontDir() + "/calligra.ttf")
	if err == nil {
		fmt.Printf("Postscript name:  %s\n", ttf.PostScriptName)
		fmt.Printf("unitsPerEm:       %8d\n", ttf.UnitsPerEm)
		fmt.Printf("Xmin:             %8d\n", ttf.Xmin)
		fmt.Printf("Ymin:             %8d\n", ttf.Ymin)
		fmt.Printf("Xmax:             %8d\n", ttf.Xmax)
		fmt.Printf("Ymax:             %8d\n", ttf.Ymax)
	} else {
		fmt.Printf("%s\n", err)
	}
	// Output:
	// Postscript name:  CalligrapherRegular
	// unitsPerEm:           1000
	// Xmin:                 -173
	// Ymin:                 -234
	// Xmax:                 1328
	// Ymax:                  899
}

func TestParseTTFTruncatedFontReturnsReadError(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "truncated.ttf")
	if err := os.WriteFile(fontPath, []byte("\x00\x01\x00\x00"), 0644); err != nil {
		t.Fatalf("write truncated font: %s", err)
	}

	_, err := font.ParseTTF(fontPath)
	if err == nil {
		t.Fatal("expected truncated font error")
	}
	if !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("ParseTTF error = %q, want EOF", err.Error())
	}
}

func TestParseOpenTypeCFFFont(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "minimal.otf")
	cffData := []byte{1, 0, 4, 4, 0, 1, 1, 1}
	if err := os.WriteFile(fontPath, minimalOpenTypeCFF(cffData), 0644); err != nil {
		t.Fatalf("write OpenType fixture: %s", err)
	}

	otf, err := font.ParseOpenType(fontPath)
	if err != nil {
		t.Fatalf("ParseOpenType: %s", err)
	}
	if !otf.PostScriptOutlines {
		t.Fatal("PostScriptOutlines = false, want true")
	}
	if !bytes.Equal(otf.CFFData, cffData) {
		t.Fatalf("CFFData = %x, want %x", otf.CFFData, cffData)
	}
	if otf.PostScriptName != "MinimalOTFCFF" {
		t.Fatalf("PostScriptName = %q, want MinimalOTFCFF", otf.PostScriptName)
	}
	if otf.Chars['A'] != 1 {
		t.Fatalf("glyph for A = %d, want 1", otf.Chars['A'])
	}

	_, err = font.ParseTTF(fontPath)
	if err == nil || !strings.Contains(err.Error(), "not a TrueType font") {
		t.Fatalf("ParseTTF error = %v, want not a TrueType font", err)
	}
}

func TestMakeOpenTypeCFF(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "minimal.otf")
	cffData := []byte{1, 0, 4, 4, 0, 1, 1, 1}
	if err := os.WriteFile(fontPath, minimalOpenTypeCFF(cffData), 0644); err != nil {
		t.Fatalf("write OpenType fixture: %s", err)
	}

	err := font.Make(fontPath, example.FontFile("cp1252.map"), dir, io.Discard, true)
	if err != nil {
		t.Fatalf("Make: %s", err)
	}

	def, err := os.ReadFile(filepath.Join(dir, "minimal.json"))
	if err != nil {
		t.Fatalf("read generated definition: %s", err)
	}
	for _, want := range []string{`"Tp":"OpenTypeCFF"`, `"Name":"MinimalOTFCFF"`, `"File":"minimal.z"`} {
		if !bytes.Contains(def, []byte(want)) {
			t.Fatalf("definition missing %s: %s", want, def)
		}
	}

	zFile, err := os.Open(filepath.Join(dir, "minimal.z"))
	if err != nil {
		t.Fatalf("open compressed CFF: %s", err)
	}
	defer func() { _ = zFile.Close() }()
	zr, err := zlib.NewReader(zFile)
	if err != nil {
		t.Fatalf("open zlib CFF: %s", err)
	}
	embedded, err := io.ReadAll(zr)
	if closeErr := zr.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read zlib CFF: %s", err)
	}
	if !bytes.Equal(embedded, cffData) {
		t.Fatalf("embedded CFF = %x, want %x", embedded, cffData)
	}

	zBytes, err := os.ReadFile(filepath.Join(dir, "minimal.z"))
	if err != nil {
		t.Fatalf("read generated compressed font: %s", err)
	}
	pdf := document.New("P", "mm", "A4", "")
	pdf.AddFontFromBytes("Minimal", "", def, zBytes)
	pdf.SetFont("Minimal", "", 12)
	pdf.AddPage()
	pdf.Text(10, 10, "A")
	var out bytes.Buffer
	err = pdf.Output(&out)
	if err != nil {
		t.Fatalf("output PDF with OpenType/CFF font: %s", err)
	}
	for _, want := range []string{"/Subtype /Type1C", "/FontFile3"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("PDF missing %s", want)
		}
	}
}

func minimalOpenTypeCFF(cffData []byte) []byte {
	tables := []struct {
		tag  string
		data []byte
	}{
		{"head", minimalHeadTable()},
		{"hhea", minimalHheaTable()},
		{"maxp", minimalMaxpTable()},
		{"hmtx", minimalHmtxTable()},
		{"cmap", minimalCmapTable()},
		{"name", minimalNameTable("MinimalOTFCFF")},
		{"OS/2", minimalOS2Table()},
		{"post", minimalPostTable()},
		{"CFF ", cffData},
	}
	var out bytes.Buffer
	out.WriteString("OTTO")
	writeU16(&out, len(tables))
	writeU16(&out, 0)
	writeU16(&out, 0)
	writeU16(&out, 0)
	offset := 12 + len(tables)*16
	for _, table := range tables {
		out.WriteString(table.tag)
		writeU32(&out, 0)
		writeU32(&out, offset)
		writeU32(&out, len(table.data))
		offset += paddedLen(len(table.data))
	}
	for _, table := range tables {
		out.Write(table.data)
		for range paddedLen(len(table.data)) - len(table.data) {
			out.WriteByte(0)
		}
	}
	return out.Bytes()
}

func minimalHeadTable() []byte {
	var b bytes.Buffer
	writeU32(&b, 0x00010000)
	writeU32(&b, 0)
	writeU32(&b, 0)
	writeU32(&b, 0x5F0F3CF5)
	writeU16(&b, 0)
	writeU16(&b, 1000)
	b.Write(make([]byte, 16))
	writeI16(&b, -10)
	writeI16(&b, -200)
	writeI16(&b, 900)
	writeI16(&b, 800)
	b.Write(make([]byte, 10))
	return b.Bytes()
}

func minimalHheaTable() []byte {
	var b bytes.Buffer
	writeU32(&b, 0x00010000)
	b.Write(make([]byte, 30))
	writeU16(&b, 2)
	return b.Bytes()
}

func minimalMaxpTable() []byte {
	var b bytes.Buffer
	writeU32(&b, 0x00010000)
	writeU16(&b, 2)
	return b.Bytes()
}

func minimalHmtxTable() []byte {
	var b bytes.Buffer
	writeU16(&b, 500)
	writeI16(&b, 0)
	writeU16(&b, 600)
	writeI16(&b, 0)
	return b.Bytes()
}

func minimalCmapTable() []byte {
	var b bytes.Buffer
	writeU16(&b, 0)
	writeU16(&b, 1)
	writeU16(&b, 3)
	writeU16(&b, 1)
	writeU32(&b, 12)
	writeU16(&b, 4)
	writeU16(&b, 32)
	writeU16(&b, 0)
	writeU16(&b, 4)
	writeU16(&b, 4)
	writeU16(&b, 1)
	writeU16(&b, 0)
	writeU16(&b, 65)
	writeU16(&b, 0xFFFF)
	writeU16(&b, 0)
	writeU16(&b, 65)
	writeU16(&b, 0xFFFF)
	writeI16(&b, -64)
	writeI16(&b, 1)
	writeU16(&b, 0)
	writeU16(&b, 0)
	return b.Bytes()
}

func minimalNameTable(name string) []byte {
	utf16Name := make([]byte, 0, len(name)*2)
	for _, r := range name {
		utf16Name = append(utf16Name, 0, byte(r))
	}
	var b bytes.Buffer
	writeU16(&b, 0)
	writeU16(&b, 1)
	writeU16(&b, 18)
	writeU16(&b, 3)
	writeU16(&b, 1)
	writeU16(&b, 0x0409)
	writeU16(&b, 6)
	writeU16(&b, len(utf16Name))
	writeU16(&b, 0)
	b.Write(utf16Name)
	return b.Bytes()
}

func minimalOS2Table() []byte {
	var b bytes.Buffer
	writeU16(&b, 2)
	writeI16(&b, 500)
	writeU16(&b, 400)
	writeU16(&b, 5)
	writeU16(&b, 0)
	b.Write(make([]byte, 54))
	writeU16(&b, 0)
	writeU16(&b, 0)
	writeU16(&b, 255)
	writeI16(&b, 700)
	writeI16(&b, -200)
	b.Write(make([]byte, 16))
	writeI16(&b, 700)
	return b.Bytes()
}

func minimalPostTable() []byte {
	var b bytes.Buffer
	writeU32(&b, 0x00030000)
	writeI16(&b, 0)
	writeU16(&b, 0)
	writeI16(&b, -100)
	writeI16(&b, 50)
	writeU32(&b, 0)
	return b.Bytes()
}

func paddedLen(n int) int {
	return (n + 3) &^ 3
}

func writeU16(w io.Writer, v int) {
	_ = binary.Write(w, binary.BigEndian, uint16(v))
}

func writeI16(w io.Writer, v int) {
	_ = binary.Write(w, binary.BigEndian, int16(v))
}

func writeU32(w io.Writer, v int) {
	_ = binary.Write(w, binary.BigEndian, uint32(v))
}

func hexStr(s string) string {
	var b bytes.Buffer
	b.WriteString("\"")
	for _, ch := range []byte(s) {
		_, _ = fmt.Fprintf(&b, "\\x%02x", ch)
	}
	b.WriteString("\":")
	return b.String()
}

func ExampleDocument_GetStringWidth() {
	pdf := document.New("", "", "", example.FontDir())
	pdf.SetFont("Helvetica", "", 12)
	pdf.AddPage()
	for _, s := range []string{"Hello", "世界", "\xe7a va?"} {
		fmt.Printf("%-32s width %5.2f, bytes %2d, runes %2d\n",
			hexStr(s), pdf.GetStringWidth(s), len(s), len([]rune(s)))
		if pdf.Err() {
			fmt.Println(pdf.Error())
		}
	}
	pdf.Close()
	// Output:
	// "\x48\x65\x6c\x6c\x6f":          width  9.64, bytes  5, runes  5
	// "\xe4\xb8\x96\xe7\x95\x8c":      width 13.95, bytes  6, runes  2
	// "\xe7\x61\x20\x76\x61\x3f":      width 12.47, bytes  6, runes  6
}
