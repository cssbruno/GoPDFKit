// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package font

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSegmentReadRejectsShortSegment(t *testing.T) {
	var input bytes.Buffer
	input.WriteByte(128)
	input.WriteByte(1)
	if err := binary.Write(&input, binary.LittleEndian, uint32(4)); err != nil {
		t.Fatalf("Write size: %v", err)
	}
	input.WriteByte(0)

	_, err := segmentRead(&input)
	if err == nil {
		t.Fatal("segmentRead() accepted short segment")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("segmentRead() error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestSegmentReadRejectsHugeSegment(t *testing.T) {
	var input bytes.Buffer
	input.WriteByte(128)
	input.WriteByte(1)
	if err := binary.Write(&input, binary.LittleEndian, uint32(maxFontSourceBytes+1)); err != nil {
		t.Fatalf("Write size: %v", err)
	}
	if _, err := segmentRead(&input); err == nil {
		t.Fatal("segmentRead accepted oversized Type1 segment")
	}
}

func TestGetInfoFromType1RejectsMalformedAFMCharacterRecord(t *testing.T) {
	dir := t.TempDir()
	pfbPath := filepath.Join(dir, "bad.pfb")
	afmPath := filepath.Join(dir, "bad.afm")
	if err := os.WriteFile(pfbPath, []byte{}, 0o600); err != nil {
		t.Fatalf("write pfb: %v", err)
	}
	if err := os.WriteFile(afmPath, []byte("FontName Bad\nC 32 ;\n"), 0o600); err != nil {
		t.Fatalf("write afm: %v", err)
	}

	_, err := getInfoFromType1(pfbPath, io.Discard, false, encListType{})
	if err == nil {
		t.Fatal("getInfoFromType1() accepted malformed AFM character record")
	}
	if !strings.Contains(err.Error(), "malformed AFM character record") {
		t.Fatalf("getInfoFromType1() error = %v, want malformed character record", err)
	}
}

func TestOpenTypeMetricsRejectsUnicodeValuesOutsideBMP(t *testing.T) {
	t.Parallel()

	encodings := encListType{}
	for i := range encodings {
		encodings[i] = encType{uv: -1, name: ".notdef"}
	}
	encodings[0] = encType{uv: 0x10000, name: "outside-bmp"}
	font := OpenType{
		UnitsPerEm: 1000,
		Widths:     []uint16{500, 900},
		Chars:      map[uint16]uint16{0: 1},
	}
	var info fontInfoType
	var messages bytes.Buffer
	fillInfoFromOpenTypeMetrics(&info, font, &messages, encodings)
	if got := info.Widths[0]; got != 500 {
		t.Fatalf("width for unsupported Unicode value = %d, want missing width 500", got)
	}
	if !strings.Contains(messages.String(), "unsupported Unicode value") {
		t.Fatalf("messages = %q, want unsupported Unicode diagnostic", messages.String())
	}
}
