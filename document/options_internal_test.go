// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"math"
	"testing"
)

func TestMustNewBestCompression(t *testing.T) {
	pdf := MustNew(WithBestCompression())
	if pdf.compressLevel != zlib.BestCompression {
		t.Fatalf("compressLevel = %d, want %d", pdf.compressLevel, zlib.BestCompression)
	}
	if !pdf.compress {
		t.Fatal("Optimize should leave compression enabled")
	}
}

func TestMustNewCompressionCanStillBeOverridden(t *testing.T) {
	pdf := MustNew(WithBestCompression())
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
	pdf, err := NewDocumentWithDefaults(Defaults{Compression: true}, WithResourceCachePolicy(ResourceCachePolicy(99)))
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
		Mode:                     CompressionEnabled,
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

func TestCompressionOptionsRespectCallOrder(t *testing.T) {
	bestSpeed := CompressionPolicy{Mode: CompressionEnabled, Level: zlib.BestSpeed}

	pdf := MustNew(WithBestCompression(), WithCompressionPolicy(bestSpeed))
	if got := pdf.CompressionPolicy().Level; got != zlib.BestSpeed {
		t.Fatalf("later WithCompressionPolicy level = %d, want BestSpeed", got)
	}

	pdf = MustNew(WithCompressionPolicy(bestSpeed), WithBestCompression())
	if got := pdf.CompressionPolicy().Level; got != zlib.BestCompression {
		t.Fatalf("later WithBestCompression level = %d, want BestCompression", got)
	}

	pdf = MustNew(WithBestCompression(), WithProductionPolicy(ServerSafePolicy()))
	if got := pdf.CompressionPolicy().Level; got != zlib.BestSpeed {
		t.Fatalf("later server policy level = %d, want BestSpeed", got)
	}

	pdf = MustNew(WithProductionPolicy(ServerSafePolicy()), WithBestCompression())
	if got := pdf.CompressionPolicy().Level; got != zlib.BestCompression {
		t.Fatalf("later WithBestCompression after server policy level = %d, want BestCompression", got)
	}
}

func TestDeterministicOutputOptionsRespectCallOrder(t *testing.T) {
	pdf := MustNew(WithDeterministicOutput(), WithOutputPolicy(OutputPolicy{}))
	if !pdf.creationDate.IsZero() {
		t.Fatalf("later non-deterministic output policy kept creation date %v", pdf.creationDate)
	}

	pdf = MustNew(WithOutputPolicy(OutputPolicy{}), WithDeterministicOutput())
	if pdf.creationDate.IsZero() {
		t.Fatal("later WithDeterministicOutput did not install a fixed creation date")
	}

	pdf = MustNew(WithProductionPolicy(DeterministicPolicy()), WithOutputPolicy(OutputPolicy{}))
	if !pdf.creationDate.IsZero() {
		t.Fatalf("later output policy did not disable production determinism: %v", pdf.creationDate)
	}
}

func TestCompressionPolicyPartialStructsDoNotDisableCompression(t *testing.T) {
	pdf, err := NewDocument(WithCompressionPolicy(CompressionPolicy{PageWorkers: 2}))
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	got := pdf.CompressionPolicy()
	if got.Mode != CompressionEnabled || got.Level != zlib.BestSpeed || got.PageWorkers != 2 {
		t.Fatalf("CompressionPolicy() = %#v, want enabled best-speed with 2 page workers", got)
	}

	pdf, err = NewDocument(WithCompressionPolicy(CompressionPolicy{Level: zlib.BestCompression}))
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	got = pdf.CompressionPolicy()
	if got.Mode != CompressionEnabled || got.Level != zlib.BestCompression || got.PageWorkers == 0 || got.AttachmentWorkers == 0 {
		t.Fatalf("CompressionPolicy() = %#v, want enabled best-compression with default workers", got)
	}
}

func TestCompressionPolicyCanExplicitlyDisableWorkers(t *testing.T) {
	pdf, err := NewDocument(WithCompressionPolicy(CompressionPolicy{
		Level:             zlib.BestSpeed,
		PageWorkers:       CompressionWorkersDisabled,
		AttachmentWorkers: CompressionWorkersDisabled,
	}))
	if err != nil {
		t.Fatalf("NewDocument returned error: %s", err)
	}
	got := pdf.CompressionPolicy()
	if got.Mode != CompressionEnabled || got.PageWorkers != 0 || got.AttachmentWorkers != 0 {
		t.Fatalf("CompressionPolicy() = %#v, want enabled compression with workers disabled", got)
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

func TestRuntimePolicyFromProductionPolicyResolvesOperationalDefaults(t *testing.T) {
	policy := ProductionPolicy{
		Limits:        ServerSafeLimits(),
		Deterministic: true,
	}
	runtime := runtimePolicyFromProductionPolicy(policy)
	if runtime.cachePolicy != ResourceCacheDocument {
		t.Fatalf("runtime cachePolicy = %v, want ResourceCacheDocument", runtime.cachePolicy)
	}
	if !runtime.compressionPolicySet {
		t.Fatal("runtime policy should carry explicit compression settings for production policies")
	}
	if !runtime.limitsSet || runtime.limits.MaxAttachmentBytes != ServerSafeLimits().MaxAttachmentBytes {
		t.Fatalf("runtime limits = %#v, want server-safe limits set", runtime.limits)
	}
	if !runtime.securityPolicySet {
		t.Fatal("runtime policy should install production security gates")
	}
	if !runtime.outputPolicySet {
		t.Fatal("runtime policy should carry output defaults")
	}
	if !runtime.hooksSet {
		t.Fatal("runtime policy should carry hook defaults")
	}
	if !runtime.deterministicOutput {
		t.Fatal("runtime policy should resolve deterministic output")
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

func TestMustNewUsesTypedConstructionOptions(t *testing.T) {
	pdf := MustNew(
		WithOrientation(OrientationLandscape),
		WithUnit(UnitInch),
		WithPageSize(PageSizeLetter),
	)
	width, height := pdf.GetPageSize()
	if math.Abs(width-11) > 1e-9 || math.Abs(height-8.5) > 1e-9 {
		t.Fatalf("page size = %.4f x %.4f, want 11 x 8.5", width, height)
	}
}
