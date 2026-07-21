// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPageStreamCompressorStopReleasesBlockedPublishers(t *testing.T) {
	compressed := make(chan struct{}, 8)
	pdf, err := NewDocument(
		WithCompressionPolicy(CompressionPolicy{
			Mode:                     CompressionEnabled,
			Level:                    zlib.BestSpeed,
			PageWorkers:              2,
			TinyStreamThresholdBytes: 1,
		}),
		WithHooks(Hooks{
			OnPageCompressed: func(page int, inputBytes, outputBytes int) {
				compressed <- struct{}{}
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	for page := 0; page < 8; page++ {
		pdf.AddPage()
		pdf.RawWriteStr(strings.Repeat("0 0 m 1 1 l S\n", 1024))
	}

	compressor := pdf.startPageStreamCompressor(pdf.page)
	if compressor == nil {
		t.Fatal("startPageStreamCompressor() = nil")
	}
	t.Cleanup(compressor.stop)
	select {
	case <-compressed:
		// The hook runs immediately before the unbuffered result publish, so at
		// least one worker is publishing or blocked trying to publish here.
	case <-time.After(5 * time.Second):
		t.Fatal("page compression did not start")
	}

	stopped := make(chan struct{})
	go func() {
		compressor.stop()
		compressor.stop() // Cleanup is safe when multiple return paths defer it.
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("page compressor cleanup did not release blocked publishers")
	}

	select {
	case _, ok := <-compressor.results:
		if ok {
			t.Fatal("page compressor published a result after cleanup")
		}
	default:
		t.Fatal("page compressor result channel is not closed after cleanup")
	}
}

func TestOutputStreamWriterFailureWithPageCompressionReturns(t *testing.T) {
	pdf, err := NewDocument(WithCompressionPolicy(CompressionPolicy{
		Mode:                     CompressionEnabled,
		Level:                    zlib.BestSpeed,
		PageWorkers:              2,
		TinyStreamThresholdBytes: 1,
	}))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	for page := 0; page < 8; page++ {
		pdf.AddPage()
		pdf.RawWriteStr(strings.Repeat("0 0 m 1 1 l S\n", 1024))
	}

	wantErr := errors.New("page stream writer failure")
	w := &failOnPDFStreamWriter{err: wantErr}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() {
		result <- pdf.OutputStreamContext(ctx, w)
	}()
	select {
	case err := <-result:
		if !errors.Is(err, wantErr) {
			t.Fatalf("OutputStreamContext() error = %v, want %v", err, wantErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OutputStreamContext() did not clean up page compression after writer failure")
	}
	if !w.failed {
		t.Fatal("test writer did not reach a page stream")
	}
}

type failOnPDFStreamWriter struct {
	err    error
	failed bool
}

func (w *failOnPDFStreamWriter) Write(p []byte) (int, error) {
	if string(p) == "stream" {
		w.failed = true
		return 0, w.err
	}
	return len(p), nil
}
