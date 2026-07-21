// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func utf8toutf16(s string, withBOM ...bool) string {
	bom := true
	if len(withBOM) > 0 {
		bom = withBOM[0]
	}
	res := make([]byte, 0, len(s)*2+2)
	if bom {
		res = append(res, 0xFE, 0xFF)
	}
	for _, r := range s {
		if r <= 0xFFFF {
			res = appendUTF16BECodeUnit(res, r)
			continue
		}
		r1, r2 := utf16.EncodeRune(r)
		res = appendUTF16BECodeUnit(res, r1)
		res = appendUTF16BECodeUnit(res, r2)
	}
	return string(res)
}

func appendEscapedUTF16BE(dst []byte, s string, withBOM bool, usedRunes map[int]int) []byte {
	if withBOM {
		dst = appendEscapedPDFLiteralByte(dst, 0xFE)
		dst = appendEscapedPDFLiteralByte(dst, 0xFF)
	}
	for _, r := range s {
		if usedRunes != nil {
			usedRunes[int(r)] = int(r)
		}
		if r <= 0xFFFF {
			dst = appendEscapedUTF16BECodeUnit(dst, r)
			continue
		}
		r1, r2 := utf16.EncodeRune(r)
		dst = appendEscapedUTF16BECodeUnit(dst, r1)
		dst = appendEscapedUTF16BECodeUnit(dst, r2)
	}
	return dst
}

func appendEscapedUTF16BEReverse(dst []byte, s string, withBOM bool, usedRunes map[int]int) []byte {
	if withBOM {
		dst = appendEscapedPDFLiteralByte(dst, 0xFE)
		dst = appendEscapedPDFLiteralByte(dst, 0xFF)
	}
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if size <= 0 {
			break
		}
		s = s[:len(s)-size]
		if usedRunes != nil {
			usedRunes[int(r)] = int(r)
		}
		if r <= 0xFFFF {
			dst = appendEscapedUTF16BECodeUnit(dst, r)
			continue
		}
		r1, r2 := utf16.EncodeRune(r)
		dst = appendEscapedUTF16BECodeUnit(dst, r1)
		dst = appendEscapedUTF16BECodeUnit(dst, r2)
	}
	return dst
}

func appendUTF16BECodeUnit(dst []byte, value rune) []byte {
	return append(dst, byte(value>>8), byte(value)) // #nosec G115 -- Deliberate big-endian packing of a validated UTF-16 code unit.
}

func appendEscapedUTF16BECodeUnit(dst []byte, value rune) []byte {
	dst = appendEscapedPDFLiteralByte(dst, byte(value>>8)) // #nosec G115 -- Deliberate high byte of a validated UTF-16 code unit.
	return appendEscapedPDFLiteralByte(dst, byte(value))   // #nosec G115 -- Deliberate low byte of a validated UTF-16 code unit.
}

func appendEscapedPDFLiteralByte(dst []byte, b byte) []byte {
	switch b {
	case '\\', '(', ')':
		dst = append(dst, '\\', b)
	case '\r':
		dst = append(dst, '\\', 'r')
	default:
		dst = append(dst, b)
	}
	return dst
}

// doNothing returns the passed string with no translation.
func doNothing(s string) string {
	return s
}

func repClosure(m map[rune]byte) func(string) string {
	return func(str string) string {
		var buf bytes.Buffer
		var ch byte
		var ok bool
		for _, r := range str {
			if r < 0x80 {
				ch = byte(r) // #nosec G115 -- The branch proves this rune is a single-byte ASCII value.
			} else {
				ch, ok = m[r]
				if !ok {
					ch = byte('.')
				}
			}
			buf.WriteByte(ch)
		}
		return buf.String()
	}
}

// UnicodeTranslator returns a function that can be used to translate, where
// possible, UTF-8 strings to a form that is compatible with the specified code
// page. The returned function accepts a string and returns a string.
//
// r should read content lines for the code page of interest. Each line is made
// up of three whitespace-separated fields. The first begins with "!" and is
// followed by two hexadecimal digits that identify the glyph position in the
// code page of interest. The second field begins with "U+" and is followed by
// the Unicode code point value. The third is the glyph name. A number of these
// code page map files are packaged with gopdfkit in the font directory.
//
// An error occurs only if a line is read that does not conform to the expected
// format. In this case, the returned function is valid but does not perform
// any rune translation.
func UnicodeTranslator(r io.Reader) (f func(string) string, err error) {
	m := make(map[rune]byte)
	var uPos, cPos uint32
	var lineStr, nameStr string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lineStr = sc.Text()
		lineStr = strings.TrimSpace(lineStr)
		if len(lineStr) > 0 {
			_, err = fmt.Sscanf(lineStr, "!%2X U+%4X %s", &cPos, &uPos, &nameStr)
			if err == nil {
				if cPos >= 0x80 {
					m[rune(uPos)] = byte(cPos)
				}
			}
		}
	}
	if err == nil {
		f = repClosure(m)
	} else {
		f = doNothing
	}
	return
}

// UnicodeTranslatorFromFile returns a function that can be used to translate,
// where possible, UTF-8 strings to a form that is compatible with the
// specified code page. See UnicodeTranslator for more details.
//
// fileStr identifies a font descriptor file that maps glyph positions to names.
//
// If an error occurs reading the file, the returned function is valid but does
// not perform any rune translation.
func UnicodeTranslatorFromFile(fileStr string) (f func(string) string, err error) {
	var fl *os.File
	fl, err = os.Open(fileStr) // #nosec G304 -- Utility reads an explicit caller-selected encoding file.
	if err == nil {
		f, err = UnicodeTranslator(fl)
		_ = fl.Close()
	} else {
		f = doNothing
	}
	return
}

// UnicodeTranslatorFromDescriptor returns a function that can be used to
// translate, where possible, UTF-8 strings to a form that is compatible with
// the specified code page. See UnicodeTranslator for more details.
//
// cpStr identifies a code page. A descriptor file in the font directory, set
// with the fontDirStr argument in the call to New(), should have this name
// plus the extension ".map". If cpStr is empty, it will be replaced with
// "cp1252", the gopdfkit code page default.
//
// If an error occurs reading the descriptor, the returned function is valid
// but does not perform any rune translation.
//
// The CellFormat_codepage example demonstrates this method.
func (f *Document) UnicodeTranslatorFromDescriptor(cpStr string) (rep func(string) string) {
	var str string
	var ok bool
	if f.err == nil {
		if len(cpStr) == 0 {
			cpStr = "cp1252"
		}
		str, ok = embeddedMapList[cpStr]
		if ok {
			rep, f.err = UnicodeTranslator(strings.NewReader(str))
		} else {
			rep, f.err = UnicodeTranslatorFromFile(filepath.Join(f.fontpath, cpStr) + ".map")
		}
	} else {
		rep = doNothing
	}
	return
}

// fontFamilyEscape conditions a font family string for PDF name compliance.
// See section 5.3 (Names) in
// https://resources.infosecinstitute.com/pdf-file-format-basic-structure/
func fontFamilyEscape(familyStr string) (escStr string) {
	escStr = strings.ReplaceAll(familyStr, " ", "#20")
	// Additional replacements can be added here.
	return
}
