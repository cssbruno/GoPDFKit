// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"testing"
)

func TestNewWithOptionsOptimizeSetsBestCompression(t *testing.T) {
	pdf := NewWithOptions(Options{Optimize: true})
	if pdf.compressLevel != zlib.BestCompression {
		t.Fatalf("compressLevel = %d, want %d", pdf.compressLevel, zlib.BestCompression)
	}
	if !pdf.compress {
		t.Fatal("Optimize should leave compression enabled")
	}
}

func TestNewWithOptionsDefaultCompressionCanStillBeOverridden(t *testing.T) {
	pdf := NewWithOptions(Options{Optimize: true})
	pdf.SetNoCompression()
	if pdf.compressLevel != zlib.NoCompression {
		t.Fatalf("compressLevel = %d, want %d", pdf.compressLevel, zlib.NoCompression)
	}
	if pdf.compress {
		t.Fatal("SetNoCompression should disable compression after Optimize")
	}
}
