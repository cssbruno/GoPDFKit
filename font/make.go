// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package font

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxFontSourceBytes = 64 * 1024 * 1024

type fontBoxType struct {
	Xmin, Ymin, Xmax, Ymax int
}

type fontDescriptor struct {
	Ascent       int
	Descent      int
	CapHeight    int
	Flags        int
	FontBBox     fontBoxType
	ItalicAngle  int
	StemV        int
	MissingWidth int
}

type fontDefinition struct {
	Tp           string
	Name         string
	Desc         fontDescriptor
	Up           int
	Ut           int
	Cw           []int
	Enc          string
	Diff         string
	File         string
	Size1, Size2 int
	OriginalSize int
}

type fontInfoType struct {
	Data               []byte
	File               string
	OriginalSize       int
	FontName           string
	Bold               bool
	IsFixedPitch       bool
	UnderlineThickness int
	UnderlinePosition  int
	Widths             []int
	Size1, Size2       uint32
	Desc               fontDescriptor
}

type encType struct {
	uv   int
	name string
}

type encListType [256]encType

type fmtBuffer struct {
	bytes.Buffer
}

func (b *fmtBuffer) printf(fmtStr string, args ...any) {
	fmt.Fprintf(&b.Buffer, fmtStr, args...)
}

func round(f float64) int {
	if f < 0 {
		return -int(math.Floor(-f + 0.5))
	}
	return int(math.Floor(f + 0.5))
}

func fileExist(filename string) bool {
	info, err := os.Stat(filename)
	return err == nil && info != nil && info.Mode().IsRegular()
}

func fileSize(filename string) (int64, bool) {
	info, err := os.Stat(filename)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		return 0, false
	}
	return info.Size(), true
}

func baseNoExt(fileStr string) string {
	str := filepath.Base(fileStr)
	extLen := len(filepath.Ext(str))
	if extLen > 0 {
		str = str[:len(str)-extLen]
	}
	return str
}

func loadMap(encodingFileStr string) (encList encListType, err error) {
	var f *os.File
	f, err = os.Open(encodingFileStr) // #nosec G304 -- Font generation accepts explicit caller-supplied paths.
	if err == nil {
		defer func() { _ = f.Close() }()
		for j := range encList {
			encList[j].uv = -1
			encList[j].name = ".notdef"
		}
		scanner := bufio.NewScanner(f)
		var enc encType
		var pos int
		for scanner.Scan() {
			// "!3F U+003F question"
			_, err = fmt.Sscanf(scanner.Text(), "!%x U+%x %s", &pos, &enc.uv, &enc.name)
			if err == nil {
				if pos < 256 {
					encList[pos] = enc
				} else {
					err = fmt.Errorf("map position 0x%2X exceeds 0xFF", pos)
					return
				}
			} else {
				return
			}
		}
		if err = scanner.Err(); err != nil {
			return
		}
	}
	return
}

// getInfoFromTrueType returns information from a TrueType font.
func getInfoFromTrueType(fileStr string, msgWriter io.Writer, embed bool, encList encListType) (info fontInfoType, err error) {
	var ttf TrueType
	ttf, err = ParseTTF(fileStr)
	if err != nil {
		return
	}
	if embed {
		if !ttf.Embeddable {
			err = errors.New("font license does not allow embedding")
			return
		}
		info.Data, err = os.ReadFile(fileStr) // #nosec G304 -- Font generation accepts an explicit source path.
		if err != nil {
			return
		}
		info.OriginalSize = len(info.Data)
	}
	fillInfoFromTrueTypeMetrics(&info, ttf, msgWriter, encList)
	return
}

// getInfoFromOpenTypeCFF returns information from an OpenType font with CFF
// PostScript outlines.
func getInfoFromOpenTypeCFF(fileStr string, msgWriter io.Writer, embed bool, encList encListType) (info fontInfoType, err error) {
	var otf OpenType
	otf, err = ParseOpenType(fileStr)
	if err != nil {
		return
	}
	if !otf.PostScriptOutlines {
		err = errors.New("OpenType/CFF font type requires PostScript outlines")
		return
	}
	if embed {
		if !otf.Embeddable {
			err = errors.New("font license does not allow embedding")
			return
		}
		if len(otf.CFFData) == 0 {
			err = errors.New("OpenType/CFF font is missing CFF table")
			return
		}
		info.Data = otf.CFFData
		info.OriginalSize = len(info.Data)
	}
	fillInfoFromOpenTypeMetrics(&info, otf, msgWriter, encList)
	return
}

func fillInfoFromTrueTypeMetrics(info *fontInfoType, ttf TrueType, msgWriter io.Writer, encList encListType) {
	otf := OpenType{
		Embeddable:         ttf.Embeddable,
		UnitsPerEm:         ttf.UnitsPerEm,
		PostScriptName:     ttf.PostScriptName,
		Bold:               ttf.Bold,
		ItalicAngle:        ttf.ItalicAngle,
		IsFixedPitch:       ttf.IsFixedPitch,
		TypoAscender:       ttf.TypoAscender,
		TypoDescender:      ttf.TypoDescender,
		UnderlinePosition:  ttf.UnderlinePosition,
		UnderlineThickness: ttf.UnderlineThickness,
		Xmin:               ttf.Xmin,
		Ymin:               ttf.Ymin,
		Xmax:               ttf.Xmax,
		Ymax:               ttf.Ymax,
		CapHeight:          ttf.CapHeight,
		Widths:             ttf.Widths,
		Chars:              ttf.Chars,
	}
	fillInfoFromOpenTypeMetrics(info, otf, msgWriter, encList)
}

func fillInfoFromOpenTypeMetrics(info *fontInfoType, otf OpenType, msgWriter io.Writer, encList encListType) {
	info.Widths = make([]int, 256)
	k := 1000.0 / float64(otf.UnitsPerEm)
	info.FontName = otf.PostScriptName
	info.Bold = otf.Bold
	info.Desc.ItalicAngle = int(otf.ItalicAngle)
	info.IsFixedPitch = otf.IsFixedPitch
	info.Desc.Ascent = round(k * float64(otf.TypoAscender))
	info.Desc.Descent = round(k * float64(otf.TypoDescender))
	info.UnderlineThickness = round(k * float64(otf.UnderlineThickness))
	info.UnderlinePosition = round(k * float64(otf.UnderlinePosition))
	info.Desc.FontBBox = fontBoxType{
		round(k * float64(otf.Xmin)),
		round(k * float64(otf.Ymin)),
		round(k * float64(otf.Xmax)),
		round(k * float64(otf.Ymax)),
	}
	info.Desc.CapHeight = round(k * float64(otf.CapHeight))
	if len(otf.Widths) > 0 {
		info.Desc.MissingWidth = round(k * float64(otf.Widths[0]))
	}
	var wd int
	for j := 0; j < len(info.Widths); j++ {
		wd = info.Desc.MissingWidth
		if encList[j].name != ".notdef" {
			uv := encList[j].uv
			if uv < 0 || uv > 0xffff {
				_, _ = fmt.Fprintf(msgWriter, "Character %s has unsupported Unicode value U+%X\n", encList[j].name, uv)
				info.Widths[j] = wd
				continue
			}
			pos, ok := otf.Chars[uint16(uv)]
			if ok && int(pos) < len(otf.Widths) {
				wd = round(k * float64(otf.Widths[pos]))
			} else {
				_, _ = fmt.Fprintf(msgWriter, "Character %s is missing\n", encList[j].name)
			}
		}
		info.Widths[j] = wd
	}
}

type segmentType struct {
	marker uint8
	tp     uint8
	size   uint32
	data   []byte
}

func segmentRead(r io.Reader) (s segmentType, err error) {
	if err = binary.Read(r, binary.LittleEndian, &s.marker); err != nil {
		return
	}
	if s.marker != 128 {
		err = errors.New("font file is not a valid binary Type1")
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &s.tp); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &s.size); err != nil {
		return
	}
	if s.size > maxFontSourceBytes {
		err = errors.New("Type1 font segment exceeds maximum size")
		return
	}
	s.data = make([]byte, s.size)
	_, err = io.ReadFull(r, s.data)
	return
}

// -rw-r--r-- 1 root 9532 2010-04-22 11:27 /usr/share/fonts/type1/mathml/Symbol.afm
// -rw-r--r-- 1 root 37744 2010-04-22 11:27 /usr/share/fonts/type1/mathml/Symbol.pfb

// getInfoFromType1 returns information from a Type1 font.
func getInfoFromType1(fileStr string, msgWriter io.Writer, embed bool, encList encListType) (info fontInfoType, err error) {
	info.Widths = make([]int, 256)
	if embed {
		var f *os.File
		f, err = os.Open(fileStr) // #nosec G304 -- Font generation accepts an explicit source path.
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		// Read first segment
		var s1, s2 segmentType
		s1, err = segmentRead(f)
		if err != nil {
			return
		}
		s2, err = segmentRead(f)
		if err != nil {
			return
		}
		info.Data = s1.data
		info.Data = append(info.Data, s2.data...)
		info.Size1 = s1.size
		info.Size2 = s2.size
	}
	afmFileStr := fileStr[0:len(fileStr)-3] + "afm"
	size, ok := fileSize(afmFileStr)
	if !ok {
		err = fmt.Errorf("font file (ATM) %s not found", afmFileStr)
		return
	} else if size == 0 {
		err = fmt.Errorf("font file (AFM) %s empty or not readable", afmFileStr)
		return
	}
	var f *os.File
	f, err = os.Open(afmFileStr) // #nosec G304 -- Font generation accepts an explicit AFM path.
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	var fields []string
	var wd int
	var wt, name string
	wdMap := make(map[string]int)
	for scanner.Scan() {
		fields = strings.Fields(strings.TrimSpace(scanner.Text()))
		// Comment Generated by FontForge 20080203
		// FontName Symbol
		// C 32 ; WX 250 ; N space ; B 0 0 0 0 ;
		if len(fields) >= 2 {
			switch fields[0] {
			case "C":
				if len(fields) < 8 {
					err = errors.New("malformed AFM character record")
					break
				}
				if wd, err = strconv.Atoi(fields[4]); err == nil {
					name = fields[7]
					wdMap[name] = wd
				}
			case "FontName":
				info.FontName = fields[1]
			case "Weight":
				wt = strings.ToLower(fields[1])
			case "ItalicAngle":
				info.Desc.ItalicAngle, err = strconv.Atoi(fields[1])
			case "Ascender":
				info.Desc.Ascent, err = strconv.Atoi(fields[1])
			case "Descender":
				info.Desc.Descent, err = strconv.Atoi(fields[1])
			case "UnderlineThickness":
				info.UnderlineThickness, err = strconv.Atoi(fields[1])
			case "UnderlinePosition":
				info.UnderlinePosition, err = strconv.Atoi(fields[1])
			case "IsFixedPitch":
				info.IsFixedPitch = fields[1] == "true"
			case "FontBBox":
				if len(fields) < 5 {
					err = errors.New("malformed AFM FontBBox record")
					break
				}
				if info.Desc.FontBBox.Xmin, err = strconv.Atoi(fields[1]); err == nil {
					if info.Desc.FontBBox.Ymin, err = strconv.Atoi(fields[2]); err == nil {
						if info.Desc.FontBBox.Xmax, err = strconv.Atoi(fields[3]); err == nil {
							info.Desc.FontBBox.Ymax, err = strconv.Atoi(fields[4])
						}
					}
				}
			case "CapHeight":
				info.Desc.CapHeight, err = strconv.Atoi(fields[1])
			case "StdVW":
				info.Desc.StemV, err = strconv.Atoi(fields[1])
			}
		}
		if err != nil {
			return
		}
	}
	if err = scanner.Err(); err != nil {
		return
	}
	if info.FontName == "" {
		err = fmt.Errorf("the field FontName missing in AFM file %s", afmFileStr)
		return
	}
	info.Bold = wt == "bold" || wt == "black"
	var missingWd int
	missingWd, ok = wdMap[".notdef"]
	if ok {
		info.Desc.MissingWidth = missingWd
	}
	for j := 0; j < len(info.Widths); j++ {
		info.Widths[j] = info.Desc.MissingWidth
	}
	for j := 0; j < len(info.Widths); j++ {
		name = encList[j].name
		if name != ".notdef" {
			wd, ok = wdMap[name]
			if ok {
				info.Widths[j] = wd
			} else {
				_, _ = fmt.Fprintf(msgWriter, "Character %s is missing\n", name)
			}
		}
	}
	return
}

func makeFontDescriptor(info *fontInfoType) {
	if info.Desc.CapHeight == 0 {
		info.Desc.CapHeight = info.Desc.Ascent
	}
	info.Desc.Flags = 1 << 5
	if info.IsFixedPitch {
		info.Desc.Flags |= 1
	}
	if info.Desc.ItalicAngle != 0 {
		info.Desc.Flags |= 1 << 6
	}
	if info.Desc.StemV == 0 {
		if info.Bold {
			info.Desc.StemV = 120
		} else {
			info.Desc.StemV = 70
		}
	}
}

// makeFontEncoding builds differences from the reference encoding.
func makeFontEncoding(encList encListType, refEncFileStr string) (diffStr string, err error) {
	var refList encListType
	if refList, err = loadMap(refEncFileStr); err != nil {
		return
	}
	var buf fmtBuffer
	last := 0
	for j := 32; j < 256; j++ {
		if encList[j].name != refList[j].name {
			if j != last+1 {
				buf.printf("%d ", j)
			}
			last = j
			buf.printf("/%s ", encList[j].name)
		}
	}
	diffStr = strings.TrimSpace(buf.String())
	return
}

func makeDefinitionFile(fileStr, tpStr, encodingFileStr string, embed bool, encList encListType, info fontInfoType) error {
	var err error
	var def fontDefinition
	def.Tp = tpStr
	def.Name = info.FontName
	makeFontDescriptor(&info)
	def.Desc = info.Desc
	def.Up = info.UnderlinePosition
	def.Ut = info.UnderlineThickness
	def.Cw = info.Widths
	def.Enc = baseNoExt(encodingFileStr)
	def.Diff, err = makeFontEncoding(encList, filepath.Join(filepath.Dir(encodingFileStr), "cp1252.map"))
	if err != nil {
		return err
	}
	def.File = info.File
	def.Size1 = int(info.Size1)
	def.Size2 = int(info.Size2)
	def.OriginalSize = info.OriginalSize
	var buf []byte
	buf, err = json.Marshal(def)
	if err != nil {
		return err
	}
	var f *os.File
	f, err = os.Create(fileStr) // #nosec G304 -- Destination is explicitly selected by the font-generation caller.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(buf)
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	return err
}

// Make generates a font definition file in JSON format. A definition file
// of this type is required to use non-core fonts in the PDF documents that
// gopdfkit generates. See the fontmaker command in the gopdfkit package for a
// command-line interface to this function.
//
// fontFileStr is the name of the TrueType file (extension .ttf), OpenType file
// (extension .otf) or binary Type1 file (extension .pfb) from which to
// generate a definition file. OpenType files with TrueType outlines and
// CFF/PostScript outlines are both supported; the outline type cannot be
// determined from the file extension alone. If a Type1 file is specified, a
// metric file with the same pathname except with the extension .afm must be
// present.
//
// encodingFileStr is the name of the encoding file that corresponds to the
// font.
//
// dstDirStr is the name of the directory in which to save the definition file
// and, if embed is true, the compressed font file.
//
// msgWriter is the writer that receives messages throughout the
// process. Use nil to turn off messages.
//
// embed is true if the font is to be embedded in the PDF files.
func Make(fontFileStr, encodingFileStr, dstDirStr string, msgWriter io.Writer, embed bool) error {
	if msgWriter == nil {
		msgWriter = io.Discard
	}
	if !fileExist(fontFileStr) {
		return fmt.Errorf("font file not found: %s", fontFileStr)
	}
	extStr := strings.TrimPrefix(strings.ToLower(filepath.Ext(fontFileStr)), ".")
	var tpStr string
	switch extStr {
	case "ttf":
		tpStr = "TrueType"
	case "otf":
		var err error
		tpStr, err = openTypeFontType(fontFileStr)
		if err != nil {
			return err
		}
	case "pfb":
		tpStr = "Type1"
	default:
		return fmt.Errorf("unrecognized font file extension: %s", extStr)
	}

	var info fontInfoType
	encList, err := loadMap(encodingFileStr)
	if err != nil {
		return err
	}
	switch tpStr {
	case "TrueType":
		info, err = getInfoFromTrueType(fontFileStr, msgWriter, embed, encList)
		if err != nil {
			return err
		}
	case "OpenTypeCFF":
		info, err = getInfoFromOpenTypeCFF(fontFileStr, msgWriter, embed, encList)
		if err != nil {
			return err
		}
	default:
		info, err = getInfoFromType1(fontFileStr, msgWriter, embed, encList)
		if err != nil {
			return err
		}
	}
	baseStr := baseNoExt(fontFileStr)
	if embed {
		var f *os.File
		info.File = baseStr + ".z"
		zFileStr := filepath.Join(dstDirStr, info.File)
		f, err = os.Create(zFileStr) // #nosec G304 -- Destination is explicitly selected by the font-generation caller.
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		cmp := zlib.NewWriter(f)
		_, err = cmp.Write(info.Data)
		if err != nil {
			return err
		}
		err = cmp.Close()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(msgWriter, "Font file compressed: %s\n", zFileStr)
	}
	defFileStr := filepath.Join(dstDirStr, baseStr+".json")
	err = makeDefinitionFile(defFileStr, tpStr, encodingFileStr, embed, encList, info)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(msgWriter, "Font definition file successfully generated: %s\n", defFileStr)
	return nil
}

func openTypeFontType(fileStr string) (string, error) {
	var header [4]byte
	f, err := os.Open(fileStr) // #nosec G304 -- Font generation accepts an explicit source path.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	_, err = io.ReadFull(f, header[:])
	if err != nil {
		return "", err
	}
	switch string(header[:]) {
	case "OTTO":
		return "OpenTypeCFF", nil
	case "\x00\x01\x00\x00", "true":
		return "TrueType", nil
	default:
		return "", errors.New("unrecognized OpenType file format")
	}
}
