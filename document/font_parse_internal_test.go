// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticTablesFromParsedFontMatchesStandaloneParser(t *testing.T) {
	data := readUTF8FontFixture(t)
	parsed, err := parseUTF8Font(data)
	if err != nil {
		t.Fatalf("parseUTF8Font() error = %v", err)
	}
	parsedHead := parsed.tableDescriptions["head"]
	if parsedHead == nil {
		t.Fatal("parseUTF8Font() did not parse the head table")
	}

	reused, err := parsed.staticTablesFromParsedFont()
	if err != nil {
		t.Fatalf("staticTablesFromParsedFont() error = %v", err)
	}
	if reused.tableDescriptions["head"] != parsedHead {
		t.Fatal("staticTablesFromParsedFont() reparsed the SFNT directory")
	}

	freshReader := fileReader{array: data}
	freshFont := newUTF8Font(&freshReader)
	freshFont.sourceID = sha256.Sum256(data)
	fresh, err := freshFont.parseStaticTables()
	if err != nil {
		t.Fatalf("parseStaticTables() error = %v", err)
	}
	if reused.oldMetrics != fresh.oldMetrics ||
		reused.numSymbols != fresh.numSymbols ||
		reused.locaFormat != fresh.locaFormat ||
		len(reused.symbolPosition) != len(fresh.symbolPosition) {
		t.Fatalf("reused static tables = metrics:%d symbols:%d loca:%d offsets:%d; fresh = metrics:%d symbols:%d loca:%d offsets:%d",
			reused.oldMetrics, reused.numSymbols, reused.locaFormat, len(reused.symbolPosition),
			fresh.oldMetrics, fresh.numSymbols, fresh.locaFormat, len(fresh.symbolPosition))
	}
	for _, r := range []int{'A', '世', '�'} {
		if reused.charSymbolDictionary[r] != fresh.charSymbolDictionary[r] {
			t.Fatalf("glyph for %U = %d, want %d", r, reused.charSymbolDictionary[r], fresh.charSymbolDictionary[r])
		}
	}
}

func TestNewCachedUTF8FontAttachesReusableStaticTables(t *testing.T) {
	data := readUTF8FontFixture(t)
	cached, err := newCachedUTF8Font("dejavu", "fixture.ttf", data)
	if err != nil {
		t.Fatalf("newCachedUTF8Font() error = %v", err)
	}
	if cached.static == nil {
		t.Fatal("newCachedUTF8Font() static tables = nil")
	}
	if cached.def.utf8File != nil {
		t.Fatal("newCachedUTF8Font() retained a mutable parser in the cached definition")
	}

	font := cached.newUTF8Font()
	if font == nil {
		t.Fatal("cached.newUTF8Font() = nil")
	}
	if font.static != cached.static {
		t.Fatal("cached.newUTF8Font() did not attach the shared static tables")
	}
	if font.sourceID != sha256.Sum256(data) {
		t.Fatal("cached.newUTF8Font() source ID does not match the font bytes")
	}
}

func readUTF8FontFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "assets", "static", "font", "DejaVuSansCondensed.ttf"))
	if err != nil {
		t.Fatalf("ReadFile(font fixture) error = %v", err)
	}
	return data
}
