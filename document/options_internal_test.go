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
		WithPageCompressionWorkers(2),
		WithAttachmentCompressionWorkers(1),
	)
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	if pdf.compressLevel != zlib.BestCompression {
		t.Fatalf("compressLevel = %d, want %d", pdf.compressLevel, zlib.BestCompression)
	}
	if pdf.pageCompressionWorkers != 2 {
		t.Fatalf("pageCompressionWorkers = %d, want 2", pdf.pageCompressionWorkers)
	}
	if pdf.attachmentCompressionWorkers != 1 {
		t.Fatalf("attachmentCompressionWorkers = %d, want 1", pdf.attachmentCompressionWorkers)
	}
	width, height := pdf.GetPageSize()
	if math.Abs(width-11) > 1e-9 || math.Abs(height-8.5) > 1e-9 {
		t.Fatalf("page size = %.4f x %.4f, want 11 x 8.5", width, height)
	}
}

func TestCompressionWorkerOptionsCanDisableBackgroundWork(t *testing.T) {
	pdf, err := NewDocument(
		WithPageCompressionWorkers(0),
		WithAttachmentCompressionWorkers(0),
	)
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	if pdf.pageCompressionWorkers != 0 {
		t.Fatalf("pageCompressionWorkers = %d, want 0", pdf.pageCompressionWorkers)
	}
	if pdf.attachmentCompressionWorkers != 0 {
		t.Fatalf("attachmentCompressionWorkers = %d, want 0", pdf.attachmentCompressionWorkers)
	}
}

func TestCompressionPolicyOptionConfiguresAllFields(t *testing.T) {
	policy := CompressionPolicy{
		Enabled:                  true,
		Level:                    zlib.BestCompression,
		PageWorkers:              3,
		AttachmentWorkers:        2,
		TinyStreamThresholdBytes: 128,
	}
	pdf, err := NewDocument(WithCompressionPolicy(policy))
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	if got := pdf.CompressionPolicy(); got != policy {
		t.Fatalf("CompressionPolicy() = %#v, want %#v", got, policy)
	}
}

func TestNoCompressionOptionDisablesCompression(t *testing.T) {
	pdf, err := NewDocument(WithNoCompression())
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	if pdf.compress || pdf.compressLevel != zlib.NoCompression {
		t.Fatalf("compression = %v level %d, want disabled", pdf.compress, pdf.compressLevel)
	}
}

func TestCompressionWorkerOptionsRejectNegativeValues(t *testing.T) {
	pdf, err := NewDocument(WithPageCompressionWorkers(-1))
	if err == nil {
		t.Fatal("expected page worker constructor error")
	}
	if pdf != nil {
		t.Fatalf("pdf = %#v, want nil on constructor error", pdf)
	}

	pdf, err = NewDocument(WithAttachmentCompressionWorkers(-1))
	if err == nil {
		t.Fatal("expected attachment worker constructor error")
	}
	if pdf != nil {
		t.Fatalf("pdf = %#v, want nil on constructor error", pdf)
	}
}

func TestNewDocumentFontCacheOptions(t *testing.T) {
	cache := NewFontCache()
	pdf, err := NewDocument(WithFontCache(cache))
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	if pdf.fontCache != cache {
		t.Fatal("WithFontCache did not install explicit font cache")
	}

	pdf, err = NewDocument(WithResourceCachePolicy(ResourceCacheDocument))
	if err != nil {
		t.Fatalf("NewDocument(document cache) returned error: %s", err)
	}
	if pdf.fontCache == nil {
		t.Fatal("ResourceCacheDocument should create a document-local font cache")
	}

	pdf, err = NewDocument(WithResourceCachePolicy(ResourceCacheDisabled))
	if err != nil {
		t.Fatalf("NewDocument(disabled cache) returned error: %s", err)
	}
	if pdf.fontCache != nil {
		t.Fatal("ResourceCacheDisabled should not install a font cache")
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
