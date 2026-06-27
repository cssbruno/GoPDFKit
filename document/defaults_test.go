// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/document"
)

func TestNewWithDefaultsUsesExplicitSettings(t *testing.T) {
	defaults := document.DefaultSettings()
	defaults.CatalogSort = true
	defaults.Compression = false
	defaults.CreationDate = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	defaults.ModificationDate = time.Date(2007, 8, 9, 10, 11, 12, 0, time.UTC)

	out, err := outputWithDefaults(document.Options{}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "D:20010203040506") {
		t.Fatal("expected explicit creation date in PDF output")
	}
	if !strings.Contains(out, "D:20070809101112") {
		t.Fatal("expected explicit modification date in PDF output")
	}
	if strings.Contains(out, "/Filter /FlateDecode") {
		t.Fatal("expected explicit defaults to disable Flate compression")
	}
}

func TestNewWithDefaultsIgnoresPackageCompressionDefault(t *testing.T) {
	restore := document.DefaultSettings()
	t.Cleanup(func() {
		document.SetDefaultCatalogSort(restore.CatalogSort)
		document.SetDefaultCompression(restore.Compression)
		document.SetDefaultCreationDate(restore.CreationDate)
		document.SetDefaultModificationDate(restore.ModificationDate)
	})

	document.SetDefaultCompression(false)

	defaults := document.Defaults{
		Compression:      true,
		CreationDate:     time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC),
		ModificationDate: time.Date(2007, 8, 9, 10, 11, 12, 0, time.UTC),
	}
	out, err := outputWithDefaults(document.Options{}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/Filter /FlateDecode") {
		t.Fatal("expected explicit defaults to enable Flate compression")
	}
}

func TestNewWithDefaultsHonorsExplicitNoCompressionWithOptimize(t *testing.T) {
	defaults := document.DefaultSettings()
	defaults.Compression = false
	defaults.CreationDate = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	defaults.ModificationDate = time.Date(2007, 8, 9, 10, 11, 12, 0, time.UTC)

	out, err := outputWithDefaults(document.Options{Optimize: true}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "/Filter /FlateDecode") {
		t.Fatal("expected explicit no-compression default to override Optimize")
	}
}

func outputWithDefaults(options document.Options, defaults document.Defaults) (string, error) {
	pdf := document.NewWithDefaults(options, defaults)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "explicit defaults")
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
