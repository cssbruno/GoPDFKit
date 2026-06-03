// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// SetFontLocation sets the filesystem location of the font and font
// definition files.
func (f *Document) SetFontLocation(fontDirStr string) {
	f.fontpath = fontDirStr
}

// SetFontLoader sets a loader used to read font files (.json and .z) from an
// arbitrary source. If a font loader has been specified, it is used to load
// the named font resources when AddFont() is called. If this operation fails,
// an attempt is made to load the resources from the configured font directory
// (see SetFontLocation()).
func (f *Document) SetFontLoader(loader FontLoader) {
	f.fontLoader = loader
}

// AddFont imports a TrueType, OpenType or Type1 font and makes it available.
// It is necessary to generate a font definition file first with the fontmaker
// utility. You do not need to call this function for the core PDF fonts
// (courier, helvetica, times, zapfdingbats).
//
// The JSON definition file (and the font file itself when embedding) must be
// present in the font directory. If it is not found, the error "Could not
// include font definition file" is set.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// fileStr specifies the base name with ".json" extension of the font
// definition file to be added. The file will be loaded from the font directory
// specified in the call to New() or SetFontLocation().
func (f *Document) AddFont(familyStr, styleStr, fileStr string) {
	f.addFont(fontFamilyEscape(familyStr), styleStr, fileStr, false)
}

// AddUTF8Font imports a TrueType font with UTF-8 symbols and makes it available.
// It is necessary to generate a font definition file first with the fontmaker
// utility. You do not need to call this function for the core PDF fonts
// (courier, helvetica, times, zapfdingbats).
//
// The JSON definition file (and the font file itself when embedding) must be
// present in the font directory. If it is not found, the error "Could not
// include font definition file" is set.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// fileStr specifies the base name with ".ttf" or ".otf" extension of the font
// file to be added. OpenType files with TrueType outlines are supported. CFF
// OpenType files are supported by font.Make/AddFont for single-byte
// encodings, not by this UTF-8 subsetting path.
func (f *Document) AddUTF8Font(familyStr, styleStr, fileStr string) {
	f.addFont(fontFamilyEscape(familyStr), styleStr, fileStr, true)
}

func (f *Document) addFont(familyStr, styleStr, fileStr string, isUTF8 bool) {
	if fileStr == "" {
		if isUTF8 {
			fileStr = strings.ReplaceAll(familyStr, " ", "") + strings.ToLower(styleStr) + ".ttf"
		} else {
			fileStr = strings.ReplaceAll(familyStr, " ", "") + strings.ToLower(styleStr) + ".json"
		}
	}
	if f.fontpath != "" && !validFontFilePath(fileStr) {
		f.SetErrorf("invalid font resource name: %s", fileStr)
		return
	}
	if isUTF8 {
		fontKey := getFontKey(familyStr, styleStr)
		if !validPDFNameFragment(fontKey) {
			f.SetErrorf("invalid UTF-8 font name: %s", fontKey)
			return
		}
		_, ok := f.fonts[fontKey]
		if ok {
			return
		}
		var ttfStat os.FileInfo
		var err error
		fileStr = joinFontPath(f.fontpath, fileStr)
		ttfStat, err = os.Stat(fileStr)
		if err != nil && strings.HasSuffix(strings.ToLower(fileStr), ".ttf") {
			otfStr := strings.TrimSuffix(fileStr, filepath.Ext(fileStr)) + ".otf"
			if otfStat, otfErr := os.Stat(otfStr); otfErr == nil {
				fileStr = otfStr
				ttfStat = otfStat
				err = nil
			}
		}
		if err != nil || ttfStat == nil {
			if err == nil {
				err = fmt.Errorf("font file not found: %s", fileStr)
			}
			f.SetError(err)
			return
		}
		originalSize := ttfStat.Size()
		if originalSize > maxFontSourceBytes {
			f.SetError(errors.New("font data exceeds maximum size"))
			return
		}
		utf8Bytes, err := readFontResourceFile(fileStr, maxFontSourceBytes)
		if err != nil {
			f.SetError(err)
			return
		}
		def, err := utf8FontDefinition(fontKey, fileStr, utf8Bytes)
		if err != nil {
			f.SetError(err)
			return
		}
		def.usedRunes = defaultUTF8UsedRunes(f.aliasNbPagesStr)
		f.fonts[fontKey] = def
		f.fontFiles[fontKey] = fontFile{length1: originalSize, fontType: "UTF8"}
		f.fontFiles[fileStr] = fontFile{fontType: "UTF8"}
	} else {
		if !validFontResourceName(fileStr) {
			f.SetErrorf("invalid font resource name: %s", fileStr)
			return
		}
		if f.fontLoader != nil {
			reader, err := f.fontLoader.Open(fileStr)
			if err == nil {
				f.AddFontFromReader(familyStr, styleStr, reader)
				if closer, ok := reader.(io.Closer); ok {
					_ = closer.Close()
				}
				return
			}
		}
		fileStr = joinFontPath(f.fontpath, fileStr)
		file, err := os.Open(fileStr)
		if err != nil {
			f.err = err
			return
		}
		defer func() { _ = file.Close() }()
		f.AddFontFromReader(familyStr, styleStr, file)
	}
}

func validFontResourceName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name == path.Base(name) && name != "." && name != ".." && !strings.Contains(name, "\\")
}

func joinFontPath(fontDirStr, fileStr string) string {
	if fontDirStr == "" || filepath.IsAbs(fileStr) {
		return fileStr
	}
	return filepath.Join(fontDirStr, fileStr)
}

func validFontFilePath(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "\\") {
		return false
	}
	clean := path.Clean(name)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func makeSubsetRange(end int) map[int]int {
	answer := make(map[int]int)
	for i := range end {
		answer[i] = 0
	}
	return answer
}

func validateFontDefinition(info fontDefinition) error {
	switch info.Tp {
	case "Core", "Type1", "TrueType", "OpenTypeCFF":
	default:
		return fmt.Errorf("invalid font type: %s", info.Tp)
	}
	if info.Tp != "Core" && len(info.Cw) < 256 {
		return errors.New("invalid font width table")
	}
	if info.File != "" {
		if !validFontResourceName(info.File) {
			return fmt.Errorf("invalid font resource name: %s", info.File)
		}
		if len(info.File) < 2 {
			return fmt.Errorf("invalid font resource name: %s", info.File)
		}
		switch {
		case info.Tp == "TrueType":
			if info.OriginalSize < 0 {
				return errors.New("invalid TrueType font size")
			}
		case info.Tp == "OpenTypeCFF":
			if info.OriginalSize < 0 {
				return errors.New("invalid OpenType/CFF font size")
			}
		case info.Size1 < 0 || info.Size2 < 0:
			return errors.New("invalid Type1 font size")
		}
	}
	if info.Name != "" && !validPDFNameFragment(info.Name) {
		return fmt.Errorf("invalid font name: %s", info.Name)
	}
	if info.Diff != "" && !validFontDiff(info.Diff) {
		return errors.New("invalid font encoding differences")
	}
	return nil
}

func validFontDiff(diff string) bool {
	fields := strings.Fields(diff)
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if strings.HasPrefix(field, "/") {
			if !validPDFResourceName(field) {
				return false
			}
			continue
		}
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

func validPDFResourceName(name string) bool {
	if len(name) < 2 || name[0] != '/' {
		return false
	}
	return validPDFNameFragment(name[1:])
}

func validPDFNameFragment(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func utf8FontDefinition(fontKey, fileStr string, utf8Bytes []byte) (fontDefinition, error) {
	reader := fileReader{readerPosition: 0, array: append([]byte(nil), utf8Bytes...)}
	utf8File := newUTF8Font(&reader)
	if err := utf8File.parseFile(); err != nil {
		return fontDefinition{}, err
	}
	desc := FontDescriptor{Ascent: utf8File.Ascent, Descent: utf8File.Descent, CapHeight: utf8File.CapHeight, Flags: utf8File.Flags, FontBBox: utf8File.Bbox, ItalicAngle: utf8File.ItalicAngle, StemV: utf8File.StemV, MissingWidth: round(utf8File.DefaultWidth)}
	def := fontDefinition{Tp: "UTF8", Name: fontKey, Desc: desc, Up: round(utf8File.UnderlinePosition), Ut: round(utf8File.UnderlineThickness), Cw: append([]int(nil), utf8File.CharWidths...), File: fileStr, utf8File: utf8File}
	def.i, _ = generateFontID(def)
	return def, nil
}

func defaultUTF8UsedRunes(alias string) map[int]int {
	if alias == "" {
		return makeSubsetRange(57)
	}
	return makeSubsetRange(32)
}

// AddFontFromBytes imports a TrueType, OpenType or Type1 font from static
// bytes within the executable and makes it available for use in the generated
// document.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// jsonFileBytes contains all bytes of the JSON definition file.
//
// zFileBytes contains all bytes of the zlib-compressed font file.
func (f *Document) AddFontFromBytes(familyStr, styleStr string, jsonFileBytes, zFileBytes []byte) {
	f.addFontFromBytes(fontFamilyEscape(familyStr), styleStr, jsonFileBytes, zFileBytes, nil)
}

// AddUTF8FontFromBytes imports a TrueType font with UTF-8 symbols from static
// bytes within the executable and makes it available for use in the generated
// document.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// utf8Bytes contains all bytes of the UTF-8 font file.
func (f *Document) AddUTF8FontFromBytes(familyStr, styleStr string, utf8Bytes []byte) {
	f.addFontFromBytes(fontFamilyEscape(familyStr), styleStr, nil, nil, utf8Bytes)
}

func (f *Document) addFontFromBytes(familyStr, styleStr string, jsonFileBytes, zFileBytes, utf8Bytes []byte) {
	if f.err != nil {
		return
	}
	var ok bool
	fontkey := getFontKey(familyStr, styleStr)
	if utf8Bytes != nil && !validPDFNameFragment(fontkey) {
		f.SetErrorf("invalid UTF-8 font name: %s", fontkey)
		return
	}
	_, ok = f.fonts[fontkey]
	if ok {
		return
	}
	if utf8Bytes != nil {
		if err := validateFontDataSize(utf8Bytes, maxFontSourceBytes, "font data"); err != nil {
			f.err = err
			return
		}
		def, err := utf8FontDefinition(fontkey, "", utf8Bytes)
		if err != nil {
			f.SetError(err)
			return
		}
		def.usedRunes = defaultUTF8UsedRunes(f.aliasNbPagesStr)
		f.fonts[fontkey] = def
	} else {
		if err := validateFontDataSize(jsonFileBytes, maxFontDefinitionBytes, "font definition"); err != nil {
			f.err = err
			return
		}
		if err := validateFontDataSize(zFileBytes, maxFontSourceBytes, "font data"); err != nil {
			f.err = err
			return
		}
		var info fontDefinition
		err := json.Unmarshal(jsonFileBytes, &info)
		if err != nil {
			f.err = err
		}
		if f.err != nil {
			return
		}
		if err = validateFontDefinition(info); err != nil {
			f.err = err
			return
		}
		if info.i, err = generateFontID(info); err != nil {
			f.err = err
			return
		}
		if len(info.Diff) > 0 {
			// Register the encoding differences.
			n := -1
			for j, str := range f.diffs {
				if str == info.Diff {
					n = j + 1
					break
				}
			}
			if n < 0 {
				f.diffs = append(f.diffs, info.Diff)
				n = len(f.diffs)
			}
			info.DiffN = n
		}
		if len(info.File) > 0 {
			switch info.Tp {
			case "TrueType":
				f.fontFiles[info.File] = fontFile{length1: int64(info.OriginalSize), embedded: true, content: zFileBytes}
			case "OpenTypeCFF":
				f.fontFiles[info.File] = fontFile{embedded: true, content: zFileBytes, fontType: "OpenTypeCFF"}
			default:
				f.fontFiles[info.File] = fontFile{length1: int64(info.Size1), length2: int64(info.Size2), embedded: true, content: zFileBytes}
			}
		}
		f.fonts[fontkey] = info
	}
}

// AddFontFromReader imports a TrueType, OpenType or Type1 font and makes it
// available using a reader that satisfies the io.Reader interface. See AddFont()
// for details about familyStr and styleStr.
func (f *Document) AddFontFromReader(familyStr, styleStr string, r io.Reader) {
	if f.err != nil {
		return
	}
	familyStr = fontFamilyEscape(familyStr)
	var ok bool
	fontkey := getFontKey(familyStr, styleStr)
	_, ok = f.fonts[fontkey]
	if ok {
		return
	}
	info := f.loadfont(r)
	if f.err != nil {
		return
	}
	if err := validateFontDefinition(info); err != nil {
		f.err = err
		return
	}
	if len(info.Diff) > 0 {
		n := -1
		for j, str := range f.diffs {
			if str == info.Diff {
				n = j + 1
				break
			}
		}
		if n < 0 {
			f.diffs = append(f.diffs, info.Diff)
			n = len(f.diffs)
		}
		info.DiffN = n
	}
	if len(info.File) > 0 {
		switch info.Tp {
		case "TrueType":
			f.fontFiles[info.File] = fontFile{length1: int64(info.OriginalSize)}
		case "OpenTypeCFF":
			f.fontFiles[info.File] = fontFile{fontType: "OpenTypeCFF"}
		default:
			f.fontFiles[info.File] = fontFile{length1: int64(info.Size1), length2: int64(info.Size2)}
		}
	}
	f.fonts[fontkey] = info
}

// loadfont loads a font definition file from the given reader.
func (f *Document) loadfont(r io.Reader) (def fontDefinition) {
	if f.err != nil {
		return
	}
	data, err := readFontResourceReader(r, maxFontDefinitionBytes)
	if err != nil {
		f.err = err
		return
	}
	err = json.Unmarshal(data, &def)
	if err != nil {
		f.err = err
		return
	}
	if def.i, err = generateFontID(def); err != nil {
		f.err = err
	}
	return
}

func (f *Document) putfonts() {
	if f.err != nil {
		return
	}
	nf := f.n
	for _, diff := range f.diffs {
		f.newobj()
		f.outf("<</Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [%s]>>", diff)
		f.out("endobj")
	}
	{
		var fileList []string
		var info fontFile
		var file string
		for file = range f.fontFiles {
			fileList = append(fileList, file)
		}
		if f.catalogSort {
			sort.SliceStable(fileList, func(i, j int) bool {
				return fileList[i] < fileList[j]
			})
		}
		for _, file = range fileList {
			info = f.fontFiles[file]
			if info.fontType != "UTF8" {
				f.newobj()
				info.n = f.n
				f.fontFiles[file] = info
				var font []byte
				if info.embedded {
					font = info.content
				} else {
					var err error
					font, err = f.loadFontFile(file)
					if err != nil {
						f.err = err
						return
					}
				}
				compressed := strings.HasSuffix(file, ".z")
				if !compressed && info.length2 > 0 {
					if info.length1 < 6 || info.length1 > int64(len(font)) || info.length2 > int64(len(font)) || 6+info.length1+6 > info.length2 {
						f.err = fmt.Errorf("invalid Type1 font segment lengths: %s", file)
						return
					}
					buf := font[6:info.length1]
					buf = append(buf, font[6+info.length1+6:info.length2]...)
					font = buf
				}
				f.outf("<</Length %d", len(font))
				if compressed {
					f.out("/Filter /FlateDecode")
				}
				if info.fontType == "OpenTypeCFF" {
					f.out("/Subtype /Type1C")
				} else {
					f.outf("/Length1 %d", info.length1)
					if info.length2 > 0 {
						f.outf("/Length2 %d /Length3 0", info.length2)
					}
				}
				f.out(">>")
				f.putstream(font)
				f.out("endobj")
			}
		}
	}
	{
		var keyList []string
		var font fontDefinition
		var key string
		for key = range f.fonts {
			keyList = append(keyList, key)
		}
		if f.catalogSort {
			sort.SliceStable(keyList, func(i, j int) bool {
				return keyList[i] < keyList[j]
			})
		}
		for _, key = range keyList {
			font = f.fonts[key]
			font.N = f.n + 1
			f.fonts[key] = font
			tp := font.Tp
			name := font.Name
			switch tp {
			case "Core":
				f.newobj()
				f.out("<</Type /Font")
				f.outf("/BaseFont /%s", name)
				f.out("/Subtype /Type1")
				if name != "Symbol" && name != "ZapfDingbats" {
					f.out("/Encoding /WinAnsiEncoding")
				}
				f.out(">>")
				f.out("endobj")
			case "Type1", "TrueType", "OpenTypeCFF":
				f.newobj()
				f.out("<</Type /Font")
				f.outf("/BaseFont /%s", name)
				fontSubtype := tp
				if tp == "OpenTypeCFF" {
					fontSubtype = "Type1"
				}
				f.outf("/Subtype /%s", fontSubtype)
				f.out("/FirstChar 32 /LastChar 255")
				f.outf("/Widths %d 0 R", f.n+1)
				f.outf("/FontDescriptor %d 0 R", f.n+2)
				if font.DiffN > 0 {
					f.outf("/Encoding %d 0 R", nf+font.DiffN)
				} else {
					f.out("/Encoding /WinAnsiEncoding")
				}
				f.out(">>")
				f.out("endobj")
				f.newobj()
				var s fmtBuffer
				_, _ = s.WriteString("[")
				for j := 32; j < 256; j++ {
					s.printf("%d ", font.Cw[j])
				}
				_, _ = s.WriteString("]")
				f.out(s.String())
				f.out("endobj")
				f.newobj()
				s.Truncate(0)
				s.printf("<</Type /FontDescriptor /FontName /%s ", name)
				s.printf("/Ascent %d ", font.Desc.Ascent)
				s.printf("/Descent %d ", font.Desc.Descent)
				s.printf("/CapHeight %d ", font.Desc.CapHeight)
				s.printf("/Flags %d ", font.Desc.Flags)
				s.printf("/FontBBox [%d %d %d %d] ", font.Desc.FontBBox.Xmin, font.Desc.FontBBox.Ymin, font.Desc.FontBBox.Xmax, font.Desc.FontBBox.Ymax)
				s.printf("/ItalicAngle %d ", font.Desc.ItalicAngle)
				s.printf("/StemV %d ", font.Desc.StemV)
				s.printf("/MissingWidth %d ", font.Desc.MissingWidth)
				var suffix string
				if tp == "OpenTypeCFF" {
					suffix = "3"
				} else if tp != "Type1" {
					suffix = "2"
				}
				s.printf("/FontFile%s %d 0 R>>", suffix, f.fontFiles[font.File].n)
				f.out(s.String())
				f.out("endobj")
			case "UTF8":
				fontName := "utf8" + font.Name
				usedRunes := font.usedRunes
				delete(usedRunes, 0)
				utf8FontStream := font.utf8File.GenerateCutFont(usedRunes)
				if font.utf8File.fileReader.err != nil {
					f.err = font.utf8File.fileReader.err
					return
				}
				if utf8FontStream == nil {
					f.err = errors.New("unable to generate UTF-8 font subset")
					return
				}
				utf8FontSize := len(utf8FontStream)
				compressedFontStream := f.compressBytes(utf8FontStream)
				if f.err != nil {
					return
				}
				CodeSignDictionary := font.utf8File.CodeSymbolDictionary
				delete(CodeSignDictionary, 0)
				f.newobj()
				f.out(fmt.Sprintf("<</Type /Font\n/Subtype /Type0\n/BaseFont /%s\n/Encoding /Identity-H\n/DescendantFonts [%d 0 R]\n/ToUnicode %d 0 R>>\n"+"endobj", fontName, f.n+1, f.n+2))
				f.newobj()
				f.out("<</Type /Font\n/Subtype /CIDFontType2\n/BaseFont /" + fontName + "\n" + "/CIDSystemInfo " + strconv.Itoa(f.n+2) + " 0 R\n/FontDescriptor " + strconv.Itoa(f.n+3) + " 0 R")
				if font.Desc.MissingWidth != 0 {
					f.out("/DW " + strconv.Itoa(font.Desc.MissingWidth) + "")
				}
				f.generateCIDFontMap(&font, font.utf8File.LastRune)
				f.out("/CIDToGIDMap " + strconv.Itoa(f.n+4) + " 0 R>>")
				f.out("endobj")
				f.newobj()
				f.out("<</Length " + strconv.Itoa(len(toUnicode)) + ">>")
				f.putstream([]byte(toUnicode))
				f.out("endobj")
				f.newobj()
				f.out("<</Registry (Adobe)\n/Ordering (UCS)\n/Supplement 0>>")
				f.out("endobj")
				f.newobj()
				var s fmtBuffer
				s.printf("<</Type /FontDescriptor /FontName /%s\n /Ascent %d", fontName, font.Desc.Ascent)
				s.printf(" /Descent %d", font.Desc.Descent)
				s.printf(" /CapHeight %d", font.Desc.CapHeight)
				v := font.Desc.Flags
				v |= 4
				v &^= 32
				s.printf(" /Flags %d", v)
				s.printf("/FontBBox [%d %d %d %d] ", font.Desc.FontBBox.Xmin, font.Desc.FontBBox.Ymin, font.Desc.FontBBox.Xmax, font.Desc.FontBBox.Ymax)
				s.printf(" /ItalicAngle %d", font.Desc.ItalicAngle)
				s.printf(" /StemV %d", font.Desc.StemV)
				s.printf(" /MissingWidth %d", font.Desc.MissingWidth)
				s.printf("/FontFile2 %d 0 R", f.n+2)
				s.printf(">>")
				f.out(s.String())
				f.out("endobj")
				cidToGidMap := make([]byte, 256*256*2)
				for cc, glyph := range CodeSignDictionary {
					cidToGidMap[cc*2] = byte(glyph >> 8)
					cidToGidMap[cc*2+1] = byte(glyph & 0xFF)
				}
				cidToGidMap = f.compressBytes(cidToGidMap)
				if f.err != nil {
					return
				}
				f.newobj()
				f.out("<</Length " + strconv.Itoa(len(cidToGidMap)) + "/Filter /FlateDecode>>")
				f.putstream(cidToGidMap)
				f.out("endobj")
				f.newobj()
				f.out("<</Length " + strconv.Itoa(len(compressedFontStream)))
				f.out("/Filter /FlateDecode")
				f.out("/Length1 " + strconv.Itoa(utf8FontSize))
				f.out(">>")
				f.putstream(compressedFontStream)
				f.out("endobj")
			default:
				f.err = fmt.Errorf("unsupported font type: %s", tp)
				return
			}
		}
	}
}

func (f *Document) generateCIDFontMap(font *fontDefinition, lastRune int) {
	if font == nil {
		f.err = errors.New("missing font definition")
		return
	}
	if lastRune >= len(font.Cw) {
		lastRune = len(font.Cw) - 1
	}
	if lastRune < 1 {
		f.out("/W []")
		return
	}
	rangeID := 0
	cidArray := make(map[int]*phpOrderedIntMap)
	cidArrayKeys := make([]int, 0)
	prevCid := -2
	prevWidth := -1
	interval := false
	startCid := 1
	cwLen := lastRune + 1
	for cid := startCid; cid < cwLen; cid++ {
		if font.Cw[cid] == 0x00 {
			continue
		}
		width := font.Cw[cid]
		if width == 65535 {
			width = 0
		}
		if numb, OK := font.usedRunes[cid]; cid > 255 && (!OK || numb == 0) {
			continue
		}
		if cid == prevCid+1 {
			if width == prevWidth {
				if width == cidArray[rangeID].get(0) {
					cidArray[rangeID].put(nil, width)
				} else {
					cidArray[rangeID].pop()
					rangeID = prevCid
					r := phpOrderedIntMap{valueSet: make([]int, 0), keySet: make([]any, 0)}
					cidArray[rangeID] = &r
					cidArrayKeys = append(cidArrayKeys, rangeID)
					cidArray[rangeID].put(nil, prevWidth)
					cidArray[rangeID].put(nil, width)
				}
				interval = true
				cidArray[rangeID].put("interval", 1)
			} else {
				if interval {
					rangeID = cid
					r := phpOrderedIntMap{valueSet: make([]int, 0), keySet: make([]any, 0)}
					cidArray[rangeID] = &r
					cidArrayKeys = append(cidArrayKeys, rangeID)
					cidArray[rangeID].put(nil, width)
				} else {
					cidArray[rangeID].put(nil, width)
				}
				interval = false
			}
		} else {
			rangeID = cid
			r := phpOrderedIntMap{valueSet: make([]int, 0), keySet: make([]any, 0)}
			cidArray[rangeID] = &r
			cidArrayKeys = append(cidArrayKeys, rangeID)
			cidArray[rangeID].put(nil, width)
			interval = false
		}
		prevCid = cid
		prevWidth = width
	}
	previousKey := -1
	nextKey := -1
	isInterval := false
	for g := 0; g < len(cidArrayKeys); {
		key := cidArrayKeys[g]
		if cidArray[key] == nil {
			g++
			continue
		}
		ws := *cidArray[key]
		cws := len(ws.keySet)
		if (key == nextKey) && (!isInterval) && (ws.getIndex("interval") < 0 || cws < 4) {
			if cidArray[key].getIndex("interval") >= 0 {
				cidArray[key].delete("interval")
			}
			cidArray[previousKey] = arrayMerge(cidArray[previousKey], cidArray[key])
			cidArrayKeys = remove(cidArrayKeys, key)
		} else {
			g++
			previousKey = key
		}
		nextKey = key + cws
		if ws.getIndex("interval") >= 0 {
			if cws > 3 {
				isInterval = true
			} else {
				isInterval = false
			}
			cidArray[key].delete("interval")
			nextKey--
		} else {
			isInterval = false
		}
	}
	var w fmtBuffer
	for _, k := range cidArrayKeys {
		ws := cidArray[k]
		if ws == nil {
			continue
		}
		if len(arrayCountValues(ws.valueSet)) == 1 {
			w.printf(" %d %d %d", k, k+len(ws.valueSet)-1, ws.get(0))
		} else {
			w.printf(" %d [ %s ]\n", k, implode(" ", ws.valueSet))
		}
	}
	f.out("/W [" + w.String() + " ]")
}

func implode(sep string, arr []int) string {
	var s fmtBuffer
	for i := 0; i < len(arr)-1; i++ {
		s.printf("%v", arr[i])
		_, _ = s.WriteString(sep)
	}
	if len(arr) > 0 {
		s.printf("%v", arr[len(arr)-1])
	}
	return s.String()
}

// arrayCountValues counts the occurrences of each item in mp.
func arrayCountValues(mp []int) map[int]int {
	answer := make(map[int]int)
	for _, v := range mp {
		answer[v]++
	}
	return answer
}

func (f *Document) loadFontFile(name string) ([]byte, error) {
	if !validFontResourceName(name) {
		return nil, fmt.Errorf("invalid font resource name: %s", name)
	}
	if f.fontLoader != nil {
		reader, err := f.fontLoader.Open(name)
		if err == nil {
			data, err := readFontResourceReader(reader, maxFontSourceBytes)
			if closer, ok := reader.(io.Closer); ok {
				_ = closer.Close()
			}
			return data, err
		}
	}
	return readFontResourceFile(joinFontPath(f.fontpath, name), maxFontSourceBytes)
}
