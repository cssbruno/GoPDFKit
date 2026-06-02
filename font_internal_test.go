/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"bytes"
	"encoding/binary"
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
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("segmentRead() error = %v, want io.ErrUnexpectedEOF", err)
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
