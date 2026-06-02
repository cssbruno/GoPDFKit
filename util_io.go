// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

const (
	maxImageSourceBytes  = 64 * 1024 * 1024
	maxImageDecodedBytes = 256 * 1024 * 1024
	maxImagePixels       = 50 * 1000 * 1000
)

// fileExist returns true if the specified regular file exists.
func fileExist(filename string) (ok bool) {
	info, err := os.Stat(filename)
	if err == nil {
		if ^os.ModePerm&info.Mode() == 0 {
			ok = true
		}
	}
	return ok
}

// fileSize returns the size of the specified file; ok is false if the file does
// not exist or is not a regular file.
func fileSize(filename string) (size int64, ok bool) {
	info, err := os.Stat(filename)
	ok = err == nil && info != nil
	if ok {
		size = info.Size()
	}
	return
}

func bufferFromReaderLimit(r io.Reader, limit int) (b *bytes.Buffer, err error) {
	b = new(bytes.Buffer)
	if limit >= 0 {
		_, err = b.ReadFrom(io.LimitReader(r, int64(limit)+1))
		if err == nil && b.Len() > limit {
			err = fmt.Errorf("image data exceeds maximum size")
		}
		return
	}
	_, err = b.ReadFrom(r)
	return
}
