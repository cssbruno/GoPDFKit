// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package example provides internal helpers for deterministic example PDF generation.
package example

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cssbruno/gopdfkit/document"
)

var (
	gopdfkitDir string
	pdfDir      string
)

const pdfDisplayDir = "assets/generated/pdf"

func init() {
	setRoot()
	document.SetDefaultCompression(false)
	document.SetDefaultCatalogSort(true)
	document.SetDefaultCreationDate(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	document.SetDefaultModificationDate(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
}

// setRoot records the repository root from this source file instead of the
// process working directory. Go may execute tests from temporary directories or
// from the module cache.
func setRoot() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("could not resolve GoPDFKit example helper source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "assets", "static")); err != nil {
		panic(fmt.Errorf("could not find GoPDFKit static assets from %s: %w", root, err))
	}
	gopdfkitDir = root
	pdfDir = filepath.Join(gopdfkitDir, pdfDisplayDir)
	if !ensureWritableDir(pdfDir) {
		tmpDir, err := os.MkdirTemp("", "gopdfkit-generated-pdf-*")
		if err != nil {
			panic(err)
		}
		pdfDir = tmpDir
	}
}

func ensureWritableDir(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	file, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return false
	}
	return os.Remove(name) == nil
}

// ImageFile returns the path to a file in the static image directory.
func ImageFile(fileStr string) string {
	return filepath.Join(gopdfkitDir, "assets", "static", "image", fileStr)
}

// FontDir returns the path to the static font directory.
func FontDir() string {
	return filepath.Join(gopdfkitDir, "assets", "static", "font")
}

// FontFile returns the path to a file in the static font directory.
func FontFile(fileStr string) string {
	return filepath.Join(FontDir(), fileStr)
}

// TextFile returns the path to a file in the static text directory.
func TextFile(fileStr string) string {
	return filepath.Join(gopdfkitDir, "assets", "static", "text", fileStr)
}

// PdfDir returns the path to the PDF output directory.
func PdfDir() string {
	return pdfDir
}

// PdfFile returns the path to a file in the PDF output directory.
func PdfFile(fileStr string) string {
	return filepath.Join(PdfDir(), fileStr)
}

// Filename returns the output path for an example PDF, adding the ".pdf"
// suffix to baseStr.
func Filename(baseStr string) string {
	return PdfFile(baseStr + ".pdf")
}

// Summary prints a predictable report for test examples.
//
// If err is nil, Summary normalizes path separators and prints a success
// message for fileStr. Otherwise, it prints err.
func Summary(err error, fileStr string) {
	if err == nil {
		fileStr = filepath.ToSlash(displayPath(fileStr))
		fmt.Printf("Successfully generated %s\n", fileStr)
	} else {
		fmt.Println(err)
	}
}

func displayPath(fileStr string) string {
	if rel, ok := relativeInside(pdfDir, fileStr); ok {
		return filepath.Join(pdfDisplayDir, rel)
	}
	if rel, ok := relativeInside(gopdfkitDir, fileStr); ok {
		return rel
	}
	return fileStr
}

func relativeInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
