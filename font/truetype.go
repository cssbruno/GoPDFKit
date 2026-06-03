// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package font

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const maxOpenTypeTableBytes = 256 * 1024 * 1024

// OpenType contains metrics and embedded font data from an OpenType font.
type OpenType struct {
	Embeddable         bool              // Whether the font permits embedding.
	PostScriptOutlines bool              // Whether the font uses CFF/PostScript outlines.
	CFFData            []byte            // Raw CFF table data.
	UnitsPerEm         uint16            // Font units per em.
	PostScriptName     string            // PostScript font name.
	Bold               bool              // Whether the font is bold.
	ItalicAngle        int16             // Italic angle from the font metadata.
	IsFixedPitch       bool              // Whether the font is fixed pitch.
	TypoAscender       int16             // Typographic ascender.
	TypoDescender      int16             // Typographic descender.
	UnderlinePosition  int16             // Underline position.
	UnderlineThickness int16             // Underline thickness.
	Xmin               int16             // Minimum glyph bounding-box X coordinate.
	Ymin               int16             // Minimum glyph bounding-box Y coordinate.
	Xmax               int16             // Maximum glyph bounding-box X coordinate.
	Ymax               int16             // Maximum glyph bounding-box Y coordinate.
	CapHeight          int16             // Capital-letter height.
	Widths             []uint16          // Glyph widths.
	Chars              map[uint16]uint16 // Character-to-glyph map.
}

// TrueType contains metrics of a TrueType font.
type TrueType struct {
	Embeddable         bool              // Whether the font permits embedding.
	UnitsPerEm         uint16            // Font units per em.
	PostScriptName     string            // PostScript font name.
	Bold               bool              // Whether the font is bold.
	ItalicAngle        int16             // Italic angle from the font metadata.
	IsFixedPitch       bool              // Whether the font is fixed pitch.
	TypoAscender       int16             // Typographic ascender.
	TypoDescender      int16             // Typographic descender.
	UnderlinePosition  int16             // Underline position.
	UnderlineThickness int16             // Underline thickness.
	Xmin               int16             // Minimum glyph bounding-box X coordinate.
	Ymin               int16             // Minimum glyph bounding-box Y coordinate.
	Xmax               int16             // Maximum glyph bounding-box X coordinate.
	Ymax               int16             // Maximum glyph bounding-box Y coordinate.
	CapHeight          int16             // Capital-letter height.
	Widths             []uint16          // Glyph widths.
	Chars              map[uint16]uint16 // Character-to-glyph map.
}

type ttfParser struct {
	rec              OpenType
	f                *os.File
	tables           map[string]uint32
	tableLengths     map[string]uint32
	numberOfHMetrics uint16
	numGlyphs        uint16
	err              error
}

// ParseTTF extracts various metrics from a TrueType font file.
func ParseTTF(fileStr string) (TtfRec TrueType, err error) {
	var otf OpenType
	otf, err = ParseOpenType(fileStr)
	if err != nil {
		return
	}
	if otf.PostScriptOutlines {
		err = fmt.Errorf("not a TrueType font: OpenType/CFF uses PostScript outlines")
		return
	}
	TtfRec = TrueType{
		Embeddable:         otf.Embeddable,
		UnitsPerEm:         otf.UnitsPerEm,
		PostScriptName:     otf.PostScriptName,
		Bold:               otf.Bold,
		ItalicAngle:        otf.ItalicAngle,
		IsFixedPitch:       otf.IsFixedPitch,
		TypoAscender:       otf.TypoAscender,
		TypoDescender:      otf.TypoDescender,
		UnderlinePosition:  otf.UnderlinePosition,
		UnderlineThickness: otf.UnderlineThickness,
		Xmin:               otf.Xmin,
		Ymin:               otf.Ymin,
		Xmax:               otf.Xmax,
		Ymax:               otf.Ymax,
		CapHeight:          otf.CapHeight,
		Widths:             otf.Widths,
		Chars:              otf.Chars,
	}
	return
}

// ParseOpenType extracts metrics from an OpenType font file. It supports both
// TrueType outlines and CFF/PostScript outlines.
func ParseOpenType(fileStr string) (OpenTypeRec OpenType, err error) {
	var t ttfParser
	t.f, err = os.Open(fileStr)
	if err != nil {
		return
	}
	defer func() {
		closeErr := t.f.Close()
		if err == nil {
			err = closeErr
		}
	}()
	version, err := t.ReadStr(4)
	if err != nil {
		return
	}
	switch version {
	case "OTTO":
		t.rec.PostScriptOutlines = true
	case "\x00\x01\x00\x00", "true":
	default:
		err = fmt.Errorf("unrecognized file format")
		return
	}
	numTables := int(t.ReadUShort())
	if t.err != nil {
		err = t.err
		return
	}
	t.Skip(3 * 2) // searchRange, entrySelector, rangeShift
	t.tables = make(map[string]uint32)
	t.tableLengths = make(map[string]uint32)
	var tag string
	for range numTables {
		if t.err != nil {
			err = t.err
			return
		}
		tag, err = t.ReadStr(4)
		if err != nil {
			return
		}
		t.Skip(4) // checkSum
		offset := t.ReadULong()
		length := t.ReadULong()
		t.tables[tag] = offset
		t.tableLengths[tag] = length
	}
	if t.err != nil {
		err = t.err
		return
	}
	err = t.ParseComponents()
	if err != nil {
		return
	}
	if t.rec.PostScriptOutlines {
		t.rec.CFFData, err = t.tableData("CFF ")
		if err != nil {
			return
		}
	}
	OpenTypeRec = t.rec
	return
}

func (t *ttfParser) tableData(tag string) ([]byte, error) {
	offset, ok := t.tables[tag]
	if !ok {
		return nil, fmt.Errorf("table not found: %s", tag)
	}
	length := t.tableLengths[tag]
	if length > maxOpenTypeTableBytes {
		return nil, fmt.Errorf("table too large: %s", tag)
	}
	stat, err := t.f.Stat()
	if err != nil {
		return nil, err
	}
	if int64(offset) > stat.Size() || int64(length) > stat.Size()-int64(offset) {
		return nil, fmt.Errorf("invalid table bounds: %s", tag)
	}
	if _, err := t.f.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, err
	}
	data := make([]byte, int(length))
	_, err = io.ReadFull(t.f, data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (t *ttfParser) ParseComponents() (err error) {
	for _, parse := range []func() error{
		t.ParseHead,
		t.ParseHhea,
		t.ParseMaxp,
		t.ParseHmtx,
		t.ParseCmap,
		t.ParseName,
		t.ParseOS2,
		t.ParsePost,
	} {
		if t.err != nil {
			return t.err
		}
		if err = parse(); err != nil {
			return err
		}
		if t.err != nil {
			return t.err
		}
	}
	return
}

func (t *ttfParser) ParseHead() (err error) {
	err = t.Seek("head")
	t.Skip(3 * 4) // version, fontRevision, checkSumAdjustment
	magicNumber := t.ReadULong()
	if magicNumber != 0x5F0F3CF5 {
		err = fmt.Errorf("incorrect magic number")
		return
	}
	t.Skip(2) // flags
	t.rec.UnitsPerEm = t.ReadUShort()
	t.Skip(2 * 8) // created, modified
	t.rec.Xmin = t.ReadShort()
	t.rec.Ymin = t.ReadShort()
	t.rec.Xmax = t.ReadShort()
	t.rec.Ymax = t.ReadShort()
	return
}

func (t *ttfParser) ParseHhea() (err error) {
	err = t.Seek("hhea")
	if err == nil {
		t.Skip(4 + 15*2)
		t.numberOfHMetrics = t.ReadUShort()
	}
	return
}

func (t *ttfParser) ParseMaxp() (err error) {
	err = t.Seek("maxp")
	if err == nil {
		t.Skip(4)
		t.numGlyphs = t.ReadUShort()
	}
	return
}

func (t *ttfParser) ParseHmtx() (err error) {
	err = t.Seek("hmtx")
	if err == nil {
		if t.numberOfHMetrics == 0 && t.numGlyphs > 0 {
			return fmt.Errorf("invalid horizontal metrics count")
		}
		t.rec.Widths = make([]uint16, 0, 8)
		for j := uint16(0); j < t.numberOfHMetrics; j++ {
			t.rec.Widths = append(t.rec.Widths, t.ReadUShort())
			t.Skip(2) // lsb
		}
		if t.numberOfHMetrics < t.numGlyphs {
			lastWidth := t.rec.Widths[t.numberOfHMetrics-1]
			for j := t.numberOfHMetrics; j < t.numGlyphs; j++ {
				t.rec.Widths = append(t.rec.Widths, lastWidth)
			}
		}
	}
	return
}

func (t *ttfParser) ParseCmap() (err error) {
	var offset int64
	if err = t.Seek("cmap"); err != nil {
		return
	}
	t.Skip(2) // version
	numTables := int(t.ReadUShort())
	offset31 := int64(0)
	for range numTables {
		platformID := t.ReadUShort()
		encodingID := t.ReadUShort()
		offset = int64(t.ReadULong())
		if platformID == 3 && encodingID == 1 {
			offset31 = offset
		}
	}
	if offset31 == 0 {
		err = fmt.Errorf("no Unicode encoding found")
		return
	}
	startCount := make([]uint16, 0, 8)
	endCount := make([]uint16, 0, 8)
	idDelta := make([]int16, 0, 8)
	idRangeOffset := make([]uint16, 0, 8)
	t.rec.Chars = make(map[uint16]uint16)
	if _, err = t.f.Seek(int64(t.tables["cmap"])+offset31, io.SeekStart); err != nil {
		return
	}
	format := t.ReadUShort()
	if format != 4 {
		err = fmt.Errorf("unexpected subtable format: %d", format)
		return
	}
	t.Skip(2 * 2) // length, language
	segCount := int(t.ReadUShort() / 2)
	t.Skip(3 * 2) // searchRange, entrySelector, rangeShift
	for range segCount {
		endCount = append(endCount, t.ReadUShort())
	}
	t.Skip(2) // reservedPad
	for range segCount {
		startCount = append(startCount, t.ReadUShort())
	}
	for range segCount {
		idDelta = append(idDelta, t.ReadShort())
	}
	offset, _ = t.f.Seek(int64(0), io.SeekCurrent)
	for range segCount {
		idRangeOffset = append(idRangeOffset, t.ReadUShort())
	}
	for j := range segCount {
		c1 := startCount[j]
		c2 := endCount[j]
		d := idDelta[j]
		ro := idRangeOffset[j]
		if ro > 0 {
			if _, err = t.f.Seek(offset+2*int64(j)+int64(ro), io.SeekStart); err != nil {
				return
			}
		}
		for c := c1; c <= c2; c++ {
			if c == 0xFFFF {
				break
			}
			var gid int32
			if ro > 0 {
				gid = int32(t.ReadUShort())
				if gid > 0 {
					gid += int32(d)
				}
			} else {
				gid = int32(c) + int32(d)
			}
			if gid >= 65536 {
				gid -= 65536
			}
			if gid > 0 && gid <= 0xFFFF && uint16(gid) < t.numGlyphs {
				t.rec.Chars[c] = uint16(gid)
			}
		}
	}
	return
}

func (t *ttfParser) ParseName() (err error) {
	err = t.Seek("name")
	if err == nil {
		tableOffset, _ := t.f.Seek(0, io.SeekCurrent)
		t.rec.PostScriptName = ""
		t.Skip(2) // format
		count := t.ReadUShort()
		stringOffset := t.ReadUShort()
		for j := uint16(0); j < count && t.rec.PostScriptName == ""; j++ {
			t.Skip(3 * 2) // platformID, encodingID, languageID
			nameID := t.ReadUShort()
			length := t.ReadUShort()
			offset := t.ReadUShort()
			if nameID == 6 {
				// PostScript name
				if _, err = t.f.Seek(int64(tableOffset)+int64(stringOffset)+int64(offset), io.SeekStart); err != nil {
					return
				}
				var s string
				s, err = t.ReadStr(int(length))
				if err != nil {
					return
				}
				s = strings.ReplaceAll(s, "\x00", "")
				var re *regexp.Regexp
				if re, err = regexp.Compile(`[(){}<> /%[\]]`); err != nil {
					return
				}
				t.rec.PostScriptName = re.ReplaceAllString(s, "")
			}
		}
		if t.rec.PostScriptName == "" {
			err = fmt.Errorf("the name PostScript was not found")
		}
	}
	return
}

func (t *ttfParser) ParseOS2() (err error) {
	err = t.Seek("OS/2")
	if err == nil {
		version := t.ReadUShort()
		t.Skip(3 * 2) // xAvgCharWidth, usWeightClass, usWidthClass
		fsType := t.ReadUShort()
		t.rec.Embeddable = (fsType != 2) && (fsType&0x200) == 0
		t.Skip(11*2 + 10 + 4*4 + 4)
		fsSelection := t.ReadUShort()
		t.rec.Bold = (fsSelection & 32) != 0
		t.Skip(2 * 2) // usFirstCharIndex, usLastCharIndex
		t.rec.TypoAscender = t.ReadShort()
		t.rec.TypoDescender = t.ReadShort()
		if version >= 2 {
			t.Skip(3*2 + 2*4 + 2)
			t.rec.CapHeight = t.ReadShort()
		} else {
			t.rec.CapHeight = 0
		}
	}
	return
}

func (t *ttfParser) ParsePost() (err error) {
	err = t.Seek("post")
	if err == nil {
		t.Skip(4) // version
		t.rec.ItalicAngle = t.ReadShort()
		t.Skip(2) // Skip decimal part
		t.rec.UnderlinePosition = t.ReadShort()
		t.rec.UnderlineThickness = t.ReadShort()
		t.rec.IsFixedPitch = t.ReadULong() != 0
	}
	return
}

func (t *ttfParser) Seek(tag string) (err error) {
	if t.err != nil {
		return t.err
	}
	ofs, ok := t.tables[tag]
	if ok {
		_, err = t.f.Seek(int64(ofs), io.SeekStart)
		if err != nil && t.err == nil {
			t.err = err
		}
	} else {
		err = fmt.Errorf("table not found: %s", tag)
	}
	return
}

func (t *ttfParser) Skip(n int) {
	if t.err != nil {
		return
	}
	if _, err := t.f.Seek(int64(n), io.SeekCurrent); err != nil && t.err == nil {
		t.err = err
	}
}

func (t *ttfParser) ReadStr(length int) (str string, err error) {
	buf := make([]byte, length)
	_, err = io.ReadFull(t.f, buf)
	if err != nil {
		if t.err == nil {
			t.err = err
		}
		return
	}
	str = string(buf)
	return
}

func (t *ttfParser) ReadUShort() (val uint16) {
	if err := binary.Read(t.f, binary.BigEndian, &val); err != nil && t.err == nil {
		t.err = err
	}
	return
}

func (t *ttfParser) ReadShort() (val int16) {
	if err := binary.Read(t.f, binary.BigEndian, &val); err != nil && t.err == nil {
		t.err = err
	}
	return
}

func (t *ttfParser) ReadULong() (val uint32) {
	if err := binary.Read(t.f, binary.BigEndian, &val); err != nil && t.err == nil {
		t.err = err
	}
	return
}
