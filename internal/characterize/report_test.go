// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package characterize

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/testsupport/example"
)

func TestBuildProducesDeterministicStructuralAndTextEvidence(t *testing.T) {
	pdf := document.MustNew(document.WithUnit(document.UnitPoint), document.WithNoCompression(), document.WithDeterministicOutput())
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Lang: "en-US", Title: "Characterization evidence"})
	pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 10)
	pdf.BeginStructure("P")
	pdf.Text(20, 20, "baseline one")
	pdf.EndStructure()
	pdf.LinkString(20, 25, 40, 10, "https://example.test/evidence")
	pdf.AddPage()
	pdf.Text(20, 20, "baseline two")
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatal(err)
	}
	artifacts := []Artifact{{Name: "z-last", PDF: output.Bytes()}, {Name: "a-first", PDF: append([]byte(nil), output.Bytes()...)}}
	fingerprint := Fingerprint{GOOS: "test", GOARCH: "test", GoVersion: "go-test", CPUs: 4}
	first, err := Build(context.Background(), artifacts, "go test ./...", fingerprint, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(context.Background(), artifacts, "go test ./...", fingerprint, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := first.CanonicalJSON(DefaultLimits().MaxJSONBytes)
	secondJSON, _ := second.CanonicalJSON(DefaultLimits().MaxJSONBytes)
	if !bytes.Equal(firstJSON, secondJSON) || first.Fixtures[0].Name != "a-first" {
		t.Fatalf("non-deterministic report:\n%s\n%s", firstJSON, secondJSON)
	}
	evidence := first.Fixtures[0]
	if evidence.Pages != 2 || !strings.Contains(evidence.Text, "baseline one") ||
		!strings.Contains(evidence.PageText[1], "baseline two") || evidence.Structure.LinkAnnotations != 1 ||
		evidence.Structure.URIActions != 1 || evidence.Structure.StructureTrees == 0 || !evidence.Structure.HasTagMarkInfo {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestBuildRejectsLimitsDuplicatesAndCancellationAtomically(t *testing.T) {
	valid := []byte("%PDF-1.4\n")
	fingerprint := Fingerprint{GOOS: "test", GOARCH: "test", GoVersion: "go-test", CPUs: 1}
	if _, err := Build(context.Background(), []Artifact{{Name: "same", PDF: valid}, {Name: "same", PDF: valid}}, "test", fingerprint, DefaultLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate error = %v", err)
	}
	limits := DefaultLimits()
	limits.MaxPDFBytes = 1
	if _, err := Build(context.Background(), []Artifact{{Name: "x", PDF: valid}}, "test", fingerprint, limits); !errors.Is(err, ErrLimit) {
		t.Fatalf("limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, []Artifact{{Name: "x", PDF: valid}}, "test", fingerprint, DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
	if _, err := Build(context.Background(), []Artifact{{Name: "x", PDF: valid}}, "test", Fingerprint{}, DefaultLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty fingerprint error = %v", err)
	}
}
