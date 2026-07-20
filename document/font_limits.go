// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"io"
	"os"
)

const (
	maxFontDefinitionBytes = 4 * 1024 * 1024
	maxFontSourceBytes     = 64 * 1024 * 1024
)

func validateFontDataSize(data []byte, limit int, label string) error {
	if len(data) > limit {
		return errors.New(label + " exceeds maximum size")
	}
	return nil
}

func readFontResourceFile(path string, limit int) ([]byte, error) {
	file, err := os.Open(path) // #nosec G304 -- Internal helper serves explicit font-path APIs and enforces limits.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if info, err := file.Stat(); err == nil && info.Mode().IsRegular() && info.Size() > int64(limit) {
		return nil, errors.New("font data exceeds maximum size")
	}
	return readFontResourceReader(file, limit)
}

func readFontResourceReader(r io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err == nil && len(data) > limit {
		err = errors.New("font data exceeds maximum size")
	}
	return data, err
}
