// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	maxImageSourceBytes  = 64 * 1024 * 1024
	maxImageDecodedBytes = 256 * 1024 * 1024
	maxImageDimension    = 16384
	maxImagePixels       = 50 * 1000 * 1000
)

func bufferFromReaderLimit(r io.Reader, limit int) (b *bytes.Buffer, err error) {
	b = new(bytes.Buffer)
	if limit >= 0 {
		_, err = b.ReadFrom(io.LimitReader(r, int64(limit)+1))
		if err == nil && b.Len() > limit {
			err = errors.New("image data exceeds maximum size")
		}
		return
	}
	_, err = b.ReadFrom(r)
	return
}

func readFileLimit(filename string, limit int64) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if limit >= 0 {
		if info, statErr := file.Stat(); statErr == nil && info.Mode().IsRegular() && info.Size() > limit {
			return nil, errors.New("image data exceeds maximum size")
		}
		data, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err == nil && int64(len(data)) > limit {
			err = errors.New("image data exceeds maximum size")
		}
		return data, err
	}
	return io.ReadAll(file)
}

func validImagePixelCount(width, height int) bool {
	return validateImageDimensions(width, height) == nil
}

func validateImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid image dimensions: %d x %d", width, height)
	}
	if width > maxImageDimension || height > maxImageDimension {
		return fmt.Errorf("image dimensions exceed maximum: %d x %d", width, height)
	}
	if int64(width)*int64(height) > int64(maxImagePixels) {
		return fmt.Errorf("image pixel count exceeds maximum: %d x %d", width, height)
	}
	return nil
}
