// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package example_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/testsupport/example"
)

// ExampleFilename demonstrates Filename and Summary error output.
func ExampleFilename() {
	fileStr := example.Filename("example")
	example.Summary(errors.New("printer on fire"), fileStr)
	// Output:
	// printer on fire
}

func TestPathsDoNotDependOnWorkingDirectory(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	imagePath := example.ImageFile("logo.png")
	if _, err := os.Stat(imagePath); err != nil {
		t.Fatalf("ImageFile() path is not usable outside repo cwd: %v", err)
	}
	if got := filepath.Base(example.Filename("sample")); got != "sample.pdf" {
		t.Fatalf("Filename() base = %q, want sample.pdf", got)
	}
	if _, err := os.Stat(example.RepoFile("LICENSE")); err != nil {
		t.Fatalf("RepoFile() path is not usable outside repo cwd: %v", err)
	}
}

func TestSummaryUsesStableGeneratedPDFPath(t *testing.T) {
	output := captureStdout(t, func() {
		example.Summary(nil, example.Filename("sample"))
	})
	want := "Successfully generated assets/generated/pdf/sample.pdf\n"
	if output != want {
		t.Fatalf("Summary() = %q, want %q", output, want)
	}
}

func TestGeneratedPDFsStayOutsideRepository(t *testing.T) {
	output := example.Filename("isolated")
	rel, err := filepath.Rel(example.RepoFile(), output)
	if err != nil {
		t.Fatalf("relative output path: %v", err)
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("example output %q is inside repository %q", output, example.RepoFile())
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = old
	})

	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	return buf.String()
}
