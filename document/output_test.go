// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
)

type nilOutputWriter struct{}

func (*nilOutputWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (*nilOutputWriter) Close() error {
	return nil
}

type failingOutputWriter struct{}

func (failingOutputWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestOutputRejectsNilWriter(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	if err := pdf.Output(nil); !errors.Is(err, document.ErrNilWriter) {
		t.Fatalf("Output(nil) error = %v, want ErrNilWriter", err)
	}
}

func TestOutputRejectsTypedNilWriter(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")
	var w *nilOutputWriter

	if err := pdf.Output(w); !errors.Is(err, document.ErrNilWriter) {
		t.Fatalf("Output(typed nil) error = %v, want ErrNilWriter", err)
	}
}

func TestOutputAndCloseRejectsNilWriter(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	if err := pdf.OutputAndClose(nil); !errors.Is(err, document.ErrNilWriter) {
		t.Fatalf("OutputAndClose(nil) error = %v, want ErrNilWriter", err)
	}
}

func TestOutputIsIdempotent(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	var first bytes.Buffer
	if err := pdf.Output(&first); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := pdf.Output(&second); err != nil {
		t.Fatal(err)
	}
	if first.Len() == 0 {
		t.Fatal("first output is empty")
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("second Output call differed from first")
	}
}

func TestOutputWriterFailureDoesNotPoisonDocument(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	if err := pdf.Output(failingOutputWriter{}); err == nil {
		t.Fatal("Output() error = nil, want writer error")
	}
	var retry bytes.Buffer
	if err := pdf.Output(&retry); err != nil {
		t.Fatalf("retry Output() error = %v", err)
	}
	if retry.Len() == 0 {
		t.Fatal("retry output is empty")
	}
}

func TestOutputDefaultTrailerOmitsFileID(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "/ID ") {
		t.Fatal("default output trailer contains /ID")
	}
}

func TestOutputEncryptedTrailerKeepsEmptyFileID(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")
	if err := pdf.SetLegacyProtection(document.CnProtectPrint, "reader", "owner"); err != nil {
		t.Fatalf("SetLegacyProtection() error = %v", err)
	}
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/ID [()()]") {
		t.Fatal("encrypted output trailer did not contain empty /ID")
	}
}

func TestOutputArlingtonTrailerKeepsHashFileID(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")
	pdf.SetComplianceMetadata(document.ComplianceMetadata{Arlington: true})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/ID [<") {
		t.Fatal("Arlington output trailer did not contain hash /ID")
	}
}

func TestOutputFileAndCloseNoSyncWritesPDF(t *testing.T) {
	fileStr := filepath.Join(t.TempDir(), "out.pdf")

	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	if err := pdf.OutputFileAndCloseNoSync(fileStr); err != nil {
		t.Fatalf("OutputFileAndCloseNoSync() error = %v", err)
	}
	got, err := os.ReadFile(fileStr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("OutputFileAndCloseNoSync() wrote non-PDF prefix %q", got[:min(len(got), 8)])
	}
}

func TestOutputFileAndCloseWithOptionsZeroValueWritesPDF(t *testing.T) {
	fileStr := filepath.Join(t.TempDir(), "out.pdf")

	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	if err := pdf.OutputFileAndCloseWithOptions(fileStr, document.OutputFileOptions{}); err != nil {
		t.Fatalf("OutputFileAndCloseWithOptions() error = %v", err)
	}
	if info, err := os.Stat(fileStr); err != nil {
		t.Fatal(err)
	} else if info.Size() == 0 {
		t.Fatal("OutputFileAndCloseWithOptions() wrote empty file")
	}
}

func TestOutputFileAndCloseDoesNotTruncateOnCloseValidationError(t *testing.T) {
	fileStr := filepath.Join(t.TempDir(), "out.pdf")
	original := []byte("previous valid output")
	if err := os.WriteFile(fileStr, original, 0o644); err != nil {
		t.Fatal(err)
	}

	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.ClipRect(10, 10, 20, 20, false)

	err := pdf.OutputFileAndClose(fileStr)
	if err == nil || !strings.Contains(err.Error(), "clip procedure must be explicitly ended") {
		t.Fatalf("OutputFileAndClose() error = %v, want open clip validation error", err)
	}
	got, err := os.ReadFile(fileStr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("OutputFileAndClose() changed destination on failure: got %q, want %q", got, original)
	}
}

func TestOutputFileAndCloseNewFileIsReadableByGroupAndOthers(t *testing.T) {
	fileStr := filepath.Join(t.TempDir(), "out.pdf")

	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	if err := pdf.OutputFileAndClose(fileStr); err != nil {
		t.Fatalf("OutputFileAndClose() error = %v", err)
	}
	info, err := os.Stat(fileStr)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Fatalf("output file mode = %v, want %v", got, want)
	}
}

func TestOutputFileAndClosePreservesExistingFileMode(t *testing.T) {
	fileStr := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(fileStr, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 10, "hello")

	if err := pdf.OutputFileAndClose(fileStr); err != nil {
		t.Fatalf("OutputFileAndClose() error = %v", err)
	}
	info, err := os.Stat(fileStr)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("output file mode = %v, want %v", got, want)
	}
}
