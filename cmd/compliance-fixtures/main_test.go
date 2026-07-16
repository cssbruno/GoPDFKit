// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestBaseDocumentProducesDeterministicUnsignedBytes(t *testing.T) {
	t.Parallel()

	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot() error = %v", err)
	}
	fontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed.ttf")
	boldFontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed-Bold.ttf")
	render := func() []byte {
		pdf := baseDocument(fontPath, boldFontPath)
		pdf.SetTitle("deterministic compliance fixture", false)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.Write(6, "stable content")
		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		return output.Bytes()
	}

	first := render()
	second := render()
	if !bytes.Equal(first, second) {
		t.Fatal("deterministic compliance base changed between identical renders")
	}
}
