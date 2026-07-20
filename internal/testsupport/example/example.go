// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package example provides internal helpers for deterministic example PDF generation.
package example

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	paperruneDir string
	pdfDir      string
)

const pdfDisplayDir = "assets/generated/pdf"

func init() {
	setRoot()
}

// setRoot records the repository root from this source file instead of the
// process working directory. Go may execute tests from temporary directories or
// from the module cache.
func setRoot() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("could not resolve PaperRune example helper source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "assets", "static")); err != nil {
		panic(fmt.Errorf("could not find PaperRune static assets from %s: %w", root, err))
	}
	paperruneDir = root
	tmpDir, err := os.MkdirTemp("", "paperrune-generated-pdf-*")
	if err != nil {
		panic(err)
	}
	pdfDir = tmpDir
}

// ImageFile returns the path to a file in the static image directory.
func ImageFile(fileStr string) string {
	return filepath.Join(paperruneDir, "assets", "static", "image", fileStr)
}

// FontDir returns the path to the static font directory.
func FontDir() string {
	return filepath.Join(paperruneDir, "assets", "static", "font")
}

// FontFile returns the path to a file in the static font directory.
func FontFile(fileStr string) string {
	return filepath.Join(FontDir(), fileStr)
}

// TextFile returns the path to a file in the static text directory.
func TextFile(fileStr string) string {
	return filepath.Join(paperruneDir, "assets", "static", "text", fileStr)
}

// RepoFile returns the path to a file at the repository root.
func RepoFile(elems ...string) string {
	parts := append([]string{paperruneDir}, elems...)
	return filepath.Join(parts...)
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
		fmt.Printf("Successfully generated %s\n", DisplayPath(fileStr))
	} else {
		fmt.Println(err)
	}
}

// DisplayPath returns a stable slash-separated path for example output.
func DisplayPath(fileStr string) string {
	return filepath.ToSlash(displayPath(fileStr))
}

func displayPath(fileStr string) string {
	if rel, ok := relativeInside(pdfDir, fileStr); ok {
		return filepath.Join(pdfDisplayDir, rel)
	}
	if rel, ok := relativeInside(paperruneDir, fileStr); ok {
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
