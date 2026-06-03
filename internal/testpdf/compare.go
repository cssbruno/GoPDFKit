// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package testpdf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

func writeBytes(leadStr string, startPos int, sl []byte) {
	var pos, max int
	var b byte
	fmt.Printf("%s %07x", leadStr, startPos)
	max = len(sl)
	for pos < max {
		fmt.Printf(" ")
		for range 8 {
			if pos < max {
				fmt.Printf(" %02x", sl[pos])
			} else {
				fmt.Printf("   ")
			}
			pos++
		}
	}
	fmt.Printf("  |")
	pos = 0
	for pos < max {
		b = sl[pos]
		if b < 32 || b >= 128 {
			b = '.'
		}
		fmt.Printf("%c", b)
		pos++
	}
	fmt.Printf("|\n")
}

func checkBytes(pos int, sl1, sl2 []byte, printDiff bool) (eq bool) {
	eq = bytes.Equal(sl1, sl2)
	if !eq && printDiff {
		writeBytes("<", pos, sl1)
		writeBytes(">", pos, sl2)
	}
	return
}

// CompareBytes compares the bytes referred to by sl1 with those referred to by
// sl2. nil is returned if the buffers are equal; otherwise an error is
// returned.
func CompareBytes(sl1, sl2 []byte, printDiff bool) (err error) {
	var posStart, posEnd, len1, len2, length int
	var diffs bool

	len1 = len(sl1)
	len2 = len(sl2)
	if len1 != len2 {
		diffs = true
	}
	length = min(len1, len2)
	for posStart < length {
		posEnd = min(posStart+16, length)
		if !checkBytes(posStart, sl1[posStart:posEnd], sl2[posStart:posEnd], printDiff) {
			diffs = true
		}
		posStart = posEnd
	}
	if diffs {
		err = errors.New("documents are different")
	}
	return
}

// ComparePDFs reads and compares the full contents of the two specified
// readers byte-for-byte. nil is returned if the buffers are equal; otherwise
// an error is returned.
func ComparePDFs(rdr1, rdr2 io.Reader, printDiff bool) (err error) {
	b1 := new(bytes.Buffer)
	b2 := new(bytes.Buffer)
	_, err = b1.ReadFrom(rdr1)
	if err == nil {
		_, err = b2.ReadFrom(rdr2)
		if err == nil {
			err = CompareBytes(b1.Bytes(), b2.Bytes(), printDiff)
		}
	}
	return
}

// ComparePDFFiles reads and compares the full contents of the two specified
// files byte-for-byte. nil is returned if the file contents are equal or if
// the second file cannot be read; otherwise an error is returned.
func ComparePDFFiles(file1Str, file2Str string, printDiff bool) (err error) {
	var sl1, sl2 []byte
	sl1, err = os.ReadFile(file1Str)
	if err == nil {
		sl2, err = os.ReadFile(file2Str)
		if err == nil {
			err = CompareBytes(sl1, sl2, printDiff)
		} else {
			// Treat an unreadable second file as success.
			err = nil
		}
	}
	return
}
