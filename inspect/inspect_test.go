// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package inspect

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
)

func TestInspectGeneratedPDF(t *testing.T) {
	pdfBytes := inspectTestPDF(t)

	if err := ValidateStructure(pdfBytes); err != nil {
		t.Fatalf("ValidateStructure() error = %v", err)
	}

	count, err := PageCount(pdfBytes)
	if err != nil {
		t.Fatalf("PageCount() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("PageCount() = %d, want 2", count)
	}

	width, height, err := FirstPageSizePoints(pdfBytes)
	if err != nil {
		t.Fatalf("FirstPageSizePoints() error = %v", err)
	}
	if width <= 0 || height <= 0 {
		t.Fatalf("FirstPageSizePoints() = %f, %f, want positive dimensions", width, height)
	}

	text, err := Text(pdfBytes)
	if err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if !strings.Contains(text, "Inspect page one") || !strings.Contains(text, "Inspect page two") {
		t.Fatalf("Text() = %q, want both page strings", text)
	}

	pageText, err := PageText(pdfBytes, 2)
	if err != nil {
		t.Fatalf("PageText() error = %v", err)
	}
	if !strings.Contains(pageText, "Inspect page two") {
		t.Fatalf("PageText() = %q, want second page string", pageText)
	}

	streams, err := DecodedStreams(pdfBytes)
	if err != nil {
		t.Fatalf("DecodedStreams() error = %v", err)
	}
	if len(streams) == 0 {
		t.Fatal("DecodedStreams() returned no streams")
	}
}

func TestInspectContextCanceled(t *testing.T) {
	pdfBytes := inspectTestPDF(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := ValidateStructureContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateStructureContext() error = %v, want context.Canceled", err)
	}
	if _, err := PageCountContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("PageCountContext() error = %v, want context.Canceled", err)
	}
	if _, err := TextContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("TextContext() error = %v, want context.Canceled", err)
	}
	if _, err := PageTextContext(ctx, pdfBytes, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("PageTextContext() error = %v, want context.Canceled", err)
	}
	if _, err := DecodedStreamsContext(ctx, pdfBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodedStreamsContext() error = %v, want context.Canceled", err)
	}
}

func TestFirstPageSizePointsUsesMediaBoxDimensions(t *testing.T) {
	width, height, err := FirstPageSizePoints([]byte("/MediaBox [-10.5 20 110 220]"))
	if err != nil {
		t.Fatalf("FirstPageSizePoints() error = %v", err)
	}
	if width != 120.5 || height != 200 {
		t.Fatalf("FirstPageSizePoints() = %v, %v; want 120.5, 200", width, height)
	}
}

func TestDecodedStreamsEnforcesAggregateLimits(t *testing.T) {
	data := []byte("<<>>\nstream\nabc\nendstream\n<<>>\nstream\ndef\nendstream\n")
	if _, err := decodedStreamsContext(context.Background(), data, 3, 5, 2); err == nil || !strings.Contains(err.Error(), "decoded pdf streams exceed") {
		t.Fatalf("decodedStreamsContext() aggregate limit error = %v", err)
	}
	if _, err := decodedStreamsContext(context.Background(), data, 3, 6, 1); err == nil || !strings.Contains(err.Error(), "stream count") {
		t.Fatalf("decodedStreamsContext() stream-count limit error = %v", err)
	}
}

func inspectTestPDF(t *testing.T) []byte {
	t.Helper()

	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "Inspect page one")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "Inspect page two")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	return out.Bytes()
}
