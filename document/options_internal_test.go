// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"math"
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

func TestNewDocumentReturnsConstructorError(t *testing.T) {
	pdf, err := NewDocument(WithUnit(Unit("parsec")))
	if err == nil {
		t.Fatal("expected constructor error for invalid unit")
	}
	if pdf != nil {
		t.Fatalf("pdf = %#v, want nil on constructor error", pdf)
	}
}

func TestNewDocumentReturnsCachePolicyError(t *testing.T) {
	pdf, err := NewDocument(WithResourceCachePolicy(ResourceCachePolicy(99)))
	if err == nil {
		t.Fatal("expected constructor error for invalid cache policy")
	}
	if pdf != nil {
		t.Fatalf("pdf = %#v, want nil on constructor error", pdf)
	}
}

func TestNewDocumentWithDefaultsReturnsCachePolicyError(t *testing.T) {
	pdf, err := NewDocumentWithDefaults(Options{CachePolicy: ResourceCachePolicy(99)}, Defaults{Compression: true})
	if err == nil {
		t.Fatal("expected constructor error for invalid cache policy")
	}
	if pdf != nil {
		t.Fatalf("pdf = %#v, want nil on constructor error", pdf)
	}
}

func TestNewDocumentFunctionalOptions(t *testing.T) {
	pdf, err := NewDocument(
		WithOrientation(OrientationLandscape),
		WithUnit(UnitInch),
		WithPageSize(PageSizeLetter),
		WithBestCompression(),
	)
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	if pdf.compressLevel != zlib.BestCompression {
		t.Fatalf("compressLevel = %d, want %d", pdf.compressLevel, zlib.BestCompression)
	}
	width, height := pdf.GetPageSize()
	if math.Abs(width-11) > 1e-9 || math.Abs(height-8.5) > 1e-9 {
		t.Fatalf("page size = %.4f x %.4f, want 11 x 8.5", width, height)
	}
}

func TestNewWithOptionsPrefersTypedFields(t *testing.T) {
	pdf := NewWithOptions(Options{
		Orientation:    OrientationLandscape,
		Unit:           UnitInch,
		PageSize:       PageSizeLetter,
		OrientationStr: "P",
		UnitStr:        "mm",
		SizeStr:        "A4",
	})
	if err := pdf.Error(); err != nil {
		t.Fatalf("NewWithOptions returned error: %s", err)
	}
	width, height := pdf.GetPageSize()
	if math.Abs(width-11) > 1e-9 || math.Abs(height-8.5) > 1e-9 {
		t.Fatalf("page size = %.4f x %.4f, want 11 x 8.5", width, height)
	}
}
