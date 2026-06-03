// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package example provides helpers for deterministic example PDF generation.
package example

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cssbruno/gopdfkit/compare"
	"github.com/cssbruno/gopdfkit/document"
)

var gopdfkitDir string

func init() {
	setRoot()
	document.SetDefaultCompression(false)
	document.SetDefaultCatalogSort(true)
	document.SetDefaultCreationDate(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	document.SetDefaultModificationDate(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
}

// setRoot records the path from the current working directory to the repository
// root.
func setRoot() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	gopdfkitDir = ""
	for {
		if _, err := os.Stat(filepath.Join(gopdfkitDir, "assets", "static")); err == nil {
			if gopdfkitDir != "" {
				if err := os.Chdir(gopdfkitDir); err != nil {
					panic(err)
				}
				gopdfkitDir = ""
			}
			return
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			panic("could not find GoPDFKit repository root")
		}
		wd = parent
		gopdfkitDir = filepath.Join(gopdfkitDir, "..")
	}
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
	return filepath.Join(gopdfkitDir, "assets", "generated", "pdf")
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

// referenceCompare compares fileStr with its copy in the reference
// subdirectory.
//
// It compares all bytes except the PDF /CreationDate value. The comparison
// succeeds if both files are equivalent except for /CreationDate values, or if
// the reference file does not exist.
func referenceCompare(fileStr string) (err error) {
	var refFileStr, refDirStr, dirStr, baseFileStr string
	dirStr, baseFileStr = filepath.Split(fileStr)
	refDirStr = filepath.Join(dirStr, "reference")
	err = os.MkdirAll(refDirStr, 0o750)
	if err == nil {
		refFileStr = filepath.Join(refDirStr, baseFileStr)
		err = compare.ComparePDFFiles(fileStr, refFileStr, false)
	}
	return
}

// Summary prints a predictable report for test examples.
//
// If err is nil, Summary normalizes path separators and prints a success
// message for fileStr. Otherwise, it prints err.
func Summary(err error, fileStr string) {
	if err == nil {
		fileStr = filepath.ToSlash(fileStr)
		fmt.Printf("Successfully generated %s\n", fileStr)
	} else {
		fmt.Println(err)
	}
}

// SummaryCompare prints a predictable report for test examples.
//
// If err is nil, SummaryCompare compares the generated file with its reference
// copy. Matching files produce the same success message as Summary; mismatches
// and non-nil errors are printed to standard output.
func SummaryCompare(err error, fileStr string) {
	if err == nil {
		err = referenceCompare(fileStr)
	}
	if err == nil {
		fileStr = filepath.ToSlash(fileStr)
		fmt.Printf("Successfully generated %s\n", fileStr)
	} else {
		fmt.Println(err)
	}
}
